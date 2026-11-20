package docsync

import (
	"encoding/json"
	"maps"
	"testing"
)

func TestBundleDigestIgnoresTransportIDsAndRowOrder(t *testing.T) {
	row := func(source, ref int64, key string) PortableRow {
		refJSON, _ := json.Marshal(ref)
		keyJSON, _ := json.Marshal(map[string]string{"prov_id": key})
		return PortableRow{
			SourceID:      source,
			Fields:        map[string]json.RawMessage{"metric_name": json.RawMessage(`"length"`), "provision_id": refJSON},
			ReferenceKeys: map[string]json.RawMessage{"provision_id": keyJSON},
		}
	}
	a := Bundle{Rows: map[string][]PortableRow{"metrics": {row(10, 15, "a"), row(20, 25, "b")}}}
	b := Bundle{Rows: map[string][]PortableRow{"metrics": {row(120, 125, "b"), row(110, 115, "a")}}}
	if da, db := digestForTest(t, a), digestForTest(t, b); da != db {
		t.Fatalf("same portable data produced %s and %s", da, db)
	}
	b.Rows["metrics"][1] = row(110, 115, "c")
	if da, db := digestForTest(t, a), digestForTest(t, b); da == db {
		t.Fatal("changing a referenced logical key left the digest unchanged")
	}
}

func digestForTest(t *testing.T, bundle Bundle) string {
	t.Helper()
	digest, err := bundle.Digest()
	if err != nil {
		t.Fatal(err)
	}
	return digest
}

func TestPortableArtifactID(t *testing.T) {
	for input, want := range map[string]string{
		"416_sum_1_0001":       "1234_sum_1_0001",
		"416_custom_name":      "416_custom_name",
		"metric:416:416_mtc_5": "metric:1234:1234_mtc_5",
		"obj_416_abcd":         "obj_1234_abcd",
		"1416_mtc_5":           "1416_mtc_5",
	} {
		if got := portableArtifactID(input, 416, 1234); got != want {
			t.Errorf("%q remapped to %q, want %q", input, got, want)
		}
	}
}

func TestMetricRowsCannotCrossOwnership(t *testing.T) {
	fields := map[string]json.RawMessage{"ref_mention_id": json.RawMessage(`12`)}
	if err := validateRowOwner("extract_metrics", "metrics", fields); err == nil {
		t.Fatal("extract_metrics accepted reference-owned row")
	}
	if err := validateRowOwner("extract_references", "metrics", fields); err != nil {
		t.Fatal(err)
	}
	unresolved := maps.Clone(fields)
	unresolved["ref_mention_id"] = json.RawMessage(`null`)
	if err := validateRowOwner("extract_metrics_simple", "metrics", unresolved); err != nil {
		t.Fatal(err)
	}
}

func TestPortableJSONNullSurvivesRowMaterialization(t *testing.T) {
	row := PortableRow{SourceID: 1, Fields: make(map[string]json.RawMessage)}
	table := tableContract{name: "example", id: "id", fields: "source payload", jsonFields: "payload"}
	row.Fields["source"] = json.RawMessage(`null`)
	row.Fields["payload"] = json.RawMessage(`null`)
	sqlNull := digestForTest(t, Bundle{Rows: map[string][]PortableRow{"example": {row}}})
	row.JSONNulls = []string{"payload"}
	if err := validatePortableRow(table, row); err != nil {
		t.Fatal(err)
	}
	if actual := insertValues([]string{"source", "payload"}, row.JSONNulls); actual != `"source",'null'::jsonb` {
		t.Fatalf("insert values %s do not preserve JSON null", actual)
	}
	if jsonNull := digestForTest(t, Bundle{Rows: map[string][]PortableRow{"example": {row}}}); jsonNull == sqlNull {
		t.Fatal("JSON and SQL null produced the same portable digest")
	}
	row.JSONNulls = []string{"source"}
	if err := validatePortableRow(table, row); err == nil {
		t.Fatal("JSON null accepted for a scalar column")
	}
}
