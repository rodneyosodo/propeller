package badger

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/absmach/propeller/pkg/proplet"
)

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
		return fmt.Errorf("marshal error: %w", err)
	}
	if err := r.db.set(key, val); err != nil {
		return fmt.Errorf("%w: %w", ErrCreate, err)
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
		return proplet.Proplet{}, fmt.Errorf("unmarshal error: %w", err)
	}

	return p, nil
}

func (r *propletRepo) Update(ctx context.Context, p proplet.Proplet) error {
	key := []byte("proplet:" + p.ID)
	val, err := json.Marshal(p)
	if err != nil {
		return fmt.Errorf("marshal error: %w", err)
	}
	if err := r.db.set(key, val); err != nil {
		return fmt.Errorf("%w: %w", ErrUpdate, err)
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
			return nil, 0, fmt.Errorf("unmarshal error: %w", err)
		}
		proplets[i] = p
	}

	return proplets, total, nil
}

func (r *propletRepo) Delete(ctx context.Context, id string) error {
	key := []byte("proplet:" + id)

	return r.db.delete(key)
}

func (r *propletRepo) GetAliveHistory(ctx context.Context, id string, offset, limit uint64) ([]time.Time, uint64, error) {
	p, err := r.Get(ctx, id)
	if err != nil {
		return nil, 0, err
	}

	total := uint64(len(p.AliveHistory))
	if offset >= total {
		return []time.Time{}, total, nil
	}

	end := offset + limit
	if end > total {
		end = total
	}

	return p.AliveHistory[offset:end], total, nil
}
