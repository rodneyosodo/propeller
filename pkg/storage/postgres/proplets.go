package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/absmach/propeller/pkg/proplet"
)

type propletRepo struct {
	db *Database
}

func NewPropletRepository(db *Database) PropletRepository {
	return &propletRepo{db: db}
}

type dbProplet struct {
	ID               string `db:"id"`
	Name             string `db:"name"`
	TaskCount        uint64 `db:"task_count"`
	Alive            bool   `db:"alive"`
	AliveHistory     []byte `db:"alive_history"`
	Description      string `db:"description"`
	Tags             []byte `db:"tags"`
	Location         string `db:"location"`
	IP               string `db:"ip"`
	Environment      string `db:"environment"`
	OS               string `db:"os"`
	Hostname         string `db:"hostname"`
	CPUArch          string `db:"cpu_arch"`
	TotalMemoryBytes uint64 `db:"total_memory_bytes"`
	PropletVersion   string `db:"proplet_version"`
	WasmRuntime      string `db:"wasm_runtime"`
}

func (r *propletRepo) Create(ctx context.Context, p proplet.Proplet) error {
	query := `INSERT INTO proplets (id, name, task_count, alive, alive_history, description, tags, location, ip, environment, os, hostname, cpu_arch, total_memory_bytes, proplet_version, wasm_runtime) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)`

	aliveHistory, err := jsonBytes(p.AliveHistory)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrDBQuery, err)
	}

	tags, err := jsonBytes(p.Metadata.Tags)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrDBQuery, err)
	}

	if _, err = r.db.ExecContext(ctx, query, p.ID, p.Name, p.TaskCount, p.Alive, aliveHistory, p.Metadata.Description, tags, p.Metadata.Location, p.Metadata.IP, p.Metadata.Environment, p.Metadata.OS, p.Metadata.Hostname, p.Metadata.CPUArch, p.Metadata.TotalMemoryBytes, p.Metadata.PropletVersion, p.Metadata.WasmRuntime); err != nil {
		return fmt.Errorf("%w: %w", ErrCreate, err)
	}

	return nil
}

func (r *propletRepo) Get(ctx context.Context, id string) (proplet.Proplet, error) {
	query := `SELECT id, name, task_count, alive, alive_history, description, tags, location, ip, environment, os, hostname, cpu_arch, total_memory_bytes, proplet_version, wasm_runtime FROM proplets WHERE id = $1`

	var dbp dbProplet

	if err := r.db.GetContext(ctx, &dbp, query, id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return proplet.Proplet{}, ErrPropletNotFound
		}

		return proplet.Proplet{}, fmt.Errorf("%w: %w", ErrDBQuery, err)
	}

	return r.toProplet(dbp)
}

func (r *propletRepo) Update(ctx context.Context, p proplet.Proplet) error {
	query := `UPDATE proplets SET name = $2, task_count = $3, alive = $4, alive_history = $5, description = $6, tags = $7, location = $8, ip = $9, environment = $10, os = $11, hostname = $12, cpu_arch = $13, total_memory_bytes = $14, proplet_version = $15, wasm_runtime = $16 WHERE id = $1`

	aliveHistory, err := jsonBytes(p.AliveHistory)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrDBQuery, err)
	}

	tags, err := jsonBytes(p.Metadata.Tags)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrDBQuery, err)
	}

	if _, err = r.db.ExecContext(ctx, query, p.ID, p.Name, p.TaskCount, p.Alive, aliveHistory, p.Metadata.Description, tags, p.Metadata.Location, p.Metadata.IP, p.Metadata.Environment, p.Metadata.OS, p.Metadata.Hostname, p.Metadata.CPUArch, p.Metadata.TotalMemoryBytes, p.Metadata.PropletVersion, p.Metadata.WasmRuntime); err != nil {
		return fmt.Errorf("%w: %w", ErrUpdate, err)
	}

	return nil
}

func (r *propletRepo) List(ctx context.Context, offset, limit uint64) ([]proplet.Proplet, uint64, error) {
	var total uint64
	err := r.db.GetContext(ctx, &total, "SELECT COUNT(*) FROM proplets")
	if err != nil {
		return nil, 0, fmt.Errorf("%w: %w", ErrDBQuery, err)
	}

	query := `SELECT id, name, task_count, alive, alive_history, description, tags, location, ip, environment, os, hostname, cpu_arch, total_memory_bytes, proplet_version, wasm_runtime FROM proplets LIMIT $1 OFFSET $2`

	rows, err := r.db.QueryContext(ctx, query, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("%w: %w", ErrDBQuery, err)
	}
	defer rows.Close()

	proplets := make([]proplet.Proplet, 0)
	for rows.Next() {
		var dbp dbProplet
		if err := rows.Scan(&dbp.ID, &dbp.Name, &dbp.TaskCount, &dbp.Alive, &dbp.AliveHistory, &dbp.Description, &dbp.Tags, &dbp.Location, &dbp.IP, &dbp.Environment, &dbp.OS, &dbp.Hostname, &dbp.CPUArch, &dbp.TotalMemoryBytes, &dbp.PropletVersion, &dbp.WasmRuntime); err != nil {
			return nil, 0, fmt.Errorf("%w: %w", ErrDBScan, err)
		}

		p, err := r.toProplet(dbp)
		if err != nil {
			return nil, 0, fmt.Errorf("%w: %w", ErrDBScan, err)
		}

		proplets = append(proplets, p)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("%w: %w", ErrDBQuery, err)
	}

	return proplets, total, nil
}

func (r *propletRepo) Delete(ctx context.Context, id string) error {
	query := `DELETE FROM proplets WHERE id = $1`

	if _, err := r.db.ExecContext(ctx, query, id); err != nil {
		return fmt.Errorf("%w: %w", ErrDelete, err)
	}

	return nil
}

func (r *propletRepo) toProplet(dbp dbProplet) (proplet.Proplet, error) {
	p := proplet.Proplet{
		ID:        dbp.ID,
		Name:      dbp.Name,
		TaskCount: dbp.TaskCount,
		Alive:     dbp.Alive,
		Metadata: proplet.PropletMetadata{
			Description:      dbp.Description,
			Location:         dbp.Location,
			IP:               dbp.IP,
			Environment:      dbp.Environment,
			OS:               dbp.OS,
			Hostname:         dbp.Hostname,
			CPUArch:          dbp.CPUArch,
			TotalMemoryBytes: dbp.TotalMemoryBytes,
			PropletVersion:   dbp.PropletVersion,
			WasmRuntime:      dbp.WasmRuntime,
		},
	}

	if dbp.AliveHistory != nil {
		if err := jsonUnmarshal(dbp.AliveHistory, &p.AliveHistory); err != nil {
			return proplet.Proplet{}, err
		}
	}

	if dbp.Tags != nil {
		if err := jsonUnmarshal(dbp.Tags, &p.Metadata.Tags); err != nil {
			return proplet.Proplet{}, err
		}
	}

	return p, nil
}
