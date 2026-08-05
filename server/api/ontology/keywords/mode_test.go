package keywords

import "testing"

// §18.2: an unset KEYWORD_RESOLVER_MODE must behave as "off" — the gate
// fails closed (K6).
func TestResolverModeFromUnsetIsOff(t *testing.T) {
	tests := []struct{ raw, want string }{
		{"", "off"},
		{"   ", "off"},
		{"off", "off"},
		{"bogus", "off"},
		{"observe", "observe"},
		{" on ", "on"},
		{"ON", "off"}, // case-sensitive: unrecognized values fail closed
	}
	for _, tt := range tests {
		if got := resolverModeFrom(tt.raw); got != tt.want {
			t.Errorf("resolverModeFrom(%q) = %q, want %q", tt.raw, got, tt.want)
		}
	}
}

// A family whose ResolverMode was never set (empty string) must be inert —
// the same fail-closed rule K6 enforces at the REST layer.
func TestKeywordFamilyUnsetModeIsInert(t *testing.T) {
	kf := &KeywordFamily{} // ResolverMode == ""
	res, err := kf.ResolveSurface(nil, "test", "_")
	if err != nil {
		t.Fatalf("ResolveSurface (unset mode): %v", err)
	}
	if res.Verdict != "" {
		t.Errorf("expected zero resolution when ResolverMode is unset, got verdict %q", res.Verdict)
	}
	observed, err := kf.ObserveSurface(nil, "test", "_", "art", "ctx")
	if err != nil {
		t.Fatalf("ObserveSurface (unset mode): %v", err)
	}
	if observed != nil {
		t.Error("expected nil observation when ResolverMode is unset")
	}
	candidates, err := kf.CandidateNodes(nil, "test", "_")
	if err != nil {
		t.Fatalf("CandidateNodes (unset mode): %v", err)
	}
	if len(candidates) != 0 {
		t.Errorf("expected 0 candidates when ResolverMode is unset, got %d", len(candidates))
	}
}
