package main

import (
	"log"

	"github.com/chendingplano/deepdoc/server/api/database"
	"github.com/chendingplano/deepdoc/server/cmd/config"
	"github.com/chendingplano/shared/go/api/databaseutil"
)

func main() {
	err := config.LoadConfig("./config.toml")
	if err != nil {
		log.Fatal(err)
	}

	databaseutil.InitDB(config.MySQLConfig, config.PGConfig)
	log.Printf("To create process status table (main)")
	database.CreateTables()

	sleepSeconds := 3600 * 24
	tags := "test"
	process_table_name := "xk_parse_file_process"
    RunSQLPeriodically(process_table_name, tags, sleepSeconds)
}