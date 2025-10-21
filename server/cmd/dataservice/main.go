package main

import (
	"log"

	"github.com/chendingplano/deepdoc/server/api/Databases"
	"github.com/chendingplano/deepdoc/server/cmd/config"
)

func main() {
	Databases.InitDB()
	log.Printf("To create process status table (main)")
	db_type := config.GetDatabaseType()
	Databases.AosCreateProcessStatusTable(Databases.MySql_DB_miner, db_type)

	sleepSeconds := 3600 * 24
	tags := "test"
	process_table_name := "xk_parse_file_process"
    RunSQLPeriodically(process_table_name, tags, sleepSeconds)
}