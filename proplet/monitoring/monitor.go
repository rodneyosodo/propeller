package monitoring

import (
	"context"
	"runtime"
	"sync"
	"time"

	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/net"
	"github.com/shirou/gopsutil/v3/process"
)

type ProcessMetrics struct {
	CPUPercent          float64   `json:"cpu_percent"`
	MemoryBytes         uint64    `json:"memory_bytes"`
	MemoryPercent       float32   `json:"memory_percent"`
	DiskReadBytes       uint64    `json:"disk_read_bytes"`
	DiskWriteBytes      uint64    `json:"disk_write_bytes"`
	NetworkRxBytes      uint64    `json:"network_rx_bytes"`
	NetworkTxBytes      uint64    `json:"network_tx_bytes"`
	UptimeSeconds       int64     `json:"uptime_seconds"`
	ThreadCount         int32     `json:"thread_count"`
	FileDescriptorCount int32     `json:"file_descriptor_count,omitempty"`
	Timestamp           time.Time `json:"timestamp"`
}

type AggregatedMetrics struct {
	AvgCPUUsage    float64 `json:"avg_cpu_usage"`
	MaxCPUUsage    float64 `json:"max_cpu_usage"`
	AvgMemoryUsage uint64  `json:"avg_memory_usage"`
	MaxMemoryUsage uint64  `json:"max_memory_usage"`
	TotalDiskRead  uint64  `json:"total_disk_read"`
	TotalDiskWrite uint64  `json:"total_disk_write"`
	TotalNetworkRx uint64  `json:"total_network_rx"`
	TotalNetworkTx uint64  `json:"total_network_tx"`
	SampleCount    int     `json:"sample_count"`
}

type ProcessMonitor struct {
	pid            int32
	profile        MonitoringProfile
	proc           *process.Process
	metricsHistory []ProcessMetrics
	startTime      time.Time
	mu             sync.RWMutex
	initialNetIO   *net.IOCountersStat
	initialDiskIO  *disk.IOCountersStat
}

func NewProcessMonitor(pid int32, profile MonitoringProfile) (*ProcessMonitor, error) {
	proc, err := process.NewProcess(pid)
	if err != nil {
		return nil, err
	}

	monitor := &ProcessMonitor{
		pid:            pid,
		profile:        profile,
		proc:           proc,
		metricsHistory: make([]ProcessMetrics, 0, profile.HistorySize),
		startTime:      time.Now(),
	}

	if profile.CollectDiskIO {
		if ioCounters, err := disk.IOCounters(); err == nil {
			for _, io := range ioCounters {
				monitor.initialDiskIO = &io
				break
			}
		}
	}

	if profile.CollectNetworkIO {
		if ioCounters, err := net.IOCounters(false); err == nil && len(ioCounters) > 0 {
			monitor.initialNetIO = &ioCounters[0]
		}
	}

	return monitor, nil
}

func (m *ProcessMonitor) Collect(ctx context.Context) (*ProcessMetrics, error) {
	metrics := &ProcessMetrics{
		Timestamp: time.Now(),
	}

	if m.profile.CollectCPU {
		if cpuPercent, err := m.proc.CPUPercentWithContext(ctx); err == nil {
			metrics.CPUPercent = cpuPercent
		}
	}

	if m.profile.CollectMemory {
		if memInfo, err := m.proc.MemoryInfoWithContext(ctx); err == nil {
			metrics.MemoryBytes = memInfo.RSS
		}
		if memPercent, err := m.proc.MemoryPercentWithContext(ctx); err == nil {
			metrics.MemoryPercent = memPercent
		}
	}

	if m.profile.CollectDiskIO {
		if ioCounters, err := m.proc.IOCountersWithContext(ctx); err == nil {
			metrics.DiskReadBytes = ioCounters.ReadBytes
			metrics.DiskWriteBytes = ioCounters.WriteBytes
		}
	}

	if m.profile.CollectNetworkIO {
		if ioCounters, err := net.IOCountersWithContext(ctx, false); err == nil && len(ioCounters) > 0 {
			if m.initialNetIO != nil {
				metrics.NetworkRxBytes = ioCounters[0].BytesRecv - m.initialNetIO.BytesRecv
				metrics.NetworkTxBytes = ioCounters[0].BytesSent - m.initialNetIO.BytesSent
			}
		}
	}

	if m.profile.CollectThreads {
		if numThreads, err := m.proc.NumThreadsWithContext(ctx); err == nil {
			metrics.ThreadCount = numThreads
		}
	}

	if m.profile.CollectFileDescriptors {
		if runtime.GOOS == "linux" || runtime.GOOS == "darwin" {
			if numFDs, err := m.proc.NumFDsWithContext(ctx); err == nil {
				metrics.FileDescriptorCount = numFDs
			}
		}
	}

	metrics.UptimeSeconds = int64(time.Since(m.startTime).Seconds())

	m.mu.Lock()
	defer m.mu.Unlock()

	if m.profile.RetainHistory {
		m.metricsHistory = append(m.metricsHistory, *metrics)
		if len(m.metricsHistory) > m.profile.HistorySize {
			m.metricsHistory = m.metricsHistory[1:]
		}
	}

	return metrics, nil
}

func (m *ProcessMonitor) GetAggregatedMetrics() *AggregatedMetrics {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if len(m.metricsHistory) == 0 {
		return nil
	}

	agg := &AggregatedMetrics{
		SampleCount: len(m.metricsHistory),
	}

	var totalCPU float64
	var totalMemory uint64

	first := m.metricsHistory[0]
	last := m.metricsHistory[len(m.metricsHistory)-1]

	for _, metrics := range m.metricsHistory {
		totalCPU += metrics.CPUPercent
		totalMemory += metrics.MemoryBytes

		if metrics.CPUPercent > agg.MaxCPUUsage {
			agg.MaxCPUUsage = metrics.CPUPercent
		}
		if metrics.MemoryBytes > agg.MaxMemoryUsage {
			agg.MaxMemoryUsage = metrics.MemoryBytes
		}
	}

	agg.AvgCPUUsage = totalCPU / float64(len(m.metricsHistory))
	agg.AvgMemoryUsage = totalMemory / uint64(len(m.metricsHistory))

	if last.DiskReadBytes >= first.DiskReadBytes {
		agg.TotalDiskRead = last.DiskReadBytes - first.DiskReadBytes
	}
	if last.DiskWriteBytes >= first.DiskWriteBytes {
		agg.TotalDiskWrite = last.DiskWriteBytes - first.DiskWriteBytes
	}
	if last.NetworkRxBytes >= first.NetworkRxBytes {
		agg.TotalNetworkRx = last.NetworkRxBytes - first.NetworkRxBytes
	}
	if last.NetworkTxBytes >= first.NetworkTxBytes {
		agg.TotalNetworkTx = last.NetworkTxBytes - first.NetworkTxBytes
	}

	return agg
}

func (m *ProcessMonitor) GetLatestMetrics() *ProcessMetrics {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if len(m.metricsHistory) == 0 {
		return nil
	}

	latest := m.metricsHistory[len(m.metricsHistory)-1]
	return &latest
}

func (m *ProcessMonitor) GetHistory() []ProcessMetrics {
	m.mu.RLock()
	defer m.mu.RUnlock()

	history := make([]ProcessMetrics, len(m.metricsHistory))
	copy(history, m.metricsHistory)
	return history
}
