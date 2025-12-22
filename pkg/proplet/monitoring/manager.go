package monitoring

import (
	"context"
	"fmt"
	"sync"
	"time"

	"log/slog"
)

type MonitorManager struct {
	monitors map[string]*ProcessMonitor
	mu       sync.RWMutex
	logger   *slog.Logger
}

func NewMonitorManager(logger *slog.Logger) *MonitorManager {
	return &MonitorManager{
		monitors: make(map[string]*ProcessMonitor),
		logger:   logger,
	}
}

func (mm *MonitorManager) StartMonitoring(ctx context.Context, taskID string, pid int32, profile MonitoringProfile, exportFunc func(taskID string, metrics *ProcessMetrics, agg *AggregatedMetrics)) error {
	if !profile.Enabled {
		mm.logger.Debug("Monitoring disabled for task", slog.String("task_id", taskID))
		return nil
	}

	monitor, err := NewProcessMonitor(pid, profile)
	if err != nil {
		return fmt.Errorf("failed to create process monitor: %w", err)
	}

	mm.mu.Lock()
	mm.monitors[taskID] = monitor
	mm.mu.Unlock()

	mm.logger.Info("Started monitoring", slog.String("task_id", taskID), slog.Int("pid", int(pid)))

	go mm.monitorLoop(ctx, taskID, monitor, profile, exportFunc)

	return nil
}

func (mm *MonitorManager) StopMonitoring(taskID string) {
	mm.mu.Lock()
	defer mm.mu.Unlock()

	if _, exists := mm.monitors[taskID]; exists {
		delete(mm.monitors, taskID)
		mm.logger.Info("Stopped monitoring", slog.String("task_id", taskID))
	}
}

func (mm *MonitorManager) GetMetrics(taskID string) (*ProcessMetrics, *AggregatedMetrics, error) {
	mm.mu.RLock()
	monitor, exists := mm.monitors[taskID]
	mm.mu.RUnlock()

	if !exists {
		return nil, nil, fmt.Errorf("no monitor found for task %s", taskID)
	}

	latest := monitor.GetLatestMetrics()
	agg := monitor.GetAggregatedMetrics()

	return latest, agg, nil
}

func (mm *MonitorManager) monitorLoop(ctx context.Context, taskID string, monitor *ProcessMonitor, profile MonitoringProfile, exportFunc func(string, *ProcessMetrics, *AggregatedMetrics)) {
	ticker := time.NewTicker(profile.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			mm.logger.Debug("Monitor loop stopped by context", slog.String("task_id", taskID))
			return
		case <-ticker.C:
			mm.mu.RLock()
			_, exists := mm.monitors[taskID]
			mm.mu.RUnlock()

			if !exists {
				mm.logger.Debug("Monitor removed, stopping loop", slog.String("task_id", taskID))
				return
			}

			metrics, err := monitor.Collect(ctx)
			if err != nil {
				mm.logger.Warn("Failed to collect metrics", slog.String("task_id", taskID), slog.Any("error", err))
				mm.StopMonitoring(taskID)
				return
			}

			mm.logger.Debug("Collected metrics",
				slog.String("task_id", taskID),
				slog.Float64("cpu_percent", metrics.CPUPercent),
				slog.Uint64("memory_bytes", metrics.MemoryBytes),
			)

			if profile.ExportToMQTT && exportFunc != nil {
				agg := monitor.GetAggregatedMetrics()
				exportFunc(taskID, metrics, agg)
			}
		}
	}
}
