package assertions

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
)

// NormalizeReport summarizes one normalizer's run over one input record
// (spec §10.9 association-run telemetry feeds from this).
type NormalizeReport struct {
	ArtifactFamily string
	Examined       int
	Proposed       int
	Reused         int
	Skipped        int
}

// Normalizer turns one artifact family's extracted output into candidate
// qualified assertions (DR8's normalize_assertions stage). A normalizer must
// only ever write kb.semantic_decision_candidates rows via Propose -- never
// kb.semantic_assertions directly (that is associate_semantics' job).
type Normalizer interface {
	// FamilyName is the artifact family this normalizer handles (e.g.
	// "metric", "provision"), matching the artifact_type values used in
	// kb.artifact_objects/kb.assertion_evidence.
	FamilyName() string
	// Normalize reads this family's artifacts for the given input record and
	// proposes candidates.
	Normalize(ctx context.Context, db *sql.DB, inputRecordID int64) (NormalizeReport, error)
}

// AssertionNormalizerRegistry is DR11 seam 5: adding a new artifact-family
// normalizer never requires editing normalize_assertions.go, only calling
// RegisterNormalizer.
type AssertionNormalizerRegistry struct {
	mu         sync.RWMutex
	normalizers map[string]Normalizer
}

var defaultRegistry = &AssertionNormalizerRegistry{normalizers: map[string]Normalizer{}}

// RegisterNormalizer adds a normalizer to the default registry. Registering
// two normalizers for the same family is a programming error.
func RegisterNormalizer(n Normalizer) error {
	return defaultRegistry.Register(n)
}

// Register adds a normalizer to this registry instance.
func (r *AssertionNormalizerRegistry) Register(n Normalizer) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	family := n.FamilyName()
	if family == "" {
		return fmt.Errorf("normalizer family name is required")
	}
	if _, exists := r.normalizers[family]; exists {
		return fmt.Errorf("normalizer already registered for family %q", family)
	}
	r.normalizers[family] = n
	return nil
}

// LookupNormalizer returns the default registry's normalizer for a family.
func LookupNormalizer(family string) (Normalizer, bool) {
	return defaultRegistry.Lookup(family)
}

// Lookup returns the normalizer registered for a family, if any.
func (r *AssertionNormalizerRegistry) Lookup(family string) (Normalizer, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	n, ok := r.normalizers[family]
	return n, ok
}

// RegisteredFamilies returns every family with a registered normalizer, in
// no particular order.
func RegisteredFamilies() []string {
	return defaultRegistry.Families()
}

// Families returns every family registered on this registry instance.
func (r *AssertionNormalizerRegistry) Families() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, 0, len(r.normalizers))
	for family := range r.normalizers {
		out = append(out, family)
	}
	return out
}
