package manager_test

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/absmach/propeller/manager"
	mqttmocks "github.com/absmach/propeller/pkg/mqtt/mocks"
	"github.com/absmach/propeller/pkg/proplet"
	"github.com/absmach/propeller/pkg/scheduler"
	"github.com/absmach/propeller/pkg/storage"
	"github.com/absmach/propeller/pkg/task"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func newServiceWithRepos(t *testing.T) (manager.Service, *storage.Repositories) {
	t.Helper()
	repos, err := storage.NewRepositories(storage.Config{Type: "memory"})
	require.NoError(t, err)
	sched := scheduler.NewRoundRobin()
	pubsub := mqttmocks.NewMockPubSub(t)
	pubsub.On("Publish", mock.Anything, mock.Anything, mock.Anything).Return(nil).Maybe()
	pubsub.On("Subscribe", mock.Anything, mock.Anything, mock.Anything).Return(nil).Maybe()
	pubsub.On("Unsubscribe", mock.Anything, mock.Anything).Return(nil).Maybe()
	pubsub.On("Disconnect", mock.Anything).Return(nil).Maybe()
	logger := slog.Default()
	svc, _ := manager.NewService(repos, sched, pubsub, "test-domain", "test-channel", logger)

	return svc, repos
}

func TestListPropletsFilterByStatus(t *testing.T) {
	t.Parallel()
	svc, repos := newServiceWithRepos(t)
	ctx := context.Background()

	activeProplet := proplet.Proplet{
		ID:           uuid.NewString(),
		Name:         "active-proplet",
		AliveHistory: []time.Time{time.Now()},
	}
	inactiveProplet := proplet.Proplet{
		ID:           uuid.NewString(),
		Name:         "inactive-proplet",
		AliveHistory: []time.Time{time.Now().Add(-1 * time.Hour)},
	}
	require.NoError(t, repos.Proplets.Create(ctx, activeProplet))
	require.NoError(t, repos.Proplets.Create(ctx, inactiveProplet))

	all, err := svc.ListProplets(ctx, 0, 100, "")
	require.NoError(t, err)
	assert.Equal(t, uint64(2), all.Total)

	active, err := svc.ListProplets(ctx, 0, 100, manager.PropletStatusActive)
	require.NoError(t, err)
	assert.Equal(t, uint64(1), active.Total)
	assert.True(t, active.Proplets[0].Alive)

	inactive, err := svc.ListProplets(ctx, 0, 100, manager.PropletStatusInactive)
	require.NoError(t, err)
	assert.Equal(t, uint64(1), inactive.Total)
	assert.False(t, inactive.Proplets[0].Alive)
}

func TestListPropletsInvalidStatus(t *testing.T) {
	t.Parallel()
	svc, _ := newServiceWithRepos(t)

	_, err := svc.ListProplets(context.Background(), 0, 100, "unknown")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid value")
}

func TestListPropletsFilterPagination(t *testing.T) {
	t.Parallel()
	svc, repos := newServiceWithRepos(t)
	ctx := context.Background()

	for range 5 {
		p := proplet.Proplet{
			ID:           uuid.NewString(),
			Name:         uuid.NewString(),
			AliveHistory: []time.Time{time.Now()},
		}
		require.NoError(t, repos.Proplets.Create(ctx, p))
	}

	page, err := svc.ListProplets(ctx, 0, 3, manager.PropletStatusActive)
	require.NoError(t, err)
	assert.Equal(t, uint64(5), page.Total)
	assert.Len(t, page.Proplets, 3)

	page2, err := svc.ListProplets(ctx, 3, 3, manager.PropletStatusActive)
	require.NoError(t, err)
	assert.Equal(t, uint64(5), page2.Total)
	assert.Len(t, page2.Proplets, 2)
}

func TestListJobsInterruptedMapsToFailed(t *testing.T) {
	t.Parallel()
	svc := newService(t)
	ctx := context.Background()

	_, _, err := svc.CreateJob(ctx, "interrupted-job", []task.Task{
		{Name: "t1", State: task.Interrupted},
	}, "parallel")
	require.NoError(t, err)

	page, err := svc.ListJobs(ctx, 0, 100, manager.JobStatusFailed)
	require.NoError(t, err)
	assert.Equal(t, uint64(1), page.Total, "interrupted job must appear under 'failed' filter")
}

func TestListJobsScheduledMapsToRunning(t *testing.T) {
	t.Parallel()
	svc := newService(t)
	ctx := context.Background()

	_, _, err := svc.CreateJob(ctx, "scheduled-job", []task.Task{
		{Name: "t1", State: task.Scheduled},
	}, "parallel")
	require.NoError(t, err)

	page, err := svc.ListJobs(ctx, 0, 100, manager.JobStatusRunning)
	require.NoError(t, err)
	assert.Equal(t, uint64(1), page.Total, "scheduled job must appear under 'running' filter")
}
