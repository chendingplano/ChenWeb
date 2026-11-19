package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestUsageErrors(t *testing.T) {
	for _, args := range [][]string{
		nil,
		{"unknown"},
		{"reserve"},
		{"commit"},
		{"commit", "--version", "1.2.3"},
		{"notes", "--version", "01.02.06"},
		{"add-item", "--type", "bug fix"},
		{"import"},
	} {
		var out, errOut bytes.Buffer
		if code := run(args, &out, &errOut); code != 2 {
			t.Errorf("%v: exit %d, want 2 (stderr %q)", args, code, errOut.String())
		}
	}
}

func TestReorderPutsFlagsFirst(t *testing.T) {
	got := strings.Join(reorder([]string{"Fix the table", "--type", "bug fix", "--set-current", "--issue=12"}), "|")
	want := "--type|bug fix|--set-current|--issue=12|Fix the table"
	if got != want {
		t.Fatalf("reorder = %q, want %q", got, want)
	}
}
