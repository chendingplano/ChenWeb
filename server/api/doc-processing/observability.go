package docprocessing

import (
	"context"
	"strings"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

const docProcessorTracerName = "chenweb-doc-processor"

func startPipelineSpan(ctx context.Context, subject string, eventID string, evt LineFileGeneratedEvent) (context.Context, trace.Span) {
	return otel.Tracer(docProcessorTracerName).Start(ctx, "doc_processor.pipeline",
		trace.WithSpanKind(trace.SpanKindConsumer),
		trace.WithAttributes(
			attribute.String("messaging.system", "nats_jetstream"),
			attribute.String("messaging.destination.name", strings.TrimSpace(subject)),
			attribute.String("doc_processor.event_id", strings.TrimSpace(eventID)),
			attribute.Int64("doc.record_id", evt.RecordID),
			attribute.String("doc.filename", strings.TrimSpace(evt.Filename)),
			attribute.String("doc.operation.requested", strings.Join(evt.Operations, ",")),
			attribute.Bool("doc.force", evt.Force),
			attribute.String("pipeline.mode", pipelineMode(evt)),
			attribute.Bool("pipeline.concurrent", RunDocProcessorConcurrentFromEnv()),
		),
	)
}

func startPhaseSpan(ctx context.Context, phase string, recordID int64, processors []Processor) (context.Context, trace.Span) {
	processorNames := processorNamesForTrace(processors)
	return otel.Tracer(docProcessorTracerName).Start(ctx, "doc_processor.phase_"+strings.ToLower(phase),
		trace.WithAttributes(
			attribute.Int64("doc.record_id", recordID),
			attribute.String("processor.phase", strings.ToUpper(phase)),
			attribute.Int("processor.count", len(processorNames)),
			attribute.String("processor.names", strings.Join(processorNames, ",")),
		),
	)
}

func startProcessorSpan(ctx context.Context, p Processor, phase string, recordID int64) (context.Context, trace.Span) {
	processorName := processorLogName(p)
	return otel.Tracer(docProcessorTracerName).Start(ctx, "doc_processor.processor."+processorName,
		trace.WithAttributes(
			attribute.Int64("doc.record_id", recordID),
			attribute.String("processor.name", processorName),
			attribute.String("processor.phase", strings.ToUpper(phase)),
			attribute.String("processor.type", processorTypeForPhase(phase)),
			attribute.Bool("processor.requires_llm", processorRequiresLLM(processorName)),
		),
	)
}

func processorPhase(p Processor) string {
	if p == nil {
		return ""
	}
	if canonicalOperationName(processorLogName(p)) == "blocking" || isPhaseAProcessor(p.Name()) {
		return "A"
	}
	return "B"
}

func endSpanWithStatus(span trace.Span, status string, err error) {
	if span == nil {
		return
	}
	if status != "" {
		span.SetAttributes(attribute.String("status", status))
	}
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return
	}
	span.SetStatus(codes.Ok, strings.TrimSpace(status))
}

func setProcessorSpanResult(span trace.Span, status string, err error, durationMS int64) {
	if span == nil {
		return
	}
	span.SetAttributes(
		attribute.String("processor.status", status),
		attribute.Int64("processor.duration_ms", durationMS),
	)
	endSpanWithStatus(span, status, err)
}

func recordDocProcLogSpan(ctx context.Context, rec DocProcLogRecord) {
	if rec.LLMCallID == nil && rec.EntryType != EntryTypeLLMCall {
		return
	}
	callID := ""
	if rec.LLMCallID != nil {
		callID = strings.TrimSpace(*rec.LLMCallID)
	}
	if callID == "" && rec.EntryType != EntryTypeLLMCall {
		return
	}

	attrs := []attribute.KeyValue{
		attribute.String("doc_proc.name", strings.TrimSpace(rec.DocProcName)),
		attribute.String("doc_proc.entry_type", strings.TrimSpace(rec.EntryType)),
		attribute.String("llm.call_id", callID),
		attribute.String("llm.prompt_name", strings.TrimSpace(rec.PromptName)),
		attribute.StringSlice("llm.model_names", rec.ModelNames),
	}
	if rec.RecordID != nil {
		attrs = append(attrs, attribute.Int64("doc.record_id", *rec.RecordID))
	}
	if rec.Pass != nil {
		attrs = append(attrs, attribute.Int("llm.pass", *rec.Pass))
	}
	if rec.ActivityName != nil {
		attrs = append(attrs, attribute.String("llm.activity_name", strings.TrimSpace(*rec.ActivityName)))
	}
	if rec.MSUsed != nil {
		attrs = append(attrs, attribute.Int64("llm.duration_ms", *rec.MSUsed))
	}
	if rec.LogLoc != nil {
		attrs = append(attrs, attribute.String("doc_proc.log_loc", strings.TrimSpace(*rec.LogLoc)))
	}

	_, span := otel.Tracer(docProcessorTracerName).Start(ctx, "doc_processor.llm_call",
		trace.WithAttributes(attrs...),
	)
	defer span.End()

	if rec.Errors != nil && strings.TrimSpace(*rec.Errors) != "" {
		err := strings.TrimSpace(*rec.Errors)
		span.SetAttributes(attribute.String("llm.status", "failed"))
		span.SetStatus(codes.Error, err)
		return
	}
	span.SetAttributes(attribute.String("llm.status", "success"))
	span.SetStatus(codes.Ok, "success")
}

func pipelineMode(evt LineFileGeneratedEvent) string {
	if len(evt.Operations) == 0 {
		return "all_processors"
	}
	return "selected_processors"
}

func processorNamesForTrace(processors []Processor) []string {
	names := make([]string, 0, len(processors))
	for _, p := range processors {
		if p == nil {
			continue
		}
		names = append(names, processorLogName(p))
	}
	return names
}

func processorTypeForPhase(phase string) string {
	switch strings.ToUpper(strings.TrimSpace(phase)) {
	case "A":
		return "mandatory"
	case "B":
		return "configurable"
	case "C":
		return "post_process"
	default:
		return ""
	}
}

func processorRequiresLLM(processorName string) bool {
	switch canonicalOperationName(processorName) {
	case "extract_doc_metadata",
		"extract_metrics",
		"extract_provisions",
		"extract_semantic_projections",
		"generate_summaries",
		"generate_topics",
		"generate_scene_blocks",
		"extract_structured_knowledge",
		"extract_entity_relation",
		"extract_inventory_items":
		return true
	default:
		return false
	}
}
