package sdk

import (
	"encoding/json"
	"net/http"
)

const healthEndpoint = "/health"

// HealthInfo mirrors the manager health check response.
type HealthInfo struct {
	Status      string `json:"status"`
	Version     string `json:"version"`
	Commit      string `json:"commit"`
	Description string `json:"description"`
	BuildTime   string `json:"build_time"`
	InstanceID  string `json:"instance_id"`
}

func (sdk *propSDK) GetHealth() (HealthInfo, error) {
	reqURL := sdk.managerURL + healthEndpoint

	body, err := sdk.processRequest(http.MethodGet, reqURL, nil, http.StatusOK)
	if err != nil {
		return HealthInfo{}, err
	}

	var info HealthInfo
	if err := json.Unmarshal(body, &info); err != nil {
		return HealthInfo{}, err
	}

	return info, nil
}
