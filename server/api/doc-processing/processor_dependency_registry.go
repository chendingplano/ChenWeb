package docprocessing

import (
	"context"
	"database/sql"
	"errors"

	"github.com/lib/pq"
)

// ProcessorRegistryStore reads kb.processor_registry (ADR 2026081001 DR7):
// processor dependency metadata for processors added after
// productionProcessorSpecs was written. The table is expected to be empty
// for a long time -- incremental adoption means existing processors never
// move into it -- so an empty result is not an error.
type ProcessorRegistryStore interface {
	ListProcessorRegistry(ctx context.Context) ([]ProcessorSpec, error)
}

type ProcessorRegistrySQLStore struct {
	DB *sql.DB
}

func (s ProcessorRegistrySQLStore) ListProcessorRegistry(ctx context.Context) ([]ProcessorSpec, error) {
	if s.DB == nil {
		return nil, errors.New("db is nil")
	}
	const stmt = `
SELECT name, phase, class, cost, on_undetermined, idempotent, requires, produces
FROM kb.processor_registry
ORDER BY name`
	rows, err := s.DB.QueryContext(ctx, stmt)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	specs := make([]ProcessorSpec, 0)
	for rows.Next() {
		var (
			spec     ProcessorSpec
			requires pq.StringArray
			produces pq.StringArray
		)
		if err := rows.Scan(&spec.Name, &spec.Phase, &spec.Class, &spec.Cost, &spec.OnUndetermined, &spec.Idempotent, &requires, &produces); err != nil {
			return nil, err
		}
		spec.Requires = []string(requires)
		spec.Produces = []string(produces)
		specs = append(specs, spec)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return specs, nil
}

// LoadProcessorRegistry loads kb.processor_registry into the existing
// seam-1 processor registry (registries.go's RegisterProcessor/LookupProcessor,
// already seeded from productionProcessorSpecs at init time). Only names not
// already known are registered, so the hardcoded productionProcessorSpecs
// entries always win -- the union DR7 describes, not a replacement.
func LoadProcessorRegistry(ctx context.Context, store ProcessorRegistryStore) error {
	if store == nil {
		return errors.New("processor registry store is nil")
	}
	specs, err := store.ListProcessorRegistry(ctx)
	if err != nil {
		return err
	}
	for _, spec := range specs {
		if _, ok := LookupProcessor(spec.Name); ok {
			continue
		}
		if err := RegisterProcessor(spec); err != nil {
			return err
		}
	}
	return nil
}
