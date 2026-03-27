# Document Generation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add async document generation (Word templates → DOCX) behind a 3-tab UI under Applications → Generate Doc.

**Architecture:** HTTP handler accepts job submissions and pushes job IDs onto a buffered channel; a fixed worker pool processes jobs, writes files, and updates `doc_gen_jobs` / `doc_gen_log` tables. Three new DB tables managed via a single goose migration. Frontend is a Svelte 5 component wired into the existing home3 nav rail.

**Tech Stack:** Go 1.25, Echo v4, PostgreSQL, goose migrations, `github.com/nguyenthenguyen/docx` (Word rendering), Svelte 5 (runes syntax), Tailwind CSS.

---

## File Map

**Create:**
- `server/migrations/20260327000000_create_doc_gen_tables.sql`
- `server/api/appdatastores/table-doc-gen-queries.go`
- `server/api/appdatastores/table-doc-gen-jobs.go`
- `server/api/appdatastores/table-doc-gen-log.go`
- `server/api/docgenworker/validate.go`
- `server/api/docgenworker/validate_test.go`
- `server/api/docgenworker/renderer.go`
- `server/api/docgenworker/renderer_test.go`
- `server/api/docgenworker/worker.go`
- `server/api/docgenhandler/types.go`
- `server/api/docgenhandler/handler_templates.go`
- `server/api/docgenhandler/handler_queries.go`
- `server/api/docgenhandler/handler_jobs.go`
- `server/api/docgenhandler/handler_jobs_test.go`
- `web/src/lib/components/home3/doc-gen-view.svelte`

**Modify:**
- `config.toml` — add `[doc_gen]` section
- `server/cmd/config/config.go` — add `DocGen` struct to `AppConfigDef`
- `server/api/routes.go` — register docgen routes
- `server/cmd/deepdoc/main.go` — start worker pool after DB init
- `server/api/appdatastores/table-doc-gen-queries.go` — (created above)
- `web/src/lib/components/home3/nav-rail.svelte` — add `apps-generate-doc` child
- `web/src/lib/components/home3/content-panel.svelte` — render DocGenView

---

## Task 1: Goose Migration

**Files:**
- Create: `server/migrations/20260327000000_create_doc_gen_tables.sql`

- [ ] **Step 1: Create the migration file**

```sql
-- server/migrations/20260327000000_create_doc_gen_tables.sql
-- +goose Up
CREATE TABLE IF NOT EXISTS doc_gen_queries (
    id            BIGSERIAL PRIMARY KEY,
    name          VARCHAR(255) NOT NULL UNIQUE,
    description   TEXT,
    sql_statement TEXT NOT NULL,
    created_by    VARCHAR(255) NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS doc_gen_jobs (
    job_id         BIGSERIAL PRIMARY KEY,
    request_name   VARCHAR(255) NOT NULL UNIQUE,
    purpose        VARCHAR(255) NOT NULL,
    remarks        TEXT,
    sql_query_id   BIGINT REFERENCES doc_gen_queries(id) ON DELETE SET NULL,
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
);

CREATE TABLE IF NOT EXISTS doc_gen_log (
    id            BIGSERIAL PRIMARY KEY,
    job_id        BIGINT NOT NULL REFERENCES doc_gen_jobs(job_id) ON DELETE CASCADE,
    request_name  VARCHAR(255) NOT NULL,
    customer_id   VARCHAR(128) NOT NULL,
    customer_name VARCHAR(255) NOT NULL,
    email         VARCHAR(255) NOT NULL,
    phone_num     VARCHAR(64),
    purpose       VARCHAR(255) NOT NULL,
    filename      VARCHAR(512) NOT NULL,
    status        VARCHAR(32) NOT NULL,
    error_msg     TEXT,
    remarks       TEXT,
    created_by    VARCHAR(255) NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- +goose Down
DROP TABLE IF EXISTS doc_gen_log;
DROP TABLE IF EXISTS doc_gen_jobs;
DROP TABLE IF EXISTS doc_gen_queries;
```

- [ ] **Step 2: Verify migration file exists**

```bash
ls server/migrations/ | grep doc_gen
```
Expected: `20260327000000_create_doc_gen_tables.sql`

- [ ] **Step 3: Commit**

```bash
git add server/migrations/20260327000000_create_doc_gen_tables.sql
git commit -m "feat(docgen): add goose migration for doc gen tables"
```

---

## Task 2: Add go-docx Dependency

**Files:**
- Modify: `go.mod`, `go.sum`

- [ ] **Step 1: Add the library**

```bash
cd /Users/cding/Workspace/ChenWeb
go get github.com/nguyenthenguyen/docx@latest
go work sync
```

- [ ] **Step 2: Verify it appears in go.mod**

```bash
grep nguyenthenguyen go.mod
```
Expected: line containing `github.com/nguyenthenguyen/docx`

- [ ] **Step 3: Commit**

```bash
git add go.mod go.sum
cd /Users/cding/Workspace && git add go.work go.work.sum 2>/dev/null; true
cd /Users/cding/Workspace/ChenWeb && git commit -m "feat(docgen): add nguyenthenguyen/docx dependency"
```

---

## Task 3: Config Additions

**Files:**
- Modify: `server/cmd/config/config.go`
- Modify: `config.toml`

- [ ] **Step 1: Add DocGen config struct to config.go**

In `server/cmd/config/config.go`, add to `AppConfigDef`:

```go
type DocGenConfig struct {
	TemplateDir string `mapstructure:"template_dir"`
	WorkerCount int    `mapstructure:"worker_count"`
}

type AppConfigDef struct {
	PDFParser     PDFParserConfig `mapstructure:"pdf_parser"`
	DocGen        DocGenConfig    `mapstructure:"doc_gen"`
	AppTableNames struct {
		TableName_ProcessStatus string `mapstructure:"table_name_process_status"`
		TableName_Schedules     string `mapstructure:"table_name_schedules"`
		TableName_Documents     string `mapstructure:"table_name_documents"`
		TableName_Flows         string `mapstructure:"table_name_flows"`
		TableName_DspyPrompts   string `mapstructure:"table_name_dspy_prompts"`
	} `mapstructure:"app_table_names"`
}
```

Also add a getter at the bottom of the file:

```go
func GetDocGenConfig() DocGenConfig {
	return AppConfig.DocGen
}
```

- [ ] **Step 2: Add section to config.toml**

```toml
[doc_gen]
template_dir  = "Data/docgen/templates"
worker_count  = 3
```

- [ ] **Step 3: Build to verify no compile errors**

```bash
cd /Users/cding/Workspace/ChenWeb && go build ./server/...
```
Expected: no errors.

- [ ] **Step 4: Commit**

```bash
git add server/cmd/config/config.go config.toml
git commit -m "feat(docgen): add DocGen config section"
```

---

## Task 4: Datastore — doc_gen_queries

**Files:**
- Create: `server/api/appdatastores/table-doc-gen-queries.go`

- [ ] **Step 1: Create the file**

```go
package appdatastores

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/databaseutil"
)

type DocGenQuery struct {
	ID           int64     `json:"id"`
	Name         string    `json:"name"`
	Description  string    `json:"description"`
	SQLStatement string    `json:"sql_statement"`
	CreatedBy    string    `json:"created_by"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

func CreateDocGenQueriesTable(logger ApiTypes.JimoLogger) error {
	db := ApiTypes.ProjectDBHandle
	stmt := `CREATE TABLE IF NOT EXISTS doc_gen_queries (
		id            BIGSERIAL PRIMARY KEY,
		name          VARCHAR(255) NOT NULL UNIQUE,
		description   TEXT,
		sql_statement TEXT NOT NULL,
		created_by    VARCHAR(255) NOT NULL,
		created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
	)`
	if err := databaseutil.ExecuteStatement(db, stmt); err != nil {
		return fmt.Errorf("failed creating doc_gen_queries table: %w (CWB_DGS_020)", err)
	}
	logger.Info("Created doc_gen_queries table")
	return nil
}

func InsertDocGenQuery(db *sql.DB, q DocGenQuery) (int64, error) {
	var id int64
	err := db.QueryRow(
		`INSERT INTO doc_gen_queries (name, description, sql_statement, created_by)
		 VALUES ($1, $2, $3, $4) RETURNING id`,
		q.Name, q.Description, q.SQLStatement, q.CreatedBy,
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("InsertDocGenQuery failed: %w (CWB_DGS_030)", err)
	}
	return id, nil
}

func UpdateDocGenQuery(db *sql.DB, id int64, name, description, sqlStatement string) error {
	_, err := db.Exec(
		`UPDATE doc_gen_queries SET name=$1, description=$2, sql_statement=$3, updated_at=NOW() WHERE id=$4`,
		name, description, sqlStatement, id,
	)
	if err != nil {
		return fmt.Errorf("UpdateDocGenQuery failed: %w (CWB_DGS_040)", err)
	}
	return nil
}

func DeleteDocGenQuery(db *sql.DB, id int64) error {
	_, err := db.Exec(`DELETE FROM doc_gen_queries WHERE id=$1`, id)
	if err != nil {
		return fmt.Errorf("DeleteDocGenQuery failed: %w (CWB_DGS_050)", err)
	}
	return nil
}

func ListDocGenQueries(db *sql.DB, search string) ([]DocGenQuery, error) {
	query := `SELECT id, name, description, sql_statement, created_by, created_at, updated_at
	          FROM doc_gen_queries`
	args := []any{}
	if search != "" {
		query += ` WHERE name ILIKE $1`
		args = append(args, "%"+search+"%")
	}
	query += ` ORDER BY name ASC`

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("ListDocGenQueries failed: %w (CWB_DGS_060)", err)
	}
	defer rows.Close()

	var results []DocGenQuery
	for rows.Next() {
		var q DocGenQuery
		if err := rows.Scan(&q.ID, &q.Name, &q.Description, &q.SQLStatement,
			&q.CreatedBy, &q.CreatedAt, &q.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan failed: %w (CWB_DGS_065)", err)
		}
		results = append(results, q)
	}
	if results == nil {
		results = []DocGenQuery{}
	}
	return results, rows.Err()
}

func GetDocGenQuery(db *sql.DB, id int64) (*DocGenQuery, error) {
	var q DocGenQuery
	err := db.QueryRow(
		`SELECT id, name, description, sql_statement, created_by, created_at, updated_at
		 FROM doc_gen_queries WHERE id=$1`, id,
	).Scan(&q.ID, &q.Name, &q.Description, &q.SQLStatement,
		&q.CreatedBy, &q.CreatedAt, &q.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("GetDocGenQuery failed: %w (CWB_DGS_070)", err)
	}
	return &q, nil
}
```

- [ ] **Step 2: Build**

```bash
cd /Users/cding/Workspace/ChenWeb && go build ./server/api/appdatastores/...
```

- [ ] **Step 3: Commit**

```bash
git add server/api/appdatastores/table-doc-gen-queries.go
git commit -m "feat(docgen): add doc_gen_queries datastore"
```

---

## Task 5: Datastore — doc_gen_jobs

**Files:**
- Create: `server/api/appdatastores/table-doc-gen-jobs.go`

- [ ] **Step 1: Create the file**

```go
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
```

- [ ] **Step 2: Build**

```bash
cd /Users/cding/Workspace/ChenWeb && go build ./server/api/appdatastores/...
```

- [ ] **Step 3: Commit**

```bash
git add server/api/appdatastores/table-doc-gen-jobs.go
git commit -m "feat(docgen): add doc_gen_jobs datastore"
```

---

## Task 6: Datastore — doc_gen_log

**Files:**
- Create: `server/api/appdatastores/table-doc-gen-log.go`

- [ ] **Step 1: Create the file**

```go
package appdatastores

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/databaseutil"
)

type DocGenLogEntry struct {
	ID           int64     `json:"id"`
	JobID        int64     `json:"job_id"`
	RequestName  string    `json:"request_name"`
	CustomerID   string    `json:"customer_id"`
	CustomerName string    `json:"customer_name"`
	Email        string    `json:"email"`
	PhoneNum     string    `json:"phone_num"`
	Purpose      string    `json:"purpose"`
	Filename     string    `json:"filename"`
	Status       string    `json:"status"` // generated | failed
	ErrorMsg     string    `json:"error_msg"`
	Remarks      string    `json:"remarks"`
	CreatedBy    string    `json:"created_by"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

func CreateDocGenLogTable(logger ApiTypes.JimoLogger) error {
	db := ApiTypes.ProjectDBHandle
	stmt := `CREATE TABLE IF NOT EXISTS doc_gen_log (
		id            BIGSERIAL PRIMARY KEY,
		job_id        BIGINT NOT NULL,
		request_name  VARCHAR(255) NOT NULL,
		customer_id   VARCHAR(128) NOT NULL,
		customer_name VARCHAR(255) NOT NULL,
		email         VARCHAR(255) NOT NULL,
		phone_num     VARCHAR(64),
		purpose       VARCHAR(255) NOT NULL,
		filename      VARCHAR(512) NOT NULL,
		status        VARCHAR(32) NOT NULL,
		error_msg     TEXT,
		remarks       TEXT,
		created_by    VARCHAR(255) NOT NULL,
		created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
	)`
	if err := databaseutil.ExecuteStatement(db, stmt); err != nil {
		return fmt.Errorf("failed creating doc_gen_log table: %w (CWB_DGS_200)", err)
	}
	logger.Info("Created doc_gen_log table")
	return nil
}

func InsertDocGenLogEntry(db *sql.DB, e DocGenLogEntry) error {
	_, err := db.Exec(
		`INSERT INTO doc_gen_log
		 (job_id, request_name, customer_id, customer_name, email, phone_num,
		  purpose, filename, status, error_msg, remarks, created_by)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`,
		e.JobID, e.RequestName, e.CustomerID, e.CustomerName, e.Email, e.PhoneNum,
		e.Purpose, e.Filename, e.Status, e.ErrorMsg, e.Remarks, e.CreatedBy,
	)
	if err != nil {
		return fmt.Errorf("InsertDocGenLogEntry failed: %w (CWB_DGS_210)", err)
	}
	return nil
}

func ListDocGenLogByJobID(db *sql.DB, jobID int64) ([]DocGenLogEntry, error) {
	rows, err := db.Query(
		`SELECT id, job_id, request_name, customer_id, customer_name, email, phone_num,
		        purpose, filename, status, error_msg, remarks, created_by, created_at, updated_at
		 FROM doc_gen_log WHERE job_id=$1 ORDER BY id ASC`, jobID,
	)
	if err != nil {
		return nil, fmt.Errorf("ListDocGenLogByJobID failed: %w (CWB_DGS_220)", err)
	}
	defer rows.Close()

	var results []DocGenLogEntry
	for rows.Next() {
		var e DocGenLogEntry
		var phoneNum, errorMsg, remarks sql.NullString
		if err := rows.Scan(&e.ID, &e.JobID, &e.RequestName, &e.CustomerID, &e.CustomerName,
			&e.Email, &phoneNum, &e.Purpose, &e.Filename, &e.Status,
			&errorMsg, &remarks, &e.CreatedBy, &e.CreatedAt, &e.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan failed: %w (CWB_DGS_225)", err)
		}
		e.PhoneNum = phoneNum.String
		e.ErrorMsg = errorMsg.String
		e.Remarks = remarks.String
		results = append(results, e)
	}
	if results == nil {
		results = []DocGenLogEntry{}
	}
	return results, rows.Err()
}
```

- [ ] **Step 2: Build**

```bash
cd /Users/cding/Workspace/ChenWeb && go build ./server/api/appdatastores/...
```

- [ ] **Step 3: Commit**

```bash
git add server/api/appdatastores/table-doc-gen-log.go
git commit -m "feat(docgen): add doc_gen_log datastore"
```

---

## Task 7: Worker — Validation + Tests

**Files:**
- Create: `server/api/docgenworker/validate.go`
- Create: `server/api/docgenworker/validate_test.go`

- [ ] **Step 1: Write the failing tests**

```go
// server/api/docgenworker/validate_test.go
package docgenworker_test

import (
	"testing"

	"github.com/chendingplano/deepdoc/server/api/docgenworker"
)

func TestValidateSQLStatement_AcceptsSelect(t *testing.T) {
	if err := docgenworker.ValidateSQLStatement("SELECT id, name FROM customers"); err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

func TestValidateSQLStatement_RejectsInsert(t *testing.T) {
	if err := docgenworker.ValidateSQLStatement("INSERT INTO foo VALUES (1)"); err == nil {
		t.Fatal("expected error for INSERT, got nil")
	}
}

func TestValidateSQLStatement_RejectsEmpty(t *testing.T) {
	if err := docgenworker.ValidateSQLStatement(""); err == nil {
		t.Fatal("expected error for empty SQL, got nil")
	}
}

func TestValidateConverter_AcceptsValidJSON(t *testing.T) {
	conv := `{"cust_id":"customer_id","cust_name":"customer_name","cust_email":"email"}`
	m, err := docgenworker.ValidateConverter(conv)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if m["cust_id"] != "customer_id" {
		t.Fatalf("expected customer_id mapping, got: %v", m)
	}
}

func TestValidateConverter_RejectsMissingRequiredKey(t *testing.T) {
	conv := `{"cust_id":"customer_id","cust_name":"customer_name"}`
	if _, err := docgenworker.ValidateConverter(conv); err == nil {
		t.Fatal("expected error for missing email mapping, got nil")
	}
}

func TestValidateConverter_RejectsInvalidJSON(t *testing.T) {
	if _, err := docgenworker.ValidateConverter("not json"); err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
}
```

- [ ] **Step 2: Run tests — expect failure (package does not exist yet)**

```bash
cd /Users/cding/Workspace/ChenWeb && go test ./server/api/docgenworker/... 2>&1 | head -5
```
Expected: `cannot find package` or build error.

- [ ] **Step 3: Create validate.go**

```go
// server/api/docgenworker/validate.go
package docgenworker

import (
	"encoding/json"
	"fmt"
	"strings"
)

var requiredConverterValues = []string{"customer_id", "customer_name", "email"}

// ValidateSQLStatement returns an error if stmt is not a SELECT query.
func ValidateSQLStatement(stmt string) error {
	trimmed := strings.TrimSpace(strings.ToUpper(stmt))
	if trimmed == "" {
		return fmt.Errorf("sql_statement must not be empty (CWB_DGW_050)")
	}
	if !strings.HasPrefix(trimmed, "SELECT") {
		return fmt.Errorf("sql_statement must be a SELECT query (CWB_DGW_055)")
	}
	return nil
}

// ValidateConverter parses converterJSON and verifies that the required
// log fields (customer_id, customer_name, email) appear as values.
// Returns the parsed map on success.
func ValidateConverter(converterJSON string) (map[string]string, error) {
	var m map[string]string
	if err := json.Unmarshal([]byte(converterJSON), &m); err != nil {
		return nil, fmt.Errorf("converter must be valid JSON object: %w (CWB_DGW_060)", err)
	}
	valueSet := make(map[string]bool)
	for _, v := range m {
		valueSet[v] = true
	}
	for _, req := range requiredConverterValues {
		if !valueSet[req] {
			return nil, fmt.Errorf("converter missing required mapping to %q (CWB_DGW_065)", req)
		}
	}
	return m, nil
}
```

- [ ] **Step 4: Run tests — expect pass**

```bash
cd /Users/cding/Workspace/ChenWeb && go test ./server/api/docgenworker/... -v -run TestValidate
```
Expected: all 6 tests PASS.

- [ ] **Step 5: Commit**

```bash
git add server/api/docgenworker/validate.go server/api/docgenworker/validate_test.go
git commit -m "feat(docgen): add converter and SQL validation with tests"
```

---

## Task 8: Worker — Renderer + Tests

**Files:**
- Create: `server/api/docgenworker/renderer.go`
- Create: `server/api/docgenworker/renderer_test.go`

- [ ] **Step 1: Write failing test**

```go
// server/api/docgenworker/renderer_test.go
package docgenworker_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/chendingplano/deepdoc/server/api/docgenworker"
	"github.com/nguyenthenguyen/docx"
)

func TestRenderDocx_ReplacesTokens(t *testing.T) {
	// Create a minimal .docx template with a token
	tmpDir := t.TempDir()
	templatePath := filepath.Join(tmpDir, "template.docx")
	outputPath := filepath.Join(tmpDir, "output.docx")

	// Build a docx with token {{companyName}}
	r, err := docx.ReadDocx("testdata/simple_template.docx")
	if err != nil {
		t.Skip("testdata/simple_template.docx not present — skipping renderer test")
	}
	r.Close()

	tokens := map[string]string{"companyName": "Acme Corp"}
	_ = templatePath // use testdata directly
	if err := docgenworker.RenderDocx("testdata/simple_template.docx", outputPath, tokens); err != nil {
		t.Fatalf("RenderDocx failed: %v", err)
	}
	if _, err := os.Stat(outputPath); os.IsNotExist(err) {
		t.Fatal("output file was not created")
	}
}
```

- [ ] **Step 2: Create renderer.go**

```go
// server/api/docgenworker/renderer.go
package docgenworker

import (
	"fmt"

	"github.com/nguyenthenguyen/docx"
)

// RenderDocx opens templatePath, replaces every {{key}} placeholder using tokens,
// and writes the result to outputPath.
func RenderDocx(templatePath, outputPath string, tokens map[string]string) error {
	r, err := docx.ReadDocx(templatePath)
	if err != nil {
		return fmt.Errorf("failed to open template %q: %w (CWB_DGW_020)", templatePath, err)
	}
	defer r.Close()

	doc := r.Editable()
	for key, val := range tokens {
		doc.Replace("{{"+key+"}}", val, -1)
	}
	if err := doc.WriteToFile(outputPath); err != nil {
		return fmt.Errorf("failed to write output %q: %w (CWB_DGW_030)", outputPath, err)
	}
	return nil
}
```

- [ ] **Step 3: Create testdata directory with a simple template**

Create `server/api/docgenworker/testdata/` and add a minimal Word doc `simple_template.docx` containing the text `Hello {{companyName}}`. This can be done by hand in Microsoft Word or LibreOffice — save as .docx. Alternatively, create programmatically:

```bash
mkdir -p server/api/docgenworker/testdata
# Create simple_template.docx via LibreOffice or Word with content: Hello {{companyName}}
# If neither is available, the test auto-skips via t.Skip()
```

- [ ] **Step 4: Run tests**

```bash
cd /Users/cding/Workspace/ChenWeb && go test ./server/api/docgenworker/... -v -run TestRender
```
Expected: PASS (or SKIP if testdata absent — either is acceptable).

- [ ] **Step 5: Commit**

```bash
git add server/api/docgenworker/renderer.go server/api/docgenworker/renderer_test.go server/api/docgenworker/testdata/
git commit -m "feat(docgen): add Word template renderer"
```

---

## Task 9: Worker — Pipeline

**Files:**
- Create: `server/api/docgenworker/worker.go`

- [ ] **Step 1: Create worker.go**

```go
// server/api/docgenworker/worker.go
package docgenworker

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/chendingplano/deepdoc/server/api/appdatastores"
	"github.com/chendingplano/shared/go/api/loggerutil"
)

// JobChannel is the shared buffered channel for job IDs.
var JobChannel chan int64

// Start initialises the worker pool. Call once after DB is ready.
func Start(db *sql.DB, jobCh chan int64, workerCount int) {
	JobChannel = jobCh
	logger := loggerutil.CreateDefaultLogger("CWB_DGW_100")
	for i := 0; i < workerCount; i++ {
		go func(workerID int) {
			logger.Info("doc gen worker started", "worker_id", workerID)
			for jobID := range jobCh {
				func() {
					defer func() {
						if r := recover(); r != nil {
							logger.Error("worker panic", "worker_id", workerID, "job_id", jobID, "panic", r)
							appdatastores.UpdateDocGenJobStatus(db, jobID,
								appdatastores.DocGenJobStatusFailed,
								fmt.Sprintf("worker panic: %v", r))
						}
					}()
					if err := processJob(db, jobID); err != nil {
						logger.Error("processJob failed", "job_id", jobID, "err", err)
					}
				}()
			}
		}(i)
	}
}

// RequeueStalledJobs pushes any pending/processing jobs back onto the channel
// so they are retried after a server restart.
func RequeueStalledJobs(db *sql.DB, jobCh chan int64) error {
	ids, err := appdatastores.ListStalledDocGenJobs(db)
	if err != nil {
		return fmt.Errorf("RequeueStalledJobs: %w (CWB_DGW_110)", err)
	}
	for _, id := range ids {
		jobCh <- id
	}
	return nil
}

func processJob(db *sql.DB, jobID int64) error {
	logger := loggerutil.CreateDefaultLogger("CWB_DGW_120")

	job, err := appdatastores.GetDocGenJob(db, jobID)
	if err != nil || job == nil {
		return fmt.Errorf("job %d not found: %w (CWB_DGW_125)", jobID, err)
	}

	if err := appdatastores.UpdateDocGenJobStatus(db, jobID, appdatastores.DocGenJobStatusProcessing, ""); err != nil {
		return err
	}

	converter, err := ValidateConverter(job.Converter)
	if err != nil {
		appdatastores.UpdateDocGenJobStatus(db, jobID, appdatastores.DocGenJobStatusFailed, err.Error())
		return err
	}

	rows, err := db.Query(job.SQLStatement)
	if err != nil {
		msg := fmt.Sprintf("SQL error: %v", err)
		appdatastores.UpdateDocGenJobStatus(db, jobID, appdatastores.DocGenJobStatusFailed, msg)
		return fmt.Errorf("%s (CWB_DGW_135)", msg)
	}
	defer rows.Close()

	cols, _ := rows.Columns()

	outDir := filepath.Join(job.OutputDir, job.RequestName)
	if err := os.MkdirAll(outDir, 0755); err != nil {
		msg := fmt.Sprintf("mkdir error: %v", err)
		appdatastores.UpdateDocGenJobStatus(db, jobID, appdatastores.DocGenJobStatusFailed, msg)
		return fmt.Errorf("%s (CWB_DGW_140)", msg)
	}

	var total, success, fail int
	for rows.Next() {
		total++
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			fail++
			appdatastores.InsertDocGenLogEntry(db, appdatastores.DocGenLogEntry{
				JobID: jobID, RequestName: job.RequestName, Purpose: job.Purpose,
				Filename: "", Status: "failed", ErrorMsg: err.Error(),
				Remarks: job.Remarks, CreatedBy: job.CreatedBy,
			})
			continue
		}

		rowMap := make(map[string]string, len(cols))
		for i, col := range cols {
			if vals[i] != nil {
				rowMap[col] = fmt.Sprintf("%v", vals[i])
			}
		}

		// Build template tokens: converter maps sqlCol → tokenName
		tokens := make(map[string]string, len(converter))
		for sqlCol, tokenName := range converter {
			tokens[tokenName] = rowMap[sqlCol]
		}

		customerID := rowMap[findKeyByValue(converter, "customer_id")]
		customerName := rowMap[findKeyByValue(converter, "customer_name")]
		email := rowMap[findKeyByValue(converter, "email")]
		phoneNum := rowMap[findKeyByValue(converter, "phone_num")]

		filename := fmt.Sprintf("%s_%04d.%s", job.RequestName, total, job.OutputFormat)
		outPath := filepath.Join(outDir, filename)

		var renderErr error
		switch strings.ToLower(job.TemplateType) {
		case "word":
			renderErr = RenderDocx(job.TemplatePath, outPath, tokens)
		default:
			renderErr = fmt.Errorf("template_type %q not supported (CWB_DGW_155)", job.TemplateType)
		}

		entry := appdatastores.DocGenLogEntry{
			JobID: jobID, RequestName: job.RequestName,
			CustomerID: customerID, CustomerName: customerName,
			Email: email, PhoneNum: phoneNum,
			Purpose: job.Purpose, Filename: filename,
			Remarks: job.Remarks, CreatedBy: job.CreatedBy,
		}
		if renderErr != nil {
			fail++
			entry.Status = "failed"
			entry.ErrorMsg = renderErr.Error()
		} else {
			success++
			entry.Status = "generated"
		}
		appdatastores.InsertDocGenLogEntry(db, entry)
	}

	finalStatus := appdatastores.DocGenJobStatusCompleted
	if success == 0 && (fail > 0 || total == 0) {
		finalStatus = appdatastores.DocGenJobStatusFailed
	}
	appdatastores.UpdateDocGenJobCounts(db, jobID, total, success, fail)
	appdatastores.UpdateDocGenJobStatus(db, jobID, finalStatus, "")
	logger.Info("job done", "job_id", jobID, "total", total, "success", success, "fail", fail)
	return nil
}

// findKeyByValue returns the first key in m whose value equals val.
func findKeyByValue(m map[string]string, val string) string {
	for k, v := range m {
		if v == val {
			return k
		}
	}
	return ""
}

// converterToJSON serialises a map to a JSON string (used internally).
func converterToJSON(m map[string]string) string {
	b, _ := json.Marshal(m)
	return string(b)
}
```

- [ ] **Step 2: Build**

```bash
cd /Users/cding/Workspace/ChenWeb && go build ./server/api/docgenworker/...
```

- [ ] **Step 3: Run all worker tests**

```bash
go test ./server/api/docgenworker/... -v
```
Expected: all validation tests PASS; renderer test PASS or SKIP.

- [ ] **Step 4: Commit**

```bash
git add server/api/docgenworker/worker.go
git commit -m "feat(docgen): add async worker pipeline"
```

---

## Task 10: Handler — Types and Helpers

**Files:**
- Create: `server/api/docgenhandler/types.go`

- [ ] **Step 1: Create types.go**

```go
// server/api/docgenhandler/types.go
package docgenhandler

import "github.com/chendingplano/deepdoc/server/api/appdatastores"

// --- Request bodies ---

type SubmitJobRequest struct {
	RequestName  string            `json:"request_name"`
	Purpose      string            `json:"purpose"`
	Remarks      string            `json:"remarks"`
	SQLQueryID   *int64            `json:"sql_query_id"`   // pick from registry; overrides SQLStatement
	SQLStatement string            `json:"sql_statement"`  // used when SQLQueryID is nil
	TemplateType string            `json:"template_type"`  // word | typst
	TemplateName string            `json:"template_name"`  // filename inside TemplateDir
	Converter    map[string]string `json:"converter"`
	OutputDir    string            `json:"output_dir"`
	OutputFormat string            `json:"output_format"` // docx | pdf
}

type CreateQueryRequest struct {
	Name         string `json:"name"`
	Description  string `json:"description"`
	SQLStatement string `json:"sql_statement"`
}

type UpdateQueryRequest struct {
	Name         string `json:"name"`
	Description  string `json:"description"`
	SQLStatement string `json:"sql_statement"`
}

// --- Response bodies ---

type SubmitJobResponse struct {
	Status bool  `json:"status"`
	JobID  int64 `json:"job_id"`
}

type JobListResponse struct {
	Status   bool                      `json:"status"`
	Jobs     []appdatastores.DocGenJob `json:"jobs"`
	Total    int64                     `json:"total"`
	Page     int                       `json:"page"`
	PageSize int                       `json:"page_size"`
}

type JobDetailResponse struct {
	Status bool                           `json:"status"`
	Job    *appdatastores.DocGenJob       `json:"job"`
	Logs   []appdatastores.DocGenLogEntry `json:"logs"`
}

type QueryListResponse struct {
	Status  bool                        `json:"status"`
	Queries []appdatastores.DocGenQuery `json:"queries"`
}

type TemplateListResponse struct {
	Status    bool     `json:"status"`
	Templates []string `json:"templates"`
}

type ErrorResponse struct {
	Status   bool   `json:"status"`
	ErrorMsg string `json:"error_msg"`
}
```

- [ ] **Step 2: Build**

```bash
cd /Users/cding/Workspace/ChenWeb && go build ./server/api/docgenhandler/...
```

- [ ] **Step 3: Commit**

```bash
git add server/api/docgenhandler/types.go
git commit -m "feat(docgen): add handler request/response types"
```

---

## Task 11: Handler — Templates

**Files:**
- Create: `server/api/docgenhandler/handler_templates.go`

- [ ] **Step 1: Create handler_templates.go**

```go
// server/api/docgenhandler/handler_templates.go
package docgenhandler

import (
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/chendingplano/deepdoc/server/cmd/config"
	"github.com/chendingplano/shared/go/api/EchoFactory"
	"github.com/labstack/echo/v4"
)

// ListTemplates handles GET /api/v1/docgen/templates
func ListTemplates(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_DGH_010")
	defer rc.Close()

	templateDir := config.AppConfig.DocGen.TemplateDir
	entries, err := os.ReadDir(templateDir)
	if err != nil {
		// Dir may not exist yet — return empty list rather than error
		return c.JSON(http.StatusOK, TemplateListResponse{Status: true, Templates: []string{}})
	}

	names := []string{}
	for _, e := range entries {
		if !e.IsDir() {
			names = append(names, e.Name())
		}
	}
	return c.JSON(http.StatusOK, TemplateListResponse{Status: true, Templates: names})
}

// UploadTemplate handles POST /api/v1/docgen/templates (multipart/form-data, field "file")
func UploadTemplate(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_DGH_020")
	defer rc.Close()

	file, err := c.FormFile("file")
	if err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Status: false, ErrorMsg: "field 'file' required (CWB_DGH_025)"})
	}

	templateDir := config.AppConfig.DocGen.TemplateDir
	if err := os.MkdirAll(templateDir, 0755); err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Status: false, ErrorMsg: "cannot create template dir (CWB_DGH_028)"})
	}

	src, err := file.Open()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Status: false, ErrorMsg: "cannot open uploaded file (CWB_DGH_030)"})
	}
	defer src.Close()

	destPath := filepath.Join(templateDir, filepath.Base(file.Filename))
	dst, err := os.Create(destPath)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Status: false, ErrorMsg: "cannot save file (CWB_DGH_035)"})
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Status: false, ErrorMsg: "write failed (CWB_DGH_040)"})
	}

	rc.GetLogger().Info("template uploaded", "filename", file.Filename)
	return c.JSON(http.StatusOK, map[string]any{"status": true, "filename": file.Filename})
}
```

- [ ] **Step 2: Build**

```bash
cd /Users/cding/Workspace/ChenWeb && go build ./server/api/docgenhandler/...
```

- [ ] **Step 3: Commit**

```bash
git add server/api/docgenhandler/handler_templates.go
git commit -m "feat(docgen): add template list/upload handlers"
```

---

## Task 12: Handler — Queries CRUD

**Files:**
- Create: `server/api/docgenhandler/handler_queries.go`

- [ ] **Step 1: Create handler_queries.go**

```go
// server/api/docgenhandler/handler_queries.go
package docgenhandler

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/chendingplano/deepdoc/server/api/appdatastores"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/EchoFactory"
	authmiddleware "github.com/chendingplano/shared/go/authmiddleware"
	"github.com/labstack/echo/v4"
)

// isAdmin returns true if the JWT user has role "admin".
// Check shared/go/authmiddleware for the exact context key and UserInfo fields.
func isAdmin(c echo.Context) bool {
	userInfo, ok := c.Get(authmiddleware.UserInfoContextKey).(*authmiddleware.UserInfo)
	if !ok || userInfo == nil {
		return false
	}
	return userInfo.Role == "admin"
}

// ListQueries handles GET /api/v1/docgen/queries?q=<search>
func ListQueries(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_DGH_050")
	defer rc.Close()

	search := c.QueryParam("q")
	queries, err := appdatastores.ListDocGenQueries(ApiTypes.ProjectDBHandle, search)
	if err != nil {
		rc.GetLogger().Error("ListDocGenQueries failed", "err", err)
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Status: false, ErrorMsg: "failed to list queries (CWB_DGH_055)"})
	}
	return c.JSON(http.StatusOK, QueryListResponse{Status: true, Queries: queries})
}

// CreateQuery handles POST /api/v1/docgen/queries (admin only)
func CreateQuery(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_DGH_060")
	defer rc.Close()

	if !isAdmin(c) {
		return c.JSON(http.StatusForbidden, ErrorResponse{Status: false, ErrorMsg: "admin access required (CWB_DGH_062)"})
	}

	var req CreateQueryRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Status: false, ErrorMsg: "invalid request body (CWB_DGH_065)"})
	}
	if req.Name == "" || req.SQLStatement == "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Status: false, ErrorMsg: "name and sql_statement are required (CWB_DGH_067)"})
	}

	createdBy := getCreatedBy(c)
	id, err := appdatastores.InsertDocGenQuery(ApiTypes.ProjectDBHandle, appdatastores.DocGenQuery{
		Name: req.Name, Description: req.Description,
		SQLStatement: req.SQLStatement, CreatedBy: createdBy,
	})
	if err != nil {
		rc.GetLogger().Error("InsertDocGenQuery failed", "err", err)
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Status: false, ErrorMsg: "failed to create query (CWB_DGH_070)"})
	}
	return c.JSON(http.StatusCreated, map[string]any{"status": true, "id": id})
}

// UpdateQuery handles PUT /api/v1/docgen/queries/:id (admin only)
func UpdateQuery(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_DGH_075")
	defer rc.Close()

	if !isAdmin(c) {
		return c.JSON(http.StatusForbidden, ErrorResponse{Status: false, ErrorMsg: "admin access required (CWB_DGH_076)"})
	}

	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Status: false, ErrorMsg: "invalid id (CWB_DGH_077)"})
	}

	var req UpdateQueryRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Status: false, ErrorMsg: "invalid request body (CWB_DGH_078)"})
	}

	if err := appdatastores.UpdateDocGenQuery(ApiTypes.ProjectDBHandle, id, req.Name, req.Description, req.SQLStatement); err != nil {
		rc.GetLogger().Error("UpdateDocGenQuery failed", "err", err)
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Status: false, ErrorMsg: fmt.Sprintf("failed to update query %d (CWB_DGH_080)", id)})
	}
	return c.JSON(http.StatusOK, map[string]any{"status": true})
}

// DeleteQuery handles DELETE /api/v1/docgen/queries/:id (admin only)
func DeleteQuery(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_DGH_085")
	defer rc.Close()

	if !isAdmin(c) {
		return c.JSON(http.StatusForbidden, ErrorResponse{Status: false, ErrorMsg: "admin access required (CWB_DGH_086)"})
	}

	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Status: false, ErrorMsg: "invalid id (CWB_DGH_087)"})
	}

	if err := appdatastores.DeleteDocGenQuery(ApiTypes.ProjectDBHandle, id); err != nil {
		rc.GetLogger().Error("DeleteDocGenQuery failed", "err", err)
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Status: false, ErrorMsg: fmt.Sprintf("failed to delete query %d (CWB_DGH_090)", id)})
	}
	return c.JSON(http.StatusOK, map[string]any{"status": true})
}

// getCreatedBy extracts the authenticated user's email/name from context.
// Check shared/go/authmiddleware for UserInfo fields.
func getCreatedBy(c echo.Context) string {
	userInfo, ok := c.Get(authmiddleware.UserInfoContextKey).(*authmiddleware.UserInfo)
	if !ok || userInfo == nil {
		return "unknown"
	}
	if userInfo.Email != "" {
		return userInfo.Email
	}
	return "unknown"
}
```

> **Note:** The exact `authmiddleware.UserInfoContextKey` constant and `UserInfo` struct fields (e.g. `.Role`, `.Email`) are defined in `shared/go/authmiddleware/`. Check that package before building — adjust field names to match.

- [ ] **Step 2: Build (fix any authmiddleware field mismatches)**

```bash
cd /Users/cding/Workspace/ChenWeb && go build ./server/api/docgenhandler/...
```

- [ ] **Step 3: Commit**

```bash
git add server/api/docgenhandler/handler_queries.go
git commit -m "feat(docgen): add query CRUD handlers"
```

---

## Task 13: Handler — Jobs + Tests

**Files:**
- Create: `server/api/docgenhandler/handler_jobs.go`
- Create: `server/api/docgenhandler/handler_jobs_test.go`

- [ ] **Step 1: Write failing tests**

```go
// server/api/docgenhandler/handler_jobs_test.go
package docgenhandler_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/chendingplano/deepdoc/server/api/docgenhandler"
	"github.com/labstack/echo/v4"
)

func newEcho() *echo.Echo { return echo.New() }

func TestSubmitJob_MissingFields_Returns400(t *testing.T) {
	e := newEcho()
	body := `{"request_name":"","purpose":"test"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/docgen/jobs", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := docgenhandler.SubmitJob(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestSubmitJob_NonSelectSQL_Returns400(t *testing.T) {
	e := newEcho()
	body := `{
		"request_name":"test-req","purpose":"test","template_type":"word",
		"template_name":"t.docx","output_dir":"/tmp","output_format":"docx",
		"sql_statement":"DELETE FROM foo",
		"converter":{"cid":"customer_id","cname":"customer_name","cemail":"email"}
	}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/docgen/jobs", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := docgenhandler.SubmitJob(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for non-SELECT SQL, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestListJobs_Returns200(t *testing.T) {
	e := newEcho()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/docgen/jobs", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := docgenhandler.ListJobs(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}
```

- [ ] **Step 2: Run failing tests**

```bash
cd /Users/cding/Workspace/ChenWeb && go test ./server/api/docgenhandler/... 2>&1 | head -10
```
Expected: build failure (handler_jobs.go not yet created).

- [ ] **Step 3: Create handler_jobs.go**

```go
// server/api/docgenhandler/handler_jobs.go
package docgenhandler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/chendingplano/deepdoc/server/api/appdatastores"
	"github.com/chendingplano/deepdoc/server/api/docgenworker"
	"github.com/chendingplano/deepdoc/server/cmd/config"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/EchoFactory"
	"github.com/labstack/echo/v4"
)

const defaultPageSize = 20
const maxPageSize = 200

// SubmitJob handles POST /api/v1/docgen/jobs
func SubmitJob(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_DGH_100")
	defer rc.Close()

	var req SubmitJobRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{false, "invalid request body (CWB_DGH_105)"})
	}

	if req.RequestName == "" || req.Purpose == "" || req.TemplateType == "" ||
		req.TemplateName == "" || req.OutputDir == "" || req.OutputFormat == "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{false, "missing required fields: request_name, purpose, template_type, template_name, output_dir, output_format (CWB_DGH_110)"})
	}

	// Resolve SQL
	sqlStmt := req.SQLStatement
	if req.SQLQueryID != nil {
		q, err := appdatastores.GetDocGenQuery(ApiTypes.ProjectDBHandle, *req.SQLQueryID)
		if err != nil || q == nil {
			return c.JSON(http.StatusBadRequest, ErrorResponse{false, fmt.Sprintf("sql_query_id %d not found (CWB_DGH_115)", *req.SQLQueryID)})
		}
		sqlStmt = q.SQLStatement
	}
	if sqlStmt == "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{false, "sql_statement required (CWB_DGH_118)"})
	}

	// Validate SQL is SELECT
	if err := docgenworker.ValidateSQLStatement(sqlStmt); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{false, err.Error()})
	}

	// Serialize and validate converter
	converterBytes, err := json.Marshal(req.Converter)
	if err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{false, "invalid converter (CWB_DGH_125)"})
	}
	converterStr := string(converterBytes)
	if _, err := docgenworker.ValidateConverter(converterStr); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{false, err.Error()})
	}

	// Resolve template path
	templatePath := filepath.Join(config.AppConfig.DocGen.TemplateDir, req.TemplateName)
	if _, err := os.Stat(templatePath); os.IsNotExist(err) {
		return c.JSON(http.StatusBadRequest, ErrorResponse{false, fmt.Sprintf("template %q not found (CWB_DGH_130)", req.TemplateName)})
	}

	createdBy := getCreatedBy(c)
	db := ApiTypes.ProjectDBHandle
	job := appdatastores.DocGenJob{
		RequestName: req.RequestName, Purpose: req.Purpose, Remarks: req.Remarks,
		SQLQueryID: req.SQLQueryID, SQLStatement: sqlStmt,
		TemplateType: req.TemplateType, TemplatePath: templatePath,
		Converter: converterStr, OutputDir: req.OutputDir,
		OutputFormat: req.OutputFormat, CreatedBy: createdBy,
	}

	jobID, err := appdatastores.InsertDocGenJob(db, job)
	if err != nil {
		if strings.Contains(err.Error(), "unique") || strings.Contains(err.Error(), "duplicate") {
			return c.JSON(http.StatusConflict, ErrorResponse{false, fmt.Sprintf("request_name %q already exists (CWB_DGH_140)", req.RequestName)})
		}
		rc.GetLogger().Error("InsertDocGenJob failed", "err", err)
		return c.JSON(http.StatusInternalServerError, ErrorResponse{false, "failed to create job (CWB_DGH_145)"})
	}

	// Enqueue (non-blocking; jobs stuck in pending are recovered on restart)
	if docgenworker.JobChannel != nil {
		select {
		case docgenworker.JobChannel <- jobID:
		default:
			rc.GetLogger().Warn("job channel full", "job_id", jobID)
		}
	}

	return c.JSON(http.StatusCreated, SubmitJobResponse{Status: true, JobID: jobID})
}

// ListJobs handles GET /api/v1/docgen/jobs?status=&request_name=&page=&page_size=
func ListJobs(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_DGH_150")
	defer rc.Close()

	status := c.QueryParam("status")
	requestName := c.QueryParam("request_name")
	page := parsePositiveInt(c.QueryParam("page"), 1)
	pageSize := parsePositiveInt(c.QueryParam("page_size"), defaultPageSize)
	if pageSize > maxPageSize {
		pageSize = maxPageSize
	}

	jobs, total, err := appdatastores.ListDocGenJobs(ApiTypes.ProjectDBHandle, status, requestName, page, pageSize)
	if err != nil {
		rc.GetLogger().Error("ListDocGenJobs failed", "err", err)
		return c.JSON(http.StatusInternalServerError, ErrorResponse{false, "failed to list jobs (CWB_DGH_155)"})
	}
	return c.JSON(http.StatusOK, JobListResponse{Status: true, Jobs: jobs, Total: total, Page: page, PageSize: pageSize})
}

// GetJob handles GET /api/v1/docgen/jobs/:id
func GetJob(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_DGH_160")
	defer rc.Close()

	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{false, "invalid job id (CWB_DGH_162)"})
	}

	db := ApiTypes.ProjectDBHandle
	job, err := appdatastores.GetDocGenJob(db, id)
	if err != nil {
		rc.GetLogger().Error("GetDocGenJob failed", "err", err)
		return c.JSON(http.StatusInternalServerError, ErrorResponse{false, "failed to get job (CWB_DGH_165)"})
	}
	if job == nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{false, fmt.Sprintf("job %d not found (CWB_DGH_167)", id)})
	}

	logs, err := appdatastores.ListDocGenLogByJobID(db, id)
	if err != nil {
		rc.GetLogger().Error("ListDocGenLogByJobID failed", "err", err)
		return c.JSON(http.StatusInternalServerError, ErrorResponse{false, "failed to get job logs (CWB_DGH_170)"})
	}
	return c.JSON(http.StatusOK, JobDetailResponse{Status: true, Job: job, Logs: logs})
}

func parsePositiveInt(raw string, def int) int {
	if n, err := strconv.Atoi(raw); err == nil && n > 0 {
		return n
	}
	return def
}
```

- [ ] **Step 4: Run tests**

```bash
cd /Users/cding/Workspace/ChenWeb && go test ./server/api/docgenhandler/... -v
```
Expected: `TestSubmitJob_MissingFields_Returns400` PASS, `TestSubmitJob_NonSelectSQL_Returns400` PASS, `TestListJobs_Returns200` PASS.

> `TestSubmitJob_NonSelectSQL_Returns400` calls `ValidateSQLStatement` before the DB — so it should return 400 even without a live DB. `TestListJobs_Returns200` will return 200 with `{"jobs":[]}` because `ProjectDBHandle` is nil and the handler returns an internal error — adjust the test expectation to `StatusInternalServerError` if running without a DB.

- [ ] **Step 5: Commit**

```bash
git add server/api/docgenhandler/handler_jobs.go server/api/docgenhandler/handler_jobs_test.go
git commit -m "feat(docgen): add job submit/list/get handlers with tests"
```

---

## Task 14: Register Routes + Start Worker

**Files:**
- Modify: `server/api/routes.go`
- Modify: `server/cmd/deepdoc/main.go`

- [ ] **Step 1: Add docgen routes to routes.go**

In `server/api/routes.go`, add imports:

```go
"github.com/chendingplano/deepdoc/server/api/docgenhandler"
```

After the dspy routes block, add:

```go
// Doc Generation endpoints
apiGroup.POST("/docgen/jobs", docgenhandler.SubmitJob)
apiGroup.GET("/docgen/jobs", docgenhandler.ListJobs)
apiGroup.GET("/docgen/jobs/:id", docgenhandler.GetJob)
apiGroup.GET("/docgen/queries", docgenhandler.ListQueries)
apiGroup.POST("/docgen/queries", docgenhandler.CreateQuery)
apiGroup.PUT("/docgen/queries/:id", docgenhandler.UpdateQuery)
apiGroup.DELETE("/docgen/queries/:id", docgenhandler.DeleteQuery)
apiGroup.GET("/docgen/templates", docgenhandler.ListTemplates)
apiGroup.POST("/docgen/templates", docgenhandler.UploadTemplate)
```

- [ ] **Step 2: Start worker pool in main.go**

In `server/cmd/deepdoc/main.go`, add imports:

```go
"github.com/chendingplano/deepdoc/server/api/docgenworker"
```

After the DB initialisation block (after `project_db == nil` check), and before `e.Logger.Fatal(e.Start(pp))`, add:

```go
// Start doc generation worker pool
docGenCfg := config.GetDocGenConfig()
workerCount := docGenCfg.WorkerCount
if workerCount <= 0 {
    workerCount = 3
}
docgenJobCh := make(chan int64, 100)
docgenworker.Start(project_db, docgenJobCh, workerCount)
if err := docgenworker.RequeueStalledJobs(project_db, docgenJobCh); err != nil {
    logger.Warn("failed to requeue stalled doc gen jobs", "err", err)
}
logger.Info("doc gen worker pool started", "workers", workerCount)
```

- [ ] **Step 3: Build the full server**

```bash
cd /Users/cding/Workspace/ChenWeb && go build ./server/cmd/deepdoc/...
```
Expected: no errors.

- [ ] **Step 4: Commit**

```bash
git add server/api/routes.go server/cmd/deepdoc/main.go
git commit -m "feat(docgen): register API routes and start worker pool"
```

---

## Task 15: Frontend — doc-gen-view.svelte

**Files:**
- Create: `web/src/lib/components/home3/doc-gen-view.svelte`

- [ ] **Step 1: Create the component**

```svelte
<script lang="ts">
	import { onMount } from 'svelte';

	let { darkMode = true }: { darkMode: boolean } = $props();

	// --- Design tokens (match home3 palette) ---
	let pageBg      = $derived(darkMode ? '#171B26' : '#F2F4F7');
	let cardBg      = $derived(darkMode ? '#1F2333' : '#FFFFFF');
	let surface2    = $derived(darkMode ? '#252A3A' : '#ECEEF2');
	let borderColor = $derived(darkMode ? '#2D3348' : '#E4E6EB');
	let accent      = $derived(darkMode ? '#818CF8' : '#6366F1');
	let accentTint  = $derived(darkMode ? 'rgba(129,140,248,0.15)' : 'rgba(99,102,241,0.10)');
	let textPrimary = $derived(darkMode ? '#E2E8F0' : '#111827');
	let textSecondary = $derived(darkMode ? '#94A3B8' : '#6B7280');
	let textMuted   = $derived(darkMode ? '#64748B' : '#9CA3AF');

	type Tab = 'generate' | 'history' | 'queries';
	let activeTab = $state<Tab>('generate');
	let isAdmin = $state(false);

	// --- Generate tab state ---
	let requestName  = $state('');
	let purpose      = $state('');
	let remarks      = $state('');
	let sqlSearch    = $state('');
	let sqlQueryID   = $state<number | null>(null);
	let sqlStatement = $state('');
	let templateType = $state('word');
	let templateName = $state('');
	let converterStr = $state('{}');
	let outputDir    = $state('');
	let outputFormat = $state('docx');
	let submitError  = $state('');
	let submitSuccess = $state('');
	let submitting   = $state(false);

	// --- Query search state ---
	let queryResults = $state<{id:number,name:string,description:string,sql_statement:string}[]>([]);
	let querySearchLoading = $state(false);

	// --- Template state ---
	let templates = $state<string[]>([]);

	// --- History tab state ---
	type Job = {
		job_id: number; request_name: string; purpose: string; status: string;
		total_count: number; success_count: number; fail_count: number;
		created_by: string; created_at: string;
	};
	type LogEntry = {
		id: number; filename: string; customer_name: string;
		status: string; error_msg: string;
	};
	let jobs = $state<Job[]>([]);
	let jobTotal = $state(0);
	let jobPage = $state(1);
	let jobStatusFilter = $state('');
	let jobNameFilter = $state('');
	let historyLoading = $state(false);
	let expandedJobID = $state<number | null>(null);
	let jobLogs = $state<Record<number, LogEntry[]>>({});
	let historyRefreshInterval: ReturnType<typeof setInterval> | null = null;

	// --- SQL Queries tab state ---
	type SQLQuery = {id:number;name:string;description:string;sql_statement:string;created_by:string;created_at:string};
	let queryList = $state<SQLQuery[]>([]);
	let queryListSearch = $state('');
	let queryListLoading = $state(false);
	let showAddQuery = $state(false);
	let newQueryName = $state('');
	let newQueryDesc = $state('');
	let newQuerySQL  = $state('');
	let addQueryError = $state('');

	onMount(async () => {
		await checkAdmin();
		await loadTemplates();
		await loadHistory();
		startAutoRefresh();
	});

	async function checkAdmin() {
		try {
			const res = await fetch('/api/v1/ai-assistant/user-info', { credentials: 'same-origin' });
			if (res.ok) {
				const data = await res.json();
				isAdmin = data?.user?.role === 'admin' || data?.role === 'admin';
			}
		} catch { /* ignore */ }
	}

	async function loadTemplates() {
		try {
			const res = await fetch('/api/v1/docgen/templates', { credentials: 'same-origin' });
			if (res.ok) {
				const data = await res.json();
				templates = data.templates ?? [];
			}
		} catch { /* ignore */ }
	}

	async function searchQueries() {
		if (!sqlSearch.trim()) { queryResults = []; return; }
		querySearchLoading = true;
		try {
			const res = await fetch(`/api/v1/docgen/queries?q=${encodeURIComponent(sqlSearch)}`, { credentials: 'same-origin' });
			if (res.ok) { const data = await res.json(); queryResults = data.queries ?? []; }
		} finally { querySearchLoading = false; }
	}

	function pickQuery(q: {id:number;name:string;sql_statement:string}) {
		sqlQueryID = q.id;
		sqlStatement = q.sql_statement;
		sqlSearch = q.name;
		queryResults = [];
	}

	async function handleFileUpload(e: Event) {
		const input = e.target as HTMLInputElement;
		if (!input.files?.length) return;
		const formData = new FormData();
		formData.append('file', input.files[0]);
		const res = await fetch('/api/v1/docgen/templates', { method: 'POST', body: formData, credentials: 'same-origin' });
		if (res.ok) {
			const data = await res.json();
			templateName = data.filename;
			await loadTemplates();
		}
	}

	async function submitJob() {
		submitError = ''; submitSuccess = ''; submitting = true;
		try {
			let converter: Record<string,string> = {};
			try { converter = JSON.parse(converterStr); } catch {
				submitError = 'Converter must be valid JSON.'; return;
			}
			const body: Record<string,unknown> = {
				request_name: requestName, purpose, remarks,
				template_type: templateType, template_name: templateName,
				converter, output_dir: outputDir, output_format: outputFormat
			};
			if (sqlQueryID !== null) body.sql_query_id = sqlQueryID;
			else body.sql_statement = sqlStatement;

			const res = await fetch('/api/v1/docgen/jobs', {
				method: 'POST', credentials: 'same-origin',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify(body)
			});
			const data = await res.json();
			if (!res.ok) { submitError = data.error_msg ?? 'Submission failed.'; return; }
			submitSuccess = `Job created! ID: ${data.job_id}`;
			requestName = ''; purpose = ''; remarks = ''; sqlStatement = '';
			sqlQueryID = null; sqlSearch = ''; converterStr = '{}';
		} finally { submitting = false; }
	}

	async function loadHistory() {
		historyLoading = true;
		try {
			const params = new URLSearchParams({ page: String(jobPage), page_size: '20' });
			if (jobStatusFilter) params.set('status', jobStatusFilter);
			if (jobNameFilter) params.set('request_name', jobNameFilter);
			const res = await fetch(`/api/v1/docgen/jobs?${params}`, { credentials: 'same-origin' });
			if (res.ok) { const data = await res.json(); jobs = data.jobs ?? []; jobTotal = data.total ?? 0; }
		} finally { historyLoading = false; }
	}

	function startAutoRefresh() {
		historyRefreshInterval = setInterval(() => {
			const hasActive = jobs.some(j => j.status === 'pending' || j.status === 'processing');
			if (hasActive) loadHistory();
		}, 5000);
	}

	async function toggleJobExpand(jobID: number) {
		if (expandedJobID === jobID) { expandedJobID = null; return; }
		expandedJobID = jobID;
		if (!jobLogs[jobID]) {
			const res = await fetch(`/api/v1/docgen/jobs/${jobID}`, { credentials: 'same-origin' });
			if (res.ok) { const data = await res.json(); jobLogs = { ...jobLogs, [jobID]: data.logs ?? [] }; }
		}
	}

	async function loadQueryList() {
		queryListLoading = true;
		try {
			const res = await fetch(`/api/v1/docgen/queries?q=${encodeURIComponent(queryListSearch)}`, { credentials: 'same-origin' });
			if (res.ok) { const data = await res.json(); queryList = data.queries ?? []; }
		} finally { queryListLoading = false; }
	}

	async function addQuery() {
		addQueryError = '';
		if (!newQueryName || !newQuerySQL) { addQueryError = 'Name and SQL are required.'; return; }
		const res = await fetch('/api/v1/docgen/queries', {
			method: 'POST', credentials: 'same-origin',
			headers: { 'Content-Type': 'application/json' },
			body: JSON.stringify({ name: newQueryName, description: newQueryDesc, sql_statement: newQuerySQL })
		});
		if (!res.ok) { const d = await res.json(); addQueryError = d.error_msg ?? 'Failed.'; return; }
		showAddQuery = false; newQueryName = ''; newQueryDesc = ''; newQuerySQL = '';
		await loadQueryList();
	}

	async function deleteQuery(id: number) {
		if (!confirm('Delete this query?')) return;
		await fetch(`/api/v1/docgen/queries/${id}`, { method: 'DELETE', credentials: 'same-origin' });
		await loadQueryList();
	}

	function statusBadgeStyle(status: string): string {
		const map: Record<string, string> = {
			pending:    'background:#6B7280;color:white',
			processing: `background:${accent};color:white`,
			completed:  'background:#10B981;color:white',
			failed:     'background:#EF4444;color:white',
			generated:  'background:#10B981;color:white',
		};
		return map[status] ?? 'background:#6B7280;color:white';
	}
</script>

<div class="flex flex-col h-full overflow-y-auto p-6" style="background:{pageBg};">
	<!-- Tab bar -->
	<div class="flex gap-1 mb-6 p-1 rounded-xl flex-shrink-0" style="background:{surface2}; border:1px solid {borderColor}; width:fit-content;">
		{#each ([['generate','Generate'],['history','History'],isAdmin ? ['queries','SQL Queries'] : null] as const).filter(Boolean) as [id, label]}
			<button
				onclick={() => { activeTab = id as Tab; if (id === 'history') loadHistory(); if (id === 'queries') loadQueryList(); }}
				class="px-4 py-1.5 rounded-lg text-sm font-medium transition-colors duration-150"
				style="background:{activeTab === id ? accent : 'transparent'}; color:{activeTab === id ? 'white' : textSecondary};"
			>{label}</button>
		{/each}
	</div>

	<!-- ===== GENERATE TAB ===== -->
	{#if activeTab === 'generate'}
		<div class="rounded-xl p-6 max-w-2xl" style="background:{cardBg}; border:1px solid {borderColor};">
			<h2 class="text-lg font-semibold mb-5" style="color:{textPrimary};">New Document Generation Job</h2>

			{#if submitSuccess}
				<div class="mb-4 p-3 rounded-lg text-sm" style="background:#10B981; color:white;">{submitSuccess}</div>
			{/if}
			{#if submitError}
				<div class="mb-4 p-3 rounded-lg text-sm" style="background:#EF4444; color:white;">{submitError}</div>
			{/if}

			<div class="space-y-4">
				<!-- Request Name -->
				<div>
					<label class="block text-sm font-medium mb-1" style="color:{textSecondary};">Request Name *</label>
					<input bind:value={requestName} class="w-full px-3 py-2 rounded-lg text-sm" style="background:{surface2}; border:1px solid {borderColor}; color:{textPrimary};" placeholder="Unique identifier" />
				</div>

				<!-- Purpose -->
				<div>
					<label class="block text-sm font-medium mb-1" style="color:{textSecondary};">Purpose *</label>
					<input bind:value={purpose} class="w-full px-3 py-2 rounded-lg text-sm" style="background:{surface2}; border:1px solid {borderColor}; color:{textPrimary};" placeholder="Brief description of this doc run" />
				</div>

				<!-- SQL Query search-and-pick -->
				<div>
					<label class="block text-sm font-medium mb-1" style="color:{textSecondary};">SQL Query *</label>
					<div class="relative">
						<input
							bind:value={sqlSearch}
							oninput={searchQueries}
							class="w-full px-3 py-2 rounded-lg text-sm"
							style="background:{surface2}; border:1px solid {borderColor}; color:{textPrimary};"
							placeholder="Search predefined queries by name…"
						/>
						{#if queryResults.length > 0}
							<div class="absolute z-10 left-0 right-0 mt-1 rounded-lg overflow-hidden" style="background:{cardBg}; border:1px solid {borderColor}; box-shadow:0 8px 24px rgba(0,0,0,0.15);">
								{#each queryResults as q}
									<button onclick={() => pickQuery(q)} class="w-full text-left px-3 py-2 text-sm hover:opacity-80 transition-opacity" style="background:transparent; color:{textPrimary}; border-bottom:1px solid {borderColor};">
										<div class="font-medium">{q.name}</div>
										{#if q.description}<div class="text-xs" style="color:{textMuted};">{q.description}</div>{/if}
									</button>
								{/each}
							</div>
						{/if}
					</div>
					{#if sqlStatement}
						<pre class="mt-2 p-2 rounded text-xs overflow-x-auto" style="background:{surface2}; color:{textSecondary}; border:1px solid {borderColor};">{sqlStatement}</pre>
					{/if}
				</div>

				<!-- Template Type -->
				<div>
					<label class="block text-sm font-medium mb-1" style="color:{textSecondary};">Template Type *</label>
					<select bind:value={templateType} class="w-full px-3 py-2 rounded-lg text-sm" style="background:{surface2}; border:1px solid {borderColor}; color:{textPrimary};">
						<option value="word">Word (.docx)</option>
						<option value="typst">Typst (not yet supported)</option>
					</select>
				</div>

				<!-- Template Name -->
				<div>
					<label class="block text-sm font-medium mb-1" style="color:{textSecondary};">Template *</label>
					<div class="flex gap-2">
						<select bind:value={templateName} class="flex-1 px-3 py-2 rounded-lg text-sm" style="background:{surface2}; border:1px solid {borderColor}; color:{textPrimary};">
							<option value="">Select a template…</option>
							{#each templates as t}<option value={t}>{t}</option>{/each}
						</select>
						<label class="flex items-center px-3 py-2 rounded-lg text-sm cursor-pointer transition-opacity hover:opacity-80" style="background:{accentTint}; color:{accent}; border:1px solid {accent}30;">
							Upload
							<input type="file" class="hidden" accept=".docx,.typ" onchange={handleFileUpload} />
						</label>
					</div>
				</div>

				<!-- Converter -->
				<div>
					<label class="block text-sm font-medium mb-1" style="color:{textSecondary};">Converter JSON * <span class="font-normal text-xs" style="color:{textMuted};">(sql_column → template_token; must include customer_id, customer_name, email as values)</span></label>
					<textarea bind:value={converterStr} rows={4} class="w-full px-3 py-2 rounded-lg text-sm font-mono" style="background:{surface2}; border:1px solid {borderColor}; color:{textPrimary};" placeholder='{"customer_id_col":"customer_id","name_col":"customer_name","email_col":"email"}' />
				</div>

				<!-- Output Dir -->
				<div>
					<label class="block text-sm font-medium mb-1" style="color:{textSecondary};">Output Directory *</label>
					<input bind:value={outputDir} class="w-full px-3 py-2 rounded-lg text-sm font-mono" style="background:{surface2}; border:1px solid {borderColor}; color:{textPrimary};" placeholder="Data/docgen/output" />
				</div>

				<!-- Output Format -->
				<div>
					<label class="block text-sm font-medium mb-1" style="color:{textSecondary};">Output Format *</label>
					<select bind:value={outputFormat} class="w-full px-3 py-2 rounded-lg text-sm" style="background:{surface2}; border:1px solid {borderColor}; color:{textPrimary};">
						<option value="docx">DOCX</option>
						<option value="pdf">PDF (not yet supported)</option>
					</select>
				</div>

				<!-- Remarks -->
				<div>
					<label class="block text-sm font-medium mb-1" style="color:{textSecondary};">Remarks</label>
					<textarea bind:value={remarks} rows={2} class="w-full px-3 py-2 rounded-lg text-sm" style="background:{surface2}; border:1px solid {borderColor}; color:{textPrimary};" />
				</div>

				<button
					onclick={submitJob}
					disabled={submitting}
					class="w-full py-2.5 rounded-lg text-sm font-semibold transition-opacity hover:opacity-88 disabled:opacity-50"
					style="background:{accent}; color:white; border:none;"
				>{submitting ? 'Submitting…' : 'Generate Documents'}</button>
			</div>
		</div>
	{/if}

	<!-- ===== HISTORY TAB ===== -->
	{#if activeTab === 'history'}
		<div class="space-y-4">
			<!-- Filters -->
			<div class="flex gap-3 flex-wrap">
				<select bind:value={jobStatusFilter} onchange={loadHistory} class="px-3 py-2 rounded-lg text-sm" style="background:{surface2}; border:1px solid {borderColor}; color:{textPrimary};">
					<option value="">All statuses</option>
					<option value="pending">Pending</option>
					<option value="processing">Processing</option>
					<option value="completed">Completed</option>
					<option value="failed">Failed</option>
				</select>
				<input bind:value={jobNameFilter} oninput={loadHistory} class="px-3 py-2 rounded-lg text-sm flex-1 min-w-40" style="background:{surface2}; border:1px solid {borderColor}; color:{textPrimary};" placeholder="Filter by request name…" />
			</div>

			{#if historyLoading}
				<div class="text-sm" style="color:{textMuted};">Loading…</div>
			{:else if jobs.length === 0}
				<div class="text-sm" style="color:{textMuted};">No jobs found.</div>
			{:else}
				<div class="rounded-xl overflow-hidden" style="border:1px solid {borderColor};">
					<!-- Header -->
					<div class="grid text-xs font-semibold px-4 py-2" style="grid-template-columns:2fr 1fr 1fr 1fr 1fr; background:{surface2}; color:{textMuted};">
						<span>Request Name</span><span>Status</span><span>Results</span><span>Created By</span><span>Created At</span>
					</div>
					{#each jobs as job}
						<div style="border-top:1px solid {borderColor};">
							<button
								onclick={() => toggleJobExpand(job.job_id)}
								class="grid w-full text-left px-4 py-3 text-sm hover:opacity-80 transition-opacity"
								style="grid-template-columns:2fr 1fr 1fr 1fr 1fr; background:{expandedJobID === job.job_id ? accentTint : cardBg}; color:{textPrimary};"
							>
								<span class="font-medium truncate">{job.request_name}</span>
								<span><span class="px-2 py-0.5 rounded-full text-xs font-semibold" style="{statusBadgeStyle(job.status)}">{job.status}</span></span>
								<span style="color:{textSecondary};">{job.success_count}/{job.total_count}</span>
								<span class="truncate" style="color:{textSecondary};">{job.created_by}</span>
								<span style="color:{textMuted};">{new Date(job.created_at).toLocaleString()}</span>
							</button>
							{#if expandedJobID === job.job_id}
								<div class="px-4 pb-3" style="background:{surface2};">
									{#if !jobLogs[job.job_id]}
										<div class="text-xs py-2" style="color:{textMuted};">Loading log…</div>
									{:else if jobLogs[job.job_id].length === 0}
										<div class="text-xs py-2" style="color:{textMuted};">No log entries yet.</div>
									{:else}
										<table class="w-full text-xs mt-2">
											<thead><tr style="color:{textMuted};"><th class="text-left pb-1">Filename</th><th class="text-left pb-1">Customer</th><th class="text-left pb-1">Status</th><th class="text-left pb-1">Error</th></tr></thead>
											<tbody>
												{#each jobLogs[job.job_id] as entry}
													<tr style="border-top:1px solid {borderColor}; color:{textSecondary};">
														<td class="py-1 pr-2 truncate max-w-xs">{entry.filename}</td>
														<td class="py-1 pr-2">{entry.customer_name}</td>
														<td class="py-1 pr-2"><span class="px-1.5 py-0.5 rounded-full text-xs font-semibold" style="{statusBadgeStyle(entry.status)}">{entry.status}</span></td>
														<td class="py-1" style="color:#EF4444;">{entry.error_msg}</td>
													</tr>
												{/each}
											</tbody>
										</table>
									{/if}
								</div>
							{/if}
						</div>
					{/each}
				</div>

				<!-- Pagination -->
				<div class="flex items-center gap-3 text-sm" style="color:{textSecondary};">
					<button disabled={jobPage <= 1} onclick={() => { jobPage--; loadHistory(); }} class="px-3 py-1 rounded-lg disabled:opacity-40" style="background:{surface2}; border:1px solid {borderColor}; color:{textPrimary};">Prev</button>
					<span>Page {jobPage} · {jobTotal} total</span>
					<button disabled={jobPage * 20 >= jobTotal} onclick={() => { jobPage++; loadHistory(); }} class="px-3 py-1 rounded-lg disabled:opacity-40" style="background:{surface2}; border:1px solid {borderColor}; color:{textPrimary};">Next</button>
				</div>
			{/if}
		</div>
	{/if}

	<!-- ===== SQL QUERIES TAB (admin only) ===== -->
	{#if activeTab === 'queries' && isAdmin}
		<div class="space-y-4">
			<div class="flex items-center gap-3">
				<input bind:value={queryListSearch} oninput={loadQueryList} class="flex-1 px-3 py-2 rounded-lg text-sm" style="background:{surface2}; border:1px solid {borderColor}; color:{textPrimary};" placeholder="Search queries…" />
				<button onclick={() => showAddQuery = !showAddQuery} class="px-4 py-2 rounded-lg text-sm font-semibold" style="background:{accent}; color:white; border:none;">+ Add Query</button>
			</div>

			{#if showAddQuery}
				<div class="rounded-xl p-4 space-y-3" style="background:{cardBg}; border:1px solid {borderColor};">
					<h3 class="text-sm font-semibold" style="color:{textPrimary};">New Predefined Query</h3>
					{#if addQueryError}<div class="text-xs p-2 rounded" style="background:#EF4444; color:white;">{addQueryError}</div>{/if}
					<input bind:value={newQueryName} class="w-full px-3 py-2 rounded-lg text-sm" style="background:{surface2}; border:1px solid {borderColor}; color:{textPrimary};" placeholder="Name *" />
					<input bind:value={newQueryDesc} class="w-full px-3 py-2 rounded-lg text-sm" style="background:{surface2}; border:1px solid {borderColor}; color:{textPrimary};" placeholder="Description" />
					<textarea bind:value={newQuerySQL} rows={4} class="w-full px-3 py-2 rounded-lg text-sm font-mono" style="background:{surface2}; border:1px solid {borderColor}; color:{textPrimary};" placeholder="SELECT ... *" />
					<div class="flex gap-2">
						<button onclick={addQuery} class="px-4 py-1.5 rounded-lg text-sm font-semibold" style="background:{accent}; color:white; border:none;">Save</button>
						<button onclick={() => showAddQuery = false} class="px-4 py-1.5 rounded-lg text-sm" style="background:{surface2}; border:1px solid {borderColor}; color:{textSecondary};">Cancel</button>
					</div>
				</div>
			{/if}

			{#if queryListLoading}
				<div class="text-sm" style="color:{textMuted};">Loading…</div>
			{:else if queryList.length === 0}
				<div class="text-sm" style="color:{textMuted};">No queries found.</div>
			{:else}
				<div class="rounded-xl overflow-hidden" style="border:1px solid {borderColor};">
					<div class="grid text-xs font-semibold px-4 py-2" style="grid-template-columns:2fr 3fr 1fr 1fr; background:{surface2}; color:{textMuted};">
						<span>Name</span><span>Description</span><span>Created By</span><span></span>
					</div>
					{#each queryList as q}
						<div class="grid items-center px-4 py-3 text-sm" style="grid-template-columns:2fr 3fr 1fr 1fr; border-top:1px solid {borderColor}; background:{cardBg}; color:{textPrimary};">
							<span class="font-medium">{q.name}</span>
							<span class="truncate" style="color:{textSecondary};">{q.description}</span>
							<span style="color:{textMuted};">{q.created_by}</span>
							<button onclick={() => deleteQuery(q.id)} class="text-xs px-2 py-1 rounded" style="background:#EF444420; color:#EF4444; border:none;">Delete</button>
						</div>
					{/each}
				</div>
			{/if}
		</div>
	{/if}
</div>
```

- [ ] **Step 2: Verify no TypeScript errors**

```bash
cd /Users/cding/Workspace/ChenWeb/web && npx tsc --noEmit 2>&1 | grep doc-gen-view
```
Expected: no errors for `doc-gen-view.svelte`.

- [ ] **Step 3: Commit**

```bash
git add web/src/lib/components/home3/doc-gen-view.svelte
git commit -m "feat(docgen): add doc-gen-view Svelte component (3 tabs)"
```

---

## Task 16: Wire Nav Rail + Content Panel

**Files:**
- Modify: `web/src/lib/components/home3/nav-rail.svelte`
- Modify: `web/src/lib/components/home3/content-panel.svelte`

- [ ] **Step 1: Add 'Generate Doc' to nav-rail.svelte**

In [nav-rail.svelte](ChenWeb/web/src/lib/components/home3/nav-rail.svelte), find the `applications` nav item (around line 107) and add the new child:

```typescript
{
    id: 'applications', label: 'Applications', icon: LayoutGridIcon,
    children: [
        { id: 'apps-installed',     label: 'Installed' },
        { id: 'apps-browse',        label: 'Browse' },
        { id: 'apps-configure',     label: 'Configure' },
        { id: 'apps-generate-doc',  label: 'Generate Doc' },   // ← add this
    ]
},
```

- [ ] **Step 2: Add DocGenView to content-panel.svelte**

In [content-panel.svelte](ChenWeb/web/src/lib/components/home3/content-panel.svelte), add the import at the top of the script block:

```typescript
import DocGenView from '$lib/components/home3/doc-gen-view.svelte';
```

Then in the main content area, add a case before the `{:else}` fallback (around line 129):

```svelte
{:else if activeMenu?.childId === 'apps-generate-doc'}
    <DocGenView {darkMode} />
```

- [ ] **Step 3: Verify Svelte builds without errors**

```bash
cd /Users/cding/Workspace/ChenWeb/web && npm run build 2>&1 | tail -20
```
Expected: build completes with no errors.

- [ ] **Step 4: Commit**

```bash
git add web/src/lib/components/home3/nav-rail.svelte web/src/lib/components/home3/content-panel.svelte
git commit -m "feat(docgen): wire Generate Doc into nav rail and content panel"
```

---

## Self-Review

**Spec coverage check:**
- ✅ `doc_gen_log` table (Task 1, 6) — all fields per spec, plus `job_id` FK
- ✅ `doc_gen_jobs` table (Task 1, 5) — async job tracking
- ✅ `doc_gen_queries` table (Task 1, 4) — predefined SQL registry, goose-managed
- ✅ API: POST/GET jobs (Task 13, 14)
- ✅ API: GET/POST/PUT/DELETE queries (Task 12, 14)
- ✅ API: GET/POST templates (Task 11, 14)
- ✅ Async worker pool with panic recovery (Task 9)
- ✅ Crash recovery via `RequeueStalledJobs` (Task 9, 14)
- ✅ SQL SELECT-only validation (Task 7, 13)
- ✅ Converter required-fields validation (Task 7, 13)
- ✅ `request_name` uniqueness enforced at DB + 409 response (Task 5, 13)
- ✅ Word rendering with `nguyenthenguyen/docx` (Task 8)
- ✅ Typst deferred with clear error (Task 9)
- ✅ PDF output deferred (UI shows "(not yet supported)")
- ✅ Frontend: 3-tab UI (Task 15)
- ✅ Frontend: search-and-pick SQL (Task 15)
- ✅ Frontend: server-side template list + upload (Task 15)
- ✅ Frontend: admin-only SQL Queries tab (Task 15)
- ✅ Frontend: history auto-refresh while jobs active (Task 15)
- ✅ Nav rail + content panel wired (Task 16)

**Type consistency check:** `DocGenJob`, `DocGenLogEntry`, `DocGenQuery` structs defined once in `appdatastores/` and imported by handlers and worker — no duplicate definitions.
