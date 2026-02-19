package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/absmach/propeller/job"
)

type jobRepo struct {
	db *Database
}

func NewJobRepository(db *Database) JobRepository {
	return &jobRepo{db: db}
}

func (r *jobRepo) Create(ctx context.Context, j job.Job) (job.Job, error) {
	query := `INSERT INTO jobs (id, name, execution_mode, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)`

	_, err := r.db.ExecContext(ctx, query, j.ID, j.Name, j.ExecutionMode, j.CreatedAt, j.UpdatedAt)
	if err != nil {
		return job.Job{}, fmt.Errorf("%w: %w", ErrCreate, err)
	}

	return j, nil
}

func (r *jobRepo) Get(ctx context.Context, id string) (job.Job, error) {
	query := `SELECT id, name, execution_mode, created_at, updated_at FROM jobs WHERE id = ?`

	var j job.Job
	if err := r.db.GetContext(ctx, &j, query, id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return job.Job{}, ErrNotFound
		}

		return job.Job{}, fmt.Errorf("%w: %w", ErrDBQuery, err)
	}

	return j, nil
}

func (r *jobRepo) List(ctx context.Context, offset, limit uint64) (jobs []job.Job, total uint64, err error) {
	if err = r.db.GetContext(ctx, &total, "SELECT COUNT(*) FROM jobs"); err != nil {
		return nil, 0, fmt.Errorf("%w: %w", ErrDBQuery, err)
	}

	query := `SELECT id, name, execution_mode, created_at, updated_at FROM jobs ORDER BY created_at DESC LIMIT ? OFFSET ?`
	if err = r.db.SelectContext(ctx, &jobs, query, limit, offset); err != nil {
		return nil, 0, fmt.Errorf("%w: %w", ErrDBQuery, err)
	}

	if jobs == nil {
		jobs = []job.Job{}
	}

	return jobs, total, nil
}

func (r *jobRepo) Delete(ctx context.Context, id string) error {
	query := `DELETE FROM jobs WHERE id = ?`

	if _, err := r.db.ExecContext(ctx, query, id); err != nil {
		return fmt.Errorf("%w: %w", ErrDelete, err)
	}

	return nil
}
