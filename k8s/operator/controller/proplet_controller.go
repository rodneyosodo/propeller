package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	propellerv1alpha1 "github.com/absmach/propeller/k8s/api/v1alpha1"
	"github.com/absmach/propeller/pkg/mqtt"
)

// PropletController reconciles Proplet resources
type PropletController struct {
	client.Client
	logger     *slog.Logger
	pubsub     mqtt.PubSub
	domainID   string
	channelID  string
	Scheme     *runtime.Scheme
}

func NewPropletController(client client.Client, logger *slog.Logger, pubsub mqtt.PubSub, domainID, channelID string) *PropletController {
	return &PropletController{
		Client:    client,
		logger:    logger,
		pubsub:    pubsub,
		domainID:  domainID,
		channelID: channelID,
	}
}

func (r *PropletController) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := r.logger.With("proplet", req.NamespacedName)
	
	var proplet propellerv1alpha1.Proplet
	if err := r.Get(ctx, req.NamespacedName, &proplet); err != nil {
		if errors.IsNotFound(err) {
			log.Info("Proplet resource not found, ignoring")
			return ctrl.Result{}, nil
		}
		log.Error("Failed to get Proplet", "error", err)
		return ctrl.Result{}, err
	}

	log.Info("Reconciling Proplet", "type", proplet.Spec.Type)

	switch proplet.Spec.Type {
	case propellerv1alpha1.PropletTypeK8s:
		return r.reconcileK8sProplet(ctx, &proplet)
	case propellerv1alpha1.PropletTypeExternal:
		return r.reconcileExternalProplet(ctx, &proplet)
	default:
		log.Error("Unknown proplet type", "type", proplet.Spec.Type)
		return ctrl.Result{}, fmt.Errorf("unknown proplet type: %s", proplet.Spec.Type)
	}
}

func (r *PropletController) reconcileK8sProplet(ctx context.Context, proplet *propellerv1alpha1.Proplet) (ctrl.Result, error) {
	log := r.logger.With("proplet", proplet.Name, "type", "k8s")
	
	if proplet.Spec.K8s == nil {
		return ctrl.Result{}, fmt.Errorf("k8s spec is required for k8s proplet type")
	}

	// Create or update Deployment
	deployment := &appsv1.Deployment{}
	deploymentName := types.NamespacedName{
		Name:      fmt.Sprintf("%s-proplet", proplet.Name),
		Namespace: proplet.Namespace,
	}

	err := r.Get(ctx, deploymentName, deployment)
	if err != nil && errors.IsNotFound(err) {
		// Create new deployment
		deployment = r.buildPropletDeployment(proplet)
		if err := controllerutil.SetControllerReference(proplet, deployment, r.Scheme); err != nil {
			return ctrl.Result{}, err
		}
		
		log.Info("Creating proplet deployment")
		if err := r.Create(ctx, deployment); err != nil {
			log.Error("Failed to create deployment", "error", err)
			return ctrl.Result{}, err
		}
	} else if err != nil {
		log.Error("Failed to get deployment", "error", err)
		return ctrl.Result{}, err
	}

	// Update status based on deployment status
	if err := r.updateK8sPropletStatus(ctx, proplet, deployment); err != nil {
		log.Error("Failed to update proplet status", "error", err)
		return ctrl.Result{}, err
	}

	return ctrl.Result{RequeueAfter: time.Minute * 2}, nil
}

func (r *PropletController) reconcileExternalProplet(ctx context.Context, proplet *propellerv1alpha1.Proplet) (ctrl.Result, error) {
	log := r.logger.With("proplet", proplet.Name, "type", "external")
	
	if proplet.Spec.External == nil {
		return ctrl.Result{}, fmt.Errorf("external spec is required for external proplet type")
	}

	// Subscribe to MQTT topics for external proplet discovery and heartbeat
	if err := r.setupExternalPropletMQTT(ctx, proplet); err != nil {
		log.Error("Failed to setup MQTT for external proplet", "error", err)
		return ctrl.Result{}, err
	}

	// Check if we've received heartbeat recently
	if proplet.Status.LastSeen != nil {
		timeSinceLastSeen := time.Since(proplet.Status.LastSeen.Time)
		if timeSinceLastSeen > time.Minute*5 {
			proplet.Status.Phase = propellerv1alpha1.PropletPhaseOffline
		} else {
			proplet.Status.Phase = propellerv1alpha1.PropletPhaseRunning
		}
	} else {
		proplet.Status.Phase = propellerv1alpha1.PropletPhaseInitializing
	}

	// Update status
	if err := r.Status().Update(ctx, proplet); err != nil {
		log.Error("Failed to update external proplet status", "error", err)
		return ctrl.Result{}, err
	}

	return ctrl.Result{RequeueAfter: time.Minute * 1}, nil
}

func (r *PropletController) buildPropletDeployment(proplet *propellerv1alpha1.Proplet) *appsv1.Deployment {
	replicas := int32(1)
	if proplet.Spec.K8s.Replicas != nil {
		replicas = *proplet.Spec.K8s.Replicas
	}

	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-proplet", proplet.Name),
			Namespace: proplet.Namespace,
			Labels:    map[string]string{
				"app.kubernetes.io/name":      "proplet",
				"app.kubernetes.io/instance":  proplet.Name,
				"app.kubernetes.io/component": "worker",
				"propeller.absmach.fr/proplet": proplet.Name,
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"propeller.absmach.fr/proplet": proplet.Name,
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"propeller.absmach.fr/proplet": proplet.Name,
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "proplet",
							Image: proplet.Spec.K8s.Image,
							Env: []corev1.EnvVar{
								{
									Name:  "PROPELLER_DOMAIN_ID",
									Value: proplet.Spec.ConnectionConfig.DomainID,
								},
								{
									Name:  "PROPELLER_CHANNEL_ID",
									Value: proplet.Spec.ConnectionConfig.ChannelID,
								},
								{
									Name:  "MQTT_BROKER",
									Value: proplet.Spec.ConnectionConfig.Broker,
								},
								{
									Name: "PROPLET_ID",
									ValueFrom: &corev1.EnvVarSource{
										FieldRef: &corev1.ObjectFieldSelector{
											FieldPath: "metadata.name",
										},
									},
								},
							},
							Ports: []corev1.ContainerPort{
								{
									ContainerPort: 8080,
									Name:          "http",
								},
							},
							ReadinessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path: "/health",
										Port: intstr.FromString("http"),
									},
								},
								InitialDelaySeconds: 10,
								PeriodSeconds:       5,
							},
							LivenessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path: "/health",
										Port: intstr.FromString("http"),
									},
								},
								InitialDelaySeconds: 15,
								PeriodSeconds:       10,
							},
						},
					},
				},
			},
		},
	}
}

func (r *PropletController) updateK8sPropletStatus(ctx context.Context, proplet *propellerv1alpha1.Proplet, deployment *appsv1.Deployment) error {
	// Update status based on deployment
	proplet.Status.K8sStatus = &propellerv1alpha1.K8sStatus{
		ReadyReplicas:     deployment.Status.ReadyReplicas,
		AvailableReplicas: deployment.Status.AvailableReplicas,
	}

	if deployment.Status.ReadyReplicas > 0 {
		proplet.Status.Phase = propellerv1alpha1.PropletPhaseRunning
	} else {
		proplet.Status.Phase = propellerv1alpha1.PropletPhaseInitializing
	}

	// Update conditions
	now := metav1.Now()
	proplet.Status.Conditions = []propellerv1alpha1.PropletCondition{
		{
			Type:               propellerv1alpha1.PropletConditionReady,
			Status:             metav1.ConditionTrue,
			LastTransitionTime: now,
			Reason:             "DeploymentReady",
			Message:            fmt.Sprintf("Deployment has %d ready replicas", deployment.Status.ReadyReplicas),
		},
	}

	return r.Status().Update(ctx, proplet)
}

func (r *PropletController) setupExternalPropletMQTT(ctx context.Context, proplet *propellerv1alpha1.Proplet) error {
	// Subscribe to proplet alive messages
	aliveTopicTemplate := fmt.Sprintf("m/%s/c/%s/messages/control/proplet/alive", r.domainID, r.channelID)
	
	return r.pubsub.Subscribe(ctx, aliveTopicTemplate, func(topic string, msg map[string]interface{}) error {
		propletID, ok := msg["proplet_id"].(string)
		if !ok || propletID != proplet.Name {
			return nil // Not for this proplet
		}

		// Update last seen timestamp
		now := metav1.Now()
		proplet.Status.LastSeen = &now
		proplet.Status.Phase = propellerv1alpha1.PropletPhaseRunning

		// Update the proplet status
		return r.Status().Update(ctx, proplet)
	})
}

func (r *PropletController) SetupWithManager(mgr ctrl.Manager) error {
	r.Scheme = mgr.GetScheme()
	
	return ctrl.NewControllerManagedBy(mgr).
		For(&propellerv1alpha1.Proplet{}).
		Owns(&appsv1.Deployment{}).
		Complete(r)
}