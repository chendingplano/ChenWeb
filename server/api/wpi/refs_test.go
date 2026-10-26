package wpi

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestParseRef(t *testing.T) {
	cases := []struct {
		in      string
		isRef   bool
		wantErr string
		root    string
	}{
		{in: "plain text"},
		{in: "$input_doc.user_name", isRef: true, root: RootInputDoc},
		{in: "$input_doc.address.city", isRef: true, root: RootInputDoc},
		{in: "$record.id", isRef: true, root: RootRecord},
		{in: "$args.id", isRef: true, root: RootArgs},
		{in: "$input_doc", isRef: true, wantErr: "needs a path"},
		{in: "$state.x", isRef: true, wantErr: "unknown reference root"},
		{in: "$input_doc.__proto__.x", isRef: true, wantErr: `"__proto__" is not allowed`},
		{in: "$input_doc.constructor", isRef: true, wantErr: "not allowed"},
		{in: "$input_doc.a-b", isRef: true, wantErr: "invalid path segment"},
		{in: "$input_doc.", isRef: true, wantErr: "invalid path segment"},
	}
	for _, c := range cases {
		ref, isRef, err := parseRef(c.in)
		if isRef != c.isRef {
			t.Errorf("%q: isRef = %v", c.in, isRef)
		}
		if c.wantErr != "" {
			if err == nil || !strings.Contains(err.Error(), c.wantErr) {
				t.Errorf("%q: err = %v, want %q", c.in, err, c.wantErr)
			}
			continue
		}
		if err != nil {
			t.Errorf("%q: unexpected error %v", c.in, err)
		}
		if ref.Root != c.root {
			t.Errorf("%q: root = %q", c.in, ref.Root)
		}
	}
}

func TestCheckValueRoots(t *testing.T) {
	if err := checkValue("$args.id", RootArgs); err != nil {
		t.Fatal(err)
	}
	if err := checkValue("$input_doc.id", RootArgs); err == nil || !strings.Contains(err.Error(), "$input_doc is not allowed") {
		t.Fatalf("err = %v", err)
	}
	if err := checkValue("$args.id"); err == nil {
		t.Fatal("a reference with no allowed roots must fail")
	}
	for _, literal := range []any{"x", json.Number("7"), true, nil, 1.5} {
		if err := checkValue(literal); err != nil {
			t.Fatalf("%v: %v", literal, err)
		}
	}
	if err := checkValue(map[string]any{}); err == nil {
		t.Fatal("objects are not values")
	}
}

func TestSubstituteArgs(t *testing.T) {
	args := map[string]any{"id": json.Number("7"), "filter": map[string]any{"city": "Paris"}}
	if got, err := substituteArgs("$args.id", args); err != nil || got != json.Number("7") {
		t.Fatalf("got %v, %v", got, err)
	}
	if got, err := substituteArgs("$args.filter.city", args); err != nil || got != "Paris" {
		t.Fatalf("got %v, %v", got, err)
	}
	if got, err := substituteArgs("literal", args); err != nil || got != "literal" {
		t.Fatalf("got %v, %v", got, err)
	}
	if _, err := substituteArgs("$args.missing", args); err == nil || !strings.Contains(err.Error(), `missing argument "missing"`) {
		t.Fatalf("err = %v", err)
	}
	if _, err := substituteArgs("$input_doc.id", args); err == nil {
		t.Fatal("$input_doc cannot be resolved on the server")
	}
	// A value that looks like SQL is just a value.
	if got, _ := substituteArgs("$args.id", map[string]any{"id": "1; DROP TABLE x"}); got != "1; DROP TABLE x" {
		t.Fatalf("got %v", got)
	}
}
