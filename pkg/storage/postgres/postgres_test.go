package postgres_test

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"testing"
	"time"

	"github.com/absmach/propeller/pkg/proplet"
	"github.com/absmach/propeller/pkg/storage/postgres"
	"github.com/absmach/propeller/pkg/storage/testutil"
	"github.com/absmach/propeller/task"
	"github.com/google/uuid"
	"github.com/ory/dockertest/v3"
	"github.com/ory/dockertest/v3/docker"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	testDB    *postgres.Database
	invalidID = "invalid-id-that-does-not-exist"
)

func TestMain(m *testing.M) {
	pool, err := dockertest.NewPool("")
	if err != nil {
		log.Fatalf("Could not connect to docker: %s", err)
	}

	container, err := pool.RunWithOptions(&dockertest.RunOptions{
		Repository: "postgres",
		Tag:        "16.2-alpine",
		Env: []string{
			"POSTGRES_USER=test",
			"POSTGRES_PASSWORD=test",
			"POSTGRES_DB=test",
			"listen_addresses = '*'",
		},
	}, func(config *docker.HostConfig) {
		config.AutoRemove = true
		config.RestartPolicy = docker.RestartPolicy{Name: "no"}
	})
	if err != nil {
		log.Fatalf("Could not start container: %s", err)
	}

	port := container.GetPort("5432/tcp")

	pool.MaxWait = 120 * time.Second
	if err := pool.Retry(func() error {
		url := fmt.Sprintf("host=localhost port=%s user=test dbname=test password=test sslmode=disable", port)
		db, err := sql.Open("pgx", url)
		if err != nil {
			return err
		}
		return db.Ping()
	}); err != nil {
		log.Fatalf("Could not connect to docker: %s", err)
	}

	testDB, err = postgres.NewDatabase("localhost", port, "test", "test", "test", "disable")
	if err != nil {
		log.Fatalf("Could not setup test DB connection: %s", err)
	}

	code := m.Run()

	testDB.Close()
	if err := pool.Purge(container); err != nil {
		log.Fatalf("Could not purge container: %s", err)
	}

	os.Exit(code)
}

func TestTaskRepository_Create(t *testing.T) {
	repo := postgres.NewTaskRepository(testDB)

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
			desc: "create task with daemon flag",
			task: func() task.Task {
				tk := testutil.TestTask(uuid.NewString())
				tk.Daemon = true
				return tk
			}(),
			err: nil,
		},
		{
			desc: "create task with encrypted flag",
			task: func() task.Task {
				tk := testutil.TestTask(uuid.NewString())
				tk.Encrypted = true
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
	repo := postgres.NewTaskRepository(testDB)
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
			err:    postgres.ErrTaskNotFound,
		},
		{
			desc:   "get with empty ID",
			taskID: "",
			err:    postgres.ErrTaskNotFound,
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
	repo := postgres.NewTaskRepository(testDB)
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
		{
			desc: "update task with results",
			task: func() task.Task {
				tk := testTask
				tk.Results = map[string]any{"status": "success", "output": "test output"}
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
	repo := postgres.NewTaskRepository(testDB)
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
	repo := postgres.NewTaskRepository(testDB)
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
			err:    nil, // PostgreSQL doesn't return error for non-existing deletes
		},
	}

	for _, tc := range cases {
		t.Run(tc.desc, func(t *testing.T) {
			err := repo.Delete(ctx, tc.taskID)
			assert.Equal(t, tc.err, err)
			if err == nil && tc.taskID == testTask.ID {
				_, err := repo.Get(ctx, tc.taskID)
				assert.Equal(t, postgres.ErrTaskNotFound, err)
			}
		})
	}
}

func TestPropletRepository_Create(t *testing.T) {
	repo := postgres.NewPropletRepository(testDB)

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
	}

	for _, tc := range cases {
		t.Run(tc.desc, func(t *testing.T) {
			ctx := context.Background()
			err := repo.Create(ctx, tc.proplet)
			assert.Equal(t, tc.err, err)
			if err == nil {
				testDB.ExecContext(ctx, "DELETE FROM proplets WHERE id = $1", tc.proplet.ID)
			}
		})
	}
}

func TestPropletRepository_Get(t *testing.T) {
	repo := postgres.NewPropletRepository(testDB)
	ctx := context.Background()

	testProplet := testutil.TestProplet(uuid.NewString())
	err := repo.Create(ctx, testProplet)
	require.Nil(t, err)
	defer testDB.ExecContext(ctx, "DELETE FROM proplets WHERE id = $1", testProplet.ID)

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
			err:       postgres.ErrPropletNotFound,
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
	repo := postgres.NewPropletRepository(testDB)
	ctx := context.Background()

	testProplet := testutil.TestProplet(uuid.NewString())
	err := repo.Create(ctx, testProplet)
	require.Nil(t, err)
	defer testDB.ExecContext(ctx, "DELETE FROM proplets WHERE id = $1", testProplet.ID)

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
	repo := postgres.NewPropletRepository(testDB)
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
	taskRepo := postgres.NewTaskRepository(testDB)
	propletRepo := postgres.NewPropletRepository(testDB)
	repo := postgres.NewTaskPropletRepository(testDB)
	ctx := context.Background()

	testTask := testutil.TestTask(uuid.NewString())
	testProplet := testutil.TestProplet(uuid.NewString())

	_, err := taskRepo.Create(ctx, testTask)
	require.Nil(t, err)
	defer taskRepo.Delete(ctx, testTask.ID)

	err = propletRepo.Create(ctx, testProplet)
	require.Nil(t, err)
	defer testDB.ExecContext(ctx, "DELETE FROM proplets WHERE id = $1", testProplet.ID)

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
			err:       postgres.ErrNotFound,
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
	taskRepo := postgres.NewTaskRepository(testDB)
	propletRepo := postgres.NewPropletRepository(testDB)
	repo := postgres.NewMetricsRepository(testDB)
	ctx := context.Background()

	testTask := testutil.TestTask(uuid.NewString())
	testProplet := testutil.TestProplet(uuid.NewString())

	_, err := taskRepo.Create(ctx, testTask)
	require.Nil(t, err)
	defer taskRepo.Delete(ctx, testTask.ID)

	err = propletRepo.Create(ctx, testProplet)
	require.Nil(t, err)
	defer testDB.ExecContext(ctx, "DELETE FROM proplets WHERE id = $1", testProplet.ID)

	// Create metrics
	metrics := postgres.TaskMetrics{
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
		metrics   postgres.TaskMetrics
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

	testDB.ExecContext(ctx, "DELETE FROM task_metrics WHERE task_id = $1", testTask.ID)
}

func TestMetricsRepository_PropletMetrics(t *testing.T) {
	propletRepo := postgres.NewPropletRepository(testDB)
	repo := postgres.NewMetricsRepository(testDB)
	ctx := context.Background()

	testProplet := testutil.TestProplet(uuid.NewString())
	err := propletRepo.Create(ctx, testProplet)
	require.Nil(t, err)
	defer testDB.ExecContext(ctx, "DELETE FROM proplets WHERE id = $1", testProplet.ID)

	// Create metrics
	metrics := postgres.PropletMetrics{
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
		metrics   postgres.PropletMetrics
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

	testDB.ExecContext(ctx, "DELETE FROM proplet_metrics WHERE proplet_id = $1", testProplet.ID)
}
