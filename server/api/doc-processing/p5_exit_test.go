package docprocessing

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
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
		"TestResolveProcessorGateFallbackDefaultsAndDeferFingerprint",
	},
	// 8. unresolved conflict blocks and raises exactly one alarm
	8: {
		"TestFinalizeRoutingPlan_OperatorFailureFailsClosedAndAlarms",
		"TestDedupeRoutingAlarmsKeepsExactlyOnePerKind",
	},
	// 9. review-profile selection evaluates identical predicates
	9: {
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
		"TestClassifyDocumentMandatoryGatedImmunity",
		"TestResolverEnrichedFactsDoNotOverwriteKnown",
	},
	// 12. module proposals produce draft policy without changing active routing
	12: {
		"TestPolicyPromotionStoreImplementsInterface",
		"TestPolicyPromotionStoreRequiresReleaseID",
		"TestPolicyPromotionStoreRequiresChecksum",
	},
	// 13. failed policy compilation/activation leaves previous active version
	13: {
		"TestPolicyCompileFailureLeavesActiveUntouched",
		"TestPolicyActivationFailureRollback",
	},
	// 14. execution/review snapshots reproducible after activation changes
	14: {
		"TestPersistedP5PlanReloadIgnoresLaterActivation",
		"profiles: TestReviewReloadAfterActivationChange",
	},
	// 15. benchmark report records cost, yield, recall/precision
	15: {
		"doc-benchmark: TestBenchmarkReportRecordsCostYield",
		"doc-benchmark: TestBenchmarkReportRecordsRecallPrecision",
		"I2: live proof with synthetic corpus",
	},
	// 16. suppressive decisions require approved unrevoked clearance
	16: {
		"TestFinalizeRoutingPlan_IncomparablePipelineIsSuppressiveAndGatedByClearance",
		"TestRoutingClearanceRevokeIsAppendOnly",
		"I2: live proof with clearance coverage",
	},
}

// TestP5AcceptanceCriteriaCoverage fails if any criterion lacks at least one
// named test or live-proof pointer, AND verifies each named test (excluding
// cross-package references prefixed with "semrules:", "profiles:",
// "doc-benchmark:", and "I2:" live-proof pointers) resolves to a real
// func Test* declaration in this package's _test.go files.
func TestP5AcceptanceCriteriaCoverage(t *testing.T) {
	for criterion := 1; criterion <= 16; criterion++ {
		tests, ok := P5AcceptanceCriteria[criterion]
		if !ok || len(tests) == 0 {
			t.Errorf("criterion %d lacks named test/live-proof pointer", criterion)
		}
	}

	// Collect real func Test* names from this package's _test.go files.
	realTests := collectTestFuncNames(t)

	for criterion, tests := range P5AcceptanceCriteria {
		for _, name := range tests {
			// Skip cross-package references and live-proof pointers.
			if strings.Contains(name, ":") {
				continue
			}
			if !realTests[name] {
				t.Errorf("criterion %d: phantom test name %q (no func Test* found in doc-processing _test.go files)", criterion, name)
			}
		}
	}
}

// collectTestFuncNames parses all _test.go files in this package directory
// and returns the set of func Test* declarations.
func collectTestFuncNames(t *testing.T) map[string]bool {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("readdir %s: %v", dir, err)
	}
	fset := token.NewFileSet()
	names := map[string]bool{}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, filepath.Join(dir, entry.Name()), nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", entry.Name(), err)
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
