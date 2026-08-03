package profiles

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/chendingplano/deepdoc/server/api/ontology/semrules"
)

// Selection modes (kb.ontology_review_scopes.selection_mode). Explicit mode
// is preserved exactly (P4); the deterministic path is reached only when the
// request says deterministic_rule (spec 2026080102 section 6).
const (
	SelectionModeExplicit          = "explicit"
	SelectionModeDeterministicRule = "deterministic_rule"
)

// selection_status closed vocabulary (spec 2026080102 section 6/11).
const (
	SelectionStatusComplete      = "complete"
	SelectionStatusIndeterminate = "indeterminate"
)

// SelectionSubject is one applicability subject: each reviewed document paired
// with each requested target object/class, or a document-only subject when no
// target is supplied (spec 2026080102 section 6 item 3).
type SelectionSubject struct {
	DocumentID     int64  `json:"document_id"`
	TargetObjectID string `json:"target_object_id,omitempty"`
}

// SubjectFactsLoader supplies the per-subject facts (document facet
// observations, object classification, deployment context) that a profile's
// applicability predicate may reference. It is injected so the review-consumer
// package stays decoupled from doc-processing (controller guidance item 2);
// the kbhandler wiring provides the concrete loader using the doc-processing
// builders.
type SubjectFactsLoader interface {
	LoadSubjectFacts(context.Context, SelectionSubject) (semrules.FactSet, error)
}

// SelectionSource is the F1 pinned-load + store-derivation surface the
// selector needs, so its tests run without SQL.
type SelectionSource interface {
	DeriveKnowledgeStore(context.Context, []int64) (int64, error)
	LoadReleasedProfiles(context.Context) (ReleasedProfiles, error)
}

// SelectionRequest is the immutable deterministic-scope request. It must not
// supply selected_profiles: the selector derives the frozen selection from the
// released profiles (spec 2026080102 section 6: "must not supply
// selected_profiles"), and knowledge-store identity comes from
// kb.inputs.ks_store_id, never client input.
type SelectionRequest struct {
	ReviewScopeID       string
	ReviewedDocumentIDs []int64
	TargetObjectIDs     []string
	ReviewContext       ReviewApplicabilityContext
	ClosedDimensions    []string
	SelectedBy          string
	SelectionReason     string
	SubjectFacts        SubjectFactsLoader
}

// SelectedIdentity is one frozen profile/release identity, the scope's
// selected_profiles payload that EvaluatePinnedScope already unmarshals.
type SelectedIdentity struct {
	ProfileID      string `json:"profile_id"`
	ProfileVersion int    `json:"profile_version"`
	ReleaseID      int64  `json:"release_id"`
}

// SelectionResult is the frozen deterministic selection, kept strongly typed
// so the handler can marshal the JSON snapshots into the P5 scope columns and
// the scope is reproducible from the scope record alone (spec 2026080102
// section 6).
type SelectionResult struct {
	KnowledgeStoreID   int64
	SelectionAttemptID string
	SelectionStatus    string
	SelectedProfiles   []SelectedIdentity
	FactSnapshot       []SubjectFactSnapshot
	Snapshot           SelectionSnapshot
}

// SelectionAlarmWriter persists one automatic-selection warning. It is
// injected so the selector can be tested with an in-memory writer; the
// production implementation writes to alarms_errors with scope_id + a stable
// kind so the warning is deduplicated by scope id (controller guidance item 3).
type SelectionAlarmWriter interface {
	WriteSelectionAlarm(context.Context, SelectionAlarm) error
}

// SelectionLogger receives the best-effort alarm-write failure. Both the
// standard library's *slog.Logger and shared's ApiTypes.JimoLogger satisfy it,
// so the handler can inject its request logger with no adapter code.
type SelectionLogger interface {
	Error(message string, args ...any)
}

// Selector performs deterministic review-profile selection. Source supplies
// the pinned released profiles and knowledge-store derivation; Alarms (may be
// nil) raises the one-per-scope indeterminacy warning; Logger (may be nil)
// receives the write failure when the warning cannot be persisted, because
// that must never abort the scope's creation (spec 2026080102 section 11:
// "create the scope with selection_status=indeterminate, continue review, and
// raise one warning deduplicated by scope id" -- an alarms-table outage must
// not stop document/review processing, matching the E3 routing-alarm
// precedent).
type Selector struct {
	Source SelectionSource
	Alarms SelectionAlarmWriter
	Logger SelectionLogger
}

// SelectionSnapshot is the immutable selection record persisted as
// selection_snapshot. Releases are the releases pinned at attempt time;
// Selected holds every frozen profile/release with its applicable subjects;
// Evaluations records every per-subject profile outcome (including false and
// indeterminate) with its trace (spec 2026080102 section 6 items 4/5).
type SelectionSnapshot struct {
	Releases    []SnapshotRelease   `json:"releases"`
	Selected    []SelectedProfile   `json:"selected"`
	Evaluations []ProfileEvaluation `json:"evaluations"`
	Status      string              `json:"status"`
}

// SnapshotRelease is one release pinned at selection time (spec section 6 item
// 2), retained as an attempt input so a later activation change cannot
// partially change the selection.
type SnapshotRelease struct {
	ModuleID  string `json:"module_id"`
	ReleaseID int64  `json:"release_id"`
	Version   string `json:"version"`
	Checksum  string `json:"content_checksum"`
}

// SelectedProfile is one frozen profile/release with the exact subjects to
// which it applies (spec section 6 item 4). ClosedDimensions is the profile's
// own closed dimension set, needed later to decide rule-level decision
// relevance (spec section 6 last paragraph).
type SelectedProfile struct {
	ProfileID         string             `json:"profile_id"`
	ProfileVersion    int                `json:"profile_version"`
	ReleaseID         int64              `json:"release_id"`
	ReleaseChecksum   string             `json:"release_checksum"`
	ClosedDimensions  []string           `json:"closed_dimensions"`
	Subjects          []SelectionSubject `json:"subjects"`
	PredicateChecksum string             `json:"predicate_checksum"`
	Outcome           semrules.Truth     `json:"outcome"`
	Trace             semrules.TraceNode `json:"trace"`
}

// ProfileEvaluation is one per-subject profile applicability outcome recorded
// in the snapshot with its structured trace (spec section 6 item 5).
type ProfileEvaluation struct {
	ProfileID         string             `json:"profile_id"`
	ProfileVersion    int                `json:"profile_version"`
	ReleaseID         int64              `json:"release_id"`
	PredicateChecksum string             `json:"predicate_checksum"`
	Subject           SelectionSubject   `json:"subject"`
	Outcome           semrules.Truth     `json:"outcome"`
	Trace             semrules.TraceNode `json:"trace"`
}

// SubjectFactSnapshot is the per-subject merged fact set (review context plus
// subject facts) captured in fact_snapshot so the selection decision is
// reproducible from the scope record alone.
type SubjectFactSnapshot struct {
	Subject SelectionSubject `json:"subject"`
	Facts   semrules.FactSet `json:"facts"`
}

// Select derives the knowledge store, pins the active releases and released
// profiles, evaluates each profile once per subject, freezes every profile
// with at least one true subject, records false/indeterminate subjects with
// traces, marks the scope indeterminate when an indeterminate candidate
// profile's closed dimensions intersect the request's, and raises exactly one
// warning deduplicated by scope id when indeterminate (spec sections 6 and 11).
func (s Selector) Select(ctx context.Context, req SelectionRequest) (SelectionResult, error) {
	if s.Source == nil {
		return SelectionResult{}, errors.New("selection source is required")
	}
	if strings.TrimSpace(req.ReviewScopeID) == "" {
		return SelectionResult{}, errors.New("review_scope_id is required")
	}
	if len(req.ReviewedDocumentIDs) == 0 {
		return SelectionResult{}, errors.New("reviewed document ids are required")
	}
	if req.SubjectFacts == nil {
		return SelectionResult{}, errors.New("subject facts loader is required")
	}

	// Knowledge-store identity comes from kb.inputs.ks_store_id; mixed or
	// unresolvable document sets are rejected by DeriveKnowledgeStore.
	storeID, err := s.Source.DeriveKnowledgeStore(ctx, req.ReviewedDocumentIDs)
	if err != nil {
		return SelectionResult{}, err
	}
	// Pin active module releases and load only profiles visible through them,
	// in one short repeatable-read transaction ended before any classifier call.
	released, err := s.Source.LoadReleasedProfiles(ctx)
	if err != nil {
		return SelectionResult{}, err
	}

	reviewFacts, err := BuildReviewContextFacts(req.ReviewContext)
	if err != nil {
		return SelectionResult{}, err
	}

	attemptID, err := newSelectionAttemptID()
	if err != nil {
		return SelectionResult{}, err
	}

	snapshot := SelectionSnapshot{Status: SelectionStatusComplete}
	releaseChecksums := make(map[int64]string, len(released.Releases))
	for _, rel := range released.Releases {
		snapshot.Releases = append(snapshot.Releases, SnapshotRelease{
			ModuleID: rel.ModuleID, ReleaseID: rel.ReleaseID, Version: rel.Version, Checksum: rel.Checksum,
		})
		releaseChecksums[rel.ReleaseID] = rel.Checksum
	}

	subjects := buildSubjects(req.ReviewedDocumentIDs, req.TargetObjectIDs)
	requestClosed := toSet(req.ClosedDimensions)

	indeterminate := false
	factSnapshots := make([]SubjectFactSnapshot, 0, len(subjects))
	for _, subject := range subjects {
		subjectFacts, err := req.SubjectFacts.LoadSubjectFacts(ctx, subject)
		if err != nil {
			return SelectionResult{}, fmt.Errorf("subject facts for document %d: %w", subject.DocumentID, err)
		}
		merged, err := mergeFactSets(reviewFacts, subjectFacts)
		if err != nil {
			return SelectionResult{}, fmt.Errorf("subject facts for document %d: %w", subject.DocumentID, err)
		}
		factSnapshots = append(factSnapshots, SubjectFactSnapshot{Subject: subject, Facts: merged})
	}

	for _, profile := range released.Profiles {
		doc, checksum, profileClosed, err := profileApplicability(profile)
		if err != nil {
			return SelectionResult{}, fmt.Errorf("profile %s v%d: %w", profile.ProfileID, profile.Version, err)
		}
		intersects := hasIntersection(toSet(profileClosed), requestClosed)

		var subjectsTrue []SelectionSubject
		var trueTrace semrules.TraceNode
		for subjectIdx, subject := range subjects {
			merged := factSnapshots[subjectIdx].Facts
			res, err := semrules.EvaluateDocumentValidated(doc, merged)
			if err != nil {
				return SelectionResult{}, fmt.Errorf("profile %s v%d: %w", profile.ProfileID, profile.Version, err)
			}
			snapshot.Evaluations = append(snapshot.Evaluations, ProfileEvaluation{
				ProfileID:         profile.ProfileID,
				ProfileVersion:    profile.Version,
				ReleaseID:         profile.ReleaseID,
				PredicateChecksum: checksum,
				Subject:           subject,
				Outcome:           res.Truth,
				Trace:             res.TraceTree,
			})
			if res.Truth == semrules.TruthTrue {
				subjectsTrue = append(subjectsTrue, subject)
				trueTrace = res.TraceTree
			}
			if res.Truth == semrules.TruthIndeterminate && intersects {
				indeterminate = true
			}
		}
		if len(subjectsTrue) > 0 {
			snapshot.Selected = append(snapshot.Selected, SelectedProfile{
				ProfileID:         profile.ProfileID,
				ProfileVersion:    profile.Version,
				ReleaseID:         profile.ReleaseID,
				ReleaseChecksum:   releaseChecksums[profile.ReleaseID],
				ClosedDimensions:  profileClosed,
				Subjects:          subjectsTrue,
				PredicateChecksum: checksum,
				Outcome:           semrules.TruthTrue,
				Trace:             trueTrace,
			})
		}
	}
	if indeterminate {
		snapshot.Status = SelectionStatusIndeterminate
	}

	result := SelectionResult{
		KnowledgeStoreID:   storeID,
		SelectionAttemptID: attemptID,
		SelectionStatus:    snapshot.Status,
		SelectedProfiles:   selectedIdentities(snapshot.Selected),
		FactSnapshot:       factSnapshots,
		Snapshot:           snapshot,
	}

	if indeterminate && s.Alarms != nil {
		alarm := SelectionAlarm{
			Kind:     SelectionAlarmKindIndeterminate,
			Severity: SelectionAlarmSeverityWarning,
			Message:  "automatic review profile selection is indeterminate on a requested closed dimension; review continues with explicit indeterminate applicability results",
			ScopeID:  req.ReviewScopeID,
		}
		if err := s.Alarms.WriteSelectionAlarm(ctx, alarm); err != nil {
			// The warning is best-effort: a failed write must not abort the
			// scope's creation (spec section 11 -- the scope is still created
			// with selection_status=indeterminate and executable, and the
			// warning is deduplicated by scope id). Log and continue.
			if s.Logger != nil {
				s.Logger.Error("selection alarm write failed", "scope_id", alarm.ScopeID, "kind", alarm.Kind, "err", err)
			}
		}
	}
	return result, nil
}

// buildSubjects returns each reviewed document paired with each requested
// target object/class, or a document-only subject when no target is supplied
// (spec section 6 item 3). The order is deterministic: documents in request
// order, targets in request order.
func buildSubjects(documentIDs []int64, targetObjectIDs []string) []SelectionSubject {
	if len(targetObjectIDs) == 0 {
		subjects := make([]SelectionSubject, 0, len(documentIDs))
		for _, id := range documentIDs {
			subjects = append(subjects, SelectionSubject{DocumentID: id})
		}
		return subjects
	}
	subjects := make([]SelectionSubject, 0, len(documentIDs)*len(targetObjectIDs))
	for _, id := range documentIDs {
		for _, target := range targetObjectIDs {
			subjects = append(subjects, SelectionSubject{DocumentID: id, TargetObjectID: target})
		}
	}
	return subjects
}

// profileApplicability parses and canonicalizes a profile's applicability
// predicate and its closed dimensions. A profile without a predicate (empty or
// `{}`) is unconditional: it applies to every subject and evaluates to true
// against any fact set.
func profileApplicability(profile Profile) (doc semrules.Document, checksum string, closedDimensions []string, err error) {
	if len(profile.ClosedDimensions) > 0 {
		if err := json.Unmarshal(profile.ClosedDimensions, &closedDimensions); err != nil {
			return doc, "", nil, fmt.Errorf("closed_dimensions: %w", err)
		}
	}
	if isTrivialPredicate(profile.Applicability) {
		return unconditionalDocument(), "", closedDimensions, nil
	}
	if err := json.Unmarshal(profile.Applicability, &doc); err != nil {
		return doc, "", nil, fmt.Errorf("applicability: %w", err)
	}
	_, sum, err := semrules.Canonicalize(doc)
	if err != nil {
		return doc, "", nil, fmt.Errorf("applicability: %w", err)
	}
	return doc, sum, closedDimensions, nil
}

// unconditionalDocument is the trivially true predicate used for a profile
// with no applicability predicate: an empty `all` evaluates to true against
// any fact set (semrules.evaluatePredicate).
func unconditionalDocument() semrules.Document {
	return semrules.Document{Version: 1, Expression: semrules.Predicate{Kind: "all"}}
}

func isTrivialPredicate(raw json.RawMessage) bool {
	trimmed := strings.TrimSpace(string(raw))
	return trimmed == "" || trimmed == "{}" || trimmed == "null"
}

// selectedIdentities is the scope's selected_profiles payload: profile/version
// plus the pinned release id, in deterministic pinned order. This is the shape
// EvaluatePinnedScope already unmarshals.
func selectedIdentities(selected []SelectedProfile) []SelectedIdentity {
	identities := make([]SelectedIdentity, 0, len(selected))
	for _, entry := range selected {
		identities = append(identities, SelectedIdentity{
			ProfileID:      entry.ProfileID,
			ProfileVersion: entry.ProfileVersion,
			ReleaseID:      entry.ReleaseID,
		})
	}
	return identities
}

// mergeFactSets merges the review context facts with the per-subject facts,
// rejecting duplicate known producers so one source can never silently
// overwrite another (spec section 3).
func mergeFactSets(reviewFacts, subjectFacts semrules.FactSet) (semrules.FactSet, error) {
	builder := semrules.NewFactSetBuilder()
	if err := builder.AddSet(reviewFacts); err != nil {
		return nil, err
	}
	if err := builder.AddSet(subjectFacts); err != nil {
		return nil, err
	}
	return builder.Build(), nil
}

func toSet(values []string) map[string]bool {
	set := make(map[string]bool, len(values))
	for _, value := range values {
		if value != "" {
			set[value] = true
		}
	}
	return set
}

func hasIntersection(a, b map[string]bool) bool {
	for value := range a {
		if b[value] {
			return true
		}
	}
	return false
}

// newSelectionAttemptID generates a stable review-selection attempt id,
// mirroring doc-processing's event-id generation (a random 12-byte token
// hex-encoded under a prefix) so concurrent selections are distinguishable.
func newSelectionAttemptID() (string, error) {
	buf := make([]byte, 12)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate selection attempt id: %w", err)
	}
	return "sel-" + hex.EncodeToString(buf), nil
}
