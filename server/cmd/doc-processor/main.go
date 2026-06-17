package main

import (
	"context"
	"fmt"
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
	"github.com/chendingplano/shared/go/api/observability"
	"github.com/joho/godotenv"
	"github.com/spf13/viper"
)

type chunkPhaseService interface {
	HandleInput(ctx context.Context, recordID int64, inputFilename string, inputFile []byte) error
}

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

func buildChunkPhaseProcessors(
	inputStore docprocessing.DocMetadataStore,
	fixedChunkSvc chunkPhaseService,
	logger ApiTypes.JimoLogger,
) []docprocessing.Processor {
	return []docprocessing.Processor{
		docprocessing.NewChunkingProcessor(inputStore, fixedChunkSvc, logger),
		docprocessing.NewGenerateSummariesProcessor(inputStore, fixedChunkSvc, logger),
		docprocessing.NewGenerateTopicsProcessor(inputStore, fixedChunkSvc, logger),
	}
}

func normalizeProcessorName(raw string) string {
	name := strings.ToLower(strings.TrimSpace(raw))
	name = strings.ReplaceAll(name, "-", "_")
	switch name {
	case "extract_metadata":
		return "extract_doc_metadata"
	default:
		return name
	}
}

func configuredProcessorNames() []string {
	return viper.GetStringSlice("doc-processing.required_processors")
}

func filterConfiguredProcessors(
	processors []docprocessing.Processor,
	required []string,
) []docprocessing.Processor {
	mandatory := map[string]struct{}{
		"static_analyzer":      {},
		"chunking":             {},
		"extract_doc_metadata": {},
	}
	enabled := make(map[string]struct{}, len(required))
	for _, name := range required {
		if normalized := normalizeProcessorName(name); normalized != "" {
			enabled[normalized] = struct{}{}
		}
	}

	filtered := make([]docprocessing.Processor, 0, len(processors))
	for _, p := range processors {
		if p == nil {
			continue
		}
		name := normalizeProcessorName(p.Name())
		if _, ok := mandatory[name]; ok {
			filtered = append(filtered, p)
			continue
		}
		if _, ok := enabled[name]; ok {
			filtered = append(filtered, p)
		}
	}
	return filtered
}

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	_ = godotenv.Load("./.env")
	_ = godotenv.Load()

	logger := loggerutil.CreateDefaultLogger("CWB_DOCPROC_001")
	defer logger.Close()
	ApiUtils.LoadLibConfig("CWB_DOCPROC_002")

	obsCfg := observability.ConfigFromEnv("chenweb-doc-processor")
	obsShutdown, err := observability.Init(ctx, obsCfg)
	if err != nil {
		logger.Error("failed to initialize observability", "error", err, "enabled", obsCfg.Enabled)
		os.Exit(1)
	}
	defer func() {
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()
		if err := obsShutdown(shutdownCtx); err != nil {
			logger.Warn("failed to shutdown observability", "error", err)
		}
	}()
	if obsCfg.Enabled {
		logger.Info("observability enabled",
			"service", obsCfg.ServiceName,
			"environment", obsCfg.Environment,
			"otlp_endpoint", obsCfg.OTLPEndpoint)
	} else {
		logger.Info("observability disabled")
	}

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
	docProcessorMode, err := docprocessing.DocProcessorModeFromEnv()
	if err != nil {
		logger.Error("invalid DOC_PROCESSOR_MODE", "error", err)
		os.Exit(1)
	}

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
	staticAnalyzerLLMClient := newLLMClient()
	metricsLLMClient := newLLMClient()
	provisionsLLMClient := newLLMClient()
	sceneBlocksLLMClient := newLLMClient()
	semanticProjectionsLLMClient := newLLMClient()
	knowledgeLLMClient := newLLMClient()
	entityRelationLLMClient := newLLMClient()
	inventoryItemsLLMClient := newLLMClient()
	// productsLLMClient := newLLMClient()
	// structureLLMClient := newLLMClient()
	fixedChunkLLMClient := newLLMClient()

	inputStore := docprocessing.DocMetadataSQLStore{DB: ApiTypes.ProjectDBHandle}
	fixedChunkSvc := docprocessing.NewFixedSizeChunkingService(
		docprocessing.SQLStore{DB: ApiTypes.ProjectDBHandle},
		fixedChunkLLMClient,
		logger,
	)
	phaseProcessors := buildChunkPhaseProcessors(inputStore, fixedChunkSvc, logger)

	control := &docprocessing.ControlService{
		Logger:                 logger,
		InputStore:             inputStore,
		EventStore:             docprocessing.SQLStore{DB: ApiTypes.ProjectDBHandle},
		StopStore:              docprocessing.StopRequestSQLStore{DB: ApiTypes.ProjectDBHandle},
		Now:                    time.Now,
		MaxDocProcessPipelines: docprocessing.MaxDocProcessPipelinesFromEnv(),
		BlockingProcessor:      docprocessing.NewBlockingProcessor(inputStore, logger),
		Processors: filterConfiguredProcessors([]docprocessing.Processor{
			// docprocessing.NewStructureAnalyzerProcessor(inputStore, structureLLMClient, logger),
			docprocessing.NewStaticAnalyzerProcessor(inputStore, staticAnalyzerLLMClient, logger),
			phaseProcessors[0],
			phaseProcessors[1],
			phaseProcessors[2],
			docprocessing.NewExtractDocMetadataProcessor(inputStore, llmClient, logger),
			docprocessing.NewSemanticProjectionsProcessor(inputStore, docprocessing.SemanticProjectionsSQLStore{DB: ApiTypes.ProjectDBHandle}, semanticProjectionsLLMClient, logger),
			docprocessing.NewStructuredKnowledgeProcessor(inputStore, docprocessing.StructuredKnowledgeSQLStore{DB: ApiTypes.ProjectDBHandle}, knowledgeLLMClient, logger),
			// ADR 2026061702: entity and relation extraction are split into two
			// independent processors that run in parallel (concurrent mode). Endpoint
			// linking happens once in Phase C, owned by the relation processor.
			docprocessing.NewEntityProcessor(inputStore, docprocessing.EntityRelationSQLStore{DB: ApiTypes.ProjectDBHandle}, entityRelationLLMClient, logger),
			docprocessing.NewRelationProcessor(inputStore, docprocessing.EntityRelationSQLStore{DB: ApiTypes.ProjectDBHandle}, entityRelationLLMClient, logger),
			docprocessing.NewInventoryItemsProcessor(inputStore, docprocessing.InventoryItemsSQLStore{DB: ApiTypes.ProjectDBHandle}, inventoryItemsLLMClient, logger),
			docprocessing.NewMetricsProcessor(inputStore, docprocessing.MetricsSQLStore{DB: ApiTypes.ProjectDBHandle}, metricsLLMClient, logger),
			docprocessing.NewProvisionsProcessor(inputStore, docprocessing.ProvisionsSQLStore{DB: ApiTypes.ProjectDBHandle}, provisionsLLMClient, logger),
			docprocessing.NewSceneBlocksProcessor(inputStore, docprocessing.SceneObjectsSQLStore{DB: ApiTypes.ProjectDBHandle}, sceneBlocksLLMClient, logger),
			// docprocessing.NewProductsProcessor(inputStore, docprocessing.ProductsSQLStore{DB: ApiTypes.ProjectDBHandle}, productsLLMClient, logger),
		}, configuredProcessorNames()),
	}

	processorNames := []string{"blocking"}
	for _, p := range control.Processors {
		processorNames = append(processorNames, p.Name())
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
		"doc_processor_mode", docProcessorMode,
		"chunking_method", docprocessing.ChunkingMethodFixed,
		"generate_topics_method", docprocessing.ChunkingMethodTopic,
		"max_doc_process_pipelines", control.MaxDocProcessPipelines,
		"run_doc_processor_concurrent", docprocessing.RunDocProcessorConcurrentFromEnv(),
		"configured_required_processors", configuredProcessorNames(),
		"processors", processorNames,
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
