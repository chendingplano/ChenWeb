package docreviews

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveOutputLimitsDefaults(t *testing.T) {
	cfg, err := parseDocReviewConfig(nil)
	if err != nil {
		t.Fatal(err)
	}

	gotFindings, gotAnalyses := cfg.ResolveOutputLimits("grammar_spelling", 1)
	if gotFindings != 100 || gotAnalyses != 100 {
		t.Fatalf("limits = %d/%d, want 100/100", gotFindings, gotAnalyses)
	}
}

func TestResolveOutputLimitsPrecedenceAndPartialInheritance(t *testing.T) {
	cfg, err := parseDocReviewConfig([]byte(`
max_findings = [10,20,30]
max_analyses = [11,21,31]
[reviewers.grammar_spelling]
max_findings = [1,2,3]
`))
	if err != nil {
		t.Fatal(err)
	}

	gotFindings, gotAnalyses := cfg.ResolveOutputLimits("grammar_spelling", 2)
	if gotFindings != 2 || gotAnalyses != 21 {
		t.Fatalf("limits = %d/%d, want 2/21", gotFindings, gotAnalyses)
	}
}

func TestResolveOutputLimitsRootPartialOverrideInheritsBuiltIn(t *testing.T) {
	cfg, err := parseDocReviewConfig([]byte(`max_findings = [10,20,30]`))
	if err != nil {
		t.Fatal(err)
	}

	gotFindings, gotAnalyses := cfg.ResolveOutputLimits("grammar_spelling", 3)
	if gotFindings != 30 || gotAnalyses != 300 {
		t.Fatalf("limits = %d/%d, want 30/300", gotFindings, gotAnalyses)
	}
}

func TestResolveOutputLimitsReviewerAnalysesOverrideInheritsRootFindings(t *testing.T) {
	cfg, err := parseDocReviewConfig([]byte(`
max_findings = [10,20,30]
max_analyses = [11,21,31]
[reviewers.grammar_spelling]
max_analyses = [1,2,3]
`))
	if err != nil {
		t.Fatal(err)
	}

	gotFindings, gotAnalyses := cfg.ResolveOutputLimits("grammar_spelling", 2)
	if gotFindings != 20 || gotAnalyses != 2 {
		t.Fatalf("limits = %d/%d, want 20/2", gotFindings, gotAnalyses)
	}
}

func TestValidateOutputLimitsRejectsMalformedArrays(t *testing.T) {
	cases := []string{
		`max_findings=[1,2]`,
		`max_analyses=[1,0,3]`,
		"[reviewers.grammar_spelling]\nmax_findings=[1,-1,3]",
		"[reviewers.grammar_spelling]\nmax_analyses=[1,2,3,4]",
	}
	for _, raw := range cases {
		if _, err := parseDocReviewConfig([]byte(raw)); err == nil {
			t.Errorf("parseDocReviewConfig(%q) succeeded, want error", raw)
		}
	}
}

func TestLoadDocReviewConfigValidatesOutputLimits(t *testing.T) {
	path := filepath.Join(t.TempDir(), "doc-review.local.toml")
	if err := os.WriteFile(path, []byte(`max_findings=[1,2]`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DOC_REVIEW_CONFIG_FILE", path)

	if _, err := loadDocReviewConfig(); err == nil || !strings.Contains(err.Error(), "max_findings") {
		t.Fatalf("loadDocReviewConfig error=%v, want max_findings validation error", err)
	}
}
