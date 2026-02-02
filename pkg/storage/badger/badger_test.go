package badger_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/absmach/propeller/pkg/proplet"
	"github.com/absmach/propeller/pkg/storage/badger"
	"github.com/absmach/propeller/pkg/storage/testutil"
	"github.com/absmach/propeller/task"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	testDB    *badger.Database
	invalidID = "invalid-id-that-does-not-exist"
)

func TestMain(m *testing.M) {
	tmpDir := os.TempDir()
	dbPath := filepath.Join(tmpDir, "badger_test_"+uuid.NewString())

	var err error
	testDB, err = badger.NewDatabase(dbPath)
	if err != nil {
		panic(err)
	}

	code := m.Run()

	testDB.Close()
	os.RemoveAll(dbPath)

	os.Exit(code)
}

func TestTaskRepository_Create(t *testing.T) {
	repo := badger.NewTaskRepository(testDB)

	cases := []struct {
		desc string
		task task.Task
		err  error
	}{
		{
			desc: "create new task successfully",
			task: testutil.TestTask(uuid.NewString()),
			err:  nil,
		},
		{
			desc: "create task with empty name",
			task: func() task.Task {
				tk := testutil.TestTask(uuid.NewString())
				tk.Name = ""
				return tk
			}(),
			err: nil,
		},
		{
			desc: "create task with nil monitoring profile",
			task: func() task.Task {
				tk := testutil.TestTask(uuid.NewString())
				tk.MonitoringProfile = nil
				return tk
			}(),
			err: nil,
		},
		{
			desc: "create task with nil env",
			task: func() task.Task {
				tk := testutil.TestTask(uuid.NewString())
				tk.Env = nil
				return tk
			}(),
			err: nil,
		},
	}

	for _, tc := range cases {
		t.Run(tc.desc, func(t *testing.T) {
			ctx := context.Background()
			created, err := repo.Create(ctx, tc.task)
			assert.Equal(t, tc.err, err, fmt.Sprintf("%s: expected error %v, got %v", tc.desc, tc.err, err))
			if err == nil {
				assert.Equal(t, tc.task.ID, created.ID)
				assert.Equal(t, tc.task.Name, created.Name)
				assert.Equal(t, tc.task.State, created.State)

				repo.Delete(ctx, tc.task.ID)
			}
		})
	}
}

func TestTaskRepository_Get(t *testing.T) {
	repo := badger.NewTaskRepository(testDB)
	ctx := context.Background()

	testTask := testutil.TestTask(uuid.NewString())
	_, err := repo.Create(ctx, testTask)
	require.Nil(t, err)
	defer repo.Delete(ctx, testTask.ID)

	cases := []struct {
		desc   string
		taskID string
		err    error
	}{
		{
			desc:   "get existing task",
			taskID: testTask.ID,
			err:    nil,
		},
		{
			desc:   "get non-existing task",
			taskID: invalidID,
			err:    badger.ErrTaskNotFound,
		},
		{
			desc:   "get with empty ID",
			taskID: "",
			err:    badger.ErrTaskNotFound,
		},
	}

	for _, tc := range cases {
		t.Run(tc.desc, func(t *testing.T) {
			retrieved, err := repo.Get(ctx, tc.taskID)
			assert.Equal(t, tc.err, err, fmt.Sprintf("%s: expected error %v, got %v", tc.desc, tc.err, err))
			if err == nil {
				assert.Equal(t, testTask.ID, retrieved.ID)
				assert.Equal(t, testTask.Name, retrieved.Name)
				assert.Equal(t, testTask.State, retrieved.State)
			}
		})
	}
}

func TestTaskRepository_Update(t *testing.T) {
	repo := badger.NewTaskRepository(testDB)
	ctx := context.Background()

	testTask := testutil.TestTask(uuid.NewString())
	_, err := repo.Create(ctx, testTask)
	require.Nil(t, err)
	defer repo.Delete(ctx, testTask.ID)

	cases := []struct {
		desc string
		task task.Task
		err  error
	}{
		{
			desc: "update task name",
			task: func() task.Task {
				tk := testTask
				tk.Name = "updated-name"
				tk.UpdatedAt = time.Now()
				return tk
			}(),
			err: nil,
		},
		{
			desc: "update task state to running",
			task: func() task.Task {
				tk := testTask
				tk.State = task.Running
				tk.UpdatedAt = time.Now()
				return tk
			}(),
			err: nil,
		},
		{
			desc: "update task state to completed",
			task: func() task.Task {
				tk := testTask
				tk.State = task.Completed
				tk.UpdatedAt = time.Now()
				return tk
			}(),
			err: nil,
		},
	}

	for _, tc := range cases {
		t.Run(tc.desc, func(t *testing.T) {
			err := repo.Update(ctx, tc.task)
			assert.Equal(t, tc.err, err, fmt.Sprintf("%s: expected error %v, got %v", tc.desc, tc.err, err))
			if err == nil {
				retrieved, err := repo.Get(ctx, tc.task.ID)
				require.Nil(t, err)
				assert.Equal(t, tc.task.Name, retrieved.Name)
				assert.Equal(t, tc.task.State, retrieved.State)
			}
		})
	}
}

func TestTaskRepository_List(t *testing.T) {
	repo := badger.NewTaskRepository(testDB)
	ctx := context.Background()

	numTasks := 5
	taskIDs := make([]string, numTasks)
	for i := 0; i < numTasks; i++ {
		tk := testutil.TestTask(uuid.NewString())
		taskIDs[i] = tk.ID
		_, err := repo.Create(ctx, tk)
		require.Nil(t, err)
	}
	defer func() {
		for _, id := range taskIDs {
			repo.Delete(ctx, id)
		}
	}()

	cases := []struct {
		desc        string
		offset      uint64
		limit       uint64
		minExpected int
	}{
		{
			desc:        "list all tasks",
			offset:      0,
			limit:       10,
			minExpected: numTasks,
		},
		{
			desc:        "list with limit",
			offset:      0,
			limit:       3,
			minExpected: 3,
		},
		{
			desc:        "list with offset",
			offset:      2,
			limit:       10,
			minExpected: 3,
		},
		{
			desc:        "list with large offset",
			offset:      100,
			limit:       10,
			minExpected: 0,
		},
	}

	for _, tc := range cases {
		t.Run(tc.desc, func(t *testing.T) {
			tasks, total, err := repo.List(ctx, tc.offset, tc.limit)
			assert.Nil(t, err)
			assert.GreaterOrEqual(t, int(total), numTasks)
			assert.GreaterOrEqual(t, len(tasks), tc.minExpected)
			if tc.limit > 0 {
				assert.LessOrEqual(t, len(tasks), int(tc.limit))
			}
		})
	}
}

func TestTaskRepository_Delete(t *testing.T) {
	repo := badger.NewTaskRepository(testDB)
	ctx := context.Background()

	cases := []struct {
		desc   string
		taskID string
		setup  func() string
		err    error
	}{
		{
			desc: "delete existing task",
			setup: func() string {
				testTask := testutil.TestTask(uuid.NewString())
				_, err := repo.Create(ctx, testTask)
				require.Nil(t, err)
				return testTask.ID
			},
			err: nil,
		},
		{
			desc: "delete non-existing task",
			setup: func() string {
				return invalidID
			},
			err: nil, // Badger doesn't return error for non-existing deletes
		},
	}

	for _, tc := range cases {
		t.Run(tc.desc, func(t *testing.T) {
			taskID := tc.setup()
			err := repo.Delete(ctx, taskID)
			assert.Equal(t, tc.err, err)
			if err == nil && taskID != invalidID {
				_, err := repo.Get(ctx, taskID)
				assert.Equal(t, badger.ErrTaskNotFound, err)
			}
		})
	}
}

func TestPropletRepository_Create(t *testing.T) {
	repo := badger.NewPropletRepository(testDB)

	cases := []struct {
		desc    string
		proplet proplet.Proplet
		err     error
	}{
		{
			desc:    "create new proplet successfully",
			proplet: testutil.TestProplet(uuid.NewString()),
			err:     nil,
		},
		{
			desc: "create proplet with empty name",
			proplet: func() proplet.Proplet {
				p := testutil.TestProplet(uuid.NewString())
				p.Name = ""
				return p
			}(),
			err: nil,
		},
		{
			desc: "create proplet with zero task count",
			proplet: func() proplet.Proplet {
				p := testutil.TestProplet(uuid.NewString())
				p.TaskCount = 0
				return p
			}(),
			err: nil,
		},
		{
			desc: "create proplet not alive",
			proplet: func() proplet.Proplet {
				p := testutil.TestProplet(uuid.NewString())
				p.Alive = false
				return p
			}(),
			err: nil,
		},
	}

	for _, tc := range cases {
		t.Run(tc.desc, func(t *testing.T) {
			ctx := context.Background()
			err := repo.Create(ctx, tc.proplet)
			assert.Equal(t, tc.err, err)
			// Note: Cleanup is not needed for Badger as it's all in-memory during tests
		})
	}
}

func TestPropletRepository_Get(t *testing.T) {
	repo := badger.NewPropletRepository(testDB)
	ctx := context.Background()

	testProplet := testutil.TestProplet(uuid.NewString())
	err := repo.Create(ctx, testProplet)
	require.Nil(t, err)

	cases := []struct {
		desc      string
		propletID string
		err       error
	}{
		{
			desc:      "get existing proplet",
			propletID: testProplet.ID,
			err:       nil,
		},
		{
			desc:      "get non-existing proplet",
			propletID: invalidID,
			err:       badger.ErrPropletNotFound,
		},
		{
			desc:      "get with empty ID",
			propletID: "",
			err:       badger.ErrPropletNotFound,
		},
	}

	for _, tc := range cases {
		t.Run(tc.desc, func(t *testing.T) {
			retrieved, err := repo.Get(ctx, tc.propletID)
			assert.Equal(t, tc.err, err)
			if err == nil {
				assert.Equal(t, testProplet.ID, retrieved.ID)
				assert.Equal(t, testProplet.Name, retrieved.Name)
			}
		})
	}
}

func TestPropletRepository_Update(t *testing.T) {
	repo := badger.NewPropletRepository(testDB)
	ctx := context.Background()

	testProplet := testutil.TestProplet(uuid.NewString())
	err := repo.Create(ctx, testProplet)
	require.Nil(t, err)

	cases := []struct {
		desc    string
		proplet proplet.Proplet
		err     error
	}{
		{
			desc: "update proplet task count",
			proplet: func() proplet.Proplet {
				p := testProplet
				p.TaskCount = 10
				return p
			}(),
			err: nil,
		},
		{
			desc: "update proplet alive status to false",
			proplet: func() proplet.Proplet {
				p := testProplet
				p.Alive = false
				return p
			}(),
			err: nil,
		},
		{
			desc: "update proplet alive status to true",
			proplet: func() proplet.Proplet {
				p := testProplet
				p.Alive = true
				return p
			}(),
			err: nil,
		},
		{
			desc: "update proplet name",
			proplet: func() proplet.Proplet {
				p := testProplet
				p.Name = "updated-proplet-name"
				return p
			}(),
			err: nil,
		},
	}

	for _, tc := range cases {
		t.Run(tc.desc, func(t *testing.T) {
			err := repo.Update(ctx, tc.proplet)
			assert.Equal(t, tc.err, err)
			if err == nil {
				retrieved, err := repo.Get(ctx, tc.proplet.ID)
				require.Nil(t, err)
				assert.Equal(t, tc.proplet.TaskCount, retrieved.TaskCount)
				assert.Equal(t, tc.proplet.Alive, retrieved.Alive)
				assert.Equal(t, tc.proplet.Name, retrieved.Name)
			}
		})
	}
}

func TestPropletRepository_List(t *testing.T) {
	repo := badger.NewPropletRepository(testDB)
	ctx := context.Background()

	numProplets := 5
	propletIDs := make([]string, numProplets)
	for i := 0; i < numProplets; i++ {
		p := testutil.TestProplet(uuid.NewString())
		propletIDs[i] = p.ID
		err := repo.Create(ctx, p)
		require.Nil(t, err)
	}

	cases := []struct {
		desc        string
		offset      uint64
		limit       uint64
		minExpected int
	}{
		{
			desc:        "list all proplets",
			offset:      0,
			limit:       10,
			minExpected: numProplets,
		},
		{
			desc:        "list with limit",
			offset:      0,
			limit:       3,
			minExpected: 3,
		},
		{
			desc:        "list with offset",
			offset:      2,
			limit:       10,
			minExpected: 3,
		},
		{
			desc:        "list with large offset",
			offset:      100,
			limit:       10,
			minExpected: 0,
		},
	}

	for _, tc := range cases {
		t.Run(tc.desc, func(t *testing.T) {
			proplets, total, err := repo.List(ctx, tc.offset, tc.limit)
			assert.Nil(t, err)
			assert.GreaterOrEqual(t, int(total), numProplets)
			assert.GreaterOrEqual(t, len(proplets), tc.minExpected)
		})
	}
}

func TestTaskPropletRepository(t *testing.T) {
	taskRepo := badger.NewTaskRepository(testDB)
	propletRepo := badger.NewPropletRepository(testDB)
	repo := badger.NewTaskPropletRepository(testDB)
	ctx := context.Background()

	testTask := testutil.TestTask(uuid.NewString())
	testProplet := testutil.TestProplet(uuid.NewString())

	_, err := taskRepo.Create(ctx, testTask)
	require.Nil(t, err)
	defer taskRepo.Delete(ctx, testTask.ID)

	err = propletRepo.Create(ctx, testProplet)
	require.Nil(t, err)

	cases := []struct {
		desc      string
		taskID    string
		propletID string
		operation string
		err       error
	}{
		{
			desc:      "create task-proplet mapping",
			taskID:    testTask.ID,
			propletID: testProplet.ID,
			operation: "create",
			err:       nil,
		},
		{
			desc:      "get existing mapping",
			taskID:    testTask.ID,
			propletID: testProplet.ID,
			operation: "get",
			err:       nil,
		},
		{
			desc:      "get non-existing mapping",
			taskID:    invalidID,
			operation: "get",
			err:       badger.ErrNotFound,
		},
		{
			desc:      "delete existing mapping",
			taskID:    testTask.ID,
			operation: "delete",
			err:       nil,
		},
		{
			desc:      "get deleted mapping",
			taskID:    testTask.ID,
			operation: "get",
			err:       badger.ErrNotFound,
		},
	}

	for _, tc := range cases {
		t.Run(tc.desc, func(t *testing.T) {
			switch tc.operation {
			case "create":
				err := repo.Create(ctx, tc.taskID, tc.propletID)
				assert.Equal(t, tc.err, err)
			case "get":
				propletID, err := repo.Get(ctx, tc.taskID)
				assert.Equal(t, tc.err, err)
				if err == nil {
					assert.Equal(t, tc.propletID, propletID)
				}
			case "delete":
				err := repo.Delete(ctx, tc.taskID)
				assert.Equal(t, tc.err, err)
			}
		})
	}
}

func TestMetricsRepository_TaskMetrics(t *testing.T) {
	taskRepo := badger.NewTaskRepository(testDB)
	propletRepo := badger.NewPropletRepository(testDB)
	repo := badger.NewMetricsRepository(testDB)
	ctx := context.Background()

	testTask := testutil.TestTask(uuid.NewString())
	testProplet := testutil.TestProplet(uuid.NewString())

	_, err := taskRepo.Create(ctx, testTask)
	require.Nil(t, err)
	defer taskRepo.Delete(ctx, testTask.ID)

	err = propletRepo.Create(ctx, testProplet)
	require.Nil(t, err)

	cases := []struct {
		desc      string
		operation string
		metrics   badger.TaskMetrics
		err       error
	}{
		{
			desc:      "create task metrics",
			operation: "create",
			metrics: badger.TaskMetrics{
				TaskID:    testTask.ID,
				PropletID: testProplet.ID,
				Metrics: proplet.ProcessMetrics{
					CPUPercent:    25.5,
					MemoryBytes:   1024 * 1024 * 100,
					MemoryPercent: 10.5,
				},
				Aggregated: &proplet.AggregatedMetrics{
					AvgCPUUsage: 20.0,
					MaxCPUUsage: 30.0,
					SampleCount: 10,
				},
				Timestamp: time.Now(),
			},
			err: nil,
		},
		{
			desc:      "create another task metrics",
			operation: "create",
			metrics: badger.TaskMetrics{
				TaskID:    testTask.ID,
				PropletID: testProplet.ID,
				Metrics: proplet.ProcessMetrics{
					CPUPercent:  30.0,
					MemoryBytes: 1024 * 1024 * 120,
				},
				Timestamp: time.Now().Add(time.Second),
			},
			err: nil,
		},
		{
			desc:      "list task metrics",
			operation: "list",
			err:       nil,
		},
	}

	createdCount := 0
	for _, tc := range cases {
		t.Run(tc.desc, func(t *testing.T) {
			switch tc.operation {
			case "create":
				err := repo.CreateTaskMetrics(ctx, tc.metrics)
				assert.Equal(t, tc.err, err)
				if err == nil {
					createdCount++
				}
			case "list":
				retrieved, total, err := repo.ListTaskMetrics(ctx, testTask.ID, 0, 10)
				assert.Nil(t, err)
				assert.GreaterOrEqual(t, int(total), createdCount)
				assert.GreaterOrEqual(t, len(retrieved), createdCount)
				if len(retrieved) > 0 {
					assert.Equal(t, testTask.ID, retrieved[0].TaskID)
				}
			}
		})
	}
}

func TestMetricsRepository_PropletMetrics(t *testing.T) {
	propletRepo := badger.NewPropletRepository(testDB)
	repo := badger.NewMetricsRepository(testDB)
	ctx := context.Background()

	testProplet := testutil.TestProplet(uuid.NewString())
	err := propletRepo.Create(ctx, testProplet)
	require.Nil(t, err)

	cases := []struct {
		desc      string
		operation string
		metrics   badger.PropletMetrics
		err       error
	}{
		{
			desc:      "create proplet metrics",
			operation: "create",
			metrics: badger.PropletMetrics{
				PropletID: testProplet.ID,
				Namespace: "default",
				Timestamp: time.Now(),
				CPU: proplet.CPUMetrics{
					UserSeconds:   10.5,
					SystemSeconds: 5.2,
					Percent:       15.7,
				},
				Memory: proplet.MemoryMetrics{
					RSSBytes:       1024 * 1024 * 200,
					HeapAllocBytes: 1024 * 1024 * 50,
				},
			},
			err: nil,
		},
		{
			desc:      "create another proplet metrics",
			operation: "create",
			metrics: badger.PropletMetrics{
				PropletID: testProplet.ID,
				Namespace: "default",
				Timestamp: time.Now().Add(time.Second),
				CPU: proplet.CPUMetrics{
					Percent: 20.0,
				},
				Memory: proplet.MemoryMetrics{
					RSSBytes: 1024 * 1024 * 250,
				},
			},
			err: nil,
		},
		{
			desc:      "list proplet metrics",
			operation: "list",
			err:       nil,
		},
	}

	createdCount := 0
	for _, tc := range cases {
		t.Run(tc.desc, func(t *testing.T) {
			switch tc.operation {
			case "create":
				err := repo.CreatePropletMetrics(ctx, tc.metrics)
				assert.Equal(t, tc.err, err)
				if err == nil {
					createdCount++
				}
			case "list":
				retrieved, total, err := repo.ListPropletMetrics(ctx, testProplet.ID, 0, 10)
				assert.Nil(t, err)
				assert.GreaterOrEqual(t, int(total), createdCount)
				assert.GreaterOrEqual(t, len(retrieved), createdCount)
				if len(retrieved) > 0 {
					assert.Equal(t, testProplet.ID, retrieved[0].PropletID)
				}
			}
		})
	}
}
