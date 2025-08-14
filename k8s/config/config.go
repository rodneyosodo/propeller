package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config holds configuration for the hybrid orchestration system
type Config struct {
	// Kubernetes configuration
	Kubernetes KubernetesConfig `json:"kubernetes"`
	
	// MQTT configuration for external proplets
	MQTT MQTTConfig `json:"mqtt"`
	
	// Propeller domain and channel configuration
	Propeller PropellerConfig `json:"propeller"`
	
	// Health monitoring configuration
	HealthMonitor HealthMonitorConfig `json:"healthMonitor"`
	
	// Scheduling configuration
	Scheduler SchedulerConfig `json:"scheduler"`
	
	// Logging configuration
	Logging LoggingConfig `json:"logging"`
}

// KubernetesConfig holds Kubernetes-specific configuration
type KubernetesConfig struct {
	// Namespace for proplet resources
	Namespace string `json:"namespace"`
	
	// Enable leader election for operator HA
	LeaderElection bool `json:"leaderElection"`
	
	// Metrics server configuration
	MetricsAddr string `json:"metricsAddr"`
	
	// Health probe configuration
	HealthProbeAddr string `json:"healthProbeAddr"`
	
	// Webhook server configuration
	WebhookAddr string `json:"webhookAddr"`
	
	// Default proplet image
	DefaultPropletImage string `json:"defaultPropletImage"`
	
	// Default resource limits for K8s proplets
	DefaultResources ResourceLimits `json:"defaultResources"`
}

// MQTTConfig holds MQTT broker configuration
type MQTTConfig struct {
	// Broker address
	Broker string `json:"broker"`
	
	// Username for MQTT authentication
	Username string `json:"username"`
	
	// Password for MQTT authentication
	Password string `json:"password"`
	
	// TLS configuration
	TLS TLSConfig `json:"tls"`
	
	// Client ID prefix for MQTT connections
	ClientIDPrefix string `json:"clientIdPrefix"`
	
	// Keep alive interval
	KeepAlive time.Duration `json:"keepAlive"`
	
	// Connection timeout
	ConnectTimeout time.Duration `json:"connectTimeout"`
	
	// Retry configuration
	Retry RetryConfig `json:"retry"`
}

// PropellerConfig holds Propeller-specific configuration
type PropellerConfig struct {
	// Domain ID for MQTT topics
	DomainID string `json:"domainId"`
	
	// Channel ID for MQTT topics
	ChannelID string `json:"channelId"`
	
	// Registry configuration for OCI images
	Registry RegistryConfig `json:"registry"`
}

// HealthMonitorConfig holds health monitoring configuration
type HealthMonitorConfig struct {
	// Interval between health checks
	CheckInterval time.Duration `json:"checkInterval"`
	
	// Threshold for considering a proplet offline
	OfflineThreshold time.Duration `json:"offlineThreshold"`
	
	// Threshold for considering a proplet failed
	FailureThreshold time.Duration `json:"failureThreshold"`
	
	// Enable automatic task evacuation from failed proplets
	AutoEvacuateTasks bool `json:"autoEvacuateTasks"`
	
	// Enable alerts for proplet failures
	EnableAlerts bool `json:"enableAlerts"`
	
	// Alert webhook URL
	AlertWebhook string `json:"alertWebhook"`
}

// SchedulerConfig holds scheduling configuration
type SchedulerConfig struct {
	// Scheduler type: roundrobin, resource-aware, hybrid
	Type string `json:"type"`
	
	// Resource-aware scheduler configuration
	ResourceAware ResourceAwareConfig `json:"resourceAware"`
	
	// Hybrid scheduler configuration
	Hybrid HybridSchedulerConfig `json:"hybrid"`
}

// LoggingConfig holds logging configuration
type LoggingConfig struct {
	// Log level: debug, info, warn, error
	Level string `json:"level"`
	
	// Log format: json, text
	Format string `json:"format"`
	
	// Output destination: stdout, stderr, file
	Output string `json:"output"`
	
	// Log file path (when output=file)
	FilePath string `json:"filePath"`
}

// ResourceLimits defines resource constraints
type ResourceLimits struct {
	CPU    string `json:"cpu"`
	Memory string `json:"memory"`
	
	// Custom resource limits
	Custom map[string]string `json:"custom"`
}

// TLSConfig holds TLS configuration for MQTT
type TLSConfig struct {
	// Enable TLS
	Enabled bool `json:"enabled"`
	
	// CA certificate file path
	CAFile string `json:"caFile"`
	
	// Client certificate file path
	CertFile string `json:"certFile"`
	
	// Client private key file path
	KeyFile string `json:"keyFile"`
	
	// Skip certificate verification (insecure)
	SkipVerify bool `json:"skipVerify"`
}

// RetryConfig holds retry configuration
type RetryConfig struct {
	// Maximum number of retries
	MaxRetries int `json:"maxRetries"`
	
	// Initial retry delay
	InitialDelay time.Duration `json:"initialDelay"`
	
	// Maximum retry delay
	MaxDelay time.Duration `json:"maxDelay"`
	
	// Backoff multiplier
	BackoffMultiplier float64 `json:"backoffMultiplier"`
}

// RegistryConfig holds OCI registry configuration
type RegistryConfig struct {
	// Default registry URL
	URL string `json:"url"`
	
	// Username for registry authentication
	Username string `json:"username"`
	
	// Password for registry authentication
	Password string `json:"password"`
	
	// Enable insecure registry access
	Insecure bool `json:"insecure"`
}

// ResourceAwareConfig holds resource-aware scheduler configuration
type ResourceAwareConfig struct {
	// Enable CPU-based scheduling
	EnableCPU bool `json:"enableCpu"`
	
	// Enable memory-based scheduling
	EnableMemory bool `json:"enableMemory"`
	
	// CPU utilization threshold (percentage)
	CPUThreshold float64 `json:"cpuThreshold"`
	
	// Memory utilization threshold (percentage)
	MemoryThreshold float64 `json:"memoryThreshold"`
}

// HybridSchedulerConfig holds hybrid scheduler configuration
type HybridSchedulerConfig struct {
	// Prefer K8s proplets for compute-intensive tasks
	PreferK8sForCompute bool `json:"preferK8sForCompute"`
	
	// Prefer external proplets for edge tasks
	PreferExternalForEdge bool `json:"preferExternalForEdge"`
	
	// Keywords that indicate compute-intensive tasks
	ComputeKeywords []string `json:"computeKeywords"`
	
	// Keywords that indicate edge tasks
	EdgeKeywords []string `json:"edgeKeywords"`
	
	// Load balancing ratio between K8s and external proplets (0.0-1.0)
	// 0.0 = prefer external, 1.0 = prefer K8s, 0.5 = balanced
	LoadBalancingRatio float64 `json:"loadBalancingRatio"`
}

// LoadFromEnv loads configuration from environment variables
func LoadFromEnv() *Config {
	config := &Config{
		Kubernetes: KubernetesConfig{
			Namespace:           getEnv("K8S_NAMESPACE", "propeller-system"),
			LeaderElection:      getBoolEnv("ENABLE_LEADER_ELECTION", false),
			MetricsAddr:         getEnv("METRICS_ADDR", ":8080"),
			HealthProbeAddr:     getEnv("HEALTH_PROBE_ADDR", ":8081"),
			WebhookAddr:         getEnv("WEBHOOK_ADDR", ":9443"),
			DefaultPropletImage: getEnv("DEFAULT_PROPLET_IMAGE", "propeller/proplet:latest"),
			DefaultResources: ResourceLimits{
				CPU:    getEnv("DEFAULT_CPU_LIMIT", "500m"),
				Memory: getEnv("DEFAULT_MEMORY_LIMIT", "512Mi"),
			},
		},
		MQTT: MQTTConfig{
			Broker:         getEnv("MQTT_BROKER", "localhost:1883"),
			Username:       getEnv("MQTT_USERNAME", ""),
			Password:       getEnv("MQTT_PASSWORD", ""),
			ClientIDPrefix: getEnv("MQTT_CLIENT_ID_PREFIX", "propeller-operator"),
			KeepAlive:      getDurationEnv("MQTT_KEEP_ALIVE", time.Minute*5),
			ConnectTimeout: getDurationEnv("MQTT_CONNECT_TIMEOUT", time.Second*30),
			TLS: TLSConfig{
				Enabled:    getBoolEnv("MQTT_TLS_ENABLED", false),
				CAFile:     getEnv("MQTT_TLS_CA_FILE", ""),
				CertFile:   getEnv("MQTT_TLS_CERT_FILE", ""),
				KeyFile:    getEnv("MQTT_TLS_KEY_FILE", ""),
				SkipVerify: getBoolEnv("MQTT_TLS_SKIP_VERIFY", false),
			},
			Retry: RetryConfig{
				MaxRetries:        getIntEnv("MQTT_MAX_RETRIES", 5),
				InitialDelay:      getDurationEnv("MQTT_INITIAL_DELAY", time.Second*1),
				MaxDelay:          getDurationEnv("MQTT_MAX_DELAY", time.Minute*1),
				BackoffMultiplier: getFloatEnv("MQTT_BACKOFF_MULTIPLIER", 2.0),
			},
		},
		Propeller: PropellerConfig{
			DomainID:  getEnv("PROPELLER_DOMAIN_ID", "propeller"),
			ChannelID: getEnv("PROPELLER_CHANNEL_ID", "control"),
			Registry: RegistryConfig{
				URL:      getEnv("REGISTRY_URL", ""),
				Username: getEnv("REGISTRY_USERNAME", ""),
				Password: getEnv("REGISTRY_PASSWORD", ""),
				Insecure: getBoolEnv("REGISTRY_INSECURE", false),
			},
		},
		HealthMonitor: HealthMonitorConfig{
			CheckInterval:     getDurationEnv("HEALTH_CHECK_INTERVAL", time.Minute*1),
			OfflineThreshold:  getDurationEnv("OFFLINE_THRESHOLD", time.Minute*5),
			FailureThreshold:  getDurationEnv("FAILURE_THRESHOLD", time.Minute*15),
			AutoEvacuateTasks: getBoolEnv("AUTO_EVACUATE_TASKS", true),
			EnableAlerts:      getBoolEnv("ENABLE_ALERTS", true),
			AlertWebhook:      getEnv("ALERT_WEBHOOK", ""),
		},
		Scheduler: SchedulerConfig{
			Type: getEnv("SCHEDULER_TYPE", "hybrid"),
			ResourceAware: ResourceAwareConfig{
				EnableCPU:       getBoolEnv("SCHEDULER_ENABLE_CPU", true),
				EnableMemory:    getBoolEnv("SCHEDULER_ENABLE_MEMORY", true),
				CPUThreshold:    getFloatEnv("SCHEDULER_CPU_THRESHOLD", 80.0),
				MemoryThreshold: getFloatEnv("SCHEDULER_MEMORY_THRESHOLD", 80.0),
			},
			Hybrid: HybridSchedulerConfig{
				PreferK8sForCompute:   getBoolEnv("SCHEDULER_PREFER_K8S_COMPUTE", true),
				PreferExternalForEdge: getBoolEnv("SCHEDULER_PREFER_EXTERNAL_EDGE", true),
				ComputeKeywords:       getStringSliceEnv("SCHEDULER_COMPUTE_KEYWORDS", []string{"ml", "ai", "compute", "process", "analyze", "transform", "batch"}),
				EdgeKeywords:          getStringSliceEnv("SCHEDULER_EDGE_KEYWORDS", []string{"sensor", "iot", "edge", "device", "monitor", "collect"}),
				LoadBalancingRatio:    getFloatEnv("SCHEDULER_LOAD_BALANCING_RATIO", 0.5),
			},
		},
		Logging: LoggingConfig{
			Level:    getEnv("LOG_LEVEL", "info"),
			Format:   getEnv("LOG_FORMAT", "json"),
			Output:   getEnv("LOG_OUTPUT", "stdout"),
			FilePath: getEnv("LOG_FILE_PATH", ""),
		},
	}
	
	return config
}

// Validate validates the configuration
func (c *Config) Validate() error {
	// Validate Kubernetes configuration
	if c.Kubernetes.Namespace == "" {
		return fmt.Errorf("kubernetes namespace cannot be empty")
	}
	
	// Validate MQTT configuration
	if c.MQTT.Broker == "" {
		return fmt.Errorf("MQTT broker cannot be empty")
	}
	
	// Validate Propeller configuration
	if c.Propeller.DomainID == "" {
		return fmt.Errorf("propeller domain ID cannot be empty")
	}
	if c.Propeller.ChannelID == "" {
		return fmt.Errorf("propeller channel ID cannot be empty")
	}
	
	// Validate scheduler configuration
	validSchedulerTypes := map[string]bool{
		"roundrobin":     true,
		"resource-aware": true,
		"hybrid":         true,
	}
	if !validSchedulerTypes[c.Scheduler.Type] {
		return fmt.Errorf("invalid scheduler type: %s", c.Scheduler.Type)
	}
	
	// Validate logging configuration
	validLogLevels := map[string]bool{
		"debug": true,
		"info":  true,
		"warn":  true,
		"error": true,
	}
	if !validLogLevels[c.Logging.Level] {
		return fmt.Errorf("invalid log level: %s", c.Logging.Level)
	}
	
	return nil
}

// Helper functions for reading environment variables
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getBoolEnv(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		if parsed, err := strconv.ParseBool(value); err == nil {
			return parsed
		}
	}
	return defaultValue
}

func getIntEnv(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if parsed, err := strconv.Atoi(value); err == nil {
			return parsed
		}
	}
	return defaultValue
}

func getFloatEnv(key string, defaultValue float64) float64 {
	if value := os.Getenv(key); value != "" {
		if parsed, err := strconv.ParseFloat(value, 64); err == nil {
			return parsed
		}
	}
	return defaultValue
}

func getDurationEnv(key string, defaultValue time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if parsed, err := time.ParseDuration(value); err == nil {
			return parsed
		}
	}
	return defaultValue
}

func getStringSliceEnv(key string, defaultValue []string) []string {
	if value := os.Getenv(key); value != "" {
		// Simple comma-separated parsing
		// In production, you might want more sophisticated parsing
		return []string{value} // Simplified for this example
	}
	return defaultValue
}