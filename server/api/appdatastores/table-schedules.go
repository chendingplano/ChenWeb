package appdatastores

import (
	"database/sql"
	"fmt"
	"log"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/databaseutil"
)

func CreateSchedulesTable(
	db *sql.DB,
	db_type string,
	table_name string,
) error {
	var stmt string

	const common_fields = "event_name			VARCHAR(64)			NOT NULL, " +
		"urgency 			VARCHAR(32)			DEFAULT NULL, " +
		"message 			TEXT				DEFAULT NULL, " +
		"user_email 		VARCHAR(128)		DEFAULT NULL, " +
		"cell_phone 		VARCHAR(32)			DEFAULT NULL, " +
		"sales_lead 		VARCHAR(64)			DEFAULT NULL, " +
		"remarks 			TEXT				DEFAULT NULL, " +
		"schedule_date 	    DATE				NOT NULL, " +
		"schedule_hour      INTETER             NOT NULL, " +
		"schedule_minute    INTETER             NOT NULL, " +
		"creator            VARCHAR(64) 		NOT NULL, " +
		"updater            VARCHAR(64) 		NOT NULL, "

	switch db_type {
	case ApiTypes.MysqlName:
		stmt = "CREATE TABLE IF NOT EXISTS " + table_name + " (" + common_fields +
			"created_at 	DATETIME 		DEFAULT CURRENT_TIMESTAMP, " +
			"updated_at 	DATETIME 		DEFAULT CURRENT_TIMESTAMP)"

	case ApiTypes.PgName:
		stmt = "CREATE TABLE IF NOT EXISTS " + table_name + " (" + common_fields +
			"created_at 	TIMESTAMP 		WITHOUT TIME ZONE DEFAULT NOW(), " +
			"updated_at 	TIMESTAMP 		WITHOUT TIME ZONE DEFAULT NOW())"

	default:
		err := fmt.Errorf("database type not supported:%s (CWB_SCD_052)", db_type)
		log.Printf("***** Alarm:%s", err.Error())
		return err
	}

	err := databaseutil.ExecuteStatement(db, stmt)
	if err != nil {
		err1 := fmt.Errorf("failed creating table '%s' (CWB_SCD_059), err: %w, stmt:%s", table_name, err, stmt)
		log.Printf("***** Alarm: %s", err1.Error())
		return err1
	}

	log.Printf("Create table '%s' success (CWB_SCD_064)", table_name)
	return nil
}
