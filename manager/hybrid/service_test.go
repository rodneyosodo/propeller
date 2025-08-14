package hybrid

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	propellerv1alpha1 "github.com/absmach/propeller/k8s/api/v1alpha1"
	managermocks "github.com/absmach/propeller/manager/mocks"
	mqttmocks "github.com/absmach/propeller/pkg/mqtt/mocks"
	"github.com/absmach/propeller/proplet"
	"github.com/absmach/propeller/task"
)

func TestHybridOrchestrator_CreateTask(t *testing.T) {
	// Setup test scheme
	s := scheme.Scheme
	require.NoError(t, propellerv1alpha1.AddToScheme(s))

	tests := []struct {
		name         string
		taskSpec     TaskSpec
		legacyTask   task.Task
		expectedError bool
	}{
		{
			name: "Create task successfully",
			taskSpec: TaskSpec{
				Name:      "test-task",
				ImageURL:  "registry.example.com/test:latest",
				CLIArgs:   []string{"--mode", "test"},
				Inputs:    map[string]interface{}{"param1": "value1"},
				Namespace: "test-namespace",
			},
			legacyTask: task.Task{
				ID:       "test-task-id",
				Name:     "test-task",
				ImageURL: "registry.example.com/test:latest",
				CLIArgs:  []string{"--mode", "test"},
				Inputs:   map[string]interface{}{"param1": "value1"},
			},
			expectedError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create fake K8s client
			fakeClient := fake.NewClientBuilder().
				WithScheme(s).
				Build()

			// Create mock legacy manager
			mockManager := &managermocks.MockService{}
			mockManager.On("CreateTask", mock.Anything, mock.AnythingOfType("task.Task")).Return(tt.legacyTask, nil)

			// Create mock MQTT pubsub
			mockPubSub := &mqttmocks.MockPubSub{}

			// Create hybrid orchestrator
			logger := &testLogger{}
			orchestrator := NewHybridOrchestrator(
				fakeClient,
				mockManager,
				mockPubSub,
				"test-domain",
				"test-channel",
				logger,
			)

			// Execute test
			ctx := context.Background()
			err := orchestrator.CreateTask(ctx, tt.taskSpec)

			// Verify results
			if tt.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)

				// Verify K8s Task resource was created
				var tasks propellerv1alpha1.TaskList
				err = fakeClient.List(ctx, &tasks)
				require.NoError(t, err)
				assert.Len(t, tasks.Items, 1)

				k8sTask := tasks.Items[0]
				assert.Equal(t, tt.taskSpec.Name, k8sTask.Spec.Name)
				assert.Equal(t, tt.taskSpec.ImageURL, k8sTask.Spec.ImageURL)
				assert.Equal(t, tt.legacyTask.ID, k8sTask.Labels["propeller.absmach.fr/legacy-task-id"])
			}

			// Verify mock expectations
			mockManager.AssertExpectations(t)
		})
	}
}

func TestHybridOrchestrator_GetProplets(t *testing.T) {
	// Setup test scheme
	s := scheme.Scheme
	require.NoError(t, propellerv1alpha1.AddToScheme(s))

	now := metav1.Now()
	
	// Create test K8s proplets
	k8sProplet := &propellerv1alpha1.Proplet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "k8s-proplet-1",
			Namespace: "default",
		},
		Spec: propellerv1alpha1.PropletSpec{
			Type: propellerv1alpha1.PropletTypeK8s,
			K8s: &propellerv1alpha1.K8sPropletSpec{
				Image: "propeller/proplet:latest",
			},
		},
		Status: propellerv1alpha1.PropletStatus{
			Phase:     propellerv1alpha1.PropletPhaseRunning,
			TaskCount: 2,
			LastSeen:  &now,
		},
	}

	externalProplet := &propellerv1alpha1.Proplet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "external-proplet-1",
			Namespace: "default",
		},
		Spec: propellerv1alpha1.PropletSpec{
			Type: propellerv1alpha1.PropletTypeExternal,
			External: &propellerv1alpha1.ExternalPropletSpec{
				DeviceType:   "esp32",
				Capabilities: []string{"wasm", "sensor"},
			},
		},
		Status: propellerv1alpha1.PropletStatus{
			Phase:     propellerv1alpha1.PropletPhaseRunning,
			TaskCount: 1,
			LastSeen:  &now,
		},
	}

	// Create fake K8s client
	fakeClient := fake.NewClientBuilder().
		WithScheme(s).
		WithObjects(k8sProplet, externalProplet).
		Build()

	// Create mock legacy manager
	mockManager := &managermocks.MockService{}
	legacyProplets := proplet.PropletPage{
		Proplets: []proplet.Proplet{
			{
				ID:           "legacy-proplet-1",
				Name:         "legacy-device-001",
				TaskCount:    1,
				Alive:        true,
				AliveHistory: []time.Time{time.Now()},
			},
		},
	}
	mockManager.On("ListProplets", mock.Anything, uint64(0), uint64(100)).Return(legacyProplets, nil)

	// Create mock MQTT pubsub
	mockPubSub := &mqttmocks.MockPubSub{}

	// Create hybrid orchestrator
	logger := &testLogger{}
	orchestrator := NewHybridOrchestrator(
		fakeClient,
		mockManager,
		mockPubSub,
		"test-domain",
		"test-channel",
		logger,
	)

	// Execute test
	ctx := context.Background()
	proplets, err := orchestrator.GetProplets(ctx)

	// Verify results
	require.NoError(t, err)
	assert.Len(t, proplets, 3) // 2 K8s + 1 legacy

	// Verify K8s proplets
	k8sPropletInfo := findPropletByID(proplets, "k8s-proplet-1")
	require.NotNil(t, k8sPropletInfo)
	assert.Equal(t, "k8s", k8sPropletInfo.Type)
	assert.Equal(t, "Running", k8sPropletInfo.Phase)
	assert.Equal(t, int32(2), k8sPropletInfo.TaskCount)

	externalPropletInfo := findPropletByID(proplets, "external-proplet-1")
	require.NotNil(t, externalPropletInfo)
	assert.Equal(t, "external", externalPropletInfo.Type)
	assert.Equal(t, "Running", externalPropletInfo.Phase)
	assert.Equal(t, "esp32", externalPropletInfo.DeviceType)
	assert.Equal(t, []string{"wasm", "sensor"}, externalPropletInfo.Capabilities)

	// Verify legacy proplet
	legacyPropletInfo := findPropletByID(proplets, "legacy-proplet-1")
	require.NotNil(t, legacyPropletInfo)
	assert.Equal(t, "external", legacyPropletInfo.Type)
	assert.Equal(t, "Running", legacyPropletInfo.Phase)

	// Verify mock expectations
	mockManager.AssertExpectations(t)
}

func TestHybridOrchestrator_HandleExternalPropletDiscovery(t *testing.T) {
	// Setup test scheme
	s := scheme.Scheme
	require.NoError(t, propellerv1alpha1.AddToScheme(s))

	// Create fake K8s client
	fakeClient := fake.NewClientBuilder().
		WithScheme(s).
		Build()

	// Create mock legacy manager
	mockManager := &managermocks.MockService{}
	mockManager.On("Subscribe", mock.Anything).Return(nil)

	// Create mock MQTT pubsub
	mockPubSub := &mqttmocks.MockPubSub{}

	// Create hybrid orchestrator
	logger := &testLogger{}
	orchestrator := NewHybridOrchestrator(
		fakeClient,
		mockManager,
		mockPubSub,
		"test-domain",
		"test-channel",
		logger,
	)

	// Test discovery message
	ctx := context.Background()
	handler := orchestrator.handleExternalPropletDiscovery(ctx)

	discoveryMsg := map[string]interface{}{
		"proplet_id":     "esp32-device-123",
		"device_type":    "esp32",
		"capabilities":   []interface{}{"wasm", "sensor", "wifi"},
	}

	err := handler("discovery-topic", discoveryMsg)
	assert.NoError(t, err)

	// Verify K8s Proplet resource was created
	var proplets propellerv1alpha1.PropletList
	err = fakeClient.List(ctx, &proplets)
	require.NoError(t, err)
	assert.Len(t, proplets.Items, 1)

	proplet := proplets.Items[0]
	assert.Equal(t, "esp32-device-123", proplet.Name)
	assert.Equal(t, propellerv1alpha1.PropletTypeExternal, proplet.Spec.Type)
	require.NotNil(t, proplet.Spec.External)
	assert.Equal(t, "esp32", proplet.Spec.External.DeviceType)
	assert.Equal(t, []string{"wasm", "sensor", "wifi"}, proplet.Spec.External.Capabilities)

	// Verify mock expectations
	mockManager.AssertExpectations(t)
}

func TestConvertPropletPreference(t *testing.T) {
	tests := []struct {
		input    string
		expected propellerv1alpha1.PropletPreference
	}{
		{"k8s", propellerv1alpha1.PropletPreferenceK8s},
		{"external", propellerv1alpha1.PropletPreferenceExternal},
		{"any", propellerv1alpha1.PropletPreferenceAny},
		{"invalid", propellerv1alpha1.PropletPreferenceAny}, // default
		{"", propellerv1alpha1.PropletPreferenceAny},        // default
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := convertPropletPreference(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestInferDeviceType(t *testing.T) {
	tests := []struct {
		name     string
		msg      map[string]interface{}
		expected string
	}{
		{
			name: "Device type in message",
			msg: map[string]interface{}{
				"device_type": "raspberry-pi",
			},
			expected: "raspberry-pi",
		},
		{
			name: "No device type in message",
			msg: map[string]interface{}{
				"proplet_id": "device-123",
			},
			expected: "unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := inferDeviceType(tt.msg)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestInferCapabilities(t *testing.T) {
	tests := []struct {
		name     string
		msg      map[string]interface{}
		expected []string
	}{
		{
			name: "Capabilities in message",
			msg: map[string]interface{}{
				"capabilities": []interface{}{"wasm", "sensor", "wifi"},
			},
			expected: []string{"wasm", "sensor", "wifi"},
		},
		{
			name: "No capabilities in message",
			msg: map[string]interface{}{
				"proplet_id": "device-123",
			},
			expected: []string{"wasm"}, // default
		},
		{
			name: "Invalid capabilities format",
			msg: map[string]interface{}{
				"capabilities": "not-an-array",
			},
			expected: []string{"wasm"}, // default
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := inferCapabilities(tt.msg)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// Helper functions and types

func findPropletByID(proplets []PropletInfo, id string) *PropletInfo {
	for _, p := range proplets {
		if p.ID == id {
			return &p
		}
	}
	return nil
}

// Simple test logger
type testLogger struct{}

func (l *testLogger) Debug(msg string, fields ...interface{}) {}
func (l *testLogger) Info(msg string, fields ...interface{})  {}
func (l *testLogger) Warn(msg string, fields ...interface{})  {}
func (l *testLogger) Error(msg string, fields ...interface{}) {}
func (l *testLogger) With(fields ...interface{}) *testLogger  { return l }