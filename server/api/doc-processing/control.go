package docprocessing

import (
	"bufio"
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/chendingplano/deepdoc/server/api/ontology/policyaudit"
	"github.com/chendingplano/deepdoc/server/api/ontology/semrules"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

type Processor interface {
	Name() string
	HandleEvent(ctx context.Context, payload []byte) error
}

type logNamedProcessor interface {
	LogName() string
}

type ControlService struct {
	Processors        []Processor
	BlockingProcessor Processor // always runs before Processors, regardless of requested operations
	Logger            ApiTypes.JimoLogger
	InputStore        DocMetadataStore
	EventStore        EventStore
	StopStore         StopRequestStore
	RunStore          DocProcessRunStore
	PlanStore         DocProcessPlanStore
	FacetStore        DocFacetStore
	// Facets persists tier-1/tier-2 facet observations (spec 2026072901
	// S16.1 "Facet tiers 1-2"). Deliberately independent of Resolver.Facets:
	// Resolver is nil whenever the tier-3 LLM classifier isn't configured
	// (buildProductionResolver, runtime.go -- degrades to nil on missing
	// model config/prompt), and tier 1 must not be accidentally gated
	// behind tier 3's configuration -- that would invert DR4's "cheapest
	// first" economics, coupling the free tier to the LLM tier's
	// availability.
	Facets FacetObservationStore
	// RoutingClearances/RoutingAlarms/PolicyAudit are E3's clearance-aware
	// enforcement seams. All three are optional (nil-safe): a nil
	// RoutingClearances means every suppressive routing decision stays
	// shadow-only (fail closed, never enforced); a nil RoutingAlarms/
	// PolicyAudit simply skips persistence of alarms/events.
	RoutingClearances RoutingClearanceChecker
	RoutingAlarms     RoutingAlarmWriter
	PolicyAudit       policyaudit.Writer
	// Resolver is the optional two-pass applicability resolver (P5 spec
	// section 7). When non-nil, handleEvent calls ResolveExtractionFacts
	// after resolveProductionPlanFacts to enrich the fact set with
	// tier-3 classifier observations before pipeline binding and processor
	// gate evaluation. When nil, behavior is unchanged (base facts only).
	Resolver *ApplicabilityResolver
	Now      func() time.Time

	DocProcessorMode       string
	MaxDocProcessPipelines int
	pipelineSlots          chan struct{}
	pipelineSlotsMax       int
	pipelineSlotsMu        sync.Mutex

	// inFlightPipelines tracks the detached goroutines HandleJetStreamEvent
	// dispatches (it returns before the pipeline runs, so message-handler
	// completion alone doesn't reflect pipeline completion). Shutdown code
	// should wait on this via WaitForInFlightPipelines before closing the
	// DB pool the pipelines still write to.
	inFlightPipelines sync.WaitGroup
}

// WaitForInFlightPipelines blocks until every pipeline dispatched via
// HandleJetStreamEvent has finished, or grace elapses, whichever comes
// first. Returns true if everything drained within the grace period.
func (s *ControlService) WaitForInFlightPipelines(grace time.Duration) bool {
	done := make(chan struct{})
	go func() {
		s.inFlightPipelines.Wait()
		close(done)
	}()
	select {
	case <-done:
		return true
	case <-time.After(grace):
		return false
	}
}

func (s *ControlService) HandleEvent(ctx context.Context, payload []byte) {
	s.handleEvent(ctx, payload)
}

// RunEvent processes a payload synchronously through the production event path.
// It is the embeddable counterpart to HandleEvent for callers that need the
// resulting error rather than fire-and-forget behavior.
func (s *ControlService) RunEvent(ctx context.Context, payload []byte) error {
	return s.handleEvent(ctx, payload)
}

func (s *ControlService) HandleJetStreamEvent(ctx context.Context, subject string, payload []byte) error {
	eventID := ""
	if s.EventStore != nil {
		id, err := s.insertReceivedEvent(ctx, subject, payload)
		if err != nil {
			if s.Logger != nil {
				s.Logger.Error("failed inserting kb.events received record", "subject", subject, "error", err)
			}
		} else {
			eventID = id
		}
	}

	releaseSlot, err := s.acquirePipelineSlot(ctx)
	if err != nil {
		return err
	}
	s.inFlightPipelines.Go(func() {
		defer releaseSlot()
		procStart := s.now()
		var procErr error
		procCtx := withEventID(ctx, eventID)
		if strings.TrimSpace(subject) == DefaultEventSubject {
			procErr = s.handleDefaultSubjectEvent(procCtx, payload)
		} else {
			var spanEnd func(error)
			if evt, err := ParseLineFileGeneratedEvent(payload); err == nil {
				procCtx, spanEnd = s.startPipelineTrace(procCtx, subject, eventID, evt)
				defer func() { spanEnd(procErr) }()
			}
			procErr = s.handleEvent(procCtx, payload)
		}
		if s.EventStore == nil || strings.TrimSpace(eventID) == "" {
			return
		}
		if err := s.EventStore.UpsertConsumedStatus(context.Background(), eventID, procStart, time.Since(procStart).Milliseconds(), procErr); err != nil && s.Logger != nil {
			s.Logger.Error("failed upserting kb.events consumed status", "event_id", eventID, "error", err)
		}
	})
	return nil
}

const (
	DocProcessorModeAuto = "auto"
	DocProcessorModeDev  = "dev"
)

func DocProcessorModeFromEnv() (string, error) {
	return normalizeDocProcessorMode(os.Getenv("DOC_PROCESSOR_MODE"))
}

func normalizeDocProcessorMode(raw string) (string, error) {
	mode := strings.ToLower(strings.TrimSpace(raw))
	if mode == "" {
		return DocProcessorModeAuto, nil
	}
	switch mode {
	case DocProcessorModeAuto, DocProcessorModeDev:
		return mode, nil
	default:
		return "", fmt.Errorf("invalid DOC_PROCESSOR_MODE %q; allowed values are %q and %q", raw, DocProcessorModeAuto, DocProcessorModeDev)
	}
}

func (s *ControlService) docProcessorMode() (string, error) {
	if strings.TrimSpace(s.DocProcessorMode) != "" {
		return normalizeDocProcessorMode(s.DocProcessorMode)
	}
	return DocProcessorModeFromEnv()
}

func (s *ControlService) handleDefaultSubjectEvent(ctx context.Context, payload []byte) error {
	mode, err := s.docProcessorMode()
	if err != nil {
		if s.Logger != nil {
			s.Logger.Error("invalid doc processor mode", "error", err)
		}
		return err
	}
	switch mode {
	case DocProcessorModeAuto:
		return s.handleEvent(ctx, payload)
	case DocProcessorModeDev:
		return s.HandleStartDocProcessingEvent(ctx, payload)
	default:
		return fmt.Errorf("invalid doc processor mode %q", mode)
	}
}

type parsedInputRecordLister interface {
	ListParsedInputRecords(ctx context.Context) ([]DocMetadataInputRecord, error)
}

type failedDocProcessorRecordLister interface {
	ListRecordsWithFailedDocProcessors(ctx context.Context) ([]DocMetadataInputRecord, error)
}

func (s *ControlService) HandleStartDocProcessingEvent(ctx context.Context, payload []byte) error {
	cmd, err := ParseStartDocProcessingEvent(payload)
	if err != nil {
		if s.Logger != nil {
			s.Logger.Error("doc processor failed parsing start command", "error", err)
		}
		return err
	}
	records, err := s.resolveStartDocProcessingRecords(ctx, cmd)
	if err != nil {
		return err
	}
	var (
		firstErr error
		mu       sync.Mutex
		wg       sync.WaitGroup
	)

	// Use a detached context so goroutines waiting for a pipeline slot
	// don't time out when the parent context has a short deadline.
	slotCtx := context.WithoutCancel(ctx)

	for _, rec := range records {
		ops := append([]string(nil), cmd.DocProcessors...)
		if len(ops) == 0 && cmd.FailedProcOnly {
			ops = failedDocProcessorNames(rec.StatusRaw)
		} else if cmd.FailedProcOnly {
			ops = intersectProcessorNames(ops, failedDocProcessorNames(rec.StatusRaw))
		}
		if cmd.FailedProcOnly && len(ops) == 0 {
			if s.Logger != nil {
				s.Logger.Info("doc processor skipped record, no failed processors selected",
					"record_id", rec.ID,
					"requested_processors", cmd.DocProcessors,
				)
			}
			continue
		}
		eventPayload, err := buildLineFileGeneratedPayload(rec.ID, cmd.Filename, ops)
		if err != nil {
			mu.Lock()
			if firstErr == nil {
				firstErr = err
			}
			mu.Unlock()
			continue
		}

		// Launch each document's pipeline in its own goroutine, limited by
		// MaxDocProcessPipelines via the semaphore returned by acquirePipelineSlot.
		wg.Add(1)
		go func() {
			defer wg.Done()

			releaseSlot, slotErr := s.acquirePipelineSlot(slotCtx)
			if slotErr != nil {
				mu.Lock()
				if firstErr == nil {
					firstErr = slotErr
				}
				mu.Unlock()
				return
			}
			defer releaseSlot()

			if err := s.handleEvent(ctx, eventPayload); err != nil {
				mu.Lock()
				if firstErr == nil {
					firstErr = err
				}
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	return firstErr
}

func (s *ControlService) resolveStartDocProcessingRecords(ctx context.Context, cmd StartDocProcessingEvent) ([]DocMetadataInputRecord, error) {
	switch cmd.All {
	case "parsed":
		lister, ok := s.InputStore.(parsedInputRecordLister)
		if !ok {
			return nil, errors.New("input store does not support listing parsed records")
		}
		return lister.ListParsedInputRecords(ctx)
	case "failed-procs", "with-failed-procs":
		lister, ok := s.InputStore.(failedDocProcessorRecordLister)
		if !ok {
			return nil, errors.New("input store does not support listing records with failed doc processors")
		}
		return lister.ListRecordsWithFailedDocProcessors(ctx)
	}
	records := make([]DocMetadataInputRecord, 0, len(cmd.RecordIDs))
	for _, id := range cmd.RecordIDs {
		rec := DocMetadataInputRecord{ID: id}
		if s.InputStore != nil {
			loaded, err := s.InputStore.GetInputRecord(ctx, id)
			if err != nil {
				return nil, fmt.Errorf("load kb.inputs record %d: %w", id, err)
			}
			rec = loaded
		}
		records = append(records, rec)
	}
	return records, nil
}

func buildLineFileGeneratedPayload(recordID int64, filename string, operations []string) ([]byte, error) {
	body := map[string]any{"record_id": strconv.FormatInt(recordID, 10)}
	if strings.TrimSpace(filename) != "" {
		body["filename"] = strings.TrimSpace(filename)
	}
	if len(operations) > 0 {
		body["operation"] = operations
	}
	return json.Marshal(body)
}

func intersectProcessorNames(requested []string, failed []string) []string {
	failedSet := make(map[string]struct{}, len(failed))
	for _, name := range failed {
		if key := canonicalOperationName(name); key != "" {
			failedSet[key] = struct{}{}
		}
	}
	out := make([]string, 0, len(requested))
	seen := map[string]struct{}{}
	for _, name := range requested {
		key := canonicalOperationName(name)
		if key == "" {
			continue
		}
		if _, ok := failedSet[key]; !ok {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, key)
	}
	return out
}

func failedDocProcessorNames(statusRaw string) []string {
	entries := decodeDocMetaStatus(statusRaw)
	out := make([]string, 0, len(entries))
	seen := map[string]struct{}{}
	for _, entry := range entries {
		if statusValue(entry) != "failed" {
			continue
		}
		op := canonicalOperationName(asString(entry["operation"]))
		if !isDocProcessorStatusOperation(op) {
			continue
		}
		if _, ok := seen[op]; ok {
			continue
		}
		seen[op] = struct{}{}
		out = append(out, op)
	}
	return out
}

func (s *ControlService) resetProcessorStatuses(ctx context.Context, recordID int64, processors []Processor) {
	if s.InputStore == nil || recordID <= 0 || len(processors) == 0 {
		return
	}
	names := make([]string, 0, len(processors))
	for _, p := range processors {
		if p == nil {
			continue
		}
		if name := canonicalOperationName(p.Name()); isDocProcessorStatusOperation(name) {
			names = append(names, name)
		}
	}
	if len(names) == 0 {
		return
	}
	if err := updateInputStatusAtomic(ctx, s.InputStore, recordID, func(current string) (DocMetadataUpdate, error) {
		statusRaw, err := resetRequestedProcessorStatuses(current, names)
		if err != nil {
			return DocMetadataUpdate{}, err
		}
		return DocMetadataUpdate{StatusRaw: statusRaw}, nil
	}); err != nil && s.Logger != nil {
		s.Logger.Error("failed resetting selected doc processor statuses", "record_id", recordID, "processors", names, "error", err)
	}
}

func resetRequestedProcessorStatuses(raw string, requested []string) (string, error) {
	targets := make(map[string]struct{}, len(requested))
	for _, name := range requested {
		for _, alias := range processorStatusAliases(name) {
			targets[alias] = struct{}{}
		}
	}
	if len(targets) == 0 {
		if strings.TrimSpace(raw) == "" {
			return "[]", nil
		}
		return raw, nil
	}

	entries := decodeDocMetaStatus(raw)
	out := make([]map[string]any, 0, len(entries))
	for _, entry := range entries {
		op := canonicalOperationName(asString(entry["operation"]))
		if _, ok := targets[op]; ok {
			continue
		}
		out = append(out, entry)
	}

	bs, err := json.Marshal(out)
	if err != nil {
		return "", err
	}
	return string(bs), nil
}

func processorStatusAliases(name string) []string {
	key := canonicalOperationName(name)
	switch key {
	case "chunking":
		return []string{"chunking", "chunked", "topic_chunking", "fix_size_chunking"}
	case "static_analyzer":
		return []string{"static_analyzer", "static_analzyer"}
	case "structure_analyzer":
		return []string{"structure_analyzer"}
	case "extract_doc_metadata":
		return []string{"extract_doc_metadata", "extract_metadata"}
	case "generate_scene_blocks":
		return []string{"generate_scene_blocks", "extract_scene_blocks"}
	case "extract_semantic_projections":
		return []string{"extract_semantic_projections", "extract_semantic_projection"}
	case "extract_entity":
		return []string{"extract_entity", "extract_entity_relation"}
	case "extract_relation":
		return []string{"extract_relation", "extract_entity_relation"}
	case "extract_inventory_items":
		return []string{"extract_inventory_items"}
	case "extract_structured_knowledge":
		return []string{"extract_structured_knowledge", "extract_structured_knowledges"}
	default:
		if key == "" {
			return nil
		}
		return []string{key}
	}
}

func primaryProcessorStatusOperation(name string) string {
	switch canonicalOperationName(name) {
	case "chunking":
		return "chunked"
	case "static_analyzer":
		return "static_analzyer"
	case "extract_doc_metadata":
		return "extract_metadata"
	case "generate_scene_blocks":
		return "extract_scene_blocks"
	default:
		return canonicalOperationName(name)
	}
}

func upsertProcessorRuntimeStatus(raw string, now time.Time, processorName string, procStatus string, progress string) (string, error) {
	status := strings.ToLower(strings.TrimSpace(procStatus))
	if status == "" {
		status = "active"
	}
	operation := primaryProcessorStatusOperation(processorName)
	if operation == "" {
		return raw, nil
	}
	aliases := processorStatusAliases(processorName)
	if len(aliases) == 0 {
		aliases = []string{operation}
	}
	aliasSet := make(map[string]struct{}, len(aliases))
	for _, alias := range aliases {
		if key := canonicalOperationName(alias); key != "" {
			aliasSet[key] = struct{}{}
		}
	}

	entry := map[string]any{
		"operation":   operation,
		"start_time":  now.Format(defaultDocMetaStatusTime),
		"proc_status": status,
	}
	if trimmedProgress := strings.TrimSpace(progress); trimmedProgress != "" {
		entry["progress"] = trimmedProgress
	}

	entries := decodeDocMetaStatus(raw)
	replaced := false
	out := make([]map[string]any, 0, len(entries)+1)
	for _, e := range entries {
		if _, ok := aliasSet[canonicalOperationName(asString(e["operation"]))]; !ok {
			out = append(out, e)
			continue
		}
		if !replaced {
			if originalStart := strings.TrimSpace(asString(e["start_time"])); originalStart != "" {
				entry["start_time"] = originalStart
			}
			out = append(out, entry)
			replaced = true
		}
	}
	if !replaced {
		out = append(out, entry)
	}

	bs, err := json.Marshal(out)
	if err != nil {
		return "", err
	}
	return string(bs), nil
}

func statusValue(entry map[string]any) string {
	return strings.ToLower(strings.TrimSpace(firstNonEmptyTrimmed(
		asString(entry["proc_status"]),
		asString(entry["status"]),
	)))
}

func isDocProcessorStatusOperation(op string) bool {
	switch canonicalOperationName(op) {
	case "", "parsed", "parsing", "converted", "converting", "doc_processing", "doc_processor":
		return false
	default:
		return true
	}
}

func (s *ControlService) startPipelineTrace(ctx context.Context, subject string, eventID string, evt LineFileGeneratedEvent) (context.Context, func(error)) {
	ctx, span := startPipelineSpan(ctx, subject, eventID, evt)
	return ctx, func(err error) {
		status := "success"
		if err != nil {
			status = "failed"
		}
		span.SetAttributes(attribute.String("pipeline.status", status))
		endSpanWithStatus(span, status, err)
		span.End()
	}
}

func (s *ControlService) acquirePipelineSlot(ctx context.Context) (func(), error) {
	limit := s.maxDocProcessPipelines()
	if limit <= 0 {
		return func() {}, nil
	}
	s.pipelineSlotsMu.Lock()
	if s.pipelineSlots == nil || s.pipelineSlotsMax != limit {
		s.pipelineSlots = make(chan struct{}, limit)
		s.pipelineSlotsMax = limit
	}
	slots := s.pipelineSlots
	s.pipelineSlotsMu.Unlock()

	select {
	case slots <- struct{}{}:
		return func() { <-slots }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (s *ControlService) maxDocProcessPipelines() int {
	if s.MaxDocProcessPipelines > 0 {
		return s.MaxDocProcessPipelines
	}
	return MaxDocProcessPipelinesFromEnv()
}

// RunDocProcessorConcurrentFromEnv reports whether Phase B processors run
// concurrently. Defaults to true; set RUN_DOC_PROCESSOR_CONCURRENT=false to
// fall back to the sequential pipeline.
func RunDocProcessorConcurrentFromEnv() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("RUN_DOC_PROCESSOR_CONCURRENT")))
	return v != "false"
}

func MaxDocProcessPipelinesFromEnv() int {
	raw := strings.TrimSpace(os.Getenv("MAX_DOC_PROCESS_PIPELINES"))
	if raw == "" {
		return 10
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return 10
	}
	return n
}

func (s *ControlService) handleEvent(ctx context.Context, payload []byte) error {
	requestStart := s.now()
	processors := s.Processors
	evt, err := ParseLineFileGeneratedEvent(payload)
	if err != nil {
		if s.Logger != nil {
			s.Logger.Error("doc processor failed parsing event", "error", err)
			s.Logger.Info("finish processing request",
				"record_id", int64(0),
				"proc_status", "failed",
				"ms_used", time.Since(requestStart).Milliseconds(),
			)
		}
		return err
	}
	if s.Logger != nil {
		s.Logger.Info("start processing request",
			"record_id", evt.RecordID,
			"operations", evt.Operations,
		)
	}

	if skipReason := skipReasonLineFileGeneratedEvent(evt); skipReason != "" {
		if s.Logger != nil {
			s.Logger.Info("doc processor skipped event",
				"record_id", evt.RecordID,
				"reason", skipReason,
				"type", evt.Type,
				"status", evt.Status,
				"filename", evt.Filename,
				"operations", evt.Operations,
			)
			s.Logger.Info("finish processing request",
				"record_id", evt.RecordID,
				"proc_status", "skipped",
				"ms_used", time.Since(requestStart).Milliseconds(),
			)
		}
		return nil
	}
	if !s.preflightInput(ctx, evt) {
		if s.Logger != nil {
			s.Logger.Info("finish processing request",
				"record_id", evt.RecordID,
				"proc_status", "failed",
				"ms_used", time.Since(requestStart).Milliseconds(),
			)
		}
		return errors.New("(MID_26042404) preflight input failed")
	}
	// ADR 2026081001 DR3: the "active policy pointer that can fail to load"
	// failure mode (spec 2026080102 section 11) no longer exists -- bindings/
	// rules are read from the in-process cache loaded at startup, not a
	// per-event policy-store lookup, so resolveProductionPlanFacts no longer
	// has a distinguishable hard-fail error to check for here.
	planFacts, factsErr := s.resolveProductionPlanFacts(ctx, evt)
	if factsErr != nil && s.Logger != nil {
		s.Logger.Warn("failed resolving production plan facts", "record_id", evt.RecordID, "error", factsErr)
	}
	if factsErr != nil {
		planFacts = ProductionPlanFacts{RequestedProcessors: append([]string(nil), evt.Operations...)}
	} else {
		s.persistDocFacets(ctx, evt.RecordID, planFacts)
		// Tier-1 deterministic facets (spec 2026072901 S16.1 "Facet tiers
		// 1-2"): the line file static_analyzer wrote is guaranteed available
		// at this point (this handler runs on LineFileGeneratedEvent), so
		// this can run unconditionally as far as its own inputs go. Gated on
		// s.Facets, not s.Resolver -- tier 1 has no LLM dependency and must
		// not be coupled to whether the tier-3 classifier happens to be
		// configured (s.Resolver is nil whenever buildProductionResolver
		// couldn't construct one, e.g. no classifier model configured in
		// this environment). Independently, facetTier1GatedOff consults an
		// authored kb.pipeline_rules row (target_processor="facet_tier1")
		// so the processor can be individually disabled for debugging,
		// testing, or bug fixing without touching s.Facets/s.Resolver wiring;
		// a gate-resolution error fails open (runs) rather than silently
		// dropping load-bearing facets over a misconfigured rule.
		if s.Facets != nil {
			skipTier1, gateErr := facetTier1GatedOff(semrules.FactSet{})
			if gateErr != nil && s.Logger != nil {
				s.Logger.Warn("facet_tier1 gate resolution failed, running unconditionally", "record_id", evt.RecordID, "error", gateErr)
			}
			if skipTier1 {
				if s.Logger != nil {
					s.Logger.Info("facet_tier1 skipped by processor gate", "record_id", evt.RecordID)
				}
			} else if tier1, err := ComputeTier1Facets(ctx, s.InputStore, evt.RecordID); err != nil {
				if s.Logger != nil {
					s.Logger.Warn("tier-1 facet computation failed, continuing without it", "record_id", evt.RecordID, "error", err)
				}
			} else {
				// InsertFacetObservation requires a non-empty DecisionAttemptID/
				// InvocationID (validated before the SQL, doc_facet_store.go) --
				// reuse the same per-attempt key the resolver call below uses,
				// tier1-prefixed so it reads distinctly in kb.doc_facet_values
				// even though path already disambiguates it from tier 3.
				attemptKey := resolverAttemptKey(evt)
				for _, obs := range tier1 {
					obs.DecisionAttemptID = "tier1-" + attemptKey
					obs.InvocationID = fmt.Sprintf("tier1-%d-%s", evt.RecordID, attemptKey)
					if _, err := s.Facets.InsertFacetObservation(ctx, obs); err != nil && s.Logger != nil {
						s.Logger.Warn("tier-1 facet persist failed", "record_id", evt.RecordID, "path", obs.Path, "error", err)
					}
				}
			}
		}
		if s.Resolver != nil {
			// P5 two-pass resolver: enrich tier-3 facts before binding/gate
			// eval. ResolveExtractionFacts also merges tiers 1-2 internally
			// (enrichWithPersistedFacets) before ever considering tier 3.
			enrichedFacts, _, resolverErr := s.Resolver.ResolveExtractionFacts(ctx, planFacts, evt.RecordID, resolverAttemptKey(evt), boundedDocumentSample(evt, planFacts))
			if resolverErr != nil && s.Logger != nil {
				s.Logger.Warn("applicability resolver failed, continuing with base facts", "record_id", evt.RecordID, "error", resolverErr)
			}
			if resolverErr == nil {
				planFacts.EnrichedFacts = enrichedFacts
				planFacts.RoutingFacets.DocKind = documentKindFromEnrichedFacts(enrichedFacts)
			}
		} else if s.Facets != nil {
			// No tier-3 resolver configured, but tiers 1-2 must still reach
			// routing/gate predicates -- mirrors ResolveExtractionFacts'
			// enrichWithPersistedFacets without needing a full
			// ApplicabilityResolver (which also requires a Classifier).
			planFacts.EnrichedFacts = mergeTier12Facts(ctx, s.Facets, evt.RecordID, BuildPipelineBindingFactSet(planFacts))
		}
	}
	plan, planErr := BuildProductionProcessorPlanFromFacts(planFacts)
	if planErr != nil {
		if s.Logger != nil {
			s.Logger.Warn("failed building production processor plan", "record_id", evt.RecordID, "error", planErr)
		}
		alarm := AlarmForPlanError(planErr)
		// RunID is unavailable here -- no kb.doc_process_runs row exists yet
		// at this call site -- so RecordID is the dedup correlator instead
		// (stable across redeliveries/retries of the same record; see
		// RoutingAlarmSQLWriter.WriteAlarm).
		alarm.RecordID = evt.RecordID
		s.raiseRoutingAlarms(ctx, []RoutingAlarm{alarm})
		s.emitAlarmAuditEvent(ctx, evt.RecordID, 0, plan.PipelineSpec().Name, plan.PipelineSpec().Version, alarm)
		if len(evt.Operations) == 0 && IsDecisionRelevantPlanConflict(planErr) {
			// A decision-relevant pipeline-binding/processor-gate conflict
			// (DR7 indeterminacy/conflict, not a structural/configuration
			// error such as an unknown pipeline or processor name) is
			// block-mode today (ResolveProductionPipelineBinding hardcodes
			// PipelineBindingOnConflictBlock), so it must fail before any
			// processor runs, per spec 2026080102 section 11 ("fail before
			// processors run"). An explicit evt.Operations request bypasses
			// policy entirely (existing precedence) and is unaffected; a
			// non-decision-relevant plan-build failure keeps the prior
			// alarm-and-continue behavior below.
			if s.Logger != nil {
				s.Logger.Info("finish processing request",
					"record_id", evt.RecordID,
					"proc_status", "failed",
					"ms_used", time.Since(requestStart).Milliseconds(),
				)
			}
			return fmt.Errorf("(MID_26080201) decision-relevant routing conflict blocked processing: %w", planErr)
		}
	}
	if len(evt.Operations) > 0 {
		processors = s.selectProcessors(evt.Operations)
		processors = s.skipSatisfiedAutoDependencies(ctx, evt, processors)
		if len(processors) == 0 {
			if s.Logger != nil {
				allowed := make([]string, 0, len(s.Processors))
				for _, p := range s.Processors {
					allowed = append(allowed, p.Name())
				}
				s.Logger.Info("doc processor skipped, no matching operation", "requested", evt.Operations, "allowed", allowed)
				s.Logger.Info("finish processing request",
					"record_id", evt.RecordID,
					"proc_status", "skipped",
					"ms_used", time.Since(requestStart).Milliseconds(),
				)
			}
			return nil
		}
	}
	var routingResult RoutingEnforcementResult
	var routingFinalized bool
	if planErr == nil && len(evt.Operations) == 0 {
		routingResult, err = s.finalizeRoutingPlan(ctx, plan)
		if err != nil {
			if s.Logger != nil {
				s.Logger.Warn("failed finalizing routing enforcement, falling back to legacy policy exclusions", "record_id", evt.RecordID, "error", err)
			}
			processors = s.applyPlanEnforcement(processors, plan.ExcludedByPolicy())
		} else {
			routingFinalized = true
			processors = s.applyPlanEnforcement(processors, routingResult.ExcludedProcessorNames())
			// Alarms/events for routingResult are raised after runID is
			// known (below) so they carry a RunID for per-run dedup
			// (spec 2026080102 section 11) instead of firing unscoped here.
		}
	}
	var runID int64
	var runCreated bool
	if s.RunStore != nil {
		mode, modeErr := s.docProcessorMode()
		if modeErr != nil {
			mode = DocProcessorModeAuto
		}
		processorNames := make([]string, 0, len(processors))
		for _, p := range processors {
			if p == nil {
				continue
			}
			processorNames = append(processorNames, p.Name())
		}
		parameters := map[string]any{
			"force":       evt.Force,
			"force_clear": evt.ForceClear,
		}
		if len(evt.Operations) > 0 {
			parameters["operations"] = evt.Operations
			parameters["processor_override_bypasses_policy"] = true
		}
		if strings.TrimSpace(evt.PipelineOverride) != "" {
			parameters["pipeline_override"] = strings.TrimSpace(evt.PipelineOverride)
		}
		if strings.TrimSpace(evt.Filename) != "" {
			parameters["filename"] = evt.Filename
		}
		parameters["processor_plan_facts"] = planFacts
		if planErr == nil {
			parameters["processor_routing_snapshot"] = plan.RoutingSnapshot()
			parameters["processor_plan_steps"] = plan.Steps()
			parameters["processor_pipeline_binding"] = plan.PipelineBinding()
			parameters["processor_pipeline_selection"] = plan.PipelineSelection()
			parameters["processor_pipeline_spec"] = plan.PipelineSpec()
			if excluded := plan.ExcludedByPolicy(); len(excluded) > 0 {
				parameters["processor_excluded_by_policy"] = excluded
			}
		}
		id, createErr := s.RunStore.CreateDocProcessRun(ctx, DocProcessRunRecord{
			RecordID:   evt.RecordID,
			EventID:    eventIDFromContext(ctx),
			Mode:       mode,
			Processors: processorNames,
			Parameters: parameters,
		})
		if createErr != nil {
			if s.Logger != nil {
				s.Logger.Warn("failed to create kb.doc_process_runs row", "record_id", evt.RecordID, "error", createErr)
			}
		} else {
			runID = id
			runCreated = true
			ctx = withRunID(ctx, runID)
			if s.PlanStore != nil && planErr == nil {
				if _, persistErr := s.PlanStore.CreateDocProcessPlan(ctx, DocProcessPlanRecord{
					RunID:             runID,
					RecordID:          evt.RecordID,
					PlanFacts:         plan.Facts(),
					PlanSteps:         plan.Steps(),
					PipelineBinding:   plan.PipelineBinding(),
					PipelineSelection: plan.PipelineSelection(),
					PipelineSpec:      plan.PipelineSpec(),
					ExcludedByPolicy:  plan.ExcludedByPolicy(),
					RoutingSnapshot:   plan.RoutingSnapshot(),
				}); persistErr != nil {
					if s.Logger != nil {
						s.Logger.Warn("failed to create kb.doc_process_plans row", "run_id", runID, "record_id", evt.RecordID, "error", persistErr)
					}
				}
			}
		}
	}
	if routingFinalized {
		scopedAlarms := make([]RoutingAlarm, len(routingResult.Alarms))
		copy(scopedAlarms, routingResult.Alarms)
		for i := range scopedAlarms {
			scopedAlarms[i].RunID = runID
			scopedAlarms[i].RecordID = evt.RecordID
		}
		s.raiseRoutingAlarms(ctx, scopedAlarms)
		for _, alarm := range DedupeRoutingAlarms(scopedAlarms) {
			s.emitAlarmAuditEvent(ctx, evt.RecordID, runID, plan.PipelineSpec().Name, plan.PipelineSpec().Version, alarm)
		}
		s.recordRoutingDecisionEvents(ctx, evt.RecordID, runID, plan.PipelineSpec().Name, plan.PipelineSpec().Version, routingResult)
	}
	if len(processors) > 0 {
		s.resetProcessorStatuses(ctx, evt.RecordID, processors)
	}
	// Inject buffer holders so blocking and chunking processors can share
	// their output with downstream processors via context.
	ctx, _ = withBlockBufferHolder(ctx)
	ctx, _ = withChunkBufferHolder(ctx)

	// Clear any stale stop_requested flag left over from a previous run that was
	// killed before the deferred ClearStopRequested could execute. Without this,
	// pollForStop would cancel the new context within 1 s of startup.
	if s.StopStore != nil {
		if err := s.StopStore.ClearStopRequested(ctx, evt.RecordID); err != nil && s.Logger != nil {
			s.Logger.Warn("failed to clear stale stop_requested flag at pipeline start", "record_id", evt.RecordID, "error", err)
		}
	}

	// Wrap with a cancellable context so a user stop request can interrupt
	// in-flight LLM calls. pollForStop cancels the context (with cause
	// ErrPipelineStopped) when it detects the stop flag in the database.
	if s.StopStore != nil {
		var cancelCause context.CancelCauseFunc
		ctx, cancelCause = context.WithCancelCause(ctx)
		defer cancelCause(nil) // ensures polling goroutine exits when pipeline completes
		go s.pollForStop(ctx, evt.RecordID, cancelCause)
	}

	requestFailed := false
	requestStopped := false
	var firstErr error
	s.persistPipelineStatusWithPlan(ctx, evt.RecordID, "running", "", nil, planFacts, plan.Steps(), plan.PipelineSelection(), plan.PipelineBinding(), plan.PipelineSpec(), plan.ExcludedByPolicy())
	defer func() {
		status := "success"
		if requestStopped {
			status = "stopped"
		} else if requestFailed {
			status = "failed"
		}
		s.persistPipelineStatusWithPlan(context.Background(), evt.RecordID, status, "", firstErr, ProductionPlanFacts{}, nil, ProductionPipelineSelection{}, ProductionPipelineBindingResolution{}, ProductionPipelineSpec{}, nil)
		if requestStopped && s.StopStore != nil {
			_ = s.StopStore.ClearStopRequested(context.Background(), evt.RecordID)
		}
		if runCreated && s.RunStore != nil {
			var errMsg *string
			if firstErr != nil {
				msg := firstErr.Error()
				errMsg = &msg
			}
			if closeErr := s.RunStore.CloseDocProcessRun(context.Background(), runID, status, errMsg); closeErr != nil && s.Logger != nil {
				s.Logger.Warn("failed to close kb.doc_process_runs row", "run_id", runID, "error", closeErr)
			}
		}
	}()

	var allProcResults []procResult

	// Always run the blocking processor first, regardless of requested operations.
	if s.BlockingProcessor != nil {
		s.runSingleProcessor(ctx, payload, s.BlockingProcessor, evt.RecordID, &requestFailed, &firstErr, &allProcResults)
	}

	if RunDocProcessorConcurrentFromEnv() {
		s.runPhaseBProcessors(ctx, payload, processors, evt.RecordID, &requestFailed, &requestStopped, &firstErr, &allProcResults)
	} else {
		s.runProcessorsSequential(ctx, payload, processors, evt.RecordID, &requestFailed, &requestStopped, &firstErr, &allProcResults)
	}
	if requestStopped {
		return nil
	}

	// Phase C can legitimately run after every direct processor status is final.
	// Clear the per-processor marker before post-processing so the list API's
	// stuck-pipeline auto-heal path does not mistake a live Phase C run for a
	// crashed coordinator.
	s.persistPipelineStatusWithPlan(ctx, evt.RecordID, "running", "", nil, ProductionPlanFacts{}, nil, ProductionPipelineSelection{}, ProductionPipelineBindingResolution{}, ProductionPipelineSpec{}, nil)

	// Phase C (post-process): now that every doc processor has finished, index the
	// artifacts of the invoked processors that defer indexing to this phase. This is the
	// only place cross-artifact indexing (e.g. metrics) may run, so it sees all outputs.
	// DR8's Phase D stages (normalize_assertions, associate_semantics,
	// project_semantics) run here too, as PostProcessIndexer processors
	// ordered after the rest via PostProcessDependsOn -- see phase_d.go.
	// Inert unless SEMANTIC_ASSOCIATION_ENABLED, and only invoked at all if
	// present in this run's processors (routed, per ADR §8.2).
	s.runPostProcessIndexing(ctx, processors, evt.RecordID)

	pipelineMSUsed := time.Since(requestStart).Milliseconds()
	status := "success"
	if requestFailed {
		status = "failed"
	}
	if s.Logger != nil {
		s.Logger.Info("finish processing request",
			"record_id", evt.RecordID,
			"proc_status", status,
			"ms_used", pipelineMSUsed,
		)
	}
	s.logPipelineFinish(context.Background(), evt.RecordID, pipelineMSUsed, allProcResults)
	return firstErr
}

func (s *ControlService) skipSatisfiedAutoDependencies(ctx context.Context, evt LineFileGeneratedEvent, processors []Processor) []Processor {
	if len(processors) == 0 || !requestedOperationsNeedAutoChunking(evt.Operations) || s.InputStore == nil {
		return processors
	}
	rec, err := s.InputStore.GetInputRecord(ctx, evt.RecordID)
	if err != nil || !docProcessorSucceeded(rec.StatusRaw, "chunking") {
		return processors
	}
	explicit := make(map[string]struct{}, len(evt.Operations))
	for _, op := range evt.Operations {
		if key := canonicalOperationName(op); key != "" {
			explicit[key] = struct{}{}
		}
	}
	pruned := make([]Processor, 0, len(processors))
	for _, p := range processors {
		name := canonicalOperationName(p.Name())
		if _, ok := explicit[name]; ok {
			pruned = append(pruned, p)
			continue
		}
		if name == "static_analyzer" || name == "chunking" {
			continue
		}
		pruned = append(pruned, p)
	}
	return pruned
}

// PostProcessIndexer is implemented by processors that index their artifacts in
// Phase C (post-process) rather than inside HandleEvent. Phase C runs once, after every
// doc processor in the pipeline has finished, so cross-artifact indexing (for example
// metrics, which links to semantic_projections, topics, scenes, provisions, entities and
// inventory items) can see every processor's output regardless of Phase B ordering.
// Implementations must be idempotent.
type PostProcessIndexer interface {
	PostProcessIndex(ctx context.Context, recordID int64) error
}

// PostProcessDependent is implemented by a Phase C processor whose indexing
// must not start until other named Phase C processors have finished for the
// same record (for example, structural relation harvesting needs entity
// reconciliation's kb.artifact_objects rows to already exist). A name absent
// from this run's invoked processors is not waited on.
type PostProcessDependent interface {
	PostProcessDependsOn() []string
}

// runPostProcessIndexing executes Phase C: for each invoked processor that implements
// PostProcessIndexer, run its indexing step concurrently (one goroutine per processor).
// A processor that also implements PostProcessDependent waits for its named
// dependencies (if invoked this run) to finish before it starts. Errors are logged and
// do not abort sibling processors' indexing. The caller skips this when the pipeline was
// stopped.
func (s *ControlService) runPostProcessIndexing(ctx context.Context, processors []Processor, recordID int64) {
	if isCtxStopped(ctx) {
		return
	}
	ctx, phaseSpan := startPhaseSpan(ctx, "C", recordID, processors)
	defer phaseSpan.End()

	done := make(map[string]chan struct{}, len(processors))
	for _, p := range processors {
		if _, ok := p.(PostProcessIndexer); ok {
			done[processorLogName(p)] = make(chan struct{})
		}
	}

	var wg sync.WaitGroup
	for _, p := range processors {
		indexer, ok := p.(PostProcessIndexer)
		if !ok {
			continue
		}
		wg.Add(1)
		go func(p Processor, indexer PostProcessIndexer) {
			name := processorLogName(p)
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					if s.Logger != nil {
						s.Logger.Error("post-process indexing panicked", "record_id", recordID, "processor", name, "panic", r)
					}
				}
				if ch, ok := done[name]; ok {
					close(ch)
				}
			}()

			if dependent, ok := indexer.(PostProcessDependent); ok {
				for _, depName := range dependent.PostProcessDependsOn() {
					depDone, invoked := done[depName]
					if !invoked {
						continue
					}
					select {
					case <-depDone:
					case <-ctx.Done():
						return
					}
				}
			}

			startTime := time.Now()
			if s.Logger != nil {
				s.Logger.Info("post-process indexing start", "record_id", recordID, "processor", name)
			}
			indexCtx, indexSpan := otel.Tracer(docProcessorTracerName).Start(ctx, "doc_processor.post_process_index",
				trace.WithAttributes(
					attribute.Int64("doc.record_id", recordID),
					attribute.String("processor.name", name),
					attribute.String("processor.phase", "C"),
				),
			)
			if err := indexer.PostProcessIndex(indexCtx, recordID); err != nil {
				indexSpan.RecordError(err)
				indexSpan.SetStatus(codes.Error, err.Error())
				indexSpan.SetAttributes(attribute.String("index.status", "failed"))
				indexSpan.End()
				if s.Logger != nil {
					s.Logger.Error("post-process indexing failed", "record_id", recordID, "processor", name, "error", err)
				}
				return
			}
			indexSpan.SetAttributes(attribute.String("index.status", "success"))
			indexSpan.SetStatus(codes.Ok, "success")
			indexSpan.End()
			if s.Logger != nil {
				s.Logger.Info("post-process indexing finished",
					"record_id", recordID,
					"processor", name,
					"ms_used", time.Since(startTime).Milliseconds(),
				)
			}
		}(p, indexer)
	}
	wg.Wait()
}

// isPhaseAProcessor reports whether the named processor is a mandatory,
// sequential Phase A processor. Must stay in sync with the mandatory set in
// cmd/doc-processor/main.go filterConfiguredProcessors.
func isPhaseAProcessor(name string) bool {
	return productionProcessorPhase(canonicalOperationName(name)) == "A"
}

type procResult struct {
	failed    bool
	stopped   bool
	err       error
	operation string
	msUsed    int64
}

// runSingleProcessorCollect runs one processor and returns its outcome without
// mutating shared controller state, so it is safe to call from concurrent
// goroutines. All status writes inside go through the per-record status lock.
func (s *ControlService) runSingleProcessorCollect(ctx context.Context, payload []byte, p Processor, recordID int64) procResult {
	procStart := s.now()
	processorName := processorLogName(p)
	ctx, span := startProcessorSpan(ctx, p, processorPhase(p), recordID)
	defer span.End()
	if s.Logger != nil {
		s.Logger.Info("start running processor",
			"record_id", recordID,
			"processor", processorName,
		)
	}
	s.persistProcessorRuntimeStatus(ctx, recordID, processorName, "active", "")
	s.persistPipelineStatus(ctx, recordID, "running", processorName, nil)
	if err := p.HandleEvent(ctx, payload); err != nil {
		res := procResult{failed: true, err: err, operation: processorName, msUsed: time.Since(procStart).Milliseconds()}
		procStatus := "failed"
		if errors.Is(err, ErrPipelineStopped) || isCtxStopped(ctx) {
			res.stopped = true
			procStatus = "stopped"
			if s.Logger != nil {
				s.Logger.Info("processor stopped by user request", "processor", processorName, "record_id", recordID)
			}
		} else if s.Logger != nil {
			s.Logger.Error("doc processor failed", "processor", processorName, "error", err)
		}
		if s.Logger != nil {
			s.Logger.Info("finish running processor",
				"record_id", recordID,
				"processor", processorName,
				"proc_status", procStatus,
				"ms_used", res.msUsed,
			)
		}
		s.persistProcessorRuntimeStatus(ctx, recordID, processorName, procStatus, "")
		setProcessorSpanResult(span, procStatus, err, res.msUsed)
		return res
	}
	res := procResult{operation: processorName, msUsed: time.Since(procStart).Milliseconds()}
	if s.Logger != nil {
		s.Logger.Info("finish running processor",
			"record_id", recordID,
			"processor", processorName,
			"proc_status", "success",
			"ms_used", res.msUsed,
		)
	}
	s.persistProcessorRuntimeStatus(ctx, recordID, processorName, "success", "")
	setProcessorSpanResult(span, "success", nil, res.msUsed)
	return res
}

func (s *ControlService) runSingleProcessor(ctx context.Context, payload []byte, p Processor, recordID int64, requestFailed *bool, firstErr *error, summaries *[]procResult) {
	res := s.runSingleProcessorCollect(ctx, payload, p, recordID)
	if summaries != nil {
		*summaries = append(*summaries, res)
	}
	if res.failed {
		*requestFailed = true
		if *firstErr == nil {
			*firstErr = res.err
		}
	}
}

// runProcessorsSequential runs all processors in order exactly as the original
// sequential loop did. Used as the fallback when RUN_DOC_PROCESSOR_CONCURRENT=false.
func (s *ControlService) runProcessorsSequential(
	ctx context.Context, payload []byte, processors []Processor,
	recordID int64, requestFailed, requestStopped *bool, firstErr *error,
	summaries *[]procResult,
) {
	phaseA, phaseB := splitProcessorsByPhase(processors)
	phaseACtx, phaseASpan := startPhaseSpan(ctx, "A", recordID, phaseA)
	phaseAEnded := false
	endPhaseA := func() {
		if !phaseAEnded {
			phaseASpan.End()
			phaseAEnded = true
		}
	}
	defer endPhaseA()
	var phaseBSpan trace.Span
	phaseBCtx := ctx
	phaseBStarted := false
	for _, p := range processors {
		if p == nil {
			continue
		}
		if isCtxStopped(ctx) {
			*requestStopped = true
			if s.Logger != nil {
				s.Logger.Info("doc processor stop requested, halting pipeline", "record_id", recordID)
			}
			return
		}
		runCtx := phaseACtx
		if !isPhaseAProcessor(p.Name()) {
			if !phaseBStarted {
				endPhaseA()
				phaseBCtx, phaseBSpan = startPhaseSpan(ctx, "B", recordID, phaseB)
				phaseBStarted = true
				defer phaseBSpan.End()
			}
			runCtx = phaseBCtx
		}
		s.runSingleProcessor(runCtx, payload, p, recordID, requestFailed, firstErr, summaries)
		if !*requestFailed && canonicalOperationName(p.Name()) == "static_analyzer" {
			clearBlockBufferInContext(ctx)
		}
		if *requestFailed && isCtxStopped(ctx) {
			*requestFailed = false
			*firstErr = nil
			*requestStopped = true
			if s.Logger != nil {
				s.Logger.Info("doc processor stop detected after processor", "record_id", recordID, "processor", p.Name())
			}
			return
		}
	}
}

// runProcessorsTwoPhase runs Phase A processors (mandatory, sequential) and then
// fans out Phase B (configurable, concurrent) under a WaitGroup. Each goroutine
// runs independently via runSingleProcessorCollect so shared controller state is
// not mutated. A panic in a Phase B processor is recovered and recorded as a
// failure without crashing the parent.
func (s *ControlService) runProcessorsTwoPhase(
	ctx context.Context, payload []byte, processors []Processor,
	recordID int64, requestFailed, requestStopped *bool, firstErr *error,
	summaries *[]procResult,
) {
	phaseA, phaseB := splitProcessorsByPhase(processors)
	phaseACtx, phaseASpan := startPhaseSpan(ctx, "A", recordID, phaseA)
	for _, p := range phaseA {
		if p == nil {
			continue
		}
		if isCtxStopped(ctx) {
			*requestStopped = true
			phaseASpan.End()
			return
		}
		s.runSingleProcessor(phaseACtx, payload, p, recordID, requestFailed, firstErr, summaries)
		if !*requestFailed && canonicalOperationName(p.Name()) == "static_analyzer" {
			clearBlockBufferInContext(ctx)
		}
		if *requestFailed && isCtxStopped(ctx) {
			*requestFailed = false
			*firstErr = nil
			*requestStopped = true
			phaseASpan.End()
			return
		}
	}
	phaseASpan.End()
	if isCtxStopped(ctx) {
		*requestStopped = true
		return
	}
	if len(phaseB) == 0 {
		return
	}

	phaseBCtx, phaseBSpan := startPhaseSpan(ctx, "B", recordID, phaseB)
	defer phaseBSpan.End()
	results := make([]procResult, len(phaseB))
	var wg sync.WaitGroup
	for i, p := range phaseB {
		wg.Add(1)
		go func(i int, p Processor) {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					results[i] = procResult{failed: true, err: fmt.Errorf("(MID_26060101) processor %q panicked: %v", p.Name(), r)}
					if s.Logger != nil {
						s.Logger.Error("doc processor panicked", "processor", p.Name(), "record_id", recordID, "panic", r)
					}
				}
			}()
			results[i] = s.runSingleProcessorCollect(phaseBCtx, payload, p, recordID)
		}(i, p)
	}
	wg.Wait()

	if summaries != nil {
		*summaries = append(*summaries, results...)
	}
	for _, r := range results {
		if r.failed {
			*requestFailed = true
			if *firstErr == nil {
				*firstErr = r.err
			}
		}
	}
	if isCtxStopped(ctx) {
		*requestFailed = false
		*firstErr = nil
		*requestStopped = true
	}
}

type pipelineProcSummaryJSON struct {
	Operation  string `json:"operation"`
	ProcStatus string `json:"proc_status"`
	MSUsed     int64  `json:"ms_used"`
}

func (s *ControlService) logPipelineFinish(ctx context.Context, recordID int64, msUsed int64, results []procResult) {
	summaries := make([]pipelineProcSummaryJSON, len(results))
	for i, r := range results {
		procStatus := "success"
		if r.stopped {
			procStatus = "stopped"
		} else if r.failed {
			procStatus = "failed"
		}
		summaries[i] = pipelineProcSummaryJSON{
			Operation:  r.operation,
			ProcStatus: procStatus,
			MSUsed:     r.msUsed,
		}
	}
	extraInfoBytes, err := json.Marshal(summaries)
	if err != nil {
		if s.Logger != nil {
			s.Logger.Warn("logPipelineFinish: failed marshalling extra_info", "record_id", recordID, "error", err)
		}
		return
	}
	extraInfoStr := string(extraInfoBytes)
	activityName := "pipeline finish"
	logger := DocProcLogger{}
	if err := logger.LogPipelineFinish(ctx, DocProcLogRecord{
		CallReason:    "pipeline finish",
		DocProcName:   "pipeline",
		RecordID:      int64Ptr(recordID),
		ActivityName:  &activityName,
		MSUsed:        int64Ptr(msUsed),
		ExtraInfoJSON: &extraInfoStr,
	}, "MID-26060801"); err != nil && s.Logger != nil {
		s.Logger.Warn("failed to write pipeline finish log", "record_id", recordID, "error", err)
	}
}

// persistDocFacets upserts the deterministic routing facets for a record
// into kb.doc_facets. Best-effort: a failure here does not affect the
// document-processing run itself, matching how plan-fact/plan-store
// failures elsewhere in this file are logged and treated as non-blocking.
func (s *ControlService) persistDocFacets(ctx context.Context, recordID int64, facts ProductionPlanFacts) {
	if s.FacetStore == nil || recordID <= 0 {
		return
	}
	err := s.FacetStore.UpsertDocFacets(ctx, DocFacetRecord{
		RecordID:              recordID,
		KSStoreID:             facts.KnowledgeStoreID,
		KnowledgeStoreBinding: facts.RoutingFacets.KnowledgeStoreBinding,
		InputDocType:          facts.RoutingFacets.InputDocType,
		SourceLanguage:        facts.RoutingFacets.SourceLanguage,
		HasDocumentNumber:     facts.RoutingFacets.HasDocumentNumber,
	})
	if err != nil && s.Logger != nil {
		s.Logger.Warn("failed persisting doc facets", "record_id", recordID, "error", err)
	}
}

func (s *ControlService) persistPipelineStatus(ctx context.Context, recordID int64, procStatus string, processorName string, procErr error) {
	s.persistPipelineStatusWithPlan(ctx, recordID, procStatus, processorName, procErr, ProductionPlanFacts{}, nil, ProductionPipelineSelection{}, ProductionPipelineBindingResolution{}, ProductionPipelineSpec{}, nil)
}

func (s *ControlService) persistPipelineStatusWithPlan(ctx context.Context, recordID int64, procStatus string, processorName string, procErr error, planFacts ProductionPlanFacts, planSteps []ProcessorPlanStep, pipelineSelection ProductionPipelineSelection, pipelineBinding ProductionPipelineBindingResolution, pipelineSpec ProductionPipelineSpec, excludedByPolicy []string) {
	if s.InputStore == nil || recordID <= 0 {
		return
	}
	err := updateInputStatusAtomic(ctx, s.InputStore, recordID, func(current string) (DocMetadataUpdate, error) {
		statusRaw, err := appendPipelineStatusWithPlan(current, s.now(), procStatus, processorName, procErr, planFacts, planSteps, pipelineSelection, pipelineBinding, pipelineSpec, excludedByPolicy)
		if err != nil {
			return DocMetadataUpdate{}, err
		}
		var errMsg *string
		if procErr != nil {
			msg := strings.TrimSpace(procErr.Error())
			errMsg = &msg
		}
		return DocMetadataUpdate{StatusRaw: statusRaw, ErrorMsg: errMsg}, nil
	})
	if err != nil && s.Logger != nil {
		s.Logger.Error("failed persisting doc pipeline status", "record_id", recordID, "error", err)
	}
}

func (s *ControlService) persistProcessorRuntimeStatus(ctx context.Context, recordID int64, processorName string, procStatus string, progress string) {
	if s.InputStore == nil || recordID <= 0 || strings.TrimSpace(processorName) == "" {
		return
	}
	err := updateInputStatusAtomic(ctx, s.InputStore, recordID, func(current string) (DocMetadataUpdate, error) {
		statusRaw, err := upsertProcessorRuntimeStatus(current, s.now(), processorName, procStatus, progress)
		if err != nil {
			return DocMetadataUpdate{}, err
		}
		return DocMetadataUpdate{StatusRaw: statusRaw}, nil
	})
	if err != nil && s.Logger != nil {
		s.Logger.Error("failed persisting processor runtime status", "record_id", recordID, "processor", processorName, "error", err)
	}
}

func processorLogName(p Processor) string {
	if p == nil {
		return ""
	}
	if named, ok := p.(logNamedProcessor); ok {
		if logName := canonicalOperationName(named.LogName()); logName != "" {
			return logName
		}
	}
	return p.Name()
}

func splitProcessorsByPhase(processors []Processor) ([]Processor, []Processor) {
	phaseA := make([]Processor, 0, len(processors))
	phaseB := make([]Processor, 0, len(processors))
	for _, p := range processors {
		if p == nil {
			continue
		}
		if isPhaseAProcessor(p.Name()) {
			phaseA = append(phaseA, p)
			continue
		}
		phaseB = append(phaseB, p)
	}
	return phaseA, phaseB
}

func (s *ControlService) preflightInput(ctx context.Context, evt LineFileGeneratedEvent) bool {
	if s.InputStore == nil {
		return true
	}
	_, recordSpan := otel.Tracer(docProcessorTracerName).Start(ctx, "doc_processor.record.load",
		trace.WithAttributes(attribute.Int64("doc.record_id", evt.RecordID)),
	)
	rec, err := s.InputStore.GetInputRecord(ctx, evt.RecordID)
	if err != nil {
		recordSpan.RecordError(err)
		recordSpan.SetStatus(codes.Error, err.Error())
		recordSpan.End()
		if errors.Is(err, sql.ErrNoRows) {
			if s.Logger != nil {
				s.Logger.Error("kb.inputs record not found", "record_id", evt.RecordID)
			}
			return false
		}
		if s.Logger != nil {
			s.Logger.Error("load kb.inputs record failed", "record_id", evt.RecordID, "error", err)
		}
		return false
	}
	recordSpan.SetAttributes(
		attribute.String("doc.parser_name", strings.TrimSpace(rec.ParserName)),
		attribute.String("doc.result_filename", strings.TrimSpace(rec.ResultFilename)),
	)
	recordSpan.SetStatus(codes.Ok, "success")
	recordSpan.End()

	if strings.TrimSpace(rec.ParserName) == "" {
		s.persistControlFailure(ctx, rec, errors.New("(MID_26042401) missing parser name"))
		return false
	}
	if strings.TrimSpace(rec.ResultFilename) == "" {
		s.persistControlFailure(ctx, rec, errors.New("(MID_26042402) missing result filename"))
		return false
	}

	_, resolveSpan := otel.Tracer(docProcessorTracerName).Start(ctx, "doc_processor.input_file.resolve",
		trace.WithAttributes(
			attribute.Int64("doc.record_id", evt.RecordID),
			attribute.String("doc.parser_name", strings.TrimSpace(rec.ParserName)),
		),
	)
	inputPath, err := ResolveInputFilePath(evt, rec.ResultFilename, rec.ParserName, rec.StagingFilename)
	if err != nil {
		resolveSpan.RecordError(err)
		resolveSpan.SetStatus(codes.Error, err.Error())
		resolveSpan.End()
		s.persistControlFailure(ctx, rec, err)
		return false
	}
	resolveSpan.SetAttributes(attribute.String("doc.input_path", inputPath))
	resolveSpan.SetStatus(codes.Ok, "success")
	resolveSpan.End()

	_, validateSpan := otel.Tracer(docProcessorTracerName).Start(ctx, "doc_processor.input_file.validate",
		trace.WithAttributes(
			attribute.Int64("doc.record_id", evt.RecordID),
			attribute.String("doc.input_path", inputPath),
		),
	)
	fi, err := os.Stat(inputPath)
	if err != nil {
		if os.IsNotExist(err) {
			procErr := fmt.Errorf("(MID_26042405) input file not exist: %s", inputPath)
			validateSpan.RecordError(procErr)
			validateSpan.SetStatus(codes.Error, procErr.Error())
			validateSpan.End()
			s.persistControlFailure(ctx, rec, procErr)
			return false
		}
		procErr := fmt.Errorf("(MID_26042406) stat input file: %w", err)
		validateSpan.RecordError(procErr)
		validateSpan.SetStatus(codes.Error, procErr.Error())
		validateSpan.End()
		s.persistControlFailure(ctx, rec, procErr)
		return false
	}
	if fi.Size() == 0 {
		procErr := errors.New("(MID_26042403) input file empty")
		validateSpan.RecordError(procErr)
		validateSpan.SetStatus(codes.Error, procErr.Error())
		validateSpan.End()
		s.persistControlFailure(ctx, rec, procErr)
		return false
	}
	validateSpan.SetAttributes(attribute.Int64("doc.input_size_bytes", fi.Size()))
	validateSpan.SetStatus(codes.Ok, "success")
	validateSpan.End()
	return true
}

func (s *ControlService) resolveProductionPlanFacts(ctx context.Context, evt LineFileGeneratedEvent) (ProductionPlanFacts, error) {
	mode, modeErr := DocPipelineModeFromEnv()
	if modeErr != nil {
		mode = DocPipelineModePlanOnly
	}
	// evt.Operations is only set for an explicit per-event processor
	// request. The common ingestion path (no operations in the payload)
	// runs the server's registered s.Processors instead, so that must be
	// the RequestedProcessors baseline here too -- otherwise plan.
	// ExcludedByPolicy() is computed against an empty list and enforcement
	// (which trusts that field) becomes a no-op for the common case.
	requested := append([]string(nil), evt.Operations...)
	if len(requested) == 0 && s != nil {
		for _, p := range s.Processors {
			if p != nil {
				requested = append(requested, p.Name())
			}
		}
	}
	if s == nil || s.InputStore == nil {
		if len(evt.Operations) > 0 {
			mode = DocPipelineModePlanOnly
		}
		return ProductionPlanFacts{RequestedProcessors: requested, Mode: mode}, nil
	}
	rec, err := s.InputStore.GetInputRecord(ctx, evt.RecordID)
	if err != nil {
		return ProductionPlanFacts{}, err
	}
	facts := BuildProductionPlanFactsFromInputRecord(requested, rec)
	if override := strings.TrimSpace(evt.PipelineOverride); override != "" {
		facts.RequestedPipeline = override
	}
	if len(evt.ProcessorGateOverrides) > 0 {
		facts.ProcessorGateOverrides = evt.ProcessorGateOverrides
	}
	if len(evt.Operations) > 0 {
		facts.Mode = DocPipelineModePlanOnly
	} else {
		facts.Mode = mode
	}
	return facts, nil
}

func (s *ControlService) persistControlFailure(ctx context.Context, rec DocMetadataInputRecord, procErr error) {
	if s.InputStore == nil {
		return
	}
	errMsg := strings.TrimSpace(procErr.Error())
	statusRaw, err := appendControlStatus(rec.StatusRaw, s.now(), procErr)
	if err != nil {
		if s.Logger != nil {
			s.Logger.Error("failed building doc processor failure status", "record_id", rec.ID, "error", err)
		}
		return
	}
	if err := s.InputStore.UpdateInputMetadata(ctx, rec.ID, DocMetadataUpdate{
		StatusRaw: statusRaw,
		ErrorMsg:  &errMsg,
	}); err != nil && s.Logger != nil {
		s.Logger.Error("failed persisting doc processor failure status", "record_id", rec.ID, "error", err)
	}
	if s.Logger != nil {
		s.Logger.Error("doc processor input validation failed", "record_id", rec.ID, "error", procErr)
	}
}

// pollForStop polls the database every second and cancels the pipeline context
// (with cause ErrPipelineStopped) if a stop request is detected. It exits when
// ctx is cancelled (either by the stop itself or by the pipeline completing).
func (s *ControlService) pollForStop(ctx context.Context, recordID int64, cancelCause context.CancelCauseFunc) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if ok, _ := s.StopStore.IsStopRequested(context.Background(), recordID); ok {
				if s.Logger != nil {
					s.Logger.Info("stop request detected, cancelling pipeline context", "record_id", recordID)
				}
				cancelCause(ErrPipelineStopped)
				return
			}
		}
	}
}

func (s *ControlService) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func (s *ControlService) insertReceivedEvent(ctx context.Context, subject string, payload []byte) (string, error) {
	eventID := newEventID()
	if s.EventStore == nil {
		return eventID, nil
	}
	start := s.now()
	status := []map[string]any{
		{
			"operation":   "received",
			"proc_status": "success",
			"start_time":  start.Format(defaultDocMetaStatusTime),
			"ms_used":     int64(0),
		},
	}
	statusJSON, err := json.Marshal(status)
	if err != nil {
		return "", err
	}
	eventPayload := normalizeStoredEventPayload(payload)
	req := EventRecord{
		EventName:    DefaultEventSubject,
		EventID:      eventID,
		EventSubject: strings.TrimSpace(subject),
		EventPayload: eventPayload,
		EventStatus:  string(statusJSON),
		EventSource:  "JetStream",
		EventNotes:   "",
	}
	if err := s.EventStore.InsertEvent(ctx, req); err != nil {
		return "", err
	}
	return eventID, nil
}

func normalizeStoredEventPayload(payload []byte) string {
	trimmed := strings.TrimSpace(string(payload))
	if trimmed == "" {
		return "{}"
	}
	if json.Valid([]byte(trimmed)) {
		return trimmed
	}
	out, err := json.Marshal(map[string]any{
		"_raw_payload":   trimmed,
		"_payload_error": "invalid_json",
	})
	if err != nil {
		return `{"_payload_error":"invalid_json"}`
	}
	return string(out)
}

type eventIDCtxKey struct{}

func withEventID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, eventIDCtxKey{}, id)
}

func eventIDFromContext(ctx context.Context) string {
	id, _ := ctx.Value(eventIDCtxKey{}).(string)
	return id
}

type runIDCtxKey struct{}

// withRunID attaches the kb.doc_process_runs.id of the current pipeline
// invocation to ctx, mirroring withEventID above. Set once in handleEvent;
// insertDocProcLog (doc_proc_log_store.go) reads it at its single choke
// point so every Log* call site stamps run_id without a signature change.
// See ADR 2026071201.
func withRunID(ctx context.Context, id int64) context.Context {
	return context.WithValue(ctx, runIDCtxKey{}, id)
}

func runIDFromContext(ctx context.Context) (int64, bool) {
	id, ok := ctx.Value(runIDCtxKey{}).(int64)
	return id, ok
}

func newEventID() string {
	var bs [12]byte
	if _, err := rand.Read(bs[:]); err != nil {
		return fmt.Sprintf("evt-%d", time.Now().UnixNano())
	}
	return "evt-" + hex.EncodeToString(bs[:])
}

func appendPipelineStatus(raw string, now time.Time, procStatus string, processorName string, procErr error) (string, error) {
	return appendPipelineStatusWithPlan(raw, now, procStatus, processorName, procErr, ProductionPlanFacts{}, nil, ProductionPipelineSelection{}, ProductionPipelineBindingResolution{}, ProductionPipelineSpec{}, nil)
}

func appendPipelineStatusWithPlan(raw string, now time.Time, procStatus string, processorName string, procErr error, planFacts ProductionPlanFacts, planSteps []ProcessorPlanStep, pipelineSelection ProductionPipelineSelection, pipelineBinding ProductionPipelineBindingResolution, pipelineSpec ProductionPipelineSpec, excludedByPolicy []string) (string, error) {
	status := strings.ToLower(strings.TrimSpace(procStatus))
	if status == "" {
		status = "running"
	}
	entry := map[string]any{
		"operation":   "doc_processing",
		"start_time":  now.Format(defaultDocMetaStatusTime),
		"proc_status": status,
	}
	if processorName != "" {
		entry["doc_processor_name"] = processorName
	}
	if procErr != nil {
		entry["error"] = strings.TrimSpace(procErr.Error())
	}
	if len(planFacts.RequestedProcessors) > 0 || planFacts.KnowledgeStoreID != 0 || strings.TrimSpace(planFacts.KnowledgeStoreType) != "" || strings.TrimSpace(planFacts.ParserName) != "" || strings.TrimSpace(planFacts.DocumentTitle) != "" {
		entry["processor_plan_facts"] = planFacts
	}
	if len(planSteps) > 0 {
		entry["processor_plan_steps"] = planSteps
	}
	if strings.TrimSpace(pipelineSelection.PipelineName) != "" || strings.TrimSpace(pipelineSelection.Reason) != "" {
		entry["processor_pipeline_selection"] = pipelineSelection
	}
	if strings.TrimSpace(pipelineBinding.Source) != "" || strings.TrimSpace(pipelineBinding.SelectedPipeline) != "" || strings.TrimSpace(pipelineBinding.RequestedPipeline) != "" || strings.TrimSpace(pipelineBinding.StoreBoundPipeline) != "" {
		entry["processor_pipeline_binding"] = pipelineBinding
	}
	if strings.TrimSpace(pipelineSpec.Name) != "" {
		entry["processor_pipeline_spec"] = pipelineSpec
	}
	if len(excludedByPolicy) > 0 {
		entry["processor_excluded_by_policy"] = excludedByPolicy
	}

	entries := decodeDocMetaStatus(raw)
	replaced := false
	out := make([]map[string]any, 0, len(entries)+1)
	for _, e := range entries {
		op := strings.ToLower(strings.TrimSpace(asString(e["operation"])))
		if op != "doc_processing" {
			out = append(out, e)
			continue
		}
		if !replaced {
			if originalStart := strings.TrimSpace(asString(e["start_time"])); originalStart != "" {
				entry["start_time"] = originalStart
			}
			if _, ok := entry["processor_plan_facts"]; !ok {
				if existingFacts, ok := e["processor_plan_facts"]; ok {
					entry["processor_plan_facts"] = existingFacts
				}
			}
			if _, ok := entry["processor_plan_steps"]; !ok {
				if existingSteps, ok := e["processor_plan_steps"]; ok {
					entry["processor_plan_steps"] = existingSteps
				}
			}
			if _, ok := entry["processor_pipeline_selection"]; !ok {
				if existingSelection, ok := e["processor_pipeline_selection"]; ok {
					entry["processor_pipeline_selection"] = existingSelection
				}
			}
			if _, ok := entry["processor_pipeline_binding"]; !ok {
				if existingBinding, ok := e["processor_pipeline_binding"]; ok {
					entry["processor_pipeline_binding"] = existingBinding
				}
			}
			if _, ok := entry["processor_pipeline_spec"]; !ok {
				if existingSpec, ok := e["processor_pipeline_spec"]; ok {
					entry["processor_pipeline_spec"] = existingSpec
				}
			}
			if _, ok := entry["processor_excluded_by_policy"]; !ok {
				if existingExcluded, ok := e["processor_excluded_by_policy"]; ok {
					entry["processor_excluded_by_policy"] = existingExcluded
				}
			}
			out = append(out, entry)
			replaced = true
		}
	}
	if !replaced {
		out = append(out, entry)
	}

	bs, err := json.Marshal(out)
	if err != nil {
		return "", err
	}
	return string(bs), nil
}

func appendControlStatus(raw string, now time.Time, procErr error) (string, error) {
	entries := decodeDocMetaStatus(raw)
	entry := map[string]any{
		"operation":   "doc_processor",
		"start_time":  now.Format(defaultDocMetaStatusTime),
		"ms-used":     int64(0),
		"proc-status": "failed",
		"error":       strings.TrimSpace(procErr.Error()),
	}

	replaced := false
	out := make([]map[string]any, 0, len(entries)+1)
	for _, e := range entries {
		op := strings.ToLower(strings.TrimSpace(asString(e["operation"])))
		if op != "doc_processor" {
			out = append(out, e)
			continue
		}
		if !replaced {
			out = append(out, entry)
			replaced = true
		}
	}
	if !replaced {
		out = append(out, entry)
	}

	bs, err := json.Marshal(out)
	if err != nil {
		return "", err
	}
	return string(bs), nil
}

func (s *ControlService) selectProcessors(ops []string) []Processor {
	if len(ops) == 0 {
		return s.Processors
	}

	available := make(map[string]Processor, len(s.Processors))
	for _, p := range s.Processors {
		if p == nil {
			continue
		}
		available[canonicalOperationName(p.Name())] = p
	}

	expanded := expandProcessorDependencies(ops)
	selected := make([]Processor, 0, len(expanded))
	seen := make(map[string]struct{}, len(expanded))
	for _, op := range expanded {
		key := canonicalOperationName(op)
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		p, ok := available[key]
		if !ok {
			if s.Logger != nil {
				s.Logger.Info("doc processor operation ignored", "operation", op)
			}
			continue
		}
		seen[key] = struct{}{}
		selected = append(selected, p)
	}
	return selected
}

// applyPlanEnforcement drops any already-selected processor named in
// excluded. excluded is empty in plan-only mode, when the resolved pipeline
// declares no explicit Processors, and whenever FinalizeRoutingPlan left a
// suppressive decision shadow-only for lack of an effective D2 clearance --
// i.e. every case that existed before E1-E3 was a no-op here, so this
// changes nothing about that legacy behavior. It only ever removes
// something for a decision FinalizeRoutingPlan actually enforced.
func (s *ControlService) applyPlanEnforcement(processors []Processor, excluded []string) []Processor {
	if len(excluded) == 0 {
		return processors
	}
	excludedSet := make(map[string]bool, len(excluded))
	for _, name := range excluded {
		excludedSet[normalizeRuntimeName(name)] = true
	}
	filtered := make([]Processor, 0, len(processors))
	for _, p := range processors {
		if p == nil || excludedSet[normalizeRuntimeName(p.Name())] {
			continue
		}
		filtered = append(filtered, p)
	}
	return filtered
}

// documentKindFromEnrichedFacts extracts the governed document.doc_kind
// tier-3 facet from resolver-enriched facts, if known. Spec 2026080102
// section 9 requires clearance coverage to key on this governed document
// kind, not the file-format input_doc_type -- a clearance approved for one
// document kind must never be treated as covering a record merely because
// it shares a file format. When the facet is unresolved (nil facts, missing,
// non-known state, or an unexpected value type), the empty string is
// returned, which correctly leaves the subject shadow-only rather than
// falling back to a different key (P5 review 2026080302 finding P5-6).
// resolverAttemptKey derives the P5 two-pass resolver's attempt identity
// from the event that triggered this dispatch. It must be stable across a
// retry/redelivery of the same event (so the classifier's stable-retry-via-
// invocation-id correctly avoids a duplicate LLM call) and distinct across a
// genuinely new processing attempt (so a stale observation is never
// silently reused). A run id cannot serve this purpose: routing must be
// decided before any kb.doc_process_runs row exists (before dispatch).
// evt.Filename -- the just-generated line file's own name -- is
// attempt-unique by construction, since a fresh parse produces a fresh
// file (P5 review 2026080302 finding P5-2).
func resolverAttemptKey(evt LineFileGeneratedEvent) string {
	if filename := strings.TrimSpace(evt.Filename); filename != "" {
		return filename
	}
	// Should not happen for a real LineFileGeneratedEvent, but avoid an
	// empty attempt key: the record id at least gives retries of the same
	// record a stable (if coarser) key.
	return fmt.Sprintf("record-%d", evt.RecordID)
}

// boundedDocumentSample reads a bounded prefix of the already-parsed line
// file's extracted text content (never the raw upload) for classify_document.
// A read failure degrades to an empty sample rather than aborting the
// request -- the classifier already treats an empty/unusable sample the
// same as any other unresolved tier-3 facet (P5 review 2026080302 finding
// P5-2: the call site previously hardcoded an empty string).
func boundedDocumentSample(evt LineFileGeneratedEvent, facts ProductionPlanFacts) string {
	path, err := ResolveInputFilePath(evt, facts.ResultFilename, facts.ParserName, facts.StagingFilename)
	if err != nil || strings.TrimSpace(path) == "" {
		return ""
	}
	return readBoundedDocumentSample(path)
}

// readBoundedDocumentSample reads a bounded prefix of an already-parsed line
// file's extracted text content (never the raw upload). A missing/unreadable
// file degrades to an empty sample.
func readBoundedDocumentSample(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()

	var sb strings.Builder
	sc := bufio.NewScanner(f)
	for sc.Scan() && sb.Len() < DefaultClassifyDocumentMaxSample {
		line, err := parseLine(sc.Text())
		if err != nil || line.Content == "" {
			continue
		}
		if sb.Len() > 0 {
			sb.WriteByte('\n')
		}
		sb.WriteString(line.Content)
	}
	sample := sb.String()
	if len(sample) > DefaultClassifyDocumentMaxSample {
		sample = sample[:DefaultClassifyDocumentMaxSample]
	}
	return sample
}

func documentKindFromEnrichedFacts(enriched semrules.FactSet) string {
	fact, ok := enriched["document.doc_kind"]
	if !ok || fact.State != semrules.FactKnown {
		return ""
	}
	value, ok := fact.Value.(string)
	if !ok {
		return ""
	}
	return value
}

// finalizeRoutingPlan builds a RoutingEnforcementRequest from the already
// resolved plan (E1's pure gate shadow and C1's pure binding selection) and
// asks FinalizeRoutingPlan to turn it into a real effective processor set,
// consulting s.RoutingClearances for every suppressive decision. It is the
// control.go call site for E3's single FinalizeRoutingPlan boundary.
func (s *ControlService) finalizeRoutingPlan(ctx context.Context, plan ProductionProcessorPlan) (RoutingEnforcementResult, error) {
	facts := plan.Facts()
	binding := plan.PipelineBinding()
	snapshot := plan.RoutingSnapshot()
	gateShadow := ProcessorGateShadowPlan{}
	if snapshot != nil {
		gateShadow = snapshot.GateShadow
	}
	baselineSpec, _ := LookupProductionPipeline(DefaultProductionPipelineName)
	req := RoutingEnforcementRequest{
		Mode:                     facts.Mode,
		Explicit:                 facts.ExplicitProcessorOverride,
		PipelineName:             plan.PipelineSpec().Name,
		PipelineVersion:          plan.PipelineSpec().Version,
		DocumentKind:             facts.RoutingFacets.DocKind,
		RequestedProcessors:      facts.RequestedProcessors,
		BindingSource:            binding.Source,
		BindingID:                binding.BindingID,
		BindingName:              binding.RuleName,
		BindingPredicateChecksum: binding.PredicateChecksum,
		SelectedSpec:             plan.PipelineSpec(),
		BaselineSpec:             baselineSpec,
		GateShadow:               gateShadow,
	}
	return FinalizeRoutingPlan(ctx, req, s.RoutingClearances)
}

// raiseRoutingAlarms dedupes and persists operator alarms via
// s.RoutingAlarms. It is a no-op (never blocks/fails the pipeline) when no
// writer is configured or a write fails -- an alarms_errors outage must not
// stop document processing.
func (s *ControlService) raiseRoutingAlarms(ctx context.Context, alarms []RoutingAlarm) {
	if len(alarms) == 0 {
		return
	}
	for _, err := range WriteRoutingAlarms(ctx, s.RoutingAlarms, alarms) {
		if s.Logger != nil {
			s.Logger.Warn("failed writing routing alarm", "error", err)
		}
	}
}

// policyAuditEventKindForAlarm maps a RoutingAlarm's Kind to the
// policyaudit event kind the P5 plan's "conflicts, fallback" checklist item
// asks for. Alarm kinds with no defined event (operator_failure,
// policy_integrity_failure) return ok=false and stay alarm-only, matching
// the plan's event-kind list.
func policyAuditEventKindForAlarm(kind string) (string, bool) {
	switch kind {
	case RoutingAlarmKindBindingConflict:
		return policyaudit.EventBindingConflict, true
	case RoutingAlarmKindGateConflict:
		return policyaudit.EventGateConflict, true
	case RoutingAlarmKindFallbackWarning:
		return policyaudit.EventFallbackApplied, true
	default:
		return "", false
	}
}

// emitAlarmAuditEvent writes the policyaudit event matching a conflict or
// fallback RoutingAlarm (the "conflicts, fallback" P5 plan checklist item),
// at the same decision point that already raised the alarm. Best-effort: a
// write failure is logged, never propagated. A no-op when no PolicyAudit
// writer is configured or the alarm kind has no matching event kind.
func (s *ControlService) emitAlarmAuditEvent(ctx context.Context, recordID, runID int64, pipelineName string, pipelineVersion int, alarm RoutingAlarm) {
	kind, ok := policyAuditEventKindForAlarm(alarm.Kind)
	if !ok || s.PolicyAudit == nil {
		return
	}
	if err := s.PolicyAudit.WriteEvent(ctx, policyaudit.Event{
		Kind: kind, PipelineName: pipelineName, PipelineVersion: pipelineVersion, RunID: runID, RecordID: recordID,
		Detail: map[string]any{"alarm_kind": alarm.Kind, "message": alarm.Message},
	}); err != nil && s.Logger != nil {
		s.Logger.Warn("failed writing policy audit event", "kind", kind, "record_id", recordID, "error", err)
	}
}

// recordRoutingDecisionEvents writes one content-safe policyaudit event per
// suppressive routing decision FinalizeRoutingPlan evaluated -- enforced or
// shadowed -- so operators can reconstruct why a processor did or did not
// run without ever logging document content. Best-effort: a write failure is
// logged, never propagated.
func (s *ControlService) recordRoutingDecisionEvents(ctx context.Context, recordID, runID int64, pipelineName string, pipelineVersion int, result RoutingEnforcementResult) {
	if s.PolicyAudit == nil {
		return
	}
	emit := func(kind string, detail map[string]any) {
		if err := s.PolicyAudit.WriteEvent(ctx, policyaudit.Event{
			Kind: kind, PipelineName: pipelineName, PipelineVersion: pipelineVersion,
			RunID: runID, RecordID: recordID, Detail: detail,
		}); err != nil && s.Logger != nil {
			s.Logger.Warn("failed writing policy audit event", "kind", kind, "record_id", recordID, "error", err)
		}
	}
	for _, processor := range result.ExcludedByGate {
		emit(policyaudit.EventDecisionEnforced, map[string]any{"subject_kind": "processor_rule", "processor": processor})
	}
	for _, processor := range result.ShadowGateExclusions {
		emit(policyaudit.EventDecisionShadowed, map[string]any{"subject_kind": "processor_rule", "processor": processor})
	}
	if result.UsedFallbackPipeline {
		emit(policyaudit.EventDecisionShadowed, map[string]any{"subject_kind": "conditional_binding", "removed_processors": result.ShadowPipelineExclusions, "fallback_pipeline": result.Pipeline.Name})
	} else if len(result.ExcludedByPipeline) > 0 {
		emit(policyaudit.EventDecisionEnforced, map[string]any{"subject_kind": "conditional_binding", "excluded_processors": result.ExcludedByPipeline, "pipeline": result.Pipeline.Name})
	}
}

func requestedOperationsNeedAutoChunking(ops []string) bool {
	if len(ops) == 0 {
		return false
	}
	for _, raw := range ops {
		if canonicalOperationName(raw) == "chunking" {
			return false
		}
	}
	for _, raw := range ops {
		if requiresChunkingDependency(raw) {
			return true
		}
	}
	return false
}

func docProcessorSucceeded(statusRaw string, op string) bool {
	want := canonicalOperationName(op)
	if want == "" {
		return false
	}
	for _, entry := range decodeDocMetaStatus(statusRaw) {
		if canonicalOperationName(asString(entry["operation"])) != want {
			continue
		}
		if statusValue(entry) == "success" {
			return true
		}
	}
	return false
}

func expandProcessorDependencies(ops []string) []string {
	if len(ops) == 0 {
		return nil
	}
	expanded := make([]string, 0, len(ops)+2)
	appendOnce := func(op string) {
		key := canonicalOperationName(op)
		if key == "" {
			return
		}
		for _, existing := range expanded {
			if canonicalOperationName(existing) == key {
				return
			}
		}
		expanded = append(expanded, key)
	}

	hasRequestedChunking := false
	for _, raw := range ops {
		if canonicalOperationName(raw) == "chunking" {
			hasRequestedChunking = true
			break
		}
	}
	for _, raw := range ops {
		op := canonicalOperationName(raw)
		switch op {
		case "chunking":
			appendOnce("static_analyzer")
		default:
			if !hasRequestedChunking && requiresChunkingDependency(op) {
				appendOnce("static_analyzer")
				appendOnce("chunking")
			}
		}
		appendOnce(op)
	}
	return expanded
}

func requiresChunkingDependency(op string) bool {
	switch canonicalOperationName(op) {
	case "generate_summaries",
		"generate_topics",
		"generate_scene_blocks",
		"extract_entity",
		"extract_relation",
		"extract_entity_relation",
		"extract_inventory_items",
		"extract_metrics",
		"extract_provisions",
		"extract_metric_definitions",
		"extract_product_structure",
		"extract_test_methods",
		"extract_semantic_projections",
		"extract_structured_knowledge":
		return true
	default:
		return false
	}
}
