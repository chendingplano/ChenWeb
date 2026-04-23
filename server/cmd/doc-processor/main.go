package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	docprocessing "github.com/chendingplano/deepdoc/server/api/doc-processing"
	fileconverters "github.com/chendingplano/deepdoc/server/api/file-converters"
	"github.com/chendingplano/deepdoc/server/cmd/config"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/ApiUtils"
	"github.com/chendingplano/shared/go/api/databaseutil"
	llmclients "github.com/chendingplano/shared/go/api/llm"
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

	logger := loggerutil.CreateDefaultLogger("CWB_DOCPROC_001")
	defer logger.Close()
	ApiUtils.LoadLibConfig("CWB_DOCPROC_002")

	configPath := envFirst("DOC_PROCESSOR_CONFIG", "EXTRACT_DOCMETA_CONFIG", "FILE_CONVERTER_CONFIG")
	if configPath == "" {
		configPath = "../../../config.toml"
	}
	loadCtx := context.WithValue(ctx, ApiTypes.CallFlowKey, "CWB_DOCPROC_003")
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
	subject := envOrDefault("DOC_PROCESSOR_EVENT_SUBJECT", docprocessing.DefaultEventSubject)
	durable := envOrDefault("DOC_PROCESSOR_EVENT_DURABLE", "doc-processor")
	streamName := envFirst("DOC_PROCESSOR_EVENT_STREAM", "EXTRACT_DOCMETA_EVENT_STREAM")

	ns, err := fileconverters.NewNATSSubscriber(natsURL)
	if err != nil {
		logger.Error("failed creating nats subscriber", "error", err, "nats_url", natsURL)
		os.Exit(1)
	}
	defer ns.Close()
	if err := ns.EnsureStream(streamName, subject); err != nil {
		logger.Error("failed to ensure stream", "error", err, "stream", streamName, "subject", subject)
		os.Exit(1)
	}

	newLLMClient := func() *llmclients.OpenAIJSONClient {
		return &llmclients.OpenAIJSONClient{
			HTTPClient: &http.Client{Timeout: 100 * time.Second},
		}
	}
	llmClient := newLLMClient()
	metricsLLMClient := newLLMClient()
	structureLLMClient := newLLMClient()
	topicChunkLLMClient := newLLMClient()

	inputStore := docprocessing.DocMetadataSQLStore{DB: ApiTypes.ProjectDBHandle}
	fixedChunkSvc := docprocessing.NewFixedSizeChunkingService(
		docprocessing.SQLStore{DB: ApiTypes.ProjectDBHandle},
		logger,
	)
	topicChunkSvc := docprocessing.NewSemanticChunkingService(
		docprocessing.SQLStore{DB: ApiTypes.ProjectDBHandle},
		topicChunkLLMClient,
		logger,
	)
	chunkSvc, err := docprocessing.NewChunkingControllerFromEnv(fixedChunkSvc, topicChunkSvc)
	if err != nil {
		logger.Error("failed creating chunking controller", "error", err)
		os.Exit(1)
	}

	control := &docprocessing.ControlService{
		Logger:     logger,
		InputStore: inputStore,
		EventStore: docprocessing.SQLStore{DB: ApiTypes.ProjectDBHandle},
		Now:        time.Now,
		Processors: []docprocessing.Processor{
			docprocessing.NewStructureAnalyzerProcessor(inputStore, structureLLMClient, logger),
			docprocessing.NewStaticAnalyzerProcessor(inputStore, logger),
			docprocessing.NewChunkingProcessor(inputStore, chunkSvc, logger),
			docprocessing.NewExtractDocMetadataProcessor(inputStore, llmClient, logger),
			docprocessing.NewMetricsProcessor(inputStore, docprocessing.MetricsSQLStore{DB: ApiTypes.ProjectDBHandle}, metricsLLMClient, slog.Default()),
		},
	}

	go func() {
		quit := make(chan os.Signal, 1)
		signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
		<-quit
		logger.Info("shutdown signal received")
		cancel()
	}()

	logger.Info("doc processor starting",
		"nats_url", natsURL,
		"subject", subject,
		"durable", durable,
		"stream", streamName,
		"chunking_method", chunkSvc.Method,
		"processors", []string{"structure_analyzer", "static_analyzer", "chunking", "extract_doc_metadata", "extract_metrics"},
		"started_at", time.Now().Format(time.RFC3339),
	)

	err = ns.Subscribe(ctx, subject, durable, func(msgCtx context.Context, payload []byte) error {
		return control.HandleJetStreamEvent(msgCtx, subject, payload)
	})
	if err != nil && ctx.Err() == nil {
		logger.Error("doc processor exited with error", "error", err)
		os.Exit(1)
	}

	fmt.Println("doc processor stopped")
}
