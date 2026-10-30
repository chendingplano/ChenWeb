package wpi

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestInputRuleValidation(t *testing.T) {
	for _, c := range []struct {
		value any
		desc  map[string]any
		want  string
	}{
		{"", map[string]any{"required": true}, "required"},
		{"😀a", map[string]any{"max_length": float64(1)}, "max_length"},
		{float64(5), map[string]any{"input_type": "number", "validation": map[string]any{"min": float64(2), "step": float64(2)}}, "step"},
	} {
		got := checkInputValue(c.value, c.desc)
		found := false
		for _, x := range got {
			if x == c.want {
				found = true
			}
		}
		if !found {
			t.Errorf("value %v: got %v, want %s", c.value, got, c.want)
		}
	}
}

func TestInputRulesMatchWholeString(t *testing.T) {
	d := map[string]any{"validation": map[string]any{"pattern": "[a-z]+"}}
	if got := checkInputValue("abc", d); len(got) != 0 {
		t.Fatalf("valid pattern: %v", got)
	}
	if got := checkInputValue("1abc", d); len(got) != 1 || got[0] != "pattern" {
		t.Fatalf("invalid pattern: %v", got)
	}
}

func TestPageInputRulesUseClassCopy(t *testing.T) {
	page := decode(t, validFormPage)
	classes := testClasses()
	input := decode(t, classes[KindElement]["input"])
	input["required"] = map[string]any{"default_value": true}
	raw, _ := json.Marshal(input)
	classes[KindElement]["input"] = string(raw)
	issues, err := fillClassCopies(context.Background(), "customer-form", page, fixtureLoader(t, classes))
	if err != nil || len(issues) > 0 {
		t.Fatalf("copies: %v %v", issues, err)
	}
	_, els, _ := validatePage(str(page["descriptor_id"]), page)
	if len(els) == 0 {
		t.Fatal("no elements")
	}
	var binding string
	for _, el := range els {
		if str(el.copy["code_module_id"]) == "input" {
			binding = str(el.body["data_binding"])
			break
		}
	}
	if binding == "" {
		t.Fatal("no input")
	}
	// Page-level read-only makes this field exempt.
	if errs := checkPageInputs(page, map[string]any{}); len(errs) != 0 {
		t.Fatalf("readonly errors: %v", errs)
	}
	panels := page["panels"].(map[string]any)
	main := panels["main"].(map[string]any)
	entries := main["elem_list"].([]any)
	entries[0].(map[string]any)["binding_mode"] = "read-write"
	errs := checkPageInputs(page, map[string]any{})
	if len(errs[strings.TrimPrefix(binding, "$input_doc.")]) == 0 {
		t.Fatalf("required input not checked: %v", errs)
	}
}

func TestInputDescriptorMigration(t *testing.T) {
	data, err := os.ReadFile("../../../project_migrations/20261030000003_wpi_input_attributes.sql")
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(string(data), "$attrs$")
	if len(parts) < 3 {
		t.Fatal("missing attributes")
	}
	attrs := decode(t, parts[1])
	row := decode(t, testClasses()[KindElement]["input"])
	for k, v := range attrs {
		row[k] = v
	}
	if issues := ValidateDescriptor(KindElement, "input", row); len(issues) != 0 {
		t.Fatalf("invalid class: %+v", issues)
	}
	if issues := classWarnings("input", row); len(issues) != 0 {
		t.Fatalf("unsupported attributes: %+v", issues)
	}
}
