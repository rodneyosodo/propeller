package controller

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	propellerv1alpha1 "github.com/absmach/propeller/k8s/api/v1alpha1"
	"github.com/absmach/propeller/pkg/mqtt/mocks"
)

func TestPropletController_ReconcileK8sProplet(t *testing.T) {
	// Setup test scheme
	s := scheme.Scheme
	require.NoError(t, propellerv1alpha1.AddToScheme(s))

	tests := []struct {
		name           string
		proplet        *propellerv1alpha1.Proplet
		expectedError  bool
		expectedPhase  propellerv1alpha1.PropletPhase
		checkDeployment bool
	}{
		{
			name: "Create K8s proplet successfully",
			proplet: &propellerv1alpha1.Proplet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-proplet",
					Namespace: "test-namespace",
				},
				Spec: propellerv1alpha1.PropletSpec{
					Type: propellerv1alpha1.PropletTypeK8s,
					K8s: &propellerv1alpha1.K8sPropletSpec{
						Image:    "propeller/proplet:latest",
						Replicas: int32Ptr(2),
					},
					ConnectionConfig: propellerv1alpha1.ConnectionConfig{
						DomainID:  "test-domain",
						ChannelID: "test-channel",
						Broker:    "test-broker:1883",
					},
				},
			},
			expectedError:   false,
			expectedPhase:   propellerv1alpha1.PropletPhaseInitializing,
			checkDeployment: true,
		},
		{
			name: "K8s proplet without K8s spec should fail",
			proplet: &propellerv1alpha1.Proplet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "invalid-proplet",
					Namespace: "test-namespace",
				},
				Spec: propellerv1alpha1.PropletSpec{
					Type: propellerv1alpha1.PropletTypeK8s,
					// Missing K8s spec
					ConnectionConfig: propellerv1alpha1.ConnectionConfig{
						DomainID:  "test-domain",
						ChannelID: "test-channel",
					},
				},
			},
			expectedError:   true,
			checkDeployment: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create fake client
			fakeClient := fake.NewClientBuilder().
				WithScheme(s).
				WithObjects(tt.proplet).
				Build()

			// Create mock MQTT pubsub
			mockPubSub := &mocks.MockPubSub{}

			// Create controller
			logger := zap.New(zap.UseDevMode(true))
			controller := NewPropletController(
				fakeClient,
				logger.Sugar(),
				mockPubSub,
				"test-domain",
				"test-channel",
			)
			controller.Scheme = s

			// Test reconcile
			req := ctrl.Request{
				NamespacedName: types.NamespacedName{
					Name:      tt.proplet.Name,
					Namespace: tt.proplet.Namespace,
				},
			}

			ctx := context.Background()
			result, err := controller.Reconcile(ctx, req)

			if tt.expectedError {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)
			assert.NotEqual(t, ctrl.Result{}, result) // Should requeue

			// Check proplet status
			var updatedProplet propellerv1alpha1.Proplet
			err = fakeClient.Get(ctx, req.NamespacedName, &updatedProplet)
			require.NoError(t, err)

			assert.Equal(t, tt.expectedPhase, updatedProplet.Status.Phase)

			// Check deployment creation if expected
			if tt.checkDeployment {
				var deployment appsv1.Deployment
				deploymentName := types.NamespacedName{
					Name:      tt.proplet.Name + "-proplet",
					Namespace: tt.proplet.Namespace,
				}
				err = fakeClient.Get(ctx, deploymentName, &deployment)
				assert.NoError(t, err)

				// Verify deployment spec
				assert.Equal(t, tt.proplet.Spec.K8s.Image, deployment.Spec.Template.Spec.Containers[0].Image)
				assert.Equal(t, *tt.proplet.Spec.K8s.Replicas, *deployment.Spec.Replicas)
			}
		})
	}
}

func TestPropletController_ReconcileExternalProplet(t *testing.T) {
	// Setup test scheme
	s := scheme.Scheme
	require.NoError(t, propellerv1alpha1.AddToScheme(s))

	proplet := &propellerv1alpha1.Proplet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "external-proplet",
			Namespace: "test-namespace",
		},
		Spec: propellerv1alpha1.PropletSpec{
			Type: propellerv1alpha1.PropletTypeExternal,
			External: &propellerv1alpha1.ExternalPropletSpec{
				Endpoint:     "192.168.1.100:8080",
				DeviceType:   "esp32",
				Capabilities: []string{"wasm", "sensor"},
			},
			ConnectionConfig: propellerv1alpha1.ConnectionConfig{
				DomainID:  "test-domain",
				ChannelID: "test-channel",
				Broker:    "test-broker:1883",
			},
		},
	}

	// Create fake client
	fakeClient := fake.NewClientBuilder().
		WithScheme(s).
		WithObjects(proplet).
		Build()

	// Create mock MQTT pubsub
	mockPubSub := &mocks.MockPubSub{}
	mockPubSub.On("Subscribe", context.Background(), mock.AnythingOfType("string"), mock.AnythingOfType("func(string, map[string]interface{}) error")).Return(nil)

	// Create controller
	logger := zap.New(zap.UseDevMode(true))
	controller := NewPropletController(
		fakeClient,
		logger.Sugar(),
		mockPubSub,
		"test-domain",
		"test-channel",
	)
	controller.Scheme = s

	// Test reconcile
	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      proplet.Name,
			Namespace: proplet.Namespace,
		},
	}

	ctx := context.Background()
	result, err := controller.Reconcile(ctx, req)

	assert.NoError(t, err)
	assert.NotEqual(t, ctrl.Result{}, result) // Should requeue

	// Check proplet status
	var updatedProplet propellerv1alpha1.Proplet
	err = fakeClient.Get(ctx, req.NamespacedName, &updatedProplet)
	require.NoError(t, err)

	assert.Equal(t, propellerv1alpha1.PropletPhaseInitializing, updatedProplet.Status.Phase)

	// Verify MQTT subscription was called
	mockPubSub.AssertExpectations(t)
}

func TestPropletController_BuildPropletDeployment(t *testing.T) {
	proplet := &propellerv1alpha1.Proplet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-proplet",
			Namespace: "test-namespace",
		},
		Spec: propellerv1alpha1.PropletSpec{
			Type: propellerv1alpha1.PropletTypeK8s,
			K8s: &propellerv1alpha1.K8sPropletSpec{
				Image:    "propeller/proplet:v1.0.0",
				Replicas: int32Ptr(3),
			},
			ConnectionConfig: propellerv1alpha1.ConnectionConfig{
				DomainID:  "prod-domain",
				ChannelID: "control-channel",
				Broker:    "mqtt.example.com:1883",
			},
		},
	}

	// Create controller
	logger := zap.New(zap.UseDevMode(true))
	controller := NewPropletController(
		nil, // client not needed for this test
		logger.Sugar(),
		nil, // pubsub not needed for this test
		"prod-domain",
		"control-channel",
	)

	// Build deployment
	deployment := controller.buildPropletDeployment(proplet)

	// Verify deployment metadata
	assert.Equal(t, "test-proplet-proplet", deployment.Name)
	assert.Equal(t, "test-namespace", deployment.Namespace)

	// Verify deployment spec
	assert.Equal(t, int32(3), *deployment.Spec.Replicas)
	assert.Equal(t, "propeller/proplet:v1.0.0", deployment.Spec.Template.Spec.Containers[0].Image)

	// Verify environment variables
	container := deployment.Spec.Template.Spec.Containers[0]
	envMap := make(map[string]string)
	for _, env := range container.Env {
		envMap[env.Name] = env.Value
	}

	assert.Equal(t, "prod-domain", envMap["PROPELLER_DOMAIN_ID"])
	assert.Equal(t, "control-channel", envMap["PROPELLER_CHANNEL_ID"])
	assert.Equal(t, "mqtt.example.com:1883", envMap["MQTT_BROKER"])

	// Verify labels
	expectedLabels := map[string]string{
		"app.kubernetes.io/name":      "proplet",
		"app.kubernetes.io/instance":  "test-proplet",
		"app.kubernetes.io/component": "worker",
		"propeller.absmach.fr/proplet": "test-proplet",
	}
	for key, value := range expectedLabels {
		assert.Equal(t, value, deployment.Labels[key])
	}
}

// Helper function
func int32Ptr(i int32) *int32 {
	return &i
}