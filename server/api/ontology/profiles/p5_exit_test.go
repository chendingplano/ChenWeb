package profiles

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// P5ProfilesAcceptanceCriteria maps the profile-related acceptance criteria
// (9, 10, 14) from spec 2026080102 section 12 to named tests in this package.
// The doc-processing package's p5_exit_test.go references these pointers.
var P5ProfilesAcceptanceCriteria = map[int][]string{
	// 9. review-profile selection evaluates identical predicates
	9: {
		"TestSelectEvaluatesEachProfileOncePerDocumentTargetSubject",
		"TestLoadReleasedProfilesPinsReleaseIDsAndChecksums",
	},
	// 10. deterministic scope creation
	10: {
		"TestSelectRejectsDuplicateKnownFactProducers",
		"TestSelectSnapshotCarriesPinnedReleaseAndPredicateChecksums",
		"TestSelectMarksScopeIndeterminateWhenProfileClosedDimensionsIntersectRequest",
		"TestScopeCreationLeavesExplicitP4ScopesByteCompatible",
	},
	// 14. review snapshots reproducible after activation changes
	14: {
		"TestReviewReloadAfterActivationChange",
	},
}

// TestP5ProfilesAcceptanceCriteriaCoverage fails if any profile-related
// criterion lacks at least one named test, AND verifies each named test
// resolves to a real func Test* declaration in this package's _test.go files.
func TestP5ProfilesAcceptanceCriteriaCoverage(t *testing.T) {
	for _, criterion := range []int{9, 10, 14} {
		tests, ok := P5ProfilesAcceptanceCriteria[criterion]
		if !ok || len(tests) == 0 {
			t.Errorf("criterion %d lacks named test in profiles package", criterion)
		}
	}

	realTests := collectProfilesTestFuncNames(t)

	for criterion, tests := range P5ProfilesAcceptanceCriteria {
		for _, name := range tests {
			if !realTests[name] {
				t.Errorf("criterion %d: phantom test name %q (no func Test* found in profiles _test.go files)", criterion, name)
			}
		}
	}
}

func collectProfilesTestFuncNames(t *testing.T) map[string]bool {
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
