package appdatastores

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/chendingplano/deepdoc/server/cmd/config"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/databaseutil"
)

type ProcessStatus struct {
	Type     string    `json:"type"`
	Status   string    `json:"status"`
	RcdCount int       `json:"rcd_count"`
	CreateAt time.Time `json:"create_at"`
}

func CreateProcessStatusTable(logger ApiTypes.JimoLogger) error {
	db_type := ApiTypes.DBType
	table_name := config.AppConfig.AppTableNames.TableName_ProcessStatus
	var db *sql.DB = ApiTypes.ProjectDBHandle
	var stmt string
	switch db_type {
	case ApiTypes.MysqlName:
		stmt = "CREATE TABLE IF NOT EXISTS " + table_name + " (" +
			"id 			SERIAL 			PRIMARY KEY, " +
			"status_name 	VARCHAR(32) 	NOT NULL, " +
			"status_value 	VARCHAR(32) 	NOT NULL, " +
			"rcd_count 		INTEGER 		DEFAULT 0, " +
			"tags 			VARCHAR(255) 	NOT NULL, " +
			"created_at 	DATETIME 		DEFAULT CURRENT_TIMESTAMP)"

	case ApiTypes.PgName:
		stmt = "CREATE TABLE IF NOT EXISTS " + table_name + " (" +
			"id 			SERIAL 			PRIMARY KEY, " +
			"status_name 	VARCHAR(32) 	NOT NULL, " +
			"status_value 	VARCHAR(32) 	NOT NULL, " +
			"rcd_count 		INTEGER 		DEFAULT 0, " +
			"tags 			VARCHAR(255) 	NOT NULL, " +
			"created_at 	TIMESTAMP 		WITHOUT TIME ZONE DEFAULT NOW())"

	default:
		err := fmt.Errorf("database type not supported:%s (CWB_PST_061)", db_type)
		return err
	}

	err := databaseutil.ExecuteStatement(db, stmt)
	if err != nil {
		err1 := fmt.Errorf("failed creating table '%s' (CWB_PST_048), err: %w, stmt:%s", table_name, err, stmt)
		return err1
	}

	logger.Info("Create table success", "tablename", table_name)
	return nil
}

func RetrieveProcessStatus(rc ApiTypes.RequestContext) ([]ProcessStatus, error) {
	// Process the data (in a real application, you might save to database)
	logger := rc.GetLogger()
	logger.Info("To retrieve process_status records")

	// Query the database for dashboard data
	const select_fields = "status_name, status_value, rcd_count, created_at"
	table_name := config.AppConfig.AppTableNames.TableName_ProcessStatus
	stmt := fmt.Sprintf("SELECT %s FROM %s ORDER BY created_at desc LIMIT 3000", select_fields, table_name)
	var db *sql.DB = ApiTypes.ProjectDBHandle

	rows, err := db.Query(stmt)
	if err != nil {
		return nil, err
	}

	defer rows.Close()

	var scanned_results []ProcessStatus
	for rows.Next() {
		var row ProcessStatus

		// Use sql.NullTime for the datetime field to handle NULLs properly
		var createAt sql.NullTime

		if err := rows.Scan(&row.Type,
			&row.Status,
			&row.RcdCount,
			&createAt); err != nil {
			logger.Error("Row scan error", "error", err)
			continue
		}

		if row.Status == "" {
			row.Status = "empty"
		}

		scanned_results = append(scanned_results, row)
	}
	return scanned_results, nil
}
