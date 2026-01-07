package main

import (
	"context"
	"log"
	"time"

	"github.com/chendingplano/deepdoc/server/api/database"
	"github.com/chendingplano/deepdoc/server/cmd/config"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/databaseutil"
)

func main() {
	// Create a context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	new_ctx := context.WithValue(ctx, ApiTypes.CallFlowKey, "CWB_DMN_019")

	// Load config
	log.Printf("Loading config from config.toml (main)")
	err := config.LoadConfig(new_ctx, "./config.toml")
	if err != nil {
		log.Fatal(err)
	}

	databaseutil.InitDB(new_ctx, config.MySQLConfig, config.PGConfig)
	log.Printf("To create process status table (main)")
	database.CreateTables()

	sleepSeconds := 3600 * 24
	tags := "test"
	process_table_name := "xk_parse_file_process"
	RunSQLPeriodically(process_table_name, tags, sleepSeconds)
}
