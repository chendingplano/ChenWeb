package main

import (
	"bytes"
	"context"
	"encoding/json"
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
