package semrules

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestCanonicalizeIsStableAcrossObjectKeyOrderAndNumberSpelling(t *testing.T) {
	a := decodeDocument(t, `{"version":1,"expression":{"kind":"fact","path":"document.numeric_unit_density","op":"eq","value":1.00}}`)
	b := decodeDocument(t, `{"expression":{"value":1e0,"op":"eq","path":"document.numeric_unit_density","kind":"fact"},"version":1}`)
	aBytes, aSum, err := Canonicalize(a)
	if err != nil {
		t.Fatalf("Canonicalize(a): %v", err)
	}
	bBytes, bSum, err := Canonicalize(b)
	if err != nil {
		t.Fatalf("Canonicalize(b): %v", err)
	}
	if string(aBytes) != string(bBytes) || aSum != bSum {
		t.Fatalf("canonical results differ:\n%s\n%s\n%s\n%s", aBytes, bBytes, aSum, bSum)
	}
}

func decodeDocument(t *testing.T, raw string) Document {
	t.Helper()
	decoder := json.NewDecoder(bytes.NewBufferString(raw))
	decoder.UseNumber()
	var doc Document
	if err := decoder.Decode(&doc); err != nil {
		t.Fatalf("decode document: %v", err)
	}
	return doc
}

func TestCanonicalizeNormalizesEquivalentJSONNumbers(t *testing.T) {
	a := factDocument("document.numeric_unit_density", "in", []any{json.Number("1.00"), json.Number("2e0")})
	b := factDocument("document.numeric_unit_density", "in", []any{1, 2.0})
	aBytes, aSum, err := Canonicalize(a)
	if err != nil {
		t.Fatalf("Canonicalize(a): %v", err)
	}
	bBytes, bSum, err := Canonicalize(b)
	if err != nil {
		t.Fatalf("Canonicalize(b): %v", err)
	}
	if string(aBytes) != string(bBytes) || aSum != bSum {
		t.Fatalf("number normalization differs:\n%s\n%s", aBytes, bBytes)
	}
}

func TestCanonicalizePreservesAuthoredChildOrder(t *testing.T) {
	first := Predicate{Kind: "fact", Path: "document.doc_kind", Op: "eq", Value: "standard"}
	second := Predicate{Kind: "fact", Path: "document.domain", Op: "eq", Value: "mechanical"}
	a := Document{Version: 1, Expression: Predicate{Kind: "all", Items: []Predicate{first, second}}}
	b := Document{Version: 1, Expression: Predicate{Kind: "all", Items: []Predicate{second, first}}}

	aBytes, aSum, err := Canonicalize(a)
	if err != nil {
		t.Fatalf("Canonicalize(a): %v", err)
	}
	bBytes, bSum, err := Canonicalize(b)
	if err != nil {
		t.Fatalf("Canonicalize(b): %v", err)
	}
	if string(aBytes) == string(bBytes) || aSum == bSum {
		t.Fatal("canonicalization erased authored child order")
	}
}

func TestCanonicalizeDoesNotMutateDocument(t *testing.T) {
	doc := Document{Version: 1, Expression: Predicate{Kind: "all", Items: []Predicate{{
		Kind: "fact", Path: "document.numeric_unit_density", Op: "eq", Value: json.Number("1.00"),
	}}}}
	if _, _, err := Canonicalize(doc); err != nil {
		t.Fatalf("Canonicalize: %v", err)
	}
	if got := doc.Expression.Items[0].Value.(json.Number); got != json.Number("1.00") {
		t.Fatalf("Canonicalize mutated input value to %q", got)
	}
}

func TestCanonicalizeRejectsInvalidDocument(t *testing.T) {
	if _, _, err := Canonicalize(Document{Version: 99}); err == nil {
		t.Fatal("Canonicalize succeeded for invalid document")
	}
}
