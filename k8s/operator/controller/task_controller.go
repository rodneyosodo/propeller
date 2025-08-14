package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	propellerv1alpha1 "github.com/absmach/propeller/k8s/api/v1alpha1"
	"github.com/absmach/propeller/pkg/mqtt"
	"github.com/absmach/propeller/pkg/scheduler"
)

// TaskController reconciles Task resources
type TaskController struct {
	client.Client
	logger     *slog.Logger
	pubsub     mqtt.PubSub
	domainID   string
	channelID  string
	scheduler  scheduler.Scheduler
	Scheme     *runtime.Scheme
}

func NewTaskController(client client.Client, logger *slog.Logger, pubsub mqtt.PubSub, domainID, channelID string) *TaskController {
	// Use round-robin scheduler for hybrid scheduling
	roundRobinScheduler := scheduler.NewRoundRobinScheduler()
	
	return &TaskController{
		Client:    client,
		logger:    logger,
		pubsub:    pubsub,
		domainID:  domainID,
		channelID: channelID,
		scheduler: roundRobinScheduler,
	}
}

func (r *TaskController) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := r.logger.With("task", req.NamespacedName)
	
	var task propellerv1alpha1.Task
	if err := r.Get(ctx, req.NamespacedName, &task); err != nil {
		if errors.IsNotFound(err) {
			log.Info("Task resource not found, ignoring")
			return ctrl.Result{}, nil
		}
		log.Error("Failed to get Task", "error", err)
		return ctrl.Result{}, err
	}

	log.Info("Reconciling Task", "phase", task.Status.Phase)

	switch task.Status.Phase {
	case "": // New task
		return r.scheduleTask(ctx, &task)
	case propellerv1alpha1.TaskPhasePending:
		return r.executeTask(ctx, &task)
	case propellerv1alpha1.TaskPhaseRunning:
		return r.monitorTask(ctx, &task)
	case propellerv1alpha1.TaskPhaseCompleted, propellerv1alpha1.TaskPhaseFailed:
		// Task is finished, no action needed
		return ctrl.Result{}, nil
	default:
		log.Error("Unknown task phase", "phase", task.Status.Phase)
		return ctrl.Result{}, nil
	}
}

func (r *TaskController) scheduleTask(ctx context.Context, task *propellerv1alpha1.Task) (ctrl.Result, error) {
	log := r.logger.With("task", task.Name, "action", "schedule")
	
	// Find suitable proplets
	proplets, err := r.findSuitableProplets(ctx, task)
	if err != nil {
		log.Error("Failed to find suitable proplets", "error", err)
		return ctrl.Result{}, err
	}

	if len(proplets) == 0 {
		log.Info("No suitable proplets found, waiting")
		task.Status.Phase = propellerv1alpha1.TaskPhasePending
		task.Status.Conditions = append(task.Status.Conditions, propellerv1alpha1.TaskCondition{
			Type:               propellerv1alpha1.TaskConditionScheduled,
			Status:             metav1.ConditionFalse,
			LastTransitionTime: metav1.Now(),
			Reason:             "NoSuitableProplets",
			Message:            "No proplets available that match the task requirements",
		})
		
		if err := r.Status().Update(ctx, task); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: time.Second * 30}, nil
	}

	// Convert to legacy format for scheduler
	legacyProplets := make([]LegacyProplet, len(proplets))
	for i, p := range proplets {
		legacyProplets[i] = LegacyProplet{
			ID:        p.Name,
			Name:      p.Name,
			TaskCount: uint64(p.Status.TaskCount),
			Alive:     p.Status.Phase == propellerv1alpha1.PropletPhaseRunning,
		}
	}

	legacyTask := LegacyTask{
		ID:   string(task.UID),
		Name: task.Spec.Name,
	}

	selectedProplet, err := r.scheduler.SelectProplet(legacyTask, legacyProplets)
	if err != nil {
		log.Error("Failed to select proplet", "error", err)
		return ctrl.Result{}, err
	}

	// Update task status
	task.Status.Phase = propellerv1alpha1.TaskPhasePending
	task.Status.AssignedProplet = selectedProplet.ID
	task.Status.Conditions = append(task.Status.Conditions, propellerv1alpha1.TaskCondition{
		Type:               propellerv1alpha1.TaskConditionScheduled,
		Status:             metav1.ConditionTrue,
		LastTransitionTime: metav1.Now(),
		Reason:             "PropletSelected",
		Message:            fmt.Sprintf("Task scheduled to proplet %s", selectedProplet.ID),
	})

	if err := r.Status().Update(ctx, task); err != nil {
		return ctrl.Result{}, err
	}

	log.Info("Task scheduled successfully", "proplet", selectedProplet.ID)
	return ctrl.Result{RequeueAfter: time.Second * 5}, nil
}

func (r *TaskController) executeTask(ctx context.Context, task *propellerv1alpha1.Task) (ctrl.Result, error) {
	log := r.logger.With("task", task.Name, "action", "execute")
	
	if task.Status.AssignedProplet == "" {
		log.Error("Task has no assigned proplet")
		return r.scheduleTask(ctx, task)
	}

	// Send task start command via MQTT
	topic := fmt.Sprintf("m/%s/c/%s/messages/control/manager/start", r.domainID, r.channelID)
	payload := map[string]interface{}{
		"id":        string(task.UID),
		"name":      task.Spec.Name,
		"state":     "running",
		"image_url": task.Spec.ImageURL,
		"file":      task.Spec.File,
		"inputs":    task.Spec.Inputs,
		"cli_args":  task.Spec.CLIArgs,
	}

	if err := r.pubsub.Publish(ctx, topic, payload); err != nil {
		log.Error("Failed to publish task start command", "error", err)
		return ctrl.Result{}, err
	}

	// Update task status
	now := metav1.Now()
	task.Status.Phase = propellerv1alpha1.TaskPhaseRunning
	task.Status.StartTime = &now
	task.Status.Conditions = append(task.Status.Conditions, propellerv1alpha1.TaskCondition{
		Type:               propellerv1alpha1.TaskConditionStarted,
		Status:             metav1.ConditionTrue,
		LastTransitionTime: now,
		Reason:             "TaskStarted",
		Message:            "Task execution started on proplet",
	})

	if err := r.Status().Update(ctx, task); err != nil {
		return ctrl.Result{}, err
	}

	// Subscribe to task results
	go r.subscribeToTaskResults(ctx, task)

	log.Info("Task execution started", "proplet", task.Status.AssignedProplet)
	return ctrl.Result{RequeueAfter: time.Minute * 2}, nil
}

func (r *TaskController) monitorTask(ctx context.Context, task *propellerv1alpha1.Task) (ctrl.Result, error) {
	// Check for timeout
	if task.Status.StartTime != nil {
		elapsed := time.Since(task.Status.StartTime.Time)
		if elapsed > time.Hour { // 1 hour timeout
			task.Status.Phase = propellerv1alpha1.TaskPhaseFailed
			task.Status.Error = "Task execution timeout"
			
			now := metav1.Now()
			task.Status.FinishTime = &now
			
			if err := r.Status().Update(ctx, task); err != nil {
				return ctrl.Result{}, err
			}
			
			return ctrl.Result{}, nil
		}
	}

	// Task is still running, continue monitoring
	return ctrl.Result{RequeueAfter: time.Minute * 1}, nil
}

func (r *TaskController) findSuitableProplets(ctx context.Context, task *propellerv1alpha1.Task) ([]propellerv1alpha1.Proplet, error) {
	var propletList propellerv1alpha1.PropletList
	if err := r.List(ctx, &propletList, client.InNamespace(task.Namespace)); err != nil {
		return nil, err
	}

	var suitableProplets []propellerv1alpha1.Proplet
	for _, proplet := range propletList.Items {
		if r.isPropletSuitable(&proplet, task) {
			suitableProplets = append(suitableProplets, proplet)
		}
	}

	return suitableProplets, nil
}

func (r *TaskController) isPropletSuitable(proplet *propellerv1alpha1.Proplet, task *propellerv1alpha1.Task) bool {
	// Check if proplet is running
	if proplet.Status.Phase != propellerv1alpha1.PropletPhaseRunning {
		return false
	}

	// Check proplet type preference
	if task.Spec.PreferredPropletType != propellerv1alpha1.PropletPreferenceAny {
		if (task.Spec.PreferredPropletType == propellerv1alpha1.PropletPreferenceK8s && 
			 proplet.Spec.Type != propellerv1alpha1.PropletTypeK8s) ||
		   (task.Spec.PreferredPropletType == propellerv1alpha1.PropletPreferenceExternal && 
			 proplet.Spec.Type != propellerv1alpha1.PropletTypeExternal) {
			return false
		}
	}

	// Check selector requirements
	if task.Spec.PropletSelector != nil {
		// Check device types for external proplets
		if len(task.Spec.PropletSelector.MatchDeviceTypes) > 0 {
			if proplet.Spec.Type != propellerv1alpha1.PropletTypeExternal ||
			   proplet.Spec.External == nil {
				return false
			}
			
			matched := false
			for _, deviceType := range task.Spec.PropletSelector.MatchDeviceTypes {
				if deviceType == proplet.Spec.External.DeviceType {
					matched = true
					break
				}
			}
			if !matched {
				return false
			}
		}

		// Check capabilities
		if len(task.Spec.PropletSelector.MatchCapabilities) > 0 {
			if proplet.Spec.Type != propellerv1alpha1.PropletTypeExternal ||
			   proplet.Spec.External == nil {
				return false
			}
			
			for _, reqCapability := range task.Spec.PropletSelector.MatchCapabilities {
				found := false
				for _, capability := range proplet.Spec.External.Capabilities {
					if capability == reqCapability {
						found = true
						break
					}
				}
				if !found {
					return false
				}
			}
		}
		
		// Check labels
		if len(task.Spec.PropletSelector.MatchLabels) > 0 {
			for key, value := range task.Spec.PropletSelector.MatchLabels {
				if proplet.Labels[key] != value {
					return false
				}
			}
		}
	}

	return true
}

func (r *TaskController) subscribeToTaskResults(ctx context.Context, task *propellerv1alpha1.Task) {
	topic := fmt.Sprintf("m/%s/c/%s/messages/control/proplet/results", r.domainID, r.channelID)
	
	err := r.pubsub.Subscribe(ctx, topic, func(topic string, msg map[string]interface{}) error {
		taskID, ok := msg["task_id"].(string)
		if !ok || taskID != string(task.UID) {
			return nil // Not for this task
		}

		// Update task status with results
		task.Status.Results = msg["results"]
		task.Status.Phase = propellerv1alpha1.TaskPhaseCompleted
		
		now := metav1.Now()
		task.Status.FinishTime = &now
		
		if errMsg, ok := msg["error"].(string); ok && errMsg != "" {
			task.Status.Error = errMsg
			task.Status.Phase = propellerv1alpha1.TaskPhaseFailed
		}
		
		task.Status.Conditions = append(task.Status.Conditions, propellerv1alpha1.TaskCondition{
			Type:               propellerv1alpha1.TaskConditionCompleted,
			Status:             metav1.ConditionTrue,
			LastTransitionTime: now,
			Reason:             "TaskCompleted",
			Message:            "Task execution completed",
		})

		return r.Status().Update(ctx, task)
	})
	
	if err != nil {
		r.logger.Error("Failed to subscribe to task results", "error", err)
	}
}

func (r *TaskController) SetupWithManager(mgr ctrl.Manager) error {
	r.Scheme = mgr.GetScheme()
	
	return ctrl.NewControllerManagedBy(mgr).
		For(&propellerv1alpha1.Task{}).
		Complete(r)
}

// Legacy types for compatibility with existing scheduler
type LegacyProplet struct {
	ID        string
	Name      string
	TaskCount uint64
	Alive     bool
}

type LegacyTask struct {
	ID   string
	Name string
}