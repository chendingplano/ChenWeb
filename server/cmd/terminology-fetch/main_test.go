package main

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/chendingplano/deepdoc/server/api/ontology/terminology"
)

type permissionError struct{}

func (permissionError) Error() string { return "requires permission" }

type stubApp struct {
	fetchErr error
}

func (stubApp) resources() []terminology.Resource { return terminology.Resources() }
func (stubApp) status(dir string, id terminology.ResourceID) (terminology.FetchStatus, error) {
	return terminology.FetchStatus{Source: string(id)}, nil
}
func (s stubApp) fetch(_ context.Context, dir string, id terminology.ResourceID, _ ...terminology.FetchOption) (terminology.FetchStatus, error) {
	if s.fetchErr != nil {
		return terminology.FetchStatus{Source: string(id), Error: s.fetchErr.Error()}, s.fetchErr
	}
	return terminology.FetchStatus{
		Source: string(id), Release: "3.5.0", Downloaded: true,
		DownloadedAt: time.Date(2026, 8, 7, 12, 0, 0, 0, time.UTC),
		SHA256:       "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		SizeBytes:    42, Artifact: "qudt-all.ttl", ManifestDraft: "manifest.draft.json",
	}, nil
}

func TestExecuteListPrintsCatalog(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := execute(context.Background(), []string{"list"}, &stdout, &stderr, stubApp{}); code != 0 {
		t.Fatalf("code=%d stderr=%s", code, stderr.String())
	}
	var got []terminology.Resource
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v\n%s", err, stdout.String())
	}
	if len(got) != len(terminology.Resources()) {
		t.Fatalf("got %d resources", len(got))
	}
}

func TestExecuteStatusAll(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := execute(context.Background(), []string{"status", "--dir", t.TempDir()}, &stdout, &stderr, stubApp{}); code != 0 {
		t.Fatalf("code=%d stderr=%s", code, stderr.String())
	}
	var got []terminology.FetchStatus
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != len(terminology.Resources()) {
		t.Fatalf("got %d statuses, want %d", len(got), len(terminology.Resources()))
	}
}

func TestExecuteFetch(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := execute(context.Background(),
		[]string{"fetch", "--source", "qudt", "--dir", t.TempDir()},
		&stdout, &stderr, stubApp{})
	if code != 0 {
		t.Fatalf("code=%d stderr=%s", code, stderr.String())
	}
	var st terminology.FetchStatus
	if err := json.Unmarshal(stdout.Bytes(), &st); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !st.Downloaded || st.Source != "qudt" || st.ManifestDraft != "manifest.draft.json" {
		t.Fatalf("status = %+v", st)
	}
}

func TestExecuteFetchFailureReturnsNonZero(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := execute(context.Background(),
		[]string{"fetch", "--source", "iec-60050-845", "--dir", t.TempDir()},
		&stdout, &stderr, stubApp{fetchErr: permissionError{}})
	if code == 0 {
		t.Fatal("expected non-zero exit")
	}
}

func TestExecuteUnknownSubcommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := execute(context.Background(), []string{"bogus"}, &stdout, &stderr, stubApp{}); code != 2 {
		t.Fatalf("code = %d, want 2", code)
	}
}
