package propeller

var (
	Version   = "dev"
	Commit    = "none"
	BuildTime = "unknown"
)

type HealthInfo struct {
	Status      string `json:"status"`
	Version     string `json:"version"`
	Commit      string `json:"commit"`
	Description string `json:"description"`
	BuildTime   string `json:"build_time"`
	InstanceID  string `json:"instance_id"`
}
