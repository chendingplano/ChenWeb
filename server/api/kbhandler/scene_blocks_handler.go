package kbhandler

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/EchoFactory"
	"github.com/labstack/echo/v4"
)

// sceneBlockRecord is one row from kb.scene_objects flattened for the UI.
// JSONB array columns are passed through verbatim as json.RawMessage so the
// frontend receives the LLM-extracted structure without a re-marshal round trip.
type sceneBlockRecord struct {
	ID             int64           `json:"id"`
	ObjectID       string          `json:"object_id"`
	EventID        string          `json:"event_id"`
	SceneID        string          `json:"scene_id"`
	SceneType      string          `json:"scene_type"`
	Title          string          `json:"title"`
	TitleEn        string          `json:"title_en,omitempty"`
	Summary        string          `json:"summary"`
	SummaryEn      string          `json:"summary_en,omitempty"`
	EvidenceLines  json.RawMessage `json:"evidence_lines,omitempty"`
	Actors         json.RawMessage `json:"actors"`
	Resources      json.RawMessage `json:"resources"`
	Preconditions  json.RawMessage `json:"preconditions"`
	Triggers       json.RawMessage `json:"triggers"`
	States         json.RawMessage `json:"states"`
	StatesEn       json.RawMessage `json:"states_en,omitempty"`
	Actions        json.RawMessage `json:"actions"`
	Constraints    json.RawMessage `json:"constraints"`
	Decisions      json.RawMessage `json:"decisions"`
	Outcomes       json.RawMessage `json:"outcomes"`
	FailureModes   json.RawMessage `json:"failure_modes"`
	RootCauses     json.RawMessage `json:"root_causes"`
	Resolutions    json.RawMessage `json:"resolutions"`
	Relationships  json.RawMessage `json:"relationships"`
	Discriminators json.RawMessage `json:"discriminators"`
	Keywords       json.RawMessage `json:"keywords"`
	KeywordsEn     json.RawMessage `json:"keywords_en,omitempty"`
	Confidence     float64         `json:"confidence"`
	SourceRefs     json.RawMessage `json:"source_refs"`
	ModelName      string          `json:"model_name"`
	PromptName     string          `json:"prompt_name"`
	CreateTime     string          `json:"create_time"`
	ModifyTime     string          `json:"modify_time"`
}

type listSceneBlocksResponse struct {
	Status   bool               `json:"status"`
	InputID  int64              `json:"input_id"`
	FileName string             `json:"file_name,omitempty"`
	Results  []sceneBlockRecord `json:"results"`
	Total    int                `json:"total"`
}

// jsonArrayOrEmpty guarantees the column is valid JSON for the client. JSONB
// columns default to '[]'/'{}' so this only guards against a NULL slip.
func jsonArrayOrEmpty(b []byte) json.RawMessage {
	if len(b) == 0 {
		return json.RawMessage("[]")
	}
	return json.RawMessage(b)
}

func normalizeJSONStringArray(v any) json.RawMessage {
	items, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		s, ok := item.(string)
		if !ok {
			continue
		}
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		out = append(out, s)
	}
	if len(out) == 0 {
		return nil
	}
	b, err := json.Marshal(out)
	if err != nil {
		return nil
	}
	return json.RawMessage(b)
}

func applySceneBlockExtInfo(r *sceneBlockRecord, raw json.RawMessage) {
	if len(raw) == 0 || string(raw) == "null" {
		return
	}
	var ext map[string]any
	if err := json.Unmarshal(raw, &ext); err != nil {
		return
	}
	if titleEn := strings.TrimSpace(fmt.Sprint(ext["title_en"])); titleEn != "" && titleEn != "<nil>" {
		r.TitleEn = titleEn
	}
	if summaryEn := strings.TrimSpace(fmt.Sprint(ext["summary_en"])); summaryEn != "" && summaryEn != "<nil>" {
		r.SummaryEn = summaryEn
	}
	if evidenceLines := normalizeJSONStringArray(ext["evidence_lines"]); len(evidenceLines) > 0 {
		r.EvidenceLines = evidenceLines
	}
	if keywordsEn := normalizeJSONStringArray(ext["keywords_en"]); len(keywordsEn) > 0 {
		r.KeywordsEn = keywordsEn
	}
	if statesEn := normalizeJSONStringArray(ext["states_en"]); len(statesEn) > 0 {
		r.StatesEn = statesEn
	}
}

// ListSceneBlocks handles GET /api/v1/kb/scene-blocks?input_record_id=N.
// It returns every scene block extracted for the record from kb.scene_objects,
// ordered by primary key (extraction order).
func ListSceneBlocks(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_SB_001")
	defer rc.Close()
	logger := rc.GetLogger()

	idStr := strings.TrimSpace(c.QueryParam("input_record_id"))
	inputID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || inputID <= 0 {
		return c.JSON(http.StatusBadRequest, errorResponse{
			Status: false, ErrorMsg: "invalid input_record_id (CWB_KB_SB_010)",
		})
	}

	db := ApiTypes.ProjectDBHandle
	if db == nil {
		logger.Error("project db handle is nil")
		return c.JSON(http.StatusInternalServerError, errorResponse{
			Status: false, ErrorMsg: "database unavailable (CWB_KB_SB_011)",
		})
	}

	// file_name lets the UI decide whether the source can render as a PDF.
	// It is best effort: a lookup failure must not hide the scene blocks.
	var fileName string
	if inputTable, terr := resolveInputTable(db); terr == nil {
		var fn sql.NullString
		q := fmt.Sprintf(`SELECT file_name FROM %s WHERE id = $1`, inputTable)
		if qerr := db.QueryRow(q, inputID).Scan(&fn); qerr == nil && fn.Valid {
			fileName = strings.TrimSpace(fn.String)
		}
	}

	const q = `
SELECT
	id, object_id, event_id, scene_id, scene_type, title, summary,
	actors, resources, preconditions, triggers, states, actions, constraints,
	decisions, outcomes, failure_modes, root_causes, resolutions, relationships,
	discriminators, keywords, confidence, source_refs, model_name, prompt_name, ext_info,
	create_time, modify_time
FROM kb.scene_objects
WHERE input_record_id = $1
ORDER BY id`

	rows, err := db.Query(q, inputID)
	if err != nil {
		logger.Error("query kb.scene_objects failed", "err", err, "input_id", inputID)
		return c.JSON(http.StatusInternalServerError, errorResponse{
			Status: false, ErrorMsg: "failed to load scene blocks (CWB_KB_SB_020)",
		})
	}
	defer rows.Close()

	results := make([]sceneBlockRecord, 0, 16)
	for rows.Next() {
		var (
			r          sceneBlockRecord
			actors     []byte
			resources  []byte
			precond    []byte
			triggers   []byte
			states     []byte
			actions    []byte
			constr     []byte
			decisions  []byte
			outcomes   []byte
			failure    []byte
			rootCauses []byte
			resolution []byte
			relations  []byte
			discrim    []byte
			keywords   []byte
			srcRefs    []byte
			extInfo    []byte
			createTime time.Time
			modifyTime time.Time
		)
		if err := rows.Scan(
			&r.ID, &r.ObjectID, &r.EventID, &r.SceneID, &r.SceneType, &r.Title, &r.Summary,
			&actors, &resources, &precond, &triggers, &states, &actions, &constr,
			&decisions, &outcomes, &failure, &rootCauses, &resolution, &relations,
			&discrim, &keywords, &r.Confidence, &srcRefs, &r.ModelName, &r.PromptName, &extInfo,
			&createTime, &modifyTime,
		); err != nil {
			logger.Error("scan kb.scene_objects row failed", "err", err, "input_id", inputID)
			return c.JSON(http.StatusInternalServerError, errorResponse{
				Status: false, ErrorMsg: "failed to read scene blocks (CWB_KB_SB_021)",
			})
		}
		r.Actors = jsonArrayOrEmpty(actors)
		r.Resources = jsonArrayOrEmpty(resources)
		r.Preconditions = jsonArrayOrEmpty(precond)
		r.Triggers = jsonArrayOrEmpty(triggers)
		r.States = jsonArrayOrEmpty(states)
		r.Actions = jsonArrayOrEmpty(actions)
		r.Constraints = jsonArrayOrEmpty(constr)
		r.Decisions = jsonArrayOrEmpty(decisions)
		r.Outcomes = jsonArrayOrEmpty(outcomes)
		r.FailureModes = jsonArrayOrEmpty(failure)
		r.RootCauses = jsonArrayOrEmpty(rootCauses)
		r.Resolutions = jsonArrayOrEmpty(resolution)
		r.Relationships = jsonArrayOrEmpty(relations)
		r.Discriminators = jsonArrayOrEmpty(discrim)
		r.Keywords = jsonArrayOrEmpty(keywords)
		r.SourceRefs = jsonArrayOrEmpty(srcRefs)
		applySceneBlockExtInfo(&r, json.RawMessage(extInfo))
		r.CreateTime = createTime.Format(time.RFC3339)
		r.ModifyTime = modifyTime.Format(time.RFC3339)
		results = append(results, r)
	}
	if err := rows.Err(); err != nil {
		logger.Error("iterate kb.scene_objects rows failed", "err", err, "input_id", inputID)
		return c.JSON(http.StatusInternalServerError, errorResponse{
			Status: false, ErrorMsg: "failed to read scene blocks (CWB_KB_SB_022)",
		})
	}

	logger.Info("kb scene-blocks response",
		"input_id", inputID,
		"file_name", fileName,
		"scene_block_count", len(results),
	)
	return c.JSON(http.StatusOK, listSceneBlocksResponse{
		Status:   true,
		InputID:  inputID,
		FileName: fileName,
		Results:  results,
		Total:    len(results),
	})
}
