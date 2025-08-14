package controller

import (
	"context"
	"log/slog"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	propellerv1alpha1 "github.com/absmach/propeller/k8s/api/v1alpha1"
)

// HealthMonitor monitors the health of proplets and handles failures
type HealthMonitor struct {
	client               client.Client
	logger               *slog.Logger
	healthCheckInterval  time.Duration
	offlineThreshold     time.Duration
	failureThreshold     time.Duration
}

// NewHealthMonitor creates a new health monitor
func NewHealthMonitor(client client.Client, logger *slog.Logger) *HealthMonitor {
	return &HealthMonitor{
		client:               client,
		logger:               logger,
		healthCheckInterval:  time.Minute * 1,    // Check health every minute
		offlineThreshold:     time.Minute * 5,    // Consider offline after 5 minutes
		failureThreshold:     time.Minute * 15,   // Consider failed after 15 minutes
	}
}

// Start begins the health monitoring process
func (h *HealthMonitor) Start(ctx context.Context) error {
	h.logger.Info("Starting health monitor")
	
	ticker := time.NewTicker(h.healthCheckInterval)
	defer ticker.Stop()
	
	for {
		select {
		case <-ctx.Done():
			h.logger.Info("Health monitor stopping")
			return nil
		case <-ticker.C:
			if err := h.checkAllPropletsHealth(ctx); err != nil {
				h.logger.Error("Failed to check proplets health", "error", err)
			}
		}
	}
}

// checkAllPropletsHealth checks the health of all proplets
func (h *HealthMonitor) checkAllPropletsHealth(ctx context.Context) error {
	var proplets propellerv1alpha1.PropletList
	if err := h.client.List(ctx, &proplets); err != nil {
		return err
	}
	
	for i := range proplets.Items {
		proplet := &proplets.Items[i]
		if err := h.checkPropletHealth(ctx, proplet); err != nil {
			h.logger.Error("Failed to check proplet health", 
				"proplet", proplet.Name, 
				"error", err)
		}
	}
	
	return nil
}

// checkPropletHealth evaluates the health of a single proplet
func (h *HealthMonitor) checkPropletHealth(ctx context.Context, proplet *propellerv1alpha1.Proplet) error {
	log := h.logger.With("proplet", proplet.Name, "type", proplet.Spec.Type)
	
	now := time.Now()
	updated := false
	
	// Check last seen timestamp
	if proplet.Status.LastSeen != nil {
		timeSinceLastSeen := now.Sub(proplet.Status.LastSeen.Time)
		
		// Determine new phase based on time since last seen
		newPhase := h.determinePhase(timeSinceLastSeen, proplet.Status.Phase)
		
		if newPhase != proplet.Status.Phase {
			log.Info("Proplet phase changing", 
				"from", proplet.Status.Phase, 
				"to", newPhase,
				"timeSinceLastSeen", timeSinceLastSeen)
			
			proplet.Status.Phase = newPhase
			updated = true
			
			// Update conditions based on new phase
			h.updateHealthConditions(proplet, newPhase, timeSinceLastSeen)
		}
	} else if proplet.Spec.Type == propellerv1alpha1.PropletTypeExternal {
		// External proplets without LastSeen should be considered initializing
		if proplet.Status.Phase != propellerv1alpha1.PropletPhaseInitializing {
			proplet.Status.Phase = propellerv1alpha1.PropletPhaseInitializing
			updated = true
			
			h.updateHealthConditions(proplet, propellerv1alpha1.PropletPhaseInitializing, 0)
		}
	}
	
	// Handle failure scenarios
	if proplet.Status.Phase == propellerv1alpha1.PropletPhaseFailed {
		if err := h.handlePropletFailure(ctx, proplet); err != nil {
			log.Error("Failed to handle proplet failure", "error", err)
		}
		updated = true
	}
	
	// Update the proplet status if needed
	if updated {
		if err := h.client.Status().Update(ctx, proplet); err != nil {
			return err
		}
		
		log.Debug("Updated proplet health status", "phase", proplet.Status.Phase)
	}
	
	return nil
}

// determinePhase determines the appropriate phase based on last seen time
func (h *HealthMonitor) determinePhase(timeSinceLastSeen time.Duration, currentPhase propellerv1alpha1.PropletPhase) propellerv1alpha1.PropletPhase {
	if timeSinceLastSeen > h.failureThreshold {
		return propellerv1alpha1.PropletPhaseFailed
	}
	
	if timeSinceLastSeen > h.offlineThreshold {
		return propellerv1alpha1.PropletPhaseOffline
	}
	
	// If we've seen the proplet recently and it was previously offline/failed, mark as running
	if currentPhase == propellerv1alpha1.PropletPhaseOffline || 
	   currentPhase == propellerv1alpha1.PropletPhaseFailed ||
	   currentPhase == propellerv1alpha1.PropletPhaseInitializing {
		return propellerv1alpha1.PropletPhaseRunning
	}
	
	return currentPhase
}

// updateHealthConditions updates the proplet's health conditions
func (h *HealthMonitor) updateHealthConditions(proplet *propellerv1alpha1.Proplet, phase propellerv1alpha1.PropletPhase, timeSinceLastSeen time.Duration) {
	now := metav1.Now()
	
	// Update Ready condition
	readyCondition := propellerv1alpha1.PropletCondition{
		Type:               propellerv1alpha1.PropletConditionReady,
		LastTransitionTime: now,
	}
	
	switch phase {
	case propellerv1alpha1.PropletPhaseRunning:
		readyCondition.Status = metav1.ConditionTrue
		readyCondition.Reason = "PropletRunning"
		readyCondition.Message = "Proplet is running and ready to accept tasks"
		
	case propellerv1alpha1.PropletPhaseOffline:
		readyCondition.Status = metav1.ConditionFalse
		readyCondition.Reason = "PropletOffline"
		readyCondition.Message = "Proplet has not been seen recently"
		
	case propellerv1alpha1.PropletPhaseFailed:
		readyCondition.Status = metav1.ConditionFalse
		readyCondition.Reason = "PropletFailed"
		readyCondition.Message = "Proplet has been offline for an extended period"
		
	case propellerv1alpha1.PropletPhaseInitializing:
		readyCondition.Status = metav1.ConditionFalse
		readyCondition.Reason = "PropletInitializing"
		readyCondition.Message = "Proplet is starting up"
	}
	
	// Update Connected condition
	connectedCondition := propellerv1alpha1.PropletCondition{
		Type:               propellerv1alpha1.PropletConditionConnected,
		LastTransitionTime: now,
	}
	
	if phase == propellerv1alpha1.PropletPhaseRunning {
		connectedCondition.Status = metav1.ConditionTrue
		connectedCondition.Reason = "PropletConnected"
		connectedCondition.Message = "Proplet is actively communicating"
	} else {
		connectedCondition.Status = metav1.ConditionFalse
		connectedCondition.Reason = "PropletDisconnected"
		connectedCondition.Message = "Proplet is not responding"
	}
	
	// Update Healthy condition
	healthyCondition := propellerv1alpha1.PropletCondition{
		Type:               propellerv1alpha1.PropletConditionHealthy,
		LastTransitionTime: now,
	}
	
	switch phase {
	case propellerv1alpha1.PropletPhaseRunning:
		healthyCondition.Status = metav1.ConditionTrue
		healthyCondition.Reason = "PropletHealthy"
		healthyCondition.Message = "Proplet is operating normally"
		
	case propellerv1alpha1.PropletPhaseOffline:
		healthyCondition.Status = metav1.ConditionFalse
		healthyCondition.Reason = "PropletUnresponsive"
		healthyCondition.Message = "Proplet has not responded to health checks"
		
	case propellerv1alpha1.PropletPhaseFailed:
		healthyCondition.Status = metav1.ConditionFalse
		healthyCondition.Reason = "PropletUnhealthy"
		healthyCondition.Message = "Proplet has failed health checks consistently"
		
	case propellerv1alpha1.PropletPhaseInitializing:
		healthyCondition.Status = metav1.ConditionUnknown
		healthyCondition.Reason = "PropletStarting"
		healthyCondition.Message = "Proplet health status is unknown during initialization"
	}
	
	// Replace or add conditions
	proplet.Status.Conditions = h.updateConditions(proplet.Status.Conditions, []propellerv1alpha1.PropletCondition{
		readyCondition,
		connectedCondition,
		healthyCondition,
	})
}

// updateConditions updates the conditions list with new conditions
func (h *HealthMonitor) updateConditions(existing []propellerv1alpha1.PropletCondition, newConditions []propellerv1alpha1.PropletCondition) []propellerv1alpha1.PropletCondition {
	conditionMap := make(map[propellerv1alpha1.PropletConditionType]propellerv1alpha1.PropletCondition)
	
	// Start with existing conditions
	for _, condition := range existing {
		conditionMap[condition.Type] = condition
	}
	
	// Override with new conditions
	for _, condition := range newConditions {
		conditionMap[condition.Type] = condition
	}
	
	// Convert back to slice
	result := make([]propellerv1alpha1.PropletCondition, 0, len(conditionMap))
	for _, condition := range conditionMap {
		result = append(result, condition)
	}
	
	return result
}

// handlePropletFailure handles failed proplets
func (h *HealthMonitor) handlePropletFailure(ctx context.Context, proplet *propellerv1alpha1.Proplet) error {
	log := h.logger.With("proplet", proplet.Name, "type", proplet.Spec.Type)
	
	switch proplet.Spec.Type {
	case propellerv1alpha1.PropletTypeK8s:
		return h.handleK8sPropletFailure(ctx, proplet)
	case propellerv1alpha1.PropletTypeExternal:
		return h.handleExternalPropletFailure(ctx, proplet)
	default:
		log.Error("Unknown proplet type for failure handling", "type", proplet.Spec.Type)
		return nil
	}
}

// handleK8sPropletFailure handles failures of K8s-managed proplets
func (h *HealthMonitor) handleK8sPropletFailure(ctx context.Context, proplet *propellerv1alpha1.Proplet) error {
	log := h.logger.With("proplet", proplet.Name, "action", "k8s-failure-handling")
	
	log.Info("Handling K8s proplet failure - Kubernetes should handle pod restarts automatically")
	
	// For K8s proplets, we typically rely on Kubernetes' built-in mechanisms:
	// - Deployment controller will restart failed pods
	// - ReplicaSets ensure desired number of replicas
	// - Health checks (liveness/readiness probes) trigger restarts
	
	// We could add additional logic here such as:
	// - Scaling up replicas temporarily
	// - Triggering alerts
	// - Moving tasks to other proplets
	
	// For now, just log the failure
	log.Info("K8s proplet failure logged - relying on Kubernetes self-healing")
	
	return nil
}

// handleExternalPropletFailure handles failures of external proplets
func (h *HealthMonitor) handleExternalPropletFailure(ctx context.Context, proplet *propellerv1alpha1.Proplet) error {
	log := h.logger.With("proplet", proplet.Name, "action", "external-failure-handling")
	
	log.Info("Handling external proplet failure")
	
	// For external proplets, we need to be more active since there's no K8s self-healing:
	
	// 1. Move any running tasks to other proplets
	if err := h.evacuateTasksFromProplet(ctx, proplet); err != nil {
		log.Error("Failed to evacuate tasks from failed proplet", "error", err)
	}
	
	// 2. Update proplet's task count to 0 since it's failed
	proplet.Status.TaskCount = 0
	
	// 3. Could trigger alerts/notifications here
	h.triggerFailureAlert(proplet)
	
	log.Info("External proplet failure handling completed")
	
	return nil
}

// evacuateTasksFromProplet moves tasks away from a failed proplet
func (h *HealthMonitor) evacuateTasksFromProplet(ctx context.Context, proplet *propellerv1alpha1.Proplet) error {
	log := h.logger.With("proplet", proplet.Name, "action", "evacuate-tasks")
	
	// Find tasks assigned to this proplet
	var taskList propellerv1alpha1.TaskList
	if err := h.client.List(ctx, &taskList, client.MatchingFields{
		"status.assignedProplet": proplet.Name,
	}); err != nil {
		return err
	}
	
	for i := range taskList.Items {
		task := &taskList.Items[i]
		
		// Only handle running tasks
		if task.Status.Phase == propellerv1alpha1.TaskPhaseRunning {
			log.Info("Rescheduling task due to proplet failure", "task", task.Name)
			
			// Reset task to pending so it gets rescheduled
			task.Status.Phase = propellerv1alpha1.TaskPhasePending
			task.Status.AssignedProplet = ""
			task.Status.Error = "Proplet failed during execution"
			
			// Add condition about rescheduling
			now := metav1.Now()
			task.Status.Conditions = append(task.Status.Conditions, propellerv1alpha1.TaskCondition{
				Type:               propellerv1alpha1.TaskConditionScheduled,
				Status:             metav1.ConditionFalse,
				LastTransitionTime: now,
				Reason:             "PropletFailed",
				Message:            "Task rescheduled due to proplet failure",
			})
			
			if err := h.client.Status().Update(ctx, task); err != nil {
				log.Error("Failed to reschedule task", "task", task.Name, "error", err)
			}
		}
	}
	
	return nil
}

// triggerFailureAlert sends alerts about proplet failures
func (h *HealthMonitor) triggerFailureAlert(proplet *propellerv1alpha1.Proplet) {
	// This is where you would integrate with your alerting system
	// For example: Prometheus alerts, Slack notifications, PagerDuty, etc.
	
	h.logger.Warn("ALERT: External proplet has failed",
		"proplet", proplet.Name,
		"type", proplet.Spec.Type,
		"deviceType", func() string {
			if proplet.Spec.External != nil {
				return proplet.Spec.External.DeviceType
			}
			return "unknown"
		}(),
	)
	
	// TODO: Implement actual alerting mechanisms:
	// - Send metrics to Prometheus
	// - Send notifications to Slack/Teams
	// - Create incidents in PagerDuty
	// - Write to external monitoring systems
}