package docprocessing

// The resolver contract between the reference layer and the artifact extractors (design D7/D8,
// devdoc §5). extract_references never writes an artifact table; each artifact family registers a
// ReferenceResolver and the same resolver serves both steps:
//
//	step 5  ResolveOutgoing  — extract_references, after storing a record's mentions
//	step 6  ResolveIncoming  — an artifact extractor, after saving a record's artifacts
//	        (and extract_references, when a record becomes "held" for other records' mentions)
//
// Each event commits its own writes before it looks for the other side, so when two documents are
// processed at the same time at least one of them sees the other. Apply replaces a mention's
// previous links and rows, so resolving twice never duplicates. Deleting a mention removes its
// links and rows by foreign-key cascade.

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/chendingplano/shared/go/api/ApiTypes"
)

// ArtifactCandidate is one target artifact a mention may mean.
type ArtifactCandidate struct {
	Table       string // table holding the artifact, e.g. "metrics" or "metrics_dropped"
	ArtifactID  string // positional id (metric_id, drop_id)
	ArtifactKey string // stable key, survives re-extraction
	RowID       int64  // primary key of the artifact row
	Score       float64
	Resolved    bool // chosen by the policy; the others are candidates for review
}

// ResolveOutcome is a family's decision for one mention.
type ResolveOutcome struct {
	State      string // family state, e.g. resolved | needs_review | no_match
	Notes      []string
	Candidates []ArtifactCandidate // ranked, best first
}

// ReferenceResolver is implemented by each artifact family (metric, provision, ...).
type ReferenceResolver interface {
	Family() string
	Takes(m RefMention) bool
	// HasArtifacts reports whether the record has been processed by the family's extractor.
	HasArtifacts(ctx context.Context, db *sql.DB, recordID int64) (bool, error)
	// Pending records that the mention waits for its target (not held, or not yet processed).
	Pending(ctx context.Context, db *sql.DB, m RefMention) error
	Resolve(ctx context.Context, db *sql.DB, m RefMention, targetRecordID int64) (ResolveOutcome, error)
	Apply(ctx context.Context, db *sql.DB, m RefMention, out ResolveOutcome) error
}

var (
	refResolversMu sync.RWMutex
	refResolvers   []ReferenceResolver
)

// RegisterReferenceResolver adds a family resolver; called from init by each family.
func RegisterReferenceResolver(r ReferenceResolver) {
	refResolversMu.Lock()
	defer refResolversMu.Unlock()
	refResolvers = append(refResolvers, r)
}

func registeredReferenceResolvers() []ReferenceResolver {
	refResolversMu.RLock()
	defer refResolversMu.RUnlock()
	return append([]ReferenceResolver(nil), refResolvers...)
}

func referenceResolverFor(family string) ReferenceResolver {
	for _, r := range registeredReferenceResolvers() {
		if r.Family() == family {
			return r
		}
	}
	return nil
}

// ResolveSummary counts outcomes of one resolution pass, per family and state.
type ResolveSummary map[string]map[string]int

func (s ResolveSummary) add(family, state string) {
	if s[family] == nil {
		s[family] = map[string]int{}
	}
	s[family][state]++
}

func (s ResolveSummary) merge(o ResolveSummary) {
	for fam, states := range o {
		for st, n := range states {
			if s[fam] == nil {
				s[fam] = map[string]int{}
			}
			s[fam][st] += n
		}
	}
}

// ResolveOutgoing is step 5: resolve the citing record's mentions for every registered family.
func ResolveOutgoing(ctx context.Context, db *sql.DB, citingRecordID int64, logger ApiTypes.JimoLogger) (ResolveSummary, error) {
	ms, err := loadRefMentions(ctx, db, `m.input_record_id = $1 AND NOT m.internal`, citingRecordID)
	if err != nil {
		return nil, err
	}
	sum := ResolveSummary{}
	for _, r := range registeredReferenceResolvers() {
		has := map[int64]bool{}
		for _, m := range ms {
			if !r.Takes(m) {
				continue
			}
			if m.R2State != refR2Held || m.TargetInputID == 0 {
				if err := r.Pending(ctx, db, m); err != nil {
					return sum, err
				}
				sum.add(r.Family(), "pending_target")
				continue
			}
			ok, seen := has[m.TargetInputID]
			if !seen {
				if ok, err = r.HasArtifacts(ctx, db, m.TargetInputID); err != nil {
					return sum, err
				}
				has[m.TargetInputID] = ok
			}
			if !ok {
				if err := r.Pending(ctx, db, m); err != nil {
					return sum, err
				}
				sum.add(r.Family(), "pending_target")
				continue
			}
			state, err := resolveAndApply(ctx, db, r, m, m.TargetInputID, logger)
			if err != nil {
				return sum, err
			}
			sum.add(r.Family(), state)
		}
	}
	return sum, nil
}

// ResolveIncoming is step 6 (and the holding event): resolve, with one family's resolver, every
// other record's mention whose holding is targetRecordID.
func ResolveIncoming(ctx context.Context, db *sql.DB, family string, targetRecordID int64, logger ApiTypes.JimoLogger) (ResolveSummary, error) {
	r := referenceResolverFor(family)
	sum := ResolveSummary{}
	if r == nil {
		return sum, fmt.Errorf("no reference resolver registered for family %q", family)
	}
	ms, err := loadRefMentions(ctx, db, `m.target_input_id = $1 AND m.r2_state = 'held' AND m.input_record_id <> $1 AND NOT m.internal`, targetRecordID)
	if err != nil {
		return sum, err
	}
	for _, m := range ms {
		if !r.Takes(m) {
			continue
		}
		state, err := resolveAndApply(ctx, db, r, m, targetRecordID, logger)
		if err != nil {
			return sum, err
		}
		sum.add(family, state)
	}
	return sum, nil
}

func resolveAndApply(ctx context.Context, db *sql.DB, r ReferenceResolver, m RefMention, target int64, logger ApiTypes.JimoLogger) (string, error) {
	out, err := r.Resolve(ctx, db, m, target)
	if err != nil {
		return "", fmt.Errorf("(MID_26101110) %s resolve %s: %w", r.Family(), m.RefID, err)
	}
	if err := r.Apply(ctx, db, m, out); err != nil {
		return "", fmt.Errorf("(MID_26101111) %s apply %s: %w", r.Family(), m.RefID, err)
	}
	if logger != nil && out.State != "resolved" {
		logger.Warn("reference not resolved with confidence", "family", r.Family(), "ref_id", m.RefID,
			"citing_record", m.InputRecordID, "target_record", target, "state", out.State,
			"candidates", len(out.Candidates), "notes", out.Notes)
	}
	return out.State, nil
}

// loadRefMentions reads the mentions matching a WHERE clause over kb.ref_mentions m ($1 bound to arg).
func loadRefMentions(ctx context.Context, db *sql.DB, where string, arg int64) ([]RefMention, error) {
	rows, err := db.QueryContext(ctx, `
SELECT m.id, m.input_record_id, m.ref_id, COALESCE(m.section, ''), COALESCE(m.source_line_spans, '[]'::jsonb),
       COALESCE(m.evidence, ''), m.target_content, m.target_scope, COALESCE(m.target_locator, ''),
       COALESCE(m.metric_name, ''), COALESCE(m.metric_name_en, ''), COALESCE(m.metric_subject, ''),
       COALESCE(m.metric_subject_en, ''), COALESCE(m.condition, ''), COALESCE(m.cited_raw, ''),
       COALESCE(m.r2_state, ''), COALESCE(m.target_input_id, 0), m.mention_key, m.internal
FROM kb.ref_mentions m WHERE `+where+` ORDER BY m.id`, arg)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []RefMention
	for rows.Next() {
		var m RefMention
		var spans []byte
		if err := rows.Scan(&m.ID, &m.InputRecordID, &m.RefID, &m.Section, &spans, &m.Evidence, &m.TargetContent,
			&m.TargetScope, &m.TargetLocator, &m.MetricName, &m.MetricNameEn, &m.MetricSubject, &m.MetricSubjectEn,
			&m.Condition, &m.CitedRaw, &m.R2State, &m.TargetInputID, &m.MentionKey, &m.Internal); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(spans, &m.SourceLineSpans)
		out = append(out, m)
	}
	return out, rows.Err()
}
