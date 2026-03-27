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
		var description sql.NullString
		if err := rows.Scan(&q.ID, &q.Name, &description, &q.SQLStatement,
			&q.CreatedBy, &q.CreatedAt, &q.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan failed: %w (CWB_DGS_065)", err)
		}
		q.Description = description.String
		results = append(results, q)
	}
	if results == nil {
		results = []DocGenQuery{}
	}
	return results, rows.Err()
}

func GetDocGenQuery(db *sql.DB, id int64) (*DocGenQuery, error) {
	var q DocGenQuery
	var description sql.NullString
	err := db.QueryRow(
		`SELECT id, name, description, sql_statement, created_by, created_at, updated_at
		 FROM doc_gen_queries WHERE id=$1`, id,
	).Scan(&q.ID, &q.Name, &description, &q.SQLStatement,
		&q.CreatedBy, &q.CreatedAt, &q.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("GetDocGenQuery failed: %w (CWB_DGS_070)", err)
	}
	q.Description = description.String
	return &q, nil
}
