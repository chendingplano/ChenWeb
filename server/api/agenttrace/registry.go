package agenttrace

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
)

type Registry struct {
	mu      sync.RWMutex
	plugins map[string]Plugin
}

func NewRegistry() *Registry {
	return &Registry{plugins: map[string]Plugin{}}
}

func NewDefaultRegistry() *Registry {
	reg := NewRegistry()
	_ = reg.Register(CodexPlugin{})
	_ = reg.Register(ClaudeCodePlugin{})
	return reg
}

func (r *Registry) Register(plugin Plugin) error {
	if plugin == nil {
		return errors.New("agenttrace: nil plugin")
	}
	kind := normalizeKind(plugin.Kind())
	if kind == "" {
		return errors.New("agenttrace: plugin kind is empty")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.plugins[kind]; exists {
		return fmt.Errorf("agenttrace: plugin %q already registered", kind)
	}
	r.plugins[kind] = plugin
	return nil
}

func (r *Registry) Plugin(kind string) (Plugin, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	plugin, ok := r.plugins[normalizeKind(kind)]
	return plugin, ok
}

func (r *Registry) Parse(ctx context.Context, in ParseInput) (Trace, error) {
	plugin, ok := r.Plugin(in.AgentKind)
	if !ok {
		return Trace{}, fmt.Errorf("agenttrace: no plugin registered for %q", in.AgentKind)
	}
	trace, err := plugin.Parse(ctx, in)
	if err != nil {
		return Trace{}, err
	}
	if trace.AgentKind == "" {
		trace.AgentKind = normalizeKind(in.AgentKind)
	}
	if trace.RunID == "" {
		trace.RunID = in.RunID
	}
	if trace.Metadata == nil && len(in.Metadata) > 0 {
		trace.Metadata = in.Metadata
	}
	trace.Usage = trace.Usage.withTotal()
	return trace, nil
}

func normalizeKind(kind string) string {
	return strings.ToLower(strings.TrimSpace(kind))
}
