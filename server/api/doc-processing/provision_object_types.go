package docprocessing

import (
	"context"
	"fmt"

	llmclients "github.com/chendingplano/shared/go/api/llm"
)

type provisionDiagnosticsKey struct{}

func withProvisionChunkDiagnostics(ctx context.Context, recordID int64, index, total int, chunk Chunk) context.Context {
	metadata := map[string]any{"chunk_index": index, "chunk_total": total}
	for _, line := range chunk.Lines {
		n := line.Line.LineNo
		if v, ok := metadata["source_line_start"].(int); !ok || n < v {
			metadata["source_line_start"] = n
		}
		if v, ok := metadata["source_line_end"].(int); !ok || n > v {
			metadata["source_line_end"] = n
		}
	}
	return context.WithValue(withLLMRecordID(ctx, recordID), provisionDiagnosticsKey{}, metadata)
}

// Keep both primary and fallback responses in persisted processor diagnostics.
// Error() deliberately omits Raw; nested multi-errors must be traversed explicitly.
func provisionStructuredFailures(err error) []map[string]any {
	failures := []map[string]any{}
	seen := map[string]bool{}
	var visit func(error)
	visit = func(current error) {
		if current == nil {
			return
		}
		if detail, ok := current.(*llmclients.StructuredOutputError); ok {
			key := detail.Error() + "\x00" + detail.Raw
			if !seen[key] {
				seen[key] = true
				failures = append(failures, map[string]any{"error": detail.Error(), "raw_response": detail.Raw})
			}
		}
		switch wrapped := current.(type) {
		case interface{ Unwrap() []error }:
			for _, child := range wrapped.Unwrap() {
				visit(child)
			}
		case interface{ Unwrap() error }:
			visit(wrapped.Unwrap())
		}
	}
	visit(err)
	return failures
}

func provisionDiagnosticMetadata(ctx context.Context) map[string]any {
	metadata, _ := ctx.Value(provisionDiagnosticsKey{}).(map[string]any)
	return metadata
}

func (p *ProvisionsProcessor) registerProvisionObjectTypes(ctx context.Context, items []any, model string) error {
	if p.ObjectTypes == nil {
		return nil
	}
	for _, item := range items {
		provision, ok := item.(map[string]any)
		if !ok {
			continue
		}
		for _, object := range objectItemsFromValue(provision["objects"]) {
			raw := firstNonEmptyTrimmed(asString(object["object_type_raw"]), asString(object["object_type"]))
			if normalizeObjectToken(raw) == "" {
				continue
			}
			source := ObjectTypeSource{RecordID: llmRecordIDFromContext(ctx), RunID: llmRunIDFromContext(ctx), Processor: p.Name(), Model: model, Prompt: p.PromptRef,
				Spans: normalizeSourceLineSpans(firstNonNil(object["source_line_spans"], provision["source_line_spans"]))}
			registered, err := p.ObjectTypes.Resolve(ctx, raw, source)
			if err != nil {
				return fmt.Errorf("register provision object type %q (record %d): %w", raw, source.RecordID, err)
			}
			object["object_type_raw"] = raw
			object["object_type"] = registered.Name
			if registered.Status == "proposed" && p.Logger != nil {
				attrs := []any{"record_id", source.RecordID, "run_id", source.RunID, "model_name", model, "prompt_name", p.PromptRef, "type_id", registered.ID, "raw_value", raw, "canonical_name", registered.Name, "source_spans", source.Spans}
				for key, value := range provisionDiagnosticMetadata(ctx) {
					attrs = append(attrs, key, value)
				}
				p.Logger.Info("provision object type awaits human approval", attrs...)
			}
		}
	}
	return nil
}
