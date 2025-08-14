package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PropletSpec defines the desired state of Proplet
type PropletSpec struct {
	// Type specifies whether this is a K8s-managed or external proplet
	// +kubebuilder:validation:Enum=k8s;external
	Type PropletType `json:"type"`
	
	// External proplet configuration (only used when Type=external)
	External *ExternalPropletSpec `json:"external,omitempty"`
	
	// K8s proplet configuration (only used when Type=k8s) 
	K8s *K8sPropletSpec `json:"k8s,omitempty"`
	
	// Resources defines resource requirements
	Resources *PropletResources `json:"resources,omitempty"`
	
	// ConnectionConfig for MQTT communication
	ConnectionConfig ConnectionConfig `json:"connectionConfig"`
}

type PropletType string

const (
	PropletTypeK8s      PropletType = "k8s"
	PropletTypeExternal PropletType = "external"
)

type ExternalPropletSpec struct {
	// Endpoint is the address of the external proplet
	Endpoint string `json:"endpoint,omitempty"`
	
	// DeviceType specifies the type of external device (e.g., esp32, raspberry-pi)
	DeviceType string `json:"deviceType,omitempty"`
	
	// Capabilities describes what this external proplet can do
	Capabilities []string `json:"capabilities,omitempty"`
}

type K8sPropletSpec struct {
	// Image is the container image for the proplet
	Image string `json:"image"`
	
	// Replicas is the number of proplet instances to run
	Replicas *int32 `json:"replicas,omitempty"`
	
	// Resources for the K8s pods
	Resources *metav1.LabelSelector `json:"resources,omitempty"`
}

type PropletResources struct {
	// CPU capacity (e.g., "1000m" for 1 CPU core)
	CPU string `json:"cpu,omitempty"`
	
	// Memory capacity (e.g., "1Gi")
	Memory string `json:"memory,omitempty"`
	
	// Custom resource constraints
	Custom map[string]string `json:"custom,omitempty"`
}

type ConnectionConfig struct {
	// DomainID for MQTT communication
	DomainID string `json:"domainId"`
	
	// ChannelID for MQTT communication
	ChannelID string `json:"channelId"`
	
	// MQTT broker configuration
	Broker string `json:"broker,omitempty"`
}

// PropletStatus defines the observed state of Proplet
type PropletStatus struct {
	// Phase indicates the overall status of the proplet
	Phase PropletPhase `json:"phase"`
	
	// Conditions represent the latest available observations
	Conditions []PropletCondition `json:"conditions,omitempty"`
	
	// LastSeen is when we last received a heartbeat from this proplet
	LastSeen *metav1.Time `json:"lastSeen,omitempty"`
	
	// TaskCount is the number of tasks currently running on this proplet
	TaskCount int32 `json:"taskCount"`
	
	// AvailableResources shows remaining capacity
	AvailableResources *PropletResources `json:"availableResources,omitempty"`
	
	// K8s-specific status (only for K8s proplets)
	K8sStatus *K8sStatus `json:"k8sStatus,omitempty"`
}

type PropletPhase string

const (
	PropletPhaseInitializing PropletPhase = "Initializing"
	PropletPhaseRunning      PropletPhase = "Running"
	PropletPhaseOffline      PropletPhase = "Offline"
	PropletPhaseFailed       PropletPhase = "Failed"
)

type PropletCondition struct {
	// Type of proplet condition
	Type PropletConditionType `json:"type"`
	
	// Status of the condition (True, False, Unknown)
	Status metav1.ConditionStatus `json:"status"`
	
	// LastTransitionTime is when the condition last changed
	LastTransitionTime metav1.Time `json:"lastTransitionTime"`
	
	// Reason is a brief reason for the condition
	Reason string `json:"reason,omitempty"`
	
	// Message is a human readable description
	Message string `json:"message,omitempty"`
}

type PropletConditionType string

const (
	PropletConditionReady       PropletConditionType = "Ready"
	PropletConditionConnected   PropletConditionType = "Connected"
	PropletConditionHealthy     PropletConditionType = "Healthy"
)

type K8sStatus struct {
	// ReadyReplicas is the number of ready pod replicas
	ReadyReplicas int32 `json:"readyReplicas"`
	
	// AvailableReplicas is the number of available pod replicas
	AvailableReplicas int32 `json:"availableReplicas"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced
// +kubebuilder:printcolumn:name="Type",type=string,JSONPath=`.spec.type`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Tasks",type=integer,JSONPath=`.status.taskCount`
// +kubebuilder:printcolumn:name="Last Seen",type=date,JSONPath=`.status.lastSeen`

// Proplet represents a worker node that can execute WebAssembly workloads
type Proplet struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PropletSpec   `json:"spec,omitempty"`
	Status PropletStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// PropletList contains a list of Proplets
type PropletList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Proplet `json:"items"`
}

// TaskSpec defines the desired state of Task
type TaskSpec struct {
	// Name of the task
	Name string `json:"name"`
	
	// ImageURL is the OCI registry URL for the WebAssembly workload
	ImageURL string `json:"imageUrl"`
	
	// File contains the WebAssembly binary (alternative to ImageURL)
	File []byte `json:"file,omitempty"`
	
	// CLIArgs are command line arguments to pass to the WebAssembly module
	CLIArgs []string `json:"cliArgs,omitempty"`
	
	// Inputs are parameters to pass to the WebAssembly module
	Inputs map[string]interface{} `json:"inputs,omitempty"`
	
	// PropletSelector determines which proplets can run this task
	PropletSelector *PropletSelector `json:"propletSelector,omitempty"`
	
	// PreferredPropletType indicates preference for K8s or external proplets
	// +kubebuilder:validation:Enum=k8s;external;any
	PreferredPropletType PropletPreference `json:"preferredPropletType,omitempty"`
	
	// Resources required by this task
	ResourceRequirements *PropletResources `json:"resourceRequirements,omitempty"`
}

type PropletSelector struct {
	// MatchLabels selects proplets by labels
	MatchLabels map[string]string `json:"matchLabels,omitempty"`
	
	// MatchDeviceTypes selects external proplets by device type
	MatchDeviceTypes []string `json:"matchDeviceTypes,omitempty"`
	
	// MatchCapabilities selects proplets with specific capabilities
	MatchCapabilities []string `json:"matchCapabilities,omitempty"`
}

type PropletPreference string

const (
	PropletPreferenceK8s      PropletPreference = "k8s"
	PropletPreferenceExternal PropletPreference = "external" 
	PropletPreferenceAny      PropletPreference = "any"
)

// TaskStatus defines the observed state of Task
type TaskStatus struct {
	// Phase indicates the current state of the task
	Phase TaskPhase `json:"phase"`
	
	// AssignedProplet is the proplet running this task
	AssignedProplet string `json:"assignedProplet,omitempty"`
	
	// StartTime when the task started execution
	StartTime *metav1.Time `json:"startTime,omitempty"`
	
	// FinishTime when the task completed
	FinishTime *metav1.Time `json:"finishTime,omitempty"`
	
	// Results from task execution
	Results interface{} `json:"results,omitempty"`
	
	// Error message if task failed
	Error string `json:"error,omitempty"`
	
	// Conditions represent the latest available observations
	Conditions []TaskCondition `json:"conditions,omitempty"`
}

type TaskPhase string

const (
	TaskPhasePending   TaskPhase = "Pending"
	TaskPhaseRunning   TaskPhase = "Running" 
	TaskPhaseCompleted TaskPhase = "Completed"
	TaskPhaseFailed    TaskPhase = "Failed"
)

type TaskCondition struct {
	// Type of task condition
	Type TaskConditionType `json:"type"`
	
	// Status of the condition
	Status metav1.ConditionStatus `json:"status"`
	
	// LastTransitionTime when the condition last changed
	LastTransitionTime metav1.Time `json:"lastTransitionTime"`
	
	// Reason for the condition
	Reason string `json:"reason,omitempty"`
	
	// Message is human readable description
	Message string `json:"message,omitempty"`
}

type TaskConditionType string

const (
	TaskConditionScheduled TaskConditionType = "Scheduled"
	TaskConditionStarted   TaskConditionType = "Started"
	TaskConditionCompleted TaskConditionType = "Completed"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Proplet",type=string,JSONPath=`.status.assignedProplet`
// +kubebuilder:printcolumn:name="Start Time",type=date,JSONPath=`.status.startTime`
// +kubebuilder:printcolumn:name="Duration",type=string,JSONPath=`.status.finishTime`

// Task represents a WebAssembly workload to be executed
type Task struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   TaskSpec   `json:"spec,omitempty"`
	Status TaskStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// TaskList contains a list of Tasks
type TaskList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Task `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Proplet{}, &PropletList{}, &Task{}, &TaskList{})
}