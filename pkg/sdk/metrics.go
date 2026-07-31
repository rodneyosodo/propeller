package sdk

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/absmach/propeller/pkg/proplet"
)

// TaskMetrics mirrors the manager task metrics response.
type TaskMetrics struct {
	TaskID     string                     `json:"task_id"`
	PropletID  string                     `json:"proplet_id"`
	Metrics    proplet.ProcessMetrics     `json:"metrics"`
	Aggregated *proplet.AggregatedMetrics `json:"aggregated,omitempty"`
	Timestamp  time.Time                  `json:"timestamp"`
}

// TaskMetricsPage mirrors the manager task metrics list response.
type TaskMetricsPage struct {
	Offset  uint64        `json:"offset"`
	Limit   uint64        `json:"limit"`
	Total   uint64        `json:"total"`
	Metrics []TaskMetrics `json:"metrics"`
}

// PropletMetrics mirrors the manager proplet metrics response.
type PropletMetrics struct {
	PropletID string                `json:"proplet_id"`
	Namespace string                `json:"namespace"`
	Timestamp time.Time             `json:"timestamp"`
	CPU       proplet.CPUMetrics    `json:"cpu_metrics"`
	Memory    proplet.MemoryMetrics `json:"memory_metrics"`
}

// PropletMetricsPage mirrors the manager proplet metrics list response.
type PropletMetricsPage struct {
	Offset  uint64           `json:"offset"`
	Limit   uint64           `json:"limit"`
	Total   uint64           `json:"total"`
	Metrics []PropletMetrics `json:"metrics"`
}

func (sdk *propSDK) GetTaskMetrics(id string, offset, limit uint64) (TaskMetricsPage, error) {
	reqURL := fmt.Sprintf("%s%s/%s/metrics?offset=%d&limit=%d", sdk.managerURL, tasksEndpoint, id, offset, limit)

	body, err := sdk.processRequest(http.MethodGet, reqURL, nil, http.StatusOK)
	if err != nil {
		return TaskMetricsPage{}, err
	}

	var page TaskMetricsPage
	if err := json.Unmarshal(body, &page); err != nil {
		return TaskMetricsPage{}, err
	}

	return page, nil
}

func (sdk *propSDK) GetPropletMetrics(id string, offset, limit uint64) (PropletMetricsPage, error) {
	reqURL := fmt.Sprintf("%s%s/%s/metrics?offset=%d&limit=%d", sdk.managerURL, propletsEndpoint, id, offset, limit)

	body, err := sdk.processRequest(http.MethodGet, reqURL, nil, http.StatusOK)
	if err != nil {
		return PropletMetricsPage{}, err
	}

	var page PropletMetricsPage
	if err := json.Unmarshal(body, &page); err != nil {
		return PropletMetricsPage{}, err
	}

	return page, nil
}
