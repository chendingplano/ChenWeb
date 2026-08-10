package docprocessing

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// P5AcceptanceCriteria maps each of the 16 acceptance criteria from spec
// 2026080102 section 12 to named tests or live-proof pointers. This registry
// ensures no criterion lacks coverage.
//
// Criteria 1-4 are in semrules package tests (p5_exit_test.go there).
// Criteria 9-10 are in profiles package tests (p5_exit_test.go there).
// Criteria 15-16 are in doc-benchmark package tests and live proof (I2).
var P5AcceptanceCriteria = map[int][]string{
	// 1. three-valued logical truth tables and typed operator behavior
	1: {
		"semrules: TestEvaluateDocumentTruthTablesAndDecisionRelevantMissingPaths",
		"semrules: TestEvaluateDocumentTypedOperators",
	},
	// 2. exists distinguishes missing from unusable; structured traces
	2: {
		"semrules: TestEvaluateDocumentIndeterminateReasons",
		"semrules: TestFactStates",
	},
	// 3. validation rejects malformed or unknown predicates
	3: {
		"semrules: TestValidateRejectsInvalidDocuments",
	},
	// 4. required-fact extraction and specificity are deterministic
	4: {
		"semrules: TestAnalyzeReturnsStableRequirementsAndDistinctSpecificity",
	},
	// 5. legacy flat rule adapter parity
	5: {
		"TestPipelineBindingLegacyAdapterCanonicalPathsOrderAndChecksum",
		"TestBuildPipelineBindingFactSet",
	},
	// 6. migrated conditional bindings outrank store defaults
	6: {
		"TestPipelineBindingDR7DecisionTable",
	},
	// 7. processor gate effects alter enforced execution
	7: {
		"TestResolveProcessorGateIteratesRanksAndUsesEffectPrecedence",
		"TestBuildProcessorGateShadowPlanDoesNotChangeEffectiveProcessors",
		"TestResolveProcessorGateIndeterminateAlwaysHardFailsEvenInFallbackMode",
	},
	// 8. unresolved conflict blocks and raises exactly one alarm
	8: {
		"TestFinalizeRoutingPlan_OperatorFailureFailsClosedAndAlarms",
		"TestDedupeRoutingAlarmsKeepsExactlyOnePerKind",
	},
	// 9. review-profile selection evaluates identical predicates
	9: {
		"profiles: TestB3CrossConsumerPredicateFixtureIdenticalTruthTraceAndProvenance",
		"profiles: TestSelectEvaluatesEachProfileOncePerDocumentTargetSubject",
		"profiles: TestLoadReleasedProfilesPinsReleaseIDsAndChecksums",
	},
	// 10. deterministic scope creation
	10: {
		"profiles: TestSelectRejectsDuplicateKnownFactProducers",
		"profiles: TestSelectSnapshotCarriesPinnedReleaseAndPredicateChecksums",
		"profiles: TestSelectMarksScopeIndeterminateWhenProfileClosedDimensionsIntersectRequest",
		"profiles: TestScopeCreationLeavesExplicitP4ScopesByteCompatible",
	},
	// 11. classify_document runs only for unresolved decision-relevant facets
	11: {
		"TestResolverDecisionRelevantUnmaskedTier3Path",
		"TestResolverOneInvocationPerRecordExtractionRun",
		"TestClassifyDocumentIsRoutedNotMandatory",
		"TestResolverEnrichedFactsDoNotOverwriteKnown",
	},
	// 12. module proposals produce draft policy without changing active routing
	12: {
		"TestPolicyPromotionStoreImplementsInterface",
		"TestPolicyPromotionStoreRequiresReleaseID",
		"TestPolicyPromotionStoreRequiresChecksum",
	},
	// 13. a failed pipeline-version authoring transaction leaves the
	// previous active version untouched (ADR 2026081001 DR2 retired the
	// separate policy-activation step this criterion originally named;
	// atomic version authoring is the replacement mechanism).
	13: {
		"kbhandler: TestCreatePipelineMidTransactionFailureRollsBackSupersede",
		"kbhandler: TestCreatePipelineRejectsFailedClosureValidationBeforeTouchingDB",
	},
	// 14. execution/review snapshots reproducible after activation changes
	14: {
		"TestPersistedP5PlanReloadIgnoresLaterActivation",
		"profiles: TestReviewReloadAfterActivationChange",
	},
	// 15. benchmark report records cost, yield, recall/precision
	15: {
		"doc-benchmark: TestBenchmarkReportRoutingOffVsOnDiffers",
		"doc-benchmark: TestBenchmarkReportCostYieldSerializationRoundTrip",
		"doc-benchmark: TestBenchmarkReportRecallPrecisionSerializationRoundTrip",
		"I2: live proof with synthetic corpus",
	},
	// 16. suppressive decisions require approved unrevoked clearance
	16: {
		"TestFinalizeRoutingPlan_IncomparablePipelineIsSuppressiveAndGatedByClearance",
		"TestRoutingClearanceRevokeIsAppendOnly",
		"I2: live proof with clearance coverage",
	},
}

// packageTestDirs maps a cross-package pointer prefix to that package's
// directory, relative to this package's own directory (server/api/doc-processing).
// The coverage test parses the sibling package's _test.go files directly, so a
// cross-package pointer cannot silently rot (P5 review 2026080302 finding P5-22).
var packageTestDirs = map[string]string{
	"semrules":      "../ontology/semrules",
	"profiles":      "../ontology/profiles",
	"doc-benchmark": "../doc-benchmark",
	"kbhandler":     "../kbhandler",
}

// TestP5AcceptanceCriteriaCoverage fails if any criterion lacks at least one
// named test or live-proof pointer, AND verifies every named test resolves to
// a real func Test* declaration: same-package names against this package's
// _test.go files, and "pkg:"-prefixed names against the named package's own
// _test.go files. "I2:" live-proof pointers are the only exemption, asserted
// explicitly to be live-proof pointers rather than skipped incidentally.
func TestP5AcceptanceCriteriaCoverage(t *testing.T) {
	for criterion := 1; criterion <= 16; criterion++ {
		tests, ok := P5AcceptanceCriteria[criterion]
		if !ok || len(tests) == 0 {
			t.Errorf("criterion %d lacks named test/live-proof pointer", criterion)
		}
	}

	realTests := collectTestFuncNames(t)

	for criterion, tests := range P5AcceptanceCriteria {
		for _, name := range tests {
			if err := checkCriterionName(t, criterion, name, realTests); err != nil {
				t.Error(err)
			}
		}
	}
}

// checkCriterionName verifies one named test/live-proof pointer resolves.
// Same-package names are checked against realTests; cross-package names are
// resolved by parsing the named package's own _test.go files; "I2:" live-proof
// pointers are exempt only when they look like explicit live-proof pointers.
func checkCriterionName(t *testing.T, criterion int, name string, realTests map[string]bool) error {
	prefix, testName, prefixed := strings.Cut(name, ":")
	if !prefixed {
		if !realTests[strings.TrimSpace(name)] {
			return criterionErr(criterion, "phantom test name %q (no func Test* found in doc-processing _test.go files)", name)
		}
		return nil
	}
	if prefix == "I2" {
		if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(testName)), "live proof") {
			return criterionErr(criterion, "I2 pointer %q does not look like a live-proof pointer", name)
		}
		return nil
	}
	dir, ok := packageTestDirs[prefix]
	if !ok {
		return criterionErr(criterion, "unknown cross-package prefix %q in %q", prefix, name)
	}
	names := collectTestFuncNamesInDir(t, dir)
	if !names[strings.TrimSpace(testName)] {
		return criterionErr(criterion, "phantom cross-package test %q (no func Test* found in %s)", name, dir)
	}
	return nil
}

func criterionErr(criterion int, format string, args ...any) error {
	return &criterionError{criterion: criterion, format: format, args: args}
}

type criterionError struct {
	criterion int
	format    string
	args      []any
}

func (e *criterionError) Error() string {
	return "criterion " + strconv.Itoa(e.criterion) + ": " + fmt.Sprintf(e.format, e.args...)
}

// collectTestFuncNames parses all _test.go files in this package directory
// and returns the set of func Test* declarations.
func collectTestFuncNames(t *testing.T) map[string]bool {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	return collectTestFuncNamesInDir(t, dir)
}

// collectTestFuncNamesInDir parses all _test.go files in dir (relative to
// this package's working directory) and returns the set of func Test*
// declarations. It is used both for same-package names and, via
// checkCriterionName, for cross-package pointers (P5 review 2026080302
// finding P5-22: the old code skipped every name containing ":").
func collectTestFuncNamesInDir(t *testing.T, dir string) map[string]bool {
	t.Helper()
	fset := token.NewFileSet()
	names := map[string]bool{}
	for _, entry := range listTestFiles(t, dir) {
		file, err := parser.ParseFile(fset, entry, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", entry, err)
		}
		for _, decl := range file.Decls {
			fd, ok := decl.(*ast.FuncDecl)
			if !ok || fd.Name == nil {
				continue
			}
			if strings.HasPrefix(fd.Name.Name, "Test") {
				names[fd.Name.Name] = true
			}
		}
	}
	return names
}

func listTestFiles(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("readdir %s: %v", dir, err)
	}
	var out []string
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		out = append(out, filepath.Join(dir, entry.Name()))
	}
	return out
}

// TestP5AcceptanceCriteriaCoverageDetectsPhantomCrossPackagePointer proves
// the parser-check loophole is closed (P5 review 2026080302 finding P5-22): a
// deliberately phantom "semrules:"-prefixed name fails checkCriterionName, and
// an "I2:" pointer that is not an explicit live-proof pointer is rejected too.
func TestP5AcceptanceCriteriaCoverageDetectsPhantomCrossPackagePointer(t *testing.T) {
	if err := checkCriterionName(t, 9, "semrules: TestNoSuchTestExists", nil); err == nil {
		t.Fatal("expected a phantom semrules:-prefixed name to fail coverage")
	}
	if err := checkCriterionName(t, 9, "semrules: TestEvaluateDocumentTruthTablesAndDecisionRelevantMissingPaths", nil); err != nil {
		t.Fatalf("expected a real semrules:-prefixed name to resolve: %v", err)
	}
	if err := checkCriterionName(t, 16, "I2: not a live proof", nil); err == nil {
		t.Fatal("expected a non-live-proof I2 pointer to fail coverage")
	}
	if err := checkCriterionName(t, 16, "I2: live proof with synthetic corpus", nil); err != nil {
		t.Fatalf("expected an explicit I2 live-proof pointer to be exempt: %v", err)
	}
}
