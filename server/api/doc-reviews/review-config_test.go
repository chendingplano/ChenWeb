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
	cases := []struct {
		name    string
		raw     string
		scope   string
		setting string
	}{
		{name: "root wrong length", raw: `max_findings=[1,2]`, scope: "root", setting: "max_findings"},
		{name: "root non-positive", raw: `max_analyses=[1,0,3]`, scope: "root", setting: "max_analyses"},
		{name: "root empty", raw: `max_findings=[]`, scope: "root", setting: "max_findings"},
		{name: "reviewer non-positive", raw: "[reviewers.grammar_spelling]\nmax_findings=[1,-1,3]", scope: "reviewer grammar_spelling", setting: "max_findings"},
		{name: "reviewer wrong length", raw: "[reviewers.grammar_spelling]\nmax_analyses=[1,2,3,4]", scope: "reviewer grammar_spelling", setting: "max_analyses"},
		{name: "reviewer empty", raw: "[reviewers.grammar_spelling]\nmax_analyses=[]", scope: "reviewer grammar_spelling", setting: "max_analyses"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseDocReviewConfig([]byte(tc.raw))
			if err == nil {
				t.Fatalf("parseDocReviewConfig(%q) succeeded, want error", tc.raw)
			}
			if !strings.Contains(err.Error(), tc.scope) || !strings.Contains(err.Error(), tc.setting) {
				t.Fatalf("parseDocReviewConfig(%q) error = %q, want scope %q and setting %q", tc.raw, err, tc.scope, tc.setting)
			}
		})
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
