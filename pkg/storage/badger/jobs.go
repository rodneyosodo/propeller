package badger

import (
	"context"
	"encoding/json"
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
	key := []byte("job:" + j.ID)
	val, err := json.Marshal(j)
	if err != nil {
		return job.Job{}, fmt.Errorf("marshal error: %w", err)
	}
	if err := r.db.set(key, val); err != nil {
		return job.Job{}, fmt.Errorf("%w: %w", ErrCreate, err)
	}

	return j, nil
}

func (r *jobRepo) Get(ctx context.Context, id string) (job.Job, error) {
	key := []byte("job:" + id)
	val, err := r.db.get(key)
	if err != nil {
		return job.Job{}, ErrNotFound
	}
	var j job.Job
	if err := json.Unmarshal(val, &j); err != nil {
		return job.Job{}, fmt.Errorf("unmarshal error: %w", err)
	}

	return j, nil
}

func (r *jobRepo) List(ctx context.Context, offset, limit uint64) (jobs []job.Job, total uint64, err error) {
	prefix := []byte("job:")
	total, err = r.db.countWithPrefix(prefix)
	if err != nil {
		return nil, 0, err
	}
	values, err := r.db.listWithPrefix(prefix, offset, limit)
	if err != nil {
		return nil, 0, err
	}
	jobs = make([]job.Job, len(values))
	for i, val := range values {
		var j job.Job
		if err := json.Unmarshal(val, &j); err != nil {
			return nil, 0, fmt.Errorf("unmarshal error: %w", err)
		}
		jobs[i] = j
	}

	return jobs, total, nil
}

func (r *jobRepo) Delete(ctx context.Context, id string) error {
	key := []byte("job:" + id)

	return r.db.delete(key)
}
