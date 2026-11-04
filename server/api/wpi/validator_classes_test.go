package wpi

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// Validator class rows of the shape the seed migration stores (OpenSpec
// wpi-primitive-validators, design D2), shortened.
const (
	integerValidator = `{"descriptor_id": "integer", "schema_version": "0.1", "code_module_id": "integer", "category": "primitive",
	 "description": {"en": "A whole number", "zh-cn": "整数"},
	 "value_types": ["number", "string"], "empty_values": ["absent", "null", "empty_string"], "empty_constraint": "empty",
	 "execution": "sync", "runs_in": "browser_and_backend",
	 "error_codes": {"integer.maximum": {"params": ["maximum"]}, "integer.minimum": {"params": ["minimum"]}},
	 "constraints": {"empty": {"type": "choice", "choices": ["pass", "fail"], "default_value": "pass", "help_tip": {"en": "Empty values"}},
	   "minimum": {"type": "integer", "help_tip": {"en": "The smallest value"}}, "maximum": {"type": "integer"}}}`
	ageValidator = `{"descriptor_id": "age", "schema_version": "0.1", "code_module_id": "integer", "category": "datetime",
	 "description": "An age in years",
	 "constraints": {"minimum": {"default_value": 0}, "maximum": {"default_value": 150, "instance_modifiable": false}}}`
	enumValidator = `{"descriptor_id": "enum", "schema_version": "0.1", "code_module_id": "enum", "category": "primitive",
	 "description": "One of a list", "error_codes": {"enum.not_allowed": {"params": ["allowed_values"]}},
	 "constraints": {"allowed_values": {"type": "json_array", "default_value": null}}}`
)

func validatorClasses() map[Kind]map[string]string {
	classes := testClasses()
	classes[KindValidator] = map[string]string{"integer": integerValidator, "age": ageValidator, "enum": enumValidator}
	return classes
}

func TestValidatorClassRows(t *testing.T) {
	expectIssue(t, ValidateDescriptor(KindValidator, "integer", decode(t, integerValidator)), "")
	expectIssue(t, ValidateDescriptor(KindValidator, "age", decode(t, ageValidator)), "")
	expectIssue(t, ValidateDescriptor(KindValidator, "enum", decode(t, enumValidator)), "")
	for _, c := range []struct{ from, to, want string }{
		{`"maximum": {"type": "integer"}`, `"maximum": {"type": "string"}`, `/constraints/maximum/type must be "integer"`},
		{`"maximum": {"type": "integer"}`, `"max_length": {}`, `/constraints/max_length validator code "integer" has no constraint "max_length"`},
		{`"schema_version": "0.1"`, `"schema_version": "0.4"`, `schema_version "0.4" is not supported by this interpreter for validator descriptors (supports "0.1")`},
		{`"code_module_id": "integer"`, `"code_module_id": "age"`, `/code_module_id validator code module "age" has no code`},
		{`"category": "primitive"`, `"category": "colours"`, `/category "colours" is not supported`},
		{`"execution": "sync"`, `"execution": "async"`, `/execution must be sync`},
		{`"integer.maximum": {"params": ["maximum"]}`, `"integer.maximum": {"params": ["limit"]}`, `/error_codes/integer.maximum/params must be [maximum]`},
		{`"integer.maximum": {"params": ["maximum"]}`, `"integer.size": {}`, `/error_codes/integer.size code module "integer" reports no error`},
		{`"default_value": "pass"`, `"default_value": "maybe"`, `/constraints/empty/default_value must be one of [pass fail]`},
		{`"minimum": {"type": "integer",`, `"minimum": {"type": "integer", "default_value": 200,`, `/constraints/maximum/default_value`},
		{`"maximum": {"type": "integer"}`, `"maximum": {"type": "integer", "instance_modifiable": false}`, `/constraints/maximum/default_value a constraint fixed by the class needs a default_value`},
		{`"category": "primitive",`, `"category": "primitive", "colour": "red",`, `/colour unknown attribute "colour"`},
	} {
		body := strings.Replace(integerValidator, c.from, c.to, 1)
		if body == integerValidator {
			t.Fatalf("replacement %q not applied", c.from)
		}
		if c.want == `/constraints/maximum/default_value` {
			// minimum 200 with a maximum default of 150 inverts the bounds.
			body = strings.Replace(body, `"maximum": {"type": "integer"}`, `"maximum": {"type": "integer", "default_value": 150}`, 1)
		}
		expectIssue(t, ValidateDescriptor(KindValidator, "integer", decode(t, body)), c.want)
	}
}

// inputWith is an input bound to $input_doc.age holding the given validators.
func inputWith(validators string) string {
	return pageWith(`{"element": "input", "label": "Age", "data_binding": "$input_doc.age", "validators": ` + validators + `}`)
}

func TestValidatorInstancesOnElements(t *testing.T) {
	classes := validatorClasses()
	expectIssue(t, writePage(t, classes, inputWith(`[{"validator": "integer", "constraints": {"minimum": 0, "maximum": 150}}]`)), "")
	expectIssue(t, writePage(t, classes, pageWith(`{"element": "select", "label": "Kind", "data_binding": "$input_doc.kind", "options": [{"value": "a", "label": "A"}], "validators": [{"validator": "enum", "constraints": {"allowed_values": ["a"]}}]}`)), "")
	for _, c := range []struct{ page, want string }{
		{pageWith(`{"element": "text", "text": "Hi", "validators": [{"validator": "integer"}]}`), `/validators code module "text" does not take validators`},
		{inputWith(`[{"validator": "age", "constraints": {"maximum": 120}}]`), `/validators/0/constraints/maximum constraint "maximum" is fixed by validator class "age"`},
		{inputWith(`[{"validator": "integer", "constraints": {"minimum": 10, "maximum": 5}}]`), `/validators/0/constraints/maximum must not be less than minimum`},
		{inputWith(`[{"validator": "integer", "constraints": {"max_length": 5}}]`), `/validators/0/constraints/max_length validator class "integer" has no constraint "max_length"`},
		{inputWith(`[{"validator": "integer", "constraints": {"minimum": "ten"}}]`), `/validators/0/constraints/minimum must be a whole number`},
		{inputWith(`[{"validator": "integer", "messages": {"integer.maximum": {"en": "At most {maximum}"}, "integer.type": "Whole"}}]`), `/validators/0/messages/integer.type validator class "integer" reports no error "integer.type"`},
		{inputWith(`[{"validator": "integer", "extra": 1}]`), `/validators/0/extra unknown attribute "extra"`},
		{inputWith(`[{"validator": "nope"}]`), `/validators/0/validator validator class "nope" has no active row to copy`},
		{inputWith(`{"validator": "integer"}`), `/validators must be an array`},
		{inputWith(`[{"validator": "enum"}]`), `constraint "allowed_values" has no default in this instance's copy of validator class "enum", so the instance must set it`},
	} {
		expectIssue(t, writePage(t, classes, c.page), c.want)
	}
	// An incomplete validator may be saved, as an incomplete element may.
	body := decode(t, inputWith(`[{"validator": "enum"}]`))
	if issues, err := fillClassCopies(context.Background(), "p", body, fixtureLoader(t, classes)); err != nil || len(issues) > 0 {
		t.Fatalf("copies: %v %v", issues, err)
	}
	expectIssue(t, validatePageForSave("p", body), "")
}

// A page save fills each validator instance's class copy, like an element's.
func TestValidatorCopiesFilled(t *testing.T) {
	body := decode(t, inputWith(`[{"validator": "integer"}, {"validator": "age", "class": {"descriptor_id": "age", "kept": true}}]`))
	if issues, err := fillClassCopies(context.Background(), "p", body, fixtureLoader(t, validatorClasses())); err != nil || len(issues) > 0 {
		t.Fatalf("copies: %v %v", issues, err)
	}
	list := body["panels"].(map[string]any)["main"].(map[string]any)["elem_list"].([]any)[0].(map[string]any)["validators"].([]any)
	if got := list[0].(map[string]any)[copyKey].(map[string]any)["code_module_id"]; got != "integer" {
		t.Fatalf("integer copy: %v", list[0])
	}
	if list[1].(map[string]any)[copyKey].(map[string]any)["kept"] != true {
		t.Fatalf("an existing copy was replaced: %v", list[1])
	}
}

// The backend runs every validator of a bound input, select or value editor on
// save (design D8), with the copy's defaults and the instance's own values.
func TestSaveRunsValidators(t *testing.T) {
	body := decode(t, inputWith(`[{"validator": "age", "constraints": {"minimum": 18}}, {"validator": "integer", "constraints": {"empty": "fail"}}]`))
	if issues, err := fillClassCopies(context.Background(), "p", body, fixtureLoader(t, validatorClasses())); err != nil || len(issues) > 0 {
		t.Fatalf("copies: %v %v", issues, err)
	}
	check := func(doc map[string]any) []string {
		_, vals := checkInputs(body, doc)
		var out []string
		for _, e := range vals["age"] {
			if e.Path != "/age" {
				t.Fatalf("path %q", e.Path)
			}
			b, _ := json.Marshal(e.Params)
			out = append(out, e.Code+string(b)+string(rune('0'+e.Validator)))
		}
		return out
	}
	for doc, want := range map[string]string{
		`{"age": 200}`:  `integer.maximum{"maximum":150}0`,
		`{"age": 10}`:   `integer.minimum{"minimum":18}0`,
		`{"age": "30"}`: ``,
		`{}`:            `integer.empty{}1`,
		`{"age": 1.5}`:  `integer.type{}0 integer.type{}1`,
	} {
		d := json.NewDecoder(strings.NewReader(doc))
		d.UseNumber()
		var m map[string]any
		if err := d.Decode(&m); err != nil {
			t.Fatal(err)
		}
		if got := strings.Join(check(m), " "); got != want {
			t.Fatalf("doc %s: %q, want %q", doc, got, want)
		}
	}
	// A read-only element is not checked.
	body["panels"].(map[string]any)["main"].(map[string]any)["elem_list"].([]any)[0].(map[string]any)["binding_mode"] = "read-only"
	if got := check(map[string]any{"age": json.Number("200")}); len(got) != 0 {
		t.Fatalf("read-only checked: %v", got)
	}
}

// Validator copies name validator modules, which the bundle checks against
// kb.code_modules like any other.
func TestValidatorModuleRefs(t *testing.T) {
	body := decode(t, inputWith(`[{"validator": "integer"}]`))
	if issues, err := fillClassCopies(context.Background(), "p", body, fixtureLoader(t, validatorClasses())); err != nil || len(issues) > 0 {
		t.Fatalf("copies: %v %v", issues, err)
	}
	_, els, _ := validatePage("p", body)
	found := false
	for _, r := range pageModules("p", body, els) {
		if r.kind == storedValidatorModule {
			found = r.module == "integer" && r.path == "/panels/main/elem_list/0/validators/0/class/code_module_id"
		}
	}
	if !found {
		t.Fatalf("no validator module ref: %+v", pageModules("p", body, els))
	}
}
