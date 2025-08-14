package scheduler

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/absmach/propeller/proplet"
	"github.com/absmach/propeller/task"
)

func TestHybridScheduler_SelectProplet(t *testing.T) {
	tests := []struct {
		name                string
		task                task.Task
		proplets            []proplet.Proplet
		expectedPropletID   string
		expectedError       bool
		mockK8sScheduler    *MockScheduler
		mockExternalScheduler *MockScheduler
	}{
		{
			name: "Select K8s proplet for compute-intensive task",
			task: task.Task{
				Name: "ml-inference-task",
			},
			proplets: []proplet.Proplet{
				{
					ID:        "k8s-proplet-1",
					Name:      "proplet-deployment-abc123-def456",
					TaskCount: 1,
					Alive:     true,
				},
				{
					ID:        "external-esp32-1",
					Name:      "esp32-device-001",
					TaskCount: 0,
					Alive:     true,
				},
			},
			expectedPropletID: "k8s-proplet-1",
			expectedError:     false,
			mockK8sScheduler:  &MockScheduler{},
			mockExternalScheduler: &MockScheduler{},
		},
		{
			name: "Select external proplet for edge task",
			task: task.Task{
				Name: "sensor-monitoring-task",
			},
			proplets: []proplet.Proplet{
				{
					ID:        "k8s-proplet-1",
					Name:      "proplet-deployment-abc123-def456",
					TaskCount: 2,
					Alive:     true,
				},
				{
					ID:        "external-esp32-1",
					Name:      "esp32-device-001",
					TaskCount: 0,
					Alive:     true,
				},
			},
			expectedPropletID: "external-esp32-1",
			expectedError:     false,
			mockK8sScheduler:  &MockScheduler{},
			mockExternalScheduler: &MockScheduler{},
		},
		{
			name: "No proplets available",
			task: task.Task{
				Name: "any-task",
			},
			proplets:      []proplet.Proplet{},
			expectedError: true,
			mockK8sScheduler: &MockScheduler{},
			mockExternalScheduler: &MockScheduler{},
		},
		{
			name: "Fallback when preferred type not available",
			task: task.Task{
				Name: "compute-task", // Prefers K8s
			},
			proplets: []proplet.Proplet{
				// Only external proplets available
				{
					ID:        "external-esp32-1",
					Name:      "esp32-device-001",
					TaskCount: 1,
					Alive:     true,
				},
			},
			expectedPropletID: "external-esp32-1",
			expectedError:     false,
			mockK8sScheduler:  &MockScheduler{},
			mockExternalScheduler: &MockScheduler{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup mock schedulers
			if !tt.expectedError && tt.expectedPropletID != "" {
				expectedProplet := proplet.Proplet{}
				for _, p := range tt.proplets {
					if p.ID == tt.expectedPropletID {
						expectedProplet = p
						break
					}
				}
				
				if isK8sProplet(expectedProplet) {
					tt.mockK8sScheduler.On("SelectProplet", tt.task, mock.AnythingOfType("[]proplet.Proplet")).Return(expectedProplet, nil)
				} else {
					tt.mockExternalScheduler.On("SelectProplet", tt.task, mock.AnythingOfType("[]proplet.Proplet")).Return(expectedProplet, nil)
				}
			}

			// Create hybrid scheduler
			scheduler := NewHybridScheduler(tt.mockK8sScheduler, tt.mockExternalScheduler)

			// Execute test
			selectedProplet, err := scheduler.SelectProplet(tt.task, tt.proplets)

			// Verify results
			if tt.expectedError {
				assert.Error(t, err)
				assert.Equal(t, ErrNoPropletAvailable, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedPropletID, selectedProplet.ID)
			}

			// Verify mock expectations
			tt.mockK8sScheduler.AssertExpectations(t)
			tt.mockExternalScheduler.AssertExpectations(t)
		})
	}
}

func TestIsK8sProplet(t *testing.T) {
	tests := []struct {
		name     string
		proplet  proplet.Proplet
		expected bool
	}{
		{
			name: "K8s proplet with deployment name pattern",
			proplet: proplet.Proplet{
				ID:   "proplet-deployment-abc123-def456",
				Name: "proplet-deployment-abc123-def456",
			},
			expected: true,
		},
		{
			name: "K8s proplet with proplet prefix",
			proplet: proplet.Proplet{
				ID:   "proplet-worker-1",
				Name: "proplet-worker-1",
			},
			expected: true,
		},
		{
			name: "External proplet with short name",
			proplet: proplet.Proplet{
				ID:   "esp32-001",
				Name: "esp32-device-001",
			},
			expected: false,
		},
		{
			name: "External proplet with device name",
			proplet: proplet.Proplet{
				ID:   "device123",
				Name: "raspberry-pi-edge",
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isK8sProplet(tt.proplet)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsEdgeTask(t *testing.T) {
	tests := []struct {
		name     string
		task     task.Task
		expected bool
	}{
		{
			name: "Edge task with sensor keyword",
			task: task.Task{Name: "sensor-data-collection"},
			expected: true,
		},
		{
			name: "Edge task with iot keyword",
			task: task.Task{Name: "iot-device-monitoring"},
			expected: true,
		},
		{
			name: "Edge task with edge keyword",
			task: task.Task{Name: "edge-analytics"},
			expected: true,
		},
		{
			name: "Compute task",
			task: task.Task{Name: "ml-model-training"},
			expected: false,
		},
		{
			name: "Generic task",
			task: task.Task{Name: "data-processing"},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isEdgeTask(tt.task)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsComputeIntensiveTask(t *testing.T) {
	tests := []struct {
		name     string
		task     task.Task
		expected bool
	}{
		{
			name: "ML task",
			task: task.Task{Name: "ml-inference"},
			expected: true,
		},
		{
			name: "AI task",
			task: task.Task{Name: "ai-model-training"},
			expected: true,
		},
		{
			name: "Compute task",
			task: task.Task{Name: "compute-simulation"},
			expected: true,
		},
		{
			name: "Batch processing task",
			task: task.Task{Name: "batch-data-processing"},
			expected: true,
		},
		{
			name: "Simple task",
			task: task.Task{Name: "hello-world"},
			expected: false,
		},
		{
			name: "Sensor task",
			task: task.Task{Name: "sensor-reading"},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isComputeIntensiveTask(tt.task)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestResourceAwareScheduler_SelectProplet(t *testing.T) {
	tests := []struct {
		name              string
		task              task.Task
		proplets          []proplet.Proplet
		expectedPropletID string
		expectedError     bool
	}{
		{
			name: "Select proplet with low task count",
			task: task.Task{Name: "test-task"},
			proplets: []proplet.Proplet{
				{
					ID:        "proplet-1",
					TaskCount: 3,
					Alive:     true,
				},
				{
					ID:        "proplet-2",
					TaskCount: 1,
					Alive:     true,
				},
				{
					ID:        "proplet-3",
					TaskCount: 2,
					Alive:     true,
				},
			},
			expectedPropletID: "proplet-2", // Round-robin will select the first available
			expectedError:     false,
		},
		{
			name: "Reject overloaded proplets",
			task: task.Task{Name: "test-task"},
			proplets: []proplet.Proplet{
				{
					ID:        "proplet-1",
					TaskCount: 6, // Exceeds maxTasksPerProplet (5)
					Alive:     true,
				},
				{
					ID:        "proplet-2",
					TaskCount: 5, // At threshold
					Alive:     true,
				},
			},
			expectedError: true, // All proplets are overloaded
		},
		{
			name: "No proplets available",
			task: task.Task{Name: "test-task"},
			proplets: []proplet.Proplet{},
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scheduler := NewResourceAwareScheduler()

			selectedProplet, err := scheduler.SelectProplet(tt.task, tt.proplets)

			if tt.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedPropletID, selectedProplet.ID)
			}
		})
	}
}

// Mock scheduler for testing
type MockScheduler struct {
	mock.Mock
}

func (m *MockScheduler) SelectProplet(t task.Task, proplets []proplet.Proplet) (proplet.Proplet, error) {
	args := m.Called(t, proplets)
	return args.Get(0).(proplet.Proplet), args.Error(1)
}