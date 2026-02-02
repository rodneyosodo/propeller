package testutil

import (
	"time"

	"github.com/absmach/propeller/pkg/proplet"
	"github.com/absmach/propeller/task"
)

func TestTask(id string) task.Task {
	return task.Task{
		ID:        id,
		Name:      "test-task-" + id,
		State:     task.Pending,
		ImageURL:  "docker.io/library/alpine:latest",
		File:      []byte("test file content"),
		CLIArgs:   []string{"echo", "hello"},
		Inputs:    []uint64{1, 2, 3},
		Env:       map[string]string{"KEY": "value", "FOO": "bar"},
		Daemon:    false,
		Encrypted: false,
		MonitoringProfile: &proplet.MonitoringProfile{
			Enabled:  true,
			Interval: 5 * time.Second,
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
}

func TestProplet(id string) proplet.Proplet {
	return proplet.Proplet{
		ID:           id,
		Name:         "test-proplet-" + id,
		TaskCount:    0,
		Alive:        true,
		AliveHistory: []time.Time{time.Now().Add(-10 * time.Second), time.Now()},
	}
}
