package mocks

import (
	"context"

	"github.com/stretchr/testify/mock"

	"github.com/absmach/propeller/proplet"
	"github.com/absmach/propeller/task"
)

// MockService is a mock implementation of the manager.Service interface
type MockService struct {
	mock.Mock
}

// GetProplet retrieves a proplet by ID
func (m *MockService) GetProplet(ctx context.Context, propletID string) (proplet.Proplet, error) {
	args := m.Called(ctx, propletID)
	return args.Get(0).(proplet.Proplet), args.Error(1)
}

// ListProplets lists proplets with pagination
func (m *MockService) ListProplets(ctx context.Context, offset, limit uint64) (proplet.PropletPage, error) {
	args := m.Called(ctx, offset, limit)
	return args.Get(0).(proplet.PropletPage), args.Error(1)
}

// SelectProplet selects a proplet for a task
func (m *MockService) SelectProplet(ctx context.Context, task task.Task) (proplet.Proplet, error) {
	args := m.Called(ctx, task)
	return args.Get(0).(proplet.Proplet), args.Error(1)
}

// CreateTask creates a new task
func (m *MockService) CreateTask(ctx context.Context, task task.Task) (task.Task, error) {
	args := m.Called(ctx, task)
	return args.Get(0).(task.Task), args.Error(1)
}

// GetTask retrieves a task by ID
func (m *MockService) GetTask(ctx context.Context, taskID string) (task.Task, error) {
	args := m.Called(ctx, taskID)
	return args.Get(0).(task.Task), args.Error(1)
}

// ListTasks lists tasks with pagination
func (m *MockService) ListTasks(ctx context.Context, offset, limit uint64) (task.TaskPage, error) {
	args := m.Called(ctx, offset, limit)
	return args.Get(0).(task.TaskPage), args.Error(1)
}

// UpdateTask updates a task
func (m *MockService) UpdateTask(ctx context.Context, task task.Task) (task.Task, error) {
	args := m.Called(ctx, task)
	return args.Get(0).(task.Task), args.Error(1)
}

// DeleteTask deletes a task
func (m *MockService) DeleteTask(ctx context.Context, taskID string) error {
	args := m.Called(ctx, taskID)
	return args.Error(0)
}

// StartTask starts a task
func (m *MockService) StartTask(ctx context.Context, taskID string) error {
	args := m.Called(ctx, taskID)
	return args.Error(0)
}

// StopTask stops a task
func (m *MockService) StopTask(ctx context.Context, taskID string) error {
	args := m.Called(ctx, taskID)
	return args.Error(0)
}

// Subscribe subscribes to MQTT topics
func (m *MockService) Subscribe(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}