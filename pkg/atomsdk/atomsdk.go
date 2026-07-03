// Copyright (c) Abstract Machines
// SPDX-License-Identifier: Apache-2.0

package atomsdk

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	loginPath = "/auth/login"

	createTenantMutation = `mutation($name:String!){
		createTenant(input:{name:$name}){id name}
	}`

	tenantsQuery = `query{
		tenants(limit:100){items{id name}}
	}`

	createServiceEntityMutation = `mutation($tid:ID!,$name:String!){
		createEntity(input:{kind:service,name:$name,tenantId:$tid,attributes:{}}){id}
	}`

	createSharedKeyMutation = `mutation($eid:ID!,$desc:String!){
		createSharedKey(entityId:$eid,input:{description:$desc}){credentialId key}
	}`

	createResourceMutation = `mutation($tid:ID!,$name:String!){
		createResource(input:{kind:"channel",name:$name,tenantId:$tid,attributes:{}}){id}
	}`

	actionsQuery = `query{
		actions(limit:100,offset:0){items{id name}}
	}`

	createPermissionBlockMutation = `mutation($tid:ID!,$cid:ID!,$aid1:ID!,$aid2:ID!){
		createPermissionBlock(input:{
			tenantId:$tid,
			scopeMode:"object",
			objectKind:"resource",
			objectType:"resource:channel",
			objectId:$cid,
			effect:allow,
			actionIds:[$aid1,$aid2]
		}){id}
	}`

	createDirectPolicyMutation = `mutation($tid:ID!,$sid:ID!,$pbid:ID!){
		createDirectPolicy(input:{tenantId:$tid,subjectKind:entity,subjectId:$sid,permissionBlockId:$pbid}){id}
	}`
)

const (
	entityKindService = "service"
	defaultTimeout    = 30 * time.Second
)

type Config struct {
	AtomURL string
}

type SDK interface {
	Login(ctx context.Context, identifier, secret string) (string, error)
	CreateTenant(ctx context.Context, name, token string) (string, error)
	EnsureTenant(ctx context.Context, name, token string) (string, error)
	CreateServiceEntity(ctx context.Context, name, tenantID, token string) (string, error)
	CreateAPIKey(ctx context.Context, entityID, description, token string) (string, error)
	CreateResource(ctx context.Context, name, tenantID, token string) (string, error)
	Connect(ctx context.Context, entityID, resourceID, tenantID, token string) error
}

type sdk struct {
	cfg    Config
	client *http.Client
}

func New(cfg Config) SDK {
	return &sdk{
		cfg: cfg,
		client: &http.Client{
			Timeout: defaultTimeout,
		},
	}
}

type graphQLRequest struct {
	Query     string         `json:"query"`
	Variables map[string]any `json:"variables,omitempty"`
}

type graphQLResponse struct {
	Data   map[string]json.RawMessage `json:"data"`
	Errors []graphQLError             `json:"errors,omitempty"`
}

type graphQLError struct {
	Message string `json:"message"`
}

type loginResponse struct {
	Token     string    `json:"token"`
	EntityID  string    `json:"entity_id"`
	ExpiresAt time.Time `json:"expires_at"`
}

func (s *sdk) gql(ctx context.Context, query string, vars map[string]any, token string) (map[string]json.RawMessage, error) {
	body, err := json.Marshal(graphQLRequest{Query: query, Variables: vars})
	if err != nil {
		return nil, fmt.Errorf("marshal graphql request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.cfg.AtomURL+"/graphql", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create graphql request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("graphql request: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read graphql response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("graphql request failed with status %d: %s", resp.StatusCode, string(raw))
	}

	var gqlResp graphQLResponse
	if err := json.Unmarshal(raw, &gqlResp); err != nil {
		return nil, fmt.Errorf("unmarshal graphql response: %w", err)
	}

	if len(gqlResp.Errors) > 0 {
		msgs := make([]string, 0, len(gqlResp.Errors))
		for _, e := range gqlResp.Errors {
			msgs = append(msgs, e.Message)
		}
		return nil, fmt.Errorf("graphql error: %v", msgs)
	}

	return gqlResp.Data, nil
}

func (s *sdk) Login(ctx context.Context, identifier, secret string) (string, error) {
	payload, err := json.Marshal(map[string]string{
		"identifier": identifier,
		"secret":     secret,
		"kind":       "password",
	})
	if err != nil {
		return "", fmt.Errorf("marshal login request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.cfg.AtomURL+loginPath, bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("create login request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("login request: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read login response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("login failed with status %d: %s", resp.StatusCode, string(raw))
	}

	var lr loginResponse
	if err := json.Unmarshal(raw, &lr); err != nil {
		return "", fmt.Errorf("unmarshal login response: %w", err)
	}

	if lr.Token == "" {
		return "", fmt.Errorf("login returned empty token")
	}

	return lr.Token, nil
}

func (s *sdk) CreateTenant(ctx context.Context, name, token string) (string, error) {
	data, err := s.gql(ctx, createTenantMutation, map[string]any{
		"name": name,
	}, token)
	if err != nil {
		return "", err
	}

	var tenant struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(data["createTenant"], &tenant); err != nil {
		return "", fmt.Errorf("unmarshal createTenant response: %w", err)
	}

	return tenant.ID, nil
}

func (s *sdk) EnsureTenant(ctx context.Context, name, token string) (string, error) {
	data, err := s.gql(ctx, tenantsQuery, nil, token)
	if err != nil {
		return "", fmt.Errorf("query tenants: %w", err)
	}

	var tenants struct {
		Items []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"items"`
	}
	if err := json.Unmarshal(data["tenants"], &tenants); err != nil {
		return "", fmt.Errorf("unmarshal tenants response: %w", err)
	}

	for _, t := range tenants.Items {
		if t.Name == name {
			return t.ID, nil
		}
	}

	return s.CreateTenant(ctx, name, token)
}

func (s *sdk) CreateServiceEntity(ctx context.Context, name, tenantID, token string) (string, error) {
	data, err := s.gql(ctx, createServiceEntityMutation, map[string]any{
		"tid":  tenantID,
		"name": name,
	}, token)
	if err != nil {
		return "", err
	}

	var entity struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(data["createEntity"], &entity); err != nil {
		return "", fmt.Errorf("unmarshal createEntity response: %w", err)
	}

	if entity.ID == "" {
		return "", fmt.Errorf("createEntity returned empty id")
	}

	return entity.ID, nil
}

func (s *sdk) CreateAPIKey(ctx context.Context, entityID, description, token string) (string, error) {
	data, err := s.gql(ctx, createSharedKeyMutation, map[string]any{
		"eid":  entityID,
		"desc": description,
	}, token)
	if err != nil {
		return "", err
	}

	var key struct {
		Key string `json:"key"`
	}
	if err := json.Unmarshal(data["createSharedKey"], &key); err != nil {
		return "", fmt.Errorf("unmarshal createSharedKey response: %w", err)
	}

	if key.Key == "" {
		return "", fmt.Errorf("createSharedKey returned empty key")
	}

	return key.Key, nil
}

func (s *sdk) CreateResource(ctx context.Context, name, tenantID, token string) (string, error) {
	data, err := s.gql(ctx, createResourceMutation, map[string]any{
		"tid":  tenantID,
		"name": name,
	}, token)
	if err != nil {
		return "", err
	}

	var resource struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(data["createResource"], &resource); err != nil {
		return "", fmt.Errorf("unmarshal createResource response: %w", err)
	}

	if resource.ID == "" {
		return "", fmt.Errorf("createResource returned empty id")
	}

	return resource.ID, nil
}

func (s *sdk) Connect(ctx context.Context, entityID, resourceID, tenantID, token string) error {
	data, err := s.gql(ctx, actionsQuery, nil, token)
	if err != nil {
		return fmt.Errorf("query actions: %w", err)
	}

	var actions struct {
		Items []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"items"`
	}
	if err := json.Unmarshal(data["actions"], &actions); err != nil {
		return fmt.Errorf("unmarshal actions response: %w", err)
	}

	actionIDs := make(map[string]string)
	for _, a := range actions.Items {
		actionIDs[a.Name] = a.ID
	}
	pubID, ok := actionIDs["publish"]
	if !ok {
		return fmt.Errorf("action %q not found", "publish")
	}
	subID, ok := actionIDs["subscribe"]
	if !ok {
		return fmt.Errorf("action %q not found", "subscribe")
	}

	pbData, err := s.gql(ctx, createPermissionBlockMutation, map[string]any{
		"tid":  tenantID,
		"cid":  resourceID,
		"aid1": pubID,
		"aid2": subID,
	}, token)
	if err != nil {
		return fmt.Errorf("create permission block: %w", err)
	}

	var pb struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(pbData["createPermissionBlock"], &pb); err != nil {
		return fmt.Errorf("unmarshal createPermissionBlock response: %w", err)
	}

	_, err = s.gql(ctx, createDirectPolicyMutation, map[string]any{
		"tid":  tenantID,
		"sid":  entityID,
		"pbid": pb.ID,
	}, token)
	if err != nil {
		return fmt.Errorf("create direct policy: %w", err)
	}

	return nil
}
