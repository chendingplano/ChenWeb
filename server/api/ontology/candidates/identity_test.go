package candidates

import "testing"

// TestIdentityKeyLabelAliasSwapMatches reproduces the bug doc's case
// (KnowledgeStore/doc-repo/bugs/202608/2026081101-...): two extraction
// passes over the same document produced payloads where label and the sole
// alias swapped, so Fingerprint differed but the underlying candidate is the
// same. IdentityKey must agree.
func TestIdentityKeyLabelAliasSwapMatches(t *testing.T) {
	first := []byte(`{
		"term_kind": "metric_definition",
		"label": "种子发芽指数",
		"aliases": ["发芽指数"]
	}`)
	second := []byte(`{
		"term_kind": "metric_definition",
		"label": "发芽指数",
		"aliases": ["种子发芽指数"]
	}`)
	a := IdentityKey("term", "measurement", first)
	b := IdentityKey("term", "measurement", second)
	if a == "" || b == "" {
		t.Fatalf("expected non-empty identity keys, got %q and %q", a, b)
	}
	if a != b {
		t.Fatalf("expected matching identity keys for swapped label/alias, got %q != %q", a, b)
	}
}

func TestIdentityKeyDiffersByTermKind(t *testing.T) {
	metric := []byte(`{"term_kind": "metric_definition", "label": "load"}`)
	concept := []byte(`{"term_kind": "concept", "label": "load"}`)
	if IdentityKey("term", "measurement", metric) == IdentityKey("term", "measurement", concept) {
		t.Fatal("expected different identity keys for different term_kind values")
	}
}

func TestIdentityKeyDiffersByModule(t *testing.T) {
	payload := []byte(`{"term_kind": "metric_definition", "label": "load"}`)
	if IdentityKey("term", "measurement", payload) == IdentityKey("term", "quantity", payload) {
		t.Fatal("expected different identity keys for different modules")
	}
}

func TestIdentityKeyIgnoresCaseAndWhitespace(t *testing.T) {
	a := IdentityKey("term", "measurement", []byte(`{"term_kind":"metric_definition","label":"Load"}`))
	b := IdentityKey("term", "measurement", []byte(`{"term_kind":"metric_definition","label":"  load  "}`))
	if a == "" || a != b {
		t.Fatalf("expected case/whitespace-insensitive match, got %q != %q", a, b)
	}
}

func TestIdentityKeyEmptyForNonTermCandidateKinds(t *testing.T) {
	payload := []byte(`{"term_kind": "metric_definition", "label": "load"}`)
	for _, kind := range []string{"axiom", "label", "mapping", "profile", "profile_rule", "module_change"} {
		if got := IdentityKey(kind, "measurement", payload); got != "" {
			t.Fatalf("expected empty identity key for candidate_kind %q, got %q", kind, got)
		}
	}
}

func TestIdentityKeyEmptyWithoutUsableLabel(t *testing.T) {
	cases := [][]byte{
		[]byte(`{"term_kind": "metric_definition"}`),
		[]byte(`{"term_kind": "metric_definition", "label": ""}`),
		[]byte(`{"term_kind": "metric_definition", "label": "   "}`),
	}
	for _, payload := range cases {
		if got := IdentityKey("term", "measurement", payload); got != "" {
			t.Fatalf("expected empty identity key for payload %s, got %q", payload, got)
		}
	}
}

func TestIdentityKeyMalformedPayloadDoesNotPanic(t *testing.T) {
	if got := IdentityKey("term", "measurement", []byte(`{not json`)); got != "" {
		t.Fatalf("expected empty identity key for malformed payload, got %q", got)
	}
}
