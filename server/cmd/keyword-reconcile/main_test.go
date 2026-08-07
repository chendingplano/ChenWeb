package main

import (
	"reflect"
	"strings"
	"testing"

	"github.com/chendingplano/deepdoc/server/api/ontology/keywords"
)

func TestCanonicalDeploymentKeysFailsClosed(t *testing.T) {
	for _, values := range [][]string{nil, {}, {""}, {"qudt", " qudt "}} {
		if _, err := canonicalDeploymentKeys(values); err == nil {
			t.Fatalf("expected %q to fail closed", values)
		}
	}
	want := []string{"qudt", "sirp"}
	got, err := canonicalDeploymentKeys([]string{" sirp ", "qudt"})
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, %v", got, err)
	}
}

func TestFormatStatsIncludesPartialFailureCounters(t *testing.T) {
	got := formatStats("metrics", keywords.ReconcileStats{Scanned: 2, Decided: 1, Failed: 1, Merged: 1})
	for _, field := range []string{"scope=metrics", "scanned=2", "decided=1", "failed=1", "merged=1", "deferred_unvalidated=0", "rejected=0", "no_candidate=0"} {
		if !strings.Contains(got, field) {
			t.Fatalf("%q missing %q", got, field)
		}
	}
}
