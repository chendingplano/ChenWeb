package wpi

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func textIssues(raw string) []Issue {
	var e map[string]any
	dec := json.NewDecoder(strings.NewReader(raw))
	dec.UseNumber()
	if err := dec.Decode(&e); err != nil {
		panic(err)
	}
	v := &validator{kind: KindElement, id: "text"}
	v.element("text", e, "/text")
	return v.issues
}

func TestTextR2Values(t *testing.T) {
	for _, raw := range []string{
		`{"text":{"en":"Hello","zh-cn":"你好"},"label":"Caption","help_tip":"","icon":"info"}`,
		`{"text":"","font_family":"inherit","font_weight":"inherit","line_height":"normal","color":"inherit","background_color":"transparent"}`,
		`{"text":"x","font_family":"\"Noto Sans\", sans-serif","font_size":"1rem","font_weight":500,"line_height":1.6,"padding":"0px 2rem","color":"rgba(12,34,56,0.5)"}`,
	} {
		if issues := textIssues(raw); len(issues) > 0 {
			t.Errorf("valid %s: %+v", raw, issues)
		}
	}
	for _, pair := range [][2]string{
		{"text", `{"zh-cn":"缺少英文"}`}, {"text", `null`}, {"help_tip", `null`},
		{"icon", `"arbitrary-svg"`}, {"icon", `null`}, {"label_placement", `"side"`},
		{"label_horizontal_align", `"left"`}, {"label_vertical_align", `"start"`},
		{"label_max_width", `"0px"`}, {"label_horizontal_gap", `"-1px"`}, {"icon_size", `"0px"`},
		{"icon_gap", `"1rem"`}, {"icon_color", `"var(--secret)"`},
		{"font_family", `"sans-serif; color:red"`}, {"font_family", `"url(example)"`},
		{"font_size", `"0px"`}, {"font_size", `14`}, {"font_weight", `"500"`},
		{"font_weight", `550`}, {"font_style", `"slanted"`}, {"line_height", `0`},
		{"line_height", `"1.5"`}, {"padding", `"1px 2px 3px 4px 5px"`},
		{"padding", `"-1px"`}, {"text_align", `"left"`}, {"white_space", `"nowrap"`},
		{"color", `"notacolor"`}, {"color", `"rgb(256,0,0)"`}, {"color", `"rgba(0,0,0,1.1)"`},
		{"background_color", `"inherit"`}, {"color", `"hsl(0,0%,0%)"`},
	} {
		raw := `{"` + pair[0] + `":` + pair[1] + `}`
		if issues := textIssues(raw); len(issues) == 0 {
			t.Errorf("accepted invalid %s", raw)
		}
	}
}

func TestTextR2DraftCoverage(t *testing.T) {
	raw, err := os.ReadFile("../../../project_migrations/20261030000004_wpi_text_r2_draft.sql")
	if err != nil {
		t.Fatal(err)
	}
	pieces := strings.Split(string(raw), "$descriptor$")
	var body map[string]any
	dec := json.NewDecoder(strings.NewReader(pieces[1]))
	dec.UseNumber()
	if err := dec.Decode(&body); err != nil {
		t.Fatal(err)
	}
	if issues := ValidateDescriptor(KindElement, "text", body); len(issues) > 0 {
		t.Fatalf("R2 class: %+v", issues)
	}
	if issues := classWarnings("text", body); len(issues) > 0 {
		t.Fatalf("R2 unsupported attributes: %+v", issues)
	}
}

func TestTextR2ClientParity(t *testing.T) {
	raw, err := os.ReadFile("text-attribute-cases.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Name  string `json:"name"`
		Value any    `json:"value"`
		Valid bool   `json:"valid"`
	}
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.UseNumber()
	if err := dec.Decode(&cases); err != nil {
		t.Fatal(err)
	}
	for _, c := range cases {
		v := &validator{kind: KindElement, id: "text"}
		v.element("text", map[string]any{c.Name: c.Value}, "")
		if valid := len(v.issues) == 0; valid != c.Valid {
			t.Errorf("%s %v: valid=%v, issues=%+v", c.Name, c.Value, valid, v.issues)
		}
	}
}

func TestTextR2DefaultsAndCopies(t *testing.T) {
	raw, err := os.ReadFile("../../../project_migrations/20261030000004_wpi_text_r2_draft.sql")
	if err != nil {
		t.Fatal(err)
	}
	row := strings.Split(string(raw), "$descriptor$")[1]
	classes := testClasses()
	classes[KindElement]["text"] = row
	body := withCopies(t, classes, "p", decode(t, pageWith(`{"element":"text","text":""}`)))
	issues, els, _ := validatePage("p", body)
	expectIssue(t, issues, "")
	if els[0].body["font_size"] != "0.875rem" || els[0].body["help_tip"] != "" || els[0].body["icon"] != "" {
		t.Fatalf("defaults missing: %+v", els[0].body)
	}
	missing := withCopies(t, classes, "p", decode(t, pageWith(`{"element":"text"}`)))
	expectIssue(t, validatePageForSave("p", missing), "")
	issues, _, _ = validatePage("p", missing)
	expectIssue(t, issues, "exactly one of")
	both := withCopies(t, classes, "p", decode(t, pageWith(`{"element":"text","text":"","data_binding":"$input_doc.x"}`)))
	expectIssue(t, validatePageForSave("p", both), "exactly one of")
	// An old saved copy does not pick up new attributes from the registered class.
	legacy := withCopies(t, testClasses(), "p", decode(t, pageWith(`{"element":"text","text":"old"}`)))
	issues, els, _ = validatePage("p", legacy)
	expectIssue(t, issues, "")
	if _, exists := els[0].body["font_size"]; exists {
		t.Fatal("old copy unexpectedly acquired a new attribute")
	}
	class := decode(t, row)
	class["font_size"].(map[string]any)["instance_modifiable"] = false
	encoded, _ := json.Marshal(class)
	classes[KindElement]["text"] = string(encoded)
	fixed := withCopies(t, classes, "p", decode(t, pageWith(`{"element":"text","text":"x","font_size":"0.875rem"}`)))
	expectIssue(t, validatePageForSave("p", fixed), "is fixed")
	class["text"].(map[string]any)["default_value"] = ""
	encoded, _ = json.Marshal(class)
	classes[KindElement]["text"] = string(encoded)
	conflict := withCopies(t, classes, "p", decode(t, pageWith(`{"element":"text","data_binding":"$input_doc.x"}`)))
	expectIssue(t, validatePageForSave("p", conflict), "exactly one of")
}
