package storage

import (
	"context"
	"fmt"

	pkgerrors "github.com/absmach/propeller/pkg/errors"
	"github.com/absmach/propeller/pkg/proplet"
	"github.com/absmach/propeller/pkg/task"
)

type memoryTaskRepo struct {
	storage Storage
}

func newMemoryTaskRepository(s Storage) TaskRepository {
	return &memoryTaskRepo{storage: s}
}

func (r *memoryTaskRepo) Create(ctx context.Context, t task.Task) (task.Task, error) {
	if err := r.storage.Create(ctx, t.ID, t); err != nil {
		return task.Task{}, err
	}

	return t, nil
}

func (r *memoryTaskRepo) Get(ctx context.Context, id string) (task.Task, error) {
	data, err := r.storage.Get(ctx, id)
	if err != nil {
		return task.Task{}, err
	}
	t, ok := data.(task.Task)
	if !ok {
		return task.Task{}, pkgerrors.ErrInvalidData
	}

	return t, nil
}

func (r *memoryTaskRepo) Update(ctx context.Context, t task.Task) error {
	return r.storage.Update(ctx, t.ID, t)
}

func (r *memoryTaskRepo) List(ctx context.Context, offset, limit uint64) ([]task.Task, uint64, error) {
	data, total, err := r.storage.List(ctx, offset, limit)
	if err != nil {
		return nil, 0, err
	}
	tasks := make([]task.Task, len(data))
	for i, d := range data {
		t, ok := d.(task.Task)
		if !ok {
			return nil, 0, pkgerrors.ErrInvalidData
		}
		tasks[i] = t
	}

	return tasks, total, nil
}

func (r *memoryTaskRepo) ListByWorkflowID(ctx context.Context, workflowID string, offset, limit uint64) ([]task.Task, uint64, error) {
	const pageSize = 1024

	var (
		scanOffset uint64
		total      uint64
		filtered   []task.Task
	)

	for {
		data, allTotal, err := r.storage.List(ctx, scanOffset, pageSize)
		if err != nil {
			return nil, 0, err
		}
		if len(data) == 0 {
			break
		}

		for _, d := range data {
			t, ok := d.(task.Task)
			if !ok {
				return nil, 0, pkgerrors.ErrInvalidData
			}
			if t.WorkflowID != workflowID {
				continue
			}

			if total >= offset && uint64(len(filtered)) < limit {
				filtered = append(filtered, t)
			}
			total++
		}

		scanOffset += uint64(len(data))
		if scanOffset >= allTotal {
			break
		}
	}

	if offset >= total {
		return []task.Task{}, total, nil
	}

	return filtered, total, nil
}

func (r *memoryTaskRepo) Delete(ctx context.Context, id string) error {
	return r.storage.Delete(ctx, id)
}

type memoryPropletRepo struct {
	storage Storage
}

func newMemoryPropletRepository(s Storage) PropletRepository {
	return &memoryPropletRepo{storage: s}
}

func (r *memoryPropletRepo) Create(ctx context.Context, p proplet.Proplet) error {
	return r.storage.Create(ctx, p.ID, p)
}

func (r *memoryPropletRepo) Get(ctx context.Context, id string) (proplet.Proplet, error) {
	data, err := r.storage.Get(ctx, id)
	if err != nil {
		return proplet.Proplet{}, err
	}
	p, ok := data.(proplet.Proplet)
	if !ok {
		return proplet.Proplet{}, pkgerrors.ErrInvalidData
	}

	return p, nil
}

func (r *memoryPropletRepo) Update(ctx context.Context, p proplet.Proplet) error {
	return r.storage.Update(ctx, p.ID, p)
}

func (r *memoryPropletRepo) List(ctx context.Context, offset, limit uint64) ([]proplet.Proplet, uint64, error) {
	data, total, err := r.storage.List(ctx, offset, limit)
	if err != nil {
		return nil, 0, err
	}
	proplets := make([]proplet.Proplet, len(data))
	for i, d := range data {
		p, ok := d.(proplet.Proplet)
		if !ok {
			return nil, 0, pkgerrors.ErrInvalidData
		}
		proplets[i] = p
	}

	return proplets, total, nil
}

func (r *memoryPropletRepo) Delete(ctx context.Context, id string) error {
	return r.storage.Delete(ctx, id)
}

type memoryTaskPropletRepo struct {
	storage Storage
}

func newMemoryTaskPropletRepository(s Storage) TaskPropletRepository {
	return &memoryTaskPropletRepo{storage: s}
}

func (r *memoryTaskPropletRepo) Create(ctx context.Context, taskID, propletID string) error {
	return r.storage.Create(ctx, taskID, propletID)
}

func (r *memoryTaskPropletRepo) Get(ctx context.Context, taskID string) (string, error) {
	data, err := r.storage.Get(ctx, taskID)
	if err != nil {
		return "", err
	}
	propletID, ok := data.(string)
	if !ok {
		return "", pkgerrors.ErrInvalidData
	}

	return propletID, nil
}

func (r *memoryTaskPropletRepo) Delete(ctx context.Context, taskID string) error {
	return r.storage.Delete(ctx, taskID)
}

type memoryMetricsRepo struct {
	storage Storage
}

func newMemoryMetricsRepository(s Storage) MetricsRepository {
	return &memoryMetricsRepo{storage: s}
}

func (r *memoryMetricsRepo) CreateTaskMetrics(ctx context.Context, m TaskMetrics) error {
	key := fmt.Sprintf("%s:%d", m.TaskID, m.Timestamp.UnixNano())

	return r.storage.Create(ctx, key, m)
}

func (r *memoryMetricsRepo) CreatePropletMetrics(ctx context.Context, m PropletMetrics) error {
	key := fmt.Sprintf("%s:%d", m.PropletID, m.Timestamp.UnixNano())

	return r.storage.Create(ctx, key, m)
}

func (r *memoryMetricsRepo) ListTaskMetrics(ctx context.Context, taskID string, offset, limit uint64) ([]TaskMetrics, uint64, error) {
	const pageSize = 1024

	var (
		scanOffset uint64
		total      uint64
		filtered   []TaskMetrics
	)

	for {
		data, allTotal, err := r.storage.List(ctx, scanOffset, pageSize)
		if err != nil {
			return nil, 0, err
		}
		if len(data) == 0 {
			break
		}

		for _, d := range data {
			m, ok := d.(TaskMetrics)
			if !ok {
				continue
			}
			if m.TaskID != taskID {
				continue
			}

			if total >= offset && uint64(len(filtered)) < limit {
				filtered = append(filtered, m)
			}
			total++
		}

		scanOffset += uint64(len(data))
		if scanOffset >= allTotal {
			break
		}
	}

	if offset >= total {
		return []TaskMetrics{}, total, nil
	}

	return filtered, total, nil
}

func (r *memoryMetricsRepo) ListPropletMetrics(ctx context.Context, propletID string, offset, limit uint64) ([]PropletMetrics, uint64, error) {
	const pageSize = 1024

	var (
		scanOffset uint64
		total      uint64
		filtered   []PropletMetrics
	)

	for {
		data, allTotal, err := r.storage.List(ctx, scanOffset, pageSize)
		if err != nil {
			return nil, 0, err
		}
		if len(data) == 0 {
			break
		}

		for _, d := range data {
			m, ok := d.(PropletMetrics)
			if !ok {
				continue
			}
			if m.PropletID != propletID {
				continue
			}

			if total >= offset && uint64(len(filtered)) < limit {
				filtered = append(filtered, m)
			}
			total++
		}

		scanOffset += uint64(len(data))
		if scanOffset >= allTotal {
			break
		}
	}

	if offset >= total {
		return []PropletMetrics{}, total, nil
	}

	return filtered, total, nil
}
