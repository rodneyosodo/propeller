package badger

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/absmach/propeller/pkg/proplet"
	"github.com/absmach/propeller/task"
)

type taskRepo struct {
	db *Database
}

func NewTaskRepository(db *Database) TaskRepository {
	return &taskRepo{db: db}
}

func (r *taskRepo) Create(ctx context.Context, t task.Task) (task.Task, error) {
	key := []byte("task:" + t.ID)
	val, err := json.Marshal(t)
	if err != nil {
		return task.Task{}, fmt.Errorf("marshal error: %v", err)
	}
	if err := r.db.set(key, val); err != nil {
		return task.Task{}, fmt.Errorf("%w: %v", ErrCreate, err)
	}
	return t, nil
}

func (r *taskRepo) Get(ctx context.Context, id string) (task.Task, error) {
	key := []byte("task:" + id)
	val, err := r.db.get(key)
	if err != nil {
		return task.Task{}, ErrTaskNotFound
	}
	var t task.Task
	if err := json.Unmarshal(val, &t); err != nil {
		return task.Task{}, fmt.Errorf("unmarshal error: %v", err)
	}
	return t, nil
}

func (r *taskRepo) Update(ctx context.Context, t task.Task) error {
	key := []byte("task:" + t.ID)
	val, err := json.Marshal(t)
	if err != nil {
		return fmt.Errorf("marshal error: %v", err)
	}
	if err := r.db.set(key, val); err != nil {
		return fmt.Errorf("%w: %v", ErrUpdate, err)
	}
	return nil
}

func (r *taskRepo) List(ctx context.Context, offset, limit uint64) ([]task.Task, uint64, error) {
	prefix := []byte("task:")
	total, err := r.db.countWithPrefix(prefix)
	if err != nil {
		return nil, 0, err
	}
	values, err := r.db.listWithPrefix(prefix, offset, limit)
	if err != nil {
		return nil, 0, err
	}
	tasks := make([]task.Task, len(values))
	for i, val := range values {
		var t task.Task
		if err := json.Unmarshal(val, &t); err != nil {
			return nil, 0, fmt.Errorf("unmarshal error: %v", err)
		}
		tasks[i] = t
	}
	return tasks, total, nil
}

func (r *taskRepo) Delete(ctx context.Context, id string) error {
	key := []byte("task:" + id)
	return r.db.delete(key)
}

type propletRepo struct {
	db *Database
}

func NewPropletRepository(db *Database) PropletRepository {
	return &propletRepo{db: db}
}

func (r *propletRepo) Create(ctx context.Context, p proplet.Proplet) error {
	key := []byte("proplet:" + p.ID)
	val, err := json.Marshal(p)
	if err != nil {
		return fmt.Errorf("marshal error: %v", err)
	}
	if err := r.db.set(key, val); err != nil {
		return fmt.Errorf("%w: %v", ErrCreate, err)
	}
	return nil
}

func (r *propletRepo) Get(ctx context.Context, id string) (proplet.Proplet, error) {
	key := []byte("proplet:" + id)
	val, err := r.db.get(key)
	if err != nil {
		return proplet.Proplet{}, ErrPropletNotFound
	}
	var p proplet.Proplet
	if err := json.Unmarshal(val, &p); err != nil {
		return proplet.Proplet{}, fmt.Errorf("unmarshal error: %v", err)
	}
	return p, nil
}

func (r *propletRepo) Update(ctx context.Context, p proplet.Proplet) error {
	key := []byte("proplet:" + p.ID)
	val, err := json.Marshal(p)
	if err != nil {
		return fmt.Errorf("marshal error: %v", err)
	}
	if err := r.db.set(key, val); err != nil {
		return fmt.Errorf("%w: %v", ErrUpdate, err)
	}
	return nil
}

func (r *propletRepo) List(ctx context.Context, offset, limit uint64) ([]proplet.Proplet, uint64, error) {
	prefix := []byte("proplet:")
	total, err := r.db.countWithPrefix(prefix)
	if err != nil {
		return nil, 0, err
	}
	values, err := r.db.listWithPrefix(prefix, offset, limit)
	if err != nil {
		return nil, 0, err
	}
	proplets := make([]proplet.Proplet, len(values))
	for i, val := range values {
		var p proplet.Proplet
		if err := json.Unmarshal(val, &p); err != nil {
			return nil, 0, fmt.Errorf("unmarshal error: %v", err)
		}
		proplets[i] = p
	}
	return proplets, total, nil
}

type taskPropletRepo struct {
	db *Database
}

func NewTaskPropletRepository(db *Database) TaskPropletRepository {
	return &taskPropletRepo{db: db}
}

func (r *taskPropletRepo) Create(ctx context.Context, taskID, propletID string) error {
	key := []byte("map:task:" + taskID)
	if err := r.db.set(key, []byte(propletID)); err != nil {
		return fmt.Errorf("%w: %v", ErrCreate, err)
	}
	return nil
}

func (r *taskPropletRepo) Get(ctx context.Context, taskID string) (string, error) {
	key := []byte("map:task:" + taskID)
	val, err := r.db.get(key)
	if err != nil {
		return "", ErrNotFound
	}
	return string(val), nil
}

func (r *taskPropletRepo) Delete(ctx context.Context, taskID string) error {
	key := []byte("map:task:" + taskID)
	return r.db.delete(key)
}

type metricsRepo struct {
	db *Database
}

func NewMetricsRepository(db *Database) MetricsRepository {
	return &metricsRepo{db: db}
}

func (r *metricsRepo) CreateTaskMetrics(ctx context.Context, m TaskMetrics) error {
	key := []byte(fmt.Sprintf("tm:%s:%d", m.TaskID, m.Timestamp.UnixNano()))
	val, err := json.Marshal(m)
	if err != nil {
		return fmt.Errorf("marshal error: %v", err)
	}
	if err := r.db.set(key, val); err != nil {
		return fmt.Errorf("%w: %v", ErrCreate, err)
	}
	return nil
}

func (r *metricsRepo) CreatePropletMetrics(ctx context.Context, m PropletMetrics) error {
	key := []byte(fmt.Sprintf("pm:%s:%d", m.PropletID, m.Timestamp.UnixNano()))
	val, err := json.Marshal(m)
	if err != nil {
		return fmt.Errorf("marshal error: %v", err)
	}
	if err := r.db.set(key, val); err != nil {
		return fmt.Errorf("%w: %v", ErrCreate, err)
	}
	return nil
}

func (r *metricsRepo) ListTaskMetrics(ctx context.Context, taskID string, offset, limit uint64) ([]TaskMetrics, uint64, error) {
	prefix := []byte("tm:" + taskID)
	total, err := r.db.countWithPrefix(prefix)
	if err != nil {
		return nil, 0, err
	}
	values, err := r.db.listWithPrefix(prefix, offset, limit)
	if err != nil {
		return nil, 0, err
	}
	metrics := make([]TaskMetrics, len(values))
	for i, val := range values {
		var m TaskMetrics
		if err := json.Unmarshal(val, &m); err != nil {
			return nil, 0, fmt.Errorf("unmarshal error: %v", err)
		}
		metrics[i] = m
	}
	return metrics, total, nil
}

func (r *metricsRepo) ListPropletMetrics(ctx context.Context, propletID string, offset, limit uint64) ([]PropletMetrics, uint64, error) {
	prefix := []byte("pm:" + propletID)
	total, err := r.db.countWithPrefix(prefix)
	if err != nil {
		return nil, 0, err
	}
	values, err := r.db.listWithPrefix(prefix, offset, limit)
	if err != nil {
		return nil, 0, err
	}
	metrics := make([]PropletMetrics, len(values))
	for i, val := range values {
		var m PropletMetrics
		if err := json.Unmarshal(val, &m); err != nil {
			return nil, 0, fmt.Errorf("unmarshal error: %v", err)
		}
		metrics[i] = m
	}
	return metrics, total, nil
}
