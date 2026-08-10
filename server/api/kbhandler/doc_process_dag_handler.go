package kbhandler

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	docprocessing "github.com/chendingplano/deepdoc/server/api/doc-processing"
	"github.com/chendingplano/deepdoc/server/api/ontology/semrules"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/EchoFactory"
	"github.com/labstack/echo/v4"
	"github.com/lib/pq"
)

// A Doc Process DAG (ADR 2026081001 DR10) is the composite object this
// handler manages as one unit: a named doc process pipeline (the current
// kb.pipelines version for that name), its processor gates + DAG edges
// (kb.pipeline_rules), and — read-only here — any knowledge-store bindings
// (kb.pipeline_bindings) that reference it.

type dagRuleRecord struct {
	ID                  int64           `json:"id"`
	Name                string          `json:"name"`
	Priority            int             `json:"priority"`
	TargetProcessor     string          `json:"target_processor,omitempty"`
	Effect              string          `json:"effect,omitempty"`
	Predicate           json.RawMessage `json:"predicate,omitempty"`
	PredicateChecksum   string          `json:"predicate_checksum,omitempty"`
	RequiredFacets      []string        `json:"required_facets,omitempty"`
	DependsOnProcessors []string        `json:"depends_on_processors,omitempty"`
	Active              bool            `json:"active"`
	CreateTime          time.Time       `json:"create_time"`
	ModifyTime          time.Time       `json:"modify_time"`
}

type dagBindingRecord struct {
	ID            int64           `json:"id"`
	Name          string          `json:"name,omitempty"`
	Priority      int             `json:"priority"`
	KSStoreID     int64           `json:"ks_store_id,omitempty"`
	PipelineID    int64           `json:"pipeline_id"`
	BindingKind   string          `json:"binding_kind"`
	Predicate     json.RawMessage `json:"predicate,omitempty"`
	Active        bool            `json:"active"`
	TenantID      string          `json:"tenant_id,omitempty"`
	UserID        string          `json:"user_id,omitempty"`
	InputRecordID int64           `json:"input_record_id,omitempty"`
	CreateTime    time.Time       `json:"create_time"`
	ModifyTime    time.Time       `json:"modify_time"`
}

type dagPipelineRecord struct {
	ID               int64     `json:"id"`
	Name             string    `json:"name"`
	DisplayName      *string   `json:"display_name,omitempty"`
	Description      *string   `json:"description,omitempty"`
	Processors       []string  `json:"processors,omitempty"`
	LegacyEquivalent bool      `json:"legacy_equivalent"`
	IsSystemDefault  bool      `json:"is_system_default"`
	Version          int       `json:"version"`
	Status           string    `json:"status"`
	RuleCount        int       `json:"rule_count"`
	CreateTime       time.Time `json:"create_time"`
	ModifyTime       time.Time `json:"modify_time"`
}

type dagDetailRecord struct {
	dagPipelineRecord
	Rules    []dagRuleRecord    `json:"rules,omitempty"`
	Bindings []dagBindingRecord `json:"bindings,omitempty"`
}

type listDAGsResponse struct {
	Status  bool                `json:"status"`
	Results []dagPipelineRecord `json:"results"`
	Total   int                 `json:"total"`
}

type dagDetailResponse struct {
	Status bool            `json:"status"`
	Record dagDetailRecord `json:"record"`
}

type dagDeleteResponse struct {
	Status  bool `json:"status"`
	Deleted int  `json:"deleted"`
}

const dagPipelineColumns = `
    id, name, display_name, description, processors, legacy_equivalent,
    is_system_default, version, status, create_time, modify_time
FROM kb.pipelines`

const dagListQuery = `
SELECT DISTINCT ON (p.name)
    p.id, p.name, p.display_name, p.description, p.processors, p.legacy_equivalent,
    p.is_system_default, p.version, p.status, p.create_time, p.modify_time,
    (SELECT COUNT(*) FROM kb.pipeline_rules r WHERE r.pipeline_id = p.id AND r.active) AS rule_count
FROM kb.pipelines p
WHERE $1 = '' OR p.name ILIKE '%' || $1 || '%' OR COALESCE(p.display_name, '') ILIKE '%' || $1 || '%'
ORDER BY p.name, p.version DESC`

func scanDAGPipelineRecord(scan func(dest ...any) error, withRuleCount bool) (dagPipelineRecord, error) {
	var (
		record      dagPipelineRecord
		displayName sql.NullString
		description sql.NullString
		processors  pq.StringArray
	)
	dests := []any{
		&record.ID, &record.Name, &displayName, &description, &processors, &record.LegacyEquivalent,
		&record.IsSystemDefault, &record.Version, &record.Status, &record.CreateTime, &record.ModifyTime,
	}
	if withRuleCount {
		dests = append(dests, &record.RuleCount)
	}
	if err := scan(dests...); err != nil {
		return dagPipelineRecord{}, err
	}
	if displayName.Valid && strings.TrimSpace(displayName.String) != "" {
		v := displayName.String
		record.DisplayName = &v
	}
	if description.Valid && strings.TrimSpace(description.String) != "" {
		v := description.String
		record.Description = &v
	}
	record.Processors = []string(processors)
	return record, nil
}

func fetchCurrentDAGByName(db *sql.DB, name string) (dagPipelineRecord, error) {
	query := "SELECT" + dagPipelineColumns + "\nWHERE name = $1 ORDER BY version DESC LIMIT 1"
	row := db.QueryRow(query, name)
	return scanDAGPipelineRecord(row.Scan, false)
}

const dagRulesQuery = `
SELECT id, name, priority,
       COALESCE(predicate, '{}'::jsonb)::text, COALESCE(predicate_checksum, ''),
       COALESCE(target_processor, ''), COALESCE(effect, ''),
       COALESCE(required_facets, '[]'::jsonb)::text, depends_on_processors,
       active, create_time, modify_time
FROM kb.pipeline_rules
WHERE pipeline_id = $1
ORDER BY priority DESC, id`

func scanDAGRuleRecord(scan func(dest ...any) error) (dagRuleRecord, error) {
	var (
		record       dagRuleRecord
		predicateRaw string
		facetsRaw    string
		depends      pq.StringArray
	)
	if err := scan(
		&record.ID, &record.Name, &record.Priority, &predicateRaw, &record.PredicateChecksum,
		&record.TargetProcessor, &record.Effect, &facetsRaw, &depends,
		&record.Active, &record.CreateTime, &record.ModifyTime,
	); err != nil {
		return dagRuleRecord{}, err
	}
	record.DependsOnProcessors = []string(depends)
	if strings.TrimSpace(predicateRaw) != "" && strings.TrimSpace(predicateRaw) != "{}" {
		record.Predicate = json.RawMessage(predicateRaw)
	}
	if err := json.Unmarshal([]byte(facetsRaw), &record.RequiredFacets); err != nil {
		return dagRuleRecord{}, err
	}
	return record, nil
}

func fetchVersionRules(db *sql.DB, pipelineID int64) ([]dagRuleRecord, error) {
	rows, err := db.Query(dagRulesQuery, pipelineID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	records := make([]dagRuleRecord, 0)
	for rows.Next() {
		record, err := scanDAGRuleRecord(rows.Scan)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, rows.Err()
}

const dagBindingsQuery = `
SELECT b.id, COALESCE(b.name, ''), b.priority, COALESCE(b.ks_store_id, 0),
       b.pipeline_id, b.binding_kind, COALESCE(b.predicate, '{}'::jsonb)::text,
       b.active, COALESCE(b.tenant_id, ''), COALESCE(b.user_id, ''), COALESCE(b.input_record_id, 0),
       b.create_time, b.modify_time
FROM kb.pipeline_bindings b
WHERE b.pipeline_id = $1
ORDER BY b.id`

func scanDAGBindingRecord(scan func(dest ...any) error) (dagBindingRecord, error) {
	var (
		record       dagBindingRecord
		predicateRaw string
	)
	if err := scan(
		&record.ID, &record.Name, &record.Priority, &record.KSStoreID,
		&record.PipelineID, &record.BindingKind, &predicateRaw,
		&record.Active, &record.TenantID, &record.UserID, &record.InputRecordID,
		&record.CreateTime, &record.ModifyTime,
	); err != nil {
		return dagBindingRecord{}, err
	}
	if strings.TrimSpace(predicateRaw) != "" && strings.TrimSpace(predicateRaw) != "{}" {
		record.Predicate = json.RawMessage(predicateRaw)
	}
	return record, nil
}

func fetchVersionBindings(db *sql.DB, pipelineID int64) ([]dagBindingRecord, error) {
	rows, err := db.Query(dagBindingsQuery, pipelineID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	records := make([]dagBindingRecord, 0)
	for rows.Next() {
		record, err := scanDAGBindingRecord(rows.Scan)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, rows.Err()
}

// ListDocProcessDAGs handles GET /api/v1/kb/doc-process-dags, optionally
// filtered by ?search= (ILIKE on name/display_name). One entry per pipeline
// name, using its highest (current) version.
func ListDocProcessDAGs(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_DAG_001")
	defer rc.Close()
	logger := rc.GetLogger()

	search := strings.TrimSpace(c.QueryParam("search"))

	db := ApiTypes.ProjectDBHandle
	rows, err := db.Query(dagListQuery, search)
	if err != nil {
		logger.Error("query doc process dags failed", "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to retrieve doc process dags (CWB_KB_DAG_002)"})
	}
	defer rows.Close()

	results := make([]dagPipelineRecord, 0)
	for rows.Next() {
		record, err := scanDAGPipelineRecord(rows.Scan, true)
		if err != nil {
			logger.Error("scan doc process dag failed", "err", err)
			return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to scan doc process dags (CWB_KB_DAG_003)"})
		}
		results = append(results, record)
	}
	if err := rows.Err(); err != nil {
		logger.Error("iterate doc process dags failed", "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to iterate doc process dags (CWB_KB_DAG_004)"})
	}

	return c.JSON(http.StatusOK, listDAGsResponse{Status: true, Results: results, Total: len(results)})
}

// GetDocProcessDAG handles GET /api/v1/kb/doc-process-dags/:name, returning
// the DAG's current version with its rules (gates + DAG edges) and bindings.
func GetDocProcessDAG(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_DAG_010")
	defer rc.Close()
	logger := rc.GetLogger()

	name := strings.TrimSpace(c.Param("name"))
	if name == "" {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "name is required (CWB_KB_DAG_011)"})
	}

	db := ApiTypes.ProjectDBHandle
	pipeline, err := fetchCurrentDAGByName(db, name)
	if err == sql.ErrNoRows {
		return c.JSON(http.StatusNotFound, errorResponse{Status: false, ErrorMsg: "doc process dag not found (CWB_KB_DAG_012)"})
	}
	if err != nil {
		logger.Error("fetch doc process dag failed", "name", name, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to retrieve doc process dag (CWB_KB_DAG_013)"})
	}

	detail := dagDetailRecord{dagPipelineRecord: pipeline}
	if detail.Rules, err = fetchVersionRules(db, pipeline.ID); err != nil {
		logger.Error("fetch doc process dag rules failed", "id", pipeline.ID, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to retrieve doc process dag rules (CWB_KB_DAG_014)"})
	}
	if detail.Bindings, err = fetchVersionBindings(db, pipeline.ID); err != nil {
		logger.Error("fetch doc process dag bindings failed", "id", pipeline.ID, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to retrieve doc process dag bindings (CWB_KB_DAG_015)"})
	}

	return c.JSON(http.StatusOK, dagDetailResponse{Status: true, Record: detail})
}

// buildGatesFromDrafts converts rule draft payloads into PipelineGates,
// validating predicates exactly as CreatePipeline does (ADR 2026081001 DR8
// validation runs on the resulting gates before anything is written).
func buildGatesFromDrafts(drafts []pipelineRuleDraftPayload) ([]docprocessing.PipelineGate, error) {
	gates := make([]docprocessing.PipelineGate, 0, len(drafts))
	for _, draft := range drafts {
		gate := docprocessing.PipelineGate{
			Name:                draft.Name,
			Priority:            draft.Priority,
			TargetProcessor:     draft.TargetProcessor,
			Effect:              draft.Effect,
			Active:              true,
			DependsOnProcessors: draft.DependsOnProcessors,
		}
		if len(draft.Predicate) > 0 {
			if err := json.Unmarshal(draft.Predicate, &gate.Predicate); err != nil || semrules.Validate(gate.Predicate) != nil {
				return nil, fmt.Errorf("invalid predicate for rule %q", draft.Name)
			}
			_, checksum, err := semrules.Canonicalize(gate.Predicate)
			if err != nil {
				return nil, fmt.Errorf("invalid predicate for rule %q", draft.Name)
			}
			gate.PredicateChecksum = checksum
			gate.RequiredFacets = semrules.Analyze(gate.Predicate).RequiredDocumentFacets
		}
		gates = append(gates, gate)
	}
	return gates, nil
}

// createPipelineVersion is the shared atomic authoring path used by both
// create and content-changing update: lock the prior version (if any),
// insert the new version row, supersede the prior, and insert every rule —
// all in one transaction (ADR 2026081001 DR2).
func createPipelineVersion(tx *sql.Tx, name string, displayName, description any, processors []string, legacyEquivalent, isSystemDefault bool, gates []docprocessing.PipelineGate) (int64, error) {
	var priorID sql.NullInt64
	var priorVersion int
	err := tx.QueryRow(`SELECT id, version FROM kb.pipelines WHERE name = $1 ORDER BY version DESC LIMIT 1 FOR UPDATE`, name).Scan(&priorID, &priorVersion)
	if err != nil && err != sql.ErrNoRows {
		return 0, err
	}
	nextVersion := priorVersion + 1

	var id int64
	const insertQuery = `
INSERT INTO kb.pipelines (
    name, display_name, description, processors, legacy_equivalent, is_system_default, version, status
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, 'active'
)
RETURNING id
`
	if err := tx.QueryRow(insertQuery, name, displayName, description, pq.Array(processors), legacyEquivalent, isSystemDefault, nextVersion).Scan(&id); err != nil {
		return 0, err
	}

	if priorID.Valid {
		if _, err := tx.Exec(`UPDATE kb.pipelines SET status = 'superseded', modify_time = NOW() WHERE id = $1`, priorID.Int64); err != nil {
			return 0, err
		}
	}

	for _, gate := range gates {
		var predicate, predicateChecksum any
		if gate.PredicateChecksum != "" {
			raw, err := json.Marshal(gate.Predicate)
			if err != nil {
				return 0, err
			}
			predicate = raw
			predicateChecksum = gate.PredicateChecksum
		}
		var effect any
		if strings.TrimSpace(gate.Effect) != "" {
			effect = gate.Effect
		}
		var targetProcessor any
		if strings.TrimSpace(gate.TargetProcessor) != "" {
			targetProcessor = gate.TargetProcessor
		}
		requiredFacets, err := json.Marshal(gate.RequiredFacets)
		if err != nil {
			return 0, err
		}
		if _, err := tx.Exec(`
INSERT INTO kb.pipeline_rules (
    name, priority, pipeline_id, target_processor, effect, predicate, predicate_checksum,
    required_facets, depends_on_processors, active, approval_status
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, true, 'approved'
)`, gate.Name, gate.Priority, id, targetProcessor, effect, predicate, predicateChecksum, requiredFacets, pq.Array(gate.DependsOnProcessors)); err != nil {
			return 0, err
		}
	}

	return id, nil
}

// clearSystemDefaultLocked clears the current system-default flag (if any),
// returning whether a default existed. It locks the default row first so
// concurrent default transitions serialize. Callers then insert their own
// default row, restoring the "exactly one default" invariant atomically.
func clearSystemDefaultLocked(tx *sql.Tx) (bool, error) {
	var id int64
	err := tx.QueryRow(`SELECT id FROM kb.pipelines WHERE is_system_default FOR UPDATE`).Scan(&id)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if _, err := tx.Exec(`UPDATE kb.pipelines SET is_system_default = false, modify_time = NOW() WHERE id = $1`, id); err != nil {
		return false, err
	}
	return true, nil
}

// hasAnySystemDefault reports whether some kb.pipelines row currently holds
// the system-default flag (used to decide auto-marking when a new DAG is not
// explicitly requesting default status).
func hasAnySystemDefault(tx *sql.Tx) (bool, error) {
	var exists bool
	err := tx.QueryRow(`SELECT EXISTS(SELECT 1 FROM kb.pipelines WHERE is_system_default)`).Scan(&exists)
	return exists, err
}

// resolveSystemDefaultFlag decides the flag to store on a DAG being written.
// When the caller requests default, any incumbent is cleared (locked) and the
// flag is stored. When not requested, the DAG is auto-marked default only if
// no default currently exists — the first-DAG-becomes-default rule — and the
// incumbent, if any, is left untouched.
func resolveSystemDefaultFlag(tx *sql.Tx, requested bool) (bool, error) {
	if requested {
		if _, err := clearSystemDefaultLocked(tx); err != nil {
			return false, err
		}
		return true, nil
	}
	exists, err := hasAnySystemDefault(tx)
	return !exists, err
}

// CreateDocProcessDAG handles POST /api/v1/kb/doc-process-dags. The DAG
// (pipeline version + rules) is authored atomically in one transaction after
// DR8 validation. Name must be unique; at least one processor is required;
// if no DAG is currently the system default this one becomes it.
func CreateDocProcessDAG(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_DAG_100")
	defer rc.Close()
	logger := rc.GetLogger()

	var payload map[string]json.RawMessage
	if err := json.NewDecoder(c.Request().Body).Decode(&payload); err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid request body (CWB_KB_DAG_101)"})
	}

	var (
		name             string
		displayName      any
		description      any
		processors       []string
		legacyEquivalent bool
		isSystemDefault  bool
		ruleDrafts       []pipelineRuleDraftPayload
	)

	if raw, ok := payload["name"]; ok {
		value, err := decodeStringValue(raw, true)
		if err != nil || value == nil || strings.TrimSpace(*value) == "" {
			return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "name is required (CWB_KB_DAG_102)"})
		}
		name = *value
	} else {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "name is required (CWB_KB_DAG_102)"})
	}
	if raw, ok := payload["display_name"]; ok {
		value, err := decodeStringValue(raw, false)
		if err != nil {
			return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: fmt.Sprintf("invalid display_name: %v (CWB_KB_DAG_103)", err)})
		}
		if value != nil {
			displayName = *value
		}
	}
	if raw, ok := payload["description"]; ok {
		value, err := decodeStringValue(raw, false)
		if err != nil {
			return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: fmt.Sprintf("invalid description: %v (CWB_KB_DAG_104)", err)})
		}
		if value != nil {
			description = *value
		}
	}
	if raw, ok := payload["is_system_default"]; ok {
		if err := json.Unmarshal(raw, &isSystemDefault); err != nil {
			return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: fmt.Sprintf("invalid is_system_default: %v (CWB_KB_DAG_105)", err)})
		}
	}
	if raw, ok := payload["processors"]; ok {
		value, err := decodeStringArrayValue(raw)
		if err != nil {
			return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: fmt.Sprintf("invalid processors: %v (CWB_KB_DAG_106)", err)})
		}
		if value != nil {
			processors = *value
		}
	}
	if raw, ok := payload["legacy_equivalent"]; ok {
		if err := json.Unmarshal(raw, &legacyEquivalent); err != nil {
			return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: fmt.Sprintf("invalid legacy_equivalent: %v (CWB_KB_DAG_107)", err)})
		}
	}
	if raw, ok := payload["rules"]; ok {
		if err := json.Unmarshal(raw, &ruleDrafts); err != nil {
			return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: fmt.Sprintf("invalid rules: %v (CWB_KB_DAG_108)", err)})
		}
	}

	gates, err := buildGatesFromDrafts(ruleDrafts)
	if err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: fmt.Sprintf("%v (CWB_KB_DAG_109)", err)})
	}

	// Requirement: a Doc Process DAG must have at least one doc processor,
	// and must pass ADR 2026081001 DR8 validation before anything is saved.
	if err := docprocessing.ValidatePipelineVersion(docprocessing.PipelineVersionDraft{Processors: processors, Rules: gates}); err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: fmt.Sprintf("%v (CWB_KB_DAG_110)", err)})
	}

	db := ApiTypes.ProjectDBHandle
	tx, err := db.Begin()
	if err != nil {
		logger.Error("begin doc process dag transaction failed", "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to create doc process dag (CWB_KB_DAG_111)"})
	}
	defer tx.Rollback()

	// DAG names must be unique.
	var nameExists bool
	if err := tx.QueryRow(`SELECT EXISTS(SELECT 1 FROM kb.pipelines WHERE name = $1)`, name).Scan(&nameExists); err != nil {
		logger.Error("check duplicate dag name failed", "name", name, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to create doc process dag (CWB_KB_DAG_111)"})
	}
	if nameExists {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: fmt.Sprintf("a doc process dag named %q already exists (CWB_KB_DAG_112)", name)})
	}

	// Exactly-one system default: if this DAG explicitly requests the flag (or
	// no default exists yet), set this row — clearing any incumbent first so
	// the invariant holds atomically. A new DAG that does not request the
	// flag leaves the incumbent untouched.
	finalDefault, err := resolveSystemDefaultFlag(tx, isSystemDefault)
	if err != nil {
		logger.Error("resolve system default flag failed", "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to create doc process dag (CWB_KB_DAG_111)"})
	}

	id, err := createPipelineVersion(tx, name, displayName, description, processors, legacyEquivalent, finalDefault, gates)
	if err != nil {
		logger.Error("create pipeline version failed", "name", name, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to create doc process dag (CWB_KB_DAG_111)"})
	}

	if err := tx.Commit(); err != nil {
		logger.Error("commit doc process dag transaction failed", "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to create doc process dag (CWB_KB_DAG_111)"})
	}
	reloadAfterPipelineWrite(c.Request().Context(), db, logger, "doc process dag create")

	pipeline, err := fetchCurrentDAGByName(db, name)
	if err != nil {
		logger.Error("fetch created doc process dag failed", "id", id, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to retrieve created doc process dag (CWB_KB_DAG_113)"})
	}
	detail := dagDetailRecord{dagPipelineRecord: pipeline}
	if detail.Rules, err = fetchVersionRules(db, pipeline.ID); err != nil {
		logger.Error("fetch created dag rules failed", "id", pipeline.ID, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to retrieve created doc process dag rules (CWB_KB_DAG_114)"})
	}

	return c.JSON(http.StatusOK, dagDetailResponse{Status: true, Record: detail})
}

// dagRuleSignature is a comparable projection of a gate used to decide
// whether a submitted rule set differs from the current version's.
type dagRuleSignature struct {
	Name                string
	Priority            int
	TargetProcessor     string
	Effect              string
	PredicateChecksum   string
	DependsOnProcessors []string
}

func gateSignature(g docprocessing.PipelineGate) dagRuleSignature {
	deps := append([]string(nil), g.DependsOnProcessors...)
	sort.Strings(deps)
	return dagRuleSignature{
		Name: g.Name, Priority: g.Priority, TargetProcessor: g.TargetProcessor,
		Effect: g.Effect, PredicateChecksum: g.PredicateChecksum, DependsOnProcessors: deps,
	}
}

func signaturesEqual(a, b []dagRuleSignature) bool {
	if len(a) != len(b) {
		return false
	}
	sort.Slice(a, func(i, j int) bool { return a[i].Name < a[j].Name })
	sort.Slice(b, func(i, j int) bool { return b[i].Name < b[j].Name })
	for i := range a {
		if a[i].Name != b[i].Name || a[i].Priority != b[i].Priority ||
			a[i].TargetProcessor != b[i].TargetProcessor || a[i].Effect != b[i].Effect ||
			a[i].PredicateChecksum != b[i].PredicateChecksum ||
			!strings.EqualFold(strings.Join(a[i].DependsOnProcessors, "\x00"), strings.Join(b[i].DependsOnProcessors, "\x00")) {
			return false
		}
	}
	return true
}

func currentVersionRuleSignatures(db *sql.DB, pipelineID int64) ([]dagRuleSignature, error) {
	records, err := fetchVersionRules(db, pipelineID)
	if err != nil {
		return nil, err
	}
	out := make([]dagRuleSignature, 0, len(records))
	for _, r := range records {
		out = append(out, dagRuleSignature{
			Name: r.Name, Priority: r.Priority, TargetProcessor: r.TargetProcessor,
			Effect: r.Effect, PredicateChecksum: r.PredicateChecksum, DependsOnProcessors: r.DependsOnProcessors,
		})
	}
	return out, nil
}

// UpdateDocProcessDAG handles PUT /api/v1/kb/doc-process-dags/:name. Changes
// to processors or rules author a new pipeline version (superseding the
// prior, atomically); cosmetic changes (display_name/description) and the
// is_system_default flag update the current version in place. The system
// default can never be unset while it is the only one.
func UpdateDocProcessDAG(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_DAG_200")
	defer rc.Close()
	logger := rc.GetLogger()

	name := strings.TrimSpace(c.Param("name"))
	if name == "" {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "name is required (CWB_KB_DAG_201)"})
	}

	var payload map[string]json.RawMessage
	if err := json.NewDecoder(c.Request().Body).Decode(&payload); err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid request body (CWB_KB_DAG_202)"})
	}

	var (
		displayName, description any
		processorsProvided       bool
		processors               []string
		rulesProvided            bool
		ruleDrafts               []pipelineRuleDraftPayload
		requestedDefault         *bool
		legacyEquivalentProvided bool
		legacyEquivalent         bool
	)

	if raw, ok := payload["display_name"]; ok {
		value, err := decodeStringValue(raw, false)
		if err != nil {
			return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: fmt.Sprintf("invalid display_name: %v (CWB_KB_DAG_203)", err)})
		}
		if value != nil {
			displayName = *value
		} else {
			displayName = nil
		}
	}
	if raw, ok := payload["description"]; ok {
		value, err := decodeStringValue(raw, false)
		if err != nil {
			return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: fmt.Sprintf("invalid description: %v (CWB_KB_DAG_204)", err)})
		}
		if value != nil {
			description = *value
		} else {
			description = nil
		}
	}
	if raw, ok := payload["is_system_default"]; ok {
		var v bool
		if err := json.Unmarshal(raw, &v); err != nil {
			return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: fmt.Sprintf("invalid is_system_default: %v (CWB_KB_DAG_205)", err)})
		}
		requestedDefault = &v
	}
	if raw, ok := payload["processors"]; ok {
		value, err := decodeStringArrayValue(raw)
		if err != nil {
			return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: fmt.Sprintf("invalid processors: %v (CWB_KB_DAG_206)", err)})
		}
		if value != nil {
			processorsProvided = true
			processors = *value
		}
	}
	if raw, ok := payload["legacy_equivalent"]; ok {
		if err := json.Unmarshal(raw, &legacyEquivalent); err != nil {
			return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: fmt.Sprintf("invalid legacy_equivalent: %v (CWB_KB_DAG_207)", err)})
		}
		legacyEquivalentProvided = true
	}
	if raw, ok := payload["rules"]; ok {
		if err := json.Unmarshal(raw, &ruleDrafts); err != nil {
			return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: fmt.Sprintf("invalid rules: %v (CWB_KB_DAG_208)", err)})
		}
		rulesProvided = true
	}

	db := ApiTypes.ProjectDBHandle
	current, err := fetchCurrentDAGByName(db, name)
	if err == sql.ErrNoRows {
		return c.JSON(http.StatusNotFound, errorResponse{Status: false, ErrorMsg: "doc process dag not found (CWB_KB_DAG_209)"})
	}
	if err != nil {
		logger.Error("fetch doc process dag failed", "name", name, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to retrieve doc process dag (CWB_KB_DAG_210)"})
	}

	// Unsetting the only system default is rejected: the invariant requires
	// exactly one. Since the partial unique index guarantees the current DAG
	// is the sole default when it holds the flag, clearing it is always a
	// zero-default transition.
	if requestedDefault != nil && current.IsSystemDefault && !*requestedDefault {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "cannot unset the system default; mark another doc process dag as default first (CWB_KB_DAG_211)"})
	}

	// Decide whether the processor/rule content actually changed. If it did,
	// author a new version; otherwise update cosmetics in place.
	gates := make([]docprocessing.PipelineGate, 0)
	if rulesProvided {
		gates, err = buildGatesFromDrafts(ruleDrafts)
		if err != nil {
			return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: fmt.Sprintf("%v (CWB_KB_DAG_212)", err)})
		}
	}

	// Resolve the effective display_name/description for the version being
	// written: absent fields carry forward from the current version, while an
	// explicit null clears them.
	effDisplayName, effDescription := any(nil), any(nil)
	if current.DisplayName != nil {
		effDisplayName = *current.DisplayName
	}
	if current.Description != nil {
		effDescription = *current.Description
	}
	if payloadContains(payload, "display_name") {
		effDisplayName = displayName
	}
	if payloadContains(payload, "description") {
		effDescription = description
	}

	contentChanged := false
	if processorsProvided {
		if !stringSlicesEqual(processors, current.Processors) {
			contentChanged = true
		}
	}
	if !contentChanged && rulesProvided {
		currentSigs, err := currentVersionRuleSignatures(db, current.ID)
		if err != nil {
			logger.Error("fetch current dag rule signatures failed", "id", current.ID, "err", err)
			return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to retrieve doc process dag rules (CWB_KB_DAG_213)"})
		}
		submittedSigs := make([]dagRuleSignature, 0, len(gates))
		for _, g := range gates {
			submittedSigs = append(submittedSigs, gateSignature(g))
		}
		if !signaturesEqual(currentSigs, submittedSigs) {
			contentChanged = true
		}
	}

	if contentChanged {
		if err := docprocessing.ValidatePipelineVersion(docprocessing.PipelineVersionDraft{Processors: processors, Rules: gates}); err != nil {
			return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: fmt.Sprintf("%v (CWB_KB_DAG_214)", err)})
		}

		newIsDefault := current.IsSystemDefault
		if requestedDefault != nil {
			newIsDefault = *requestedDefault
		}

		tx, err := db.Begin()
		if err != nil {
			logger.Error("begin doc process dag transaction failed", "err", err)
			return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to modify doc process dag (CWB_KB_DAG_215)"})
		}
		defer tx.Rollback()

		if newIsDefault {
			// Clear the incumbent (including the superseded prior version)
			// before inserting the new default row.
			if _, err := clearSystemDefaultLocked(tx); err != nil {
				logger.Error("clear system default failed", "err", err)
				return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to modify doc process dag (CWB_KB_DAG_215)"})
			}
		}

		legacyEquiv := current.LegacyEquivalent
		if legacyEquivalentProvided {
			legacyEquiv = legacyEquivalent
		}

		id, err := createPipelineVersion(tx, name, effDisplayName, effDescription, processors, legacyEquiv, newIsDefault, gates)
		if err != nil {
			logger.Error("create pipeline version failed", "name", name, "err", err)
			return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to modify doc process dag (CWB_KB_DAG_215)"})
		}

		if err := tx.Commit(); err != nil {
			logger.Error("commit doc process dag transaction failed", "err", err)
			return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to modify doc process dag (CWB_KB_DAG_215)"})
		}
		reloadAfterPipelineWrite(c.Request().Context(), db, logger, "doc process dag update (new version)")

		_ = id
	} else {
		// Cosmetic in-place update. When the default flag changes to true we
		// must clear the incumbent atomically, so run that path in a tx.
		flagChanged := requestedDefault != nil && *requestedDefault != current.IsSystemDefault

		if flagChanged && *requestedDefault {
			tx, err := db.Begin()
			if err != nil {
				logger.Error("begin doc process dag transaction failed", "err", err)
				return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to modify doc process dag (CWB_KB_DAG_215)"})
			}
			defer tx.Rollback()
			if _, err := clearSystemDefaultLocked(tx); err != nil {
				logger.Error("clear system default failed", "err", err)
				return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to modify doc process dag (CWB_KB_DAG_215)"})
			}
			if _, err := tx.Exec(`UPDATE kb.pipelines SET is_system_default = true, modify_time = NOW() WHERE id = $1`, current.ID); err != nil {
				logger.Error("set system default failed", "id", current.ID, "err", err)
				return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to modify doc process dag (CWB_KB_DAG_215)"})
			}
			if err := tx.Commit(); err != nil {
				logger.Error("commit doc process dag transaction failed", "err", err)
				return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to modify doc process dag (CWB_KB_DAG_215)"})
			}
			reloadAfterPipelineWrite(c.Request().Context(), db, logger, "doc process dag update (default flag)")
		} else {
			sets := []string{"modify_time = NOW()"}
			args := make([]any, 0, 4)
			addSet := func(column string, value any) {
				args = append(args, value)
				sets = append(sets, fmt.Sprintf("%s = $%d", column, len(args)))
			}
			if payloadContains(payload, "display_name") {
				addSet("display_name", displayName)
			}
			if payloadContains(payload, "description") {
				addSet("description", description)
			}
			if requestedDefault != nil {
				addSet("is_system_default", *requestedDefault)
			}
			if legacyEquivalentProvided {
				addSet("legacy_equivalent", legacyEquivalent)
			}
			args = append(args, current.ID)
			query := fmt.Sprintf("UPDATE kb.pipelines SET %s WHERE id = $%d", strings.Join(sets, ", "), len(args))
			if _, err := db.Exec(query, args...); err != nil {
				logger.Error("update doc process dag failed", "id", current.ID, "err", err)
				return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to modify doc process dag (CWB_KB_DAG_216)"})
			}
			reloadAfterPipelineWrite(c.Request().Context(), db, logger, "doc process dag update (cosmetic)")
		}
	}

	pipeline, err := fetchCurrentDAGByName(db, name)
	if err != nil {
		logger.Error("fetch updated doc process dag failed", "name", name, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to retrieve updated doc process dag (CWB_KB_DAG_217)"})
	}
	detail := dagDetailRecord{dagPipelineRecord: pipeline}
	if detail.Rules, err = fetchVersionRules(db, pipeline.ID); err != nil {
		logger.Error("fetch updated dag rules failed", "id", pipeline.ID, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to retrieve updated doc process dag rules (CWB_KB_DAG_218)"})
	}

	return c.JSON(http.StatusOK, dagDetailResponse{Status: true, Record: detail})
}

// DeleteDocProcessDAG handles DELETE /api/v1/kb/doc-process-dags/:name. The
// whole DAG — every pipeline version, its rules, and any referencing
// bindings — is removed in one transaction. The system default cannot be
// deleted (the invariant requires exactly one default; promote another first).
func DeleteDocProcessDAG(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_DAG_300")
	defer rc.Close()
	logger := rc.GetLogger()

	name := strings.TrimSpace(c.Param("name"))
	if name == "" {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "name is required (CWB_KB_DAG_301)"})
	}

	db := ApiTypes.ProjectDBHandle
	tx, err := db.Begin()
	if err != nil {
		logger.Error("begin doc process dag delete transaction failed", "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to delete doc process dag (CWB_KB_DAG_302)"})
	}
	defer tx.Rollback()

	var isDefault bool
	var exists bool
	err = tx.QueryRow(`SELECT EXISTS(SELECT 1 FROM kb.pipelines WHERE name = $1), EXISTS(SELECT 1 FROM kb.pipelines WHERE name = $1 AND is_system_default)`, name).Scan(&exists, &isDefault)
	if err != nil {
		logger.Error("check doc process dag existence failed", "name", name, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to delete doc process dag (CWB_KB_DAG_302)"})
	}
	if !exists {
		return c.JSON(http.StatusNotFound, errorResponse{Status: false, ErrorMsg: "doc process dag not found (CWB_KB_DAG_303)"})
	}
	if isDefault {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "cannot delete the system default doc process dag; mark another doc process dag as default first (CWB_KB_DAG_304)"})
	}

	// Bindings and rules reference kb.pipelines with ON DELETE RESTRICT, so
	// remove them before the pipeline rows.
	if _, err := tx.Exec(`DELETE FROM kb.pipeline_bindings WHERE pipeline_id IN (SELECT id FROM kb.pipelines WHERE name = $1)`, name); err != nil {
		logger.Error("delete doc process dag bindings failed", "name", name, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to delete doc process dag (CWB_KB_DAG_302)"})
	}
	if _, err := tx.Exec(`DELETE FROM kb.pipeline_rules WHERE pipeline_id IN (SELECT id FROM kb.pipelines WHERE name = $1)`, name); err != nil {
		logger.Error("delete doc process dag rules failed", "name", name, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to delete doc process dag (CWB_KB_DAG_302)"})
	}
	result, err := tx.Exec(`DELETE FROM kb.pipelines WHERE name = $1`, name)
	if err != nil {
		logger.Error("delete doc process dag pipelines failed", "name", name, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to delete doc process dag (CWB_KB_DAG_302)"})
	}
	deleted, err := result.RowsAffected()
	if err != nil {
		logger.Error("rows affected delete doc process dag failed", "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to delete doc process dag (CWB_KB_DAG_302)"})
	}

	if err := tx.Commit(); err != nil {
		logger.Error("commit doc process dag delete transaction failed", "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to delete doc process dag (CWB_KB_DAG_302)"})
	}
	reloadAfterPipelineWrite(c.Request().Context(), db, logger, "doc process dag delete")

	return c.JSON(http.StatusOK, dagDeleteResponse{Status: true, Deleted: int(deleted)})
}

// ListDocProcessProcessors handles GET /api/v1/kb/doc-process-processors,
// returning the registered processor catalog (Go literal + kb.processor_registry
// union) so the page can offer a real processor picker.
func ListDocProcessProcessors(c echo.Context) error {
	specs := docprocessing.RegisteredProcessors()
	type processorDTO struct {
		Name           string   `json:"name"`
		Phase          string   `json:"phase"`
		Class          string   `json:"class,omitempty"`
		Cost           string   `json:"cost,omitempty"`
		OnUndetermined string   `json:"on_undetermined,omitempty"`
		Idempotent     bool     `json:"idempotent"`
		Requires       []string `json:"requires,omitempty"`
		Produces       []string `json:"produces,omitempty"`
	}
	out := make([]processorDTO, 0, len(specs))
	for _, s := range specs {
		out = append(out, processorDTO{
			Name: s.Name, Phase: s.Phase, Class: s.Class, Cost: s.Cost,
			OnUndetermined: s.OnUndetermined, Idempotent: s.Idempotent,
			Requires: s.Requires, Produces: s.Produces,
		})
	}
	return c.JSON(http.StatusOK, map[string]any{"status": true, "results": out, "total": len(out)})
}

func stringSlicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	as := append([]string(nil), a...)
	bs := append([]string(nil), b...)
	sort.Strings(as)
	sort.Strings(bs)
	for i := range as {
		if as[i] != bs[i] {
			return false
		}
	}
	return true
}

func payloadContains(payload map[string]json.RawMessage, key string) bool {
	_, ok := payload[key]
	return ok
}
