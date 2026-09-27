package calendarhandler

import "testing"

func TestReorderDisplaySeqnosMovesHolidayEarlier(t *testing.T) {
	rows := []displaySeqRow{
		{ID: 1, Country: "CN", Seqno: 1},
		{ID: 2, Country: "CN", Seqno: 2},
		{ID: 3, Country: "CN", Seqno: 3},
	}

	updated, err := reorderDisplaySeqnos(rows, 3, "CN", 1)
	if err != nil {
		t.Fatal(err)
	}

	assertSeqnos(t, updated, map[int64]int{1: 2, 2: 3, 3: 1})
}

func TestReorderDisplaySeqnosMovesHolidayToAnotherCountry(t *testing.T) {
	rows := []displaySeqRow{
		{ID: 1, Country: "CN", Seqno: 1},
		{ID: 2, Country: "CN", Seqno: 2},
		{ID: 3, Country: "US", Seqno: 1},
	}

	updated, err := reorderDisplaySeqnos(rows, 2, "US", 1)
	if err != nil {
		t.Fatal(err)
	}

	assertSeqnos(t, updated, map[int64]int{1: 1, 2: 1, 3: 2})
}

func assertSeqnos(t *testing.T, got map[int64]int, want map[int64]int) {
	t.Helper()
	for id, wantSeqno := range want {
		if got[id] != wantSeqno {
			t.Errorf("id %d: got sequence %d, want %d", id, got[id], wantSeqno)
		}
	}
}

func TestOverlappingDate(t *testing.T) {
	if got := overlappingDate([]string{"2026-01-01", "2026-01-02"}, []string{"2026-01-04"}); got != "" {
		t.Errorf("disjoint lists: got %q, want \"\"", got)
	}
	if got := overlappingDate([]string{"2026-01-01", "2026-01-04"}, []string{"2026-01-04"}); got != "2026-01-04" {
		t.Errorf("overlap: got %q, want 2026-01-04", got)
	}
	if got := overlappingDate(nil, []string{"2026-01-04"}); got != "" {
		t.Errorf("adjusted only: got %q, want \"\"", got)
	}
}
