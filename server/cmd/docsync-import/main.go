// Command docsync-import pulls production document artifacts into development.
package main

import (
	"context"
	"errors"
	"flag"
	"os"
	"os/signal"
	"syscall"

	"github.com/chendingplano/deepdoc/server/api/docsync"
	"github.com/chendingplano/deepdoc/server/cmd/config"
	"github.com/chendingplano/deepdoc/server/processhealth"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/databaseutil"
	"github.com/chendingplano/shared/go/api/idmanager"
	"github.com/chendingplano/shared/go/api/loggerutil"
	"github.com/joho/godotenv"
)

func main() {
	path := flag.String("config", "config.toml", "application configuration path")
	scheduled := flag.Bool("schedule", false, "run nightly on the configured local time window")
	flag.Parse()
	_ = godotenv.Load(".env")
	logger := loggerutil.CreateDefaultLogger("20261120-803")
	if err := run(*path, *scheduled, logger); err != nil && !errors.Is(err, context.Canceled) {
		logger.Error("document sync importer", "error", err)
		logger.Close()
		os.Exit(1)
	}
	logger.Close()
}

func run(path string, scheduled bool, logger ApiTypes.JimoLogger) error {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, os.Interrupt)
	defer cancel()
	ctx = context.WithValue(ctx, ApiTypes.CallFlowKey, "20261120-804")
	runner, err := docsync.LoadRunner(path, logger)
	if err != nil {
		return err
	}
	if runner == nil {
		return errors.New("document sync requires a configured [data-sync] section")
	}
	if err := config.LoadConfig(ctx, logger, path); err != nil {
		return errors.New("could not load application configuration for document sync")
	}
	if err := databaseutil.InitDB(ctx, ApiTypes.CommonConfig); err != nil {
		return errors.New("could not initialize development database pools for document sync")
	}
	defer databaseutil.CloseDatabase(ApiTypes.CommonConfig)
	runner.DB = ApiTypes.ProjectDBHandle
	host, _ := os.Hostname()
	runner.IDs, err = idmanager.New(ApiTypes.SharedDBHandle, "docsync-import@"+host)
	if err != nil {
		return err
	}
	health, err := processhealth.NewReporter("docsync-import")
	if err != nil {
		return err
	}
	if scheduled {
		return runner.Schedule(ctx, health)
	}
	return runner.RunWithHealth(ctx, health)
}
