package wpi

import (
	"encoding/json"
	"reflect"
	"testing"
)

func intp(n int) *int { return &n }

var customerFields = []FieldMap{
	{Attribute: "id", Column: "id"},
	{Attribute: "user_name", Column: "user_name"},
	{Attribute: "email", Column: "email"},
}

func TestBuildLoadRecord(t *testing.T) {
	a := Action{Class: "load", Table: "kb.wpi_demo_customers", Result: "record", Fields: customerFields}
	q := buildLoad(a, map[string]any{"id": json.Number("7")})
	want := `SELECT "id", "user_name", "email" FROM "kb"."wpi_demo_customers" WHERE "id" = $1 LIMIT 1`
	if q.sql != want || !reflect.DeepEqual(q.args, []any{"7"}) {
		t.Fatalf("got %q %v", q.sql, q.args)
	}
}

func TestBuildLoadList(t *testing.T) {
	a := Action{Class: "load", Table: "people", Result: "list", Fields: customerFields[:2],
		OrderBy: []OrderBy{{Column: "user_name"}, {Column: "id", Direction: "desc"}}}
	q := buildLoad(a, map[string]any{"city": "Paris", "deleted_at": nil})
	want := `SELECT "id", "user_name" FROM "public"."people" WHERE "city" = $1 AND "deleted_at" IS NULL ORDER BY "user_name" ASC, "id" DESC LIMIT $2`
	if q.sql != want || !reflect.DeepEqual(q.args, []any{"Paris", defaultListLimit}) {
		t.Fatalf("got %q %v", q.sql, q.args)
	}
}

func TestListLimit(t *testing.T) {
	for _, c := range []struct {
		in   *int
		want int
	}{{nil, 200}, {intp(50), 50}, {intp(5000), 1000}, {intp(0), 200}} {
		if got := listLimit(c.in); got != c.want {
			t.Errorf("listLimit(%v) = %d", c.in, got)
		}
	}
}

func TestBuildLoadKeepsSQLInParameters(t *testing.T) {
	a := Action{Class: "load", Table: "people", Result: "record", Fields: customerFields[:1]}
	q := buildLoad(a, map[string]any{"id": "1; DROP TABLE x"})
	if q.sql != `SELECT "id" FROM "public"."people" WHERE "id" = $1 LIMIT 1` || q.args[0] != "1; DROP TABLE x" {
		t.Fatalf("got %q %v", q.sql, q.args)
	}
}

func TestBuildInsert(t *testing.T) {
	a := Action{Class: "save", Table: "kb.wpi_demo_customers", Mode: "upsert", Key: "id", Fields: customerFields[1:]}
	q := buildInsert(a, map[string]any{"user_name": "dana", "email": "d@example.com", "ignored": "x"})
	want := `INSERT INTO "kb"."wpi_demo_customers" ("user_name", "email") VALUES ($1, $2) RETURNING "id"`
	if q.sql != want || !reflect.DeepEqual(q.args, []any{"dana", "d@example.com"}) {
		t.Fatalf("got %q %v", q.sql, q.args)
	}
	if q := buildInsert(a, map[string]any{}); q.sql != `INSERT INTO "kb"."wpi_demo_customers" DEFAULT VALUES RETURNING "id"` {
		t.Fatal(q.sql)
	}
}

func TestBuildUpdate(t *testing.T) {
	// The key column is never SET, even when mapped.
	a := Action{Class: "save", Table: "people", Mode: "update", Key: "id", Fields: customerFields}
	q, ok := buildUpdate(a, map[string]any{"id": json.Number("7"), "email": "e@example.com", "tags": []any{"a"}}, json.Number("7"))
	want := `UPDATE "public"."people" SET "email" = $1 WHERE "id" = $2 RETURNING "id"`
	if !ok || q.sql != want || !reflect.DeepEqual(q.args, []any{"e@example.com", "7"}) {
		t.Fatalf("got %q %v", q.sql, q.args)
	}
	if _, ok := buildUpdate(a, map[string]any{"id": json.Number("7")}, json.Number("7")); ok {
		t.Fatal("an update with nothing to set must report ok=false")
	}
}

func TestParamValue(t *testing.T) {
	if got := paramValue(map[string]any{"a": json.Number("1")}); got != `{"a":1}` {
		t.Fatalf("got %v", got)
	}
	if got := paramValue(true); got != true {
		t.Fatalf("got %v", got)
	}
}

func TestCheckFields(t *testing.T) {
	fields := []FieldMap{
		{Attribute: "user_name", Column: "user_name", Required: true, Rules: &Rules{MaxLength: intp(5)}},
		{Attribute: "phone_number", Column: "phone_number", Rules: &Rules{Pattern: `^[0-9+ -]{6,20}$`}},
		{Attribute: "email", Column: "email", Required: true, Rules: &Rules{Format: "email"}},
	}
	cases := []struct {
		doc  map[string]any
		want map[string][]string
	}{
		{map[string]any{"user_name": "dana", "email": "d@example.com"}, map[string][]string{}},
		{map[string]any{"email": "d@example.com"}, map[string][]string{"user_name": {"required"}}},
		{map[string]any{"user_name": "   ", "email": "d@example.com"}, map[string][]string{"user_name": {"required"}}},
		{map[string]any{"user_name": "much too long", "email": "not-an-email"}, map[string][]string{"user_name": {"max_length"}, "email": {"format"}}},
		{map[string]any{"user_name": "dana", "email": "Dana <d@example.com>"}, map[string][]string{"email": {"format"}}},
		{map[string]any{"user_name": "dana", "email": "d@example.com", "phone_number": "abc"}, map[string][]string{"phone_number": {"pattern"}}},
		{map[string]any{"user_name": "dana", "email": "d@example.com", "phone_number": ""}, map[string][]string{}},
	}
	for _, c := range cases {
		if got := checkFields(fields, c.doc); !reflect.DeepEqual(got, c.want) {
			t.Errorf("doc %v: got %v, want %v", c.doc, got, c.want)
		}
	}
}
