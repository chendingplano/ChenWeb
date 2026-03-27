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
