package docreviews

import (
	"context"
	"testing"

	"github.com/chendingplano/shared/go/api/loggerutil"
)

// ── chunk reviewers ───────────────────────────────────────────────────────────

func TestTechnicalAccuracyReviewerProcessWindowDefaults(t *testing.T) {
	fake := &fakeJSONExtractor{
		out: map[string]any{"findings": []any{map[string]any{"title": "wrong formula"}}},
	}
	r := &technicalAccuracyReviewer{client: fake, logger: loggerutil.CreateDefaultLogger("TEST_P5_TECH")}
	findings := r.processWindow(context.Background(), 1, 0, ReviewerConfig{
		ModelName:  "m",
		PromptText: "t",
		PromptRef:  "prompt-review-technical-accuracy.md",
	}, technicalAccuracyWindow{inputJSON: `{"lines":[]}`, startLine: 1, endLine: 50})

	if len(findings) != 1 {
		t.Fatalf("len=%d want 1", len(findings))
	}
	f := findings[0]
	if f.Pass != "P5" {
		t.Errorf("Pass=%q want P5", f.Pass)
	}
	if f.Aspect != "technical_accuracy" {
		t.Errorf("Aspect=%q want technical_accuracy", f.Aspect)
	}
	if f.FindingType != "technical_error" {
		t.Errorf("FindingType=%q want technical_error", f.FindingType)
	}
	if f.Severity != "high" {
		t.Errorf("Severity=%q want high", f.Severity)
	}
	if f.Location != "1-50" {
		t.Errorf("Location=%q want 1-50", f.Location)
	}
	if len(fake.promptNames) != 1 || fake.promptNames[0] != "prompt-review-technical-accuracy.md" {
		t.Errorf("promptNames=%v want [prompt-review-technical-accuracy.md]", fake.promptNames)
	}
}

func TestTechnicalAccuracyReviewerMetadata(t *testing.T) {
	r := &technicalAccuracyReviewer{}
	if r.Name() != "technical_accuracy" {
		t.Errorf("Name=%q want technical_accuracy", r.Name())
	}
	if r.Group() != "P5" {
		t.Errorf("Group=%q want P5", r.Group())
	}
	if r.Strategy() != StrategyChunk {
		t.Errorf("Strategy=%v want StrategyChunk", r.Strategy())
	}
}

func TestAssumptionsReviewerProcessWindowDefaults(t *testing.T) {
	fake := &fakeJSONExtractor{
		out: map[string]any{"findings": []any{map[string]any{"title": "hidden assumption"}}},
	}
	r := &assumptionsReviewer{client: fake, logger: loggerutil.CreateDefaultLogger("TEST_P5_ASSUMPTIONS")}
	findings := r.processWindow(context.Background(), 2, 0, ReviewerConfig{
		ModelName: "m", PromptText: "t", PromptRef: "prompt-review-assumptions.md",
	}, assumptionsWindow{inputJSON: `{"lines":[]}`, startLine: 10, endLine: 60})

	if len(findings) != 1 {
		t.Fatalf("len=%d want 1", len(findings))
	}
	f := findings[0]
	if f.Pass != "P5" || f.Aspect != "assumptions" || f.FindingType != "undocumented_assumption" || f.Severity != "medium" {
		t.Errorf("got Pass=%q Aspect=%q FindingType=%q Severity=%q", f.Pass, f.Aspect, f.FindingType, f.Severity)
	}
	if f.Location != "10-60" {
		t.Errorf("Location=%q want 10-60", f.Location)
	}
}

func TestPrerequisitesReviewerProcessWindowDefaults(t *testing.T) {
	fake := &fakeJSONExtractor{
		out: map[string]any{"findings": []any{map[string]any{"title": "missing dep"}}},
	}
	r := &prerequisitesReviewer{client: fake, logger: loggerutil.CreateDefaultLogger("TEST_P5_PREREQ")}
	findings := r.processWindow(context.Background(), 3, 0, ReviewerConfig{
		ModelName: "m", PromptText: "t", PromptRef: "prompt-review-prerequisites.md",
	}, prerequisitesWindow{inputJSON: `{"lines":[]}`, startLine: 5, endLine: 30})

	if len(findings) != 1 {
		t.Fatalf("len=%d want 1", len(findings))
	}
	f := findings[0]
	if f.Pass != "P5" || f.Aspect != "prerequisites" || f.FindingType != "missing_prerequisite" || f.Severity != "medium" {
		t.Errorf("got Pass=%q Aspect=%q FindingType=%q Severity=%q", f.Pass, f.Aspect, f.FindingType, f.Severity)
	}
}

func TestSecurityReviewerProcessWindowDefaults(t *testing.T) {
	fake := &fakeJSONExtractor{
		out: map[string]any{"findings": []any{map[string]any{"title": "hardcoded key"}}},
	}
	r := &securityReviewer{client: fake, logger: loggerutil.CreateDefaultLogger("TEST_P5_SEC")}
	findings := r.processWindow(context.Background(), 4, 0, ReviewerConfig{
		ModelName: "m", PromptText: "t", PromptRef: "prompt-review-security.md",
	}, securityWindow{inputJSON: `{"lines":[]}`, startLine: 1, endLine: 200})

	if len(findings) != 1 {
		t.Fatalf("len=%d want 1", len(findings))
	}
	f := findings[0]
	if f.Pass != "P5" || f.Aspect != "security" || f.FindingType != "security_issue" || f.Severity != "high" {
		t.Errorf("got Pass=%q Aspect=%q FindingType=%q Severity=%q", f.Pass, f.Aspect, f.FindingType, f.Severity)
	}
	if len(fake.promptNames) != 1 || fake.promptNames[0] != "prompt-review-security.md" {
		t.Errorf("promptNames=%v want [prompt-review-security.md]", fake.promptNames)
	}
}

func TestPerformanceReviewerProcessWindowDefaults(t *testing.T) {
	fake := &fakeJSONExtractor{
		out: map[string]any{"findings": []any{map[string]any{"title": "no latency target"}}},
	}
	r := &performanceReviewer{client: fake, logger: loggerutil.CreateDefaultLogger("TEST_P5_PERF")}
	findings := r.processWindow(context.Background(), 5, 0, ReviewerConfig{
		ModelName: "m", PromptText: "t", PromptRef: "prompt-review-performance.md",
	}, performanceWindow{inputJSON: `{"lines":[]}`, startLine: 20, endLine: 80})

	if len(findings) != 1 {
		t.Fatalf("len=%d want 1", len(findings))
	}
	f := findings[0]
	if f.Pass != "P5" || f.Aspect != "performance" || f.FindingType != "performance_concern" || f.Severity != "medium" {
		t.Errorf("got Pass=%q Aspect=%q FindingType=%q Severity=%q", f.Pass, f.Aspect, f.FindingType, f.Severity)
	}
}

func TestErrorHandlingReviewerProcessWindowDefaults(t *testing.T) {
	fake := &fakeJSONExtractor{
		out: map[string]any{"findings": []any{map[string]any{"title": "no rollback"}}},
	}
	r := &errorHandlingReviewer{client: fake, logger: loggerutil.CreateDefaultLogger("TEST_P5_ERR")}
	findings := r.processWindow(context.Background(), 6, 0, ReviewerConfig{
		ModelName: "m", PromptText: "t", PromptRef: "prompt-review-error-handling.md",
	}, errorHandlingWindow{inputJSON: `{"lines":[]}`, startLine: 40, endLine: 120})

	if len(findings) != 1 {
		t.Fatalf("len=%d want 1", len(findings))
	}
	f := findings[0]
	if f.Pass != "P5" || f.Aspect != "error_handling" || f.FindingType != "missing_error_handling" || f.Severity != "medium" {
		t.Errorf("got Pass=%q Aspect=%q FindingType=%q Severity=%q", f.Pass, f.Aspect, f.FindingType, f.Severity)
	}
}

func TestLimitationsReviewerProcessWindowDefaults(t *testing.T) {
	fake := &fakeJSONExtractor{
		out: map[string]any{"findings": []any{map[string]any{"title": "scope not stated"}}},
	}
	r := &limitationsReviewer{client: fake, logger: loggerutil.CreateDefaultLogger("TEST_P5_LIM")}
	findings := r.processWindow(context.Background(), 7, 0, ReviewerConfig{
		ModelName: "m", PromptText: "t", PromptRef: "prompt-review-limitations.md",
	}, limitationsWindow{inputJSON: `{"lines":[]}`, startLine: 100, endLine: 200})

	if len(findings) != 1 {
		t.Fatalf("len=%d want 1", len(findings))
	}
	f := findings[0]
	if f.Pass != "P5" || f.Aspect != "limitations" || f.FindingType != "undocumented_limitation" || f.Severity != "medium" {
		t.Errorf("got Pass=%q Aspect=%q FindingType=%q Severity=%q", f.Pass, f.Aspect, f.FindingType, f.Severity)
	}
}

// ── document reviewers ────────────────────────────────────────────────────────

func TestStandardsComplianceReviewerProcessBlockDefaults(t *testing.T) {
	fake := &fakeJSONExtractor{
		out: map[string]any{"findings": []any{map[string]any{"title": "missing ISO clause"}}},
	}
	r := &standardsComplianceReviewer{client: fake, logger: loggerutil.CreateDefaultLogger("TEST_P5_STD")}
	findings := r.processBlock(context.Background(), 10, 0, 1, ReviewerConfig{
		ModelName: "m", PromptText: "t", PromptRef: "prompt-review-standards-compliance.md",
	}, pageBlock{inputJSON: `{"lines":[]}`, lineStart: 1, lineEnd: 400})

	if len(findings) != 1 {
		t.Fatalf("len=%d want 1", len(findings))
	}
	f := findings[0]
	if f.Pass != "P5" || f.Aspect != "standards_compliance" || f.FindingType != "standards_violation" || f.Severity != "high" {
		t.Errorf("got Pass=%q Aspect=%q FindingType=%q Severity=%q", f.Pass, f.Aspect, f.FindingType, f.Severity)
	}
	if f.Location != "1-400" {
		t.Errorf("Location=%q want 1-400", f.Location)
	}
	if len(fake.promptNames) != 1 || fake.promptNames[0] != "prompt-review-standards-compliance.md" {
		t.Errorf("promptNames=%v want [prompt-review-standards-compliance.md]", fake.promptNames)
	}
}

func TestStandardsComplianceReviewerMetadata(t *testing.T) {
	r := &standardsComplianceReviewer{}
	if r.Name() != "standards_compliance" {
		t.Errorf("Name=%q want standards_compliance", r.Name())
	}
	if r.Group() != "P5" {
		t.Errorf("Group=%q want P5", r.Group())
	}
	if r.Strategy() != StrategyDocument {
		t.Errorf("Strategy=%v want StrategyDocument", r.Strategy())
	}
}

func TestLegalComplianceReviewerProcessBlockDefaults(t *testing.T) {
	fake := &fakeJSONExtractor{
		out: map[string]any{"findings": []any{map[string]any{"title": "no copyright notice"}}},
	}
	r := &legalComplianceReviewer{client: fake, logger: loggerutil.CreateDefaultLogger("TEST_P5_LEGAL")}
	findings := r.processBlock(context.Background(), 11, 0, 1, ReviewerConfig{
		ModelName: "m", PromptText: "t", PromptRef: "prompt-review-legal-compliance.md",
	}, pageBlock{inputJSON: `{"lines":[]}`, lineStart: 5, lineEnd: 200})

	if len(findings) != 1 {
		t.Fatalf("len=%d want 1", len(findings))
	}
	f := findings[0]
	if f.Pass != "P5" || f.Aspect != "legal_compliance" || f.FindingType != "legal_compliance_issue" || f.Severity != "high" {
		t.Errorf("got Pass=%q Aspect=%q FindingType=%q Severity=%q", f.Pass, f.Aspect, f.FindingType, f.Severity)
	}
}

func TestRegulatoryComplianceReviewerProcessBlockDefaults(t *testing.T) {
	fake := &fakeJSONExtractor{
		out: map[string]any{"findings": []any{map[string]any{"title": "FDA clause missing"}}},
	}
	r := &regulatoryComplianceReviewer{client: fake, logger: loggerutil.CreateDefaultLogger("TEST_P5_REG")}
	findings := r.processBlock(context.Background(), 12, 0, 1, ReviewerConfig{
		ModelName: "m", PromptText: "t", PromptRef: "prompt-review-regulatory-compliance.md",
	}, pageBlock{inputJSON: `{"lines":[]}`, lineStart: 100, lineEnd: 300})

	if len(findings) != 1 {
		t.Fatalf("len=%d want 1", len(findings))
	}
	f := findings[0]
	if f.Pass != "P5" || f.Aspect != "regulatory_compliance" || f.FindingType != "regulatory_violation" || f.Severity != "high" {
		t.Errorf("got Pass=%q Aspect=%q FindingType=%q Severity=%q", f.Pass, f.Aspect, f.FindingType, f.Severity)
	}
}

func TestInternalPolicyReviewerProcessBlockDefaults(t *testing.T) {
	fake := &fakeJSONExtractor{
		out: map[string]any{"findings": []any{map[string]any{"title": "approval missing"}}},
	}
	r := &internalPolicyReviewer{client: fake, logger: loggerutil.CreateDefaultLogger("TEST_P5_POL")}
	findings := r.processBlock(context.Background(), 13, 0, 1, ReviewerConfig{
		ModelName: "m", PromptText: "t", PromptRef: "prompt-review-internal-policy.md",
	}, pageBlock{inputJSON: `{"lines":[]}`, lineStart: 1, lineEnd: 50})

	if len(findings) != 1 {
		t.Fatalf("len=%d want 1", len(findings))
	}
	f := findings[0]
	if f.Pass != "P5" || f.Aspect != "internal_policy" || f.FindingType != "policy_violation" || f.Severity != "high" {
		t.Errorf("got Pass=%q Aspect=%q FindingType=%q Severity=%q", f.Pass, f.Aspect, f.FindingType, f.Severity)
	}
}
