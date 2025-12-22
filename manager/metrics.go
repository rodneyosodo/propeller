package manager

import (
	"time"

	"github.com/absmach/propeller/pkg/proplet"
	"github.com/absmach/propeller/pkg/proplet/monitoring"
)

type TaskMetrics struct {
	TaskID     string                        `json:"task_id"`
	PropletID  string                        `json:"proplet_id"`
	Metrics    monitoring.ProcessMetrics     `json:"metrics"`
	Aggregated *monitoring.AggregatedMetrics `json:"aggregated,omitempty"`
	Timestamp  time.Time                     `json:"timestamp"`
}

type PropletMetrics struct {
	PropletID string                `json:"proplet_id"`
	Version   string                `json:"version"`
	Timestamp time.Time             `json:"timestamp"`
	CPU       proplet.CPUMetrics    `json:"cpu"`
	Memory    proplet.MemoryMetrics `json:"memory"`
}

type TaskMetricsPage struct {
	Offset  uint64        `json:"offset"`
	Limit   uint64        `json:"limit"`
	Total   uint64        `json:"total"`
	Metrics []TaskMetrics `json:"metrics"`
}

type PropletMetricsPage struct {
	Offset  uint64           `json:"offset"`
	Limit   uint64           `json:"limit"`
	Total   uint64           `json:"total"`
	Metrics []PropletMetrics `json:"metrics"`
}
