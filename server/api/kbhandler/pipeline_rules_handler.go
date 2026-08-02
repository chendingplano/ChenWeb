package kbhandler

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	docprocessing "github.com/chendingplano/deepdoc/server/api/doc-processing"
	"github.com/chendingplano/deepdoc/server/api/ontology/semrules"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/EchoFactory"
	"github.com/labstack/echo/v4"
)

type pipelineRuleRecord struct {
	ID                         int64           `json:"id"`
	Name                       string          `json:"name"`
	Priority                   int             `json:"priority"`
	MatchInputDocType          *string         `json:"match_input_doc_type,omitempty"`
	MatchSourceLanguage        *string         `json:"match_source_language,omitempty"`
	MatchKnowledgeStoreBinding *string         `json:"match_knowledge_store_binding,omitempty"`
	PipelineID                 int64           `json:"pipeline_id"`
	PipelineName               string          `json:"pipeline_name"`
	Active                     bool            `json:"active"`
	PolicyID                   int64           `json:"policy_id"`
	CreateTime                 time.Time       `json:"create_time"`
	ModifyTime                 time.Time       `json:"modify_time"`
	Predicate                  json.RawMessage `json:"predicate,omitempty"`
	PredicateChecksum          string          `json:"predicate_checksum,omitempty"`
	PredicateSource            string          `json:"predicate_source,omitempty"`
	TargetProcessor            string          `json:"target_processor,omitempty"`
	Effect                     string          `json:"effect,omitempty"`
	RequiredFacets             []string        `json:"required_facets,omitempty"`
}

type listPipelineRulesResponse struct {
	Status  bool                 `json:"status"`
	Results []pipelineRuleRecord `json:"results"`
	Total   int                  `json:"total"`
}

type pipelineRuleDetailResponse struct {
	Status bool               `json:"status"`
	Record pipelineRuleRecord `json:"record"`
}

const pipelineRuleSelectColumns = `
    r.id, r.name, r.priority, r.match_input_doc_type, r.match_source_language, r.match_knowledge_store_binding,
    r.pipeline_id, p.name, r.active, r.policy_id, r.create_time, r.modify_time
FROM kb.pipeline_rules r
JOIN kb.pipelines p ON p.id = r.pipeline_id`

func scanPipelineRuleRecord(scan func(dest ...any) error) (pipelineRuleRecord, error) {
	var (
		record       pipelineRuleRecord
		matchDocType sql.NullString
		matchLang    sql.NullString
		matchBinding sql.NullString
	)
	if err := scan(
		&record.ID, &record.Name, &record.Priority, &matchDocType, &matchLang, &matchBinding,
		&record.PipelineID, &record.PipelineName, &record.Active, &record.PolicyID, &record.CreateTime, &record.ModifyTime,
	); err != nil {
		return pipelineRuleRecord{}, err
	}
	if matchDocType.Valid && matchDocType.String != "" {
		v := matchDocType.String
		record.MatchInputDocType = &v
	}
	if matchLang.Valid && matchLang.String != "" {
		v := matchLang.String
		record.MatchSourceLanguage = &v
	}
	if matchBinding.Valid && matchBinding.String != "" {
		v := matchBinding.String
		record.MatchKnowledgeStoreBinding = &v
	}
	return record, nil
}

func fetchPipelineRuleByID(db *sql.DB, id int64) (pipelineRuleRecord, error) {
	query := "SELECT" + pipelineRuleSelectColumns + "\nWHERE r.id = $1"
	row := db.QueryRow(query, id)
	return scanPipelineRuleRecord(row.Scan)
}

func fetchPipelineRuleCompatBindingByID(db *sql.DB, id int64) (pipelineRuleRecord, error) {
	const query = `
SELECT b.id, COALESCE(b.name, ''), b.priority,
       COALESCE(b.predicate, '{}'::jsonb)::text,
       b.pipeline_id, p.name, b.active, b.policy_id, b.create_time, b.modify_time
FROM kb.pipeline_bindings b
JOIN kb.pipelines p ON p.id = b.pipeline_id
WHERE b.id = $1 OR b.legacy_rule_id = $1`
	row := db.QueryRow(query, id)
	record, representable, err := scanPipelineRuleCompatBindingRecord(row.Scan)
	if err != nil {
		return pipelineRuleRecord{}, err
	}
	if !representable {
		return pipelineRuleRecord{}, sql.ErrNoRows
	}
	return record, nil
}

func pipelineRuleCompatBindingPolicyStatus(db *sql.DB, id int64) (string, error) {
	var status string
	err := db.QueryRow(`
SELECT pp.status
FROM kb.pipeline_bindings b
JOIN kb.pipeline_policies pp ON pp.id = b.policy_id
WHERE b.id = $1 OR b.legacy_rule_id = $1`, id).Scan(&status)
	return strings.ToLower(strings.TrimSpace(status)), err
}

func scanPipelineRuleCompatBindingRecord(scan func(dest ...any) error) (pipelineRuleRecord, bool, error) {
	var (
		record       pipelineRuleRecord
		predicateRaw string
	)
	if err := scan(
		&record.ID, &record.Name, &record.Priority, &predicateRaw,
		&record.PipelineID, &record.PipelineName, &record.Active, &record.PolicyID, &record.CreateTime, &record.ModifyTime,
	); err != nil {
		return pipelineRuleRecord{}, false, err
	}
	if !applyLegacyPredicateToPipelineRuleRecord(&record, predicateRaw) {
		return pipelineRuleRecord{}, false, nil
	}
	record.Predicate = json.RawMessage(predicateRaw)
	record.PredicateSource = "legacy_adapter"
	return record, true, nil
}

func scanCanonicalPipelineGateRecord(scan func(dest ...any) error) (pipelineRuleRecord, error) {
	var record pipelineRuleRecord
	var predicateRaw, requiredFacetsRaw string
	if err := scan(&record.ID, &record.Name, &record.Priority, &predicateRaw, &record.PredicateChecksum,
		&record.TargetProcessor, &record.Effect, &requiredFacetsRaw, &record.Active, &record.PolicyID,
		&record.CreateTime, &record.ModifyTime); err != nil {
		return pipelineRuleRecord{}, err
	}
	record.Predicate = json.RawMessage(predicateRaw)
	record.PredicateSource = "json"
	if err := json.Unmarshal([]byte(requiredFacetsRaw), &record.RequiredFacets); err != nil {
		return pipelineRuleRecord{}, err
	}
	return record, nil
}

func applyLegacyPredicateToPipelineRuleRecord(record *pipelineRuleRecord, predicateRaw string) bool {
	var doc struct {
		Version    int `json:"version"`
		Expression struct {
			Kind  string `json:"kind"`
			Items []struct {
				Kind  string `json:"kind"`
				Path  string `json:"path"`
				Op    string `json:"op"`
				Value any    `json:"value"`
			} `json:"items"`
		} `json:"expression"`
	}
	if err := json.Unmarshal([]byte(predicateRaw), &doc); err != nil {
		return false
	}
	if doc.Version != 1 || doc.Expression.Kind != "all" {
		return false
	}
	for _, item := range doc.Expression.Items {
		if item.Kind != "fact" || item.Op != "eq" {
			return false
		}
		value := strings.TrimSpace(fmt.Sprint(item.Value))
		if value == "" {
			return false
		}
		switch item.Path {
		case "document.input_doc_type":
			record.MatchInputDocType = &value
		case "document.source_language":
			record.MatchSourceLanguage = &value
		case "document.knowledge_store_binding_state":
			record.MatchKnowledgeStoreBinding = &value
		default:
			return false
		}
	}
	return true
}

// normalizeRuleMatchInput lowercases/trims a match_* value the same way
// ProductionRoutingFacets are normalized, so rule authors don't need to
// worry about casing matching what a document's facets resolve to. Empty
// (after trimming) means "no filter on this field" and is stored as NULL.
func normalizeRuleMatchInput(raw *string) any {
	if raw == nil {
		return nil
	}
	trimmed := strings.ToLower(strings.TrimSpace(*raw))
	if trimmed == "" {
		return nil
	}
	return trimmed
}

// ListPipelineRules handles GET /api/v1/kb/pipeline-rules.
func ListPipelineRules(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_PR_001")
	defer rc.Close()
	logger := rc.GetLogger()

	db := ApiTypes.ProjectDBHandle
	query := `
SELECT b.id, COALESCE(b.name, ''), b.priority,
       COALESCE(b.predicate, '{}'::jsonb)::text,
       b.pipeline_id, p.name, b.active, b.policy_id, b.create_time, b.modify_time
FROM kb.pipeline_bindings b
JOIN kb.pipelines p ON p.id = b.pipeline_id
WHERE b.binding_kind = 'conditional'
ORDER BY b.priority DESC, b.id`
	rows, err := db.Query(query)
	if err != nil {
		logger.Error("query pipeline rules failed", "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to retrieve pipeline rules (CWB_KB_PR_002)"})
	}
	defer rows.Close()

	results := make([]pipelineRuleRecord, 0)
	for rows.Next() {
		record, representable, err := scanPipelineRuleCompatBindingRecord(rows.Scan)
		if err != nil {
			logger.Error("scan pipeline rule failed", "err", err)
			return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to scan pipeline rules (CWB_KB_PR_003)"})
		}
		if !representable {
			continue
		}
		results = append(results, record)
	}
	if err := rows.Err(); err != nil {
		logger.Error("iterate pipeline rules failed", "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to iterate pipeline rules (CWB_KB_PR_004)"})
	}

	gateRows, err := db.Query(`
SELECT id, name, priority, COALESCE(predicate, '{}'::jsonb)::text,
       COALESCE(predicate_checksum, ''), COALESCE(target_processor, ''),
       COALESCE(effect, ''), COALESCE(required_facets, '[]'::jsonb)::text,
       active, policy_id, create_time, modify_time
FROM kb.pipeline_rules
WHERE target_processor IS NOT NULL
ORDER BY priority DESC, id`)
	if err != nil {
		logger.Error("query canonical pipeline gates failed", "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to retrieve pipeline rules (CWB_KB_PR_002)"})
	}
	defer gateRows.Close()
	for gateRows.Next() {
		record, err := scanCanonicalPipelineGateRecord(gateRows.Scan)
		if err != nil {
			logger.Error("scan canonical pipeline gate failed", "err", err)
			return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to scan pipeline rules (CWB_KB_PR_003)"})
		}
		results = append(results, record)
	}
	if err := gateRows.Err(); err != nil {
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to iterate pipeline rules (CWB_KB_PR_004)"})
	}

	return c.JSON(http.StatusOK, listPipelineRulesResponse{Status: true, Results: results, Total: len(results)})
}

// CreatePipelineRule handles POST /api/v1/kb/pipeline-rules.
func CreatePipelineRule(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_PR_100")
	defer rc.Close()
	logger := rc.GetLogger()

	var payload struct {
		Name                       string          `json:"name"`
		Priority                   *int            `json:"priority"`
		MatchInputDocType          *string         `json:"match_input_doc_type"`
		MatchSourceLanguage        *string         `json:"match_source_language"`
		MatchKnowledgeStoreBinding *string         `json:"match_knowledge_store_binding"`
		PipelineID                 *int64          `json:"pipeline_id"`
		Active                     *bool           `json:"active"`
		PolicyID                   *int64          `json:"policy_id"`
		Predicate                  json.RawMessage `json:"predicate"`
		PredicateChecksum          string          `json:"predicate_checksum"`
		TargetProcessor            string          `json:"target_processor"`
		Effect                     string          `json:"effect"`
	}
	if err := json.NewDecoder(c.Request().Body).Decode(&payload); err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid request body (CWB_KB_PR_101)"})
	}
	if strings.TrimSpace(payload.Name) == "" {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "name is required (CWB_KB_PR_102)"})
	}
	isCanonicalGate := len(payload.Predicate) > 0 || strings.TrimSpace(payload.TargetProcessor) != "" || strings.TrimSpace(payload.Effect) != ""
	if !isCanonicalGate && (payload.PipelineID == nil || *payload.PipelineID <= 0) {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "pipeline_id is required (CWB_KB_PR_103)"})
	}

	priority := 0
	if payload.Priority != nil {
		priority = *payload.Priority
	}
	active := true
	if payload.Active != nil {
		active = *payload.Active
	}

	db := ApiTypes.ProjectDBHandle
	policyID := payload.PolicyID
	if policyID == nil {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "policy_id is required (CWB_KB_PR_106)"})
	}
	status, err := pipelinePolicyStatus(db, *policyID)
	if err != nil {
		logger.Error("resolve pipeline policy status failed", "policy_id", *policyID, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to resolve pipeline policy (CWB_KB_PR_109)"})
	}
	if status == "active" {
		return c.JSON(http.StatusConflict, errorResponse{Status: false, ErrorMsg: "active policy cannot be edited (CWB_KB_PR_110)"})
	}
	if isCanonicalGate {
		targetProcessor := strings.TrimSpace(payload.TargetProcessor)
		effect := strings.ToLower(strings.TrimSpace(payload.Effect))
		if targetProcessor == "" {
			return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "target_processor is required (CWB_KB_PR_111)"})
		}
		if effect != "require" && effect != "enable" && effect != "skip" && effect != "defer" {
			return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid effect (CWB_KB_PR_112)"})
		}
		var predicateDoc semrules.Document
		if err := json.Unmarshal(payload.Predicate, &predicateDoc); err != nil || semrules.Validate(predicateDoc) != nil {
			return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid predicate (CWB_KB_PR_113)"})
		}
		_, checksum, err := semrules.Canonicalize(predicateDoc)
		if err != nil {
			return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid predicate (CWB_KB_PR_113)"})
		}
		if supplied := strings.TrimSpace(payload.PredicateChecksum); supplied != "" && supplied != checksum {
			return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "predicate_checksum mismatch (CWB_KB_PR_114)"})
		}
		requiredFacets, err := json.Marshal(semrules.Analyze(predicateDoc).RequiredDocumentFacets)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to encode required facets (CWB_KB_PR_115)"})
		}
		var id int64
		if err := db.QueryRow(`
INSERT INTO kb.pipeline_rules (
    name, priority, predicate, predicate_checksum, target_processor, effect,
    required_facets, active, policy_id, approval_status
) VALUES ($1, $2, $3::jsonb, $4, $5, $6, $7::jsonb, $8, $9, 'draft')
RETURNING id`, strings.TrimSpace(payload.Name), priority, string(payload.Predicate), checksum, targetProcessor, effect, string(requiredFacets), active, *policyID).Scan(&id); err != nil {
			logger.Error("insert canonical processor gate failed", "err", err)
			return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to create pipeline rule (CWB_KB_PR_104)"})
		}
		return c.JSON(http.StatusOK, map[string]any{
			"status": true,
			"record": map[string]any{"id": id, "name": strings.TrimSpace(payload.Name), "priority": priority, "predicate": json.RawMessage(payload.Predicate), "predicate_checksum": checksum, "predicate_source": "json", "target_processor": targetProcessor, "effect": effect, "required_facets": json.RawMessage(requiredFacets), "active": active, "policy_id": *policyID},
		})
	}

	rule := docprocessing.ProductionPipelineRule{
		MatchInputDocType:          stringPtrFromNormalizedAny(normalizeRuleMatchInput(payload.MatchInputDocType)),
		MatchSourceLanguage:        stringPtrFromNormalizedAny(normalizeRuleMatchInput(payload.MatchSourceLanguage)),
		MatchKnowledgeStoreBinding: stringPtrFromNormalizedAny(normalizeRuleMatchInput(payload.MatchKnowledgeStoreBinding)),
	}
	predicateDoc, checksum, err := docprocessing.LegacyRulePredicateDocument(rule)
	if err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid predicate (CWB_KB_PR_107)"})
	}
	predicateJSON, err := json.Marshal(predicateDoc)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to encode predicate (CWB_KB_PR_108)"})
	}

	const insertQuery = `
INSERT INTO kb.pipeline_bindings (
    name, priority, binding_kind, predicate, predicate_checksum, pipeline_id, active, policy_id
) VALUES (
    $1, $2, 'conditional', $3::jsonb, $4, $5, $6, $7
)
RETURNING id
`
	var id int64
	if err := db.QueryRow(
		insertQuery,
		strings.TrimSpace(payload.Name), priority, string(predicateJSON), checksum, *payload.PipelineID, active, *policyID,
	).Scan(&id); err != nil {
		logger.Error("insert pipeline rule failed", "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to create pipeline rule (CWB_KB_PR_104)"})
	}

	record, err := fetchPipelineRuleCompatBindingByID(db, id)
	if err != nil {
		logger.Error("fetch created pipeline rule failed", "id", id, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to retrieve created pipeline rule (CWB_KB_PR_105)"})
	}

	return c.JSON(http.StatusOK, pipelineRuleDetailResponse{Status: true, Record: record})
}

func stringPtrFromNormalizedAny(value any) string {
	if value == nil {
		return ""
	}
	return fmt.Sprint(value)
}

// UpdatePipelineRule handles PUT /api/v1/kb/pipeline-rules/:id.
func UpdatePipelineRule(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_PR_200")
	defer rc.Close()
	logger := rc.GetLogger()

	idStr := strings.TrimSpace(c.Param("id"))
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid id (CWB_KB_PR_201)"})
	}

	var payload map[string]json.RawMessage
	if err := json.NewDecoder(c.Request().Body).Decode(&payload); err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid request body (CWB_KB_PR_202)"})
	}
	if isCanonicalGateUpdate(payload) {
		return updateCanonicalPipelineGate(c, logger, id, payload)
	}

	sets := []string{"modify_time = NOW()"}
	args := make([]any, 0, len(payload)+1)
	addSet := func(column string, value any) {
		args = append(args, value)
		sets = append(sets, fmt.Sprintf("%s = $%d", column, len(args)))
	}

	fields := make([]string, 0, len(payload))
	for field := range payload {
		fields = append(fields, field)
	}
	sort.Strings(fields)

	for _, field := range fields {
		raw := payload[field]
		switch field {
		case "name":
			value, err := decodeStringValue(raw, true)
			if err != nil || value == nil || strings.TrimSpace(*value) == "" {
				return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "name cannot be empty (CWB_KB_PR_203)"})
			}
			addSet(field, strings.TrimSpace(*value))
		case "priority":
			var v int
			if err := json.Unmarshal(raw, &v); err != nil {
				return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: fmt.Sprintf("invalid priority: %v (CWB_KB_PR_204)", err)})
			}
			addSet(field, v)
		case "match_input_doc_type", "match_source_language", "match_knowledge_store_binding":
			value, err := decodeStringValue(raw, false)
			if err != nil {
				return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: fmt.Sprintf("invalid %s: %v (CWB_KB_PR_205)", field, err)})
			}
			addSet(field, normalizeRuleMatchInput(value))
		case "pipeline_id":
			var v int64
			if err := json.Unmarshal(raw, &v); err != nil || v <= 0 {
				return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid pipeline_id (CWB_KB_PR_206)"})
			}
			addSet(field, v)
		case "active":
			var v bool
			if err := json.Unmarshal(raw, &v); err != nil {
				return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: fmt.Sprintf("invalid active: %v (CWB_KB_PR_207)", err)})
			}
			addSet(field, v)
		}
	}

	if len(sets) <= 1 {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "no editable fields in request (CWB_KB_PR_208)"})
	}

	db := ApiTypes.ProjectDBHandle
	status, err := pipelineRuleCompatBindingPolicyStatus(db, id)
	if err != nil {
		if err == sql.ErrNoRows {
			return c.JSON(http.StatusNotFound, errorResponse{Status: false, ErrorMsg: "record not found (CWB_KB_PR_211)"})
		}
		logger.Error("resolve pipeline rule policy status failed", "id", id, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to resolve pipeline rule policy (CWB_KB_PR_216)"})
	}
	if status == "active" {
		return c.JSON(http.StatusConflict, errorResponse{Status: false, ErrorMsg: "active policy cannot be edited (CWB_KB_PR_217)"})
	}
	existing, err := fetchPipelineRuleCompatBindingByID(db, id)
	if err != nil {
		if err == sql.ErrNoRows {
			return c.JSON(http.StatusNotFound, errorResponse{Status: false, ErrorMsg: "record not found (CWB_KB_PR_211)"})
		}
		logger.Error("fetch pipeline rule compatibility binding failed", "id", id, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to retrieve pipeline rule (CWB_KB_PR_213)"})
	}
	var matchInputDocType = derefString(existing.MatchInputDocType)
	var matchSourceLanguage = derefString(existing.MatchSourceLanguage)
	var matchKnowledgeStoreBinding = derefString(existing.MatchKnowledgeStoreBinding)
	rebuildPredicate := false
	sets = []string{"modify_time = NOW()"}
	args = args[:0]
	addSet = func(column string, value any) {
		args = append(args, value)
		sets = append(sets, fmt.Sprintf("%s = $%d", column, len(args)))
	}

	for _, field := range fields {
		raw := payload[field]
		switch field {
		case "name":
			value, _ := decodeStringValue(raw, true)
			addSet("name", strings.TrimSpace(*value))
		case "priority":
			var v int
			_ = json.Unmarshal(raw, &v)
			addSet("priority", v)
		case "match_input_doc_type":
			value, _ := decodeStringValue(raw, false)
			matchInputDocType = stringPtrFromNormalizedAny(normalizeRuleMatchInput(value))
			rebuildPredicate = true
		case "match_source_language":
			value, _ := decodeStringValue(raw, false)
			matchSourceLanguage = stringPtrFromNormalizedAny(normalizeRuleMatchInput(value))
			rebuildPredicate = true
		case "match_knowledge_store_binding":
			value, _ := decodeStringValue(raw, false)
			matchKnowledgeStoreBinding = stringPtrFromNormalizedAny(normalizeRuleMatchInput(value))
			rebuildPredicate = true
		case "pipeline_id":
			var v int64
			_ = json.Unmarshal(raw, &v)
			addSet("pipeline_id", v)
		case "active":
			var v bool
			_ = json.Unmarshal(raw, &v)
			addSet("active", v)
		}
	}
	if rebuildPredicate {
		predicateDoc, checksum, err := docprocessing.LegacyRulePredicateDocument(docprocessing.ProductionPipelineRule{
			MatchInputDocType:          matchInputDocType,
			MatchSourceLanguage:        matchSourceLanguage,
			MatchKnowledgeStoreBinding: matchKnowledgeStoreBinding,
		})
		if err != nil {
			return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid predicate (CWB_KB_PR_214)"})
		}
		predicateJSON, err := json.Marshal(predicateDoc)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to encode predicate (CWB_KB_PR_215)"})
		}
		addSet("predicate", string(predicateJSON))
		addSet("predicate_checksum", checksum)
	}

	query := fmt.Sprintf("UPDATE kb.pipeline_bindings SET %s WHERE id = $%d OR legacy_rule_id = $%d", strings.Join(sets, ", "), len(args)+1, len(args)+1)
	args = append(args, id)
	result, err := db.Exec(query, args...)
	if err != nil {
		logger.Error("update pipeline rule failed", "id", id, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to update pipeline rule (CWB_KB_PR_209)"})
	}
	affected, err := result.RowsAffected()
	if err != nil {
		logger.Error("rows affected pipeline rule failed", "id", id, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to verify pipeline rule update (CWB_KB_PR_210)"})
	}
	if affected == 0 {
		return c.JSON(http.StatusNotFound, errorResponse{Status: false, ErrorMsg: "record not found (CWB_KB_PR_211)"})
	}

	record, err := fetchPipelineRuleCompatBindingByID(db, id)
	if err != nil {
		logger.Error("fetch updated pipeline rule failed", "id", id, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to retrieve updated pipeline rule (CWB_KB_PR_212)"})
	}

	return c.JSON(http.StatusOK, pipelineRuleDetailResponse{Status: true, Record: record})
}

func isCanonicalGateUpdate(payload map[string]json.RawMessage) bool {
	for _, field := range []string{"predicate", "predicate_checksum", "target_processor", "effect", "required_facets"} {
		if _, ok := payload[field]; ok {
			return true
		}
	}
	return false
}

func updateCanonicalPipelineGate(c echo.Context, logger interface{ Error(string, ...any) }, id int64, payload map[string]json.RawMessage) error {
	db := ApiTypes.ProjectDBHandle
	var status string
	if err := db.QueryRow(`
SELECT pp.status
FROM kb.pipeline_rules r
JOIN kb.pipeline_policies pp ON pp.id = r.policy_id
WHERE r.id = $1 AND r.target_processor IS NOT NULL`, id).Scan(&status); err != nil {
		if err == sql.ErrNoRows {
			return c.JSON(http.StatusNotFound, errorResponse{Status: false, ErrorMsg: "record not found (CWB_KB_PR_211)"})
		}
		logger.Error("resolve canonical pipeline gate policy status failed", "id", id, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to resolve pipeline rule (CWB_KB_PR_216)"})
	}
	if strings.EqualFold(strings.TrimSpace(status), "active") {
		return c.JSON(http.StatusConflict, errorResponse{Status: false, ErrorMsg: "active policy cannot be edited (CWB_KB_PR_217)"})
	}
	fields := make([]string, 0, len(payload))
	for field := range payload {
		fields = append(fields, field)
	}
	sort.Strings(fields)
	sets := make([]string, 0, len(fields)+2)
	args := make([]any, 0, len(fields)+3)
	response := map[string]any{"id": id, "predicate_source": "json"}
	add := func(column string, value any) {
		args = append(args, value)
		sets = append(sets, fmt.Sprintf("%s = $%d", column, len(args)))
		response[column] = value
	}
	for _, field := range fields {
		raw := payload[field]
		switch field {
		case "name", "target_processor":
			var value string
			if json.Unmarshal(raw, &value) != nil || strings.TrimSpace(value) == "" {
				return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid " + field})
			}
			add(field, strings.TrimSpace(value))
		case "effect":
			var value string
			_ = json.Unmarshal(raw, &value)
			value = strings.ToLower(strings.TrimSpace(value))
			if value != "require" && value != "enable" && value != "skip" && value != "defer" {
				return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid effect (CWB_KB_PR_112)"})
			}
			add(field, value)
		case "priority":
			var value int
			if json.Unmarshal(raw, &value) != nil {
				return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid priority"})
			}
			add(field, value)
		case "active":
			var value bool
			if json.Unmarshal(raw, &value) != nil {
				return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid active"})
			}
			add(field, value)
		case "predicate":
			var doc semrules.Document
			if json.Unmarshal(raw, &doc) != nil || semrules.Validate(doc) != nil {
				return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid predicate (CWB_KB_PR_113)"})
			}
			_, checksum, err := semrules.Canonicalize(doc)
			if err != nil {
				return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid predicate (CWB_KB_PR_113)"})
			}
			analysisRaw, _ := json.Marshal(semrules.Analyze(doc).RequiredDocumentFacets)
			add("predicate", string(raw))
			add("predicate_checksum", checksum)
			add("required_facets", string(analysisRaw))
		case "predicate_checksum", "required_facets":
			// Derived from predicate and never independently editable.
		default:
			return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "no editable fields in request (CWB_KB_PR_208)"})
		}
	}
	if len(sets) == 0 {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "no editable fields in request (CWB_KB_PR_208)"})
	}
	sets = append(sets, "modify_time = NOW()")
	args = append(args, id)
	query := fmt.Sprintf("UPDATE kb.pipeline_rules SET %s WHERE id = $%d AND target_processor IS NOT NULL", strings.Join(sets, ", "), len(args))
	result, err := db.Exec(query, args...)
	if err != nil {
		logger.Error("update canonical pipeline gate failed", "id", id, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to update pipeline rule (CWB_KB_PR_209)"})
	}
	affected, err := result.RowsAffected()
	if err != nil || affected == 0 {
		return c.JSON(http.StatusNotFound, errorResponse{Status: false, ErrorMsg: "record not found (CWB_KB_PR_211)"})
	}
	return c.JSON(http.StatusOK, map[string]any{"status": true, "record": response})
}

func derefString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

// DeletePipelineRule handles DELETE /api/v1/kb/pipeline-rules/:id.
func DeletePipelineRule(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_PR_300")
	defer rc.Close()
	logger := rc.GetLogger()

	idStr := strings.TrimSpace(c.Param("id"))
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid id (CWB_KB_PR_301)"})
	}

	db := ApiTypes.ProjectDBHandle
	if c.QueryParam("predicate_source") != "legacy_adapter" {
		var gateStatus string
		gateStatusErr := db.QueryRow(`
SELECT pp.status
FROM kb.pipeline_rules r
JOIN kb.pipeline_policies pp ON pp.id = r.policy_id
WHERE r.id = $1 AND r.target_processor IS NOT NULL`, id).Scan(&gateStatus)
		if gateStatusErr == nil {
			if strings.EqualFold(strings.TrimSpace(gateStatus), "active") {
				return c.JSON(http.StatusConflict, errorResponse{Status: false, ErrorMsg: "active policy cannot be edited (CWB_KB_PR_306)"})
			}
			result, err := db.Exec("DELETE FROM kb.pipeline_rules WHERE id = $1 AND target_processor IS NOT NULL", id)
			if err != nil {
				logger.Error("delete canonical pipeline gate failed", "id", id, "err", err)
				return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to delete pipeline rule (CWB_KB_PR_302)"})
			}
			affected, err := result.RowsAffected()
			if err != nil || affected == 0 {
				return c.JSON(http.StatusNotFound, errorResponse{Status: false, ErrorMsg: "record not found (CWB_KB_PR_304)"})
			}
			return c.JSON(http.StatusOK, map[string]bool{"status": true})
		}
		if gateStatusErr != sql.ErrNoRows {
			logger.Error("resolve canonical pipeline gate policy status failed", "id", id, "err", gateStatusErr)
			return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to resolve pipeline rule policy (CWB_KB_PR_305)"})
		}
	}
	status, err := pipelineRuleCompatBindingPolicyStatus(db, id)
	if err != nil {
		if err == sql.ErrNoRows {
			return c.JSON(http.StatusNotFound, errorResponse{Status: false, ErrorMsg: "record not found (CWB_KB_PR_304)"})
		}
		logger.Error("resolve pipeline rule policy status failed", "id", id, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to resolve pipeline rule policy (CWB_KB_PR_305)"})
	}
	if status == "active" {
		return c.JSON(http.StatusConflict, errorResponse{Status: false, ErrorMsg: "active policy cannot be edited (CWB_KB_PR_306)"})
	}
	result, err := db.Exec("DELETE FROM kb.pipeline_bindings WHERE id = $1 OR legacy_rule_id = $1", id)
	if err != nil {
		logger.Error("delete pipeline rule failed", "id", id, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to delete pipeline rule (CWB_KB_PR_302)"})
	}
	affected, err := result.RowsAffected()
	if err != nil {
		logger.Error("rows affected delete pipeline rule failed", "id", id, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to verify pipeline rule delete (CWB_KB_PR_303)"})
	}
	if affected == 0 {
		return c.JSON(http.StatusNotFound, errorResponse{Status: false, ErrorMsg: "record not found (CWB_KB_PR_304)"})
	}

	return c.JSON(http.StatusOK, map[string]bool{"status": true})
}
