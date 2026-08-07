package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/chendingplano/deepdoc/server/api/ontology/terminology"
)

type fakeApp struct {
	importResult  terminology.ImportResult
	importErr     error
	diffResult    terminology.ReleaseDiff
	diffErr       error
	activateErr   error
	rollbackErr   error
	importPath    string
	basePath      string
	candidatePath string
	deploymentKey string
	changedBy     string
}

func (f *fakeApp) importManifest(_ context.Context, manifestPath string) (terminology.ImportResult, error) {
	f.importPath = manifestPath
	return f.importResult, f.importErr
}

func (f *fakeApp) diffManifests(_ context.Context, basePath, candidatePath string) (terminology.ReleaseDiff, error) {
	f.basePath = basePath
	f.candidatePath = candidatePath
	return f.diffResult, f.diffErr
}

func (f *fakeApp) activate(_ context.Context, deploymentKey, _, _, changedBy string) error {
	f.deploymentKey = deploymentKey
	f.changedBy = changedBy
	return f.activateErr
}

func (f *fakeApp) rollback(_ context.Context, deploymentKey, changedBy string) error {
	f.deploymentKey = deploymentKey
	f.changedBy = changedBy
	return f.rollbackErr
}

func TestExecuteImportWritesJSON(t *testing.T) {
	app := &fakeApp{importResult: terminology.ImportResult{
		Source: "iec-seed", Release: "v1",
		Counts: terminology.ImportCounts{Entries: 2, Labels: 4, NegativeDecisions: 1, Artifacts: 1},
	}}
	var stdout, stderr bytes.Buffer
	code := execute(context.Background(), []string{"import", "--manifest", "/tmp/manifest.json"}, &stdout, &stderr, app)
	if code != 0 {
		t.Fatalf("code=%d stderr=%s", code, stderr.String())
	}
	if app.importPath != "/tmp/manifest.json" {
		t.Fatalf("importPath=%q", app.importPath)
	}
	var result terminology.ImportResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("decode output: %v\n%s", err, stdout.String())
	}
	if result.Source != "iec-seed" || result.Counts.Entries != 2 {
		t.Fatalf("result=%+v", result)
	}
}

func TestExecuteImportRequiresManifest(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := execute(context.Background(), []string{"import"}, &stdout, &stderr, &fakeApp{})
	if code != 2 || !strings.Contains(stderr.String(), "--manifest is required") {
		t.Fatalf("code=%d stderr=%q", code, stderr.String())
	}
}

func TestExecuteImportPropagatesFailure(t *testing.T) {
	app := &fakeApp{importErr: errors.New("checksum mismatch")}
	var stdout, stderr bytes.Buffer
	code := execute(context.Background(), []string{"import", "--manifest", "/tmp/manifest.json"}, &stdout, &stderr, app)
	if code != 3 || !strings.Contains(stderr.String(), "checksum mismatch") {
		t.Fatalf("code=%d stderr=%q", code, stderr.String())
	}
}

func TestExecuteDiffWritesJSON(t *testing.T) {
	app := &fakeApp{diffResult: terminology.ReleaseDiff{
		Source: "iec-seed", BaseRelease: "v1", CandidateRelease: "v2",
		AddedEntries:  []terminology.DiffItem{{Key: "845-22-060", To: "current"}},
		PolicyChanges: []terminology.PolicyChange{{Field: "license", From: "CC0-1.0", To: "CC-BY-4.0"}},
	}}
	var stdout, stderr bytes.Buffer
	code := execute(context.Background(), []string{"diff", "--base", "/tmp/base.json", "--candidate", "/tmp/candidate.json"}, &stdout, &stderr, app)
	if code != 0 {
		t.Fatalf("code=%d stderr=%s", code, stderr.String())
	}
	if app.basePath != "/tmp/base.json" || app.candidatePath != "/tmp/candidate.json" {
		t.Fatalf("base=%q candidate=%q", app.basePath, app.candidatePath)
	}
	var diff terminology.ReleaseDiff
	if err := json.Unmarshal(stdout.Bytes(), &diff); err != nil {
		t.Fatalf("decode output: %v\n%s", err, stdout.String())
	}
	if diff.BaseRelease != "v1" || len(diff.AddedEntries) != 1 || len(diff.PolicyChanges) != 1 {
		t.Fatalf("diff=%+v", diff)
	}
}

func TestExecuteDiffWritesSummary(t *testing.T) {
	app := &fakeApp{diffResult: terminology.ReleaseDiff{
		Source: "iec-seed", BaseRelease: "v1", CandidateRelease: "v2",
		AddedEntries:  []terminology.DiffItem{{Key: "845-22-060"}},
		RetiredLabels: []terminology.DiffItem{{Key: "x"}},
	}}
	var stdout, stderr bytes.Buffer
	code := execute(context.Background(), []string{"diff", "--base", "/tmp/base.json", "--candidate", "/tmp/candidate.json", "--format", "summary"}, &stdout, &stderr, app)
	if code != 0 {
		t.Fatalf("code=%d stderr=%s", code, stderr.String())
	}
	want := "release diff: iec-seed v1 -> v2\nentries: +1 -0 ~0\nlabels: +0 -1 ~0\nrelations: +0 -0 ~0\nnegative decisions: +0 -0 ~0\nucum codes: +0 -0 ~0\nartifacts: +0 -0 ~0\npolicy changes: 0\n"
	if stdout.String() != want {
		t.Fatalf("summary:\n%s\nwant:\n%s", stdout.String(), want)
	}
}

func TestExecuteDiffRequiresBothManifests(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := execute(context.Background(), []string{"diff", "--base", "/tmp/base.json"}, &stdout, &stderr, &fakeApp{})
	if code != 2 || !strings.Contains(stderr.String(), "--base and --candidate are required") {
		t.Fatalf("code=%d stderr=%q", code, stderr.String())
	}
}

func TestExecuteActivateWritesJSON(t *testing.T) {
	app := &fakeApp{}
	var stdout, stderr bytes.Buffer
	code := execute(context.Background(), []string{"activate", "--deployment-key", "tier6-primary", "--source", "iec-seed", "--release", "v1", "--changed-by", "operator@example.test"}, &stdout, &stderr, app)
	if code != 0 {
		t.Fatalf("code=%d stderr=%s", code, stderr.String())
	}
	if app.deploymentKey != "tier6-primary" || app.changedBy != "operator@example.test" {
		t.Fatalf("deploymentKey=%q changedBy=%q", app.deploymentKey, app.changedBy)
	}
	var out map[string]string
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("decode output: %v\n%s", err, stdout.String())
	}
	if out["action"] != "activate" || out["source"] != "iec-seed" || out["release"] != "v1" {
		t.Fatalf("out=%+v", out)
	}
}

func TestExecuteActivateRequiresFlags(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := execute(context.Background(), []string{"activate", "--deployment-key", "tier6-primary"}, &stdout, &stderr, &fakeApp{})
	if code != 2 || !strings.Contains(stderr.String(), "--deployment-key, --source, --release, and --changed-by are required") {
		t.Fatalf("code=%d stderr=%q", code, stderr.String())
	}
}

func TestExecuteActivatePropagatesFailure(t *testing.T) {
	app := &fakeApp{activateErr: errors.New("deployment release is not approved: iec-seed/v1")}
	var stdout, stderr bytes.Buffer
	code := execute(context.Background(), []string{"activate", "--deployment-key", "tier6-primary", "--source", "iec-seed", "--release", "v1", "--changed-by", "operator@example.test"}, &stdout, &stderr, app)
	if code != 3 || !strings.Contains(stderr.String(), "not approved") {
		t.Fatalf("code=%d stderr=%q", code, stderr.String())
	}
}

func TestExecuteRollbackWritesJSON(t *testing.T) {
	app := &fakeApp{}
	var stdout, stderr bytes.Buffer
	code := execute(context.Background(), []string{"rollback", "--deployment-key", "tier6-primary", "--changed-by", "operator@example.test"}, &stdout, &stderr, app)
	if code != 0 {
		t.Fatalf("code=%d stderr=%s", code, stderr.String())
	}
	var out map[string]string
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("decode output: %v\n%s", err, stdout.String())
	}
	if out["action"] != "rollback" || out["deployment_key"] != "tier6-primary" {
		t.Fatalf("out=%+v", out)
	}
}

func TestExecuteRollbackRequiresDeploymentKey(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := execute(context.Background(), []string{"rollback", "--changed-by", "operator@example.test"}, &stdout, &stderr, &fakeApp{})
	if code != 2 || !strings.Contains(stderr.String(), "--deployment-key and --changed-by are required") {
		t.Fatalf("code=%d stderr=%q", code, stderr.String())
	}
}

func TestExecuteRejectsUnknownSubcommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := execute(context.Background(), []string{"publish"}, &stdout, &stderr, &fakeApp{})
	if code != 2 || !strings.Contains(stderr.String(), `unknown subcommand "publish"`) {
		t.Fatalf("code=%d stderr=%q", code, stderr.String())
	}
}
