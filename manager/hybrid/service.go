package hybrid

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	propellerv1alpha1 "github.com/absmach/propeller/k8s/api/v1alpha1"
	"github.com/absmach/propeller/manager"
	"github.com/absmach/propeller/pkg/mqtt"
	"github.com/absmach/propeller/proplet"
	"github.com/absmach/propeller/task"
)

// HybridOrchestrator manages both K8s-managed and external proplets
type HybridOrchestrator struct {
	k8sClient     client.Client
	legacyManager manager.Service
	pubsub        mqtt.PubSub
	domainID      string
	channelID     string
	logger        *slog.Logger
}

func NewHybridOrchestrator(
	k8sClient client.Client,
	legacyManager manager.Service,
	pubsub mqtt.PubSub,
	domainID, channelID string,
	logger *slog.Logger,
) *HybridOrchestrator {
	return &HybridOrchestrator{
		k8sClient:     k8sClient,
		legacyManager: legacyManager,
		pubsub:        pubsub,
		domainID:      domainID,
		channelID:     channelID,
		logger:        logger,
	}
}

// Start initializes the hybrid orchestrator
func (h *HybridOrchestrator) Start(ctx context.Context) error {
	h.logger.Info("Starting hybrid orchestrator")
	
	// Subscribe to MQTT topics for external proplets
	if err := h.subscribeToMQTTTopics(ctx); err != nil {
		return fmt.Errorf("failed to subscribe to MQTT topics: %w", err)
	}
	
	// Start legacy manager for backward compatibility
	if err := h.legacyManager.Subscribe(ctx); err != nil {
		return fmt.Errorf("failed to start legacy manager: %w", err)
	}
	
	h.logger.Info("Hybrid orchestrator started successfully")
	return nil
}

// CreateTask creates a task that can be executed on either K8s or external proplets
func (h *HybridOrchestrator) CreateTask(ctx context.Context, taskSpec TaskSpec) error {
	// Create both legacy task (for external proplets) and K8s Task resource
	
	// Create legacy task
	legacyTask := task.Task{
		Name:     taskSpec.Name,
		ImageURL: taskSpec.ImageURL,
		File:     taskSpec.File,
		CLIArgs:  taskSpec.CLIArgs,
		Inputs:   taskSpec.Inputs,
	}
	
	createdTask, err := h.legacyManager.CreateTask(ctx, legacyTask)
	if err != nil {
		return fmt.Errorf("failed to create legacy task: %w", err)
	}
	
	// Create K8s Task resource
	k8sTask := &propellerv1alpha1.Task{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("task-%s", createdTask.ID),
			Namespace: taskSpec.Namespace,
			Labels: map[string]string{
				"propeller.absmach.fr/legacy-task-id": createdTask.ID,
			},
		},
		Spec: propellerv1alpha1.TaskSpec{
			Name:                    taskSpec.Name,
			ImageURL:               taskSpec.ImageURL,
			File:                   taskSpec.File,
			CLIArgs:                taskSpec.CLIArgs,
			Inputs:                 taskSpec.Inputs,
			PreferredPropletType:   convertPropletPreference(taskSpec.PreferredPropletType),
			PropletSelector:        convertPropletSelector(taskSpec.PropletSelector),
			ResourceRequirements:   convertResourceRequirements(taskSpec.ResourceRequirements),
		},
	}
	
	if err := h.k8sClient.Create(ctx, k8sTask); err != nil {
		h.logger.Error("Failed to create K8s task", "error", err)
		// Continue with legacy task execution for backward compatibility
	}
	
	h.logger.Info("Task created successfully", "taskId", createdTask.ID, "name", taskSpec.Name)
	return nil
}

// GetProplets returns both K8s and external proplets
func (h *HybridOrchestrator) GetProplets(ctx context.Context) ([]PropletInfo, error) {
	var allProplets []PropletInfo
	
	// Get K8s proplets
	var k8sProplets propellerv1alpha1.PropletList
	if err := h.k8sClient.List(ctx, &k8sProplets); err == nil {
		for _, proplet := range k8sProplets.Items {
			allProplets = append(allProplets, PropletInfo{
				ID:           proplet.Name,
				Name:         proplet.Name,
				Type:         string(proplet.Spec.Type),
				Phase:        string(proplet.Status.Phase),
				TaskCount:    proplet.Status.TaskCount,
				LastSeen:     proplet.Status.LastSeen,
				DeviceType:   getDeviceType(&proplet),
				Capabilities: getCapabilities(&proplet),
			})
		}
	}
	
	// Get legacy/external proplets
	legacyProplets, err := h.legacyManager.ListProplets(ctx, 0, 100)
	if err == nil {
		for _, proplet := range legacyProplets.Proplets {
			// Skip if already included in K8s proplets
			found := false
			for _, k8sProplet := range allProplets {
				if k8sProplet.ID == proplet.ID {
					found = true
					break
				}
			}
			if !found {
				var lastSeen *metav1.Time
				if len(proplet.AliveHistory) > 0 {
					lastAlive := proplet.AliveHistory[len(proplet.AliveHistory)-1]
					lastSeen = &metav1.Time{Time: lastAlive}
				}
				
				phase := "Offline"
				if proplet.Alive {
					phase = "Running"
				}
				
				allProplets = append(allProplets, PropletInfo{
					ID:        proplet.ID,
					Name:      proplet.Name,
					Type:      "external",
					Phase:     phase,
					TaskCount: int32(proplet.TaskCount),
					LastSeen:  lastSeen,
				})
			}
		}
	}
	
	return allProplets, nil
}

func (h *HybridOrchestrator) subscribeToMQTTTopics(ctx context.Context) error {
	// Subscribe to external proplet discovery
	discoveryTopic := fmt.Sprintf("m/%s/c/%s/messages/control/proplet/create", h.domainID, h.channelID)
	if err := h.pubsub.Subscribe(ctx, discoveryTopic, h.handleExternalPropletDiscovery(ctx)); err != nil {
		return err
	}
	
	// Subscribe to external proplet alive messages
	aliveTopic := fmt.Sprintf("m/%s/c/%s/messages/control/proplet/alive", h.domainID, h.channelID)
	if err := h.pubsub.Subscribe(ctx, aliveTopic, h.handleExternalPropletAlive(ctx)); err != nil {
		return err
	}
	
	// Subscribe to task results
	resultsTopic := fmt.Sprintf("m/%s/c/%s/messages/control/proplet/results", h.domainID, h.channelID)
	if err := h.pubsub.Subscribe(ctx, resultsTopic, h.handleTaskResults(ctx)); err != nil {
		return err
	}
	
	return nil
}

func (h *HybridOrchestrator) handleExternalPropletDiscovery(ctx context.Context) func(string, map[string]interface{}) error {
	return func(topic string, msg map[string]interface{}) error {
		propletID, ok := msg["proplet_id"].(string)
		if !ok || propletID == "" {
			return fmt.Errorf("invalid proplet_id in discovery message")
		}
		
		h.logger.Info("Discovered external proplet", "propletId", propletID)
		
		// Create K8s Proplet resource for external proplet
		externalProplet := &propellerv1alpha1.Proplet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      propletID,
				Namespace: "default", // TODO: Make configurable
				Labels: map[string]string{
					"propeller.absmach.fr/type": "external",
				},
			},
			Spec: propellerv1alpha1.PropletSpec{
				Type: propellerv1alpha1.PropletTypeExternal,
				External: &propellerv1alpha1.ExternalPropletSpec{
					DeviceType:   inferDeviceType(msg),
					Capabilities: inferCapabilities(msg),
				},
				ConnectionConfig: propellerv1alpha1.ConnectionConfig{
					DomainID:  h.domainID,
					ChannelID: h.channelID,
				},
			},
		}
		
		// Try to create K8s resource, but don't fail if it exists
		if err := h.k8sClient.Create(ctx, externalProplet); err != nil {
			h.logger.Debug("Failed to create K8s proplet resource", "error", err)
		}
		
		// Also handle with legacy manager
		return h.legacyManager.Subscribe(ctx)
	}
}

func (h *HybridOrchestrator) handleExternalPropletAlive(ctx context.Context) func(string, map[string]interface{}) error {
	return func(topic string, msg map[string]interface{}) error {
		propletID, ok := msg["proplet_id"].(string)
		if !ok || propletID == "" {
			return nil
		}
		
		// Update K8s Proplet resource
		var proplet propellerv1alpha1.Proplet
		if err := h.k8sClient.Get(ctx, client.ObjectKey{Name: propletID, Namespace: "default"}, &proplet); err == nil {
			now := metav1.Now()
			proplet.Status.LastSeen = &now
			proplet.Status.Phase = propellerv1alpha1.PropletPhaseRunning
			
			if err := h.k8sClient.Status().Update(ctx, &proplet); err != nil {
				h.logger.Error("Failed to update proplet status", "proplet", propletID, "error", err)
			}
		}
		
		return nil
	}
}

func (h *HybridOrchestrator) handleTaskResults(ctx context.Context) func(string, map[string]interface{}) error {
	return func(topic string, msg map[string]interface{}) error {
		taskID, ok := msg["task_id"].(string)
		if !ok || taskID == "" {
			return nil
		}
		
		// Find corresponding K8s Task resource
		var taskList propellerv1alpha1.TaskList
		if err := h.k8sClient.List(ctx, &taskList, client.MatchingLabels{
			"propeller.absmach.fr/legacy-task-id": taskID,
		}); err == nil && len(taskList.Items) > 0 {
			task := &taskList.Items[0]
			task.Status.Results = msg["results"]
			task.Status.Phase = propellerv1alpha1.TaskPhaseCompleted
			
			now := metav1.Now()
			task.Status.FinishTime = &now
			
			if errMsg, ok := msg["error"].(string); ok && errMsg != "" {
				task.Status.Error = errMsg
				task.Status.Phase = propellerv1alpha1.TaskPhaseFailed
			}
			
			if err := h.k8sClient.Status().Update(ctx, task); err != nil {
				h.logger.Error("Failed to update task status", "task", taskID, "error", err)
			}
		}
		
		return nil
	}
}

// Helper functions for type conversion
func convertPropletPreference(pref string) propellerv1alpha1.PropletPreference {
	switch pref {
	case "k8s":
		return propellerv1alpha1.PropletPreferenceK8s
	case "external":
		return propellerv1alpha1.PropletPreferenceExternal
	default:
		return propellerv1alpha1.PropletPreferenceAny
	}
}

func convertPropletSelector(selector *PropletSelectorSpec) *propellerv1alpha1.PropletSelector {
	if selector == nil {
		return nil
	}
	return &propellerv1alpha1.PropletSelector{
		MatchLabels:       selector.MatchLabels,
		MatchDeviceTypes:  selector.MatchDeviceTypes,
		MatchCapabilities: selector.MatchCapabilities,
	}
}

func convertResourceRequirements(reqs *ResourceRequirements) *propellerv1alpha1.PropletResources {
	if reqs == nil {
		return nil
	}
	return &propellerv1alpha1.PropletResources{
		CPU:    reqs.CPU,
		Memory: reqs.Memory,
		Custom: reqs.Custom,
	}
}

func getDeviceType(proplet *propellerv1alpha1.Proplet) string {
	if proplet.Spec.External != nil {
		return proplet.Spec.External.DeviceType
	}
	return ""
}

func getCapabilities(proplet *propellerv1alpha1.Proplet) []string {
	if proplet.Spec.External != nil {
		return proplet.Spec.External.Capabilities
	}
	return nil
}

func inferDeviceType(msg map[string]interface{}) string {
	// Try to infer device type from message
	if deviceType, ok := msg["device_type"].(string); ok {
		return deviceType
	}
	return "unknown"
}

func inferCapabilities(msg map[string]interface{}) []string {
	// Try to infer capabilities from message
	if caps, ok := msg["capabilities"].([]interface{}); ok {
		capabilities := make([]string, len(caps))
		for i, cap := range caps {
			if capStr, ok := cap.(string); ok {
				capabilities[i] = capStr
			}
		}
		return capabilities
	}
	return []string{"wasm"}
}

// Types for the hybrid orchestrator API
type TaskSpec struct {
	Name                    string                   `json:"name"`
	ImageURL               string                   `json:"imageUrl"`
	File                   []byte                   `json:"file,omitempty"`
	CLIArgs                []string                 `json:"cliArgs,omitempty"`
	Inputs                 map[string]interface{}   `json:"inputs,omitempty"`
	Namespace              string                   `json:"namespace,omitempty"`
	PreferredPropletType   string                   `json:"preferredPropletType,omitempty"`
	PropletSelector        *PropletSelectorSpec     `json:"propletSelector,omitempty"`
	ResourceRequirements   *ResourceRequirements    `json:"resourceRequirements,omitempty"`
}

type PropletSelectorSpec struct {
	MatchLabels       map[string]string `json:"matchLabels,omitempty"`
	MatchDeviceTypes  []string          `json:"matchDeviceTypes,omitempty"`
	MatchCapabilities []string          `json:"matchCapabilities,omitempty"`
}

type ResourceRequirements struct {
	CPU    string            `json:"cpu,omitempty"`
	Memory string            `json:"memory,omitempty"`
	Custom map[string]string `json:"custom,omitempty"`
}

type PropletInfo struct {
	ID           string        `json:"id"`
	Name         string        `json:"name"`
	Type         string        `json:"type"`
	Phase        string        `json:"phase"`
	TaskCount    int32         `json:"taskCount"`
	LastSeen     *metav1.Time  `json:"lastSeen,omitempty"`
	DeviceType   string        `json:"deviceType,omitempty"`
	Capabilities []string      `json:"capabilities,omitempty"`
}