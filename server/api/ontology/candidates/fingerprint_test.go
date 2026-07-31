package candidates

import "testing"

func TestFingerprintDeterministicAcrossMapKeyOrder(t *testing.T) {
	a := []byte(`{"term_id":"core:assertion","term_kind":"class","definition":"x"}`)
	b := []byte(`{"definition":"x","term_kind":"class","term_id":"core:assertion"}`)
	fa, err := Fingerprint(a, "llm", "rec:1", "core")
	if err != nil {
		t.Fatalf("Fingerprint: %v", err)
	}
	fb, err := Fingerprint(b, "llm", "rec:1", "core")
	if err != nil {
		t.Fatalf("Fingerprint: %v", err)
	}
	if fa != fb {
		t.Fatalf("fingerprint depends on map key order: %s != %s", fa, fb)
	}
}

func TestFingerprintChangesWithSourceOrModule(t *testing.T) {
	payload := []byte(`{"term_id":"core:assertion"}`)
	base, err := Fingerprint(payload, "llm", "rec:1", "core")
	if err != nil {
		t.Fatalf("Fingerprint: %v", err)
	}
	cases := [][2]string{
		{"llm", "rec:2"}, // different source_ref
		{"import", "rec:1"}, // different source_type
		{"llm", "rec:1" + "\x00"}, // trailing whitespace normalized
	}
	for _, c := range cases {
		fp, err := Fingerprint(payload, c[0], c[1], "core")
		if err != nil {
			t.Fatalf("Fingerprint: %v", err)
		}
		if fp == base {
			t.Fatalf("expected different fingerprint for source %q/%q", c[0], c[1])
		}
	}
	// Different intended module changes the fingerprint.
	fpModule, err := Fingerprint(payload, "llm", "rec:1", "quantity")
	if err != nil {
		t.Fatalf("Fingerprint: %v", err)
	}
	if fpModule == base {
		t.Fatal("expected different fingerprint for different module")
	}
}

func TestFingerprintRejectsInvalidJSON(t *testing.T) {
	if _, err := Fingerprint([]byte(`{not json`), "llm", "rec:1", "core"); err == nil {
		t.Fatal("expected error for invalid payload JSON")
	}
}
