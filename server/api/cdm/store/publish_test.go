package store_test

import (
	"context"
	"os"
	"os/exec"
	"testing"

	"github.com/chendingplano/deepdoc/server/api/cdm/cdmfixtures"
	"github.com/chendingplano/deepdoc/server/api/cdm/store"
)

func requireTypstBin(t *testing.T) string {
	t.Helper()
	bin, err := exec.LookPath("typst")
	if err != nil {
		t.Skip("typst not found on PATH")
	}
	return bin
}

func TestPublisher_PublishEndToEnd(t *testing.T) {
	db := testDB(t)
	typstBin := requireTypstBin(t)
	theme, err := os.ReadFile("../rendering/theme.typ")
	if err != nil {
		t.Fatalf("read theme: %v", err)
	}

	docStore := store.New(db)
	inputs := store.NewInputRegistrar(db)
	pub := store.NewPublisher(db, theme, typstBin)

	key := uniqueKey(t)
	cleanupDocument(t, db, key)

	doc := cdmfixtures.JaroWinkler()
	doc.Key = key
	if _, err := docStore.Save(context.Background(), &doc, 0); err != nil {
		t.Fatalf("save: %v", err)
	}

	inputID, err := inputs.CreateDraft(context.Background(), store.DraftInput{
		TenantID: "tenant-x", Title: doc.Title,
	})
	if err != nil {
		t.Fatalf("create draft: %v", err)
	}
	cleanupInput(t, db, inputID)

	res, err := pub.Publish(context.Background(), key, inputID)
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	if res.ContentVersion != 1 {
		t.Errorf("expected content_version 1, got %d", res.ContentVersion)
	}
	if res.PageCount < 1 {
		t.Errorf("expected at least 1 rendered page, got %d", res.PageCount)
	}
	if res.LineCount == 0 {
		t.Error("expected a non-empty line file")
	}

	// Publish must have transitioned kb.inputs: parse_state stays
	// parsed_success, pipeline_state moves to pending (spec §10.1).
	st, err := inputs.LoadInputState(context.Background(), inputID)
	if err != nil {
		t.Fatalf("load input state: %v", err)
	}
	if st.ParseState != "parsed_success" {
		t.Errorf("expected parse_state parsed_success after publish, got %q", st.ParseState)
	}
	if st.PipelineState != "pending" {
		t.Errorf("expected pipeline_state pending after publish, got %q", st.PipelineState)
	}

	// Verify the stored renderings: typst source, line-file, and at least
	// one SVG page all landed in kb.cdm_renderings.
	var typstCount, lineFileCount, svgCount int
	rows := map[string]*int{"typst": &typstCount, "line-file": &lineFileCount, "svg": &svgCount}
	for renderer, dest := range rows {
		if err := db.QueryRow(`
			SELECT count(*) FROM kb.cdm_renderings r
			JOIN kb.cdm_documents d ON d.id = r.document_id
			WHERE d.document_key = $1 AND r.renderer = $2
		`, key, renderer).Scan(dest); err != nil {
			t.Fatalf("count %s renderings: %v", renderer, err)
		}
	}
	if typstCount != 1 {
		t.Errorf("expected exactly 1 typst rendering row, got %d", typstCount)
	}
	if lineFileCount != 1 {
		t.Errorf("expected exactly 1 line-file rendering row, got %d", lineFileCount)
	}
	if svgCount != res.PageCount {
		t.Errorf("expected %d svg rendering rows, got %d", res.PageCount, svgCount)
	}

	// Verify anchors: at least one row per line.
	var anchorCount int
	if err := db.QueryRow(`
		SELECT count(*) FROM kb.cdm_anchors a
		JOIN kb.cdm_documents d ON d.id = a.document_id
		WHERE d.document_key = $1
	`, key).Scan(&anchorCount); err != nil {
		t.Fatalf("count anchors: %v", err)
	}
	if anchorCount < res.LineCount {
		t.Errorf("expected at least %d anchor rows (one per line), got %d", res.LineCount, anchorCount)
	}

	// Resolve a highlight for line 1 and confirm it returns a well-formed
	// fragment -- the same {page, bbox} shape the PDF path returns (spec
	// "Navigate and highlight parity").
	frags, err := pub.ResolveHighlight(context.Background(), key, res.ContentVersion, []int{1})
	if err != nil {
		t.Fatalf("resolve highlight: %v", err)
	}
	if len(frags) == 0 {
		t.Fatal("expected at least one fragment for line 1")
	}
	if frags[0].Page < 1 || frags[0].W <= 0 || frags[0].H <= 0 {
		t.Errorf("resolved fragment looks malformed: %+v", frags[0])
	}
}

func TestPublisher_RepublishSupersedesArtifacts(t *testing.T) {
	db := testDB(t)
	typstBin := requireTypstBin(t)
	theme, err := os.ReadFile("../rendering/theme.typ")
	if err != nil {
		t.Fatalf("read theme: %v", err)
	}

	docStore := store.New(db)
	inputs := store.NewInputRegistrar(db)
	pub := store.NewPublisher(db, theme, typstBin)

	key := uniqueKey(t)
	cleanupDocument(t, db, key)

	doc := cdmfixtures.JaroWinkler()
	doc.Key = key
	if _, err := docStore.Save(context.Background(), &doc, 0); err != nil {
		t.Fatalf("save: %v", err)
	}
	inputID, err := inputs.CreateDraft(context.Background(), store.DraftInput{TenantID: "tenant-x"})
	if err != nil {
		t.Fatalf("create draft: %v", err)
	}
	cleanupInput(t, db, inputID)

	if _, err := pub.Publish(context.Background(), key, inputID); err != nil {
		t.Fatalf("first publish: %v", err)
	}

	// This document was created with Save + a separate CreateDraft, so its
	// input_record_id is NULL and the frozen rule (D8) does not apply to it.
	// That is deliberate here: this test is about artifact superseding across
	// content versions, not about the editorial lifecycle.
	doc.Title = "Jaro-Winkler Similarity (v2)"
	if _, err := docStore.Save(context.Background(), &doc, doc.ContentVersion); err != nil {
		t.Fatalf("resave: %v", err)
	}
	res2, err := pub.Publish(context.Background(), key, inputID)
	if err != nil {
		t.Fatalf("second publish: %v", err)
	}
	if res2.ContentVersion != 2 {
		t.Fatalf("expected content_version 2 on republish, got %d", res2.ContentVersion)
	}

	var v1Count, v2Count int
	if err := db.QueryRow(`
		SELECT count(*) FROM kb.cdm_renderings r JOIN kb.cdm_documents d ON d.id = r.document_id
		WHERE d.document_key = $1 AND r.content_version = 1
	`, key).Scan(&v1Count); err != nil {
		t.Fatalf("count v1 renderings: %v", err)
	}
	if err := db.QueryRow(`
		SELECT count(*) FROM kb.cdm_renderings r JOIN kb.cdm_documents d ON d.id = r.document_id
		WHERE d.document_key = $1 AND r.content_version = 2
	`, key).Scan(&v2Count); err != nil {
		t.Fatalf("count v2 renderings: %v", err)
	}
	if v1Count == 0 {
		t.Error("expected version 1's renderings to remain (not overwritten)")
	}
	if v2Count == 0 {
		t.Error("expected version 2's renderings to exist")
	}
}
