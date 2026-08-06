package semid

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"
)

// DecisionLogEntry is one append-only kernel decision (shared across families).
type DecisionLogEntry struct {
	Family        string
	Scope         string
	Input         json.RawMessage
	Output        json.RawMessage
	Verdict       string
	Model         string
	PromptVersion string
	Actor         string
	Tokens        int
}

// DecisionLogStore appends kernel decisions (ADR DR15 audit).
type DecisionLogStore struct {
	DB *sql.DB
}

// Append records one kernel decision and returns the decision-log row id,
// so the caller can link its own evidence record (e.g. a keyword
// occurrence, K4) to the exact decision that produced it.
func (s DecisionLogStore) Append(ctx context.Context, e DecisionLogEntry) (int64, error) {
	if s.DB == nil {
		return 0, errors.New("db is nil")
	}
	input := e.Input
	if len(input) == 0 {
		input = []byte("{}")
	}
	output := e.Output
	if len(output) == 0 {
		output = []byte("{}")
	}
	const stmt = `
INSERT INTO kb.semid_decision_log
	(family, scope, input, output, verdict, model, prompt_version, actor, tokens)
VALUES ($1, $2, $3::jsonb, $4::jsonb, $5, $6, $7, $8, $9)
RETURNING id`
	var id int64
	err := s.DB.QueryRowContext(ctx, stmt,
		e.Family, nullableString(e.Scope), string(input), string(output), e.Verdict,
		nullableString(e.Model), nullableString(e.PromptVersion), nullableString(e.Actor), e.Tokens,
	).Scan(&id)
	return id, err
}

// NeverMerge is a family-scoped never-merge assertion.
type NeverMerge struct {
	ID        int64
	Family    string
	NodeA     string
	NodeB     string
	Reason    string
	Actor     string
	CreatedAt time.Time
}

// NeverMergeStore persists never-merge assertions.
type NeverMergeStore struct {
	DB *sql.DB
}

// Add records a never-merge assertion. The pair is unordered (normalized
// lexicographically so the UNIQUE(family, node_a, node_b) key is stable).
func (s NeverMergeStore) Add(ctx context.Context, family, nodeA, nodeB, reason, actor string) error {
	if s.DB == nil {
		return errors.New("db is nil")
	}
	if nodeA > nodeB {
		nodeA, nodeB = nodeB, nodeA
	}
	const stmt = `
INSERT INTO kb.semid_never_merge (family, node_a, node_b, reason, actor)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (family, node_a, node_b) DO NOTHING`
	_, err := s.DB.ExecContext(ctx, stmt, family, nodeA, nodeB, nullableString(reason), nullableString(actor))
	return err
}

// IsNeverMerge reports whether the unordered pair is asserted never-mergeable.
func (s NeverMergeStore) IsNeverMerge(ctx context.Context, family, nodeA, nodeB string) (bool, error) {
	if s.DB == nil {
		return false, errors.New("db is nil")
	}
	if nodeA > nodeB {
		nodeA, nodeB = nodeB, nodeA
	}
	var exists bool
	err := s.DB.QueryRowContext(ctx, `
SELECT EXISTS (SELECT 1 FROM kb.semid_never_merge WHERE family = $1 AND node_a = $2 AND node_b = $3)`,
		family, nodeA, nodeB).Scan(&exists)
	return exists, err
}

// List returns the family's never-merge assertions.
func (s NeverMergeStore) List(ctx context.Context, family string) ([]NeverMerge, error) {
	if s.DB == nil {
		return nil, errors.New("db is nil")
	}
	rows, err := s.DB.QueryContext(ctx, `
SELECT id, family, node_a, node_b, COALESCE(reason, ''), COALESCE(actor, ''), created_at
FROM kb.semid_never_merge
WHERE family = $1
ORDER BY id`, family)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]NeverMerge, 0)
	for rows.Next() {
		var n NeverMerge
		if err := rows.Scan(&n.ID, &n.Family, &n.NodeA, &n.NodeB, &n.Reason, &n.Actor, &n.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// Snapshot is a family's kernel state at a normalizer version.
type Snapshot struct {
	ID                int64
	Family            string
	NormalizerVersion int
	Counts            json.RawMessage
	PromotedAt        time.Time
}

// SnapshotStore records and reads family snapshots.
type SnapshotStore struct {
	DB *sql.DB
}

// Record stores a snapshot for a family + normalizer version.
func (s SnapshotStore) Record(ctx context.Context, family string, normalizerVersion int, counts map[string]any) error {
	if s.DB == nil {
		return errors.New("db is nil")
	}
	raw, err := json.Marshal(counts)
	if err != nil {
		return err
	}
	_, err = s.DB.ExecContext(ctx, `
INSERT INTO kb.semid_snapshots (family, normalizer_version, counts)
VALUES ($1, $2, $3::jsonb)`,
		family, normalizerVersion, string(raw))
	return err
}

// Latest returns the most recent snapshot for a family.
func (s SnapshotStore) Latest(ctx context.Context, family string) (Snapshot, error) {
	if s.DB == nil {
		return Snapshot{}, errors.New("db is nil")
	}
	var snap Snapshot
	err := s.DB.QueryRowContext(ctx, `
SELECT id, family, normalizer_version, counts, promoted_at
FROM kb.semid_snapshots
WHERE family = $1
ORDER BY normalizer_version DESC, id DESC
LIMIT 1`, family).Scan(&snap.ID, &snap.Family, &snap.NormalizerVersion, &snap.Counts, &snap.PromotedAt)
	return snap, err
}

func nullableString(s string) any {
	if s == "" {
		return nil
	}
	return s
}
