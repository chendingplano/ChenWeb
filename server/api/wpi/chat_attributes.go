package wpi

import (
	"fmt"
	"slices"
	"unicode/utf8"
)

// Checks of the chat-composer and chat-panel element code modules (KnowledgeStore
// doc-repo/code_modules/interpreter-chat-composer.md and interpreter-chat-panel.md).
// The two share the chat of their page instance; a page has at most one composer.

var (
	chatComposerAttributes = []string{"placeholder", "rows", "slash_commands", "toolbar", "agents", "default_agent", "default_model", "message_box_height", "toolbar_height"}
	chatPanelAttributes    = []string{"tabs", "initial_tab"}
	// chatAgents are the assistants a chat may offer, in their default order: the
	// harness-backed ones (HARNESS_BY_AGENT in web harnessFrontendService.ts).
	chatAgents   = []string{"Claude Code", "Codex", "DeepSeek Harness", "pi"}
	chatControls = []string{"agent", "model", "prompt", "text_editor", "new_session"}
	chatTabs     = []string{"chat", "dialog"}
)

func (v *validator) chatComposerAttributes(e map[string]any, path string) {
	v.text(e, "placeholder", path)
	v.length(e, "message_box_height", path, lengthKind{percent: true})
	v.length(e, "toolbar_height", path, lengthKind{percent: true})
	if val, ok := e["rows"]; ok {
		if n, ok := asInt(val); !ok || n < 1 || n > 20 {
			v.errf(path+"/rows", "must be an integer from 1 to 20")
		}
	}
	v.boolean(e, "slash_commands", path)
	v.tokenList(e, "toolbar", path, chatControls, 0)
	v.tokenList(e, "agents", path, chatAgents, 1)
	v.oneOf(e, "default_agent", path, chatAgents)
	if s, ok := v.str(e, "default_model", path); ok && utf8.RuneCountInString(s) > 128 {
		v.errf(path+"/default_model", "must be at most 128 characters")
	}
}

func (v *validator) chatPanelAttributes(e map[string]any, path string) {
	v.tokenList(e, "tabs", path, chatTabs, 1)
	v.oneOf(e, "initial_tab", path, chatTabs)
}

// tokenList checks an optional array of distinct tokens from allowed, with at
// least min of them.
func (v *validator) tokenList(obj map[string]any, key, path string, allowed []string, min int) {
	val, present := obj[key]
	if !present {
		return
	}
	p := path + "/" + key
	items, ok := val.([]any)
	if !ok {
		v.errf(p, "must be an array of %v", allowed)
		return
	}
	if len(items) < min {
		v.errf(p, "must list at least %d of %v", min, allowed)
	}
	seen := map[string]bool{}
	for i, item := range items {
		s, ok := item.(string)
		switch {
		case !ok || !slices.Contains(allowed, s):
			v.errf(fmt.Sprintf("%s/%d", p, i), "%v is not supported (supported: %v)", item, allowed)
		case seen[s]:
			v.errf(fmt.Sprintf("%s/%d", p, i), "%q is listed twice", s)
		}
		seen[s] = true
	}
}

// chatComplete checks a merged chat instance: its default assistant is one it
// offers, and its first tab is one it shows.
func (v *validator) chatComplete(module string, merged map[string]any, path string) {
	member := func(key, listKey string, all []string) {
		s, ok := merged[key].(string)
		if !ok {
			return
		}
		list := all
		if items, ok := merged[listKey].([]any); ok {
			list = nil
			for _, item := range items {
				if t, ok := item.(string); ok {
					list = append(list, t)
				}
			}
		}
		if !slices.Contains(list, s) {
			v.errf(path+"/"+key, "%q is not one of %s %v", s, listKey, list)
		}
	}
	switch module {
	case "chat-composer":
		member("default_agent", "agents", chatAgents)
	case "chat-panel":
		member("initial_tab", "tabs", chatTabs)
	}
}

// oneChatComposer checks that a page has at most one chat-composer: the chat of
// a page instance has one selected assistant and one draft.
func (v *validator) oneChatComposer() {
	first := ""
	for _, e := range v.els {
		if str(e.copy["code_module_id"]) != "chat-composer" {
			continue
		}
		if first == "" {
			first = e.path
			continue
		}
		v.errf(e.path, "a page can have only one chat-composer; the first is at %s", first)
	}
}

// chatWarnings warns about a chat-composer without a chat-panel on its page,
// whose replies are not shown, and about a chat-panel without a composer.
func chatWarnings(pageID string, els []mergedElement) []Issue {
	var composer, panel string
	for _, e := range els {
		switch str(e.copy["code_module_id"]) {
		case "chat-composer":
			composer = e.path
		case "chat-panel":
			panel = e.path
		}
	}
	switch {
	case composer != "" && panel == "":
		return []Issue{{KindPage, pageID, composer, "the page has a chat-composer but no chat-panel, so replies are not shown"}}
	case panel != "" && composer == "":
		return []Issue{{KindPage, pageID, panel, "the page has a chat-panel but no chat-composer, so nothing can be sent"}}
	}
	return nil
}
