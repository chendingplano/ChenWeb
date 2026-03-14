package main

import (
	"context"
	"os"
	"time"

	"github.com/chendingplano/deepdoc/server/api/database"
	"github.com/chendingplano/deepdoc/server/cmd/config"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/ApiUtils"
	"github.com/chendingplano/shared/go/api/databaseutil"
	"github.com/chendingplano/shared/go/api/loggerutil"
	"github.com/joho/godotenv"
)

func main() {
	// Create a context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	new_ctx := context.WithValue(ctx, ApiTypes.CallFlowKey, "CWB_DMN_019")

	env_path := "./.env"
	env_err := godotenv.Load(env_path)

	ApiUtils.LoadLibConfig("CWB_0207150500")
	var logger = loggerutil.CreateDefaultLogger("CWB_MAN_032")
	logger.Info("load .env", "loc", "(CWB_DDM_198)")

	if env_err != nil {
		logger.Error("***** Alarm",
			"message", "Cound not load .env file",
			"error", env_err,
			"path", env_path)
		os.Exit(1)
	}

	// Load config
	logger.Info("Loading config from config.toml (main)")
	err := config.LoadConfig(new_ctx, logger, "./config.toml")
	if err != nil {
		logger.Error("loading config failed", "path", "./config.toml")
	}

	if err := databaseutil.InitDB(new_ctx, ApiTypes.CommonConfig); err != nil {
		logger.Error("Failed to initialize database", "error", err)
		os.Exit(2)
	}

	logger.Info("To create process status table (main)")
	database.CreateTables(logger)

	sleepSeconds := 3600 * 24
	tags := "test"
	process_table_name := "xk_parse_file_process"
	RunSQLPeriodically(process_table_name, tags, sleepSeconds)
}
