package wpi

import (
	"regexp"
	"strings"
	"testing"
)

func TestChatComposerAttributes(t *testing.T) {
	for _, tc := range []struct{ attrs, want string }{
		{`"placeholder": {"en": "Ask", "zh-cn": "提问"}, "rows": 4, "slash_commands": false, "toolbar": ["model", "agent"], "agents": ["pi", "Claude Code"], "default_agent": "pi", "default_model": "opus"`, ""},
		{`"toolbar": []`, ""},
		{`"placeholder": ""`, "/placeholder"},
		{`"rows": 0`, "/rows"},
		{`"rows": 21`, "/rows"},
		{`"rows": 2.5`, "/rows"},
		{`"rows": "3"`, "/rows"},
		{`"slash_commands": "yes"`, "/slash_commands"},
		{`"toolbar": ["output_format"]`, "/toolbar/0"},
		{`"toolbar": ["attach"]`, "/toolbar/0"},
		{`"toolbar": ["model", "model"]`, "/toolbar/1"},
		{`"toolbar": "agent"`, "/toolbar"},
		{`"agents": []`, "/agents"},
		{`"agents": ["OpenClaw"]`, "/agents/0"},
		{`"default_agent": "OpenClaw"`, "/default_agent"},
		{`"default_agent": null`, "/default_agent"},
		{`"default_model": "` + strings.Repeat("m", 129) + `"`, "/default_model"},
	} {
		t.Run(tc.attrs, func(t *testing.T) {
			v := &validator{kind: KindElement, id: "chat-composer"}
			v.element("chat-composer", decode(t, `{`+tc.attrs+`}`), "")
			expectIssue(t, v.issues, tc.want)
		})
	}
}

func TestChatPanelAttributes(t *testing.T) {
	for _, tc := range []struct{ attrs, want string }{
		{`"tabs": ["dialog", "chat"], "initial_tab": "dialog"`, ""},
		{`"tabs": ["chat"]`, ""},
		{`"tabs": []`, "/tabs"},
		{`"tabs": ["settings"]`, "/tabs/0"},
		{`"initial_tab": "settings"`, "/initial_tab"},
	} {
		t.Run(tc.attrs, func(t *testing.T) {
			v := &validator{kind: KindElement, id: "chat-panel"}
			v.element("chat-panel", decode(t, `{`+tc.attrs+`}`), "")
			expectIssue(t, v.issues, tc.want)
		})
	}
}

func chatClasses() map[Kind]map[string]string {
	classes := testClasses()
	classes[KindElement]["chat-composer"] = `{"descriptor_id": "chat-composer", "schema_version": "0.4", "code_module_id": "chat-composer",
	  "rows": {"default_value": 3}, "toolbar": {"default_value": ["agent", "model", "prompt", "text_editor", "new_session"]},
	  "agents": {}, "default_agent": {}, "default_model": {}, "placeholder": {}, "slash_commands": {"default_value": true}}`
	classes[KindElement]["claude-code-composer"] = `{"descriptor_id": "claude-code-composer", "schema_version": "0.4", "code_module_id": "chat-composer",
	  "agents": {"default_value": ["Claude Code"], "instance_modifiable": false}, "default_agent": {}}`
	classes[KindElement]["chat-panel"] = `{"descriptor_id": "chat-panel", "schema_version": "0.4", "code_module_id": "chat-panel",
	  "tabs": {"default_value": ["chat", "dialog"]}, "initial_tab": {}, "height": {"default_value": "640px"}}`
	return classes
}

func TestChatPages(t *testing.T) {
	for _, tc := range []struct{ name, elements, want string }{
		{"composer and panel", `{"element": "chat-composer", "default_agent": "Codex"}, {"element": "chat-panel", "tabs": ["chat"], "initial_tab": "chat"}`, ""},
		{"two composers", `{"element": "chat-composer"}, {"element": "chat-composer"}, {"element": "chat-panel"}`, "/panels/main/elem_list/1 a page can have only one chat-composer"},
		{"default agent not offered", `{"element": "chat-composer", "agents": ["pi"], "default_agent": "Codex"}`, "/panels/main/elem_list/0/default_agent"},
		{"default agent not in the class's agents", `{"element": "claude-code-composer", "default_agent": "pi"}`, "/panels/main/elem_list/0/default_agent"},
		{"fixed agents", `{"element": "claude-code-composer", "agents": ["pi"]}`, "/panels/main/elem_list/0/agents"},
		{"initial tab not shown", `{"element": "chat-panel", "tabs": ["dialog"], "initial_tab": "chat"}`, "/panels/main/elem_list/0/initial_tab"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			expectIssue(t, writePage(t, chatClasses(), pageWith(tc.elements)), tc.want)
		})
	}
}

func TestChatWarnings(t *testing.T) {
	for _, tc := range []struct{ name, elements, want string }{
		{"paired", `{"element": "chat-composer"}, {"element": "chat-panel"}`, ""},
		{"composer alone", `{"element": "chat-composer"}`, "replies are not shown"},
		{"panel alone", `{"element": "chat-panel"}`, "nothing can be sent"},
		{"no chat", `{"element": "text", "text": "Hello"}`, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := withCopies(t, chatClasses(), "p", decode(t, pageWith(tc.elements)))
			issues, els, _ := validatePage("p", body)
			if len(issues) > 0 {
				t.Fatalf("page issues: %+v", issues)
			}
			expectIssue(t, chatWarnings("p", els), tc.want)
		})
	}
}

// The classes the chat migration stores are valid active rows whose attributes
// their code modules support, and their instances run with the defaults.
func TestSeedChatClasses(t *testing.T) {
	bodies := regexp.MustCompile(`(?s)\$wpi\$(\{.*?\})\$wpi\$`).FindAllStringSubmatch(readFile(t, chatElementsMigration), -1)
	if len(bodies) != 2 {
		t.Fatalf("found %d class bodies, want 2", len(bodies))
	}
	classes := testClasses()
	for _, b := range bodies {
		body := decode(t, b[1])
		id := str(body["descriptor_id"])
		if issues := ValidateDescriptor(KindElement, id, body); len(issues) > 0 {
			t.Fatalf("class %s: %+v", id, issues)
		}
		if warnings := classWarnings(id, body); len(warnings) > 0 {
			t.Fatalf("class %s warnings: %+v", id, warnings)
		}
		classes[KindElement][id] = b[1]
	}
	expectIssue(t, writePage(t, classes, pageWith(`{"element": "chat-composer"}, {"element": "chat-panel"}`)), "")
}
