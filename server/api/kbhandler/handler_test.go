package kbhandler

import (
	"strings"
	"testing"
)

func TestBuildWhereClauseDocTypeAndFileName(t *testing.T) {
	whereSQL, args, err := buildWhereClause("pdf", "all", "report", nil, nil)
	if err != nil {
		t.Fatalf("buildWhereClause returned error: %v", err)
	}

	if !strings.Contains(whereSQL, "LOWER(i.type) = LOWER($1)") {
		t.Fatalf("expected doc type filter in whereSQL, got: %s", whereSQL)
	}
	if !strings.Contains(whereSQL, "COALESCE(i.file_name, '') ILIKE $2") {
		t.Fatalf("expected file_name filter in whereSQL, got: %s", whereSQL)
	}
	if len(args) != 2 {
		t.Fatalf("expected 2 args, got %d", len(args))
	}
}

func TestBuildWhereClauseParseStates(t *testing.T) {
	tests := []struct {
		name       string
		parseState string
		wantPart   string
	}{
		{
			name:       "pending",
			parseState: "pending",
			wantPart:   "NOT EXISTS",
		},
		{
			name:       "parsed_success",
			parseState: "parsed_success",
			wantPart:   "LOWER(COALESCE(st->>'status', '')) = 'success'",
		},
		{
			name:       "parsed_failed",
			parseState: "parsed_failed",
			wantPart:   "LOWER(COALESCE(st->>'status', '')) <> 'success'",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			whereSQL, _, err := buildWhereClause("all", tc.parseState, "", nil, nil)
			if err != nil {
				t.Fatalf("buildWhereClause returned error: %v", err)
			}
			if !strings.Contains(whereSQL, tc.wantPart) {
				t.Fatalf("expected whereSQL to contain %q, got: %s", tc.wantPart, whereSQL)
			}
		})
	}
}

func TestBuildWhereClauseRejectsInvalidParseState(t *testing.T) {
	_, _, err := buildWhereClause("all", "bad-state", "", nil, nil)
	if err == nil {
		t.Fatal("expected error for invalid parse_state")
	}
}

func TestParsePositiveInt(t *testing.T) {
	if got := parsePositiveInt("3", 1); got != 3 {
		t.Fatalf("expected 3, got %d", got)
	}
	if got := parsePositiveInt("-7", 1); got != 1 {
		t.Fatalf("expected default 1, got %d", got)
	}
	if got := parsePositiveInt("abc", 9); got != 9 {
		t.Fatalf("expected default 9, got %d", got)
	}
}
