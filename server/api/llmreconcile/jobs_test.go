package llmreconcile

import (
	"testing"
	"time"
)

func TestNextReconciliationRunAtUsesWorkspaceTimezone(t *testing.T) {
	loc := time.FixedZone("America/Chicago", -5*60*60)
	now := time.Date(2026, 6, 20, 1, 15, 0, 0, time.UTC)

	got := nextReconciliationRunAt(now, loc, 2)

	want := time.Date(2026, 6, 19, 2, 0, 0, 0, loc).Add(24 * time.Hour)
	if !got.Equal(want) {
		t.Fatalf("nextReconciliationRunAt() = %s, want %s", got, want)
	}
}

func TestNextReconciliationRunAtMovesToNextDayAfterRunHour(t *testing.T) {
	loc := time.FixedZone("America/Chicago", -5*60*60)
	now := time.Date(2026, 6, 20, 8, 30, 0, 0, loc)

	got := nextReconciliationRunAt(now, loc, 2)

	want := time.Date(2026, 6, 21, 2, 0, 0, 0, loc)
	if !got.Equal(want) {
		t.Fatalf("nextReconciliationRunAt() = %s, want %s", got, want)
	}
}
