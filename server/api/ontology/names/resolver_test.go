package names

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"regexp"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/chendingplano/deepdoc/server/api/ontology/assertions"
	"github.com/chendingplano/deepdoc/server/api/ontology/keywords"
	"github.com/chendingplano/deepdoc/server/api/ontology/semid"
)

var _ NameResolver = (*Resolver)(nil)

var testNow = time.Date(2026, 8, 5, 12, 0, 0, 0, time.UTC)

const (
	termQuerySQL  = "FROM kb.ontology_terms t"
	tier0SQL      = "FROM kb.keyword_surfaces s WHERE s.surface ="
	tier1SQL      = "FROM kb.keyword_surfaces s WHERE s.norm_key ="
	altKeySQL     = "FROM kb.keyword_surface_keys sk"
	rulesSQL      = "WHERE enabled = true AND scope ="
	conceptSQL    = "FROM kb.keyword_concepts WHERE concept_id ="
	decisionSQL   = "INSERT INTO kb.semid_decision_log"
	occurrenceSQL = "INSERT INTO kb.keyword_occurrences"
	normKeySQL    = "WHERE norm_key = $1 AND scope = $2"

	// aligns_to_term (Task 3) query fingerprints.
	acceptedForConceptQ = "AND subject_ref_id = $1"
	releasedExistsQ     = "SELECT EXISTS"
	getLatestQ          = "logical_identity_key = $1"
	assertionInsertQ    = "INSERT INTO kb.semantic_assertions"
	termPrefNameQ       = "WHERE l.term_id = $1"
)

func newFamily(db *sql.DB) *keywords.KeywordFamily {
	return &keywords.KeywordFamily{DB: db, ResolverMode: "observe"}
}

func termRows(rows ...[]any) *sqlmock.Rows {
	r := sqlmock.NewRows([]string{"term_id", "term_kind", "module_id", "label", "lang", "label_role"})
	for _, row := range rows {
		vals := make([]driver.Value, len(row))
		for i, v := range row {
			vals[i] = v
		}
		r.AddRow(vals...)
	}
	return r
}

func conceptRow(id, label string) *sqlmock.Rows {
	return sqlmock.NewRows([]string{
		"concept_id", "pref_label", "gloss", "scope", "status", "merged_into", "gloss_source", "create_time", "modify_time",
	}).AddRow(id, label, nil, "_", "active", nil, "human:tester", testNow, testNow)
}

func surfaceRow(id, conceptID, surface, normKey string) *sqlmock.Rows {
	return sqlmock.NewRows([]string{
		"surface_id", "concept_id", "surface", "norm_key", "norm_version",
		"label_role", "alias_type", "lang", "scope", "confidence",
		"provenance", "locked", "evidence", "create_time", "modify_time",
	}).AddRow(id, conceptID, surface, normKey, semid.CurrentNormalizerVersion,
		"pref", "pref", "en", "_", 1.0, "human:tester", false, nil, testNow, testNow)
}

// queueTotalMiss queues the keyword family's full tier sequence as misses
// for one name (mirrors KeywordFamily.CandidateNodes; "luminance"-length
// norms clear the tier-4 initials gate, so no initials query is queued).
func queueTotalMiss(mock sqlmock.Sqlmock, name, scope string) {
	n := semid.Normalizer{Version: semid.CurrentNormalizerVersion}
	ks := n.Normalize(name)
	mock.ExpectQuery(regexp.QuoteMeta(tier0SQL)).
		WithArgs(ks.Exact, scope).
		WillReturnRows(sqlmock.NewRows([]string{"concept_id"}))
	mock.ExpectQuery(regexp.QuoteMeta(tier1SQL)).
		WithArgs(ks.Norm, scope, semid.CurrentNormalizerVersion).
		WillReturnRows(sqlmock.NewRows([]string{"concept_id", "norm_key"}))
	for _, pair := range []struct{ kind, value string }{
		{"alnum", ks.Alnum},
		{"sorted", ks.Sorted},
		{"singular", ks.Singular},
	} {
		if pair.value == "" {
			continue
		}
		mock.ExpectQuery(regexp.QuoteMeta(altKeySQL)).
			WithArgs(pair.kind, pair.value, scope, semid.CurrentNormalizerVersion).
			WillReturnRows(sqlmock.NewRows([]string{"concept_id", "norm_key"}))
	}
	mock.ExpectQuery(regexp.QuoteMeta(rulesSQL)).
		WithArgs(scope).
		WillReturnRows(sqlmock.NewRows([]string{
			"rule_id", "pattern", "replacement", "scope", "enabled", "provenance", "create_time", "modify_time",
		}))
	if r := utf8.RuneCountInString(ks.Norm); r >= 2 && r <= 8 {
		mock.ExpectQuery(regexp.QuoteMeta(altKeySQL)).
			WithArgs("initials", ks.Norm, scope, semid.CurrentNormalizerVersion).
			WillReturnRows(sqlmock.NewRows([]string{"concept_id", "norm_key"}))
	}
}

// A disabled resolver (unset mode fails closed, K6) reports disabled and
// performs no reads or writes — the nil DB proves it: any query would fail.
func TestResolveNameDisabled(t *testing.T) {
	r := NewResolver(&keywords.KeywordFamily{})
	ctx := context.Background()

	res, err := r.ResolveName(ctx, ResolveNameRequest{Name: "Luminance", Scope: "_"})
	if err != nil {
		t.Fatalf("ResolveName: %v", err)
	}
	if res.Status != StatusDisabled {
		t.Errorf("status: got %q, want disabled", res.Status)
	}
	if res.NormalizedKey != "" || res.ConceptID != "" || res.TermID != "" {
		t.Errorf("disabled resolution must be empty: %#v", res)
	}

	batch, err := r.ResolveNames(ctx, []ResolveNameRequest{{Name: "A"}, {Name: "B"}})
	if err != nil || len(batch) != 2 || batch[0].Status != StatusDisabled || batch[1].Status != StatusDisabled {
		t.Errorf("ResolveNames on disabled resolver: %#v / %v", batch, err)
	}

	if err := r.ObserveName(ctx, NameOccurrence{RawName: "Luminance"}); err != nil {
		t.Errorf("ObserveName on disabled resolver must write nothing: %v", err)
	}
	if res, err := r.ResolveAndObserve(ctx, ResolveNameRequest{Name: "Luminance"}, NameOccurrence{}); err != nil || res.Status != StatusDisabled {
		t.Errorf("ResolveAndObserve on disabled resolver: %#v / %v", res, err)
	}
}

func TestResolveNameLexicalResolved(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	r := NewResolver(newFamily(db))
	ctx := context.Background()

	mock.ExpectQuery(regexp.QuoteMeta(termQuerySQL)).
		WillReturnRows(termRows())
	mock.ExpectQuery(regexp.QuoteMeta(tier0SQL)).
		WithArgs("Luminance", "_").
		WillReturnRows(sqlmock.NewRows([]string{"concept_id"}).AddRow("kwc_n2"))
	// FollowMerge inside ResolveSurface, then the candidate pref-name lookup.
	mock.ExpectQuery(regexp.QuoteMeta(conceptSQL)).
		WithArgs("kwc_n2").
		WillReturnRows(conceptRow("kwc_n2", "Luminance"))
	mock.ExpectQuery(regexp.QuoteMeta(conceptSQL)).
		WithArgs("kwc_n2").
		WillReturnRows(conceptRow("kwc_n2", "Luminance"))

	res, err := r.ResolveName(ctx, ResolveNameRequest{Name: "Luminance", Scope: "_"})
	if err != nil {
		t.Fatalf("ResolveName: %v", err)
	}
	if res.Status != StatusLexicalResolved {
		t.Errorf("status: got %q, want lexical_resolved", res.Status)
	}
	if res.ConceptID != "kwc_n2" || res.ConceptPrefName != "Luminance" {
		t.Errorf("concept: got %q/%q", res.ConceptID, res.ConceptPrefName)
	}
	if res.TermID != "" {
		t.Errorf("a lexical hit alone must never produce a TermID, got %q", res.TermID)
	}
	if res.NormalizedKey != "luminance" {
		t.Errorf("NormalizedKey: got %q", res.NormalizedKey)
	}
	if res.Method != "tier0_exact" || res.Confidence != 1.0 {
		t.Errorf("method/confidence: got %q/%v", res.Method, res.Confidence)
	}
	// Read path: every queued expectation was a SELECT — no writes exist.
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestResolveNameTermResolved(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	r := NewResolver(newFamily(db))
	ctx := context.Background()

	// Two released terms share the label; the term-kind filter breaks the
	// tie down to the single governed match.
	mock.ExpectQuery(regexp.QuoteMeta(termQuerySQL)).
		WillReturnRows(termRows(
			[]any{"core:luminance", "unit", "core", "Luminance", "en", "prefLabel"},
			[]any{"core:luminance_cls", "class", "core", "Luminance", "en", "prefLabel"},
		))
	mock.ExpectQuery(regexp.QuoteMeta(tier0SQL)).
		WithArgs("Luminance", "_").
		WillReturnRows(sqlmock.NewRows([]string{"concept_id"}).AddRow("kwc_n3"))
	mock.ExpectQuery(regexp.QuoteMeta(conceptSQL)).
		WithArgs("kwc_n3").
		WillReturnRows(conceptRow("kwc_n3", "Luminance"))
	mock.ExpectQuery(regexp.QuoteMeta(conceptSQL)).
		WithArgs("kwc_n3").
		WillReturnRows(conceptRow("kwc_n3", "Luminance"))

	res, err := r.ResolveName(ctx, ResolveNameRequest{
		Name: "Luminance", Scope: "_",
		ExpectedTermKinds: []string{"unit"},
		Language:          "en",
	})
	if err != nil {
		t.Fatalf("ResolveName: %v", err)
	}
	if res.Status != StatusTermResolved {
		t.Errorf("status: got %q, want term_resolved", res.Status)
	}
	if res.TermID != "core:luminance" || res.TermPrefName != "Luminance" ||
		res.TermKind != "unit" || res.ModuleID != "core" {
		t.Errorf("term layer: got %q/%q/%q/%q", res.TermID, res.TermPrefName, res.TermKind, res.ModuleID)
	}
	if res.ConceptID != "kwc_n3" {
		t.Errorf("lexical layer still links the concept: got %q", res.ConceptID)
	}
	if res.Method != "term_exact" || res.Confidence != 1.0 {
		t.Errorf("method/confidence: got %q/%v", res.Method, res.Confidence)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

// F1: a case/whitespace variant of a released term's label must still
// term_resolve — matching must run on Norm, not the trim-space-only Exact
// key. "  LUMINANCE  " normalizes to the same key as a released "Luminance".
func TestResolveNameTermResolvedCaseFold(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	r := NewResolver(newFamily(db))
	ctx := context.Background()

	mock.ExpectQuery(regexp.QuoteMeta(termQuerySQL)).
		WillReturnRows(termRows(
			[]any{"core:luminance", "unit", "core", "Luminance", "en", "prefLabel"},
		))
	queueTotalMiss(mock, "  LUMINANCE  ", "_")

	res, err := r.ResolveName(ctx, ResolveNameRequest{Name: "  LUMINANCE  ", Scope: "_"})
	if err != nil {
		t.Fatalf("ResolveName: %v", err)
	}
	if res.Status != StatusTermResolved || res.TermID != "core:luminance" {
		t.Errorf("case/whitespace variant must term-resolve: status %q term %q", res.Status, res.TermID)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

// Two released terms matching the same name with no filter to break the tie:
// the layer rule withholds the TermID and the answer falls through to the
// lexical layer.
func TestResolveNameTermTieFallsThrough(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	r := NewResolver(newFamily(db))
	ctx := context.Background()

	mock.ExpectQuery(regexp.QuoteMeta(termQuerySQL)).
		WillReturnRows(termRows(
			[]any{"core:luminance", "unit", "core", "Luminance", "en", "prefLabel"},
			[]any{"ext:luminance", "unit", "ext", "Luminance", "en", "prefLabel"},
		))
	mock.ExpectQuery(regexp.QuoteMeta(tier0SQL)).
		WithArgs("Luminance", "_").
		WillReturnRows(sqlmock.NewRows([]string{"concept_id"}).AddRow("kwc_n4"))
	mock.ExpectQuery(regexp.QuoteMeta(conceptSQL)).
		WithArgs("kwc_n4").
		WillReturnRows(conceptRow("kwc_n4", "Luminance"))
	mock.ExpectQuery(regexp.QuoteMeta(conceptSQL)).
		WithArgs("kwc_n4").
		WillReturnRows(conceptRow("kwc_n4", "Luminance"))

	res, err := r.ResolveName(ctx, ResolveNameRequest{Name: "Luminance", Scope: "_"})
	if err != nil {
		t.Fatalf("ResolveName: %v", err)
	}
	if res.Status != StatusLexicalResolved || res.TermID != "" {
		t.Errorf("term tie must fall through lexical: status %q term %q", res.Status, res.TermID)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestResolveNameAmbiguous(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	r := NewResolver(newFamily(db))
	ctx := context.Background()

	ks := (semid.Normalizer{Version: semid.CurrentNormalizerVersion}).Normalize("Luminance")
	mock.ExpectQuery(regexp.QuoteMeta(termQuerySQL)).
		WillReturnRows(termRows())
	mock.ExpectQuery(regexp.QuoteMeta(tier0SQL)).
		WithArgs(ks.Exact, "_").
		WillReturnRows(sqlmock.NewRows([]string{"concept_id"}))
	mock.ExpectQuery(regexp.QuoteMeta(tier1SQL)).
		WithArgs(ks.Norm, "_", semid.CurrentNormalizerVersion).
		WillReturnRows(sqlmock.NewRows([]string{"concept_id", "norm_key"}).
			AddRow("kwc_a", ks.Norm).
			AddRow("kwc_b", ks.Norm))
	// FollowMerge on the top-1, then the two candidate pref-name lookups.
	mock.ExpectQuery(regexp.QuoteMeta(conceptSQL)).
		WithArgs("kwc_a").
		WillReturnRows(conceptRow("kwc_a", "Luminance A"))
	mock.ExpectQuery(regexp.QuoteMeta(conceptSQL)).
		WithArgs("kwc_a").
		WillReturnRows(conceptRow("kwc_a", "Luminance A"))
	mock.ExpectQuery(regexp.QuoteMeta(conceptSQL)).
		WithArgs("kwc_b").
		WillReturnRows(conceptRow("kwc_b", "Luminance B"))

	res, err := r.ResolveName(ctx, ResolveNameRequest{Name: "Luminance", Scope: "_"})
	if err != nil {
		t.Fatalf("ResolveName: %v", err)
	}
	if res.Status != StatusAmbiguous {
		t.Errorf("status: got %q, want ambiguous", res.Status)
	}
	if res.ConceptID != "kwc_a" || res.ConceptPrefName != "Luminance A" {
		t.Errorf("top-1: got %q/%q", res.ConceptID, res.ConceptPrefName)
	}
	if len(res.Candidates) != 2 {
		t.Fatalf("tied set: got %d candidates, want 2", len(res.Candidates))
	}
	if res.Candidates[1].ConceptID != "kwc_b" || res.Candidates[1].PrefName != "Luminance B" {
		t.Errorf("second candidate: %#v", res.Candidates[1])
	}
	if res.Method != "tier1_norm" || res.Confidence != 1.0 {
		t.Errorf("method/confidence: got %q/%v", res.Method, res.Confidence)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

// A total miss on the read path stays unresolved with no concept —
// auto-creation is a write and belongs to the observe calls.
func TestResolveNameUnresolved(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	r := NewResolver(newFamily(db))
	ctx := context.Background()

	mock.ExpectQuery(regexp.QuoteMeta(termQuerySQL)).
		WillReturnRows(termRows())
	queueTotalMiss(mock, "Luminance", "_")

	res, err := r.ResolveName(ctx, ResolveNameRequest{Name: "Luminance", Scope: "_"})
	if err != nil {
		t.Fatalf("ResolveName: %v", err)
	}
	if res.Status != StatusUnresolved {
		t.Errorf("status: got %q, want unresolved", res.Status)
	}
	if res.ConceptID != "" || res.TermID != "" || len(res.Candidates) != 0 {
		t.Errorf("unresolved read must carry no identity: %#v", res)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestObserveNameWritesOccurrence(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	r := NewResolver(newFamily(db))
	ctx := context.Background()

	mock.ExpectQuery(regexp.QuoteMeta(tier0SQL)).
		WithArgs("Luminance", "_").
		WillReturnRows(sqlmock.NewRows([]string{"concept_id"}).AddRow("kwc_n7"))
	mock.ExpectQuery(regexp.QuoteMeta(conceptSQL)).
		WithArgs("kwc_n7").
		WillReturnRows(conceptRow("kwc_n7", "Luminance"))
	mock.ExpectQuery(regexp.QuoteMeta(decisionSQL)).
		WithArgs("keyword", "_", sqlmock.AnyArg(), sqlmock.AnyArg(), "auto_accepted",
			nil, nil, "keyword_family", 0).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(41))
	mock.ExpectQuery(regexp.QuoteMeta(occurrenceSQL)).
		WithArgs("document", "doc:9", "display.label", "Luminance", "luminance", "_",
			"ctx text", "chunk-2", "kwc_n7", nil, "lexical_resolved", int64(41)).
		WillReturnRows(sqlmock.NewRows([]string{"occurrence_id"}).AddRow(9))
	// Auto-accept side effect: the surface already exists, so no write.
	mock.ExpectQuery(regexp.QuoteMeta(normKeySQL)).
		WithArgs("luminance", "_").
		WillReturnRows(surfaceRow("kws_y", "kwc_n7", "Luminance", "luminance"))

	err = r.ObserveName(ctx, NameOccurrence{
		ArtifactType: "document",
		ArtifactID:   "doc:9",
		FieldPath:    "display.label",
		RawName:      "Luminance",
		Scope:        "_",
		Context:      "ctx text",
		ChunkRef:     "chunk-2",
	})
	if err != nil {
		t.Fatalf("ObserveName: %v", err)
	}
	// Exactly one decision and one occurrence were queued — ExpectationsWereMet
	// proves no extra writes happened.
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestResolveAndObserveTermResolvedWritesOnce(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	r := NewResolver(newFamily(db))
	ctx := context.Background()

	// Read pass: governed match + lexical tier-0 hit.
	mock.ExpectQuery(regexp.QuoteMeta(termQuerySQL)).
		WillReturnRows(termRows(
			[]any{"core:luminance", "unit", "core", "Luminance", "en", "prefLabel"},
		))
	mock.ExpectQuery(regexp.QuoteMeta(tier0SQL)).
		WithArgs("Luminance", "_").
		WillReturnRows(sqlmock.NewRows([]string{"concept_id"}).AddRow("kwc_n8"))
	mock.ExpectQuery(regexp.QuoteMeta(conceptSQL)).
		WithArgs("kwc_n8").
		WillReturnRows(conceptRow("kwc_n8", "Luminance"))
	mock.ExpectQuery(regexp.QuoteMeta(conceptSQL)).
		WithArgs("kwc_n8").
		WillReturnRows(conceptRow("kwc_n8", "Luminance"))

	// Write pass: the deterministic kernel re-run, then exactly one decision
	// and one linked occurrence carrying the governed term id.
	mock.ExpectQuery(regexp.QuoteMeta(tier0SQL)).
		WithArgs("Luminance", "_").
		WillReturnRows(sqlmock.NewRows([]string{"concept_id"}).AddRow("kwc_n8"))
	mock.ExpectQuery(regexp.QuoteMeta(conceptSQL)).
		WithArgs("kwc_n8").
		WillReturnRows(conceptRow("kwc_n8", "Luminance"))
	mock.ExpectQuery(regexp.QuoteMeta(decisionSQL)).
		WithArgs("keyword", "_", sqlmock.AnyArg(), sqlmock.AnyArg(), "auto_accepted",
			nil, nil, "keyword_family", 0).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(42))
	mock.ExpectQuery(regexp.QuoteMeta(occurrenceSQL)).
		WithArgs("metric", "m:1", "metric_name", "Luminance", "luminance", "_",
			"the luminance of the display", "chunk-1", "kwc_n8", "core:luminance",
			"term_resolved", int64(42)).
		WillReturnRows(sqlmock.NewRows([]string{"occurrence_id"}).AddRow(7))
	mock.ExpectQuery(regexp.QuoteMeta(normKeySQL)).
		WithArgs("luminance", "_").
		WillReturnRows(surfaceRow("kws_x", "kwc_n8", "Luminance", "luminance"))

	res, err := r.ResolveAndObserve(ctx,
		ResolveNameRequest{Name: "Luminance", Scope: "_", ExpectedTermKinds: []string{"unit"}},
		NameOccurrence{
			ArtifactType: "metric",
			ArtifactID:   "m:1",
			FieldPath:    "metric_name",
			RawName:      "Luminance",
			Scope:        "_",
			Context:      "the luminance of the display",
			ChunkRef:     "chunk-1",
		})
	if err != nil {
		t.Fatalf("ResolveAndObserve: %v", err)
	}
	if res.Status != StatusTermResolved || res.TermID != "core:luminance" {
		t.Errorf("resolution: status %q term %q", res.Status, res.TermID)
	}
	if res.ConceptID != "kwc_n8" {
		t.Errorf("concept link: got %q", res.ConceptID)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

// alignEvidence and alignQualifiersJSON mirror keywords/alignment_test.go for
// the auto-align evidence string the resolver's ResolveAndObserve writes
// (encoding/json sorts map keys: evidence < method).
const alignEvidence = "pref_label exact match to a released metric_definition label (auto-assign, §16.1)"

const alignQualifiersJSON = `{"evidence":"pref_label exact match to a released metric_definition label (auto-assign, §16.1)","method":"term_exact"}`

// alignAssertionRow is a full 37-column kb.semantic_assertions row as returned
// by CreateAssertion, carrying an accepted keyword_concept -> ontology_term
// alignment (mirrors keywords/alignment_test.go's row).
func alignAssertionRow(id int64, conceptID, termID string) *sqlmock.Rows {
	lk := "kwc:" + conceptID + ":core:aligns_to_term:" + termID
	return sqlmock.NewRows([]string{
		"id", "logical_identity_key", "revision", "subject_ref_kind", "subject_ref_id",
		"subject_object_id", "predicate_term_id", "object_ref_kind", "object_ref_id",
		"object_object_id", "object_literal", "assertion_kind_term_id", "polarity",
		"modality", "qualifiers", "confidence", "value_form", "numeric_value", "lower_value",
		"upper_value", "lower_inclusive", "upper_inclusive", "comparator", "unit_term_id",
		"quantity_kind_term_id", "raw_text", "status", "decision_reason",
		"dependency_fingerprint", "superseded_by", "valid_time_start", "valid_time_end",
		"transaction_time", "create_time", "create_by", "modify_time", "modify_by",
	}).AddRow(
		id, lk, 1, "keyword_concept", conceptID,
		nil, "core:aligns_to_term", "ontology_term", termID,
		nil, []byte("null"), nil, "positive",
		nil, []byte(alignQualifiersJSON), 1.0, nil, nil, nil,
		nil, nil, nil, nil, nil,
		nil, "", "accepted", alignEvidence,
		nil, nil, nil, nil,
		testNow, testNow, nil, testNow, nil,
	)
}

// alignReadRow is the focused AcceptedForConcept row (8 columns).
func alignReadRow(id int64, conceptID, termID string) *sqlmock.Rows {
	return sqlmock.NewRows([]string{
		"id", "subject_ref_id", "object_ref_kind", "object_ref_id", "status", "qualifiers", "confidence", "decision_reason",
	}).AddRow(id, conceptID, "ontology_term", termID, "accepted", []byte(alignQualifiersJSON), 1.0, alignEvidence)
}

func noAlignRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{
		"id", "subject_ref_id", "object_ref_kind", "object_ref_id", "status", "qualifiers", "confidence", "decision_reason",
	})
}

// REQ-2 (spec §16.2): a name that matches no released label still resolves to
// a governed term when the concept it auto-accepts is aligned to one. The read
// path stays write-free — no INSERT/UPDATE/decision-log expectations are
// queued, so ExpectationsWereMet proves it.
func TestResolveNameTermResolvedViaAlignment(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	r := NewResolverWithAlignments(newFamily(db), &keywords.AlignmentsStore{
		Assertions:  assertions.AssertionStore{DB: db},
		DecisionLog: semid.DecisionLogStore{DB: db},
		Scope:       "_",
	})
	ctx := context.Background()

	// Released-term lookup on the raw name finds nothing; the lexical layer
	// auto-accepts kwc_b; kwc_b is already aligned to mea:Luminance.
	mock.ExpectQuery(regexp.QuoteMeta(termQuerySQL)).
		WillReturnRows(termRows())
	mock.ExpectQuery(regexp.QuoteMeta(tier0SQL)).
		WithArgs("亮度", "_").
		WillReturnRows(sqlmock.NewRows([]string{"concept_id"}).AddRow("kwc_b"))
	mock.ExpectQuery(regexp.QuoteMeta(conceptSQL)).
		WithArgs("kwc_b").
		WillReturnRows(conceptRow("kwc_b", "亮度"))
	mock.ExpectQuery(regexp.QuoteMeta(conceptSQL)).
		WithArgs("kwc_b").
		WillReturnRows(conceptRow("kwc_b", "亮度"))
	mock.ExpectQuery(regexp.QuoteMeta(acceptedForConceptQ)).
		WithArgs("kwc_b").
		WillReturnRows(alignReadRow(1, "kwc_b", "mea:Luminance"))
	mock.ExpectQuery(regexp.QuoteMeta(termPrefNameQ)).
		WithArgs("mea:Luminance").
		WillReturnRows(sqlmock.NewRows([]string{"label"}).AddRow("Luminance"))

	res, err := r.ResolveName(ctx, ResolveNameRequest{Name: "亮度", Scope: "_"})
	if err != nil {
		t.Fatalf("ResolveName: %v", err)
	}
	if res.Status != StatusTermResolved {
		t.Errorf("status: got %q, want term_resolved", res.Status)
	}
	if res.TermID != "mea:Luminance" || res.TermPrefName != "Luminance" {
		t.Errorf("term: got %q/%q, want mea:Luminance/Luminance", res.TermID, res.TermPrefName)
	}
	if res.TermKind != "metric_definition" {
		t.Errorf("term kind: got %q, want metric_definition", res.TermKind)
	}
	if res.Method != "aligns_to_term" || res.Confidence != 1.0 {
		t.Errorf("method/confidence: got %q/%v", res.Method, res.Confidence)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

// §16.1 D11 auto-assign producer: a concept whose pref_label exactly matches a
// released metric_definition label is aligned to it on the observe path, and
// the returned resolution (and its occurrence) become term_resolved.
func TestResolveAndObserveAutoAlignsOnExactLabel(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	r := NewResolverWithAlignments(newFamily(db), &keywords.AlignmentsStore{
		Assertions:  assertions.AssertionStore{DB: db},
		DecisionLog: semid.DecisionLogStore{DB: db},
		Scope:       "_",
	})
	ctx := context.Background()

	// --- Read pass (ResolveName): the raw name matches no released label, the
	// lexical layer auto-accepts kwc_l (pref_label "Luminance"), and the
	// alignment-follow read finds no accepted alignment yet. ---
	mock.ExpectQuery(regexp.QuoteMeta(termQuerySQL)).
		WillReturnRows(termRows())
	mock.ExpectQuery(regexp.QuoteMeta(tier0SQL)).
		WithArgs("亮度", "_").
		WillReturnRows(sqlmock.NewRows([]string{"concept_id"}).AddRow("kwc_l"))
	mock.ExpectQuery(regexp.QuoteMeta(conceptSQL)).
		WithArgs("kwc_l").
		WillReturnRows(conceptRow("kwc_l", "Luminance"))
	mock.ExpectQuery(regexp.QuoteMeta(conceptSQL)).
		WithArgs("kwc_l").
		WillReturnRows(conceptRow("kwc_l", "Luminance"))
	mock.ExpectQuery(regexp.QuoteMeta(acceptedForConceptQ)).
		WithArgs("kwc_l").
		WillReturnRows(noAlignRows())

	// --- Auto-align: "Luminance" exactly matches the released mea:Luminance
	// term, and EnsureAccepted mints the alignment (conflict gate, released
	// guard, idempotency probe, assertion INSERT, DR15 audit row). ---
	mock.ExpectQuery(regexp.QuoteMeta(termQuerySQL)).
		WillReturnRows(termRows(
			[]any{"mea:Luminance", "metric_definition", "mea", "Luminance", "en", "prefLabel"},
		))
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("SELECT pg_advisory_xact_lock(1264011588, 1);")).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(regexp.QuoteMeta(acceptedForConceptQ)).
		WithArgs("kwc_l").
		WillReturnRows(noAlignRows())
	mock.ExpectQuery(regexp.QuoteMeta(releasedExistsQ)).
		WithArgs("mea:Luminance", "metric_definition").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectQuery(regexp.QuoteMeta(releasedExistsQ)).
		WithArgs("core:aligns_to_term", "property").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectQuery(regexp.QuoteMeta(getLatestQ)).
		WithArgs("kwc:kwc_l:core:aligns_to_term:mea:Luminance").
		WillReturnError(sql.ErrNoRows)
	mock.ExpectQuery(regexp.QuoteMeta(assertionInsertQ)).
		WithArgs(
			"kwc:kwc_l:core:aligns_to_term:mea:Luminance", "keyword_concept", "kwc_l", nil,
			"core:aligns_to_term", "ontology_term", "mea:Luminance", nil, "null", nil,
			"positive", nil, alignQualifiersJSON, 1.0, nil,
			nil, nil, nil, nil, nil, nil, nil, nil, nil, "accepted", nil, nil, nil, nil,
		).
		WillReturnRows(alignAssertionRow(1, "kwc_l", "mea:Luminance"))
	mock.ExpectQuery(regexp.QuoteMeta(decisionSQL)).
		WithArgs("keyword_align", "_", `{"concept_id":"kwc_l","term_id":"mea:Luminance"}`, sqlmock.AnyArg(), "accepted", nil, nil, "auto-align", 0).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(1)))
	mock.ExpectCommit()

	// --- Observe write: deterministic kernel re-run, exactly one decision and
	// one linked occurrence carrying the auto-aligned term, and no extra
	// surface write (the surface already exists). ---
	mock.ExpectQuery(regexp.QuoteMeta(tier0SQL)).
		WithArgs("亮度", "_").
		WillReturnRows(sqlmock.NewRows([]string{"concept_id"}).AddRow("kwc_l"))
	mock.ExpectQuery(regexp.QuoteMeta(conceptSQL)).
		WithArgs("kwc_l").
		WillReturnRows(conceptRow("kwc_l", "Luminance"))
	mock.ExpectQuery(regexp.QuoteMeta(decisionSQL)).
		WithArgs("keyword", "_", sqlmock.AnyArg(), sqlmock.AnyArg(), "auto_accepted",
			nil, nil, "keyword_family", 0).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(42))
	mock.ExpectQuery(regexp.QuoteMeta(occurrenceSQL)).
		WithArgs("metric", "m:1", "metric_name", "亮度", "亮度", "_",
			"the luminance of the display", "chunk-1", "kwc_l", "mea:Luminance",
			"term_resolved", int64(42)).
		WillReturnRows(sqlmock.NewRows([]string{"occurrence_id"}).AddRow(7))
	mock.ExpectQuery(regexp.QuoteMeta(normKeySQL)).
		WithArgs("亮度", "_").
		WillReturnRows(surfaceRow("kws_l", "kwc_l", "亮度", "亮度"))

	res, err := r.ResolveAndObserve(ctx,
		ResolveNameRequest{Name: "亮度", Scope: "_"},
		NameOccurrence{
			ArtifactType: "metric",
			ArtifactID:   "m:1",
			FieldPath:    "metric_name",
			RawName:      "亮度",
			Scope:        "_",
			Context:      "the luminance of the display",
			ChunkRef:     "chunk-1",
		})
	if err != nil {
		t.Fatalf("ResolveAndObserve: %v", err)
	}
	if res.Status != StatusTermResolved {
		t.Errorf("status: got %q, want term_resolved", res.Status)
	}
	if res.TermID != "mea:Luminance" || res.TermPrefName != "Luminance" {
		t.Errorf("term: got %q/%q, want mea:Luminance/Luminance", res.TermID, res.TermPrefName)
	}
	if res.TermKind != "metric_definition" || res.Method != "aligns_to_term" || res.Confidence != 1.0 {
		t.Errorf("term layer: kind/method/confidence %q/%q/%v", res.TermKind, res.Method, res.Confidence)
	}
	if res.ConceptID != "kwc_l" {
		t.Errorf("concept link: got %q", res.ConceptID)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

// A disabled resolver stays inert even with an alignments store wired — no
// alignment write, no occurrence, no reads at all.
func TestResolveAndObserveDisabledWritesNothing(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	r := NewResolverWithAlignments(&keywords.KeywordFamily{DB: db, ResolverMode: "off"}, &keywords.AlignmentsStore{
		Assertions:  assertions.AssertionStore{DB: db},
		DecisionLog: semid.DecisionLogStore{DB: db},
		Scope:       "_",
	})
	ctx := context.Background()

	res, err := r.ResolveAndObserve(ctx, ResolveNameRequest{Name: "亮度", Scope: "_"}, NameOccurrence{})
	if err != nil {
		t.Fatalf("ResolveAndObserve: %v", err)
	}
	if res.Status != StatusDisabled {
		t.Errorf("status: got %q, want disabled", res.Status)
	}
	if res.TermID != "" || res.ConceptID != "" {
		t.Errorf("disabled resolution must carry no identity: %#v", res)
	}
	// Zero queries queued — ExpectationsWereMet fails on any read or write.
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

// The governed layer's own exact label match is authoritative over a
// stale/conflicting alignment: the alignment-follow runs only on the
// no-governed-hit path, so no AcceptedForConcept query is even issued here.
func TestResolveNameAlignmentDoesNotOverrideDifferentTerm(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	r := NewResolverWithAlignments(newFamily(db), &keywords.AlignmentsStore{
		Assertions:  assertions.AssertionStore{DB: db},
		DecisionLog: semid.DecisionLogStore{DB: db},
		Scope:       "_",
	})
	ctx := context.Background()

	mock.ExpectQuery(regexp.QuoteMeta(termQuerySQL)).
		WillReturnRows(termRows(
			[]any{"mea:Luminance", "metric_definition", "mea", "Luminance", "en", "prefLabel"},
		))
	mock.ExpectQuery(regexp.QuoteMeta(tier0SQL)).
		WithArgs("Luminance", "_").
		WillReturnRows(sqlmock.NewRows([]string{"concept_id"}).AddRow("kwc_d"))
	mock.ExpectQuery(regexp.QuoteMeta(conceptSQL)).
		WithArgs("kwc_d").
		WillReturnRows(conceptRow("kwc_d", "Luminance"))
	mock.ExpectQuery(regexp.QuoteMeta(conceptSQL)).
		WithArgs("kwc_d").
		WillReturnRows(conceptRow("kwc_d", "Luminance"))

	res, err := r.ResolveName(ctx, ResolveNameRequest{Name: "Luminance", Scope: "_", ExpectedTermKinds: []string{"metric_definition"}})
	if err != nil {
		t.Fatalf("ResolveName: %v", err)
	}
	if res.Status != StatusTermResolved || res.TermID != "mea:Luminance" {
		t.Errorf("governed exact match must win: status %q term %q", res.Status, res.TermID)
	}
	if res.Method != "term_exact" || res.TermKind != "metric_definition" {
		t.Errorf("term layer: method/kind %q/%q", res.Method, res.TermKind)
	}
	// No acceptedForConcept expectation was queued — if the alignment-follow
	// block consulted the store, ExpectationsWereMet would fail.
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}
