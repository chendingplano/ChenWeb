package appdatastores

import (
	"database/sql"
	"fmt"

	_ "github.com/lib/pq"
)

// TableCustRequestLogDef represents a customer request log record stored in the database.
type TableCustRequestLogDef struct {
	ID        int64  `json:"id"`
	CustName  string `json:"cust_name"`
	CustEmail string `json:"cust_email"`
	Desc      string `json:"description"`
	Purpose   string `json:"purpose"`
	CreateAt  string `json:"create_at"`
	UpdateAt  string `json:"update_at"`
}

// InsertCustRequestLog inserts a new customer request log record and returns the new ID.
func InsertCustRequestLog(db *sql.DB, tableName string, r TableCustRequestLogDef) (int64, error) {
	stmt := fmt.Sprintf(`
		INSERT INTO %s (cust_name, cust_email, description, purpose)
		VALUES ($1, $2, $3, $4)
		RETURNING id`, tableName)

	var newID int64
	err := db.QueryRow(stmt, r.CustName, r.CustEmail, r.Desc, r.Purpose).Scan(&newID)
	if err != nil {
		return 0, fmt.Errorf("failed to insert cust_request_log (CWB_CRL_024), err: %w", err)
	}
	return newID, nil
}
