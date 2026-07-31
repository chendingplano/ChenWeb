package docprocessing

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/lib/pq"
)

// PipelineRegistryStore reads the authored named-pipeline registry.
type PipelineRegistryStore interface {
	ListPipelines(ctx context.Context) ([]ProductionPipelineSpec, error)
}

type PipelineRegistrySQLStore struct {
	DB *sql.DB
}

func (s PipelineRegistrySQLStore) ListPipelines(ctx context.Context) ([]ProductionPipelineSpec, error) {
	if s.DB == nil {
		return nil, errors.New("db is nil")
	}
	const stmt = `
SELECT name,
       COALESCE(display_name, ''),
       processors,
       legacy_equivalent
FROM kb.pipelines
ORDER BY id`
	rows, err := s.DB.QueryContext(ctx, stmt)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	specs := make([]ProductionPipelineSpec, 0)
	for rows.Next() {
		var (
			spec       ProductionPipelineSpec
			processors pq.StringArray
		)
		if err := rows.Scan(&spec.Name, &spec.DisplayName, &processors, &spec.LegacyEquivalent); err != nil {
			return nil, err
		}
		spec.Processors = []string(processors)
		specs = append(specs, spec)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return specs, nil
}

// LoadProductionPipelineRegistry loads the authored pipeline registry from
// store and installs it via SetProductionPipelineRegistry. On error, or if
// the table is empty, the previously installed registry (or the legacy-
// equivalent fallback) is left in place — callers should log the error but
// treat it as non-fatal, matching P1's "no active policy means legacy
// behavior" invariant: a missing/unreachable authored registry must never
// leave the runtime without a resolvable default pipeline.
func LoadProductionPipelineRegistry(ctx context.Context, store PipelineRegistryStore) error {
	if store == nil {
		return errors.New("pipeline registry store is nil")
	}
	specs, err := store.ListPipelines(ctx)
	if err != nil {
		return fmt.Errorf("list pipelines: %w", err)
	}
	if len(specs) == 0 {
		return errors.New("authored pipeline registry is empty")
	}
	SetProductionPipelineRegistry(specs)
	return nil
}
