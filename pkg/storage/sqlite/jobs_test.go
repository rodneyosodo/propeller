package sqlite_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/absmach/propeller/pkg/job"
	"github.com/absmach/propeller/pkg/storage/sqlite"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestDB(t *testing.T) *sqlite.Database {
	t.Helper()
	db, err := sqlite.NewDatabase(filepath.Join(t.TempDir(), "test.db"))
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	return db
}

func TestJobCreate(t *testing.T) {
	t.Parallel()
	repo := sqlite.NewJobRepository(newTestDB(t))

	j := job.Job{
		ID:            uuid.NewString(),
		Name:          "my-job",
		ExecutionMode: "parallel",
		CreatedAt:     time.Now().UTC().Truncate(time.Second),
		UpdatedAt:     time.Now().UTC().Truncate(time.Second),
	}

	created, err := repo.Create(context.Background(), j)
	require.NoError(t, err)
	assert.Equal(t, j.ID, created.ID)
	assert.Equal(t, j.Name, created.Name)
	assert.Equal(t, j.ExecutionMode, created.ExecutionMode)
}

func TestJobGet(t *testing.T) {
	t.Parallel()
	repo := sqlite.NewJobRepository(newTestDB(t))

	j := job.Job{
		ID:            uuid.NewString(),
		Name:          "get-job",
		ExecutionMode: "sequential",
		CreatedAt:     time.Now().UTC().Truncate(time.Second),
		UpdatedAt:     time.Now().UTC().Truncate(time.Second),
	}
	_, err := repo.Create(context.Background(), j)
	require.NoError(t, err)

	got, err := repo.Get(context.Background(), j.ID)
	require.NoError(t, err)
	assert.Equal(t, j.ID, got.ID)
	assert.Equal(t, j.Name, got.Name)
	assert.Equal(t, j.ExecutionMode, got.ExecutionMode)
}

func TestJobGet_NotFound(t *testing.T) {
	t.Parallel()
	repo := sqlite.NewJobRepository(newTestDB(t))

	_, err := repo.Get(context.Background(), "nonexistent-id")
	assert.ErrorIs(t, err, sqlite.ErrNotFound)
}

func TestJobList(t *testing.T) {
	t.Parallel()
	repo := sqlite.NewJobRepository(newTestDB(t))

	now := time.Now().UTC().Truncate(time.Second)
	jobs := []job.Job{
		{ID: uuid.NewString(), Name: "job-a", ExecutionMode: "parallel", CreatedAt: now, UpdatedAt: now},
		{ID: uuid.NewString(), Name: "job-b", ExecutionMode: "sequential", CreatedAt: now.Add(time.Second), UpdatedAt: now},
		{ID: uuid.NewString(), Name: "job-c", ExecutionMode: "parallel", CreatedAt: now.Add(2 * time.Second), UpdatedAt: now},
	}
	for _, j := range jobs {
		_, err := repo.Create(context.Background(), j)
		require.NoError(t, err)
	}

	got, total, err := repo.List(context.Background(), 0, 10)
	require.NoError(t, err)
	assert.Equal(t, uint64(3), total)
	assert.Len(t, got, 3)
	// Results are ordered by created_at DESC, so job-c is first.
	assert.Equal(t, "job-c", got[0].Name)
	assert.Equal(t, "job-b", got[1].Name)
	assert.Equal(t, "job-a", got[2].Name)
}

func TestJobList_Pagination(t *testing.T) {
	t.Parallel()
	repo := sqlite.NewJobRepository(newTestDB(t))

	now := time.Now().UTC().Truncate(time.Second)
	for i := range 5 {
		j := job.Job{
			ID:            uuid.NewString(),
			Name:          "job",
			ExecutionMode: "parallel",
			CreatedAt:     now.Add(time.Duration(i) * time.Second),
			UpdatedAt:     now,
		}
		_, err := repo.Create(context.Background(), j)
		require.NoError(t, err)
	}

	page1, total, err := repo.List(context.Background(), 0, 3)
	require.NoError(t, err)
	assert.Equal(t, uint64(5), total)
	assert.Len(t, page1, 3)

	page2, total, err := repo.List(context.Background(), 3, 3)
	require.NoError(t, err)
	assert.Equal(t, uint64(5), total)
	assert.Len(t, page2, 2)
}

func TestJobList_Empty(t *testing.T) {
	t.Parallel()
	repo := sqlite.NewJobRepository(newTestDB(t))

	got, total, err := repo.List(context.Background(), 0, 10)
	require.NoError(t, err)
	assert.Equal(t, uint64(0), total)
	assert.Empty(t, got)
}

func TestJobDelete(t *testing.T) {
	t.Parallel()
	repo := sqlite.NewJobRepository(newTestDB(t))

	j := job.Job{
		ID:            uuid.NewString(),
		Name:          "to-delete",
		ExecutionMode: "parallel",
		CreatedAt:     time.Now().UTC().Truncate(time.Second),
		UpdatedAt:     time.Now().UTC().Truncate(time.Second),
	}
	_, err := repo.Create(context.Background(), j)
	require.NoError(t, err)

	err = repo.Delete(context.Background(), j.ID)
	require.NoError(t, err)

	_, err = repo.Get(context.Background(), j.ID)
	assert.ErrorIs(t, err, sqlite.ErrNotFound)
}
