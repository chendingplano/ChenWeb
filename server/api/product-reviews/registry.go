package productreviews

import (
	"context"
	"database/sql"
	"encoding/json"
	"sort"
	"strings"
)

// Artifact is one row an adapter projects from its backing table, keyed the way
// kb.search_artifacts keys it: (ArtifactType, ArtifactID). It carries what
// retrieval needs to tier, score, and cite the hit.
type Artifact struct {
	ArtifactType   string
	ArtifactID     string
	SourceRowID    int64
	InputRecordID  int64
	PrimaryLabel   string          // e.g. the metric name
	SubjectText    string          // the artifact's subject, for normalized-name matching
	SubjectConcept string          // subject concept id (may be empty)
	LineSpans      json.RawMessage // source_line_spans, for citation
}

// ArtifactAdapter is the per-type indirection that keeps `metric` from being
// hard-wired (spec: Registered artifact types). Adding `provision` later is one
// adapter, no schema or read-path change.
type ArtifactAdapter interface {
	// Type is the registered artifact type, e.g. "metric".
	Type() string
	// Partition is the kb.search_artifacts partition to fuse for hybrid matching.
	Partition() string
	// ListByDocuments returns every artifact of this type belonging to any of
	// the given input records (Path D — document-first).
	ListByDocuments(ctx context.Context, db *sql.DB, recordIDs []int64) ([]Artifact, error)
	// MatchBySubjectConcept returns artifacts whose subject concept id is one of
	// conceptIDs (Path E — direct, concept equality half).
	MatchBySubjectConcept(ctx context.Context, db *sql.DB, conceptIDs []string) ([]Artifact, error)
	// Project resolves a set of artifact ids (e.g. from a hybrid-search hit
	// list) back into full result rows.
	Project(ctx context.Context, db *sql.DB, artifactIDs []string) ([]Artifact, error)
}

// registry holds the registered adapters. v1 registers `metric` only.
var registry = map[string]ArtifactAdapter{}

// Register adds an adapter. Called from init() and from tests.
func Register(a ArtifactAdapter) { registry[a.Type()] = a }

// AdapterFor returns the adapter for a registered type, or CWB_KB_PMR_020.
func AdapterFor(artifactType string) (ArtifactAdapter, error) {
	if a, ok := registry[strings.TrimSpace(artifactType)]; ok {
		return a, nil
	}
	return nil, ErrUnregisteredType(artifactType)
}

// ValidateArtifactTypes rejects the request (before any run is created) if any
// requested type is unregistered.
func ValidateArtifactTypes(types []string) error {
	for _, t := range types {
		if _, err := AdapterFor(t); err != nil {
			return err
		}
	}
	return nil
}

// RegisteredTypes lists the registered artifact types, sorted.
func RegisteredTypes() []string {
	out := make([]string, 0, len(registry))
	for k := range registry {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func init() { Register(metricAdapter{}) }
