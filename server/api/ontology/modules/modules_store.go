// Package modules implements the DB-native ontology module registry and the
// immutable release / activation lifecycle (ADR 2026072901 DR2, DR17,
// storage-revised 2026-07-31): content is authored and versioned in the
// database; the module compiler validates staged content, computes a
// deterministic checksum, and writes immutable releases; activation is a
// separate audited pointer.
package modules

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/lib/pq"
)

// Module is the identity and current metadata of an ontology module. Released
// snapshots and their versions live in kb.ontology_module_releases.
type Module struct {
	ID          int64     `json:"id"`
	ModuleID    string    `json:"module_id"`
	Title       string    `json:"title"`
	Owner       string    `json:"owner"`
	Description string    `json:"description"`
	DependsOn   []string  `json:"depends_on"`
	Status      string    `json:"status"`
	CreateTime  time.Time `json:"create_time"`
	CreateBy    string    `json:"create_by"`
	ModifyTime  time.Time `json:"modify_time"`
	ModifyBy    string    `json:"modify_by"`
}

// ModuleStore persists module identity rows.
type ModuleStore struct {
	DB *sql.DB
}

const moduleColumns = `
	id, module_id, COALESCE(title, ''), COALESCE(owner, ''), COALESCE(description, ''),
	COALESCE(depends_on, '{}'), status,
	create_time, COALESCE(create_by, ''), modify_time, COALESCE(modify_by, '')`

const moduleFrom = "FROM kb.ontology_modules"

func scanModule(scan func(dest ...any) error) (Module, error) {
	var (
		m    Module
		deps pq.StringArray
	)
	err := scan(
		&m.ID, &m.ModuleID, &m.Title, &m.Owner, &m.Description, &deps, &m.Status,
		&m.CreateTime, &m.CreateBy, &m.ModifyTime, &m.ModifyBy,
	)
	if err == nil {
		m.DependsOn = []string(deps)
	}
	return m, err
}

// CreateModule registers a module identity.
func (s ModuleStore) CreateModule(ctx context.Context, m Module) (Module, error) {
	if s.DB == nil {
		return Module{}, errors.New("db is nil")
	}
	if strings.TrimSpace(m.ModuleID) == "" {
		return Module{}, errors.New("module_id is required")
	}
	if m.Status == "" {
		m.Status = "draft"
	}
	if m.DependsOn == nil {
		m.DependsOn = []string{}
	}
	const stmt = `
INSERT INTO kb.ontology_modules (module_id, title, owner, description, depends_on, status, create_by, modify_by)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING ` + moduleColumns
	row := s.DB.QueryRowContext(ctx, stmt,
		strings.TrimSpace(m.ModuleID), nullableString(m.Title), nullableString(m.Owner),
		nullableString(m.Description), pq.Array(m.DependsOn), m.Status,
		nullableString(m.CreateBy), nullableString(m.ModifyBy),
	)
	return scanModule(row.Scan)
}

// GetModule returns a module by module_id.
func (s ModuleStore) GetModule(ctx context.Context, moduleID string) (Module, error) {
	if s.DB == nil {
		return Module{}, errors.New("db is nil")
	}
	const stmt = `SELECT ` + moduleColumns + "\n" + moduleFrom + `
WHERE module_id = $1`
	row := s.DB.QueryRowContext(ctx, stmt, strings.TrimSpace(moduleID))
	return scanModule(row.Scan)
}

// ListModules returns all modules.
func (s ModuleStore) ListModules(ctx context.Context) ([]Module, error) {
	if s.DB == nil {
		return nil, errors.New("db is nil")
	}
	const stmt = `SELECT ` + moduleColumns + "\n" + moduleFrom + `
ORDER BY module_id`
	rows, err := s.DB.QueryContext(ctx, stmt)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	modules := make([]Module, 0)
	for rows.Next() {
		m, err := scanModule(rows.Scan)
		if err != nil {
			return nil, err
		}
		modules = append(modules, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return modules, nil
}

// UpdateModuleDeps replaces a module's declared dependencies.
func (s ModuleStore) UpdateModuleDeps(ctx context.Context, moduleID string, dependsOn []string, by string) (Module, error) {
	if s.DB == nil {
		return Module{}, errors.New("db is nil")
	}
	if dependsOn == nil {
		dependsOn = []string{}
	}
	const stmt = `
UPDATE kb.ontology_modules
SET depends_on = $2, modify_time = NOW(), modify_by = $3
WHERE module_id = $1`
	if _, err := s.DB.ExecContext(ctx, stmt, strings.TrimSpace(moduleID), pq.Array(dependsOn), nullableString(by)); err != nil {
		return Module{}, err
	}
	return s.GetModule(ctx, moduleID)
}
