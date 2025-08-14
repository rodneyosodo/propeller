package mocks

import (
	"context"

	"github.com/stretchr/testify/mock"
)

// MockPubSub is a mock implementation of the PubSub interface for testing
type MockPubSub struct {
	mock.Mock
}

// Publish publishes a message to the specified topic
func (m *MockPubSub) Publish(ctx context.Context, topic string, payload interface{}) error {
	args := m.Called(ctx, topic, payload)
	return args.Error(0)
}

// Subscribe subscribes to messages on the specified topic
func (m *MockPubSub) Subscribe(ctx context.Context, topic string, handler func(string, map[string]interface{}) error) error {
	args := m.Called(ctx, topic, handler)
	return args.Error(0)
}

// Close closes the MQTT connection
func (m *MockPubSub) Close() error {
	args := m.Called()
	return args.Error(0)
}