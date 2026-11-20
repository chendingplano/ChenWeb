package docsync

import (
	"context"
	"crypto/md5"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/chendingplano/deepdoc/server/api/docsync/transfer"
	"github.com/chendingplano/shared/go/api/loggerutil"
	_ "github.com/lib/pq"
)

func TestWindowKeepsWallTimeAcrossDSTAndMidnight(t *testing.T) {
	location, err := time.LoadLocation("America/Chicago")
	if err != nil {
		t.Fatal(err)
	}
	start, err := ParseWallTime("1:30am")
	if err != nil {
		t.Fatal(err)
	}
	end, err := ParseWallTime("01:00")
	if err != nil {
		t.Fatal(err)
	}
	window := Window{Start: start, End: end}
	before := time.Date(2026, 3, 8, 0, 45, 0, 0, location)
	open, close := window.bounds(before)
	if open.Day() != 7 || close.Day() != 8 || close.Hour() != 1 {
		t.Fatalf("open %v close %v", open, close)
	}
	next := window.Next(time.Date(2026, 3, 8, 3, 0, 0, 0, location))
	if next.Day() != 9 || next.Hour() != 1 || next.Minute() != 30 {
		t.Fatalf("next sync drifted: %v", next)
	}
	nonexistent, err := ParseWallTime("2:30am")
	if err != nil {
		t.Fatal(err)
	}
	if got := nonexistent.on(before); got.Day() != before.Day() || got.Hour() != 3 || got.Minute() != 0 {
		t.Fatalf("nonexistent local time maps to %v", got)
	}
}

func TestUnpackRejectsUnknownVersion(t *testing.T) {
	file := filepath.Join(t.TempDir(), "sample.tar.gz")
	f, err := os.Create(file)
	if err != nil {
		t.Fatal(err)
	}
	if err := writeArchive(f, manifest{Version: 99, RunID: 1, TarNumber: 1}, strings.NewReader("")); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	_, err = unpackArchive(file, t.TempDir(), 1, 1, nil)
	if err == nil || !strings.Contains(err.Error(), "unsupported document sync format version 99") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestFileJournalRollsBackArtifacts(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	src := filepath.Join(dir, "source")
	if err := os.WriteFile(target, []byte("previous"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(src, []byte("new"), 0o600); err != nil {
		t.Fatal(err)
	}
	journal := fileJournal{Changes: []fileChange{{Target: target, Source: src}}}
	if err := journal.prepare(dir); err != nil {
		t.Fatal(err)
	}
	if err := journal.place(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := journal.finish(false); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(target)
	if err != nil || string(got) != "previous" {
		t.Fatalf("rollback file = %q, %v", got, err)
	}
}

func TestDocumentBundleRoundTrip(t *testing.T) {
	dbname := os.Getenv("DOCSYNC_TEST_DB")
	if !strings.HasPrefix(dbname, "docsync_test_") {
		t.Skip("set DOCSYNC_TEST_DB to a disposable migrated PostgreSQL database")
	}
	db, err := sql.Open("postgres", "dbname="+dbname+" sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	var store int64
	if err := db.QueryRowContext(ctx, `INSERT INTO kb.knowledge_store(ks_name) VALUES ('Sync test') RETURNING id`).Scan(&store); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	t.Setenv("DATA_HOME_DIR", root)
	artifactRoot := filepath.Join(root, "Artifacts")
	inbox := filepath.Join(root, "inbox")
	if err := os.Mkdir(inbox, 0o700); err != nil {
		t.Fatal(err)
	}
	owner := "6dcaff24-28da-4e6a-b5d2-e2055b371974"
	logger := loggerutil.CreateDefaultLogger("20261120-326")
	defer logger.Close()
	runner := Runner{DB: db, Logger: logger, Site: "integration", StoreID: store, ArtifactRoot: artifactRoot, Inbox: inbox}
	for _, converting := range []bool{false, true} {
		t.Run(fmt.Sprint(converting), func(t *testing.T) {
			content := fmt.Sprintf("Synthetic fixture %s for conversion=%v", root, converting)
			hash := fmt.Sprintf("%x", md5.Sum([]byte(content)))
			original := filepath.Join(root, "original")
			lines := filepath.Join(root, "lines.txt")
			if err := os.WriteFile(original, []byte(content), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(lines, []byte("line 1"), 0o600); err != nil {
				t.Fatal(err)
			}
			var id int64
			status := `[{"operation":"parsed","proc_status":"success","start_time":"1"},
				{"operation":"chunked","proc_status":"success","start_time":"2"},
				{"operation":"extract_metrics","proc_status":"failed","start_time":"3"},
				{"operation":"doc_processing","proc_status":"failed","start_time":"4"}]`
			if err := db.QueryRowContext(ctx, `INSERT INTO kb.inputs(type,user_id,title,md5,staging_filename,parser_name,file_name,result_filename,status)
				VALUES ('pdf',$1,'Source title',$2,'source.pdf','mineru',$3,$4,$5) RETURNING id`,
				owner, strings.ToUpper(hash), original, lines, status).Scan(&id); err != nil {
				t.Fatal(err)
			}
			if _, err := db.ExecContext(ctx, `INSERT INTO kb.chunks(source_record_id,chunking_method,chunking_size,overlap_percent,normal_lines,overlap_lines)
				VALUES ($1,'fixed',1,0,'[1]'::jsonb,'[]'::jsonb)`, id); err != nil {
				t.Fatal(err)
			}
			source := Source{DB: db, ArtifactRoot: artifactRoot}
			upper := time.Now().Add(time.Minute)
			line, err := source.listInput(ctx, cursor{ID: id, Time: upper}, upper)
			if err != nil || line.Issue != "" {
				t.Fatalf("list source: %#v %v", line, err)
			}
			if len(line.Items) != 3 {
				t.Fatalf("planned items: %+v", line.Items)
			}
			for _, work := range line.Items {
				if work.Issue != "" {
					t.Fatalf("unexpected work issue: %v", work)
				}
			}
			work, err := json.Marshal(line.Items)
			if err != nil {
				t.Fatal(err)
			}
			file := filepath.Join(t.TempDir(), "sample.tar.gz")
			req := transfer.PackRequest{RunID: id, TarNumber: 1, Work: work, MaxBytes: 1 << 20}
			index, err := source.pack(ctx, req, file)
			if err != nil {
				t.Fatal(err)
			}
			included, deferred, err := matchingWork(transfer.Index{PackResult: index}, req)
			if err != nil || len(included) != 3 || len(deferred) != 0 {
				t.Fatalf("pack: %v, %s", err, index.Deferred)
			}
			staging := t.TempDir()
			m, err := unpackArchive(file, staging, id, 1, included)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := db.ExecContext(ctx, `DELETE FROM kb.chunks WHERE source_record_id=$1`, id); err != nil {
				t.Fatal(err)
			}
			// This fixture uses a single temporary DB. After packing, remove the source
			// hash from the development side or reuse its row as a matching local upload.
			if !converting {
				if _, err := db.ExecContext(ctx, `DELETE FROM kb.inputs WHERE id=$1`, id); err != nil {
					t.Fatal(err)
				}
			} else if _, err := db.ExecContext(ctx, `UPDATE kb.inputs SET staging_filename='local.pdf', title='Earlier local title',status='[]' WHERE id=$1`, id); err != nil {
				t.Fatal(err)
			}
			for _, item := range m.Items {
				got, err := runner.apply(ctx, owner, staging, item)
				if err != nil {
					t.Fatalf("apply %s: %v", item.Work.Kind, err)
				}
				if item.Work.Kind == documentItem && got.Converted != converting {
					t.Fatalf("conversion=%v: %+v", converting, got)
				}
				replayed, err := runner.apply(ctx, owner, staging, item)
				if err != nil || !replayed.Skipped {
					t.Fatalf("replay %s: %+v %v", item.Work.Kind, replayed, err)
				}
			}
			var local, localStore, chunks int64
			var origin, title, linePath, pipeline, statuses string
			if err := db.QueryRowContext(ctx, `SELECT id,doc_origin,title,result_filename,ks_store_id,pipeline_state,status::text
				FROM kb.inputs WHERE lower(md5)=$1`, hash).Scan(&local, &origin, &title, &linePath, &localStore, &pipeline, &statuses); err != nil {
				t.Fatal(err)
			}
			if (local == id) != converting || origin != "remote" || title != "Source title" || localStore != store ||
				pipeline != "failed" || strings.Contains(statuses, "extract_metrics") {
				t.Fatalf("unexpected record state: %d %s %s %d %s %s", local, origin, title, localStore, pipeline, statuses)
			}
			if data, err := os.ReadFile(filepath.Join(root, linePath)); err != nil || string(data) != "line 1" {
				t.Fatalf("imported line file: %q %v", data, err)
			}
			if err := db.QueryRowContext(ctx, `SELECT count(*) FROM kb.chunks WHERE source_record_id=$1`, local).Scan(&chunks); err != nil || chunks != 1 {
				t.Fatalf("expected one chunk, got %d: %v", chunks, err)
			}
		})
	}
}
