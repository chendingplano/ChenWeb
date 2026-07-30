package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestValidateEmitsMachineReadableSnapshot(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", "..", ".."))
	var stdout, stderr bytes.Buffer
	code := execute(context.Background(), []string{"validate", "--experiment", filepath.Join(root, "benchmark/doc-processors/experiments/example.toml"), "--datasets-root", filepath.Join(root, "benchmark/doc-processors/datasets")}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("code=%d stderr=%s", code, stderr.String())
	}
	var got map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil || got["dataset_hash"] == "" || got["request_hash"] == "" {
		t.Fatalf("output=%s err=%v", stdout.String(), err)
	}
}

// TestRunArtifactWebRootFlagSetsEnv confirms --artifact-web-root propagates
// to ARTIFACT_WEB_DIR the same way --artifact-root propagates to
// ARTIFACT_DIR (main.go), independent of whether the run itself succeeds --
// this environment has no live DB, so the run is expected to fail later; the
// behavior under test is the early flag-to-env propagation, not a full run.
func TestRunArtifactWebRootFlagSetsEnv(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", "..", ".."))
	artifactRoot := t.TempDir()
	webRoot := filepath.Join(t.TempDir(), "web") // deliberately not pre-created
	t.Setenv("ARTIFACT_WEB_DIR", "")

	var stdout, stderr bytes.Buffer
	_ = execute(context.Background(), []string{
		"run",
		"--experiment", filepath.Join(root, "benchmark/doc-processors/experiments/example.toml"),
		"--artifact-root", artifactRoot,
		"--artifact-web-root", webRoot,
		"--allow-dirty",
	}, &stdout, &stderr)

	if got, want := os.Getenv("ARTIFACT_WEB_DIR"), filepath.Clean(webRoot); got != want {
		t.Fatalf("ARTIFACT_WEB_DIR = %q, want %q", got, want)
	}
}

func TestCommandValidationUsesStableJSONErrors(t *testing.T) {
	tests := [][]string{
		{"run"},
		{"compare", "--experiment-id", "x"},
		{"clean", "--discard-unverified", "--experiment-id", "x"},
		{"unknown"},
	}
	for _, args := range tests {
		var stdout, stderr bytes.Buffer
		if code := execute(context.Background(), args, &stdout, &stderr); code != 2 {
			t.Fatalf("args=%v code=%d stderr=%s", args, code, stderr.String())
		}
		var envelope errorEnvelope
		if err := json.Unmarshal(stderr.Bytes(), &envelope); err != nil || envelope.Error.Code == "" || envelope.Error.Message == "" {
			t.Fatalf("args=%v stderr=%q err=%v", args, stderr.String(), err)
		}
	}
}
