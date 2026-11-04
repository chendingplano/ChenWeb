package wpi

import (
	"regexp"
	"slices"
	"strings"
	"testing"
)

func TestValueEditorAttributes(t *testing.T) {
	for _, tc := range []struct{ attrs, want string }{
		{`"editor": "color", "label": {"en": "Accent", "zh-cn": "强调色"}, "data_binding": "$input_doc.theme.accent", "binding_mode": "read-write", "editor_options": {"theme": true}`, ""},
		{`"editor": "icon-picker", "binding_mode": "read-only"`, ""},
		{`"editor": "emoji-picker", "binding_mode": "read-write"`, ""},
		{`"editor": "string-editor", "editor_options": {"mode":"list", "languages":["en","zh-cn"], "emptyAsNull":false, "choices":["one","two"]}`, ""},
		{`"editor": "string-editor", "editor_options": {"emptyAsNull":"yes"}`, "/editor_options/emptyAsNull"},
		{`"editor": "string-editor", "editor_options": {"languages":["en","en"]}`, "/editor_options/languages"},
		{`"editor": "string-editor", "editor_options": {"languages":["bad tag"]}`, "/editor_options/languages"},
		{`"editor": "string-editor", "editor_options": {"mode":"bad"}`, "/editor_options/mode"},
		{`"editor": "slider"`, "/editor"},
		{`"editor": "date-picker"`, "/editor"},
		{`"editor_options": ["a", "b"]`, "/editor_options"},
		{`"data_binding": "theme.accent"`, "/data_binding"},
		{`"binding_mode": "write"`, "/binding_mode"},
	} {
		t.Run(tc.attrs, func(t *testing.T) {
			v := &validator{kind: KindElement, id: "value-picker"}
			v.element("value-editor", decode(t, `{`+tc.attrs+`}`), "")
			expectIssue(t, v.issues, tc.want)
		})
	}
}

// A class row as the class editor saves it: an active class draws a built
// editor; a draft may name one that is not built yet.
func TestValueEditorClassRow(t *testing.T) {
	row := func(editor, options string) string {
		return `{"descriptor_id": "x", "schema_version": "0.4", "code_module_id": "value-editor", "description": {"en": "X"},
		  "editor": {"default_value": ` + editor + `, "scope": "class"}, "label": {"default_value": "X"},
		  "binding_mode": {"default_value": "read-write"}, "data_binding": {}, "editor_options": {"default_value": ` + options + `}}`
	}
	for _, tc := range []struct {
		name, body string
		draft      bool
		want       string
	}{
		{"active built editor", row(`"color"`, `{}`), false, ""},
		{"active variant", row(`"alignment-picker"`, `{"choices": ["left", "center", "right"]}`), false, ""},
		{"unknown editor", row(`"slider"`, `{}`), false, "/editor"},
		{"active unbuilt editor", row(`"date-picker"`, `{}`), false, "/editor"},
		{"draft unbuilt editor", row(`"date-picker"`, `{}`), true, ""},
		{"draft unknown editor", row(`"slider"`, `{}`), true, "/editor"},
		{"options not an object", row(`"value-picker"`, `["a"]`), false, "/editor_options"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			expectIssue(t, validateElementClass(decode(t, tc.body), tc.draft), tc.want)
		})
	}
}

func TestValueEditorPages(t *testing.T) {
	classes := testClasses()
	classes[KindElement]["value-picker"] = `{"descriptor_id": "value-picker", "schema_version": "0.4", "code_module_id": "value-editor",
	  "editor": {"default_value": "value-picker", "scope": "class"}, "label": {"default_value": "Value"},
	  "binding_mode": {"default_value": "read-write"}, "data_binding": {}, "editor_options": {"default_value": {}}}`
	for _, tc := range []struct{ name, elements, want string }{
		{"unbound", `{"element": "value-picker"}`, ""},
		{"bound with choices", `{"element": "value-picker", "data_binding": "$input_doc.status", "editor_options": {"choices": ["draft", "final"]}}`, ""},
		{"options not an object", `{"element": "value-picker", "editor_options": ["draft", "final"]}`, "/panels/main/elem_list/0/editor_options"},
		{"fixed editor", `{"element": "value-picker", "editor": "color"}`, "/panels/main/elem_list/0/editor"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			expectIssue(t, writePage(t, classes, pageWith(tc.elements)), tc.want)
		})
	}
}

// The value-editor migration stores one class per value editor the class editor
// offers: drafts for those without code, valid active rows whose attributes the
// module supports for the rest, and their instances run with the defaults.
func TestSeedValueEditorClasses(t *testing.T) {
	rows := regexp.MustCompile(`(?s)\('([a-z0-9-]+)', '(active|draft)', \$wpi\$(\{.*?\})\$wpi\$`).FindAllStringSubmatch(readFile(t, valueEditorElementsMigration)+readFile(t, "../../../project_migrations/20261102000004_wpi_emoji_picker.sql"), -1)
	if len(rows) != len(classValueEditorIDs)-1 {
		t.Fatalf("found %d legacy classes, want %d", len(rows), len(classValueEditorIDs)-1)
	}
	classes := testClasses()
	editors := map[string]bool{}
	var elements []string
	for _, r := range rows {
		id, status, body := r[1], r[2], decode(t, r[3])
		editor := str(body["editor"].(map[string]any)["default_value"])
		editors[editor] = true
		if draft := slices.Contains(pendingValueEditors, editor); draft != (status == "draft") {
			t.Fatalf("class %s (editor %s) is %s", id, editor, status)
		}
		if status == "draft" {
			expectIssue(t, validateElementClass(body, true), "")
			continue
		}
		if issues := append(ValidateDescriptor(KindElement, id, body), validateElementClass(body, false)...); len(issues) > 0 {
			t.Fatalf("class %s: %+v", id, issues)
		}
		if warnings := classWarnings(id, body); len(warnings) > 0 {
			t.Fatalf("class %s warnings: %+v", id, warnings)
		}
		classes[KindElement][id] = r[3]
		elements = append(elements, `{"element": "`+id+`"}`)
	}
	for _, id := range classValueEditorIDs {
		if id == "string-editor" {
			continue
		} // seeded by its own later migration
		if !editors[id] {
			t.Fatalf("no class draws value editor %q", id)
		}
	}
	expectIssue(t, writePage(t, classes, pageWith(strings.Join(elements, ", "))), "")
}
