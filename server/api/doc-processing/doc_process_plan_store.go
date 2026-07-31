package docprocessing

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

type DocProcessPlanRecord struct {
	RunID             int64
	RecordID          int64
	PlanFacts         ProductionPlanFacts
	PlanSteps         []ProcessorPlanStep
	PipelineBinding   ProductionPipelineBindingResolution
	PipelineSelection ProductionPipelineSelection
}

type DocProcessPlanView struct {
	RunID             int64
	RecordID          int64
	PlanFacts         ProductionPlanFacts
	PlanSteps         []ProcessorPlanStep
	PipelineBinding   ProductionPipelineBindingResolution
	PipelineSelection ProductionPipelineSelection
	Mode              string
	Status            string
	Processors        []string
	Parameters        map[string]any
	CreateTime        string
}

type DocProcessPlanStore interface {
	CreateDocProcessPlan(ctx context.Context, rec DocProcessPlanRecord) (int64, error)
	GetLatestDocProcessPlan(ctx context.Context, recordID int64) (DocProcessPlanView, error)
	ListDocProcessPlans(ctx context.Context, recordID int64, page, pageSize int, statusFilter string, modeFilter string, pipelineNameFilter string) ([]DocProcessPlanView, int64, error)
}

func (s SQLStore) CreateDocProcessPlan(ctx context.Context, rec DocProcessPlanRecord) (int64, error) {
	if s.DB == nil {
		return 0, errors.New("db is nil")
	}
	if rec.RunID <= 0 {
		return 0, errors.New("run_id is required")
	}
	if rec.RecordID <= 0 {
		return 0, errors.New("record_id is required")
	}

	factsJSON, err := json.Marshal(rec.PlanFacts)
	if err != nil {
		return 0, err
	}
	stepsJSON, err := json.Marshal(rec.PlanSteps)
	if err != nil {
		return 0, err
	}
	selectionJSON, err := json.Marshal(rec.PipelineSelection)
	if err != nil {
		return 0, err
	}
	bindingJSON, err := json.Marshal(rec.PipelineBinding)
	if err != nil {
		return 0, err
	}

	const stmt = `
INSERT INTO kb.doc_process_plans (run_id, record_id, plan_facts, plan_steps, pipeline_selection, pipeline_binding)
VALUES ($1, $2, $3::jsonb, $4::jsonb, $5::jsonb, $6::jsonb)
RETURNING id`

	var id int64
	err = s.DB.QueryRowContext(ctx, stmt, rec.RunID, rec.RecordID, string(factsJSON), string(stepsJSON), string(selectionJSON), string(bindingJSON)).Scan(&id)
	if err != nil {
		return 0, err
	}
	return id, nil
}

func (s SQLStore) GetLatestDocProcessPlan(ctx context.Context, recordID int64) (DocProcessPlanView, error) {
	if s.DB == nil {
		return DocProcessPlanView{}, errors.New("db is nil")
	}
	if recordID <= 0 {
		return DocProcessPlanView{}, errors.New("record_id is required")
	}

	const stmt = `
SELECT p.run_id,
       p.record_id,
       p.plan_facts::text,
       p.plan_steps::text,
       p.pipeline_selection::text,
       p.pipeline_binding::text,
       COALESCE(r.mode, ''),
       COALESCE(r.status, ''),
       COALESCE(r.processors::text, '[]'::text),
       COALESCE(r.parameters::text, '{}'::text),
       COALESCE(to_char(p.create_time, 'YYYY-MM-DD"T"HH24:MI:SSOF'), '')
FROM kb.doc_process_plans p
JOIN kb.doc_process_runs r ON r.id = p.run_id
WHERE p.record_id = $1
ORDER BY p.create_time DESC, p.id DESC
LIMIT 1`

	var (
		view           DocProcessPlanView
		planFactsJSON  string
		planStepsJSON  string
		selectionJSON  string
		bindingJSON    string
		processorsJSON string
		parametersJSON string
	)
	err := s.DB.QueryRowContext(ctx, stmt, recordID).Scan(
		&view.RunID,
		&view.RecordID,
		&planFactsJSON,
		&planStepsJSON,
		&selectionJSON,
		&bindingJSON,
		&view.Mode,
		&view.Status,
		&processorsJSON,
		&parametersJSON,
		&view.CreateTime,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return DocProcessPlanView{}, sql.ErrNoRows
		}
		return DocProcessPlanView{}, err
	}
	if err := json.Unmarshal([]byte(planFactsJSON), &view.PlanFacts); err != nil {
		return DocProcessPlanView{}, err
	}
	if err := json.Unmarshal([]byte(planStepsJSON), &view.PlanSteps); err != nil {
		return DocProcessPlanView{}, err
	}
	if err := json.Unmarshal([]byte(selectionJSON), &view.PipelineSelection); err != nil {
		return DocProcessPlanView{}, err
	}
	if err := json.Unmarshal([]byte(bindingJSON), &view.PipelineBinding); err != nil {
		return DocProcessPlanView{}, err
	}
	if err := json.Unmarshal([]byte(processorsJSON), &view.Processors); err != nil {
		return DocProcessPlanView{}, err
	}
	if err := json.Unmarshal([]byte(parametersJSON), &view.Parameters); err != nil {
		return DocProcessPlanView{}, err
	}
	if view.Processors == nil {
		view.Processors = []string{}
	}
	if view.Parameters == nil {
		view.Parameters = map[string]any{}
	}
	return view, nil
}

func (s SQLStore) ListDocProcessPlans(ctx context.Context, recordID int64, page, pageSize int, statusFilter string, modeFilter string, pipelineNameFilter string) ([]DocProcessPlanView, int64, error) {
	if s.DB == nil {
		return nil, 0, errors.New("db is nil")
	}
	if recordID <= 0 {
		return nil, 0, errors.New("record_id is required")
	}
	if page < 1 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 50
	}
	offset := (page - 1) * pageSize

	whereParts := []string{"p.record_id = $1"}
	args := []any{recordID}
	nextArg := func(v any) string {
		args = append(args, v)
		return fmt.Sprintf("$%d", len(args))
	}
	if trimmed := strings.TrimSpace(statusFilter); trimmed != "" {
		whereParts = append(whereParts, fmt.Sprintf("COALESCE(r.status, '') = %s", nextArg(trimmed)))
	}
	if trimmed := strings.TrimSpace(modeFilter); trimmed != "" {
		whereParts = append(whereParts, fmt.Sprintf("COALESCE(r.mode, '') = %s", nextArg(trimmed)))
	}
	if trimmed := strings.TrimSpace(pipelineNameFilter); trimmed != "" {
		whereParts = append(whereParts, fmt.Sprintf("p.pipeline_selection->>'PipelineName' = %s", nextArg(trimmed)))
	}
	whereSQL := strings.Join(whereParts, " AND ")

	countStmt := fmt.Sprintf(`
SELECT COUNT(*)
FROM kb.doc_process_plans p
JOIN kb.doc_process_runs r ON r.id = p.run_id
WHERE %s`, whereSQL)
	var total int64
	if err := s.DB.QueryRowContext(ctx, countStmt, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	listStmt := fmt.Sprintf(`
SELECT p.run_id,
       p.record_id,
       p.plan_facts::text,
       p.plan_steps::text,
       p.pipeline_selection::text,
       p.pipeline_binding::text,
       COALESCE(r.mode, ''),
       COALESCE(r.status, ''),
       COALESCE(r.processors::text, '[]'::text),
       COALESCE(r.parameters::text, '{}'::text),
       COALESCE(to_char(p.create_time, 'YYYY-MM-DD"T"HH24:MI:SSOF'), '')
FROM kb.doc_process_plans p
JOIN kb.doc_process_runs r ON r.id = p.run_id
WHERE %s
ORDER BY p.create_time DESC, p.id DESC
LIMIT %s OFFSET %s`, whereSQL, nextArg(pageSize), nextArg(offset))
	rows, err := s.DB.QueryContext(ctx, listStmt, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	out := make([]DocProcessPlanView, 0)
	for rows.Next() {
		var (
			view           DocProcessPlanView
			planFactsJSON  string
			planStepsJSON  string
			selectionJSON  string
			bindingJSON    string
			processorsJSON string
			parametersJSON string
		)
		if err := rows.Scan(
			&view.RunID,
			&view.RecordID,
			&planFactsJSON,
			&planStepsJSON,
			&selectionJSON,
			&bindingJSON,
			&view.Mode,
			&view.Status,
			&processorsJSON,
			&parametersJSON,
			&view.CreateTime,
		); err != nil {
			return nil, 0, err
		}
		if err := json.Unmarshal([]byte(planFactsJSON), &view.PlanFacts); err != nil {
			return nil, 0, err
		}
		if err := json.Unmarshal([]byte(planStepsJSON), &view.PlanSteps); err != nil {
			return nil, 0, err
		}
		if err := json.Unmarshal([]byte(selectionJSON), &view.PipelineSelection); err != nil {
			return nil, 0, err
		}
		if err := json.Unmarshal([]byte(bindingJSON), &view.PipelineBinding); err != nil {
			return nil, 0, err
		}
		if err := json.Unmarshal([]byte(processorsJSON), &view.Processors); err != nil {
			return nil, 0, err
		}
		if err := json.Unmarshal([]byte(parametersJSON), &view.Parameters); err != nil {
			return nil, 0, err
		}
		if view.Processors == nil {
			view.Processors = []string{}
		}
		if view.Parameters == nil {
			view.Parameters = map[string]any{}
		}
		out = append(out, view)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return out, total, nil
}
