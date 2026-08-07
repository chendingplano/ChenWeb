package keywords

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/chendingplano/deepdoc/server/api/ontology/semid"
)

// DBX is the subset of database/sql that keyword stores need. Both
// *sql.DB and *sql.Tx satisfy it.
type DBX interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// Concept is one keyword concept row. ConceptID is opaque and immutable;
// PrefLabel is mutable display. Concepts are ungoverned — status transitions
// are linear (active → provisional → merged → deprecated) with no draft/review.
type Concept struct {
	ConceptID   string    `json:"concept_id"`
	PrefLabel   string    `json:"pref_label"`
	Gloss       *string   `json:"gloss"`
	Scope       string    `json:"scope"`
	Status      string    `json:"status"`
	MergedInto  *string   `json:"merged_into"`
	GlossSource string    `json:"gloss_source"`
	CreateTime  time.Time `json:"create_time"`
	ModifyTime  time.Time `json:"modify_time"`
}

// AllowedConceptStatuses is the ungoverned keyword-concept lifecycle.
var AllowedConceptStatuses = map[string]bool{
	"active":      true,
	"provisional": true,
	"merged":      true,
	"deprecated":  true,
}

var errIllegalConceptTransition = errors.New("illegal concept status transition")

// ErrMergeRejected signals a merge/un-merge refused by a guardrail (K8/D7,
// §14): self-merge, a non-live participant, an already-merged source, or a
// persisted never_merge assertion. Callers surface it as a conflict, not a
// server failure.
var ErrMergeRejected = errors.New("merge rejected")

// conceptStatusTransitions lists allowed status changes for keyword concepts.
// Ungoverned: provisional can go back to active; merged sets merged_into.
var conceptStatusTransitions = map[string]map[string]bool{
	"active":      {"provisional": true, "merged": true, "deprecated": true},
	"provisional": {"active": true, "merged": true, "deprecated": true},
	"merged":      {},
	"deprecated":  {},
}

// ConceptStore persists keyword concept rows.
type ConceptStore struct {
	DB DBX
	// Alignments, when wired, makes the §14.2 merge gate live: MergeConcept
	// refuses a merge where both sides carry accepted aligns_to_term
	// assertions to different governed terms, and re-points an alignment on
	// the absorbed side to the survivor inside the merge transaction. Left
	// zero-valued, the gate is skipped (every existing caller stays
	// unchanged; the gate is live only where the field is wired).
	Alignments AlignmentsStore
}

const conceptColumns = `concept_id, pref_label, gloss, scope, status, merged_into, gloss_source, create_time, modify_time`

const conceptFrom = `FROM kb.keyword_concepts`

func scanConcept(scan func(dest ...any) error) (Concept, error) {
	var (
		c          Concept
		gloss      sql.NullString
		mergedInto sql.NullString
	)
	if err := scan(
		&c.ConceptID, &c.PrefLabel, &gloss, &c.Scope, &c.Status,
		&mergedInto, &c.GlossSource, &c.CreateTime, &c.ModifyTime,
	); err != nil {
		return Concept{}, err
	}
	if gloss.Valid {
		c.Gloss = &gloss.String
	}
	if mergedInto.Valid {
		v := mergedInto.String
		c.MergedInto = &v
	}
	return c, nil
}

func validateConcept(c Concept) error {
	if c.ConceptID == "" {
		return fmt.Errorf("concept_id is required")
	}
	if c.PrefLabel == "" {
		return fmt.Errorf("pref_label is required")
	}
	if c.Scope == "" {
		return fmt.Errorf("scope is required")
	}
	if !AllowedConceptStatuses[c.Status] {
		return fmt.Errorf("invalid status: %s", c.Status)
	}
	return nil
}

func withKeywordIdentityMutation(ctx context.Context, db DBX, write func(DBX) error) error {
	if db == nil {
		return errors.New("db is nil")
	}
	if tx, ok := db.(*sql.Tx); ok {
		if err := semid.AcquireKeywordIdentityMutationLock(ctx, tx); err != nil {
			return err
		}
		return write(tx)
	}
	beginner, ok := db.(txBeginner)
	if !ok {
		return errors.New("keyword identity mutation requires transaction-capable database")
	}
	tx, err := beginner.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := semid.AcquireKeywordIdentityMutationLock(ctx, tx); err != nil {
		return err
	}
	if err := write(tx); err != nil {
		return err
	}
	return tx.Commit()
}

// CreateConcept inserts a new keyword concept.
func (s ConceptStore) CreateConcept(ctx context.Context, c Concept) (Concept, error) {
	if c.Status == "" {
		c.Status = "active"
	}
	if c.Scope == "" {
		c.Scope = "_"
	}
	if c.GlossSource == "" {
		c.GlossSource = "none"
	}
	if err := validateConcept(c); err != nil {
		return Concept{}, err
	}
	var created Concept
	err := withKeywordIdentityMutation(ctx, s.DB, func(db DBX) error {
		row := db.QueryRowContext(ctx, `
			INSERT INTO kb.keyword_concepts (concept_id, pref_label, gloss, scope, status, gloss_source)
			VALUES ($1, $2, $3, $4, $5, $6)
			RETURNING `+conceptColumns,
			c.ConceptID, c.PrefLabel, nullableString(func() string {
				if c.Gloss != nil {
					return *c.Gloss
				}
				return ""
			}()), c.Scope, c.Status, c.GlossSource,
		)
		var err error
		created, err = scanConcept(row.Scan)
		return err
	})
	return created, err
}

// GetConcept retrieves a concept by its immutable concept_id.
func (s ConceptStore) GetConcept(ctx context.Context, conceptID string) (Concept, error) {
	row := s.DB.QueryRowContext(ctx, `
		SELECT `+conceptColumns+`
		`+conceptFrom+`
		WHERE concept_id = $1`, conceptID)
	return scanConcept(row.Scan)
}

// ListConcepts returns concepts, optionally filtered by scope. When scope is
// empty, all active and provisional concepts are returned.
func (s ConceptStore) ListConcepts(ctx context.Context, scope string) ([]Concept, error) {
	var (
		rows *sql.Rows
		err  error
	)
	if scope == "" {
		rows, err = s.DB.QueryContext(ctx, `
			SELECT `+conceptColumns+`
			`+conceptFrom+`
			WHERE status IN ('active', 'provisional')
			ORDER BY concept_id`)
	} else {
		rows, err = s.DB.QueryContext(ctx, `
			SELECT `+conceptColumns+`
			`+conceptFrom+`
			WHERE scope = $1 AND status IN ('active', 'provisional')
			ORDER BY concept_id`, scope)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Concept
	for rows.Next() {
		c, err := scanConcept(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// ListAutoCreatedProvisional returns concepts D11's targeted-miss path
// minted (status='provisional', gloss_source='auto:d11') -- the population
// keywords.Reconciler scans for a merge target, per spec §14.3: this
// population is the only one automatic reconciliation merges (a fresh
// provisional concept has no curated content to lose).
func (s ConceptStore) ListAutoCreatedProvisional(ctx context.Context, scope string, limit int) ([]Concept, error) {
	if limit <= 0 {
		limit = 500
	}
	rows, err := s.DB.QueryContext(ctx, `
		SELECT `+conceptColumns+`
		`+conceptFrom+`
		WHERE scope = $1 AND status = 'provisional' AND gloss_source = 'auto:d11'
		ORDER BY create_time
		LIMIT $2`, scope, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Concept
	for rows.Next() {
		c, err := scanConcept(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// SearchSimilarPrefLabel is reconciliation's lexical blocking query (spec
// §13 R3: "lexical (pg_trgm) ... to k candidates"), backed by migration
// 20260806000001's trigram index on pref_label. excludeConceptID keeps a
// concept from matching itself.
func (s ConceptStore) SearchSimilarPrefLabel(ctx context.Context, label, scope, excludeConceptID string, minSimilarity float64, limit int) ([]Concept, error) {
	if limit <= 0 {
		limit = 10
	}
	rows, err := s.DB.QueryContext(ctx, `
		SELECT `+conceptColumns+`
		`+conceptFrom+`
		WHERE scope = $1 AND status IN ('active', 'provisional')
		  AND concept_id != $4
		  AND similarity(pref_label, $2) > $3
		ORDER BY similarity(pref_label, $2) DESC
		LIMIT $5`, scope, label, minSimilarity, excludeConceptID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Concept
	for rows.Next() {
		c, err := scanConcept(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// UpdateConceptLabel updates the display attributes of a concept.
func (s ConceptStore) UpdateConceptLabel(ctx context.Context, conceptID, prefLabel, gloss string) (Concept, error) {
	if prefLabel == "" {
		return Concept{}, fmt.Errorf("pref_label is required")
	}
	var updated Concept
	err := withKeywordIdentityMutation(ctx, s.DB, func(db DBX) error {
		row := db.QueryRowContext(ctx, `
			UPDATE kb.keyword_concepts
			SET pref_label = $2, gloss = $3, modify_time = NOW()
			WHERE concept_id = $1
			RETURNING `+conceptColumns,
			conceptID, prefLabel, nullableString(gloss))
		var err error
		updated, err = scanConcept(row.Scan)
		return err
	})
	return updated, err
}

// TransitionStatus enforces the concept status state machine.
func (s ConceptStore) TransitionStatus(ctx context.Context, conceptID, to string) (Concept, error) {
	if !AllowedConceptStatuses[to] {
		return Concept{}, fmt.Errorf("invalid status: %s", to)
	}

	var result Concept
	err := withKeywordIdentityMutation(ctx, s.DB, func(db DBX) error {
		current, err := (ConceptStore{DB: db}).GetConcept(ctx, conceptID)
		if err != nil {
			return err
		}
		if to == current.Status {
			result = current
			return nil
		}
		allowed, ok := conceptStatusTransitions[current.Status]
		if !ok || !allowed[to] {
			return fmt.Errorf("%w: %s → %s", errIllegalConceptTransition, current.Status, to)
		}
		row := db.QueryRowContext(ctx, `
			UPDATE kb.keyword_concepts
			SET status = $2, modify_time = NOW()
			WHERE concept_id = $1
			RETURNING `+conceptColumns,
			conceptID, to)
		result, err = scanConcept(row.Scan)
		return err
	})
	return result, err
}

// MergeConcept tombstones fromID into toID (D7, §14). The MergeGraph
// guardrails are enforced here, backed by persisted state: both concepts
// must exist, only live concepts (active/provisional) participate, an
// already-merged source is refused (unmerge first), and the persisted
// never_merge assertion (kb.semid_never_merge via semid.NeverMergeStore) is
// consulted.
//
// Per §14.1 the merge re-points surfaces in the same transaction as the
// tombstone — UPDATE kb.keyword_surfaces SET concept_id = to WHERE
// concept_id = from — recording origin_concept = from on each moved surface
// (COALESCE preserves the deepest origin across chained merges) so the merge
// stays reversible (§14.4). Without the re-point, every surface would still
// carry the tombstone id and every resolve would return it.
//
// §14.2: when the Alignments gate is wired (Alignments.Assertions.DB
// non-nil), this transaction also refuses a merge where both concepts carry
// accepted aligns_to_term assertions to different governed terms — evidence
// they are not the same thing outranks whatever similarity proposed the
// merge — and re-points an accepted alignment on the absorbed side to the
// survivor atomically with the tombstone. Left unwired, the gate is skipped.
// §14.3: this endpoint performs human-initiated merges only. Automatic
// (reconciliation) merges are additionally restricted to auto-created
// provisional concepts; that policy lives with the reconciler, not here.
func (s ConceptStore) MergeConcept(ctx context.Context, fromID, toID string) (Concept, error) {
	if fromID == "" || toID == "" {
		return Concept{}, fmt.Errorf("%w: merge requires two concept ids", ErrMergeRejected)
	}
	if fromID == toID {
		return Concept{}, fmt.Errorf("%w: cannot merge a concept into itself", ErrMergeRejected)
	}
	if tx, ok := s.DB.(*sql.Tx); ok {
		if err := semid.AcquireKeywordIdentityMutationLock(ctx, tx); err != nil {
			return Concept{}, err
		}
		if err := s.MergeConceptTx(ctx, tx, fromID, toID); err != nil {
			return Concept{}, err
		}
		return (ConceptStore{DB: tx}).GetConcept(ctx, toID)
	}

	// F10: the guardrail reads run inside the same transaction as the write,
	// with FOR UPDATE holding a row lock on both concepts until commit —
	// closing the window between the check and applyMerge where a
	// concurrent status change, competing merge, or never_merge assertion
	// could invalidate a guardrail that already appeared to pass.
	if beginner, ok := s.DB.(txBeginner); ok {
		tx, err := beginner.BeginTx(ctx, nil)
		if err != nil {
			return Concept{}, err
		}
		if err := semid.AcquireKeywordIdentityMutationLock(ctx, tx); err != nil {
			_ = tx.Rollback()
			return Concept{}, err
		}
		if err := s.MergeConceptTx(ctx, tx, fromID, toID); err != nil {
			_ = tx.Rollback()
			return Concept{}, err
		}
		if err := tx.Commit(); err != nil {
			return Concept{}, err
		}
		return s.GetConcept(ctx, toID)
	}

	return Concept{}, errors.New("keyword concept merge requires transaction-capable database")
}

// MergeConceptTx applies the supplied fromID -> toID merge within a
// caller-owned transaction. It acquires the keyword identity mutation lock;
// the caller retains commit or rollback ownership so it can append the
// corresponding decision audit atomically.
func (s ConceptStore) MergeConceptTx(ctx context.Context, tx *sql.Tx, fromID, toID string) error {
	if tx == nil {
		return errors.New("tx is nil")
	}
	if err := semid.AcquireKeywordIdentityMutationLock(ctx, tx); err != nil {
		return err
	}
	if fromID == "" || toID == "" {
		return fmt.Errorf("%w: merge requires two concept ids", ErrMergeRejected)
	}
	if fromID == toID {
		return fmt.Errorf("%w: cannot merge a concept into itself", ErrMergeRejected)
	}
	if err := s.mergeGuards(ctx, tx, fromID, toID); err != nil {
		return err
	}
	if s.Alignments.Assertions.DB != nil {
		conflict, err := s.Alignments.MergeConflict(ctx, tx, fromID, toID)
		if err != nil {
			return err
		}
		if conflict {
			return fmt.Errorf("%w: concepts %s and %s are aligned to different governed terms", ErrMergeRejected, fromID, toID)
		}
		if err := s.Alignments.FollowMerge(ctx, tx, fromID, toID); err != nil {
			return err
		}
	}
	return applyMerge(ctx, tx, fromID, toID)
}

// mergeGuards enforces D7's guardrails: both concepts must exist, only live
// concepts (active/provisional) participate, an already-merged source is
// refused (unmerge first), and the persisted never_merge assertion
// (kb.semid_never_merge via semid.NeverMergeStore) is consulted. Reads go
// through getConceptForUpdate so a caller passing a transaction handle
// holds the row lock through to applyMerge (F10).
func (s ConceptStore) mergeGuards(ctx context.Context, db DBX, fromID, toID string) error {
	from, err := getConceptForUpdate(ctx, db, fromID)
	if err != nil {
		return fmt.Errorf("source concept %s: %w", fromID, err)
	}
	to, err := getConceptForUpdate(ctx, db, toID)
	if err != nil {
		return fmt.Errorf("target concept %s: %w", toID, err)
	}

	// Only live concepts may merge, and only into a live target: re-pointing
	// surfaces at a tombstone would leave the surface path returning it.
	if from.Status != "active" && from.Status != "provisional" {
		return fmt.Errorf("%w: source %s is %s (already-merged concepts must be unmerged first)", ErrMergeRejected, fromID, from.Status)
	}
	if from.MergedInto != nil {
		return fmt.Errorf("%w: source %s already merged into %s; unmerge first", ErrMergeRejected, fromID, *from.MergedInto)
	}
	if to.Status != "active" && to.Status != "provisional" {
		return fmt.Errorf("%w: target %s is %s", ErrMergeRejected, toID, to.Status)
	}

	if blocked, err := isNeverMerge(ctx, db, fromID, toID); err != nil {
		return err
	} else if blocked {
		return fmt.Errorf("%w: %s and %s are asserted never_merge", ErrMergeRejected, fromID, toID)
	}
	return nil
}

// getConceptForUpdate reads a concept, locking the row (FOR UPDATE) when db
// is a transaction handle — a plain *sql.DB takes no lock, since there is
// no enclosing transaction to hold it in.
func getConceptForUpdate(ctx context.Context, db DBX, conceptID string) (Concept, error) {
	row := db.QueryRowContext(ctx, `
		SELECT `+conceptColumns+`
		`+conceptFrom+`
		WHERE concept_id = $1
		FOR UPDATE`, conceptID)
	return scanConcept(row.Scan)
}

func applyMerge(ctx context.Context, db DBX, fromID, toID string) error {
	if _, err := db.ExecContext(ctx, `
		UPDATE kb.keyword_concepts
		SET status = 'merged', merged_into = $2, modify_time = NOW()
		WHERE concept_id = $1`, fromID, toID); err != nil {
		return fmt.Errorf("tombstone %s → %s: %w", fromID, toID, err)
	}
	if _, err := db.ExecContext(ctx, `
		UPDATE kb.keyword_surfaces
		SET concept_id = $2, origin_concept = COALESCE(origin_concept, $1), modify_time = NOW()
		WHERE concept_id = $1`, fromID, toID); err != nil {
		return fmt.Errorf("re-point surfaces %s → %s: %w", fromID, toID, err)
	}
	return nil
}

// isNeverMerge consults the persisted never_merge assertions
// (semid.NeverMergeStore writes the same table) for the keyword family. The
// pair is unordered; Add normalizes lexicographically. Takes a DBX rather
// than a store receiver so mergeGuards can share it between the transaction
// path and the unlocked fallback (F10).
func isNeverMerge(ctx context.Context, db DBX, a, b string) (bool, error) {
	if a > b {
		a, b = b, a
	}
	var exists bool
	err := db.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM kb.semid_never_merge
			WHERE family = $1 AND node_a = $2 AND node_b = $3
		)`, "keyword", a, b).Scan(&exists)
	return exists, err
}

// FollowMerge chases the merged_into chain from a caller-supplied concept id
// to the surviving concept (§14.1 item 3): a consumer that stored the
// tombstone id before a merge must still resolve to the survivor. Read-only
// — D7 forbids transitive closure, so no decision row is ever written, but
// chains can form and are followed here. Cycle-guarded; a cycle is data
// corruption and returns an error. A live concept resolves to itself.
func (s ConceptStore) FollowMerge(ctx context.Context, conceptID string) (string, error) {
	cur := conceptID
	seen := map[string]bool{}
	for {
		if seen[cur] {
			return "", fmt.Errorf("merged_into cycle detected at %s", cur)
		}
		seen[cur] = true
		c, err := s.GetConcept(ctx, cur)
		if err != nil {
			return "", err
		}
		if c.MergedInto == nil || *c.MergedInto == "" {
			return cur, nil
		}
		cur = *c.MergedInto
	}
}

// UnmergeConcept reverses MergeConcept (§14.4): surfaces whose
// origin_concept is fromID move back to fromID, and fromID's tombstone is
// cleared with restoreStatus, which must be "active" or "provisional".
//
// Chained merges (A merged into B, then B merged into C) need one more step:
// origin_concept is root-preserving (COALESCE in applyMerge), so A's
// surfaces already carry origin_concept = A, not B, even though they
// physically sit on C. Unmerging B must reposition those too — walking
// merged_into backward from B — or they stay stranded on C forever (F4,
// 2026080601-bug). This only repositions the chained surfaces; it does not
// restore A itself, which stays merged into B: unmerging B reverses exactly
// the B→C merge, not A→B, which is a separate decision (§14.4).
func (s ConceptStore) UnmergeConcept(ctx context.Context, fromID, restoreStatus string) (Concept, error) {
	if restoreStatus != "active" && restoreStatus != "provisional" {
		return Concept{}, fmt.Errorf("%w: restore status must be active or provisional, got %q", ErrMergeRejected, restoreStatus)
	}
	var restored Concept
	err := withKeywordIdentityMutation(ctx, s.DB, func(db DBX) error {
		current, err := (ConceptStore{DB: db}).GetConcept(ctx, fromID)
		if err != nil {
			return err
		}
		if current.Status != "merged" {
			return fmt.Errorf("%w: concept %s is not merged (status %s)", ErrMergeRejected, fromID, current.Status)
		}
		if err := applyUnmerge(ctx, db, fromID, restoreStatus); err != nil {
			return err
		}
		restored, err = (ConceptStore{DB: db}).GetConcept(ctx, fromID)
		return err
	})
	return restored, err
}

func applyUnmerge(ctx context.Context, db DBX, fromID, restoreStatus string) error {
	// Reposition surfaces belonging to any concept transitively merged into
	// fromID (the chained case, F4). Their origin_concept is left as-is —
	// it still correctly names their true root — only their concept_id
	// moves back to fromID, matching the state right before fromID's own
	// merge. mergeGuards refuses re-merging an already-merged source, so
	// merged_into cannot cycle; no cycle guard is needed here.
	if _, err := db.ExecContext(ctx, `
		WITH RECURSIVE chain AS (
			SELECT concept_id FROM kb.keyword_concepts WHERE merged_into = $1
			UNION ALL
			SELECT c.concept_id FROM kb.keyword_concepts c
			JOIN chain ON c.merged_into = chain.concept_id
		)
		UPDATE kb.keyword_surfaces
		SET concept_id = $1, modify_time = NOW()
		WHERE origin_concept IN (SELECT concept_id FROM chain)`, fromID); err != nil {
		return fmt.Errorf("reposition chained surfaces of %s: %w", fromID, err)
	}
	if _, err := db.ExecContext(ctx, `
		UPDATE kb.keyword_surfaces
		SET concept_id = $1, origin_concept = NULL, modify_time = NOW()
		WHERE origin_concept = $1`, fromID); err != nil {
		return fmt.Errorf("restore surfaces of %s: %w", fromID, err)
	}
	if _, err := db.ExecContext(ctx, `
		UPDATE kb.keyword_concepts
		SET status = $2, merged_into = NULL, modify_time = NOW()
		WHERE concept_id = $1`, fromID, restoreStatus); err != nil {
		return fmt.Errorf("clear tombstone %s: %w", fromID, err)
	}
	return nil
}
