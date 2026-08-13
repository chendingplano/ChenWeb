package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	docprocessing "github.com/chendingplano/deepdoc/server/api/doc-processing"
	docreviews "github.com/chendingplano/deepdoc/server/api/doc-reviews"
	fileconverters "github.com/chendingplano/deepdoc/server/api/file-converters"
	"github.com/chendingplano/deepdoc/server/api/llmusage"
	"github.com/chendingplano/deepdoc/server/api/ontology/seed"
	"github.com/chendingplano/deepdoc/server/cmd/config"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/ApiUtils"
	"github.com/chendingplano/shared/go/api/databaseutil"
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

func uniqueNonEmpty(values ...string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func docProcessorSubjects(primary string) []string {
	return uniqueNonEmpty(
		primary,
		docprocessing.DefaultEventSubject,
		fileconverters.DefaultLineFileGeneratedSubject,
	)
}

func docProcessorDurable(primarySubject, primaryDurable, subject string) string {
	if strings.TrimSpace(subject) == strings.TrimSpace(primarySubject) {
		return primaryDurable
	}
	switch strings.TrimSpace(subject) {
	case docprocessing.DefaultEventSubject:
		return primaryDurable + "-start-doc-processing"
	case fileconverters.DefaultLineFileGeneratedSubject:
		return primaryDurable + "-line-file-generated"
	default:
		return primaryDurable + "-compat"
	}
}

func docProcessorStream(primarySubject, primaryStream, subject string) string {
	if strings.TrimSpace(subject) == strings.TrimSpace(primarySubject) {
		return primaryStream
	}
	switch strings.TrimSpace(subject) {
	case docprocessing.DefaultEventSubject:
		return "doc-processor-start-events"
	case fileconverters.DefaultLineFileGeneratedSubject:
		return "doc-processor-line-file-events"
	default:
		return "doc-processor-compat-events"
	}
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

var installDefaultUsageSink = llmusage.InstallDefaultSink

func ensureLLMUsageSink() error {
	if installDefaultUsageSink == nil {
		return nil
	}
	return installDefaultUsageSink()
}

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// wg tracks background work (event subscriptions and the pipelines they
	// dispatch) so shutdown can wait for it to finish before the deferred
	// database/connection cleanup below runs. Without this, cancel() (from a
	// signal or a subscription error) let main() return immediately, closing
	// the DB pool and NATS connections out from under still-running pipelines.
	var wg sync.WaitGroup

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
	if err := seed.EnsureCuratedModules(ctx, ApiTypes.ProjectDBHandle); err != nil {
		logger.Error("failed to ensure curated ontology modules", "error", err)
		os.Exit(1)
	}
	if err := ensureLLMUsageSink(); err != nil {
		logger.Error("failed to install default llm usage sink", "error", err)
		os.Exit(1)
	}

	natsURL := envOrDefault("NATS_URL", "nats://127.0.0.1:4222")
	subject := envOrDefault("DOC_PROCESSOR_EVENT_SUBJECT", docprocessing.DefaultEventSubject)
	durable := envOrDefault("DOC_PROCESSOR_EVENT_DURABLE", "doc-processor")
	streamName := envFirst("DOC_PROCESSOR_EVENT_STREAM", "EXTRACT_DOCMETA_EVENT_STREAM")
	subjects := docProcessorSubjects(subject)
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
	for _, subSubject := range subjects {
		subStream := docProcessorStream(subject, streamName, subSubject)
		if err := ns.EnsureStream(subStream, subSubject); err != nil {
			logger.Error("failed to ensure stream", "error", err, "stream", subStream, "subject", subSubject)
			os.Exit(1)
		}
	}

	// The production processor graph is shared with benchmark workers.
	runtime, err := docprocessing.NewProductionRuntime(logger)
	if err != nil {
		logger.Error("failed to build doc processor runtime", "error", err)
		os.Exit(1)
	}
	control := runtime.Control

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
		"subjects", subjects,
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

	// ── Doc-processing subscriptions (compatibility: accept both subjects) ───
	for _, subSubject := range subjects {
		subjectForHandler := subSubject
		durableForSubject := docProcessorDurable(subject, durable, subSubject)
		wg.Go(func() {
			if err := ns.Subscribe(ctx, subjectForHandler, durableForSubject, func(msgCtx context.Context, payload []byte) error {
				return control.HandleJetStreamEvent(msgCtx, subjectForHandler, payload)
			}); err != nil && ctx.Err() == nil {
				logger.Error("doc processor subscription exited with error",
					"error", err,
					"subject", subjectForHandler,
					"durable", durableForSubject,
				)
				cancel()
			}
		})
	}

	// ── Doc-review subscription ────────────────────────────────────────────────
	docReviewSubject := envOrDefault("DOC_REVIEW_EVENT_SUBJECT", docreviews.DefaultDocReviewEventSubject)
	docReviewStream := envOrDefault("DOC_REVIEW_EVENT_STREAM", "doc-review-events")
	docReviewDurable := envOrDefault("DOC_REVIEW_EVENT_DURABLE", "doc-processor-docreview")

	drNS, err := fileconverters.NewNATSSubscriber(natsURL)
	if err != nil {
		logger.Error("failed creating nats subscriber for doc-review", "error", err, "nats_url", natsURL)
		os.Exit(1)
	}
	defer drNS.Close()
	if err := drNS.EnsureStream(docReviewStream, docReviewSubject); err != nil {
		logger.Error("failed to ensure doc-review stream", "error", err, "stream", docReviewStream, "subject", docReviewSubject)
		os.Exit(1)
	}

	logger.Info("doc-review subscription starting",
		"subject", docReviewSubject,
		"stream", docReviewStream,
		"durable", docReviewDurable,
	)

	wg.Go(func() {
		if err := drNS.Subscribe(ctx, docReviewSubject, docReviewDurable, func(msgCtx context.Context, payload []byte) error {
			var evt struct {
				RequestID int64 `json:"request_id"`
				RunID     int64 `json:"run_id"`
			}
			if err := json.Unmarshal(payload, &evt); err != nil {
				return fmt.Errorf("unmarshal doc-review event: %w", err)
			}
			if evt.RequestID <= 0 || evt.RunID <= 0 {
				return fmt.Errorf("invalid doc-review event: request_id=%d run_id=%d", evt.RequestID, evt.RunID)
			}
			logger.Info("doc-review event received", "request_id", evt.RequestID, "run_id", evt.RunID)
			ctrl := docreviews.NewDocReviewController()
			ctrl.RunReviewAndReport(msgCtx, evt.RequestID, evt.RunID)
			return nil
		}); err != nil && ctx.Err() == nil {
			logger.Error("doc-review subscription exited with error", "error", err)
			cancel()
		}
	})

	// ── Recover stalled doc-review runs left by a previous process ────────────
	// A killed/restarted backend leaves requests in 'accepted'/'running' with no
	// worker driving them (findings persist only when a whole run completes, so
	// nothing partial is lost). Re-arm and re-run each from scratch. Disable with
	// DOC_REVIEW_RECOVER_ON_START=false (e.g. when running multiple instances).
	if envOrDefault("DOC_REVIEW_RECOVER_ON_START", "true") != "false" {
		wg.Go(func() {
			ctrl := docreviews.NewDocReviewController()
			stalled, err := ctrl.RecoverStalledReviews(ctx)
			if err != nil {
				logger.Error("doc-review recovery sweep failed", "error", err)
				return
			}
			for _, ref := range stalled {
				logger.Info("re-running recovered doc-review", "request_id", ref.RequestID, "run_id", ref.RunID)
				wg.Go(func() { ctrl.RunReviewAndReport(ctx, ref.RequestID, ref.RunID) })
			}
		})
	}

	<-ctx.Done()
	logger.Info("shutdown: waiting for in-flight subscriptions and pipelines to finish")
	grace := shutdownGrace()
	// Drain the subscription wrappers and the doc-processing pipelines they
	// dispatch concurrently, bounded by the same grace period, rather than
	// waiting up to 2x grace by draining them one after another.
	var subsDrained, pipelinesDrained bool
	var drainWG sync.WaitGroup
	drainWG.Go(func() { subsDrained = waitWithGrace(&wg, grace) })
	drainWG.Go(func() { pipelinesDrained = control.WaitForInFlightPipelines(grace) })
	drainWG.Wait()
	if subsDrained && pipelinesDrained {
		logger.Info("shutdown: all in-flight work finished cleanly")
	} else {
		logger.Warn("shutdown: grace period elapsed with work still in flight; closing database and connections now",
			"grace", grace.String(), "subscriptions_drained", subsDrained, "pipelines_drained", pipelinesDrained)
	}
	fmt.Println("doc processor stopped")
}

// shutdownGrace returns how long main() waits for in-flight subscriptions and
// pipelines to finish after ctx is canceled, before the deferred DB/connection
// cleanup runs. Shares the same knob as NATSSubscriber's own internal drain
// (fileconverters.envDurationSeconds default) so both layers agree on budget.
func shutdownGrace() time.Duration {
	raw := strings.TrimSpace(envFirst("DOC_PROCESSOR_SHUTDOWN_GRACE_SEC"))
	if raw == "" {
		return 5 * time.Minute
	}
	sec, err := strconv.Atoi(raw)
	if err != nil || sec <= 0 {
		return 5 * time.Minute
	}
	return time.Duration(sec) * time.Second
}

// waitWithGrace blocks until wg is fully drained or grace elapses, whichever
// comes first. Returns true if wg drained within the grace period.
func waitWithGrace(wg *sync.WaitGroup, grace time.Duration) bool {
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return true
	case <-time.After(grace):
		return false
	}
}
