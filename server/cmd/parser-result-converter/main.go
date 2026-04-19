package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	fileconverters "github.com/chendingplano/deepdoc/server/api/file-converters"
	"github.com/chendingplano/deepdoc/server/cmd/config"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/ApiUtils"
	"github.com/chendingplano/shared/go/api/databaseutil"
	"github.com/chendingplano/shared/go/api/loggerutil"
	"github.com/joho/godotenv"
)

func envOrDefault(key, fallback string) string {
	v := strings.TrimSpace(os.Getenv(key))
	if v != "" {
		return v
	}
	return fallback
}

func envFirst(keys ...string) string {
	for _, key := range keys {
		v := strings.TrimSpace(os.Getenv(key))
		if v != "" {
			return v
		}
	}
	return ""
}

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	_ = godotenv.Load("./.env")
	_ = godotenv.Load()

	logger := loggerutil.CreateDefaultLogger("CWB_PDF_CONVERT_001")
	defer logger.Close()
	ApiUtils.LoadLibConfig("CWB_PDF_CONVERT_002")

	configPath := envFirst("PARSER_RESULT_CONVERTER_CONFIG", "PDF_RESULT_CONVERTER_CONFIG", "FILE_CONVERTER_CONFIG")
	if configPath == "" {
		configPath = "../../../config.toml"
	}
	loadCtx := context.WithValue(ctx, ApiTypes.CallFlowKey, "CWB_PDF_CONVERT_003")
	if err := config.LoadConfig(loadCtx, logger, configPath); err != nil {
		logger.Error("failed loading config", "error", err, "config", configPath)
		os.Exit(1)
	}
	config.NormalizeMigrationPaths(logger, configPath)

	if err := databaseutil.InitDB(loadCtx, ApiTypes.CommonConfig); err != nil {
		logger.Error("failed to initialize db", "error", err)
		os.Exit(1)
	}
	defer databaseutil.CloseDatabase(ApiTypes.CommonConfig)

	if err := config.RunMigrations(ctx, logger); err != nil {
		logger.Error("migrations failed", "error", err)
		os.Exit(1)
	}

	natsURL := envOrDefault("NATS_URL", "nats://127.0.0.1:4222")
	subject := "kb.pdf.parsed"
	durable := envFirst("PARSER_RESULT_CONVERTER_DURABLE", "PDF_RESULT_CONVERTER_DURABLE", "PDF_CONVERTER_DURABLE")
	if durable == "" {
		durable = "parser-result-converter"
	}
	streamName := envFirst("PARSER_RESULT_CONVERTER_STREAM", "PDF_RESULT_CONVERTER_STREAM", "PDF_CONVERTER_STREAM")

	sub, err := fileconverters.NewNATSSubscriber(natsURL)
	if err != nil {
		logger.Error("failed creating nats subscriber", "error", err, "nats_url", natsURL)
		os.Exit(1)
	}
	defer sub.Close()
	if err := sub.EnsureStream(streamName, subject); err != nil {
		logger.Error("failed to ensure stream", "error", err, "stream", streamName, "subject", subject)
		os.Exit(1)
	}

	svc := fileconverters.NewService(
		fileconverters.SQLStore{DB: ApiTypes.ProjectDBHandle},
		slog.Default(),
	)
	svc.Publisher = sub
	svc.PublishSubject = fileconverters.DefaultLineFileGeneratedSubject

	go func() {
		quit := make(chan os.Signal, 1)
		signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
		<-quit
		logger.Info("shutdown signal received")
		cancel()
	}()

	logger.Info("file converter service starting",
		"nats_url", natsURL,
		"subject", subject,
		"durable", durable,
		"stream", streamName,
		"started_at", time.Now().Format(time.RFC3339),
	)

	if err := svc.Run(ctx, sub, subject, durable); err != nil && ctx.Err() == nil {
		logger.Error("file converter service exited with error", "error", err)
		os.Exit(1)
	}

	fmt.Println("file converter service stopped")
}
