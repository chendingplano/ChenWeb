package docprocessing

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"

	llmclients "github.com/chendingplano/shared/go/api/llm"
)

type recordingObjectTypeRegistry struct {
	raw    string
	source ObjectTypeSource
	err    error
}

func TestProvisionFailureDiagnosticsKeepPrimaryAndFallbackRaw(t *testing.T) {
	primary := &llmclients.StructuredOutputError{Kind: llmclients.ErrStructuredOutputSchemaValidation, Raw: `{"type":"new"}`}
	fallback := &llmclients.StructuredOutputError{Kind: llmclients.ErrStructuredOutputProvider, Raw: `{"code":"AllocationQuota.FreeTierOnly"}`}
	got := provisionStructuredFailures(fmt.Errorf("primary: %w; fallback: %w", primary, fallback))
	if len(got) != 2 || got[0]["raw_response"] != primary.Raw || got[1]["raw_response"] != fallback.Raw {
		t.Fatalf("lost raw error diagnostics: %+v", got)
	}
}

func (r *recordingObjectTypeRegistry) Resolve(_ context.Context, raw string, source ObjectTypeSource) (ObjectType, error) {
	r.raw, r.source = raw, source
	return ObjectType{ID: 11, Name: normalizeObjectToken(raw), Status: "proposed"}, r.err
}

func TestProvisionObjectTypeRegistrationKeepsEvidence(t *testing.T) {
	registry := &recordingObjectTypeRegistry{}
	p := &ProvisionsProcessor{ObjectTypes: registry, PromptRef: "prompt-v8"}
	object := map[string]any{"object_type": "waste_stream", "object_type_raw": "Waste Stream", "source_line_spans": []string{"42-44"}}
	items := []any{map[string]any{"objects": []any{object}}}
	ctx := withLLMRunID(withLLMRecordID(context.Background(), 224), 172)
	if err := p.registerProvisionObjectTypes(ctx, items, "test-model"); err != nil {
		t.Fatal(err)
	}
	if registry.raw != "Waste Stream" || registry.source.RecordID != 224 || registry.source.RunID != 172 || registry.source.Model != "test-model" || registry.source.Prompt != "prompt-v8" || !reflect.DeepEqual(registry.source.Spans, []string{"42:44"}) {
		// normalizeSourceLineSpans uses the system's canonical colon notation.
		t.Fatalf("lost proposal evidence: %+v %q", registry.source, registry.raw)
	}
	if object["object_type"] != "waste_stream" || object["object_type_raw"] != "Waste Stream" {
		t.Fatalf("object=%+v", object)
	}
	registry.err = errors.New("database unavailable")
	if err := p.registerProvisionObjectTypes(ctx, items, "test-model"); err == nil {
		t.Fatal("registration error silently ignored")
	}
}

func TestProvisionDiagnosticsIncludeChunkAndSourceRange(t *testing.T) {
	ctx := withProvisionChunkDiagnostics(context.Background(), 224, 3, 11, Chunk{Lines: []MarkedLine{{Line: Line{LineNo: 42}}, {Line: Line{LineNo: 51}}}})
	got := provisionDiagnosticMetadata(ctx)
	if llmRecordIDFromContext(ctx) != 224 || got["chunk_index"] != 3 || got["chunk_total"] != 11 || got["source_line_start"] != 42 || got["source_line_end"] != 51 {
		t.Fatalf("metadata=%+v", got)
	}
}
