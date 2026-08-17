package semantic

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// FingerprintVersion prefixes every dependency fingerprint. DR10 requires
// "canonical, versioned serialization": without the prefix, adding a new
// dependency axis would silently change every existing fingerprint and
// re-trigger the entire retry queue, which is exactly the alert storm the ADR
// rejects.
const FingerprintVersion = "v1"

// Dependencies is the declared dependency set of one stage attempt (DR10).
// Every axis is present in the canonical serialization even when empty, so
// "the mapping revision is unknown" and "there is no mapping axis" are
// distinguishable and a later axis addition is a deliberate version bump.
type Dependencies struct {
	MappingRevision       string `json:"mapping_revision"`
	ParserVersion         string `json:"parser_version"`
	ClassIdentityDecision string `json:"class_identity_decision"`
	ClassContractRevision string `json:"class_contract_revision"`
	ValidatorVersion      string `json:"validator_version"`
	UnitVocabularyRelease string `json:"unit_vocabulary_release"`
	ModelVersion          string `json:"model_version"`
	PromptVersion         string `json:"prompt_version"`
	// Extra carries family-specific axes an adapter declares. Sorted-key JSON
	// keeps it deterministic.
	Extra map[string]string `json:"extra,omitempty"`
}

// Fingerprint returns the canonical versioned fingerprint of this dependency
// set. Equal dependency sets always produce byte-equal output regardless of Go
// map iteration order, which is what makes "unchanged dependency" detectable
// and therefore what stops repeated retries of unchanged work.
func (d Dependencies) Fingerprint() string {
	return hashCanonical(d.canonical())
}

func (d Dependencies) canonical() string {
	// encoding/json sorts map keys, and the struct fields serialize in
	// declaration order, so the output is deterministic without extra work.
	// Extra is normalized to nil when empty so an empty map and an absent map
	// do not produce different bytes.
	if len(d.Extra) == 0 {
		d.Extra = nil
	}
	buf, err := json.Marshal(d)
	if err != nil {
		// Dependencies contains only strings and a string map, so Marshal
		// cannot fail. Panicking here would be worse than a stable sentinel:
		// a fingerprint that never matches simply forces a retry.
		return fmt.Sprintf(`{"unserializable":%q}`, err.Error())
	}
	return string(buf)
}

// AggregateFingerprint is DR10's rule that "each outcome's dependency
// fingerprint is the canonical aggregate of its stage dependency and current
// child-finding dependencies".
//
// stageFingerprint is the stage's own Dependencies.Fingerprint(). Child
// fingerprints are sorted before hashing so the aggregate does not depend on
// the order findings happen to be written in; a child-level change therefore
// necessarily changes the envelope fingerprint, and nothing else does.
//
// An outcome with no findings still aggregates, over an empty child list, so
// that a stored envelope fingerprint is always an aggregate. Mixing raw stage
// fingerprints and aggregates in one column would make the DR4 replay
// comparison ("same input and dependency fingerprint") depend on whether the
// previous attempt happened to have children.
func AggregateFingerprint(stageFingerprint string, childFingerprints []string) string {
	sorted := make([]string, len(childFingerprints))
	copy(sorted, childFingerprints)
	sort.Strings(sorted)
	payload := struct {
		Stage    string   `json:"stage"`
		Children []string `json:"children"`
	}{Stage: stageFingerprint, Children: sorted}
	buf, err := json.Marshal(payload)
	if err != nil {
		return fmt.Sprintf(`%s:unserializable`, FingerprintVersion)
	}
	return hashCanonical(string(buf))
}

func hashCanonical(canonical string) string {
	sum := sha256.Sum256([]byte(canonical))
	return FingerprintVersion + ":" + hex.EncodeToString(sum[:])
}

// ParseFingerprintVersion returns the serialization version of a fingerprint.
// A fingerprint whose version differs from FingerprintVersion must not be
// compared for equality with a current one: DR10's "unchanged fingerprint
// reuses the existing outcome" rule is only sound within one version.
func ParseFingerprintVersion(fingerprint string) (string, bool) {
	idx := strings.Index(fingerprint, ":")
	if idx <= 0 {
		return "", false
	}
	return fingerprint[:idx], true
}

// SameCurrentVersion reports whether both fingerprints use the current
// serialization version and are equal.
func SameCurrentVersion(a, b string) bool {
	va, okA := ParseFingerprintVersion(a)
	vb, okB := ParseFingerprintVersion(b)
	return okA && okB && va == FingerprintVersion && vb == FingerprintVersion && a == b
}

// OutcomeKey is DR4's deterministic hash of (input_record_id, artifact_type,
// artifact_id, stage_term_id). It is the stable scope key shared by every
// revision of one attempt, so the active-row unique index needs only this
// column.
func OutcomeKey(inputRecordID int64, artifactType, artifactID, stageTermID string) string {
	return hashCanonical(canonicalJoin(
		fmt.Sprintf("%d", inputRecordID), artifactType, artifactID, stageTermID))
}

// OccurrenceKey is DR13's deterministic key for an unresolved semantic
// occurrence. The family-declared source scope is part of the key, which is
// why uq_unresolved_semantic_occurrences_active needs only occurrence_key.
func OccurrenceKey(sourceScope string, inputRecordID int64, artifactType, artifactID string) string {
	return hashCanonical(canonicalJoin(
		sourceScope, fmt.Sprintf("%d", inputRecordID), artifactType, artifactID))
}

// FindingKey is DR4's deterministic key for a finding within its outcome's
// family-declared decision scope. Scoping by decision scope rather than by
// finding term is what lets one stage report several independent findings
// without them colliding, while a replay of the same axis reuses its row.
func FindingKey(decisionScope, dimensionTermID string) string {
	return hashCanonical(canonicalJoin(decisionScope, dimensionTermID))
}

// canonicalJoin builds an unambiguous joined string. Length-prefixing each
// part prevents ("a", "bc") and ("ab", "c") from colliding, which a plain
// separator cannot guarantee for free-text artifact IDs.
func canonicalJoin(parts ...string) string {
	var sb strings.Builder
	for _, p := range parts {
		fmt.Fprintf(&sb, "%d:%s|", len(p), p)
	}
	return sb.String()
}
