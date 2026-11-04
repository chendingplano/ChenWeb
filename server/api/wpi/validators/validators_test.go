package validators

import (
	"bytes"
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"
)

// generic round-trips v through JSON, so Go and expected values compare alike.
func generic(t *testing.T, v any) any {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	var out any
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

// Every shared case passes; the TypeScript suite runs the same file.
func TestSharedCases(t *testing.T) {
	raw, err := os.ReadFile("validator-cases.json")
	if err != nil {
		t.Fatal(err)
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	var file struct {
		Cases []map[string]any `json:"cases"`
	}
	if err := d.Decode(&file); err != nil {
		t.Fatal(err)
	}
	if len(file.Cases) < 150 {
		t.Fatalf("only %d cases", len(file.Cases))
	}
	for _, c := range file.Cases {
		name, _ := c["name"].(string)
		t.Run(name, func(t *testing.T) {
			value, present := c["value"]
			if rep, ok := c["value_repeat"].([]any); ok {
				n, _ := rep[1].(json.Number).Int64()
				value, present = strings.Repeat(rep[0].(string), int(n)), true
			}
			if !present {
				value = Absent
			}
			constraints, _ := c["constraints"].(map[string]any)
			res := Run(c["validator"].(string), constraints, value, "/v")
			got := []any{}
			for _, e := range res.Errors {
				if e.Path != "/v" {
					t.Fatalf("error %s has path %q", e.Code, e.Path)
				}
				got = append(got, map[string]any{"code": e.Code, "params": e.Params})
			}
			if want := generic(t, c["errors"]); !reflect.DeepEqual(generic(t, got), want) {
				t.Fatalf("errors %v, want %v", generic(t, got), want)
			}
			if res.Valid != (len(res.Errors) == 0) {
				t.Fatalf("valid %v with %d errors", res.Valid, len(res.Errors))
			}
			if want, ok := c["normalized"]; ok {
				if !res.HasValue {
					t.Fatalf("no normalized value, want %v", want)
				}
				if g, w := generic(t, res.Value), generic(t, want); !reflect.DeepEqual(g, w) {
					t.Fatalf("normalized %#v, want %#v", g, w)
				}
			}
		})
	}
}

// Every module in modules.json has code here, and every code has cases.
func TestModulesHaveCodeAndCases(t *testing.T) {
	raw, _ := os.ReadFile("validator-cases.json")
	for id, m := range Modules {
		if r := Run(id, nil, Absent, ""); len(r.Errors) > 0 && r.Errors[0].Code == "validator.unknown" {
			t.Fatalf("module %q has no code", id)
		}
		if !bytes.Contains(raw, []byte(`"validator": "`+id+`"`)) {
			t.Fatalf("module %q has no shared cases", id)
		}
		if !strings.Contains(strings.Join(Categories, ","), m.Category) {
			t.Fatalf("module %q category %q", id, m.Category)
		}
	}
	if len(Modules) != 9 {
		t.Fatalf("%d modules", len(Modules))
	}
}

func TestCheckConstraints(t *testing.T) {
	for _, tc := range []struct {
		module string
		given  map[string]any
		want   []Problem
	}{
		{"integer", map[string]any{"minimum": json.Number("0"), "maximum": json.Number("150")}, nil},
		{"integer", map[string]any{"minimum": json.Number("10"), "maximum": json.Number("5")}, []Problem{{"maximum", "must not be less than minimum"}}},
		{"integer", map[string]any{"minimum": json.Number("1.5")}, []Problem{{"minimum", "must be a whole number"}}},
		{"integer", map[string]any{"max_length": json.Number("3")}, []Problem{{"max_length", `validator code "integer" has no constraint "max_length"`}}},
		{"integer", map[string]any{"multiple_of": json.Number("0")}, []Problem{{"multiple_of", "must be at least 1"}}},
		{"string", map[string]any{"min_length": json.Number("4"), "max_length": json.Number("3")}, []Problem{{"max_length", "must not be less than min_length"}}},
		{"string", map[string]any{"pattern": "(?i)abc"}, []Problem{{"pattern", "must be a regular expression both Go and JavaScript read the same way: " + errNonPortable.Error()}}},
		{"string", map[string]any{"pattern": "a("}, nil},
		{"string", map[string]any{"trim": "sideways"}, []Problem{{"trim", "must be one of [none both start end]"}}},
		{"decimal", map[string]any{"minimum": "0.01", "maximum": json.Number("1")}, nil},
		{"decimal", map[string]any{"minimum": "abc"}, []Problem{{"minimum", "must be a decimal number, written as text or a JSON number"}}},
		{"decimal", map[string]any{"precision": json.Number("2"), "scale": json.Number("3")}, []Problem{{"scale", "must not exceed precision"}}},
		{"decimal", map[string]any{"multiple_of": "-0.5"}, []Problem{{"multiple_of", "must be greater than 0"}}},
		{"bigint", map[string]any{"maximum": "1.5"}, []Problem{{"maximum", "must be a whole number, written as digits or a JSON number"}}},
		{"bigint", map[string]any{"maximum": json.Number("9007199254740993")}, []Problem{{"maximum", "must be written as text, because it is beyond the numbers JSON holds exactly"}}},
		{"enum", map[string]any{"allowed_values": "red"}, []Problem{{"allowed_values", "must be an array"}}},
		{"boolean", map[string]any{"must_be": true}, []Problem{{"must_be", "must be one of [either true false]"}}},
		{"nothing", nil, []Problem{{"", `validator code module "nothing" has no code in this interpreter`}}},
	} {
		got := CheckConstraints(tc.module, tc.given)
		if tc.module == "string" && tc.given["pattern"] == "a(" {
			if len(got) != 1 || got[0].Constraint != "pattern" {
				t.Fatalf("pattern a(: %v", got)
			}
			continue
		}
		if !reflect.DeepEqual(got, tc.want) {
			t.Fatalf("%s %v: %v, want %v", tc.module, tc.given, got, tc.want)
		}
	}
}

func TestDecimal(t *testing.T) {
	for in, want := range map[string]string{"0.10": "0.1", "-0": "0", "1e3": "1000", "1.5e-7": "0.00000015", ".5": "0.5", "+12.": "12"} {
		d, ok := parseDec(in)
		if !ok || d.String() != want {
			t.Fatalf("%s: %v %s, want %s", in, ok, d, want)
		}
	}
	for _, bad := range []string{"", ".", "1..2", "1e", "e3", "0x10", "1e10001", strings.Repeat("1", 1001)} {
		if _, ok := parseDec(bad); ok {
			t.Fatalf("%q parsed", bad)
		}
	}
	if fromFloat(1e21).String() != "1000000000000000000000" || fromFloat(1e-7).String() != "0.0000001" {
		t.Fatal("shortest form")
	}
}
