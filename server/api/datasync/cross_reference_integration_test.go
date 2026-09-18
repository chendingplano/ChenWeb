package datasync

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// scratchImagesLikeTable creates a throwaway table shaped like kb.images'
// sync-relevant columns: a file column (stored_path) plus the stable,
// never-rewritten uid a referencing table (kb.videos) points at instead of
// the surrogate id (design.md Decision 8, configurable-data-sync-items).
func scratchImagesLikeTable(t *testing.T, db *sql.DB, name string) {
	t.Helper()
	_, err := db.Exec(fmt.Sprintf(`
		CREATE TABLE %s (
			id          BIGSERIAL PRIMARY KEY,
			stored_path TEXT NOT NULL,
			uid         UUID NOT NULL DEFAULT gen_random_uuid(),
			update_time TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			UNIQUE (uid)
		)`, name))
	if err != nil {
		t.Fatalf("create scratch images-like table %s: %v", name, err)
	}
	t.Cleanup(func() {
		if _, err := db.Exec("DROP TABLE IF EXISTS " + name); err != nil {
			t.Logf("cleanup: drop table %s: %v", name, err)
		}
	})
}

// scratchVideosLikeTable creates a throwaway table shaped like kb.videos'
// sync-relevant columns: its own file column (stored_path) plus image_uid, a
// plain data column carrying a copy of the referenced images-like row's uid
// -- never a FileColumn, never touched by materializeFiles.
func scratchVideosLikeTable(t *testing.T, db *sql.DB, name string) {
	t.Helper()
	_, err := db.Exec(fmt.Sprintf(`
		CREATE TABLE %s (
			id          BIGSERIAL PRIMARY KEY,
			stored_path TEXT NOT NULL,
			image_uid   UUID,
			update_time TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			UNIQUE (stored_path)
		)`, name))
	if err != nil {
		t.Fatalf("create scratch videos-like table %s: %v", name, err)
	}
	t.Cleanup(func() {
		if _, err := db.Exec("DROP TABLE IF EXISTS " + name); err != nil {
			t.Logf("cleanup: drop table %s: %v", name, err)
		}
	})
}

func imagesItemFor(table string) TableSyncItem {
	return TableSyncItem{
		ID:         "test_images_item",
		Table:      table,
		CursorCol:  "update_time",
		NaturalKey: []string{"uid"},
		Columns:    []string{"stored_path", "uid", "update_time"},
		Kind:       KindTableWithFiles,
		FileColumn: "stored_path",
	}
}

func videosItemFor(table string) TableSyncItem {
	return TableSyncItem{
		ID:         "test_videos_item",
		Table:      table,
		CursorCol:  "update_time",
		NaturalKey: []string{"stored_path"},
		Columns:    []string{"stored_path", "image_uid", "update_time"},
		Kind:       KindTableWithFiles,
		FileColumn: "stored_path",
	}
}

// newCrossItemFileServer stands in for a source instance's file-streaming
// endpoint for more than one item at once (HandleGetItemFile routes by
// itemId the same way), so one httptest.Server can serve both the
// images-like and videos-like items in these tests.
func newCrossItemFileServer(t *testing.T, db *sql.DB, items map[string]TableSyncItem, secret string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+secret {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		itemID := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/api/internal/data-sync/items/"), "/files")
		item, ok := items[itemID]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		var keyValues []string
		if err := json.Unmarshal([]byte(r.URL.Query().Get("key")), &keyValues); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		path, err := lookupItemFilePath(db, item, keyValues)
		if err != nil || path == "" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		f, err := os.Open(path)
		if err != nil {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		defer f.Close()
		_, _ = io.Copy(w, f)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// TestVideoImageUIDCrossReferenceSurvivesIndependentSync exercises the
// scenario behind design.md Decision 8: kb.videos and kb.images are synced
// by two independent table_with_files items. A video row's image_uid must
// resolve to the correct images row on the target *after* both items have
// been applied, even though source and target assign completely unrelated
// surrogate ids to both tables -- proving the natural-key (uid) reference
// survives independent syncing where the old surrogate image_id could not.
func TestVideoImageUIDCrossReferenceSurvivesIndependentSync(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	ts := time.Now().UnixNano()

	sourceImagesTable := fmt.Sprintf("public.datasync_it_xref_src_images_%d", ts)
	targetImagesTable := fmt.Sprintf("public.datasync_it_xref_tgt_images_%d", ts)
	sourceVideosTable := fmt.Sprintf("public.datasync_it_xref_src_videos_%d", ts)
	targetVideosTable := fmt.Sprintf("public.datasync_it_xref_tgt_videos_%d", ts)
	scratchImagesLikeTable(t, db, sourceImagesTable)
	scratchImagesLikeTable(t, db, targetImagesTable)
	scratchVideosLikeTable(t, db, sourceVideosTable)
	scratchVideosLikeTable(t, db, targetVideosTable)

	sourceDir := t.TempDir()
	imagePath := filepath.Join(sourceDir, "cover.jpg")
	videoPath := filepath.Join(sourceDir, "clip.mp4")
	const imageBody = "source image bytes"
	const videoBody = "source video bytes"
	if err := os.WriteFile(imagePath, []byte(imageBody), 0o644); err != nil {
		t.Fatalf("write source image: %v", err)
	}
	if err := os.WriteFile(videoPath, []byte(videoBody), 0o644); err != nil {
		t.Fatalf("write source video: %v", err)
	}

	var sourceImageUID string
	if err := db.QueryRow(
		fmt.Sprintf(`INSERT INTO %s (stored_path) VALUES ($1) RETURNING uid`, sourceImagesTable), imagePath,
	).Scan(&sourceImageUID); err != nil {
		t.Fatalf("seed source image row: %v", err)
	}
	if _, err := db.Exec(
		fmt.Sprintf(`INSERT INTO %s (stored_path, image_uid) VALUES ($1, $2)`, sourceVideosTable),
		videoPath, sourceImageUID,
	); err != nil {
		t.Fatalf("seed source video row: %v", err)
	}

	sourceImagesItem := imagesItemFor(sourceImagesTable)
	sourceVideosItem := videosItemFor(sourceVideosTable)
	const secret = "test-secret"
	fileSrv := newCrossItemFileServer(t, db, map[string]TableSyncItem{
		sourceImagesItem.ID: sourceImagesItem,
		sourceVideosItem.ID: sourceVideosItem,
	}, secret)

	targetImageDir := t.TempDir()
	targetVideoDir := t.TempDir()
	t.Setenv("TEST_XREF_IMAGE_DIR", targetImageDir)
	t.Setenv("TEST_XREF_VIDEO_DIR", targetVideoDir)
	targetImagesItem := imagesItemFor(targetImagesTable)
	targetImagesItem.FileDirEnv = "TEST_XREF_IMAGE_DIR"
	targetVideosItem := videosItemFor(targetVideosTable)
	targetVideosItem.FileDirEnv = "TEST_XREF_VIDEO_DIR"

	// Apply images first, then videos -- kb-images before video, the
	// order that leaves image_uid resolvable immediately.
	imgPage, err := fetchChangesPage(db, sourceImagesItem, "", 100)
	if err != nil {
		t.Fatalf("fetchChangesPage(images) error = %v", err)
	}
	if len(imgPage.Rows) != 1 {
		t.Fatalf("len(images Rows) = %d, want 1", len(imgPage.Rows))
	}
	if err := materializeFiles(ctx, targetImagesItem, fileSrv.URL, secret, imgPage.Rows); err != nil {
		t.Fatalf("materializeFiles(images) error = %v", err)
	}
	if err := upsertRows(ctx, db, targetImagesItem, imgPage.Rows); err != nil {
		t.Fatalf("upsertRows(images) error = %v", err)
	}

	vidPage, err := fetchChangesPage(db, sourceVideosItem, "", 100)
	if err != nil {
		t.Fatalf("fetchChangesPage(videos) error = %v", err)
	}
	if len(vidPage.Rows) != 1 {
		t.Fatalf("len(videos Rows) = %d, want 1", len(vidPage.Rows))
	}
	if err := materializeFiles(ctx, targetVideosItem, fileSrv.URL, secret, vidPage.Rows); err != nil {
		t.Fatalf("materializeFiles(videos) error = %v", err)
	}
	if err := upsertRows(ctx, db, targetVideosItem, vidPage.Rows); err != nil {
		t.Fatalf("upsertRows(videos) error = %v", err)
	}

	var targetImageUID, targetVideoImageUID string
	if err := db.QueryRow(fmt.Sprintf(`SELECT image_uid FROM %s`, targetVideosTable)).Scan(&targetVideoImageUID); err != nil {
		t.Fatalf("read target video row: %v", err)
	}
	if targetVideoImageUID != sourceImageUID {
		t.Fatalf("target video image_uid = %q, want it unchanged from the source's %q (it's plain data, not a FileColumn)", targetVideoImageUID, sourceImageUID)
	}

	var targetImageStoredPath string
	err = db.QueryRow(fmt.Sprintf(`SELECT uid, stored_path FROM %s WHERE uid = $1`, targetImagesTable), targetVideoImageUID).
		Scan(&targetImageUID, &targetImageStoredPath)
	if err == sql.ErrNoRows {
		t.Fatalf("no target images row has uid = %q; the video's cross-table reference did not resolve on the target", targetVideoImageUID)
	}
	if err != nil {
		t.Fatalf("look up target image by uid: %v", err)
	}
	if strings.HasPrefix(targetImageStoredPath, sourceDir) {
		t.Fatalf("target image stored_path = %q, want it rewritten under a target dir, not still the source path", targetImageStoredPath)
	}
	got, err := os.ReadFile(targetImageStoredPath)
	if err != nil {
		t.Fatalf("read materialized target image at %q: %v", targetImageStoredPath, err)
	}
	if string(got) != imageBody {
		t.Fatalf("materialized target image content = %q, want %q", got, imageBody)
	}
}

// TestVideoImageUIDDanglesWithoutErrorWhenAppliedOutOfOrder covers the
// accepted caveat in design.md Decision 8: if `video` is applied before
// `kb-images`, image_uid on the target names a uid that isn't in the
// target's images table yet. Since image_uid carries no REFERENCES
// constraint (a deliberate, faithful match of the old image_id column's own
// soft-reference style), this must apply cleanly with no error, and must
// self-heal (become resolvable) once the images item is applied afterward.
func TestVideoImageUIDDanglesWithoutErrorWhenAppliedOutOfOrder(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	ts := time.Now().UnixNano()

	sourceImagesTable := fmt.Sprintf("public.datasync_it_xref2_src_images_%d", ts)
	targetImagesTable := fmt.Sprintf("public.datasync_it_xref2_tgt_images_%d", ts)
	sourceVideosTable := fmt.Sprintf("public.datasync_it_xref2_src_videos_%d", ts)
	targetVideosTable := fmt.Sprintf("public.datasync_it_xref2_tgt_videos_%d", ts)
	scratchImagesLikeTable(t, db, sourceImagesTable)
	scratchImagesLikeTable(t, db, targetImagesTable)
	scratchVideosLikeTable(t, db, sourceVideosTable)
	scratchVideosLikeTable(t, db, targetVideosTable)

	sourceDir := t.TempDir()
	imagePath := filepath.Join(sourceDir, "cover.jpg")
	videoPath := filepath.Join(sourceDir, "clip.mp4")
	if err := os.WriteFile(imagePath, []byte("source image bytes"), 0o644); err != nil {
		t.Fatalf("write source image: %v", err)
	}
	if err := os.WriteFile(videoPath, []byte("source video bytes"), 0o644); err != nil {
		t.Fatalf("write source video: %v", err)
	}

	var sourceImageUID string
	if err := db.QueryRow(
		fmt.Sprintf(`INSERT INTO %s (stored_path) VALUES ($1) RETURNING uid`, sourceImagesTable), imagePath,
	).Scan(&sourceImageUID); err != nil {
		t.Fatalf("seed source image row: %v", err)
	}
	if _, err := db.Exec(
		fmt.Sprintf(`INSERT INTO %s (stored_path, image_uid) VALUES ($1, $2)`, sourceVideosTable),
		videoPath, sourceImageUID,
	); err != nil {
		t.Fatalf("seed source video row: %v", err)
	}

	sourceImagesItem := imagesItemFor(sourceImagesTable)
	sourceVideosItem := videosItemFor(sourceVideosTable)
	const secret = "test-secret"
	fileSrv := newCrossItemFileServer(t, db, map[string]TableSyncItem{
		sourceImagesItem.ID: sourceImagesItem,
		sourceVideosItem.ID: sourceVideosItem,
	}, secret)

	targetImageDir := t.TempDir()
	targetVideoDir := t.TempDir()
	t.Setenv("TEST_XREF2_IMAGE_DIR", targetImageDir)
	t.Setenv("TEST_XREF2_VIDEO_DIR", targetVideoDir)
	targetImagesItem := imagesItemFor(targetImagesTable)
	targetImagesItem.FileDirEnv = "TEST_XREF2_IMAGE_DIR"
	targetVideosItem := videosItemFor(targetVideosTable)
	targetVideosItem.FileDirEnv = "TEST_XREF2_VIDEO_DIR"

	// Apply videos BEFORE images -- the out-of-order case.
	vidPage, err := fetchChangesPage(db, sourceVideosItem, "", 100)
	if err != nil {
		t.Fatalf("fetchChangesPage(videos) error = %v", err)
	}
	if err := materializeFiles(ctx, targetVideosItem, fileSrv.URL, secret, vidPage.Rows); err != nil {
		t.Fatalf("materializeFiles(videos) error = %v", err)
	}
	if err := upsertRows(ctx, db, targetVideosItem, vidPage.Rows); err != nil {
		t.Fatalf("upsertRows(videos) applied out of order returned an error, want none (soft reference): %v", err)
	}

	var danglingCount int
	if err := db.QueryRow(fmt.Sprintf(
		`SELECT count(*) FROM %s v WHERE NOT EXISTS (SELECT 1 FROM %s i WHERE i.uid = v.image_uid)`,
		targetVideosTable, targetImagesTable,
	)).Scan(&danglingCount); err != nil {
		t.Fatalf("count dangling references: %v", err)
	}
	if danglingCount != 1 {
		t.Fatalf("dangling image_uid count = %d, want 1 (images not synced yet)", danglingCount)
	}

	// Now apply images -- the reference should self-heal with no data
	// migration or re-sync of the video row required.
	imgPage, err := fetchChangesPage(db, sourceImagesItem, "", 100)
	if err != nil {
		t.Fatalf("fetchChangesPage(images) error = %v", err)
	}
	if err := materializeFiles(ctx, targetImagesItem, fileSrv.URL, secret, imgPage.Rows); err != nil {
		t.Fatalf("materializeFiles(images) error = %v", err)
	}
	if err := upsertRows(ctx, db, targetImagesItem, imgPage.Rows); err != nil {
		t.Fatalf("upsertRows(images) error = %v", err)
	}

	if err := db.QueryRow(fmt.Sprintf(
		`SELECT count(*) FROM %s v WHERE NOT EXISTS (SELECT 1 FROM %s i WHERE i.uid = v.image_uid)`,
		targetVideosTable, targetImagesTable,
	)).Scan(&danglingCount); err != nil {
		t.Fatalf("count dangling references after images sync: %v", err)
	}
	if danglingCount != 0 {
		t.Fatalf("dangling image_uid count after images sync = %d, want 0 (self-healed)", danglingCount)
	}
}
