package sqlite_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/absmach/propeller/pkg/proplet"
	"github.com/absmach/propeller/pkg/storage/sqlite"
	"github.com/absmach/propeller/pkg/storage/testutil"
	"github.com/absmach/propeller/task"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	testDB    *sqlite.Database
	invalidID = "invalid-id-that-does-not-exist"
)

func TestMain(m *testing.M) {
	tmpDir := os.TempDir()
	dbPath := filepath.Join(tmpDir, "test_"+uuid.NewString()+".db")

	var err error
	testDB, err = sqlite.NewDatabase(dbPath)
	if err != nil {
		panic(err)
	}

	code := m.Run()

	testDB.Close()
	os.Remove(dbPath)

	os.Exit(code)
}

func TestTaskRepository_Create(t *testing.T) {
	repo := sqlite.NewTaskRepository(testDB)

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
			desc: "create task with nil metadata",
			task: func() task.Task {
				tk := testutil.TestTask(uuid.NewString())
				tk.MonitoringProfile = nil
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
	repo := sqlite.NewTaskRepository(testDB)
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
			err:    sqlite.ErrTaskNotFound,
		},
		{
			desc:   "get with empty ID",
			taskID: "",
			err:    sqlite.ErrTaskNotFound,
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
	repo := sqlite.NewTaskRepository(testDB)
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
			desc: "update task state",
			task: func() task.Task {
				tk := testTask
				tk.State = task.Running
				tk.UpdatedAt = time.Now()
				return tk
			}(),
			err: nil,
		},
		{
			desc: "update non-existing task",
			task: func() task.Task {
				tk := testutil.TestTask(invalidID)
				tk.UpdatedAt = time.Now()
				return tk
			}(),
			err: nil, // SQLite doesn't return error for non-existing updates
		},
	}

	for _, tc := range cases {
		t.Run(tc.desc, func(t *testing.T) {
			err := repo.Update(ctx, tc.task)
			assert.Equal(t, tc.err, err, fmt.Sprintf("%s: expected error %v, got %v", tc.desc, tc.err, err))
			if err == nil && tc.task.ID == testTask.ID {
				retrieved, err := repo.Get(ctx, tc.task.ID)
				require.Nil(t, err)
				assert.Equal(t, tc.task.Name, retrieved.Name)
				assert.Equal(t, tc.task.State, retrieved.State)
			}
		})
	}
}

func TestTaskRepository_List(t *testing.T) {
	repo := sqlite.NewTaskRepository(testDB)
	ctx := context.Background()

	// Clean up before test
	testDB.ExecContext(ctx, "DELETE FROM tasks")

	numTasks := 5
	for i := 0; i < numTasks; i++ {
		tk := testutil.TestTask(uuid.NewString())
		_, err := repo.Create(ctx, tk)
		require.Nil(t, err)
	}

	cases := []struct {
		desc        string
		offset      uint64
		limit       uint64
		expectedLen int
	}{
		{
			desc:        "list all tasks",
			offset:      0,
			limit:       10,
			expectedLen: numTasks,
		},
		{
			desc:        "list with limit",
			offset:      0,
			limit:       3,
			expectedLen: 3,
		},
		{
			desc:        "list with offset",
			offset:      2,
			limit:       10,
			expectedLen: 3,
		},
		{
			desc:        "list with offset out of range",
			offset:      100,
			limit:       10,
			expectedLen: 0,
		},
	}

	for _, tc := range cases {
		t.Run(tc.desc, func(t *testing.T) {
			tasks, total, err := repo.List(ctx, tc.offset, tc.limit)
			assert.Nil(t, err)
			assert.Equal(t, uint64(numTasks), total)
			assert.Equal(t, tc.expectedLen, len(tasks))
		})
	}

	testDB.ExecContext(ctx, "DELETE FROM tasks")
}

func TestTaskRepository_Delete(t *testing.T) {
	repo := sqlite.NewTaskRepository(testDB)
	ctx := context.Background()

	testTask := testutil.TestTask(uuid.NewString())
	_, err := repo.Create(ctx, testTask)
	require.Nil(t, err)

	cases := []struct {
		desc   string
		taskID string
		err    error
	}{
		{
			desc:   "delete existing task",
			taskID: testTask.ID,
			err:    nil,
		},
		{
			desc:   "delete non-existing task",
			taskID: invalidID,
			err:    nil, // SQLite doesn't return error for non-existing deletes
		},
	}

	for _, tc := range cases {
		t.Run(tc.desc, func(t *testing.T) {
			err := repo.Delete(ctx, tc.taskID)
			assert.Equal(t, tc.err, err)
			if err == nil && tc.taskID == testTask.ID {
				_, err := repo.Get(ctx, tc.taskID)
				assert.Equal(t, sqlite.ErrTaskNotFound, err)
			}
		})
	}
}

func TestPropletRepository_Create(t *testing.T) {
	repo := sqlite.NewPropletRepository(testDB)

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
	}

	for _, tc := range cases {
		t.Run(tc.desc, func(t *testing.T) {
			ctx := context.Background()
			err := repo.Create(ctx, tc.proplet)
			assert.Equal(t, tc.err, err)
			if err == nil {
				testDB.ExecContext(ctx, "DELETE FROM proplets WHERE id = ?", tc.proplet.ID)
			}
		})
	}
}

func TestPropletRepository_Get(t *testing.T) {
	repo := sqlite.NewPropletRepository(testDB)
	ctx := context.Background()

	testProplet := testutil.TestProplet(uuid.NewString())
	err := repo.Create(ctx, testProplet)
	require.Nil(t, err)
	defer testDB.ExecContext(ctx, "DELETE FROM proplets WHERE id = ?", testProplet.ID)

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
			err:       sqlite.ErrPropletNotFound,
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
	repo := sqlite.NewPropletRepository(testDB)
	ctx := context.Background()

	testProplet := testutil.TestProplet(uuid.NewString())
	err := repo.Create(ctx, testProplet)
	require.Nil(t, err)
	defer testDB.ExecContext(ctx, "DELETE FROM proplets WHERE id = ?", testProplet.ID)

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
			desc: "update proplet alive status",
			proplet: func() proplet.Proplet {
				p := testProplet
				p.Alive = false
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
			}
		})
	}
}

func TestPropletRepository_List(t *testing.T) {
	repo := sqlite.NewPropletRepository(testDB)
	ctx := context.Background()

	// Clean up before test
	testDB.ExecContext(ctx, "DELETE FROM proplets")

	numProplets := 5
	for i := 0; i < numProplets; i++ {
		p := testutil.TestProplet(uuid.NewString())
		err := repo.Create(ctx, p)
		require.Nil(t, err)
	}

	cases := []struct {
		desc        string
		offset      uint64
		limit       uint64
		expectedLen int
	}{
		{
			desc:        "list all proplets",
			offset:      0,
			limit:       10,
			expectedLen: numProplets,
		},
		{
			desc:        "list with limit",
			offset:      0,
			limit:       3,
			expectedLen: 3,
		},
		{
			desc:        "list with offset",
			offset:      2,
			limit:       10,
			expectedLen: 3,
		},
	}

	for _, tc := range cases {
		t.Run(tc.desc, func(t *testing.T) {
			proplets, total, err := repo.List(ctx, tc.offset, tc.limit)
			assert.Nil(t, err)
			assert.Equal(t, uint64(numProplets), total)
			assert.Equal(t, tc.expectedLen, len(proplets))
		})
	}

	testDB.ExecContext(ctx, "DELETE FROM proplets")
}

func TestTaskPropletRepository(t *testing.T) {
	taskRepo := sqlite.NewTaskRepository(testDB)
	propletRepo := sqlite.NewPropletRepository(testDB)
	repo := sqlite.NewTaskPropletRepository(testDB)
	ctx := context.Background()

	testTask := testutil.TestTask(uuid.NewString())
	testProplet := testutil.TestProplet(uuid.NewString())

	_, err := taskRepo.Create(ctx, testTask)
	require.Nil(t, err)
	defer taskRepo.Delete(ctx, testTask.ID)

	err = propletRepo.Create(ctx, testProplet)
	require.Nil(t, err)
	defer testDB.ExecContext(ctx, "DELETE FROM proplets WHERE id = ?", testProplet.ID)

	cases := []struct {
		desc      string
		taskID    string
		propletID string
		operation string
		err       error
	}{
		{
			desc:      "create mapping",
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
			err:       sqlite.ErrNotFound,
		},
		{
			desc:      "delete mapping",
			taskID:    testTask.ID,
			operation: "delete",
			err:       nil,
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
	taskRepo := sqlite.NewTaskRepository(testDB)
	propletRepo := sqlite.NewPropletRepository(testDB)
	repo := sqlite.NewMetricsRepository(testDB)
	ctx := context.Background()

	testTask := testutil.TestTask(uuid.NewString())
	testProplet := testutil.TestProplet(uuid.NewString())

	_, err := taskRepo.Create(ctx, testTask)
	require.Nil(t, err)
	defer taskRepo.Delete(ctx, testTask.ID)

	err = propletRepo.Create(ctx, testProplet)
	require.Nil(t, err)
	defer testDB.ExecContext(ctx, "DELETE FROM proplets WHERE id = ?", testProplet.ID)

	// Create metrics
	metrics := sqlite.TaskMetrics{
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
	}

	cases := []struct {
		desc      string
		operation string
		metrics   sqlite.TaskMetrics
		err       error
	}{
		{
			desc:      "create task metrics",
			operation: "create",
			metrics:   metrics,
			err:       nil,
		},
		{
			desc:      "list task metrics",
			operation: "list",
			err:       nil,
		},
	}

	for _, tc := range cases {
		t.Run(tc.desc, func(t *testing.T) {
			switch tc.operation {
			case "create":
				err := repo.CreateTaskMetrics(ctx, tc.metrics)
				assert.Equal(t, tc.err, err)
			case "list":
				retrieved, total, err := repo.ListTaskMetrics(ctx, testTask.ID, 0, 10)
				assert.Nil(t, err)
				assert.Equal(t, uint64(1), total)
				assert.Equal(t, 1, len(retrieved))
				if len(retrieved) > 0 {
					assert.Equal(t, testTask.ID, retrieved[0].TaskID)
					assert.Equal(t, 25.5, retrieved[0].Metrics.CPUPercent)
				}
			}
		})
	}

	testDB.ExecContext(ctx, "DELETE FROM task_metrics WHERE task_id = ?", testTask.ID)
}

func TestMetricsRepository_PropletMetrics(t *testing.T) {
	propletRepo := sqlite.NewPropletRepository(testDB)
	repo := sqlite.NewMetricsRepository(testDB)
	ctx := context.Background()

	testProplet := testutil.TestProplet(uuid.NewString())
	err := propletRepo.Create(ctx, testProplet)
	require.Nil(t, err)
	defer testDB.ExecContext(ctx, "DELETE FROM proplets WHERE id = ?", testProplet.ID)

	// Create metrics
	metrics := sqlite.PropletMetrics{
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
	}

	cases := []struct {
		desc      string
		operation string
		metrics   sqlite.PropletMetrics
		err       error
	}{
		{
			desc:      "create proplet metrics",
			operation: "create",
			metrics:   metrics,
			err:       nil,
		},
		{
			desc:      "list proplet metrics",
			operation: "list",
			err:       nil,
		},
	}

	for _, tc := range cases {
		t.Run(tc.desc, func(t *testing.T) {
			switch tc.operation {
			case "create":
				err := repo.CreatePropletMetrics(ctx, tc.metrics)
				assert.Equal(t, tc.err, err)
			case "list":
				retrieved, total, err := repo.ListPropletMetrics(ctx, testProplet.ID, 0, 10)
				assert.Nil(t, err)
				assert.Equal(t, uint64(1), total)
				assert.Equal(t, 1, len(retrieved))
				if len(retrieved) > 0 {
					assert.Equal(t, testProplet.ID, retrieved[0].PropletID)
					assert.Equal(t, 15.7, retrieved[0].CPU.Percent)
				}
			}
		})
	}

	testDB.ExecContext(ctx, "DELETE FROM proplet_metrics WHERE proplet_id = ?", testProplet.ID)
}
