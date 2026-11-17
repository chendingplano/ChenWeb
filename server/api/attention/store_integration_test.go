package attention

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	_ "github.com/lib/pq"
)

// testStore returns a SQLStore on TEST_DATABASE_URL (chenweb_test, never miner)
// with the kb.attention_items migration applied. Rows use dedupe keys that
// start with "itest-" and are removed after the test.
func testStore(t *testing.T) SQLStore {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	if strings.Contains(dsn, "/miner") {
		t.Fatal("refusing to run against miner")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Ping(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	mig, err := os.ReadFile("../../../project_migrations/20261117000001_create_kb_attention_items.sql")
	if err != nil {
		t.Fatal(err)
	}
	up := strings.SplitN(strings.SplitN(string(mig), "-- +goose Down", 2)[0], "-- +goose Up", 2)[1]
	if _, err := db.Exec(`CREATE SCHEMA IF NOT EXISTS kb`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(up); err != nil {
		t.Fatal(err)
	}
	cleanup := func() { _, _ = db.Exec(`DELETE FROM kb.attention_items WHERE dedupe_key LIKE 'itest-%'`) }
	cleanup()
	t.Cleanup(cleanup)
	return SQLStore{DB: db}
}

func testKey(t *testing.T) string {
	return fmt.Sprintf("itest-%s-%d", t.Name(), time.Now().UnixNano())
}

func report(key string, record int64, artifactID string) Report {
	return Report{
		ItemType:      TypeAmbiguousObjectName,
		DedupeKey:     key,
		Title:         "管路: 2 tied object nodes",
		InputRecordID: record,
		Details:       map[string]any{"object_name": "管路", "picked_object_id": "obj_a"},
		Occurrence:    map[string]any{"input_record_id": record, "artifact_type": "metric", "artifact_id": artifactID, "object_name": "管路"},
	}
}

func details(t *testing.T, it Item) map[string]any {
	t.Helper()
	var d map[string]any
	if err := json.Unmarshal(it.Details, &d); err != nil {
		t.Fatal(err)
	}
	return d
}

func TestRecordInsertsThenUpdatesOpenItem(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	key := testKey(t)

	first, err := s.Record(ctx, report(key, 279, "m1"))
	if err != nil || !first.Created {
		t.Fatalf("first record: %+v %v", first, err)
	}
	second, err := s.Record(ctx, report(key, 383, "m2"))
	if err != nil || second.Created || second.Reopened || second.ID != first.ID {
		t.Fatalf("second record: %+v %v", second, err)
	}
	// Same occurrence again: counted, but not listed twice.
	if _, err := s.Record(ctx, report(key, 383, "m2")); err != nil {
		t.Fatal(err)
	}
	it, err := s.Get(ctx, first.ID)
	if err != nil {
		t.Fatal(err)
	}
	if it.Status != StatusOpen || it.OccurrenceCount != 3 || it.ReopenCount != 0 || it.InputRecordID != 383 {
		t.Fatalf("item = %+v", it)
	}
	occ := details(t, it)["occurrences"].([]any)
	if len(occ) != 2 || occ[0].(map[string]any)["artifact_id"] != "m2" || occ[1].(map[string]any)["artifact_id"] != "m1" {
		t.Fatalf("occurrences = %v", occ)
	}
}

func TestRecordReopensClosedItem(t *testing.T) {
	for _, status := range []string{StatusResolved, StatusDismissed} {
		t.Run(status, func(t *testing.T) {
			s := testStore(t)
			ctx := context.Background()
			key := testKey(t)
			res, err := s.Record(ctx, report(key, 279, "m1"))
			if err != nil {
				t.Fatal(err)
			}
			if _, err := s.Close(ctx, res.ID, status, "alice", "merged the nodes"); err != nil {
				t.Fatal(err)
			}
			again, err := s.Record(ctx, report(key, 400, "m9"))
			if err != nil || !again.Reopened || again.ID != res.ID {
				t.Fatalf("reopen: %+v %v", again, err)
			}
			it, err := s.Get(ctx, res.ID)
			if err != nil {
				t.Fatal(err)
			}
			if it.Status != StatusOpen || it.ReopenCount != 1 || it.OccurrenceCount != 2 ||
				it.ResolvedBy != "" || it.ResolvedAt != nil || it.Resolution != "" {
				t.Fatalf("item = %+v", it)
			}
			d := details(t, it)
			hist := d["resolution_history"].([]any)
			h := hist[0].(map[string]any)
			if len(hist) != 1 || h["status"] != status || h["resolved_by"] != "alice" || h["resolution"] != "merged the nodes" || h["resolved_at"] == nil {
				t.Fatalf("history = %v", hist)
			}
			if occ := d["occurrences"].([]any); occ[0].(map[string]any)["artifact_id"] != "m9" {
				t.Fatalf("new evidence missing: %v", occ)
			}
		})
	}
}

func TestRecordConcurrent(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	key := testKey(t)
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := s.Record(ctx, report(key, int64(500+i), fmt.Sprintf("c%d", i)))
			errs <- err
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	items, total, err := s.List(ctx, ListFilter{ItemType: TypeAmbiguousObjectName, Status: StatusOpen, Limit: 500})
	if err != nil {
		t.Fatal(err)
	}
	var found []Item
	for _, it := range items {
		if it.DedupeKey == key {
			found = append(found, it)
		}
	}
	if total < 1 || len(found) != 1 || found[0].OccurrenceCount != 2 {
		t.Fatalf("found = %+v", found)
	}
}

func TestRecordUnknownType(t *testing.T) {
	s := testStore(t)
	key := testKey(t)
	r := report(key, 1, "x")
	r.ItemType = "no-such-type"
	if _, err := s.Record(context.Background(), r); !errors.Is(err, ErrUnknownType) {
		t.Fatalf("err = %v", err)
	}
	var n int
	if err := s.DB.QueryRow(`SELECT count(*) FROM kb.attention_items WHERE dedupe_key = $1`, key).Scan(&n); err != nil || n != 0 {
		t.Fatalf("rows = %d, %v", n, err)
	}
}

func TestCloseOnlyFromOpen(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	res, err := s.Record(ctx, report(testKey(t), 1, "x"))
	if err != nil {
		t.Fatal(err)
	}
	it, err := s.Close(ctx, res.ID, StatusResolved, "bob", "  fixed  ")
	if err != nil || it.Status != StatusResolved || it.ResolvedBy != "bob" || it.Resolution != "fixed" || it.ResolvedAt == nil {
		t.Fatalf("close: %+v %v", it, err)
	}
	if _, err := s.Close(ctx, res.ID, StatusDismissed, "bob", ""); !errors.Is(err, ErrNotOpen) {
		t.Fatalf("second close err = %v", err)
	}
	if _, err := s.Close(ctx, -1, StatusResolved, "bob", ""); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown id err = %v", err)
	}
}

func TestListFiltersAndOrder(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	base := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
	var ids []int64
	for i := 0; i < 3; i++ {
		at := base.Add(time.Duration(i) * time.Minute)
		s.Now = func() time.Time { return at }
		res, err := s.Record(ctx, report(testKey(t), int64(i+1), "x"))
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, res.ID)
	}
	s.Now = nil
	if _, err := s.Close(ctx, ids[1], StatusDismissed, "bob", ""); err != nil {
		t.Fatal(err)
	}
	items, _, err := s.List(ctx, ListFilter{ItemType: TypeAmbiguousObjectName, Status: StatusOpen, Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	// The 2030 rows are the newest, so they lead the list.
	if len(items) != 2 || items[0].ID != ids[2] || items[1].ID != ids[0] {
		t.Fatalf("items = %v, want ids %d then %d", itemIDs(items), ids[2], ids[0])
	}
}

func itemIDs(items []Item) []int64 {
	out := make([]int64, len(items))
	for i, it := range items {
		out[i] = it.ID
	}
	return out
}

func TestMergeDetailsCapsOccurrences(t *testing.T) {
	existing := map[string]any{}
	now := time.Now()
	for i := 0; i < MaxOccurrences+5; i++ {
		r := report("k", int64(i), fmt.Sprintf("a%d", i))
		raw, _ := json.Marshal(mergeDetails(existing, r, now, nil))
		existing = map[string]any{}
		_ = json.Unmarshal(raw, &existing)
	}
	occ := existing["occurrences"].([]any)
	if len(occ) != MaxOccurrences || occ[0].(map[string]any)["artifact_id"] != fmt.Sprintf("a%d", MaxOccurrences+4) {
		t.Fatalf("len = %d first = %v", len(occ), occ[0])
	}
}
