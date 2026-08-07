package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chendingplano/deepdoc/server/api/ontology/terminology"
)

func TestExecuteWritesCoverageJSONWithoutApproving(t *testing.T) {
	path := writeAcceptance(t, `{
  "schema_version": 1,
  "scope": "display",
  "corpus": [{"artifact_type":"spec","artifact_ids":["pilot"]}],
  "target_coverage": 1,
  "risk_terms": ["luminance"],
  "approver": "ontology-board",
  "target_seed_release": {"source":"iec-seed","release":"v1"}
}`)
	loader := func(_ context.Context, _ terminology.CoverageQuery) (terminology.CorpusData, error) {
		return terminology.CorpusData{
			Concepts:    []terminology.ConceptRecord{{ConceptID: "c1", PrefLabel: "Luminance", Scope: "display", Status: "active", ExactAuthority: true}},
			Surfaces:    []terminology.SurfaceRecord{{ConceptID: "c1", Surface: "luminance", NormKey: "luminance", Lang: "en", Scope: "display"}},
			Occurrences: []terminology.OccurrenceRecord{{OccurrenceID: 1, ArtifactType: "spec", ArtifactID: "pilot", Scope: "display", ConceptID: "c1", NormKey: "luminance"}},
		}, nil
	}
	var stdout, stderr bytes.Buffer
	code := execute(context.Background(), []string{"--acceptance", path, "--format", "json"}, &stdout, &stderr, loader)
	if code != 0 {
		t.Fatalf("code=%d stderr=%s", code, stderr.String())
	}
	var report terminology.CoverageReport
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatalf("decode output: %v\n%s", err, stdout.String())
	}
	if !report.Ready || report.Approval != "operator_required" || report.Approver != "ontology-board" {
		t.Fatalf("report=%+v", report)
	}
}

func TestExecuteWritesDeterministicSummary(t *testing.T) {
	path := writeAcceptance(t, `{
  "schema_version": 1,
  "scope": "display",
  "corpus": [{"artifact_type":"spec"}],
  "target_coverage": 0.9,
  "risk_terms": ["contrast"],
  "approver": "ontology-board",
  "target_seed_release": {"source":"iec-seed","release":"v1"}
}`)
	loader := func(_ context.Context, _ terminology.CoverageQuery) (terminology.CorpusData, error) {
		return terminology.CorpusData{
			Concepts:    []terminology.ConceptRecord{{ConceptID: "c1", PrefLabel: "Contrast", Scope: "display", Status: "active"}},
			Surfaces:    []terminology.SurfaceRecord{{ConceptID: "c1", Surface: "contrast", NormKey: "contrast", Lang: "en", Scope: "display"}},
			Occurrences: []terminology.OccurrenceRecord{{OccurrenceID: 1, ArtifactType: "spec", Scope: "display", ConceptID: "c1", NormKey: "contrast"}},
		}, nil
	}
	var stdout, stderr bytes.Buffer
	code := execute(context.Background(), []string{"--acceptance", path, "--format", "summary"}, &stdout, &stderr, loader)
	if code != 1 {
		t.Fatalf("code=%d stderr=%s", code, stderr.String())
	}
	want := "terminology coverage: NOT READY\nscope: display\nseed release: iec-seed@v1\ncoverage: 0.00% (0/1 eligible occurrences; target 90.00%)\nrisk terms: NOT MET (contrast)\nunresolved bilingual pairs: 1\ncontext-sensitive surfaces: 0\nhigh-frequency uncovered concepts: 1\napproval: operator_required (approver: ontology-board)\n"
	if stdout.String() != want {
		t.Fatalf("summary:\n%s\nwant:\n%s", stdout.String(), want)
	}
}

func TestExecuteRejectsUnknownAcceptanceFields(t *testing.T) {
	path := writeAcceptance(t, `{
  "schema_version": 1,
  "scope": "display",
  "corpus": [{"artifact_type":"spec"}],
  "target_coverage": 0.9,
  "approver": "ontology-board",
  "target_seed_release": {"source":"iec-seed","release":"v1"},
  "approved": true
}`)
	var stdout, stderr bytes.Buffer
	code := execute(context.Background(), []string{"--acceptance", path}, &stdout, &stderr, nil)
	if code != 2 || !strings.Contains(stderr.String(), "unknown field") {
		t.Fatalf("code=%d stderr=%q", code, stderr.String())
	}
}

func TestExecuteRejectsMissingGovernanceFields(t *testing.T) {
	path := writeAcceptance(t, `{
  "scope": "display",
  "corpus": [{"artifact_type":"spec"}],
  "target_coverage": 0.9,
  "target_seed_release": {"source":"iec-seed","release":"v1"}
}`)
	var stdout, stderr bytes.Buffer
	code := execute(context.Background(), []string{"--acceptance", path}, &stdout, &stderr, nil)
	if code != 2 || !strings.Contains(stderr.String(), "schema_version") {
		t.Fatalf("code=%d stderr=%q", code, stderr.String())
	}
}

func writeAcceptance(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "acceptance.json")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
