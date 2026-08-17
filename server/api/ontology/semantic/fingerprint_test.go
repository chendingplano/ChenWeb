package semantic

import (
	"strings"
	"testing"
)

// Task 4.3: fingerprint stability. If two equal dependency sets could produce
// different fingerprints, DR10's "unchanged fingerprint reuses the existing
// outcome" rule would silently degrade into "retry everything on every run".
func TestFingerprintIsStableAcrossMapIterationOrder(t *testing.T) {
	// Go randomizes map iteration order, so building the same Extra map
	// repeatedly and hashing exercises the ordering hazard directly.
	first := Dependencies{
		MappingRevision: "map-7",
		ParserVersion:   "p-2",
		Extra:           map[string]string{"a": "1", "b": "2", "c": "3", "d": "4", "e": "5"},
	}.Fingerprint()
	for i := 0; i < 200; i++ {
		got := Dependencies{
			MappingRevision: "map-7",
			ParserVersion:   "p-2",
			Extra:           map[string]string{"e": "5", "d": "4", "c": "3", "b": "2", "a": "1"},
		}.Fingerprint()
		if got != first {
			t.Fatalf("fingerprint not stable across iterations: %s != %s", got, first)
		}
	}
}

func TestFingerprintEmptyExtraEqualsNilExtra(t *testing.T) {
	// An empty map and an absent map describe the same dependency set; if they
	// hashed differently, an adapter that initialized Extra eagerly would
	// supersede every outcome on its first run after a refactor.
	withEmpty := Dependencies{ParserVersion: "p-1", Extra: map[string]string{}}.Fingerprint()
	withNil := Dependencies{ParserVersion: "p-1"}.Fingerprint()
	if withEmpty != withNil {
		t.Fatalf("empty Extra %s != nil Extra %s", withEmpty, withNil)
	}
}

func TestFingerprintChangesWhenAnyAxisChanges(t *testing.T) {
	base := Dependencies{MappingRevision: "map-1", ParserVersion: "p-1"}
	cases := map[string]Dependencies{
		"mapping":   {MappingRevision: "map-2", ParserVersion: "p-1"},
		"parser":    {MappingRevision: "map-1", ParserVersion: "p-2"},
		"class":     {MappingRevision: "map-1", ParserVersion: "p-1", ClassIdentityDecision: "c-1"},
		"contract":  {MappingRevision: "map-1", ParserVersion: "p-1", ClassContractRevision: "r-1"},
		"validator": {MappingRevision: "map-1", ParserVersion: "p-1", ValidatorVersion: "v-1"},
		"unit":      {MappingRevision: "map-1", ParserVersion: "p-1", UnitVocabularyRelease: "q-1"},
		"model":     {MappingRevision: "map-1", ParserVersion: "p-1", ModelVersion: "m-1"},
		"prompt":    {MappingRevision: "map-1", ParserVersion: "p-1", PromptVersion: "pr-1"},
		"extra":     {MappingRevision: "map-1", ParserVersion: "p-1", Extra: map[string]string{"x": "1"}},
	}
	baseFP := base.Fingerprint()
	for name, d := range cases {
		if d.Fingerprint() == baseFP {
			t.Errorf("changing %s did not change the fingerprint", name)
		}
	}
}

func TestFingerprintCarriesVersionPrefix(t *testing.T) {
	fp := Dependencies{ParserVersion: "p-1"}.Fingerprint()
	if !strings.HasPrefix(fp, FingerprintVersion+":") {
		t.Fatalf("fingerprint %q has no version prefix", fp)
	}
	v, ok := ParseFingerprintVersion(fp)
	if !ok || v != FingerprintVersion {
		t.Fatalf("ParseFingerprintVersion(%q) = %q, %v", fp, v, ok)
	}
}

func TestSameCurrentVersionRejectsOtherVersions(t *testing.T) {
	fp := Dependencies{ParserVersion: "p-1"}.Fingerprint()
	if !SameCurrentVersion(fp, fp) {
		t.Fatal("identical current-version fingerprints should compare equal")
	}
	// A fingerprint from an older serialization must never be treated as equal
	// to a current one, even byte-for-byte: DR10's reuse rule is only sound
	// within one version.
	legacy := "v0:" + strings.TrimPrefix(fp, FingerprintVersion+":")
	if SameCurrentVersion(legacy, legacy) {
		t.Fatal("old-version fingerprints must not satisfy SameCurrentVersion")
	}
}

// Task 4.3: the aggregate must change when a child changes, and must not
// depend on the order children were written in.
func TestAggregateFingerprintIsOrderIndependentAndChildSensitive(t *testing.T) {
	stage := Dependencies{MappingRevision: "map-1"}.Fingerprint()
	a := AggregateFingerprint(stage, []string{"v1:aaa", "v1:bbb", "v1:ccc"})
	b := AggregateFingerprint(stage, []string{"v1:ccc", "v1:aaa", "v1:bbb"})
	if a != b {
		t.Fatalf("aggregate depends on child order: %s != %s", a, b)
	}
	changed := AggregateFingerprint(stage, []string{"v1:aaa", "v1:bbb", "v1:CHANGED"})
	if changed == a {
		t.Fatal("a child-level dependency change must change the envelope fingerprint (DR10)")
	}
	noChildren := AggregateFingerprint(stage, nil)
	if noChildren == a {
		t.Fatal("gaining findings must change the envelope fingerprint")
	}
	if noChildren == stage {
		t.Fatal("a childless envelope must still store an aggregate, not the bare stage fingerprint")
	}
}

// canonicalJoin length-prefixes its parts. Without that, ("ab","c") and
// ("a","bc") would hash identically, so two different metrics could share an
// outcome envelope -- exactly what DR4 forbids.
func TestOutcomeKeyDoesNotCollideOnConcatenation(t *testing.T) {
	a := OutcomeKey(1, "metric", "ab", StageNormalize)
	b := OutcomeKey(1, "metric", "a", StageNormalize)
	if a == b {
		t.Fatal("outcome keys collide across artifact-id boundaries")
	}
	c := OutcomeKey(1, "metricab", "", StageNormalize)
	if a == c {
		t.Fatal("outcome keys collide across the artifact-type/artifact-id boundary")
	}
}

func TestOutcomeKeyIsDeterministic(t *testing.T) {
	a := OutcomeKey(42, "metric", "m-1", StageNormalize)
	b := OutcomeKey(42, "metric", "m-1", StageNormalize)
	if a != b {
		t.Fatalf("outcome key is not deterministic: %s != %s", a, b)
	}
	if OutcomeKey(42, "metric", "m-1", StageAssociate) == a {
		t.Fatal("outcome key must vary by stage: one envelope per stage (DR4)")
	}
}

func TestFindingKeyVariesByDecisionScope(t *testing.T) {
	// The whole point of scoping finding keys by decision scope is that one
	// stage can report a mapping finding and a value finding at once.
	mapping := FindingKey("range_type_mapping", DimensionMapping)
	value := FindingKey("value_literal", DimensionValue)
	if mapping == value {
		t.Fatal("findings in different decision scopes must have different keys (DR4)")
	}
}

func TestOccurrenceKeyVariesByScope(t *testing.T) {
	a := OccurrenceKey("metric_occurrence:v1", 1, "metric", "m-1")
	b := OccurrenceKey("metric_occurrence:v2", 1, "metric", "m-1")
	if a == b {
		t.Fatal("occurrence key must include the family-declared source scope (DR13)")
	}
}
