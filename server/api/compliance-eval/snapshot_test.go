package complianceeval

import "testing"

func TestDiffReportsEveryChangedMember(t *testing.T) {
	manifest := []Member{
		{RecordID: 5, MD5: "a", Generation: 1, ChunkCount: 10, ProvisionCount: 2},
		{RecordID: 3, MD5: "b", Generation: 1, ChunkCount: 4},
		{RecordID: 9, MD5: "c", Generation: 2, ChunkCount: 7},
		{RecordID: 8, MD5: "d", Generation: 1, ChunkCount: 1},
	}
	cur := map[int64]Member{
		5: {RecordID: 5, MD5: "a", Generation: 1, ChunkCount: 10, ProvisionCount: 2}, // unchanged
		3: {RecordID: 3, MD5: "b", Generation: 2, ChunkCount: 4},                     // re-indexed
		9: {RecordID: 9, MD5: "x", Generation: 2, ChunkCount: 7},                     // file replaced
		// 8 deleted
	}
	got := Diff(manifest, cur)
	if len(got) != 3 {
		t.Fatalf("differences = %v", got)
	}
	want := []struct {
		id  int64
		sub string
	}{{3, "index generation 1 -> 2"}, {8, "no longer present"}, {9, `md5 "c" -> "x"`}}
	for i, w := range want {
		if got[i].RecordID != w.id || !contains(got[i].Reason, w.sub) {
			t.Errorf("difference %d = %v, want record %d containing %q", i, got[i], w.id, w.sub)
		}
	}
}

func TestDiffEmptyWhenNothingChanged(t *testing.T) {
	m := Member{RecordID: 1, MD5: "a", Generation: 3, ChunkCount: 2, ProvisionCount: 1}
	if d := Diff([]Member{m}, map[int64]Member{1: m}); len(d) != 0 {
		t.Fatalf("differences = %v", d)
	}
}

func TestDiffNamesIndexCountChange(t *testing.T) {
	a := Member{RecordID: 1, MD5: "a", ChunkCount: 2, ProvisionCount: 1}
	b := a
	b.ChunkCount, b.ProvisionCount = 3, 0
	d := Diff([]Member{a}, map[int64]Member{1: b})
	if len(d) != 1 || !contains(d[0].Reason, "chunk artifacts 2 -> 3") || !contains(d[0].Reason, "provision artifacts 1 -> 0") {
		t.Fatalf("differences = %v", d)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func TestDocsOutsideSnapshot(t *testing.T) {
	got := DocsOutsideSnapshot([]GoldDoc{{RecordID: 1}, {RecordID: 2}, {RecordID: 3}}, []Member{{RecordID: 1}, {RecordID: 3}})
	if len(got) != 1 || got[0] != 2 {
		t.Fatalf("outside = %v", got)
	}
}
