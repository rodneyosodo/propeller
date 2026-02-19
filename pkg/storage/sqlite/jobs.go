package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/absmach/propeller/pkg/job"
)

type jobRepo struct {
	db *Database
}

func NewJobRepository(db *Database) JobRepository {
	return &jobRepo{db: db}
}

func (r *jobRepo) Create(ctx context.Context, j job.Job) (job.Job, error) {
	_, err := r.db.ExecContext(
		ctx,
		`INSERT INTO jobs (id, name, execution_mode, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
		j.ID,
		j.Name,
		j.ExecutionMode,
		j.CreatedAt,
		j.UpdatedAt,
	)
	if err != nil {
		return job.Job{}, fmt.Errorf("%w: %w", ErrCreate, err)
	}

	return j, nil
}

func (r *jobRepo) Get(ctx context.Context, id string) (job.Job, error) {
	var j job.Job
	if err := r.db.GetContext(ctx, &j, `SELECT id, name, execution_mode, created_at, updated_at FROM jobs WHERE id = ?`, id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return job.Job{}, ErrNotFound
		}

		return job.Job{}, fmt.Errorf("%w: %w", ErrDBQuery, err)
	}

	return j, nil
}

func (r *jobRepo) List(ctx context.Context, offset, limit uint64) ([]job.Job, uint64, error) {
	var total uint64
	if err := r.db.GetContext(ctx, &total, "SELECT COUNT(*) FROM jobs"); err != nil {
		return nil, 0, fmt.Errorf("%w: %w", ErrDBQuery, err)
	}

	var jobs []job.Job
	if err := r.db.SelectContext(
		ctx,
		&jobs,
		`SELECT id, name, execution_mode, created_at, updated_at FROM jobs ORDER BY created_at DESC LIMIT ? OFFSET ?`,
		limit,
		offset,
	); err != nil {
		return nil, 0, fmt.Errorf("%w: %w", ErrDBQuery, err)
	}

	if jobs == nil {
		jobs = []job.Job{}
	}

	return jobs, total, nil
}

func (r *jobRepo) Delete(ctx context.Context, id string) error {
	if _, err := r.db.ExecContext(ctx, `DELETE FROM jobs WHERE id = ?`, id); err != nil {
		return fmt.Errorf("%w: %w", ErrDelete, err)
	}

	return nil
}
