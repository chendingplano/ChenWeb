package appdatastores

import (
	"database/sql"
	"fmt"
	"log"
	"time"

	"github.com/chendingplano/deepdoc/server/cmd/config"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/databaseutil"
)

type ProcessStatus struct {
	Type 		string  	`json:"type"`
	Status		string  	`json:"status"`
	RcdCount	int			`json:"rcd_count"`
	CreateAt	time.Time 	`json:"create_at"`
}

func CreateProcessStatusTable() error {
	db_type := ApiTypes.DatabaseInfo.DBType
	table_name := config.GlobalConfig.AppTableNames.TableName_ProcessStatus
	var db *sql.DB
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
		 db = ApiTypes.DatabaseInfo.MySQLDBHandle

    case ApiTypes.PgName:
         stmt = "CREATE TABLE IF NOT EXISTS " + table_name + " (" +
            "id 			SERIAL 			PRIMARY KEY, " +
            "status_name 	VARCHAR(32) 	NOT NULL, " +
            "status_value 	VARCHAR(32) 	NOT NULL, " +
            "rcd_count 		INTEGER 		DEFAULT 0, " +
            "tags 			VARCHAR(255) 	NOT NULL, " +
            "created_at 	TIMESTAMP 		WITHOUT TIME ZONE DEFAULT NOW())"
		 db = ApiTypes.DatabaseInfo.PGDBHandle

    default:
        err := fmt.Errorf("database type not supported:%s (CWB_PST_061)", db_type)
        log.Printf("***** Alarm:%s", err.Error())
        return err
    }

    err := databaseutil.ExecuteStatement(db, stmt)
    if err != nil {
        err1 := fmt.Errorf("failed creating table '%s' (CWB_PST_048), err: %w, stmt:%s", table_name, err, stmt)
        log.Printf("***** Alarm: %s", err1.Error())
        return err1
    }

    log.Printf("Create table '%s' success (CWB_PST_060)", table_name)
    return nil
}

func RetrieveProcessStatus() ([]ProcessStatus, error) {
	// Process the data (in a real application, you might save to database)
	fmt.Printf("To retrieve process_status records (CWB_PST_073)")

	// Query the database for dashboard data
	const select_fields = "status_name, status_value, rcd_count, created_at"
	db_type := ApiTypes.DatabaseInfo.DBType
	table_name := config.GlobalConfig.AppTableNames.TableName_ProcessStatus
	stmt := fmt.Sprintf("SELECT %s FROM %s ORDER BY created_at desc LIMIT 3000", select_fields, table_name)
	var db *sql.DB
	switch db_type {
	case ApiTypes.MysqlName:
		 db = ApiTypes.MySql_DB_miner

	case ApiTypes.PgName:
		 db = ApiTypes.PG_DB_miner

	default:
        err := fmt.Errorf("database type not supported:%s (CWB_PST_082)", db_type)
        log.Printf("***** Alarm:%s", err.Error())
        return nil, err
	}

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
			log.Printf("Row scan error: %v (CWB_PST_096)", err)
			continue
		}

		if row.Status == "" {
			row.Status = "empty"
		}

		scanned_results = append(scanned_results, row)
	}
	return scanned_results, nil
}