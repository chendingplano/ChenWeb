package agenttrace

import (
	"context"
	"strings"
	"testing"
)

func TestRegistryUsesRegisteredPlugin(t *testing.T) {
	reg := NewRegistry()
	plugin := PluginFunc{
		KindValue: "fake_agent",
		ParseFunc: func(_ context.Context, in ParseInput) (Trace, error) {
			return Trace{
				AgentKind: in.AgentKind,
				RunID:     in.RunID,
				Input:     strings.TrimSpace(in.Prompt),
				Output:    "done",
			}, nil
		},
	}

	if err := reg.Register(plugin); err != nil {
		t.Fatalf("Register returned error: %v", err)
	}

	trace, err := reg.Parse(context.Background(), ParseInput{
		AgentKind: "fake_agent",
		RunID:     "run-123",
		Prompt:    "  hello  ",
	})
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if trace.AgentKind != "fake_agent" || trace.RunID != "run-123" || trace.Input != "hello" || trace.Output != "done" {
		t.Fatalf("unexpected trace: %#v", trace)
	}
}

func TestDefaultRegistryIncludesKnownAgentPlugins(t *testing.T) {
	reg := NewDefaultRegistry()
	for _, kind := range []string{"codex", "claude_code"} {
		if _, ok := reg.Plugin(kind); !ok {
			t.Fatalf("expected plugin %q to be registered", kind)
		}
	}
}
