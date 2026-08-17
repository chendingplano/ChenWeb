package semantic

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"sync"
)

// WriterMode is the per-family writer state recorded in
// kb.semantic_adapter_compliance.
type WriterMode string

const (
	// WriterLegacy: the pre-lossless writer is in charge.
	WriterLegacy WriterMode = "legacy"
	// WriterShadow: the adapter computes intended writes and compares them
	// with the existing path, persisting nothing consumer-visible (Phase 1).
	WriterShadow WriterMode = "shadow"
	// WriterFallback: the family uses the generic unresolved-occurrence path.
	WriterFallback WriterMode = "fallback"
	// WriterLossless: the family's lossless writer is active.
	WriterLossless WriterMode = "lossless"
)

// ConformanceSuiteVersion is the version of the shared conformance suite in
// this build. DR13 refuses activation when a registered adapter has not passed
// the CURRENT suite, so bumping this constant automatically de-certifies every
// adapter until it is re-verified -- which is the point: a suite that gained a
// new required check must not be satisfied by an old passing result.
const ConformanceSuiteVersion = "1.0.0"

// StageContract is what an adapter declares about one required semantic stage
// (DR13). DecisionScopes is the list of scope identifiers the stage may key
// findings within; it drives finding-key determinism and the conformance test
// that a stage never reports a finding outside its declared scopes.
type StageContract struct {
	StageTermID         string
	DecisionScopes      []string
	Dimensions          []string
	AllowedDispositions []string
}

// FamilyAdapter is DR13's per-family contract. Shared infrastructure owns
// persistence, vocabulary, idempotency, retry, logging, and query shapes;
// artifact semantics stay here.
type FamilyAdapter interface {
	// ArtifactType is the kb artifact_type this adapter owns ("metric", ...).
	ArtifactType() string
	// AdapterName and AdapterVersion identify the adapter in the compliance
	// registry.
	AdapterName() string
	AdapterVersion() string
	// RequiredStages enumerates the stages every current occurrence of this
	// family must have an outcome envelope for. The completeness projection
	// reports against this declaration, so an undeclared stage's absence is
	// not a completeness failure.
	RequiredStages() []StageContract
	// RawIdentityFields names the artifact-family columns that constitute the
	// raw occurrence identity and raw payload (DR2/DR13).
	RawIdentityFields() []string
	// SupportsInstances reports whether this family is modelled as ontology
	// object instances. DR1: such families MUST produce a raw-preserved
	// instance and may not fall back to an unresolved occurrence.
	SupportsInstances() bool
	// ValueStates and ConformanceStates are the governed states this family
	// uses; conformance checks that it uses no ungoverned value.
	ValueStates() []string
	ConformanceStates() []string
	// DependencyAxes names the dependency axes this family's fingerprints
	// cover, so retry triggers can be validated against them.
	DependencyAxes() []string
	// OccurrenceScope is the family-declared source scope that participates in
	// OccurrenceKey.
	OccurrenceScope() string
}

var (
	adapterMu sync.RWMutex
	adapters  = map[string]FamilyAdapter{}
)

// RegisterAdapter registers a family adapter. Registering twice for one
// artifact type is a programming error rather than a silent overwrite: two
// adapters disagreeing about required stages would make the completeness
// projection non-deterministic.
func RegisterAdapter(a FamilyAdapter) {
	adapterMu.Lock()
	defer adapterMu.Unlock()
	if _, exists := adapters[a.ArtifactType()]; exists {
		panic(fmt.Sprintf("semantic: adapter for artifact type %q is already registered", a.ArtifactType()))
	}
	adapters[a.ArtifactType()] = a
}

// LookupAdapter returns the registered adapter for an artifact type.
func LookupAdapter(artifactType string) (FamilyAdapter, bool) {
	adapterMu.RLock()
	defer adapterMu.RUnlock()
	a, ok := adapters[artifactType]
	return a, ok
}

// RegisteredArtifactTypes returns every registered artifact type, sorted.
func RegisteredArtifactTypes() []string {
	adapterMu.RLock()
	defer adapterMu.RUnlock()
	out := make([]string, 0, len(adapters))
	for k := range adapters {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// resetAdaptersForTest clears the registry. Tests that register adapters use
// it so registration order between tests cannot matter.
func resetAdaptersForTest() {
	adapterMu.Lock()
	defer adapterMu.Unlock()
	adapters = map[string]FamilyAdapter{}
}

// ComplianceRecord is one row of kb.semantic_adapter_compliance (DR13's
// runtime compliance registry).
type ComplianceRecord struct {
	ArtifactType            string
	AdapterName             string
	AdapterVersion          string
	WriterMode              WriterMode
	ConformanceSuiteVersion string
	LastVerifiedResult      string
	Details                 []byte
}

// ComplianceRegistry reads and writes the runtime compliance registry.
type ComplianceRegistry struct {
	DB *sql.DB
}

// Upsert records an adapter's current compliance state.
func (r ComplianceRegistry) Upsert(ctx context.Context, rec ComplianceRecord) error {
	_, err := r.DB.ExecContext(ctx, `
INSERT INTO kb.semantic_adapter_compliance (
  artifact_type, adapter_name, adapter_version, writer_mode,
  conformance_suite_version, last_verified_result, last_verified_time, last_verified_details, create_by)
VALUES ($1,$2,$3,$4,$5,$6,NOW(),$7,'semantic_conformance')
ON CONFLICT (artifact_type) DO UPDATE SET
  adapter_name = EXCLUDED.adapter_name,
  adapter_version = EXCLUDED.adapter_version,
  writer_mode = EXCLUDED.writer_mode,
  conformance_suite_version = EXCLUDED.conformance_suite_version,
  last_verified_result = EXCLUDED.last_verified_result,
  last_verified_time = NOW(),
  last_verified_details = EXCLUDED.last_verified_details,
  modify_time = NOW(),
  modify_by = 'semantic_conformance'`,
		rec.ArtifactType, rec.AdapterName, rec.AdapterVersion, string(rec.WriterMode),
		nullIfEmpty(rec.ConformanceSuiteVersion), nullIfEmpty(rec.LastVerifiedResult), rec.Details)
	return err
}

// Get returns the compliance record for an artifact type, or sql.ErrNoRows.
func (r ComplianceRegistry) Get(ctx context.Context, artifactType string) (ComplianceRecord, error) {
	var rec ComplianceRecord
	var mode string
	var suite, result sql.NullString
	err := r.DB.QueryRowContext(ctx, `
SELECT artifact_type, adapter_name, adapter_version, writer_mode,
       conformance_suite_version, last_verified_result, last_verified_details
FROM kb.semantic_adapter_compliance WHERE artifact_type = $1`, artifactType).
		Scan(&rec.ArtifactType, &rec.AdapterName, &rec.AdapterVersion, &mode,
			&suite, &result, &rec.Details)
	if err != nil {
		return ComplianceRecord{}, err
	}
	rec.WriterMode = WriterMode(mode)
	rec.ConformanceSuiteVersion = suite.String
	rec.LastVerifiedResult = result.String
	return rec, nil
}

// List returns every compliance record, sorted by artifact type.
func (r ComplianceRegistry) List(ctx context.Context) ([]ComplianceRecord, error) {
	rows, err := r.DB.QueryContext(ctx, `
SELECT artifact_type, adapter_name, adapter_version, writer_mode,
       conformance_suite_version, last_verified_result, last_verified_details
FROM kb.semantic_adapter_compliance ORDER BY artifact_type`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ComplianceRecord
	for rows.Next() {
		var rec ComplianceRecord
		var mode string
		var suite, result sql.NullString
		if err := rows.Scan(&rec.ArtifactType, &rec.AdapterName, &rec.AdapterVersion, &mode,
			&suite, &result, &rec.Details); err != nil {
			return nil, err
		}
		rec.WriterMode = WriterMode(mode)
		rec.ConformanceSuiteVersion = suite.String
		rec.LastVerifiedResult = result.String
		out = append(out, rec)
	}
	return out, rows.Err()
}
