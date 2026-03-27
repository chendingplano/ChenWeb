package appdatastores

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/databaseutil"
)

const (
	DocGenJobStatusPending    = "pending"
	DocGenJobStatusProcessing = "processing"
	DocGenJobStatusCompleted  = "completed"
	DocGenJobStatusFailed     = "failed"
)

type DocGenJob struct {
	JobID        int64     `json:"job_id"`
	RequestName  string    `json:"request_name"`
	Purpose      string    `json:"purpose"`
	Remarks      string    `json:"remarks"`
	SQLQueryID   *int64    `json:"sql_query_id"`
	SQLStatement string    `json:"sql_statement"`
	TemplateType string    `json:"template_type"`
	TemplatePath string    `json:"template_path"`
	Converter    string    `json:"converter"` // raw JSON string
	OutputDir    string    `json:"output_dir"`
	OutputFormat string    `json:"output_format"`
	Status       string    `json:"status"`
	TotalCount   int       `json:"total_count"`
	SuccessCount int       `json:"success_count"`
	FailCount    int       `json:"fail_count"`
	ErrorMsg     string    `json:"error_msg"`
	CreatedBy    string    `json:"created_by"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

func CreateDocGenJobsTable(logger ApiTypes.JimoLogger) error {
	db := ApiTypes.ProjectDBHandle
	stmt := `CREATE TABLE IF NOT EXISTS doc_gen_jobs (
		job_id         BIGSERIAL PRIMARY KEY,
		request_name   VARCHAR(255) NOT NULL UNIQUE,
		purpose        VARCHAR(255) NOT NULL,
		remarks        TEXT,
		sql_query_id   BIGINT,
		sql_statement  TEXT NOT NULL,
		template_type  VARCHAR(16) NOT NULL,
		template_path  TEXT NOT NULL,
		converter      JSONB NOT NULL,
		output_dir     TEXT NOT NULL,
		output_format  VARCHAR(16) NOT NULL,
		status         VARCHAR(32) NOT NULL DEFAULT 'pending',
		total_count    INT NOT NULL DEFAULT 0,
		success_count  INT NOT NULL DEFAULT 0,
		fail_count     INT NOT NULL DEFAULT 0,
		error_msg      TEXT,
		created_by     VARCHAR(255) NOT NULL,
		created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
	)`
	if err := databaseutil.ExecuteStatement(db, stmt); err != nil {
		return fmt.Errorf("failed creating doc_gen_jobs table: %w (CWB_DGS_100)", err)
	}
	logger.Info("Created doc_gen_jobs table")
	return nil
}

func InsertDocGenJob(db *sql.DB, job DocGenJob) (int64, error) {
	var id int64
	err := db.QueryRow(
		`INSERT INTO doc_gen_jobs
		 (request_name, purpose, remarks, sql_query_id, sql_statement,
		  template_type, template_path, converter, output_dir, output_format,
		  status, created_by)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8::jsonb,$9,$10,$11,$12)
		 RETURNING job_id`,
		job.RequestName, job.Purpose, job.Remarks, job.SQLQueryID, job.SQLStatement,
		job.TemplateType, job.TemplatePath, job.Converter, job.OutputDir, job.OutputFormat,
		DocGenJobStatusPending, job.CreatedBy,
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("InsertDocGenJob failed: %w (CWB_DGS_110)", err)
	}
	return id, nil
}

func UpdateDocGenJobStatus(db *sql.DB, jobID int64, status, errorMsg string) error {
	_, err := db.Exec(
		`UPDATE doc_gen_jobs SET status=$1, error_msg=$2, updated_at=NOW() WHERE job_id=$3`,
		status, errorMsg, jobID,
	)
	if err != nil {
		return fmt.Errorf("UpdateDocGenJobStatus failed: %w (CWB_DGS_120)", err)
	}
	return nil
}

func UpdateDocGenJobCounts(db *sql.DB, jobID int64, total, success, fail int) error {
	_, err := db.Exec(
		`UPDATE doc_gen_jobs SET total_count=$1, success_count=$2, fail_count=$3, updated_at=NOW() WHERE job_id=$4`,
		total, success, fail, jobID,
	)
	if err != nil {
		return fmt.Errorf("UpdateDocGenJobCounts failed: %w (CWB_DGS_130)", err)
	}
	return nil
}

func GetDocGenJob(db *sql.DB, jobID int64) (*DocGenJob, error) {
	var job DocGenJob
	var sqlQueryID sql.NullInt64
	var remarks, errorMsg sql.NullString
	err := db.QueryRow(
		`SELECT job_id, request_name, purpose, remarks, sql_query_id, sql_statement,
		        template_type, template_path, converter, output_dir, output_format,
		        status, total_count, success_count, fail_count, error_msg,
		        created_by, created_at, updated_at
		 FROM doc_gen_jobs WHERE job_id=$1`, jobID,
	).Scan(&job.JobID, &job.RequestName, &job.Purpose, &remarks, &sqlQueryID,
		&job.SQLStatement, &job.TemplateType, &job.TemplatePath, &job.Converter,
		&job.OutputDir, &job.OutputFormat, &job.Status,
		&job.TotalCount, &job.SuccessCount, &job.FailCount, &errorMsg,
		&job.CreatedBy, &job.CreatedAt, &job.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("GetDocGenJob failed: %w (CWB_DGS_140)", err)
	}
	if sqlQueryID.Valid {
		job.SQLQueryID = &sqlQueryID.Int64
	}
	job.Remarks = remarks.String
	job.ErrorMsg = errorMsg.String
	return &job, nil
}

func ListDocGenJobs(db *sql.DB, status, requestName string, page, pageSize int) ([]DocGenJob, int64, error) {
	where := []string{}
	args := []any{}
	n := 1
	if status != "" {
		where = append(where, fmt.Sprintf("status=$%d", n))
		args = append(args, status)
		n++
	}
	if requestName != "" {
		where = append(where, fmt.Sprintf("request_name ILIKE $%d", n))
		args = append(args, "%"+requestName+"%")
		n++
	}
	whereSQL := ""
	if len(where) > 0 {
		whereSQL = "WHERE " + strings.Join(where, " AND ")
	}

	var total int64
	if err := db.QueryRow(
		fmt.Sprintf(`SELECT COUNT(1) FROM doc_gen_jobs %s`, whereSQL), args...,
	).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("ListDocGenJobs count failed: %w (CWB_DGS_150)", err)
	}

	offset := (page - 1) * pageSize
	args = append(args, pageSize, offset)
	rows, err := db.Query(
		fmt.Sprintf(`SELECT job_id, request_name, purpose, remarks, sql_query_id,
		             sql_statement, template_type, template_path, converter, output_dir,
		             output_format, status, total_count, success_count, fail_count,
		             error_msg, created_by, created_at, updated_at
		             FROM doc_gen_jobs %s
		             ORDER BY created_at DESC LIMIT $%d OFFSET $%d`,
			whereSQL, n, n+1), args...)
	if err != nil {
		return nil, 0, fmt.Errorf("ListDocGenJobs query failed: %w (CWB_DGS_155)", err)
	}
	defer rows.Close()

	var results []DocGenJob
	for rows.Next() {
		var job DocGenJob
		var sqlQueryID sql.NullInt64
		var remarks, errorMsg sql.NullString
		if err := rows.Scan(&job.JobID, &job.RequestName, &job.Purpose, &remarks, &sqlQueryID,
			&job.SQLStatement, &job.TemplateType, &job.TemplatePath, &job.Converter,
			&job.OutputDir, &job.OutputFormat, &job.Status,
			&job.TotalCount, &job.SuccessCount, &job.FailCount, &errorMsg,
			&job.CreatedBy, &job.CreatedAt, &job.UpdatedAt); err != nil {
			return nil, 0, fmt.Errorf("scan failed: %w (CWB_DGS_160)", err)
		}
		if sqlQueryID.Valid {
			job.SQLQueryID = &sqlQueryID.Int64
		}
		job.Remarks = remarks.String
		job.ErrorMsg = errorMsg.String
		results = append(results, job)
	}
	if results == nil {
		results = []DocGenJob{}
	}
	return results, total, rows.Err()
}

func ListStalledDocGenJobs(db *sql.DB) ([]int64, error) {
	rows, err := db.Query(
		`SELECT job_id FROM doc_gen_jobs WHERE status IN ('pending','processing') ORDER BY created_at ASC`,
	)
	if err != nil {
		return nil, fmt.Errorf("ListStalledDocGenJobs failed: %w (CWB_DGS_170)", err)
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
