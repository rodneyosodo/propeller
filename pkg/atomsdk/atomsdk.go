package atomsdk

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"sync"
	"time"
)

const (
	loginPath = "/auth/login"

	createEntityMutation = `mutation($tid:ID!,$name:String!){
		createEntity(input:{kind:service,name:$name,tenantId:$tid,attributes:{}}){id}
	}`

	createAccessTokenMutation = `mutation($eid:ID!,$desc:String!,$name:String!){
		createAccessToken(input:{
			subjectId:$eid,
			name:$name,
			description:$desc,
			scoped:false,
			permissions:[]
		}){credentialId token}
	}`

	createResourceMutation = `mutation($tid:ID!,$name:String!){
		createResource(input:{kind:"channel",name:$name,tenantId:$tid,attributes:{}}){id}
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

	deleteEntityMutation = `mutation($id:ID!){
		deleteEntity(id:$id)
	}`

	deleteResourceMutation = `mutation($id:ID!){
		deleteResource(id:$id)
	}`

	createTenantMutation = `mutation($name:String!){
		createTenant(input:{name:$name}){id name}
	}`

	tenantsQuery = `query{
		tenants(limit:100){items{id name}}
	}`

	actionsQuery = `query{
		actions(limit:100,offset:0){items{id name}}
	}`
)

const (
	entityKindService = "service"
	defaultTimeout    = 30 * time.Second
)

type Config struct {
	AtomURL string
	Token   string
}

type SDK interface {
	Login(ctx context.Context, identifier, secret string) (string, error)
	CreateTenant(ctx context.Context, name, token string) (string, error)
	EnsureTenant(ctx context.Context, name, token string) (string, error)
	CreateServiceEntity(ctx context.Context, name, tenantID, token string) (string, error)
	CreateAPIKey(ctx context.Context, entityID, description, token string) (string, error)
	CreateResource(ctx context.Context, name, tenantID, token string) (string, error)
	Connect(ctx context.Context, entityID, resourceID, tenantID, token string) error
	DeleteEntity(ctx context.Context, id, token string) error
	DeleteResource(ctx context.Context, id, token string) error
}

type Entity struct {
	ID string
}

type Resource struct {
	ID string
}

type sdk struct {
	cfg       Config
	client    *http.Client
	actionIDs map[string]string
	mu        sync.RWMutex
}

func New(cfg Config) SDK {
	return &sdk{
		cfg: cfg,
		client: &http.Client{
			Timeout: defaultTimeout,
		},
		actionIDs: make(map[string]string),
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
		return "", errors.New("login returned empty token")
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
	data, err := s.gql(ctx, createEntityMutation, map[string]any{
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
		return "", errors.New("createEntity returned empty id")
	}

	return entity.ID, nil
}

func (s *sdk) CreateAPIKey(ctx context.Context, entityID, description, token string) (string, error) {
	data, err := s.gql(ctx, createAccessTokenMutation, map[string]any{
		"eid":  entityID,
		"desc": description,
		"name": description,
	}, token)
	if err != nil {
		return "", err
	}

	var key struct {
		CredentialID string `json:"credentialId"`
		Token        string `json:"token"`
	}
	if err := json.Unmarshal(data["createAccessToken"], &key); err != nil {
		return "", fmt.Errorf("unmarshal createAccessToken response: %w", err)
	}
	if key.Token == "" {
		return "", errors.New("createAccessToken returned empty token")
	}

	return key.Token, nil
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
		return "", errors.New("createResource returned empty id")
	}

	return resource.ID, nil
}

func (s *sdk) Connect(ctx context.Context, entityID, resourceID, tenantID, token string) error {
	actionIDs, err := s.findActionIDs(ctx, token, "publish", "subscribe")
	if err != nil {
		return fmt.Errorf("lookup action ids: %w", err)
	}

	pbData, err := s.gql(ctx, createPermissionBlockMutation, map[string]any{
		"tid":  tenantID,
		"cid":  resourceID,
		"aid1": actionIDs["publish"],
		"aid2": actionIDs["subscribe"],
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

func (s *sdk) DeleteEntity(ctx context.Context, id, token string) error {
	_, err := s.gql(ctx, deleteEntityMutation, map[string]any{
		"id": id,
	}, token)

	return err
}

func (s *sdk) DeleteResource(ctx context.Context, id, token string) error {
	_, err := s.gql(ctx, deleteResourceMutation, map[string]any{
		"id": id,
	}, token)

	return err
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

func (s *sdk) findActionIDs(ctx context.Context, token string, names ...string) (map[string]string, error) {
	s.mu.RLock()
	allCached := true
	for _, n := range names {
		if _, ok := s.actionIDs[n]; !ok {
			allCached = false
			break
		}
	}
	if allCached {
		result := make(map[string]string, len(s.actionIDs))
		maps.Copy(result, s.actionIDs)
		s.mu.RUnlock()

		return result, nil
	}
	s.mu.RUnlock()

	s.mu.Lock()
	defer s.mu.Unlock()

	allCached = true
	for _, n := range names {
		if _, ok := s.actionIDs[n]; !ok {
			allCached = false
			break
		}
	}
	if allCached {
		result := make(map[string]string, len(s.actionIDs))
		maps.Copy(result, s.actionIDs)

		return result, nil
	}

	data, err := s.gql(ctx, actionsQuery, nil, token)
	if err != nil {
		return nil, fmt.Errorf("query actions: %w", err)
	}

	var actions struct {
		Items []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"items"`
	}
	if err := json.Unmarshal(data["actions"], &actions); err != nil {
		return nil, fmt.Errorf("unmarshal actions response: %w", err)
	}

	for _, a := range actions.Items {
		if a.ID != "" && a.Name != "" {
			s.actionIDs[a.Name] = a.ID
		}
	}

	for _, n := range names {
		if _, ok := s.actionIDs[n]; !ok {
			return nil, fmt.Errorf("action %q not found on server", n)
		}
	}

	result := make(map[string]string, len(s.actionIDs))
	maps.Copy(result, s.actionIDs)

	return result, nil
}
