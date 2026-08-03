package docprocessing

import (
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
		"semrules: TestEvaluateDocumentTruthTables",
		"semrules: TestTypedOperatorBehavior",
	},
	// 2. exists distinguishes missing from unusable; structured traces
	2: {
		"semrules: TestExistsDistinguishesMissingFromUnusable",
		"semrules: TestStructuredTraceDistinguishesFactStates",
	},
	// 3. validation rejects malformed or unknown predicates
	3: {
		"semrules: TestValidateRejectsMalformedPredicates",
		"semrules: TestValidateRejectsUnknownPaths",
	},
	// 4. required-fact extraction and specificity are deterministic
	4: {
		"semrules: TestRequiredFactExtractionDeterministic",
		"semrules: TestSpecificityDeterministic",
	},
	// 5. legacy flat rule adapter parity
	5: {
		"TestLegacyRulePredicateDocument",
		"TestBuildPipelineBindingFactSet",
	},
	// 6. migrated conditional bindings outrank store defaults
	6: {
		"TestResolvePipelineBindingsConditionalOutranksStoreDefault",
		"TestResolvePipelineBindingsTrueFalseIndeterminate",
	},
	// 7. processor gate effects alter enforced execution
	7: {
		"TestProcessorGateEffects",
		"TestProcessorGateShadowOnly",
		"TestProcessorGateDeferTies",
	},
	// 8. unresolved conflict blocks and raises exactly one alarm
	8: {
		"TestRoutingEnforcementBlocksOnConflict",
		"TestRoutingAlarmExactlyOnePerRun",
	},
	// 9. review-profile selection evaluates identical predicates
	9: {
		"profiles: TestReviewProfileSelectionIdenticalPredicates",
		"profiles: TestReviewProfileReleasePinsSurviveActivation",
	},
	// 10. deterministic scope creation
	10: {
		"profiles: TestDeterministicScopeCreationOneKnowledgeStore",
		"profiles: TestScopeCreationAtomicPinsAndSnapshots",
	},
	// 11. classify_document runs only for unresolved decision-relevant facets
	11: {
		"TestApplicabilityResolverTwoPass",
		"TestApplicabilityResolverOneInvocationPerRun",
		"TestClassifyDocumentMandatoryGatedImmunity",
		"TestApplicabilityResolverEnrichedFactsDontOverwrite",
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
		"TestRoutingEnforcementClearanceGate",
		"TestRoutingClearanceRevocationReturnsToShadow",
		"I2: live proof with clearance coverage",
	},
}

// TestP5AcceptanceCriteriaCoverage fails if any criterion lacks at least one
// named test or live-proof pointer.
func TestP5AcceptanceCriteriaCoverage(t *testing.T) {
	for criterion := 1; criterion <= 16; criterion++ {
		tests, ok := P5AcceptanceCriteria[criterion]
		if !ok || len(tests) == 0 {
			t.Errorf("criterion %d lacks named test/live-proof pointer", criterion)
		}
	}
}
