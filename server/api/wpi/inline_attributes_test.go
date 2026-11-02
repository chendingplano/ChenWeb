package wpi

import "testing"

func TestInlineElementAttributeDefinitions(t *testing.T) {
	for _, raw := range []string{
		`{"scope":"instance","help_tip":{"en":"Caption","zh-cn":"标题"},"default_value":"Button","value_editor":{"id":"localized-text","options":{}}}`,
		`{"scope":"class","default_value":"Fixed","value_editor":{"id":"text","options":{}}}`,
		`{"scope":"class","instance_modifiable":false,"default_value":"Fixed","value_editor":{"id":"text"}}`,
	} {
		body := decode(t, `{"descriptor_id":"button","schema_version":"0.4","code_module_id":"button","label":`+raw+`,"on_click":{"default_value":null}}`)
		if issues := ValidateDescriptor(KindElement, "button", body); len(issues) != 0 {
			t.Fatalf("%s: %+v", raw, issues)
		}
	}
}

func TestInlineAttributeDefinitionsRejectInvalidConfiguration(t *testing.T) {
	for _, raw := range []string{
		`{"scope":"other","default_value":"Button"}`,
		`{"scope":"instance","instance_modifiable":false,"default_value":"Button"}`,
		`{"scope":"class","instance_modifiable":true,"default_value":"Button"}`,
		`{"scope":"class","default_value":null}`,
		`{"scope":"class"}`,
		`{"scope":"instance","value_editor":{"id":"unknown"}}`,
		`{"scope":"instance","value_editor":{"id":"number","options":{"min":"bad"}}}`,
	} {
		body := decode(t, `{"descriptor_id":"button","schema_version":"0.4","code_module_id":"button","label":`+raw+`,"on_click":{"default_value":null}}`)
		if issues := ValidateDescriptor(KindElement, "button", body); len(issues) == 0 {
			t.Fatalf("accepted %s", raw)
		}
	}
}

func TestInlineClassScopeControlsRuntimeDefaults(t *testing.T) {
	row := decode(t, `{"descriptor_id":"button","schema_version":"0.4","code_module_id":"button","label":{"scope":"class","default_value":"Fixed","value_editor":{"id":"text","options":{}}}}`)
	if !isFixed(row, "label") {
		t.Fatal("inline Class scope must fix the attribute")
	}
	if got := mergeInstance(KindElement, row, map[string]any{"label": "Own"})["label"]; got != "Fixed" {
		t.Fatalf("got %v", got)
	}
}
