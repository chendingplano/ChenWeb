package agentservicehandler

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"
)

// knowledgeContextPromptFile tells Pi which knowledge stores a run may use.
// The wording lives in the prompt file; this package only supplies the data.
const knowledgeContextPromptFile = "prompt-agent-knowledge-context-v1.md"

const maxKnowledgeStoreNameRunes = 128

var ErrKnowledgeContextUnavailable = errors.New("knowledge context prompt is unavailable")

// GrantedKnowledgeStore is a knowledge store the user holds an active grant
// for, within the stores the profile allows.
type GrantedKnowledgeStore struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// GatewayKnowledgeContext is the run-scoped knowledge context sent to Pi.
// PromptContext is appended to the profile's system prompt; DefaultStoreID
// lets the gateway fill in knowledge_store_id when the model leaves it out.
// ChenWeb still enforces the store list through the run capability.
type GatewayKnowledgeContext struct {
	Stores         []GrantedKnowledgeStore `json:"stores"`
	DefaultStoreID string                  `json:"defaultStoreId,omitempty"`
	PromptContext  string                  `json:"promptContext"`
}

func loadKnowledgeContextTemplate(promptDir string) (*template.Template, error) {
	path := filepath.Join(promptDir, knowledgeContextPromptFile)
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read prompt %q: %w", path, err)
	}
	tmpl, err := template.New(knowledgeContextPromptFile).Option("missingkey=error").Parse(string(raw))
	if err != nil {
		return nil, fmt.Errorf("parse prompt %q: %w", path, err)
	}
	return tmpl, nil
}

// selectDefaultKnowledgeStore returns the ID of the granted store named by
// [frontend].default_knowledge_store. It returns "" when the name is empty or
// does not select exactly one granted store, so an ungranted default is never
// offered to Pi.
func selectDefaultKnowledgeStore(stores []GrantedKnowledgeStore, defaultName string) string {
	defaultName = strings.TrimSpace(defaultName)
	if defaultName == "" {
		return ""
	}
	match := ""
	for _, store := range stores {
		if store.Name != defaultName {
			continue
		}
		if match != "" {
			return ""
		}
		match = store.ID
	}
	return match
}

// BuildKnowledgeContext renders the knowledge context for one run. A run with
// no granted stores still gets a context, which tells Pi it has no knowledge
// tools.
func (r *ProfileRegistry) BuildKnowledgeContext(stores []GrantedKnowledgeStore, defaultStoreID string) (GatewayKnowledgeContext, error) {
	if r == nil || r.knowledgeContext == nil {
		return GatewayKnowledgeContext{}, ErrKnowledgeContextUnavailable
	}
	safe := make([]GrantedKnowledgeStore, 0, len(stores))
	for _, store := range stores {
		safe = append(safe, GrantedKnowledgeStore{ID: store.ID, Name: promptSafeStoreName(store.Name)})
	}
	var out bytes.Buffer
	data := struct {
		Stores         []GrantedKnowledgeStore
		DefaultStoreID string
	}{safe, defaultStoreID}
	if err := r.knowledgeContext.Execute(&out, data); err != nil {
		return GatewayKnowledgeContext{}, fmt.Errorf("render knowledge context: %w", err)
	}
	return GatewayKnowledgeContext{Stores: safe, DefaultStoreID: defaultStoreID, PromptContext: strings.TrimSpace(out.String())}, nil
}

// promptSafeStoreName keeps an administrator-entered store name on one short
// line so it cannot add its own instructions to the system prompt.
func promptSafeStoreName(name string) string {
	name = strings.Join(strings.Fields(strings.ReplaceAll(name, "`", "'")), " ")
	if runes := []rune(name); len(runes) > maxKnowledgeStoreNameRunes {
		name = string(runes[:maxKnowledgeStoreNameRunes])
	}
	return name
}
