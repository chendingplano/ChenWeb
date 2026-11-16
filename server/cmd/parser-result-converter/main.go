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
	"github.com/chendingplano/deepdoc/server/buildinfo"
	"github.com/chendingplano/deepdoc/server/cmd/config"
	"github.com/chendingplano/deepdoc/server/processhealth"
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
	health, healthErr := processhealth.NewReporter("parser-result-converter")
	if healthErr != nil {
		fmt.Fprintln(os.Stderr, "cannot publish process health report:", healthErr)
	}
	healthFail := func(reason, resource, message string, recovery processhealth.Recovery) {
		// Schema/bootstrap failures can be caused by a temporary database outage.
		// Do not latch an operator-only failure when the dependency is unreachable.
		if recovery == processhealth.OperatorAction && reason != "configuration_invalid" && ApiTypes.ProjectDBHandle != nil {
			checkCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			dbErr := ApiTypes.ProjectDBHandle.PingContext(checkCtx)
			cancel()
			if dbErr != nil {
				reason, resource, message, recovery = "database_unavailable", "project database", "Service cannot access its database during initialization", processhealth.Restart
			}
		}
		if err := health.Set(processhealth.Blocked, reason, resource, message, recovery); err != nil {
			fmt.Fprintln(os.Stderr, "cannot publish blocked health report:", err)
		}
		if recovery == processhealth.OperatorAction {
			os.Exit(processhealth.ExitOperatorAction)
		}
		os.Exit(1)
	}
	defer health.CapturePanic()
	if issue := processhealth.ApplicationEnvironment(); issue != nil {
		healthFail(issue.Reason, issue.Resource, issue.Message, issue.Recovery)
	}
	ApiUtils.LoadLibConfig("CWB_PDF_CONVERT_002")

	configPath := envFirst("PARSER_RESULT_CONVERTER_CONFIG", "PDF_RESULT_CONVERTER_CONFIG", "FILE_CONVERTER_CONFIG")
	if configPath == "" {
		configPath = "../../../config.toml"
	}
	loadCtx := context.WithValue(ctx, ApiTypes.CallFlowKey, "CWB_PDF_CONVERT_003")
	if err := config.LoadConfig(loadCtx, logger, configPath); err != nil {
		logger.Error("failed loading config", "error", err, "config", configPath)
		healthFail("configuration_invalid", "config.toml", "Required service configuration could not be loaded", processhealth.OperatorAction)
	}
	config.NormalizeMigrationPaths(logger, configPath)

	if err := databaseutil.InitDB(loadCtx, ApiTypes.CommonConfig); err != nil {
		logger.Error("failed to initialize db", "error", err)
		healthFail("database_unavailable", "project database", "Project database could not be initialized", processhealth.Restart)
	}
	defer databaseutil.CloseDatabase(ApiTypes.CommonConfig)

	if err := config.RunMigrations(ctx, logger); err != nil {
		logger.Error("migrations failed", "error", err)
		healthFail("migration_failed", "database schema", "Database migrations failed; inspect migration logs", processhealth.OperatorAction)
	}
	stopBuildInfo, buildInfoErr := buildinfo.StartResponder("parser-result-converter")
	if buildInfoErr != nil {
		logger.Warn("build-info responder unavailable", "executable", "parser-result-converter", "error", buildInfoErr)
	} else {
		defer stopBuildInfo()
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
		healthFail("broker_unavailable", "NATS", "Message broker connection could not be established", processhealth.Restart)
	}
	defer sub.Close()
	if err := sub.EnsureStream(streamName, subject); err != nil {
		logger.Error("failed to ensure stream", "error", err, "stream", streamName, "subject", subject)
		healthFail("stream_unavailable", "NATS stream", "Required message stream could not be initialized", processhealth.Restart)
	}

	svc := fileconverters.NewService(
		fileconverters.SQLStore{DB: ApiTypes.ProjectDBHandle},
		slog.Default(),
	)
	svc.Publisher = sub
	svc.PublishSubject = fileconverters.DefaultLineFileGeneratedSubject

	publishStreamName := envFirst("DOC_PROCESSOR_EVENT_STREAM", "EXTRACT_DOCMETA_EVENT_STREAM")
	if err := sub.EnsureStream(publishStreamName, svc.PublishSubject); err != nil {
		logger.Error("failed to ensure downstream publish stream",
			"error", err,
			"stream", publishStreamName,
			"subject", svc.PublishSubject,
		)
		healthFail("stream_unavailable", "downstream stream", "Required downstream message stream could not be initialized", processhealth.Restart)
	}

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
		"publish_subject", svc.PublishSubject,
		"publish_stream", publishStreamName,
		"started_at", time.Now().Format(time.RFC3339),
	)

	health.GoCheck(ctx, 15*time.Second, func(checkCtx context.Context) *processhealth.Issue {
		if ApiTypes.ProjectDBHandle == nil || ApiTypes.ProjectDBHandle.PingContext(checkCtx) != nil {
			return &processhealth.Issue{Reason: "database_unavailable", Resource: "project database", Message: "Converter cannot access its database", Recovery: processhealth.Restart}
		}
		if issue := processhealth.WritableDirectory(strings.TrimSpace(os.Getenv("DATA_HOME_DIR"))); issue != nil {
			return issue
		}
		if sub.CheckHealth(checkCtx) != nil {
			return &processhealth.Issue{Reason: "subscription_unavailable", Resource: "NATS", Message: "Converter subscription is unavailable", Recovery: processhealth.Restart}
		}
		return nil
	}, func(err error) { logger.Error("failed writing process health", "error", err) })
	if err := svc.Run(ctx, sub, subject, durable); err != nil && ctx.Err() == nil {
		_ = health.Set(processhealth.Degraded, "subscription_exited", subject, "Converter subscription exited unexpectedly", processhealth.Restart)
		logger.Error("file converter service exited with error", "error", err)
		healthFail("subscription_exited", "NATS subscription", "Converter subscription exited unexpectedly", processhealth.Restart)
	}

	fmt.Println("file converter service stopped")
}
