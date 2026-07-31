package sdk

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const flEndpoint = "/fl"

const (
	flExperimentsEndpoint = flEndpoint + "/experiments"
	flTaskEndpoint        = flEndpoint + "/task"
	flUpdateEndpoint      = flEndpoint + "/update"
	flUpdateCBOREndpoint  = flEndpoint + "/update_cbor"
	flRoundsEndpoint      = flEndpoint + "/rounds"
)

// ExperimentConfig mirrors the manager FL experiment configuration request.
type ExperimentConfig struct {
	ExperimentID  string         `json:"experiment_id"`
	RoundID       string         `json:"round_id"`
	ModelRef      string         `json:"model_ref"`
	Participants  []string       `json:"participants"`
	Hyperparams   map[string]any `json:"hyperparams"`
	KOfN          int            `json:"k_of_n"`
	TimeoutS      int            `json:"timeout_s"`
	TaskWasmImage string         `json:"task_wasm_image,omitempty"`
}

// ExperimentResult mirrors the manager configure-experiment response.
type ExperimentResult struct {
	ExperimentID string `json:"experiment_id"`
	RoundID      string `json:"round_id"`
	Status       string `json:"status"`
}

// FLTask mirrors the manager FL task response.
type FLTask struct {
	RoundID     string         `json:"round_id"`
	ModelRef    string         `json:"model_ref"`
	Config      map[string]any `json:"config"`
	Hyperparams map[string]any `json:"hyperparams,omitempty"`
}

// FLUpdate mirrors the manager FL update request.
type FLUpdate struct {
	RoundID      string         `json:"round_id"`
	PropletID    string         `json:"proplet_id"`
	BaseModelURI string         `json:"base_model_uri"`
	NumSamples   int            `json:"num_samples"`
	Metrics      map[string]any `json:"metrics"`
	Update       map[string]any `json:"update"`
	ReceivedAt   time.Time      `json:"received_at"`
}

// RoundStatus mirrors the manager round completion status response.
type RoundStatus struct {
	RoundID      string `json:"round_id"`
	Completed    bool   `json:"completed"`
	NumUpdates   int    `json:"num_updates"`
	KOfN         int    `json:"k_of_n"`
	ModelVersion int    `json:"model_version,omitempty"`
}

func (sdk *propSDK) ConfigureExperiment(config ExperimentConfig) (ExperimentResult, error) {
	data, err := json.Marshal(config)
	if err != nil {
		return ExperimentResult{}, err
	}

	reqURL := sdk.managerURL + flExperimentsEndpoint

	body, err := sdk.processRequest(http.MethodPost, reqURL, data, http.StatusOK)
	if err != nil {
		return ExperimentResult{}, err
	}

	var res ExperimentResult
	if err := json.Unmarshal(body, &res); err != nil {
		return ExperimentResult{}, err
	}

	return res, nil
}

func (sdk *propSDK) GetFLTask(roundID, propletID string) (FLTask, error) {
	reqURL := sdk.managerURL + flTaskEndpoint + "?round_id=" + roundID
	if propletID != "" {
		reqURL += "&proplet_id=" + propletID
	}

	body, err := sdk.processRequest(http.MethodGet, reqURL, nil, http.StatusOK)
	if err != nil {
		return FLTask{}, err
	}

	var resp struct {
		Task FLTask `json:"task"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return FLTask{}, err
	}

	return resp.Task, nil
}

func (sdk *propSDK) PostFLUpdate(update FLUpdate) error {
	data, err := json.Marshal(update)
	if err != nil {
		return err
	}

	reqURL := sdk.managerURL + flUpdateEndpoint

	if _, err := sdk.processRequest(http.MethodPost, reqURL, data, http.StatusOK); err != nil {
		return err
	}

	return nil
}

func (sdk *propSDK) PostFLUpdateCBOR(data []byte) error {
	reqURL := sdk.managerURL + flUpdateCBOREndpoint

	req, err := http.NewRequest(http.MethodPost, reqURL, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/cbor")

	resp, err := sdk.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected response code: %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}

func (sdk *propSDK) GetRoundStatus(roundID string) (RoundStatus, error) {
	reqURL := sdk.managerURL + flRoundsEndpoint + "/" + roundID + "/complete"

	body, err := sdk.processRequest(http.MethodGet, reqURL, nil, http.StatusOK)
	if err != nil {
		return RoundStatus{}, err
	}

	var resp struct {
		Status RoundStatus `json:"status"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return RoundStatus{}, err
	}

	return resp.Status, nil
}
