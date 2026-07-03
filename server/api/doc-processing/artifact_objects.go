package docprocessing

import (
	"context"
	"crypto/sha1"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"unicode"
)

const (
	ObjectReconcilePending   = "pending"
	ObjectReconcileMatched   = "matched"
	ObjectReconcileNew       = "new"
	ObjectReconcileAmbiguous = "ambiguous"
	ObjectReconcileRejected  = "rejected"
)

type ArtifactObject struct {
	ID                  int64
	SourceRecordID      int64
	InputRecordID       int64
	ArtifactType        string
	ArtifactID          string
	ObjectID            string
	ObjectName          string
	ObjectNameEn        string
	ObjectNameZh        string
	Language            string
	ObjectType          string
	ObjectRole          string
	Aliases             []string
	Acronyms            []string
	NormalizedNames     []string
	Description         string
	EvidenceQuote       string
	SourceLineSpans     []string
	Confidence          float64
	ReconcileStatus     string
	ReconcileConfidence float64
	ExtInfo             map[string]any
}

type ObjectNode struct {
	ObjectID          string
	CanonicalObjectID string
	CanonicalName     string
	CanonicalNameEn   string
	CanonicalNameZh   string
	PrimaryLanguage   string
	ObjectType        string
	Aliases           []string
	Acronyms          []string
	NormalizedNames   []string
	Description       string
	SearchDocument    string
	ReconcileStatus   string
	ExtInfo           map[string]any
}

type ObjectNodeCandidate struct {
	Node   ObjectNode
	Score  float64
	Method string
}

type ObjectReconcileOptions struct {
	EmbeddingEnabled bool
	MaxCandidates    int
}

type ObjectNodeStore interface {
	FindCandidates(ctx context.Context, obj ArtifactObject, opts ObjectReconcileOptions) ([]ObjectNodeCandidate, error)
	CreateNode(ctx context.Context, obj ArtifactObject) (ObjectNode, error)
}

type ObjectReconcileResult struct {
	ObjectID   string
	Status     string
	Confidence float64
	Method     string
}

type ObjectReconciler struct {
	Store   ObjectNodeStore
	Options ObjectReconcileOptions
}

func (r ObjectReconciler) ReconcileOne(ctx context.Context, obj ArtifactObject) (ObjectReconcileResult, error) {
	if r.Store == nil {
		return ObjectReconcileResult{}, fmt.Errorf("object node store is nil")
	}
	candidates, err := r.Store.FindCandidates(ctx, obj, r.Options)
	if err != nil {
		return ObjectReconcileResult{}, err
	}
	if len(candidates) == 1 && candidates[0].Score >= 1 {
		return ObjectReconcileResult{
			ObjectID:   candidates[0].Node.ObjectID,
			Status:     ObjectReconcileMatched,
			Confidence: candidates[0].Score,
			Method:     candidates[0].Method,
		}, nil
	}
	if len(candidates) > 1 && candidates[0].Score == candidates[1].Score {
		return ObjectReconcileResult{Status: ObjectReconcileAmbiguous, Confidence: candidates[0].Score, Method: candidates[0].Method}, nil
	}
	if len(candidates) > 0 && candidates[0].Score >= 0.95 {
		return ObjectReconcileResult{
			ObjectID:   candidates[0].Node.ObjectID,
			Status:     ObjectReconcileMatched,
			Confidence: candidates[0].Score,
			Method:     candidates[0].Method,
		}, nil
	}
	node, err := r.Store.CreateNode(ctx, obj)
	if err != nil {
		return ObjectReconcileResult{}, err
	}
	return ObjectReconcileResult{
		ObjectID:   node.ObjectID,
		Status:     ObjectReconcileNew,
		Confidence: 1,
		Method:     "new_node",
	}, nil
}

func normalizeArtifactObjectsForArtifact(recordID int64, artifactType, artifactID string, artifact map[string]any) []ArtifactObject {
	rawObjects := objectItemsFromValue(artifact["objects"])
	if artifactType == searchArtifactInventoryItem {
		rawObjects = append(synthesizedObjectItems(artifactType, artifact), rawObjects...)
	} else if len(rawObjects) == 0 {
		rawObjects = synthesizedObjectItems(artifactType, artifact)
	}
	var out []ArtifactObject
	seen := map[string]struct{}{}
	for _, raw := range rawObjects {
		obj := normalizeArtifactObject(recordID, artifactType, artifactID, raw, artifact)
		if obj.ObjectName == "" && obj.ObjectNameEn == "" && obj.ObjectNameZh == "" {
			continue
		}
		key := normalizedObjectIdentityKey(obj)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, obj)
	}
	return out
}

func objectItemsFromValue(raw any) []map[string]any {
	switch v := raw.(type) {
	case []map[string]any:
		return v
	case []any:
		out := make([]map[string]any, 0, len(v))
		for _, item := range v {
			if m, ok := item.(map[string]any); ok {
				out = append(out, m)
			}
		}
		return out
	default:
		return nil
	}
}

func synthesizedObjectItems(artifactType string, artifact map[string]any) []map[string]any {
	switch artifactType {
	case searchArtifactMetric:
		name := firstNonEmptyTrimmed(asString(artifact["subject"]), asString(artifact["metric_subject"]))
		if name == "" {
			return nil
		}
		return []map[string]any{{
			"object_name":       name,
			"object_name_en":    firstNonEmptyTrimmed(asString(artifact["subject_en"]), asString(artifact["metric_subject_en"])),
			"object_role":       "measured_object",
			"source_line_spans": artifact["source_line_spans"],
			"confidence":        artifact["confidence"],
		}}
	case searchArtifactProvision:
		name := firstNonEmptyTrimmed(asString(artifact["subject"]), asString(artifact["provision_subject"]))
		if name == "" {
			return nil
		}
		return []map[string]any{{
			"object_name":       name,
			"object_name_en":    firstNonEmptyTrimmed(asString(artifact["subject_en"]), asString(artifact["provision_subject_en"])),
			"object_role":       "regulated_object",
			"source_line_spans": artifact["source_line_spans"],
			"confidence":        artifact["confidence"],
		}}
	case searchArtifactInventoryItem:
		name := firstNonEmptyTrimmed(asString(artifact["canonical_name"]), asString(artifact["item_name"]))
		if name == "" {
			return nil
		}
		return []map[string]any{{
			"object_name":       name,
			"object_role":       "self",
			"object_type":       firstNonEmptyTrimmed(firstString(toStringSlice(artifact["item_categories"])), "inventory_item"),
			"aliases":           artifact["aliases"],
			"source_line_spans": artifact["source_line_spans"],
			"confidence":        artifact["confidence"],
		}}
	default:
		return nil
	}
}

func normalizeArtifactObject(recordID int64, artifactType, artifactID string, raw map[string]any, artifact map[string]any) ArtifactObject {
	obj := ArtifactObject{
		SourceRecordID:      recordID,
		InputRecordID:       recordID,
		ArtifactType:        strings.TrimSpace(artifactType),
		ArtifactID:          strings.TrimSpace(artifactID),
		ObjectName:          strings.TrimSpace(asString(raw["object_name"])),
		ObjectNameEn:        strings.TrimSpace(asString(raw["object_name_en"])),
		ObjectNameZh:        strings.TrimSpace(asString(raw["object_name_zh"])),
		Language:            strings.TrimSpace(asString(raw["language"])),
		ObjectType:          normalizeObjectToken(asString(raw["object_type"])),
		ObjectRole:          normalizeObjectRole(raw, artifactType),
		Aliases:             uniqueStrings(toStringSlice(raw["aliases"])),
		Acronyms:            uniqueStrings(toStringSlice(raw["acronyms"])),
		Description:         strings.TrimSpace(asString(raw["description"])),
		EvidenceQuote:       strings.TrimSpace(asString(raw["evidence_quote"])),
		SourceLineSpans:     normalizeSourceLineSpans(firstNonNil(raw["source_line_spans"], artifact["source_line_spans"])),
		Confidence:          toFloat(raw["confidence"]),
		ReconcileStatus:     ObjectReconcilePending,
		ReconcileConfidence: 0,
		ExtInfo:             map[string]any{"source": artifactType},
	}
	if obj.ObjectName == "" {
		obj.ObjectName = firstNonEmptyTrimmed(obj.ObjectNameZh, obj.ObjectNameEn)
	}
	if obj.ObjectType == "" {
		obj.ObjectType = "other"
	}
	if obj.Confidence == 0 {
		obj.Confidence = toFloat(artifact["confidence"])
	}
	obj.NormalizedNames = buildObjectNormalizedNames(obj, toStringSlice(raw["normalized_names"]))
	return obj
}

func normalizeObjectRole(raw map[string]any, artifactType string) string {
	role := normalizeObjectToken(asString(raw["object_role"]))
	if role != "" {
		return role
	}
	switch artifactType {
	case searchArtifactMetric:
		return "measured_object"
	case searchArtifactProvision:
		return "regulated_object"
	case searchArtifactInventoryItem:
		return "self"
	default:
		return "other"
	}
}

func buildObjectNormalizedNames(obj ArtifactObject, extra []string) []string {
	var names []string
	for _, name := range []string{obj.ObjectName, obj.ObjectNameEn, obj.ObjectNameZh} {
		names = appendObjectNormalizedName(names, name)
	}
	for _, name := range obj.Aliases {
		names = appendObjectNormalizedName(names, name)
	}
	for _, name := range obj.Acronyms {
		names = appendObjectNormalizedName(names, name)
	}
	for _, name := range extra {
		names = appendObjectNormalizedName(names, name)
	}
	return names
}

func appendObjectNormalizedName(names []string, raw string) []string {
	n := normalizeObjectName(raw)
	if n == "" {
		return names
	}
	return appendUniqueString(names, n)
}

var objectPunctRE = regexp.MustCompile(`[._\-\/]+`)

func normalizeObjectName(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	raw = strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return ' '
		}
		return unicode.ToLower(r)
	}, raw)
	raw = objectPunctRE.ReplaceAllString(raw, " ")
	return strings.Join(strings.Fields(raw), " ")
}

func normalizeObjectToken(raw string) string {
	raw = normalizeObjectName(raw)
	return strings.ReplaceAll(raw, " ", "_")
}

func normalizedObjectIdentityKey(obj ArtifactObject) string {
	name := firstString(obj.NormalizedNames)
	return strings.Join([]string{
		strings.TrimSpace(obj.ArtifactType),
		strings.TrimSpace(obj.ArtifactID),
		name,
		strings.TrimSpace(obj.ObjectRole),
	}, "|")
}

func objectTypesCompatible(a, b string) bool {
	a = normalizeObjectToken(a)
	b = normalizeObjectToken(b)
	return a == "" || b == "" || a == "other" || b == "other" || a == b
}

func objectNameBundlesOverlap(a, b []string) bool {
	if len(a) == 0 || len(b) == 0 {
		return false
	}
	set := make(map[string]struct{}, len(a))
	for _, item := range a {
		n := normalizeObjectName(item)
		if n != "" {
			set[n] = struct{}{}
		}
	}
	for _, item := range b {
		n := normalizeObjectName(item)
		if _, ok := set[n]; ok {
			return true
		}
	}
	return false
}

func firstString(items []string) string {
	for _, item := range items {
		if s := strings.TrimSpace(item); s != "" {
			return s
		}
	}
	return ""
}

func containsString(items []string, want string) bool {
	want = strings.TrimSpace(want)
	for _, item := range items {
		if strings.TrimSpace(item) == want {
			return true
		}
	}
	return false
}

func buildObjectID(recordID int64, name string, seq int) string {
	h := sha1.Sum([]byte(fmt.Sprintf("%d:%s:%d", recordID, normalizeObjectName(name), seq)))
	return fmt.Sprintf("obj_%d_%s", recordID, hex.EncodeToString(h[:])[:12])
}

type ArtifactObjectSQLStore struct {
	DB *sql.DB
}

func (s ArtifactObjectSQLStore) ReplaceObjectsForRecord(ctx context.Context, recordID int64, artifactType string, objects []ArtifactObject) error {
	if s.DB == nil {
		return fmt.Errorf("db is nil")
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `DELETE FROM kb.artifact_objects WHERE source_record_id = $1 AND artifact_type = $2`, recordID, strings.TrimSpace(artifactType)); err != nil {
		return err
	}
	const stmt = `
INSERT INTO kb.artifact_objects (
	source_record_id, input_record_id, artifact_type, artifact_id, object_id,
	object_name, object_name_en, object_name_zh, language,
	object_type, object_role, aliases, acronyms, normalized_names,
	description, evidence_quote, source_line_spans, confidence,
	reconcile_status, reconcile_confidence, ext_info
) VALUES (
	$1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12::jsonb,$13::jsonb,$14::jsonb,
	$15,$16,$17::jsonb,$18,$19,$20,$21::jsonb
)`
	for _, obj := range objects {
		aliases, _ := json.Marshal(orEmptySlice(obj.Aliases))
		acronyms, _ := json.Marshal(orEmptySlice(obj.Acronyms))
		normalized, _ := json.Marshal(orEmptySlice(obj.NormalizedNames))
		spans, _ := json.Marshal(orEmptySlice(obj.SourceLineSpans))
		ext, _ := json.Marshal(obj.ExtInfo)
		sourceRecordID := obj.SourceRecordID
		if sourceRecordID <= 0 {
			sourceRecordID = recordID
		}
		if _, err := tx.ExecContext(ctx, stmt,
			sourceRecordID, obj.InputRecordID, obj.ArtifactType, obj.ArtifactID, nullEmpty(obj.ObjectID),
			obj.ObjectName, nullEmpty(obj.ObjectNameEn), nullEmpty(obj.ObjectNameZh), nullEmpty(obj.Language),
			obj.ObjectType, obj.ObjectRole, string(aliases), string(acronyms), string(normalized),
			nullEmpty(obj.Description), nullEmpty(obj.EvidenceQuote), string(spans), obj.Confidence,
			firstNonEmptyTrimmed(obj.ReconcileStatus, ObjectReconcilePending), obj.ReconcileConfidence, string(ext),
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func nullEmpty(s string) any {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	return s
}

func artifactObjectSearchTextByArtifactID(ctx context.Context, db *sql.DB, recordID int64, artifactType string) (map[string]string, error) {
	if db == nil {
		return nil, fmt.Errorf("db is nil")
	}
	rows, err := db.QueryContext(ctx, `
SELECT artifact_id,
       COALESCE(object_name, ''),
       COALESCE(object_name_en, ''),
       COALESCE(object_name_zh, ''),
       COALESCE(object_type, ''),
       COALESCE(object_role, ''),
       COALESCE(description, ''),
       COALESCE(aliases, '[]'::jsonb),
       COALESCE(acronyms, '[]'::jsonb)
FROM kb.artifact_objects
WHERE source_record_id = $1 AND artifact_type = $2
ORDER BY artifact_id, id`, recordID, strings.TrimSpace(artifactType))
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	partsByID := map[string][]string{}
	for rows.Next() {
		var (
			artifactID               string
			objectName, objectNameEn string
			objectNameZh, objectType string
			objectRole, description  string
			aliasesRaw, acronymsRaw  []byte
		)
		if err := rows.Scan(&artifactID, &objectName, &objectNameEn, &objectNameZh, &objectType, &objectRole, &description, &aliasesRaw, &acronymsRaw); err != nil {
			return nil, err
		}
		artifactID = strings.TrimSpace(artifactID)
		if artifactID == "" {
			continue
		}
		partsByID[artifactID] = append(partsByID[artifactID],
			objectName,
			objectNameEn,
			objectNameZh,
			objectType,
			objectRole,
			description,
			searchDocumentArrayText(jsonStringArray(aliasesRaw)),
			searchDocumentArrayText(jsonStringArray(acronymsRaw)),
		)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make(map[string]string, len(partsByID))
	for artifactID, parts := range partsByID {
		out[artifactID] = joinUniqueSearchParts(parts...)
	}
	return out, nil
}

func artifactObjectSearchTextExcluding(ctx context.Context, db *sql.DB, recordID int64, artifactType string, exclude map[string][]string) (map[string]string, error) {
	textByID, err := artifactObjectSearchTextByArtifactID(ctx, db, recordID, artifactType)
	if err != nil || len(textByID) == 0 || len(exclude) == 0 {
		return textByID, err
	}
	// The coarse map above is good enough for provisions. Inventory needs to avoid
	// re-adding the item itself, so reload object rows and filter per artifact.
	rows, err := db.QueryContext(ctx, `
SELECT artifact_id,
       COALESCE(object_name, ''),
       COALESCE(object_name_en, ''),
       COALESCE(object_name_zh, ''),
       COALESCE(object_type, ''),
       COALESCE(object_role, ''),
       COALESCE(description, ''),
       COALESCE(aliases, '[]'::jsonb),
       COALESCE(acronyms, '[]'::jsonb)
FROM kb.artifact_objects
WHERE source_record_id = $1 AND artifact_type = $2
ORDER BY artifact_id, id`, recordID, strings.TrimSpace(artifactType))
	if err != nil {
		return textByID, err
	}
	defer func() { _ = rows.Close() }()

	partsByID := map[string][]string{}
	for rows.Next() {
		var (
			artifactID               string
			objectName, objectNameEn string
			objectNameZh, objectType string
			objectRole, description  string
			aliasesRaw, acronymsRaw  []byte
		)
		if err := rows.Scan(&artifactID, &objectName, &objectNameEn, &objectNameZh, &objectType, &objectRole, &description, &aliasesRaw, &acronymsRaw); err != nil {
			return textByID, err
		}
		artifactID = strings.TrimSpace(artifactID)
		if artifactID == "" || objectNameMatchesAny([]string{objectName, objectNameEn, objectNameZh}, exclude[artifactID]) {
			continue
		}
		partsByID[artifactID] = append(partsByID[artifactID],
			objectName,
			objectNameEn,
			objectNameZh,
			objectType,
			objectRole,
			description,
			searchDocumentArrayText(jsonStringArray(aliasesRaw)),
			searchDocumentArrayText(jsonStringArray(acronymsRaw)),
		)
	}
	if err := rows.Err(); err != nil {
		return textByID, err
	}
	out := make(map[string]string, len(partsByID))
	for artifactID, parts := range partsByID {
		out[artifactID] = joinUniqueSearchParts(parts...)
	}
	return out, nil
}

func objectNameMatchesAny(names []string, excluded []string) bool {
	if len(excluded) == 0 {
		return false
	}
	seen := map[string]struct{}{}
	for _, name := range excluded {
		if n := normalizeObjectName(name); n != "" {
			seen[n] = struct{}{}
		}
	}
	for _, name := range names {
		if _, ok := seen[normalizeObjectName(name)]; ok {
			return true
		}
	}
	return false
}
