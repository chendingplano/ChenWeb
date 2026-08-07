package terminology

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/chendingplano/deepdoc/server/api/ontology/keywords"
	"github.com/chendingplano/deepdoc/server/api/ontology/semid"
)

// CatalogSnapshot is the deterministic, normalized output of one adapter
// conversion. Adapters never write keyword concepts and cannot grant
// authority on their own; promotion is a separate reviewed step.
type CatalogSnapshot struct {
	Entries           []CatalogEntry            `json:"entries"`
	Labels            []CatalogLabel            `json:"labels"`
	Relations         []CatalogRelation         `json:"relations"`
	NegativeDecisions []CatalogNegativeDecision `json:"negative_decisions"`
	UCUMCodes         []UCUMCode                `json:"ucum_codes"`
}

type CatalogEntry struct {
	ExternalID        string          `json:"external_id"`
	EntryStatus       string          `json:"entry_status"`
	ProvenanceLocator string          `json:"provenance_locator"`
	NativePayload     json.RawMessage `json:"native_payload,omitempty"`
}

type CatalogLabel struct {
	ExternalID        string          `json:"external_id"`
	Language          string          `json:"language"`
	LabelRole         string          `json:"label_role"`
	Label             string          `json:"label"`
	ProvenanceLocator string          `json:"provenance_locator"`
	NativePayload     json.RawMessage `json:"native_payload,omitempty"`
}

type CatalogRelation struct {
	SubjectExternalID string          `json:"subject_external_id"`
	Relation          string          `json:"relation"`
	ObjectSource      string          `json:"object_source"`
	ObjectRelease     string          `json:"object_release"`
	ObjectExternalID  string          `json:"object_external_id"`
	ProvenanceLocator string          `json:"provenance_locator"`
	NativePayload     json.RawMessage `json:"native_payload,omitempty"`
}

type CatalogNegativeDecision struct {
	SubjectExternalID string          `json:"subject_external_id"`
	ObjectSource      string          `json:"object_source"`
	ObjectRelease     string          `json:"object_release"`
	ObjectExternalID  string          `json:"object_external_id"`
	Relation          string          `json:"relation"`
	Reason            string          `json:"reason"`
	ProvenanceLocator string          `json:"provenance_locator"`
	NativePayload     json.RawMessage `json:"native_payload,omitempty"`
}

type UCUMCode struct {
	Code              string          `json:"code"`
	PrintSymbol       string          `json:"print_symbol"`
	Dimension         string          `json:"dimension"`
	ProvenanceLocator string          `json:"provenance_locator"`
	NativePayload     json.RawMessage `json:"native_payload,omitempty"`
}

// ImportCounts are the stable staging sizes reported after an import.
type ImportCounts struct {
	Entries           int `json:"entries"`
	Labels            int `json:"labels"`
	Relations         int `json:"relations"`
	NegativeDecisions int `json:"negative_decisions"`
	UCUMCodes         int `json:"ucum_codes"`
	Artifacts         int `json:"artifacts"`
}

func (s CatalogSnapshot) Counts() ImportCounts {
	return ImportCounts{
		Entries: len(s.Entries), Labels: len(s.Labels), Relations: len(s.Relations),
		NegativeDecisions: len(s.NegativeDecisions), UCUMCodes: len(s.UCUMCodes),
	}
}

// Adapter converts verified local artifacts into a normalized catalog
// snapshot. It must be deterministic and idempotent, preserve opaque native
// identifiers and provenance, and fail closed on unknown relations.
type Adapter interface {
	ID() string
	Version() string
	Convert(ctx context.Context, policy keywords.SourcePolicy, artifacts []VerifiedArtifact) (CatalogSnapshot, error)
}

var registeredAdapters = map[string]Adapter{}

// RegisterAdapter adds one adapter to the process-wide registry. Duplicate
// or blank IDs are rejected.
func RegisterAdapter(adapter Adapter) error {
	if adapter == nil {
		return errors.New("adapter is nil")
	}
	id := strings.TrimSpace(adapter.ID())
	if id == "" {
		return errors.New("adapter id is required")
	}
	if _, exists := registeredAdapters[id]; exists {
		return fmt.Errorf("adapter %q is already registered", id)
	}
	registeredAdapters[id] = adapter
	return nil
}

// LookupAdapter returns a previously registered adapter by ID.
func LookupAdapter(id string) (Adapter, bool) {
	adapter, ok := registeredAdapters[id]
	return adapter, ok
}

// ImportResult reports the immutable staging state for one source release.
type ImportResult struct {
	Source   string       `json:"source"`
	Release  string       `json:"release"`
	Counts   ImportCounts `json:"counts"`
	Replayed bool         `json:"replayed"`
}

// Runner imports governed local manifests into the immutable staging catalog.
type Runner struct {
	DB keywords.DBX
}

// Import verifies every artifact before opening a transaction, registers the
// source policy and artifacts, inserts or verifies the immutable staging
// snapshot, and reports stable counts. Changed content under an existing
// release is an error, never a replacement.
func (r Runner) Import(ctx context.Context, manifestPath string) (ImportResult, error) {
	manifest, artifacts, err := ParseAndVerifyManifest(manifestPath)
	if err != nil {
		return ImportResult{}, err
	}
	adapter, ok := LookupAdapter(manifest.Adapter)
	if !ok {
		return ImportResult{}, fmt.Errorf("unknown adapter %q", manifest.Adapter)
	}
	policy := manifest.Policy.SourcePolicy()
	snapshot, err := adapter.Convert(ctx, policy, artifacts)
	if err != nil {
		return ImportResult{}, fmt.Errorf("adapter %s: %w", manifest.Adapter, err)
	}
	if err := validateSnapshot(snapshot); err != nil {
		return ImportResult{}, err
	}
	normalizeSnapshot(&snapshot)

	replayed := false
	err = withImportTransaction(ctx, r.DB, func(tx *sql.Tx) error {
		if err := (keywords.SourcePolicyStore{DB: tx}).Register(ctx, policy); err != nil {
			return err
		}
		for _, artifact := range artifacts {
			if err := (keywords.SourcePolicyStore{DB: tx}).RegisterArtifact(ctx, keywords.SourceArtifact{
				Source: policy.Source, Release: policy.Release, ArtifactID: artifact.ID,
				ContentChecksum: artifact.SHA256, MediaType: artifact.MediaType,
				ProvenanceLocator: artifact.ProvenanceLocator, Payload: artifact.Metadata,
			}); err != nil {
				return err
			}
		}
		var err error
		replayed, err = importCatalog(ctx, tx, policy.Source, policy.Release, snapshot)
		return err
	})
	if err != nil {
		return ImportResult{}, err
	}
	return ImportResult{
		Source: policy.Source, Release: policy.Release,
		Counts: func() ImportCounts {
			counts := snapshot.Counts()
			counts.Artifacts = len(artifacts)
			return counts
		}(),
		Replayed: replayed,
	}, nil
}

func validateSnapshot(snapshot CatalogSnapshot) error {
	for i, e := range snapshot.Entries {
		if strings.TrimSpace(e.ExternalID) == "" {
			return fmt.Errorf("catalog entry[%d] external_id is required", i)
		}
	}
	for i, l := range snapshot.Labels {
		if strings.TrimSpace(l.ExternalID) == "" || strings.TrimSpace(l.Language) == "" ||
			strings.TrimSpace(l.LabelRole) == "" || strings.TrimSpace(l.Label) == "" {
			return fmt.Errorf("catalog label[%d] requires external_id, language, label_role, and label", i)
		}
	}
	for i, rel := range snapshot.Relations {
		if strings.TrimSpace(rel.SubjectExternalID) == "" || strings.TrimSpace(rel.Relation) == "" ||
			strings.TrimSpace(rel.ObjectExternalID) == "" {
			return fmt.Errorf("catalog relation[%d] requires subject, relation, and object", i)
		}
	}
	for i, neg := range snapshot.NegativeDecisions {
		if strings.TrimSpace(neg.SubjectExternalID) == "" || strings.TrimSpace(neg.ObjectExternalID) == "" ||
			strings.TrimSpace(neg.Relation) == "" || strings.TrimSpace(neg.Reason) == "" {
			return fmt.Errorf("negative decision[%d] requires subject, object, relation, and reason", i)
		}
	}
	for i, code := range snapshot.UCUMCodes {
		if strings.TrimSpace(code.Code) == "" {
			return fmt.Errorf("ucum code[%d] requires code", i)
		}
	}
	return nil
}

func normalizeSnapshot(snapshot *CatalogSnapshot) {
	normalizePayloads := func(payloads ...*json.RawMessage) {
		for _, payload := range payloads {
			*payload = canonicalJSON(*payload)
		}
	}
	sort.Slice(snapshot.Entries, func(i, j int) bool {
		return snapshot.Entries[i].ExternalID < snapshot.Entries[j].ExternalID
	})
	sort.Slice(snapshot.Labels, func(i, j int) bool {
		a, b := snapshot.Labels[i], snapshot.Labels[j]
		if a.ExternalID != b.ExternalID {
			return a.ExternalID < b.ExternalID
		}
		if a.Language != b.Language {
			return a.Language < b.Language
		}
		if a.LabelRole != b.LabelRole {
			return a.LabelRole < b.LabelRole
		}
		return a.Label < b.Label
	})
	sort.Slice(snapshot.Relations, func(i, j int) bool {
		a, b := snapshot.Relations[i], snapshot.Relations[j]
		if a.SubjectExternalID != b.SubjectExternalID {
			return a.SubjectExternalID < b.SubjectExternalID
		}
		if a.Relation != b.Relation {
			return a.Relation < b.Relation
		}
		if a.ObjectSource != b.ObjectSource {
			return a.ObjectSource < b.ObjectSource
		}
		if a.ObjectRelease != b.ObjectRelease {
			return a.ObjectRelease < b.ObjectRelease
		}
		return a.ObjectExternalID < b.ObjectExternalID
	})
	sort.Slice(snapshot.NegativeDecisions, func(i, j int) bool {
		a, b := snapshot.NegativeDecisions[i], snapshot.NegativeDecisions[j]
		if a.SubjectExternalID != b.SubjectExternalID {
			return a.SubjectExternalID < b.SubjectExternalID
		}
		if a.ObjectSource != b.ObjectSource {
			return a.ObjectSource < b.ObjectSource
		}
		if a.ObjectRelease != b.ObjectRelease {
			return a.ObjectRelease < b.ObjectRelease
		}
		if a.ObjectExternalID != b.ObjectExternalID {
			return a.ObjectExternalID < b.ObjectExternalID
		}
		return a.Relation < b.Relation
	})
	sort.Slice(snapshot.UCUMCodes, func(i, j int) bool {
		return snapshot.UCUMCodes[i].Code < snapshot.UCUMCodes[j].Code
	})
	for i := range snapshot.Entries {
		normalizePayloads(&snapshot.Entries[i].NativePayload)
	}
	for i := range snapshot.Labels {
		normalizePayloads(&snapshot.Labels[i].NativePayload)
	}
	for i := range snapshot.Relations {
		normalizePayloads(&snapshot.Relations[i].NativePayload)
	}
	for i := range snapshot.NegativeDecisions {
		normalizePayloads(&snapshot.NegativeDecisions[i].NativePayload)
	}
	for i := range snapshot.UCUMCodes {
		normalizePayloads(&snapshot.UCUMCodes[i].NativePayload)
	}
}

func canonicalJSON(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return json.RawMessage(`{}`)
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return raw
	}
	canonical, err := json.Marshal(value)
	if err != nil {
		return raw
	}
	return canonical
}

type txBeginner interface {
	BeginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error)
}

func withImportTransaction(ctx context.Context, db keywords.DBX, write func(*sql.Tx) error) error {
	if db == nil {
		return errors.New("import database is nil")
	}
	if tx, ok := db.(*sql.Tx); ok {
		return write(tx)
	}
	beginner, ok := db.(txBeginner)
	if !ok {
		return errors.New("import requires transaction-capable database")
	}
	tx, err := beginner.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := write(tx); err != nil {
		return err
	}
	return tx.Commit()
}

func importCatalog(ctx context.Context, tx *sql.Tx, source, release string, snapshot CatalogSnapshot) (bool, error) {
	inserted := 0
	insert := func(query string, args ...any) error {
		result, err := tx.ExecContext(ctx, query, args...)
		if err != nil {
			return err
		}
		if affected, _ := result.RowsAffected(); affected > 0 {
			inserted++
		}
		return nil
	}
	for _, e := range snapshot.Entries {
		if err := insert(`INSERT INTO kb.keyword_catalog_entries (source, release, external_id, entry_status, provenance_locator, native_payload) VALUES ($1,$2,$3,$4,$5,$6) ON CONFLICT (source, release, external_id) DO NOTHING`,
			source, release, e.ExternalID, e.EntryStatus, e.ProvenanceLocator, e.NativePayload); err != nil {
			return false, fmt.Errorf("insert catalog entry %q: %w", e.ExternalID, err)
		}
	}
	for _, l := range snapshot.Labels {
		if err := insert(`INSERT INTO kb.keyword_catalog_labels (source, release, external_id, language, label_role, label, provenance_locator, native_payload) VALUES ($1,$2,$3,$4,$5,$6,$7,$8) ON CONFLICT (source, release, external_id, language, label_role, label) DO NOTHING`,
			source, release, l.ExternalID, l.Language, l.LabelRole, l.Label, l.ProvenanceLocator, l.NativePayload); err != nil {
			return false, fmt.Errorf("insert catalog label %q: %w", l.Label, err)
		}
	}
	for _, rel := range snapshot.Relations {
		if err := insert(`INSERT INTO kb.keyword_catalog_relations (source, release, subject_external_id, relation, object_source, object_release, object_external_id, provenance_locator, native_payload) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9) ON CONFLICT (source, release, subject_external_id, relation, object_source, object_release, object_external_id) DO NOTHING`,
			source, release, rel.SubjectExternalID, rel.Relation, rel.ObjectSource, rel.ObjectRelease, rel.ObjectExternalID, rel.ProvenanceLocator, rel.NativePayload); err != nil {
			return false, fmt.Errorf("insert catalog relation %q: %w", rel.Relation, err)
		}
	}
	for _, neg := range snapshot.NegativeDecisions {
		if err := insert(`INSERT INTO kb.keyword_catalog_negative_decisions (source, release, subject_external_id, object_source, object_release, object_external_id, relation, reason, provenance_locator, native_payload) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) ON CONFLICT (source, release, subject_external_id, object_source, object_release, object_external_id, relation) DO NOTHING`,
			source, release, neg.SubjectExternalID, neg.ObjectSource, neg.ObjectRelease, neg.ObjectExternalID, neg.Relation, neg.Reason, neg.ProvenanceLocator, neg.NativePayload); err != nil {
			return false, fmt.Errorf("insert negative decision %q: %w", neg.Reason, err)
		}
	}
	for _, code := range snapshot.UCUMCodes {
		if err := insert(`INSERT INTO kb.keyword_ucum_codes (source, release, code, print_symbol, dimension, provenance_locator, native_payload) VALUES ($1,$2,$3,$4,$5,$6,$7) ON CONFLICT (source, release, code) DO NOTHING`,
			source, release, code.Code, code.PrintSymbol, code.Dimension, code.ProvenanceLocator, code.NativePayload); err != nil {
			return false, fmt.Errorf("insert ucum code %q: %w", code.Code, err)
		}
	}
	if err := verifyCatalog(ctx, tx, source, release, snapshot); err != nil {
		return false, err
	}
	return inserted == 0, nil
}

func verifyCatalog(ctx context.Context, tx *sql.Tx, source, release string, snapshot CatalogSnapshot) error {
	compareEntries := make(map[string]CatalogEntry, len(snapshot.Entries))
	for _, e := range snapshot.Entries {
		compareEntries[e.ExternalID] = e
	}
	rows, err := tx.QueryContext(ctx, `SELECT external_id, entry_status, provenance_locator, native_payload FROM kb.keyword_catalog_entries WHERE source = $1 AND release = $2`, source, release)
	if err != nil {
		return fmt.Errorf("read catalog entries: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var e CatalogEntry
		if err := rows.Scan(&e.ExternalID, &e.EntryStatus, &e.ProvenanceLocator, &e.NativePayload); err != nil {
			return fmt.Errorf("scan catalog entry: %w", err)
		}
		want, ok := compareEntries[e.ExternalID]
		if !ok {
			return fmt.Errorf("staging entry %q exists but is absent from this manifest", e.ExternalID)
		}
		e.NativePayload = canonicalJSON(e.NativePayload)
		if e.EntryStatus != want.EntryStatus || e.ProvenanceLocator != want.ProvenanceLocator || !reflect.DeepEqual(unjson(e.NativePayload), unjson(want.NativePayload)) {
			return fmt.Errorf("%w: entry %q differs from registered staging", keywords.ErrImmutableSourceArtifact, e.ExternalID)
		}
		delete(compareEntries, e.ExternalID)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate catalog entries: %w", err)
	}
	if len(compareEntries) != 0 {
		missing := make([]string, 0, len(compareEntries))
		for id := range compareEntries {
			missing = append(missing, id)
		}
		sort.Strings(missing)
		return fmt.Errorf("staging entries missing after import: %s", strings.Join(missing, ", "))
	}

	compareLabels := make(map[string]CatalogLabel, len(snapshot.Labels))
	for _, l := range snapshot.Labels {
		compareLabels[labelKey(l.ExternalID, l.Language, l.LabelRole, l.Label)] = l
	}
	labelRows, err := tx.QueryContext(ctx, `SELECT external_id, language, label_role, label, provenance_locator, native_payload FROM kb.keyword_catalog_labels WHERE source = $1 AND release = $2`, source, release)
	if err != nil {
		return fmt.Errorf("read catalog labels: %w", err)
	}
	defer labelRows.Close()
	for labelRows.Next() {
		var l CatalogLabel
		if err := labelRows.Scan(&l.ExternalID, &l.Language, &l.LabelRole, &l.Label, &l.ProvenanceLocator, &l.NativePayload); err != nil {
			return fmt.Errorf("scan catalog label: %w", err)
		}
		want, ok := compareLabels[labelKey(l.ExternalID, l.Language, l.LabelRole, l.Label)]
		if !ok {
			return fmt.Errorf("staging label %q/%q exists but is absent from this manifest", l.ExternalID, l.Label)
		}
		l.NativePayload = canonicalJSON(l.NativePayload)
		if l.ProvenanceLocator != want.ProvenanceLocator || !reflect.DeepEqual(unjson(l.NativePayload), unjson(want.NativePayload)) {
			return fmt.Errorf("%w: label %q/%q differs from registered staging", keywords.ErrImmutableSourceArtifact, l.ExternalID, l.Label)
		}
		delete(compareLabels, labelKey(l.ExternalID, l.Language, l.LabelRole, l.Label))
	}
	if err := labelRows.Err(); err != nil {
		return fmt.Errorf("iterate catalog labels: %w", err)
	}
	if len(compareLabels) != 0 {
		return fmt.Errorf("staging labels missing after import (%d)", len(compareLabels))
	}

	compareRelations := make(map[string]CatalogRelation, len(snapshot.Relations))
	for _, rel := range snapshot.Relations {
		compareRelations[relationKey(rel.SubjectExternalID, rel.Relation, rel.ObjectSource, rel.ObjectRelease, rel.ObjectExternalID)] = rel
	}
	relationRows, err := tx.QueryContext(ctx, `SELECT subject_external_id, relation, object_source, object_release, object_external_id, provenance_locator, native_payload FROM kb.keyword_catalog_relations WHERE source = $1 AND release = $2`, source, release)
	if err != nil {
		return fmt.Errorf("read catalog relations: %w", err)
	}
	defer relationRows.Close()
	for relationRows.Next() {
		var rel CatalogRelation
		if err := relationRows.Scan(&rel.SubjectExternalID, &rel.Relation, &rel.ObjectSource, &rel.ObjectRelease, &rel.ObjectExternalID, &rel.ProvenanceLocator, &rel.NativePayload); err != nil {
			return fmt.Errorf("scan catalog relation: %w", err)
		}
		want, ok := compareRelations[relationKey(rel.SubjectExternalID, rel.Relation, rel.ObjectSource, rel.ObjectRelease, rel.ObjectExternalID)]
		if !ok {
			return fmt.Errorf("staging relation %q exists but is absent from this manifest", rel.Relation)
		}
		rel.NativePayload = canonicalJSON(rel.NativePayload)
		if rel.ProvenanceLocator != want.ProvenanceLocator || !reflect.DeepEqual(unjson(rel.NativePayload), unjson(want.NativePayload)) {
			return fmt.Errorf("%w: relation %q differs from registered staging", keywords.ErrImmutableSourceArtifact, rel.Relation)
		}
		delete(compareRelations, relationKey(rel.SubjectExternalID, rel.Relation, rel.ObjectSource, rel.ObjectRelease, rel.ObjectExternalID))
	}
	if err := relationRows.Err(); err != nil {
		return fmt.Errorf("iterate catalog relations: %w", err)
	}
	if len(compareRelations) != 0 {
		return fmt.Errorf("staging relations missing after import (%d)", len(compareRelations))
	}

	compareNegatives := make(map[string]CatalogNegativeDecision, len(snapshot.NegativeDecisions))
	for _, neg := range snapshot.NegativeDecisions {
		compareNegatives[relationKey(neg.SubjectExternalID, neg.Relation, neg.ObjectSource, neg.ObjectRelease, neg.ObjectExternalID)] = neg
	}
	negativeRows, err := tx.QueryContext(ctx, `SELECT subject_external_id, object_source, object_release, object_external_id, relation, reason, provenance_locator, native_payload FROM kb.keyword_catalog_negative_decisions WHERE source = $1 AND release = $2`, source, release)
	if err != nil {
		return fmt.Errorf("read negative decisions: %w", err)
	}
	defer negativeRows.Close()
	for negativeRows.Next() {
		var neg CatalogNegativeDecision
		if err := negativeRows.Scan(&neg.SubjectExternalID, &neg.ObjectSource, &neg.ObjectRelease, &neg.ObjectExternalID, &neg.Relation, &neg.Reason, &neg.ProvenanceLocator, &neg.NativePayload); err != nil {
			return fmt.Errorf("scan negative decision: %w", err)
		}
		want, ok := compareNegatives[relationKey(neg.SubjectExternalID, neg.Relation, neg.ObjectSource, neg.ObjectRelease, neg.ObjectExternalID)]
		if !ok {
			return fmt.Errorf("staging negative decision %q exists but is absent from this manifest", neg.Reason)
		}
		neg.NativePayload = canonicalJSON(neg.NativePayload)
		if neg.Reason != want.Reason || neg.ProvenanceLocator != want.ProvenanceLocator || !reflect.DeepEqual(unjson(neg.NativePayload), unjson(want.NativePayload)) {
			return fmt.Errorf("%w: negative decision %q differs from registered staging", keywords.ErrImmutableSourceArtifact, neg.Reason)
		}
		delete(compareNegatives, relationKey(neg.SubjectExternalID, neg.Relation, neg.ObjectSource, neg.ObjectRelease, neg.ObjectExternalID))
	}
	if err := negativeRows.Err(); err != nil {
		return fmt.Errorf("iterate negative decisions: %w", err)
	}
	if len(compareNegatives) != 0 {
		return fmt.Errorf("staging negative decisions missing after import (%d)", len(compareNegatives))
	}

	compareUCUM := make(map[string]UCUMCode, len(snapshot.UCUMCodes))
	for _, code := range snapshot.UCUMCodes {
		compareUCUM[code.Code] = code
	}
	ucumRows, err := tx.QueryContext(ctx, `SELECT code, print_symbol, dimension, provenance_locator, native_payload FROM kb.keyword_ucum_codes WHERE source = $1 AND release = $2`, source, release)
	if err != nil {
		return fmt.Errorf("read ucum codes: %w", err)
	}
	defer ucumRows.Close()
	for ucumRows.Next() {
		var code UCUMCode
		if err := ucumRows.Scan(&code.Code, &code.PrintSymbol, &code.Dimension, &code.ProvenanceLocator, &code.NativePayload); err != nil {
			return fmt.Errorf("scan ucum code: %w", err)
		}
		want, ok := compareUCUM[code.Code]
		if !ok {
			return fmt.Errorf("staging ucum code %q exists but is absent from this manifest", code.Code)
		}
		code.NativePayload = canonicalJSON(code.NativePayload)
		if code.PrintSymbol != want.PrintSymbol || code.Dimension != want.Dimension ||
			code.ProvenanceLocator != want.ProvenanceLocator || !reflect.DeepEqual(unjson(code.NativePayload), unjson(want.NativePayload)) {
			return fmt.Errorf("%w: ucum code %q differs from registered staging", keywords.ErrImmutableSourceArtifact, code.Code)
		}
		delete(compareUCUM, code.Code)
	}
	if err := ucumRows.Err(); err != nil {
		return fmt.Errorf("iterate ucum codes: %w", err)
	}
	if len(compareUCUM) != 0 {
		return fmt.Errorf("staging ucum codes missing after import (%d)", len(compareUCUM))
	}
	return nil
}

func unjson(raw json.RawMessage) any {
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return string(raw)
	}
	return value
}

func labelKey(externalID, language, labelRole, label string) string {
	return strings.Join([]string{externalID, language, labelRole, label}, "\x00")
}

func relationKey(subject, relation, objectSource, objectRelease, objectExternalID string) string {
	return strings.Join([]string{subject, relation, objectSource, objectRelease, objectExternalID}, "\x00")
}

// ReleaseContent is one immutable release's policy, artifacts, and staged
// catalog, used as input to a release diff.
type ReleaseContent struct {
	Policy    keywords.SourcePolicy
	Artifacts []VerifiedArtifact
	Snapshot  CatalogSnapshot
}

// DiffItem is one added, retired, replaced, or changed row keyed by a
// canonical stable key.
type DiffItem struct {
	Key  string `json:"key"`
	From string `json:"from,omitempty"`
	To   string `json:"to,omitempty"`
}

// PolicyChange records one field-level governance difference.
type PolicyChange struct {
	Field string `json:"field"`
	From  string `json:"from"`
	To    string `json:"to"`
}

// ReleaseDiff reports what changes before a candidate release may be
// approved. It is a pure comparison and never writes.
type ReleaseDiff struct {
	Source                   string         `json:"source"`
	BaseRelease              string         `json:"base_release"`
	CandidateRelease         string         `json:"candidate_release"`
	AddedEntries             []DiffItem     `json:"added_entries,omitempty"`
	RetiredEntries           []DiffItem     `json:"retired_entries,omitempty"`
	ReplacedEntries          []DiffItem     `json:"replaced_entries,omitempty"`
	AddedLabels              []DiffItem     `json:"added_labels,omitempty"`
	RetiredLabels            []DiffItem     `json:"retired_labels,omitempty"`
	ChangedLabels            []DiffItem     `json:"changed_labels,omitempty"`
	AddedRelations           []DiffItem     `json:"added_relations,omitempty"`
	RetiredRelations         []DiffItem     `json:"retired_relations,omitempty"`
	ChangedRelations         []DiffItem     `json:"changed_relations,omitempty"`
	AddedNegativeDecisions   []DiffItem     `json:"added_negative_decisions,omitempty"`
	RetiredNegativeDecisions []DiffItem     `json:"retired_negative_decisions,omitempty"`
	ChangedNegativeDecisions []DiffItem     `json:"changed_negative_decisions,omitempty"`
	AddedUCUMCodes           []DiffItem     `json:"added_ucum_codes,omitempty"`
	RetiredUCUMCodes         []DiffItem     `json:"retired_ucum_codes,omitempty"`
	ChangedUCUMCodes         []DiffItem     `json:"changed_ucum_codes,omitempty"`
	AddedArtifacts           []DiffItem     `json:"added_artifacts,omitempty"`
	RetiredArtifacts         []DiffItem     `json:"retired_artifacts,omitempty"`
	ChangedArtifacts         []DiffItem     `json:"changed_artifacts,omitempty"`
	PolicyChanges            []PolicyChange `json:"policy_changes,omitempty"`
}

// DiffReleases compares two immutable releases of the same source and
// reports additions, retirements, replacements, and field changes.
func DiffReleases(base, candidate ReleaseContent) ReleaseDiff {
	diff := ReleaseDiff{
		Source: base.Policy.Source, BaseRelease: base.Policy.Release, CandidateRelease: candidate.Policy.Release,
	}
	diff.AddedEntries = addedDiff(base.Snapshot.Entries, candidate.Snapshot.Entries, func(e CatalogEntry) (string, string) {
		return e.ExternalID, e.EntryStatus
	})
	diff.RetiredEntries = retiredDiff(base.Snapshot.Entries, candidate.Snapshot.Entries, func(e CatalogEntry) (string, string) {
		return e.ExternalID, e.EntryStatus
	})
	diff.ReplacedEntries = changedDiff(base.Snapshot.Entries, candidate.Snapshot.Entries, func(e CatalogEntry) string {
		return e.ExternalID
	}, func(e CatalogEntry) string {
		return entryValue(e)
	})

	diff.AddedLabels = addedDiff(base.Snapshot.Labels, candidate.Snapshot.Labels, func(l CatalogLabel) (string, string) {
		return labelKey(l.ExternalID, l.Language, l.LabelRole, l.Label), l.Label
	})
	diff.RetiredLabels = retiredDiff(base.Snapshot.Labels, candidate.Snapshot.Labels, func(l CatalogLabel) (string, string) {
		return labelKey(l.ExternalID, l.Language, l.LabelRole, l.Label), l.Label
	})
	diff.ChangedLabels = changedDiff(base.Snapshot.Labels, candidate.Snapshot.Labels, func(l CatalogLabel) string {
		return labelKey(l.ExternalID, l.Language, l.LabelRole, "")
	}, func(l CatalogLabel) string {
		return l.Label
	})

	diff.AddedRelations = addedDiff(base.Snapshot.Relations, candidate.Snapshot.Relations, func(r CatalogRelation) (string, string) {
		return relationKey(r.SubjectExternalID, r.Relation, r.ObjectSource, r.ObjectRelease, r.ObjectExternalID), r.Relation
	})
	diff.RetiredRelations = retiredDiff(base.Snapshot.Relations, candidate.Snapshot.Relations, func(r CatalogRelation) (string, string) {
		return relationKey(r.SubjectExternalID, r.Relation, r.ObjectSource, r.ObjectRelease, r.ObjectExternalID), r.Relation
	})
	diff.ChangedRelations = changedDiff(base.Snapshot.Relations, candidate.Snapshot.Relations, func(r CatalogRelation) string {
		return relationKey(r.SubjectExternalID, r.Relation, r.ObjectSource, r.ObjectRelease, r.ObjectExternalID)
	}, func(r CatalogRelation) string {
		return relationValue(r)
	})

	diff.AddedNegativeDecisions = addedDiff(base.Snapshot.NegativeDecisions, candidate.Snapshot.NegativeDecisions, func(n CatalogNegativeDecision) (string, string) {
		return relationKey(n.SubjectExternalID, n.Relation, n.ObjectSource, n.ObjectRelease, n.ObjectExternalID), n.Reason
	})
	diff.RetiredNegativeDecisions = retiredDiff(base.Snapshot.NegativeDecisions, candidate.Snapshot.NegativeDecisions, func(n CatalogNegativeDecision) (string, string) {
		return relationKey(n.SubjectExternalID, n.Relation, n.ObjectSource, n.ObjectRelease, n.ObjectExternalID), n.Reason
	})
	diff.ChangedNegativeDecisions = changedDiff(base.Snapshot.NegativeDecisions, candidate.Snapshot.NegativeDecisions, func(n CatalogNegativeDecision) string {
		return relationKey(n.SubjectExternalID, n.Relation, n.ObjectSource, n.ObjectRelease, n.ObjectExternalID)
	}, func(n CatalogNegativeDecision) string {
		return n.Reason
	})

	diff.AddedUCUMCodes = addedDiff(base.Snapshot.UCUMCodes, candidate.Snapshot.UCUMCodes, func(c UCUMCode) (string, string) {
		return c.Code, c.PrintSymbol
	})
	diff.RetiredUCUMCodes = retiredDiff(base.Snapshot.UCUMCodes, candidate.Snapshot.UCUMCodes, func(c UCUMCode) (string, string) {
		return c.Code, c.PrintSymbol
	})
	diff.ChangedUCUMCodes = changedDiff(base.Snapshot.UCUMCodes, candidate.Snapshot.UCUMCodes, func(c UCUMCode) string {
		return c.Code
	}, func(c UCUMCode) string {
		return c.PrintSymbol
	})

	diff.AddedArtifacts = addedDiff(base.Artifacts, candidate.Artifacts, func(a VerifiedArtifact) (string, string) {
		return a.ID, a.SHA256
	})
	diff.RetiredArtifacts = retiredDiff(base.Artifacts, candidate.Artifacts, func(a VerifiedArtifact) (string, string) {
		return a.ID, a.SHA256
	})
	diff.ChangedArtifacts = changedDiff(base.Artifacts, candidate.Artifacts, func(a VerifiedArtifact) string {
		return a.ID
	}, func(a VerifiedArtifact) string {
		return a.SHA256
	})
	diff.PolicyChanges = diffPolicy(base.Policy, candidate.Policy)
	return diff
}

func addedDiff[T any](base, candidate []T, key func(T) (string, string)) []DiffItem {
	baseKeys := map[string]bool{}
	for _, item := range base {
		key, _ := key(item)
		baseKeys[key] = true
	}
	var out []DiffItem
	for _, item := range candidate {
		key, value := key(item)
		if !baseKeys[key] {
			out = append(out, DiffItem{Key: key, To: value})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}

func retiredDiff[T any](base, candidate []T, key func(T) (string, string)) []DiffItem {
	candidateKeys := map[string]bool{}
	for _, item := range candidate {
		key, _ := key(item)
		candidateKeys[key] = true
	}
	var out []DiffItem
	for _, item := range base {
		key, value := key(item)
		if !candidateKeys[key] {
			out = append(out, DiffItem{Key: key, From: value})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}

func changedDiff[T any](base, candidate []T, key func(T) string, value func(T) string) []DiffItem {
	candidateByKey := map[string]T{}
	for _, item := range candidate {
		candidateByKey[key(item)] = item
	}
	baseByKey := map[string]T{}
	for _, item := range base {
		baseByKey[key(item)] = item
	}
	var out []DiffItem
	for k, item := range baseByKey {
		if other, ok := candidateByKey[k]; ok && value(item) != value(other) {
			out = append(out, DiffItem{Key: k, From: value(item), To: value(other)})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}

func entryValue(e CatalogEntry) string {
	return fmt.Sprintf("%s|%s|%s", e.EntryStatus, e.ProvenanceLocator, string(canonicalJSON(e.NativePayload)))
}

func relationValue(r CatalogRelation) string {
	return fmt.Sprintf("%s|%s|%s", r.ProvenanceLocator, r.ObjectRelease, string(canonicalJSON(r.NativePayload)))
}

func diffPolicy(base, candidate keywords.SourcePolicy) []PolicyChange {
	from := map[string]string{
		"provider_id": base.ProviderID, "source": base.Source, "source_subset": base.SourceSubset,
		"release": base.Release, "retrieved_at": base.RetrievedAt.String(), "content_checksum": base.ContentChecksum,
		"license": base.License, "license_review_status": base.LicenseReviewStatus,
		"authority_role": string(base.AuthorityRole), "authoritative_relations": strings.Join(base.AuthoritativeRelations, ","),
		"allowed_scopes": strings.Join(base.AllowedScopes, ","), "languages": strings.Join(base.Languages, ","),
		"adapter_version": base.AdapterVersion, "provenance_locator": base.ProvenanceLocator,
		"approved_by": base.ApprovedBy, "identity_authority": fmt.Sprintf("%t", base.IdentityAuthority),
		"notes": base.Notes,
	}
	if base.ApprovedAt != nil {
		from["approved_at"] = base.ApprovedAt.String()
	}
	to := map[string]string{
		"provider_id": candidate.ProviderID, "source": candidate.Source, "source_subset": candidate.SourceSubset,
		"release": candidate.Release, "retrieved_at": candidate.RetrievedAt.String(), "content_checksum": candidate.ContentChecksum,
		"license": candidate.License, "license_review_status": candidate.LicenseReviewStatus,
		"authority_role": string(candidate.AuthorityRole), "authoritative_relations": strings.Join(candidate.AuthoritativeRelations, ","),
		"allowed_scopes": strings.Join(candidate.AllowedScopes, ","), "languages": strings.Join(candidate.Languages, ","),
		"adapter_version": candidate.AdapterVersion, "provenance_locator": candidate.ProvenanceLocator,
		"approved_by": candidate.ApprovedBy, "identity_authority": fmt.Sprintf("%t", candidate.IdentityAuthority),
		"notes": candidate.Notes,
	}
	if candidate.ApprovedAt != nil {
		to["approved_at"] = candidate.ApprovedAt.String()
	}
	var changes []PolicyChange
	for field := range from {
		if from[field] != to[field] {
			changes = append(changes, PolicyChange{Field: field, From: from[field], To: to[field]})
		}
	}
	sort.Slice(changes, func(i, j int) bool { return changes[i].Field < changes[j].Field })
	return changes
}

var (
	ErrDeploymentReleaseAbsent     = errors.New("deployment release is not registered")
	ErrDeploymentReleaseUnapproved = errors.New("deployment release is not approved")
	ErrDeploymentNoPriorRelease    = errors.New("deployment has no prior release to roll back to")
	ErrDeploymentNotEnabled        = errors.New("deployment is not enabled")
)

// DeploymentStore moves the audited deployment pointer. It never rewrites
// source policy, artifacts, or historical events.
type DeploymentStore struct {
	DB keywords.DBX
}

// Activate points one deployment key at an approved, registered release and
// appends an audited activation event. Both writes share one transaction and
// the keyword family advisory lock.
func (s DeploymentStore) Activate(ctx context.Context, deploymentKey, source, release, changedBy string) error {
	if strings.TrimSpace(deploymentKey) == "" || strings.TrimSpace(source) == "" ||
		strings.TrimSpace(release) == "" || strings.TrimSpace(changedBy) == "" {
		return errors.New("activation requires deployment_key, source, release, and changed_by")
	}
	return withDeploymentTransaction(ctx, s.DB, func(tx *sql.Tx) error {
		var approvedAt sql.NullTime
		var licenseReview string
		err := tx.QueryRowContext(ctx, `SELECT approved_at, license_review_status FROM kb.keyword_sources WHERE source = $1 AND release = $2`, source, release).
			Scan(&approvedAt, &licenseReview)
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("%w: %s/%s", ErrDeploymentReleaseAbsent, source, release)
		}
		if err != nil {
			return fmt.Errorf("read source policy for activation: %w", err)
		}
		if !approvedAt.Valid || licenseReview != keywords.LicenseReviewApproved {
			return fmt.Errorf("%w: %s/%s", ErrDeploymentReleaseUnapproved, source, release)
		}
		return writeDeployment(ctx, tx, deploymentKey, source, release, true, "activate", changedBy)
	})
}

// Rollback appends a compensating audited event and advances the pointer to
// the prior immutable release recorded in history.
func (s DeploymentStore) Rollback(ctx context.Context, deploymentKey, changedBy string) error {
	if strings.TrimSpace(deploymentKey) == "" || strings.TrimSpace(changedBy) == "" {
		return errors.New("rollback requires deployment_key and changed_by")
	}
	return withDeploymentTransaction(ctx, s.DB, func(tx *sql.Tx) error {
		var source, release string
		var enabled bool
		err := tx.QueryRowContext(ctx, `SELECT source, release, enabled FROM kb.keyword_identity_deployments WHERE deployment_key = $1 FOR UPDATE`, deploymentKey).
			Scan(&source, &release, &enabled)
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("%w: %s", ErrDeploymentNoPriorRelease, deploymentKey)
		}
		if err != nil {
			return fmt.Errorf("read deployment pointer: %w", err)
		}
		if !enabled {
			return fmt.Errorf("%w: %s", ErrDeploymentNotEnabled, deploymentKey)
		}
		prior, err := priorDeploymentState(ctx, tx, deploymentKey, source, release, enabled)
		if err != nil {
			return err
		}
		if prior == nil {
			return fmt.Errorf("%w: %s", ErrDeploymentNoPriorRelease, deploymentKey)
		}
		return writeDeployment(ctx, tx, deploymentKey, prior.source, prior.release, prior.enabled, "rollback", changedBy)
	})
}

type deploymentState struct {
	source  string
	release string
	enabled bool
}

func priorDeploymentState(ctx context.Context, tx *sql.Tx, deploymentKey, source, release string, enabled bool) (*deploymentState, error) {
	rows, err := tx.QueryContext(ctx, `SELECT source, release, enabled FROM kb.keyword_identity_deployment_history WHERE deployment_key = $1 ORDER BY history_id DESC`, deploymentKey)
	if err != nil {
		return nil, fmt.Errorf("read deployment history: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var state deploymentState
		if err := rows.Scan(&state.source, &state.release, &state.enabled); err != nil {
			return nil, fmt.Errorf("scan deployment history: %w", err)
		}
		if state.source == source && state.release == release && state.enabled == enabled {
			continue
		}
		if !state.enabled {
			return nil, nil
		}
		return &state, nil
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate deployment history: %w", err)
	}
	return nil, nil
}

func withDeploymentTransaction(ctx context.Context, db keywords.DBX, write func(*sql.Tx) error) error {
	if db == nil {
		return errors.New("deployment database is nil")
	}
	if tx, ok := db.(*sql.Tx); ok {
		if err := semid.AcquireKeywordIdentityMutationLock(ctx, tx); err != nil {
			return err
		}
		return write(tx)
	}
	beginner, ok := db.(txBeginner)
	if !ok {
		return errors.New("deployment requires transaction-capable database")
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

func writeDeployment(ctx context.Context, tx *sql.Tx, deploymentKey, source, release string, enabled bool, action, changedBy string) error {
	if _, err := tx.ExecContext(ctx, `INSERT INTO kb.keyword_identity_deployments (deployment_key, source, release, enabled, changed_by) VALUES ($1,$2,$3,$4,$5) ON CONFLICT (deployment_key) DO UPDATE SET source = EXCLUDED.source, release = EXCLUDED.release, enabled = EXCLUDED.enabled, changed_by = EXCLUDED.changed_by, changed_at = now()`,
		deploymentKey, source, release, enabled, changedBy,
	); err != nil {
		return fmt.Errorf("write source deployment pointer: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO kb.keyword_identity_deployment_history (deployment_key, source, release, enabled, action, changed_by) VALUES ($1,$2,$3,$4,$5,$6)`,
		deploymentKey, source, release, enabled, action, changedBy,
	); err != nil {
		return fmt.Errorf("append source deployment history: %w", err)
	}
	return nil
}
