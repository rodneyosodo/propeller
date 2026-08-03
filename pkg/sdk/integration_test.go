package sdk_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/absmach/propeller/manager"
	"github.com/absmach/propeller/manager/api"
	"github.com/absmach/propeller/manager/mocks"
	"github.com/absmach/propeller/pkg/proplet"
	"github.com/absmach/propeller/pkg/sdk"
	"github.com/absmach/propeller/pkg/task"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func newTestSDK(t *testing.T, svc manager.Service) sdk.SDK {
	t.Helper()

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	handler := api.MakeHandler(svc, logger, "test")
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	return sdk.NewSDK(sdk.Config{ManagerURL: server.URL})
}

func TestSDKGetProplet(t *testing.T) {
	t.Parallel()
	m := mocks.NewMockService(t)
	lastAlive := time.Now().Add(-5 * time.Second)
	m.On("GetProplet", mock.Anything, "f9d9d045-e118-48a7-927c-d3725d82f33a").Return(proplet.Proplet{
		ID:        "f9d9d045-e118-48a7-927c-d3725d82f33a",
		Name:      "node-a",
		TaskCount: 2,
		Alive:     true,
		AliveHistory: []time.Time{
			lastAlive,
		},
	}, nil)

	psdk := newTestSDK(t, m)

	p, err := psdk.GetProplet("f9d9d045-e118-48a7-927c-d3725d82f33a")
	require.NoError(t, err)
	require.Equal(t, "f9d9d045-e118-48a7-927c-d3725d82f33a", p.ID)
	require.Equal(t, "node-a", p.Name)
	require.Equal(t, uint64(2), p.TaskCount)
	require.True(t, p.Alive)
	require.NotNil(t, p.LastAliveAt)
}

func TestSDKGetPropletMetrics(t *testing.T) {
	t.Parallel()
	m := mocks.NewMockService(t)
	m.On("GetPropletMetrics", mock.Anything, "f9d9d045-e118-48a7-927c-d3725d82f33a", uint64(0), uint64(10)).Return(manager.PropletMetricsPage{
		Offset: 0,
		Limit:  10,
		Total:  1,
		Metrics: []manager.PropletMetrics{
			{
				PropletID: "f9d9d045-e118-48a7-927c-d3725d82f33a",
				Namespace: "default",
				Timestamp: time.Now(),
				CPU:       proplet.CPUMetrics{Percent: 12.5},
			},
		},
	}, nil)

	psdk := newTestSDK(t, m)

	page, err := psdk.GetPropletMetrics("f9d9d045-e118-48a7-927c-d3725d82f33a", 0, 10)
	require.NoError(t, err)
	require.Equal(t, uint64(1), page.Total)
	require.Len(t, page.Metrics, 1)
	require.Equal(t, "f9d9d045-e118-48a7-927c-d3725d82f33a", page.Metrics[0].PropletID)
}

func TestSDKGetTaskMetrics(t *testing.T) {
	t.Parallel()
	m := mocks.NewMockService(t)
	m.On("GetTaskMetrics", mock.Anything, "77a8b7ae-bacb-4505-aedb-17732b94ccc4", uint64(0), uint64(10)).Return(manager.TaskMetricsPage{
		Offset: 0,
		Limit:  10,
		Total:  1,
		Metrics: []manager.TaskMetrics{
			{
				TaskID:    "77a8b7ae-bacb-4505-aedb-17732b94ccc4",
				PropletID: "f9d9d045-e118-48a7-927c-d3725d82f33a",
				Metrics:   proplet.ProcessMetrics{CPUPercent: 3.5},
			},
		},
	}, nil)

	psdk := newTestSDK(t, m)

	page, err := psdk.GetTaskMetrics("77a8b7ae-bacb-4505-aedb-17732b94ccc4", 0, 10)
	require.NoError(t, err)
	require.Equal(t, uint64(1), page.Total)
	require.Len(t, page.Metrics, 1)
	require.Equal(t, "77a8b7ae-bacb-4505-aedb-17732b94ccc4", page.Metrics[0].TaskID)
}

func TestSDKGetTaskResults(t *testing.T) {
	t.Parallel()
	m := mocks.NewMockService(t)
	m.On("GetTaskResults", mock.Anything, "77a8b7ae-bacb-4505-aedb-17732b94ccc4").Return(map[string]any{
		"output": 42,
	}, nil)

	psdk := newTestSDK(t, m)

	results, err := psdk.GetTaskResults("77a8b7ae-bacb-4505-aedb-17732b94ccc4")
	require.NoError(t, err)
	require.NotNil(t, results)
	res, ok := results.(map[string]any)
	require.True(t, ok)
	require.InDelta(t, 42.0, res["output"], 1e-9)
}

func TestSDKCreateWorkflow(t *testing.T) {
	t.Parallel()
	m := mocks.NewMockService(t)
	m.On("CreateWorkflow", mock.Anything, mock.Anything).Return([]task.Task{
		{ID: "cc6791b7-e8a7-48d3-8ef3-db3153121f98", Name: "fetch"},
		{ID: "608f7c92-c374-43f1-8c08-cac9cc545774", Name: "process", DependsOn: []string{"cc6791b7-e8a7-48d3-8ef3-db3153121f98"}},
	}, nil)

	psdk := newTestSDK(t, m)

	created, err := psdk.CreateWorkflow([]sdk.Task{
		{Name: "fetch"},
		{Name: "process", DependsOn: []string{"cc6791b7-e8a7-48d3-8ef3-db3153121f98"}},
	})
	require.NoError(t, err)
	require.Len(t, created, 2)
	require.Equal(t, "cc6791b7-e8a7-48d3-8ef3-db3153121f98", created[0].ID)
}

func TestSDKUploadTaskFile(t *testing.T) {
	t.Parallel()
	m := mocks.NewMockService(t)
	wasm := []byte("\x00asm\x01\x00\x00\x00")

	uploaded := make(chan task.Task, 1)
	m.On("UpdateTask", mock.Anything, mock.Anything).Return(func(_ context.Context, t task.Task) task.Task {
		uploaded <- t

		return t
	}, nil)

	psdk := newTestSDK(t, m)

	dir := t.TempDir()
	path := filepath.Join(dir, "app.wasm")
	require.NoError(t, os.WriteFile(path, wasm, 0o600))

	got, err := psdk.UploadTaskFile("77a8b7ae-bacb-4505-aedb-17732b94ccc4", path)
	require.NoError(t, err)
	require.Equal(t, "77a8b7ae-bacb-4505-aedb-17732b94ccc4", got.ID)

	select {
	case tsk := <-uploaded:
		require.Equal(t, "77a8b7ae-bacb-4505-aedb-17732b94ccc4", tsk.ID)
		require.Equal(t, wasm, tsk.File)
	case <-time.After(time.Second):
		t.Fatal("UpdateTask was not called")
	}
}

func TestSDKListTasksMetadata(t *testing.T) {
	t.Parallel()
	m := mocks.NewMockService(t)
	m.On("ListTasks", mock.Anything, manager.PageMetadata{
		Offset:   0,
		Limit:    10,
		Metadata: map[string]any{"env": "prod"},
	}).Return(task.TaskPage{
		Offset: 0,
		Limit:  10,
		Total:  1,
		Tasks: []task.Task{
			{ID: "77a8b7ae-bacb-4505-aedb-17732b94ccc4", Name: "main"},
		},
	}, nil)

	psdk := newTestSDK(t, m)

	page, err := psdk.ListTasks(sdk.PageMetadata{
		Offset:   0,
		Limit:    10,
		Metadata: map[string]any{"env": "prod"},
	})
	require.NoError(t, err)
	require.Equal(t, uint64(1), page.Total)
	require.Len(t, page.Tasks, 1)
}

func TestSDKConfigureExperiment(t *testing.T) {
	t.Parallel()

	m := mocks.NewMockService(t)
	m.On("ConfigureExperiment", mock.Anything, mock.Anything).Return(nil)

	psdk := newTestSDK(t, m)

	res, err := psdk.ConfigureExperiment(sdk.ExperimentConfig{
		ExperimentID: "exp-001",
		RoundID:      "round-1",
		ModelRef:     "model-v1",
		Participants: []string{"p1", "p2"},
		KOfN:         2,
		TimeoutS:     300,
	})
	require.NoError(t, err)
	require.Equal(t, "exp-001", res.ExperimentID)
	require.Equal(t, "round-1", res.RoundID)
	require.Equal(t, "configured", res.Status)
}

func TestSDKGetFLTask(t *testing.T) {
	t.Parallel()

	m := mocks.NewMockService(t)
	m.On("GetFLTask", mock.Anything, "round-1", "p1").Return(manager.FLTask{
		RoundID:  "round-1",
		ModelRef: "model-v1",
		Config:   map[string]any{"lr": 0.01},
	}, nil)

	psdk := newTestSDK(t, m)

	flTask, err := psdk.GetFLTask("round-1", "p1")
	require.NoError(t, err)
	require.Equal(t, "round-1", flTask.RoundID)
	require.Equal(t, "model-v1", flTask.ModelRef)
}

func TestSDKPostFLUpdate(t *testing.T) {
	t.Parallel()

	m := mocks.NewMockService(t)
	m.On("PostFLUpdate", mock.Anything, mock.Anything).Return(nil)

	psdk := newTestSDK(t, m)

	err := psdk.PostFLUpdate(sdk.FLUpdate{
		RoundID:      "round-1",
		PropletID:    "p1",
		BaseModelURI: "oci://model",
		NumSamples:   10,
		Update:       map[string]any{"w": 1},
	})
	require.NoError(t, err)
}

func TestSDKGetRoundStatus(t *testing.T) {
	t.Parallel()

	m := mocks.NewMockService(t)
	m.On("GetRoundStatus", mock.Anything, "round-1").Return(manager.RoundStatus{
		RoundID:    "round-1",
		Completed:  true,
		NumUpdates: 2,
		KOfN:       2,
	}, nil)

	psdk := newTestSDK(t, m)

	status, err := psdk.GetRoundStatus("round-1")
	require.NoError(t, err)
	require.True(t, status.Completed)
	require.Equal(t, 2, status.NumUpdates)
}

func TestSDKGetHealth(t *testing.T) {
	t.Parallel()

	m := mocks.NewMockService(t)

	psdk := newTestSDK(t, m)

	info, err := psdk.GetHealth()
	require.NoError(t, err)
	require.Equal(t, "pass", info.Status)
	require.Equal(t, "test", info.InstanceID)
	require.NotEmpty(t, info.Version)
}

func TestTaskRedactedFileUnmarshal(t *testing.T) {
	t.Parallel()

	const redacted = `{
		"id": "77a8b7ae-bacb-4505-aedb-17732b94ccc4",
		"name": "add",
		"file": "AGFzbQEAAA<REDACTED>RpdmFsdWU=",
		"results": "30\n"
	}`

	var tsk sdk.Task
	require.NoError(t, json.Unmarshal([]byte(redacted), &tsk))
	require.Equal(t, "77a8b7ae-bacb-4505-aedb-17732b94ccc4", tsk.ID)
	require.Equal(t, "AGFzbQEAAA<REDACTED>RpdmFsdWU=", tsk.File)
	require.Equal(t, "30\n", tsk.Results)
}

func TestSDKListJobsEmpty(t *testing.T) {
	t.Parallel()

	m := mocks.NewMockService(t)
	m.On("ListJobs", mock.Anything, uint64(0), uint64(10), "").Return(manager.JobPage{
		Offset: 0,
		Limit:  10,
		Total:  0,
		Jobs:   []manager.JobSummary{},
	}, nil)

	psdk := newTestSDK(t, m)

	page, err := psdk.ListJobs(0, 10, "")
	require.NoError(t, err)
	require.Equal(t, uint64(0), page.Total)
	require.Empty(t, page.Jobs)
}
