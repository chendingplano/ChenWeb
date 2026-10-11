package docprocessing

// Persistence of extract_references (design D1, D4–D6): a record's mentions, the shared resource
// catalog (R1), the attachment of each processed document to its own resource, and holding
// resolution (R2), which links a mention to a copy the citing user may access.

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/lib/pq"
)

// Values of r1_state and r2_state.
const (
	refR1Resolved    = "resolved"
	refR1New         = "new_resource"
	refR1Unnamed     = "unnamed"
	refR2Held        = "held"
	refR2NotHeld     = "not_held"
	refR2EditionMis  = "edition_mismatch"
	refR2EditionAmb  = "edition_ambiguous"
	refR2Internal    = "internal"
	refKindKBInput   = "kb_input"
	refVisibilityPub = "public"
)

// refAccess is who owns a record: the scope a private resource and a holding are checked against.
type refAccess struct {
	UserID  string
	StoreID int64
}

func (a refAccess) visibility() string {
	if a.StoreID > 0 {
		return "store:" + strconv.FormatInt(a.StoreID, 10)
	}
	return "user:" + a.UserID
}

func loadRefAccess(ctx context.Context, q refQueryer, recordID int64) (refAccess, error) {
	var a refAccess
	err := q.QueryRowContext(ctx, `SELECT COALESCE(user_id, ''), COALESCE(ks_store_id, 0) FROM kb.inputs WHERE id = $1`, recordID).
		Scan(&a.UserID, &a.StoreID)
	return a, err
}

type refQueryer interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// ReferenceSQLStore stores mentions and the catalog in the project database.
type ReferenceSQLStore struct{ DB *sql.DB }

// ReplaceRecordReferences replaces every mention of the record (re-run semantics). Deleting the
// old mentions removes, by foreign-key cascade, every link and resolved row applied for them.
// Catalog entries (R1) are upserted in the same transaction; ms gets ids, resource and version ids.
func (s ReferenceSQLStore) ReplaceRecordReferences(ctx context.Context, recordID int64, eventID string, ms []RefMention) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	acc, err := loadRefAccess(ctx, tx, recordID)
	if err != nil {
		return fmt.Errorf("(MID_26101101) load access for record %d: %w", recordID, err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM kb.ref_mentions WHERE input_record_id = $1`, recordID); err != nil {
		return fmt.Errorf("(MID_26101102) delete mentions: %w", err)
	}
	for i := range ms {
		m := &ms[i]
		if err := upsertRefCatalog(ctx, tx, m, acc); err != nil {
			return fmt.Errorf("(MID_26101103) catalog for %q: %w", m.CitedRaw, err)
		}
		if err := insertRefMention(ctx, tx, eventID, m); err != nil {
			return fmt.Errorf("(MID_26101104) insert mention %s: %w", m.RefID, err)
		}
	}
	for _, m := range ms {
		if m.ListEntryIdx < 0 {
			continue
		}
		if _, err := tx.ExecContext(ctx, `UPDATE kb.ref_mentions SET list_entry_id = $2 WHERE id = $1`, m.ID, ms[m.ListEntryIdx].ID); err != nil {
			return fmt.Errorf("(MID_26101105) link list entry: %w", err)
		}
	}
	return tx.Commit()
}

// upsertRefCatalog resolves a mention to its resource and version (R1), creating them when new.
func upsertRefCatalog(ctx context.Context, tx *sql.Tx, m *RefMention, acc refAccess) error {
	if m.Internal {
		m.R1State = ""
		return nil
	}
	key, edition, public, ok := refIdentityKey(m.Identifiers, m.TargetTitle)
	if !ok || (m.Mechanism == "indirect_reference" && len(m.Identifiers) == 0) {
		// "according to the previous report" names no resource of its own.
		m.R1State = refR1Unnamed
		return nil
	}
	if edition == "" {
		edition = strings.TrimSpace(m.TargetVersion)
	}
	vis := acc.visibility()
	if public {
		vis = refVisibilityPub
	}
	resID, created, err := upsertRefResource(ctx, tx, key, vis, m.TargetType, m.TargetSubtype, m.TargetTitle)
	if err != nil {
		return err
	}
	m.ResourceID, m.R1State = resID, refR1Resolved
	if created {
		m.R1State = refR1New
	}
	if edition != "" {
		if m.VersionID, err = upsertRefVersion(ctx, tx, resID, edition); err != nil {
			return err
		}
	}
	for _, id := range m.Identifiers {
		idVis := vis
		if !refPublicKinds[id.Kind] {
			idVis = acc.visibility()
		}
		var verID any
		if id.Edition != "" && m.VersionID > 0 {
			verID = m.VersionID
		}
		if _, err := tx.ExecContext(ctx, `
INSERT INTO kb.ref_resource_identifiers (resource_id, version_id, kind, value, visibility)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (kind, value, visibility) DO NOTHING`, resID, verID, id.Kind, id.Value, idVis); err != nil {
			return err
		}
	}
	return nil
}

func upsertRefResource(ctx context.Context, q refQueryer, key, vis, targetType, subtype, title string) (id int64, created bool, err error) {
	err = q.QueryRowContext(ctx, `
INSERT INTO kb.ref_resources (identity_key, visibility, target_type, target_subtype, title)
VALUES ($1, $2, $3, $4, NULLIF($5, ''))
ON CONFLICT (identity_key, visibility) DO UPDATE
   SET title = COALESCE(kb.ref_resources.title, EXCLUDED.title), updated_at = NOW()
RETURNING id, (xmax = 0)`, key, vis, targetType, subtype, title).Scan(&id, &created)
	return id, created, err
}

func upsertRefVersion(ctx context.Context, q refQueryer, resourceID int64, label string) (int64, error) {
	var id int64
	err := q.QueryRowContext(ctx, `
INSERT INTO kb.ref_resource_versions (resource_id, version_label) VALUES ($1, $2)
ON CONFLICT (resource_id, version_label) DO UPDATE SET version_label = EXCLUDED.version_label
RETURNING id`, resourceID, label).Scan(&id)
	return id, err
}

func insertRefMention(ctx context.Context, tx *sql.Tx, eventID string, m *RefMention) error {
	spans, _ := json.Marshal(m.SourceLineSpans)
	ids, _ := json.Marshal(m.Identifiers)
	if m.Identifiers == nil {
		ids = []byte("[]")
	}
	ext, _ := json.Marshal(map[string]any{"notes": m.Notes})
	return tx.QueryRowContext(ctx, `
INSERT INTO kb.ref_mentions (
  input_record_id, ref_id, event_id, section, source_line_spans, evidence, ref_form, list_kind,
  normativity, in_requirement, list_mismatch, mechanism, cited_raw, identifiers, target_type,
  target_subtype, target_subtype_label, target_title, target_version, relationships, target_scope,
  target_locator, target_content, metric_name, metric_name_en, metric_subject, metric_subject_en,
  condition, internal, confidence, resource_id, version_id, r1_state, target_input_id, r3_state,
  mention_key, ext_info, source_table_rows)
VALUES ($1,$2,NULLIF($3,''),NULLIF($4,''),$5,NULLIF($6,''),$7,NULLIF($8,''),$9,$10,NULLIF($11,''),$12,
  NULLIF($13,''),$14,$15,$16,NULLIF($17,''),NULLIF($18,''),NULLIF($19,''),$20,$21,NULLIF($22,''),$23,
  NULLIF($24,''),NULLIF($25,''),NULLIF($26,''),NULLIF($27,''),NULLIF($28,''),$29,$30,
  NULLIF($31,0),NULLIF($32,0),NULLIF($33,''),NULLIF($34,0),NULLIF($35,''),$36,$37,$38::jsonb)
RETURNING id`,
		m.InputRecordID, m.RefID, eventID, m.Section, spans, m.Evidence, m.RefForm, m.ListKind,
		m.Normativity, m.InRequirement, m.ListMismatch, m.Mechanism, m.CitedRaw, ids, m.TargetType,
		m.TargetSubtype, m.TargetSubtypeLabel, m.TargetTitle, m.TargetVersion, pq.Array(m.Relationships), m.TargetScope,
		m.TargetLocator, m.TargetContent, m.MetricName, m.MetricNameEn, m.MetricSubject, m.MetricSubjectEn,
		m.Condition, m.Internal, m.Confidence, m.ResourceID, m.VersionID, m.R1State, m.TargetInputID, m.R3State,
		m.MentionKey, ext, sourceTableRowsSQLValue(m.SourceTableRows)).Scan(&m.ID)
}

// AttachOwnDocument attaches the processed record to its own resource and version (design D5)
// through a kb_input identifier, which is never public. It returns the resource id, 0 when the
// document has no recognizable identifier.
func (s ReferenceSQLStore) AttachOwnDocument(ctx context.Context, recordID int64, own RefIdentifier, title, targetType string) (int64, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	acc, err := loadRefAccess(ctx, tx, recordID)
	if err != nil {
		return 0, err
	}
	key, edition, public, ok := refIdentityKey([]RefIdentifier{own}, "")
	if !ok {
		return 0, nil
	}
	vis := acc.visibility()
	if public {
		vis = refVisibilityPub
	}
	resID, _, err := upsertRefResource(ctx, tx, key, vis, targetType, refSubtypeOthers, title)
	if err != nil {
		return 0, err
	}
	var verID any
	if edition != "" {
		v, err := upsertRefVersion(ctx, tx, resID, edition)
		if err != nil {
			return 0, err
		}
		verID = v
	}
	if _, err := tx.ExecContext(ctx, `
INSERT INTO kb.ref_resource_identifiers (resource_id, version_id, kind, value, visibility)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (kind, value, visibility) DO UPDATE SET resource_id = EXCLUDED.resource_id, version_id = EXCLUDED.version_id`,
		resID, verID, refKindKBInput, strconv.FormatInt(recordID, 10), acc.visibility()); err != nil {
		return 0, err
	}
	return resID, tx.Commit()
}

// refHolding is one accessible copy of a resource.
type refHolding struct {
	InputID int64
	Version string
}

// accessibleHoldings lists the copies of a resource the citing record's owner may access
// (design D6): same user, same knowledge store, or an active unexpired grant.
func accessibleHoldings(ctx context.Context, q refQueryer, resourceID, citingRecordID int64, acc refAccess) ([]refHolding, error) {
	rows, err := q.QueryContext(ctx, `
SELECT i.id, COALESCE(v.version_label, '')
FROM kb.ref_resource_identifiers ri
JOIN kb.inputs i ON i.id::text = ri.value
LEFT JOIN kb.ref_resource_versions v ON v.id = ri.version_id
WHERE ri.kind = 'kb_input' AND ri.resource_id = $1 AND i.id <> $2
  AND ( (i.user_id = $3 AND $3 <> '')
     OR ($4 > 0 AND i.ks_store_id = $4)
     OR EXISTS (SELECT 1 FROM kb.agentic_knowledge_grants g
                WHERE g.user_id = $3 AND g.active AND (g.expires_at IS NULL OR g.expires_at > NOW())
                  AND (g.document_id = i.id OR g.knowledge_store_id = i.ks_store_id)) )
ORDER BY v.version_label DESC NULLS LAST, i.id DESC`, resourceID, citingRecordID, acc.UserID, acc.StoreID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []refHolding
	for rows.Next() {
		var h refHolding
		if err := rows.Scan(&h.InputID, &h.Version); err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

// decideHolding picks the copy a mention resolves to (design D6). Holdings are ordered newest
// version first.
func decideHolding(edition string, standard bool, hs []refHolding) (state string, inputID int64) {
	if len(hs) == 0 {
		return refR2NotHeld, 0
	}
	if edition != "" {
		for _, h := range hs {
			if h.Version == edition {
				return refR2Held, h.InputID
			}
		}
		return refR2EditionMis, hs[0].InputID
	}
	if !standard {
		versions := map[string]bool{}
		for _, h := range hs {
			versions[h.Version] = true
		}
		if len(versions) > 1 {
			return refR2EditionAmb, hs[0].InputID
		}
	}
	return refR2Held, hs[0].InputID
}

// ResolveHoldings runs R2 for the mentions of the given records that have a resource, and for
// internal mentions points at the record itself. With resourceID > 0 only mentions of that
// resource are re-evaluated (the holding event, §5.4).
func (s ReferenceSQLStore) ResolveHoldings(ctx context.Context, recordIDs []int64, resourceID int64) error {
	for _, rid := range recordIDs {
		acc, err := loadRefAccess(ctx, s.DB, rid)
		if err != nil {
			return err
		}
		if _, err := s.DB.ExecContext(ctx, `UPDATE kb.ref_mentions SET target_input_id = input_record_id, r2_state = $2
WHERE input_record_id = $1 AND internal`, rid, refR2Internal); err != nil {
			return err
		}
		rows, err := s.DB.QueryContext(ctx, `
SELECT m.id, m.resource_id, COALESCE(v.version_label, ''), r.identity_key
FROM kb.ref_mentions m
JOIN kb.ref_resources r ON r.id = m.resource_id
LEFT JOIN kb.ref_resource_versions v ON v.id = m.version_id
WHERE m.input_record_id = $1 AND NOT m.internal AND ($2 = 0 OR m.resource_id = $2)`, rid, resourceID)
		if err != nil {
			return err
		}
		type pending struct {
			id, res int64
			edition string
			std     bool
		}
		var ps []pending
		for rows.Next() {
			var p pending
			var key string
			if err := rows.Scan(&p.id, &p.res, &p.edition, &key); err != nil {
				rows.Close()
				return err
			}
			p.std = strings.HasPrefix(key, "standard:") || strings.HasPrefix(key, "rfc:")
			ps = append(ps, p)
		}
		rows.Close()
		for _, p := range ps {
			hs, err := accessibleHoldings(ctx, s.DB, p.res, rid, acc)
			if err != nil {
				return err
			}
			state, target := decideHolding(p.edition, p.std, hs)
			if _, err := s.DB.ExecContext(ctx, `UPDATE kb.ref_mentions SET r2_state = $2, target_input_id = NULLIF($3, 0) WHERE id = $1`,
				p.id, state, target); err != nil {
				return err
			}
		}
	}
	return nil
}

// CitingRecordsOfResource lists records (other than exclude) whose mentions point at a resource.
func (s ReferenceSQLStore) CitingRecordsOfResource(ctx context.Context, resourceID, exclude int64) ([]int64, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT DISTINCT input_record_id FROM kb.ref_mentions WHERE resource_id = $1 AND input_record_id <> $2`, resourceID, exclude)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}
