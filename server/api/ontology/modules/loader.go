package modules

import (
	"context"
	"sync"
)

// ActiveModule is the in-process view of a module's active release. Payloads
// are fetched on demand from the release store rather than held in memory, so
// the registry stays small even for the large QUDT quantity module.
type ActiveModule struct {
	ModuleID  string `json:"module_id"`
	ReleaseID int64  `json:"release_id"`
	Version   string `json:"version"`
	Checksum  string `json:"content_checksum"`
}

var (
	activeModulesMu   sync.RWMutex
	activeModules     = map[string]ActiveModule{}
)

// SetActiveModules installs the active-module registry. Used by the loader
// and by tests; guarded so callers cannot race the loader.
func SetActiveModules(m map[string]ActiveModule) {
	activeModulesMu.Lock()
	defer activeModulesMu.Unlock()
	activeModules = m
}

// GetActiveModule returns the active release for a module_id.
func GetActiveModule(moduleID string) (ActiveModule, bool) {
	activeModulesMu.RLock()
	defer activeModulesMu.RUnlock()
	m, ok := activeModules[moduleID]
	return m, ok
}

// ActiveModuleIDs returns the module_ids with an active release.
func ActiveModuleIDs() []string {
	activeModulesMu.RLock()
	defer activeModulesMu.RUnlock()
	out := make([]string, 0, len(activeModules))
	for id := range activeModules {
		out = append(out, id)
	}
	return out
}

// LoadActiveModuleReleases loads all currently active releases into the
// in-process registry. This is extension seam 4's loader: the module compiler
// + loader, called best-effort at process startup (the same warn-don't-block
// pattern the pipeline registries use). It returns the count loaded.
func (s ReleaseStore) LoadActiveModuleReleases(ctx context.Context) (int, error) {
	if s.DB == nil {
		return 0, nil
	}
	const stmt = `
SELECT ar.module_id, ar.release_id, r.version, r.content_checksum
FROM kb.ontology_active_releases ar
JOIN kb.ontology_module_releases r ON r.id = ar.release_id
WHERE ar.deactivated_at IS NULL`
	rows, err := s.DB.QueryContext(ctx, stmt)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	loaded := make(map[string]ActiveModule)
	for rows.Next() {
		var m ActiveModule
		if err := rows.Scan(&m.ModuleID, &m.ReleaseID, &m.Version, &m.Checksum); err != nil {
			return 0, err
		}
		loaded[m.ModuleID] = m
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	SetActiveModules(loaded)
	return len(loaded), nil
}
