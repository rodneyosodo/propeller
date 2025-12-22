package monitoring

import "time"

type MonitoringProfile struct {
	Enabled                bool          `json:"enabled"`
	Interval               time.Duration `json:"interval"`
	CollectCPU             bool          `json:"collect_cpu"`
	CollectMemory          bool          `json:"collect_memory"`
	CollectDiskIO          bool          `json:"collect_disk_io"`
	CollectNetworkIO       bool          `json:"collect_network_io"`
	CollectThreads         bool          `json:"collect_threads"`
	CollectFileDescriptors bool          `json:"collect_file_descriptors"`
	ExportToMQTT           bool          `json:"export_to_mqtt"`
	RetainHistory          bool          `json:"retain_history"`
	HistorySize            int           `json:"history_size"`
}

func StandardProfile() MonitoringProfile {
	return MonitoringProfile{
		Enabled:                true,
		Interval:               10 * time.Second,
		CollectCPU:             true,
		CollectMemory:          true,
		CollectDiskIO:          true,
		CollectNetworkIO:       true,
		CollectThreads:         true,
		CollectFileDescriptors: true,
		ExportToMQTT:           true,
		RetainHistory:          true,
		HistorySize:            100,
	}
}

func MinimalProfile() MonitoringProfile {
	return MonitoringProfile{
		Enabled:                true,
		Interval:               60 * time.Second,
		CollectCPU:             true,
		CollectMemory:          true,
		CollectDiskIO:          false,
		CollectNetworkIO:       false,
		CollectThreads:         false,
		CollectFileDescriptors: false,
		ExportToMQTT:           false,
		RetainHistory:          false,
		HistorySize:            0,
	}
}

func IntensiveProfile() MonitoringProfile {
	return MonitoringProfile{
		Enabled:                true,
		Interval:               1 * time.Second,
		CollectCPU:             true,
		CollectMemory:          true,
		CollectDiskIO:          true,
		CollectNetworkIO:       true,
		CollectThreads:         true,
		CollectFileDescriptors: true,
		ExportToMQTT:           true,
		RetainHistory:          true,
		HistorySize:            1000,
	}
}

func BatchProcessingProfile() MonitoringProfile {
	return MonitoringProfile{
		Enabled:                true,
		Interval:               30 * time.Second,
		CollectCPU:             true,
		CollectMemory:          true,
		CollectDiskIO:          true,
		CollectNetworkIO:       false,
		CollectThreads:         false,
		CollectFileDescriptors: false,
		ExportToMQTT:           true,
		RetainHistory:          true,
		HistorySize:            200,
	}
}

func RealTimeAPIProfile() MonitoringProfile {
	return MonitoringProfile{
		Enabled:                true,
		Interval:               5 * time.Second,
		CollectCPU:             true,
		CollectMemory:          true,
		CollectDiskIO:          false,
		CollectNetworkIO:       true,
		CollectThreads:         true,
		CollectFileDescriptors: true,
		ExportToMQTT:           true,
		RetainHistory:          true,
		HistorySize:            500,
	}
}

func LongRunningDaemonProfile() MonitoringProfile {
	return MonitoringProfile{
		Enabled:                true,
		Interval:               120 * time.Second,
		CollectCPU:             true,
		CollectMemory:          true,
		CollectDiskIO:          true,
		CollectNetworkIO:       true,
		CollectThreads:         true,
		CollectFileDescriptors: true,
		ExportToMQTT:           true,
		RetainHistory:          true,
		HistorySize:            500,
	}
}

func DisabledProfile() MonitoringProfile {
	return MonitoringProfile{
		Enabled:                false,
		Interval:               60 * time.Second,
		CollectCPU:             false,
		CollectMemory:          false,
		CollectDiskIO:          false,
		CollectNetworkIO:       false,
		CollectThreads:         false,
		CollectFileDescriptors: false,
		ExportToMQTT:           false,
		RetainHistory:          false,
		HistorySize:            0,
	}
}
