package appdatastores

import (
	"database/sql"
	"fmt"

	"github.com/chendingplano/deepdoc/server/cmd/config"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/databaseutil"
	_ "github.com/lib/pq"
)

// TableDspyPromptDef represents a DSPy-optimized prompt configuration stored in the database.
type TableDspyPromptDef struct {
	PromptID               string `json:"prompt_id"`
	UserID                 string `json:"user_id"`
	PromptName             string `json:"prompt_name"`
	PromptDesc             string `json:"prompt_desc"`
	TaskType               string `json:"task_type"`
	SignatureInputs        string `json:"signature_inputs"`         // JSON array of SignatureField
	SignatureOutputs       string `json:"signature_outputs"`        // JSON array of SignatureField
	SignatureDocstring     string `json:"signature_docstring"`
	ModuleType             string `json:"module_type"`              // Predict, ChainOfThought, ReAct, ...
	Examples               string `json:"examples"`                 // JSON array of TrainingExample
	Optimizer              string `json:"optimizer"`                // BootstrapFewShot, MIPROv2, ...
	OptimizerConfig        string `json:"optimizer_config"`         // JSON object
	OptimizedInstructions  string `json:"optimized_instructions"`
	OptimizedExamples      string `json:"optimized_examples"`       // JSON array of TrainingExample
	Status                 string `json:"status"`                   // draft, optimizing, optimized, failed
	CreatedAt              string `json:"created_at"`
	UpdatedAt              string `json:"updated_at"`
}

// CreateDspyPromptsTable creates the dspy_prompts table if it does not already exist.
func CreateDspyPromptsTable(logger ApiTypes.JimoLogger) error {
	db_type := ApiTypes.DBType
	table_name := config.AppConfig.AppTableNames.TableName_DspyPrompts
	var stmt string
	var db *sql.DB = ApiTypes.ProjectDBHandle

	const common_fields = "" +
		"user_id                 VARCHAR(128)    DEFAULT NULL, " +
		"prompt_name             VARCHAR(256)    NOT NULL, " +
		"prompt_desc             TEXT            DEFAULT NULL, " +
		"task_type               VARCHAR(64)     DEFAULT NULL, " +
		"signature_inputs        TEXT            DEFAULT NULL, " +
		"signature_outputs       TEXT            DEFAULT NULL, " +
		"signature_docstring     TEXT            DEFAULT NULL, " +
		"module_type             VARCHAR(64)     NOT NULL DEFAULT 'ChainOfThought', " +
		"examples                TEXT            DEFAULT NULL, " +
		"optimizer               VARCHAR(64)     NOT NULL DEFAULT 'BootstrapFewShot', " +
		"optimizer_config        TEXT            DEFAULT NULL, " +
		"optimized_instructions  TEXT            DEFAULT NULL, " +
		"optimized_examples      TEXT            DEFAULT NULL, " +
		"status                  VARCHAR(32)     NOT NULL DEFAULT 'draft', " +
		"created_at              TIMESTAMP       DEFAULT CURRENT_TIMESTAMP, " +
		"updated_at              TIMESTAMP       DEFAULT CURRENT_TIMESTAMP"

	switch db_type {
	case ApiTypes.MysqlName:
		stmt = "CREATE TABLE IF NOT EXISTS " + table_name + "(" +
			"prompt_id BIGINT AUTO_INCREMENT PRIMARY KEY, " + common_fields +
			") ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;"

	case ApiTypes.PgName:
		stmt = "CREATE TABLE IF NOT EXISTS " + table_name + "(" +
			"prompt_id BIGSERIAL PRIMARY KEY, " + common_fields + ")"

	default:
		return fmt.Errorf("database type not supported: %s (CWB_DSP_044)", db_type)
	}

	err := databaseutil.ExecuteStatement(db, stmt)
	if err != nil {
		return fmt.Errorf("failed creating dspy_prompts table (CWB_DSP_051), err: %w, stmt: %s", err, stmt)
	}

	logger.Info("Creating table success", "tablename", table_name)
	return nil
}

// InsertDspyPrompt inserts a new DSPy prompt record into the dspy_prompts table.
func InsertDspyPrompt(db *sql.DB, tableName string, r TableDspyPromptDef) (int64, error) {
	stmt := fmt.Sprintf(`
		INSERT INTO %s (
			user_id, prompt_name, prompt_desc, task_type,
			signature_inputs, signature_outputs, signature_docstring,
			module_type, examples, optimizer, optimizer_config, status
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12
		) RETURNING prompt_id`, tableName)

	var newID int64
	err := db.QueryRow(stmt,
		r.UserID, r.PromptName, r.PromptDesc, r.TaskType,
		r.SignatureInputs, r.SignatureOutputs, r.SignatureDocstring,
		r.ModuleType, r.Examples, r.Optimizer, r.OptimizerConfig, r.Status,
	).Scan(&newID)
	if err != nil {
		return 0, fmt.Errorf("failed to insert dspy prompt (CWB_DSP_061), err: %w", err)
	}
	return newID, nil
}

// ListDspyPrompts retrieves all DSPy prompts, optionally filtered by user_id.
func ListDspyPrompts(db *sql.DB, tableName, userID string, limit int) ([]TableDspyPromptDef, error) {
	query := fmt.Sprintf(`
		SELECT prompt_id, user_id, prompt_name, prompt_desc, task_type,
		       signature_inputs, signature_outputs, signature_docstring,
		       module_type, examples, optimizer, optimizer_config,
		       optimized_instructions, optimized_examples, status,
		       created_at, updated_at
		FROM %s`, tableName)

	var args []interface{}
	if userID != "" {
		query += " WHERE user_id = $1"
		args = append(args, userID)
	}
	query += " ORDER BY created_at DESC"
	if limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", limit)
	}

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list dspy prompts (CWB_DSP_071), err: %w", err)
	}
	defer rows.Close()

	var results []TableDspyPromptDef
	for rows.Next() {
		var r TableDspyPromptDef
		if err := rows.Scan(
			&r.PromptID, &r.UserID, &r.PromptName, &r.PromptDesc, &r.TaskType,
			&r.SignatureInputs, &r.SignatureOutputs, &r.SignatureDocstring,
			&r.ModuleType, &r.Examples, &r.Optimizer, &r.OptimizerConfig,
			&r.OptimizedInstructions, &r.OptimizedExamples, &r.Status,
			&r.CreatedAt, &r.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan dspy prompt row (CWB_DSP_076), err: %w", err)
		}
		results = append(results, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating dspy prompt rows (CWB_DSP_081), err: %w", err)
	}
	return results, nil
}

// GetDspyPromptByID retrieves a single DSPy prompt by its ID.
func GetDspyPromptByID(db *sql.DB, tableName, promptID string) (*TableDspyPromptDef, error) {
	query := fmt.Sprintf(`
		SELECT prompt_id, user_id, prompt_name, prompt_desc, task_type,
		       signature_inputs, signature_outputs, signature_docstring,
		       module_type, examples, optimizer, optimizer_config,
		       optimized_instructions, optimized_examples, status,
		       created_at, updated_at
		FROM %s WHERE prompt_id = $1`, tableName)

	var r TableDspyPromptDef
	err := db.QueryRow(query, promptID).Scan(
		&r.PromptID, &r.UserID, &r.PromptName, &r.PromptDesc, &r.TaskType,
		&r.SignatureInputs, &r.SignatureOutputs, &r.SignatureDocstring,
		&r.ModuleType, &r.Examples, &r.Optimizer, &r.OptimizerConfig,
		&r.OptimizedInstructions, &r.OptimizedExamples, &r.Status,
		&r.CreatedAt, &r.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get dspy prompt by id (CWB_DSP_091), err: %w", err)
	}
	return &r, nil
}

// UpdateDspyPrompt updates an existing DSPy prompt record.
func UpdateDspyPrompt(db *sql.DB, tableName string, r TableDspyPromptDef) error {
	stmt := fmt.Sprintf(`
		UPDATE %s SET
			prompt_name             = $1,
			prompt_desc             = $2,
			task_type               = $3,
			signature_inputs        = $4,
			signature_outputs       = $5,
			signature_docstring     = $6,
			module_type             = $7,
			examples                = $8,
			optimizer               = $9,
			optimizer_config        = $10,
			optimized_instructions  = $11,
			optimized_examples      = $12,
			status                  = $13,
			updated_at              = CURRENT_TIMESTAMP
		WHERE prompt_id = $14`, tableName)

	result, err := db.Exec(stmt,
		r.PromptName, r.PromptDesc, r.TaskType,
		r.SignatureInputs, r.SignatureOutputs, r.SignatureDocstring,
		r.ModuleType, r.Examples, r.Optimizer, r.OptimizerConfig,
		r.OptimizedInstructions, r.OptimizedExamples, r.Status,
		r.PromptID,
	)
	if err != nil {
		return fmt.Errorf("failed to update dspy prompt (CWB_DSP_101), err: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("no dspy prompt found with id: %s (CWB_DSP_106)", r.PromptID)
	}
	return nil
}

// DeleteDspyPrompt hard-deletes a DSPy prompt record by ID.
func DeleteDspyPrompt(db *sql.DB, tableName, promptID string) error {
	stmt := fmt.Sprintf("DELETE FROM %s WHERE prompt_id = $1", tableName)
	result, err := db.Exec(stmt, promptID)
	if err != nil {
		return fmt.Errorf("failed to delete dspy prompt (CWB_DSP_111), err: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("no dspy prompt found with id: %s (CWB_DSP_116)", promptID)
	}
	return nil
}
