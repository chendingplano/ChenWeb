package profiles

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"reflect"
	"strings"
	"testing"

	"github.com/chendingplano/deepdoc/server/api/ontology/semrules"
)

// captureWriter collects slog text output into a buffer so a test can assert
// the selector surfaced a best-effort alarm-write failure via the logger.
type captureWriter struct{ buf *string }

func (w *captureWriter) Write(p []byte) (int, error) {
	*w.buf += string(p)
	return len(p), nil
}

var _ io.Writer = (*captureWriter)(nil)

// stubSelectionSource fakes the SelectionSource surface (ProfileStore): the
// knowledge-store derivation and the pinned released-profile load, so the
// selector tests exercise real evaluation logic without SQL.
type stubSelectionSource struct {
	storeID   int64
	released  ReleasedProfiles
	deriveErr error
	loadErr   error
}

func (s *stubSelectionSource) DeriveKnowledgeStore(_ context.Context, _ []int64) (int64, error) {
	return s.storeID, s.deriveErr
}

func (s *stubSelectionSource) LoadReleasedProfiles(_ context.Context) (ReleasedProfiles, error) {
	return s.released, s.loadErr
}

// stubSubjectFactsLoader records the subjects it was asked about and returns a
// fixed per-subject fact set, standing in for the doc-processing adapters.
type stubSubjectFactsLoader struct {
	facts  semrules.FactSet
	called []SelectionSubject
	err    error
}

func (l *stubSubjectFactsLoader) LoadSubjectFacts(_ context.Context, subject SelectionSubject) (semrules.FactSet, error) {
	l.called = append(l.called, subject)
	if l.err != nil {
		return nil, l.err
	}
	return l.facts, nil
}

func selectionFixture(profiles ...Profile) ReleasedProfiles {
	return ReleasedProfiles{
		Releases: []PinnedRelease{
			{ModuleID: "ventilator-display", ReleaseID: 42, Version: "0.1.0", Checksum: "sha256:aaa"},
		},
		Profiles: profiles,
	}
}

func selectionProfile(profileID, applicability, closedDimensions string) Profile {
	return Profile{
		ID: 7, ProfileID: profileID, Version: 1, ModuleID: "ventilator-display",
		Status: "included_in_release", Applicability: json.RawMessage(applicability),
		ClosedDimensions: json.RawMessage(closedDimensions), ReleaseID: 42, ReleaseVersion: "0.1.0",
	}
}

const (
	applicabilityTrueUS = `{"version":1,"expression":{"kind":"fact","path":"review.jurisdiction","op":"eq","value":"US"}}`
	applicabilityIndet  = `{"version":1,"expression":{"kind":"fact","path":"document.doc_kind","op":"eq","value":"standard"}}`
)

func selectionReviewContext() ReviewApplicabilityContext {
	return ReviewApplicabilityContext{
		AsOfDate: "2026-08-02", Jurisdiction: "US", OperatingContext: "production", Purpose: "compliance",
	}
}

func parseApplicabilityDocument(raw string) (semrules.Document, error) {
	var doc semrules.Document
	err := json.Unmarshal([]byte(raw), &doc)
	return doc, err
}

func TestSelectEvaluatesEachProfileOncePerDocumentTargetSubject(t *testing.T) {
	selector := Selector{Source: &stubSelectionSource{storeID: 9, released: selectionFixture(selectionProfile("p1", applicabilityTrueUS, `[]`))}}
	loader := &stubSubjectFactsLoader{facts: semrules.FactSet{}}
	result, err := selector.Select(context.Background(), SelectionRequest{
		ReviewScopeID:       "scope-t",
		ReviewedDocumentIDs: []int64{101, 102},
		TargetObjectIDs:     []string{"obj-1"},
		ReviewContext:       selectionReviewContext(),
		SubjectFacts:        loader,
	})
	if err != nil {
		t.Fatalf("Select: %v", err)
	}
	wantSubjects := []SelectionSubject{
		{DocumentID: 101, TargetObjectID: "obj-1"},
		{DocumentID: 102, TargetObjectID: "obj-1"},
	}
	if !reflect.DeepEqual(loader.called, wantSubjects) {
		t.Fatalf("subjects = %#v, want %#v", loader.called, wantSubjects)
	}
	// One evaluation per subject (2 subjects x 1 profile).
	if len(result.Snapshot.Evaluations) != 2 {
		t.Fatalf("evaluations = %d, want 2 (one per subject)", len(result.Snapshot.Evaluations))
	}
	if result.KnowledgeStoreID != 9 {
		t.Fatalf("knowledge_store_id = %d, want 9", result.KnowledgeStoreID)
	}
	if result.SelectionAttemptID == "" {
		t.Fatal("selection_attempt_id must be set")
	}
}

func TestSelectUsesDocumentOnlySubjectsWhenNoTargetSupplied(t *testing.T) {
	selector := Selector{Source: &stubSelectionSource{storeID: 9, released: selectionFixture(selectionProfile("p1", applicabilityTrueUS, `[]`))}}
	loader := &stubSubjectFactsLoader{facts: semrules.FactSet{}}
	result, err := selector.Select(context.Background(), SelectionRequest{
		ReviewScopeID:       "scope-t",
		ReviewedDocumentIDs: []int64{101, 102},
		ReviewContext:       selectionReviewContext(),
		SubjectFacts:        loader,
	})
	if err != nil {
		t.Fatalf("Select: %v", err)
	}
	wantSubjects := []SelectionSubject{{DocumentID: 101}, {DocumentID: 102}}
	if !reflect.DeepEqual(loader.called, wantSubjects) {
		t.Fatalf("subjects = %#v, want document-only %#v", loader.called, wantSubjects)
	}
	if len(result.Snapshot.Selected) != 1 || !reflect.DeepEqual(result.Snapshot.Selected[0].Subjects, wantSubjects) {
		t.Fatalf("selected = %#v, want the profile frozen against both document-only subjects", result.Snapshot.Selected)
	}
}

func TestSelectPinsOverlappingTrueProfilesTogether(t *testing.T) {
	profiles := []Profile{
		selectionProfile("p1", applicabilityTrueUS, `[]`),
		selectionProfile("p2", applicabilityTrueUS, `[]`),
	}
	selector := Selector{Source: &stubSelectionSource{storeID: 9, released: selectionFixture(profiles...)}}
	loader := &stubSubjectFactsLoader{facts: semrules.FactSet{}}
	result, err := selector.Select(context.Background(), SelectionRequest{
		ReviewScopeID:       "scope-t",
		ReviewedDocumentIDs: []int64{101},
		ReviewContext:       selectionReviewContext(),
		SubjectFacts:        loader,
	})
	if err != nil {
		t.Fatalf("Select: %v", err)
	}
	// Overlapping true profiles are intentionally pinned together, never
	// mutually exclusive (spec 2026080102 section 6).
	if len(result.Snapshot.Selected) != 2 {
		t.Fatalf("selected = %#v, want both overlapping true profiles pinned", result.Snapshot.Selected)
	}
	if len(result.SelectedProfiles) != 2 {
		t.Fatalf("selected identities = %#v, want both", result.SelectedProfiles)
	}
	if result.Snapshot.Selected[0].ProfileID != "p1" || result.Snapshot.Selected[1].ProfileID != "p2" {
		t.Fatalf("selected order = %#v, want deterministic p1,p2", result.Snapshot.Selected)
	}
}

func TestSelectRecordsFalseAndIndeterminateSubjectsWithTraces(t *testing.T) {
	profiles := []Profile{
		selectionProfile("p-true", applicabilityTrueUS, `[]`),
		selectionProfile("p-false", `{"version":1,"expression":{"kind":"fact","path":"review.jurisdiction","op":"eq","value":"CN"}}`, `[]`),
		selectionProfile("p-indet", applicabilityIndet, `[]`),
	}
	selector := Selector{Source: &stubSelectionSource{storeID: 9, released: selectionFixture(profiles...)}}
	loader := &stubSubjectFactsLoader{facts: semrules.FactSet{}}
	result, err := selector.Select(context.Background(), SelectionRequest{
		ReviewScopeID:       "scope-t",
		ReviewedDocumentIDs: []int64{101},
		ReviewContext:       selectionReviewContext(), // jurisdiction US
		SubjectFacts:        loader,
	})
	if err != nil {
		t.Fatalf("Select: %v", err)
	}
	byProfile := map[string]semrules.Truth{}
	for _, ev := range result.Snapshot.Evaluations {
		byProfile[ev.ProfileID] = ev.Outcome
	}
	if byProfile["p-true"] != semrules.TruthTrue || byProfile["p-false"] != semrules.TruthFalse || byProfile["p-indet"] != semrules.TruthIndeterminate {
		t.Fatalf("outcomes = %#v", byProfile)
	}
	// Only the true profile is frozen; the false/indeterminate subjects are
	// recorded with traces in the snapshot but not selected.
	if len(result.Snapshot.Selected) != 1 || result.Snapshot.Selected[0].ProfileID != "p-true" {
		t.Fatalf("selected = %#v, want only the true profile", result.Snapshot.Selected)
	}
	for _, ev := range result.Snapshot.Evaluations {
		if ev.Trace.Kind == "" {
			t.Fatalf("evaluation %+v has no trace", ev)
		}
		if ev.PredicateChecksum == "" {
			t.Fatalf("evaluation %+v has no predicate checksum", ev)
		}
	}
	if result.Snapshot.Status != SelectionStatusComplete {
		t.Fatalf("status = %q, want complete", result.Snapshot.Status)
	}
}

func TestSelectMarksScopeIndeterminateWhenProfileClosedDimensionsIntersectRequest(t *testing.T) {
	// p-indet is indeterminate on the subject (missing document.doc_kind) and
	// its closed dimension intersects the request's closed dimensions: the
	// scope is still selected but selection_status becomes indeterminate.
	selector := Selector{Source: &stubSelectionSource{storeID: 9, released: selectionFixture(selectionProfile("p-indet", applicabilityIndet, `["display_metrics"]`))}}
	result, err := selector.Select(context.Background(), SelectionRequest{
		ReviewScopeID:       "scope-t",
		ReviewedDocumentIDs: []int64{101},
		ClosedDimensions:    []string{"display_metrics"},
		ReviewContext:       selectionReviewContext(),
		SubjectFacts:        &stubSubjectFactsLoader{facts: semrules.FactSet{}},
	})
	if err != nil {
		t.Fatalf("Select: %v", err)
	}
	if result.SelectionStatus != SelectionStatusIndeterminate {
		t.Fatalf("selection_status = %q, want %q", result.SelectionStatus, SelectionStatusIndeterminate)
	}
	if result.Snapshot.Status != SelectionStatusIndeterminate {
		t.Fatalf("snapshot status = %q, want %q", result.Snapshot.Status, SelectionStatusIndeterminate)
	}
	// The indeterminate subject is still recorded with a trace.
	if len(result.Snapshot.Evaluations) != 1 || result.Snapshot.Evaluations[0].Outcome != semrules.TruthIndeterminate {
		t.Fatalf("evaluations = %#v", result.Snapshot.Evaluations)
	}
	// P5 review 2026080302 finding P5-9: a profile with zero true subjects
	// but a decision-relevant indeterminate one must still be pinned (not
	// silently absent from SelectedProfiles/scope.SelectedProfiles) so the
	// review step can produce an explicit indeterminate result -- spec
	// section 11's "continue review with explicit indeterminate
	// applicability results", not merely a scope-level status flag.
	if len(result.Snapshot.Selected) != 1 {
		t.Fatalf("selected = %#v, want the indeterminate-only profile pinned", result.Snapshot.Selected)
	}
	pinned := result.Snapshot.Selected[0]
	if pinned.ProfileID != "p-indet" || pinned.Outcome != semrules.TruthIndeterminate {
		t.Fatalf("pinned profile = %#v, want p-indet with Outcome=indeterminate", pinned)
	}
	if len(pinned.Subjects) != 1 || pinned.Subjects[0].DocumentID != 101 {
		t.Fatalf("pinned subjects = %#v, want the indeterminate subject recorded", pinned.Subjects)
	}
	if len(result.SelectedProfiles) != 1 || result.SelectedProfiles[0].ProfileID != "p-indet" {
		t.Fatalf("SelectedProfiles = %#v, want the indeterminate-only profile included (so its rules are loaded and evaluated)", result.SelectedProfiles)
	}
}

func TestSelectKeepsStatusCompleteWhenIndeterminateClosedDimensionsDoNotIntersect(t *testing.T) {
	// The indeterminate profile's closed dimension does not intersect the
	// request's: the evaluation is retained in the snapshot but does not make
	// the scope indeterminate and does not pin the profile.
	selector := Selector{Source: &stubSelectionSource{storeID: 9, released: selectionFixture(selectionProfile("p-indet", applicabilityIndet, `["dimension_x"]`))}}
	result, err := selector.Select(context.Background(), SelectionRequest{
		ReviewScopeID:       "scope-t",
		ReviewedDocumentIDs: []int64{101},
		ClosedDimensions:    []string{"display_metrics"},
		ReviewContext:       selectionReviewContext(),
		SubjectFacts:        &stubSubjectFactsLoader{facts: semrules.FactSet{}},
	})
	if err != nil {
		t.Fatalf("Select: %v", err)
	}
	if result.SelectionStatus != SelectionStatusComplete {
		t.Fatalf("selection_status = %q, want complete", result.SelectionStatus)
	}
	if len(result.Snapshot.Evaluations) != 1 {
		t.Fatalf("evaluations = %#v, want the indeterminate evaluation retained", result.Snapshot.Evaluations)
	}
	if len(result.Snapshot.Selected) != 0 {
		t.Fatalf("selected = %#v, want no profile pinned (no true subject)", result.Snapshot.Selected)
	}
}

func TestSelectSnapshotCarriesPinnedReleaseAndPredicateChecksums(t *testing.T) {
	profile := selectionProfile("p1", applicabilityTrueUS, `["display_metrics"]`)
	selector := Selector{Source: &stubSelectionSource{storeID: 9, released: selectionFixture(profile)}}
	result, err := selector.Select(context.Background(), SelectionRequest{
		ReviewScopeID:       "scope-t",
		ReviewedDocumentIDs: []int64{101},
		ReviewContext:       selectionReviewContext(),
		SubjectFacts:        &stubSubjectFactsLoader{facts: semrules.FactSet{}},
	})
	if err != nil {
		t.Fatalf("Select: %v", err)
	}
	entry := result.Snapshot.Selected[0]
	if entry.ReleaseChecksum != "sha256:aaa" {
		t.Fatalf("release_checksum = %q, want the pinned sha256:aaa", entry.ReleaseChecksum)
	}
	doc, err := parseApplicabilityDocument(string(profile.Applicability))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	_, wantChecksum, err := semrules.Canonicalize(doc)
	if err != nil {
		t.Fatalf("canonicalize: %v", err)
	}
	if entry.PredicateChecksum != wantChecksum {
		t.Fatalf("predicate_checksum = %q, want %q", entry.PredicateChecksum, wantChecksum)
	}
	if !reflect.DeepEqual(entry.ClosedDimensions, []string{"display_metrics"}) {
		t.Fatalf("closed_dimensions = %#v", entry.ClosedDimensions)
	}
	// The snapshot records every pinned release for reproducibility.
	if len(result.Snapshot.Releases) != 1 || result.Snapshot.Releases[0].ReleaseID != 42 {
		t.Fatalf("snapshot releases = %#v", result.Snapshot.Releases)
	}
	// Fact snapshot captures the merged fact set per subject.
	if len(result.FactSnapshot) != 1 || result.FactSnapshot[0].Subject.DocumentID != 101 {
		t.Fatalf("fact snapshot = %#v", result.FactSnapshot)
	}
	if result.FactSnapshot[0].Facts["review.jurisdiction"].State != semrules.FactKnown {
		t.Fatalf("fact snapshot review.jurisdiction = %+v, want the review context fact merged in", result.FactSnapshot[0].Facts["review.jurisdiction"])
	}
}

// TestSelectRaisesExactlyOneWarningPerIndeterminateScopeId proves spec
// 2026080102 section 11's "one warning deduplicated by scope id": an
// indeterminate deterministic scope raises exactly one selection warning
// carrying the review scope id as correlator, and a later complete scope
// raises none. The scope-id correlator is what the DB-level
// uq_alarms_errors_scope_id_kind index keys on, so repeated redeliveries of
// the same scope cannot create a second row.
func TestSelectRaisesExactlyOneWarningPerIndeterminateScopeId(t *testing.T) {
	alarms := &fakeSelectionAlarmWriter{}
	indetProfile := selectionProfile("p-indet", applicabilityIndet, `["display_metrics"]`)
	selector := Selector{Source: &stubSelectionSource{storeID: 9, released: selectionFixture(indetProfile)}, Alarms: alarms}

	result, err := selector.Select(context.Background(), SelectionRequest{
		ReviewScopeID:       "scope-indet",
		ReviewedDocumentIDs: []int64{101},
		ClosedDimensions:    []string{"display_metrics"},
		ReviewContext:       selectionReviewContext(),
		SubjectFacts:        &stubSubjectFactsLoader{facts: semrules.FactSet{}},
	})
	if err != nil {
		t.Fatalf("Select: %v", err)
	}
	if result.SelectionStatus != SelectionStatusIndeterminate {
		t.Fatalf("selection_status = %q, want indeterminate", result.SelectionStatus)
	}
	if len(alarms.written) != 1 || alarms.written[0].ScopeID != "scope-indet" || alarms.written[0].Kind != SelectionAlarmKindIndeterminate {
		t.Fatalf("written = %#v, want exactly one indeterminate warning for scope-indet", alarms.written)
	}

	// A complete scope raises no warning at all.
	trueProfile := selectionProfile("p-true", applicabilityTrueUS, `[]`)
	completeSelector := Selector{Source: &stubSelectionSource{storeID: 9, released: selectionFixture(trueProfile)}, Alarms: alarms}
	if _, err := completeSelector.Select(context.Background(), SelectionRequest{
		ReviewScopeID:       "scope-complete",
		ReviewedDocumentIDs: []int64{101},
		ClosedDimensions:    []string{"display_metrics"},
		ReviewContext:       selectionReviewContext(),
		SubjectFacts:        &stubSubjectFactsLoader{facts: semrules.FactSet{}},
	}); err != nil {
		t.Fatalf("Select: %v", err)
	}
	if len(alarms.written) != 1 {
		t.Fatalf("written = %#v, want still exactly one warning (the indeterminate scope only)", alarms.written)
	}
}

// TestSelectStillCreatesIndeterminateScopeWhenAlarmWriteFails proves spec
// 2026080102 section 11's "create the scope with selection_status=indeterminate,
// continue review, and raise one warning" even when the warning write fails: a
// best-effort warning must not abort scope creation, so Select returns the
// indeterminate SelectionResult with the failure surfaced via the logger
// rather than an error.
func TestSelectStillCreatesIndeterminateScopeWhenAlarmWriteFails(t *testing.T) {
	alarms := &fakeSelectionAlarmWriter{failFor: SelectionAlarmKindIndeterminate}
	var logged string
	logger := slog.New(slog.NewTextHandler(&captureWriter{buf: &logged}, nil))
	selector := Selector{
		Source: &stubSelectionSource{storeID: 9, released: selectionFixture(selectionProfile("p-indet", applicabilityIndet, `["display_metrics"]`))},
		Alarms: alarms,
		Logger: logger,
	}
	result, err := selector.Select(context.Background(), SelectionRequest{
		ReviewScopeID:       "scope-indet",
		ReviewedDocumentIDs: []int64{101},
		ClosedDimensions:    []string{"display_metrics"},
		ReviewContext:       selectionReviewContext(),
		SubjectFacts:        &stubSubjectFactsLoader{facts: semrules.FactSet{}},
	})
	if err != nil {
		t.Fatalf("Select must still succeed when the alarm write fails: %v", err)
	}
	if result.SelectionStatus != SelectionStatusIndeterminate {
		t.Fatalf("selection_status = %q, want indeterminate scope still created", result.SelectionStatus)
	}
	if !strings.Contains(logged, "selection alarm write failed") || !strings.Contains(logged, "scope-indet") {
		t.Fatalf("logger = %q, want the failed write surfaced via the logger", logged)
	}
}

// TestSelectDoesNotFailWhenAlarmWriterIsNil proves a nil alarm writer is a
// no-op: the scope is still created and executable even if no alarm surface is
// configured, and the indeterminate result is returned.
func TestSelectDoesNotFailWhenAlarmWriterIsNil(t *testing.T) {
	selector := Selector{Source: &stubSelectionSource{storeID: 9, released: selectionFixture(selectionProfile("p-indet", applicabilityIndet, `["display_metrics"]`))}}
	result, err := selector.Select(context.Background(), SelectionRequest{
		ReviewScopeID:       "scope-indet",
		ReviewedDocumentIDs: []int64{101},
		ClosedDimensions:    []string{"display_metrics"},
		ReviewContext:       selectionReviewContext(),
		SubjectFacts:        &stubSubjectFactsLoader{facts: semrules.FactSet{}},
	})
	if err != nil {
		t.Fatalf("Select with nil alarm writer: %v", err)
	}
	if result.SelectionStatus != SelectionStatusIndeterminate {
		t.Fatalf("selection_status = %q, want indeterminate", result.SelectionStatus)
	}
}

func TestSelectPropagatesMixedStoreRejection(t *testing.T) {
	selector := Selector{Source: &stubSelectionSource{storeID: 0, deriveErr: errors.New("reviewed documents resolve to multiple knowledge stores: cannot derive a single store for a deterministic scope")}}
	_, err := selector.Select(context.Background(), SelectionRequest{
		ReviewScopeID:       "scope-t",
		ReviewedDocumentIDs: []int64{101, 102},
		ReviewContext:       selectionReviewContext(),
		SubjectFacts:        &stubSubjectFactsLoader{facts: semrules.FactSet{}},
	})
	if err == nil {
		t.Fatal("Select must propagate mixed-store rejection")
	}
}

func TestSelectRequiresSubjectFactsLoaderAndDocumentIDs(t *testing.T) {
	selector := Selector{Source: &stubSelectionSource{storeID: 9, released: selectionFixture()}}
	if _, err := selector.Select(context.Background(), SelectionRequest{ReviewedDocumentIDs: []int64{101}}); err == nil {
		t.Fatal("Select without a subject facts loader must error")
	}
	if _, err := selector.Select(context.Background(), SelectionRequest{SubjectFacts: &stubSubjectFactsLoader{facts: semrules.FactSet{}}}); err == nil {
		t.Fatal("Select without reviewed document ids must error")
	}
}

func TestSelectRejectsDuplicateKnownFactProducers(t *testing.T) {
	// The subject facts loader returns a review.jurisdiction fact that the
	// review context also produces: the shared FactSetBuilder must reject the
	// duplicate known producer rather than silently overwriting one.
	loader := &stubSubjectFactsLoader{facts: semrules.FactSet{
		"review.jurisdiction": {Path: "review.jurisdiction", State: semrules.FactKnown, Value: "CN"},
	}}
	selector := Selector{Source: &stubSelectionSource{storeID: 9, released: selectionFixture(selectionProfile("p1", applicabilityTrueUS, `[]`))}}
	_, err := selector.Select(context.Background(), SelectionRequest{
		ReviewScopeID:       "scope-t",
		ReviewedDocumentIDs: []int64{101},
		ReviewContext:       selectionReviewContext(), // jurisdiction US
		SubjectFacts:        loader,
	})
	if err == nil {
		t.Fatal("Select must reject a duplicate known fact producer across fact sets")
	}
}

func TestSelectRejectsInvalidProfileApplicability(t *testing.T) {
	selector := Selector{Source: &stubSelectionSource{storeID: 9, released: selectionFixture(selectionProfile("p-bad", `{"version":1,"expression":{"kind":"bogus","path":"review.jurisdiction","op":"eq","value":"US"}}`, `[]`))}}
	_, err := selector.Select(context.Background(), SelectionRequest{
		ReviewScopeID:       "scope-t",
		ReviewedDocumentIDs: []int64{101},
		ReviewContext:       selectionReviewContext(),
		SubjectFacts:        &stubSubjectFactsLoader{facts: semrules.FactSet{}},
	})
	if err == nil {
		t.Fatal("Select must reject an invalid profile applicability predicate")
	}
}

// TestScopeCreationLeavesExplicitP4ScopesByteCompatible proves criterion 10's
// "explicit P4 scopes byte-compatible": P5 deterministic selection populates
// knowledge_store_id, selection_attempt_id, selection_status, fact_snapshot,
// and selection_snapshot on the SelectionResult, but an explicit P4 scope
// (SelectionMode=explicit, P5 fields left at zero) marshals to JSON without
// any P5 key at all (omitempty), making it byte-identical to a pre-P5 scope
// row. A P5 selection run cannot alter the serialized content of an explicit
// scope that was not produced from its result.
func TestScopeCreationLeavesExplicitP4ScopesByteCompatible(t *testing.T) {
	// Run P5 deterministic selection -- produces a result with every P5 field
	// populated (knowledge store, attempt id, status, fact/selection snapshots).
	selector := Selector{Source: &stubSelectionSource{storeID: 9, released: selectionFixture(selectionProfile("p1", applicabilityTrueUS, `[]`))}}
	p5Result, err := selector.Select(context.Background(), SelectionRequest{
		ReviewScopeID:       "scope-p5",
		ReviewedDocumentIDs: []int64{101},
		ReviewContext:       selectionReviewContext(),
		SubjectFacts:        &stubSubjectFactsLoader{facts: semrules.FactSet{}},
	})
	if err != nil {
		t.Fatalf("Select: %v", err)
	}
	if p5Result.KnowledgeStoreID == 0 || p5Result.SelectionAttemptID == "" || p5Result.SelectionStatus == "" {
		t.Fatal("P5 selection must populate all P5 fields on the result")
	}

	// An explicit P4 scope: SelectionMode=explicit, P5 fields at zero values.
	// This is what a pre-P5 caller or an explicit-mode handler creates -- the
	// P5 selection result above is never written into this scope.
	p4Scope := ReviewScope{
		ReviewScopeID:       "scope-p4",
		ReviewedDocumentIDs: json.RawMessage(`[101]`),
		TargetObjectIDs:     json.RawMessage(`[]`),
		TargetClassTermIDs:  json.RawMessage(`[]`),
		AsOfDate:            "2026-08-02",
		Jurisdiction:        "US",
		SelectedProfiles:    json.RawMessage(`[{"profile_id":"p1","profile_version":1,"release_id":42}]`),
		SelectionMode:       SelectionModeExplicit,
		PrecedencePolicy:    json.RawMessage(`{}`),
		ClosedDimensions:    json.RawMessage(`[]`),
		SelectedBy:          "reviewer",
		SelectionReason:     "explicit",
	}

	raw, err := json.Marshal(p4Scope)
	if err != nil {
		t.Fatalf("marshal P4 scope: %v", err)
	}
	// Every P5 column is omitempty and zero, so the serialized scope must not
	// carry any P5 key -- byte-identical to a pre-P5 scope row.
	for _, key := range []string{"knowledge_store_id", "selection_attempt_id", "selection_status", "fact_snapshot", "selection_snapshot"} {
		if strings.Contains(string(raw), `"`+key+`"`) {
			t.Fatalf("explicit P4 scope JSON must not contain P5 key %q (byte-compatible with pre-P5 rows), got: %s", key, raw)
		}
	}

	// Round-trip: unmarshal and verify P5 fields stay at zero values.
	var decoded ReviewScope
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.KnowledgeStoreID != 0 || decoded.SelectionAttemptID != "" || decoded.SelectionStatus != "" || len(decoded.FactSnapshot) != 0 || len(decoded.SelectionSnapshot) != 0 {
		t.Fatalf("explicit P4 scope must round-trip zero P5 fields, got %#v", decoded)
	}
	if decoded.SelectionMode != SelectionModeExplicit {
		t.Fatalf("selection_mode = %q, want %q", decoded.SelectionMode, SelectionModeExplicit)
	}
}
