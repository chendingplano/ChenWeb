package gold

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/chendingplano/deepdoc/server/api/cdm/model"
	"github.com/chendingplano/deepdoc/server/api/cdm/rendering"
)

func loadFixture(t *testing.T) File {
	t.Helper()
	f, err := Load("gold.toml")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(f.AuthorityDocument) == 0 || len(f.Clause) == 0 {
		t.Fatal("gold.toml loaded with no authority_document or clause rows -- fixture path or schema likely broken")
	}
	return f
}

func TestBuildDocumentsOneBlockPerClauseAndValid(t *testing.T) {
	f := loadFixture(t)
	docs := BuildDocuments(f)
	if len(docs) != len(f.AuthorityDocument) {
		t.Fatalf("got %d documents, want %d (one per authority_document)", len(docs), len(f.AuthorityDocument))
	}

	clauseCount := map[string]int{}
	textByClauseID := map[string]string{}
	for _, c := range f.Clause {
		clauseCount[c.Document]++
		textByClauseID[c.ID] = c.TextTemplate
	}

	for _, doc := range docs {
		t.Run(doc.Key, func(t *testing.T) {
			if err := model.Validate(&doc); err != nil {
				t.Fatalf("generated document fails CDM validation: %v", err)
			}
			if want := clauseCount[doc.Key]; len(doc.Blocks) != want {
				t.Fatalf("document %s has %d blocks, want %d (one per clause)", doc.Key, len(doc.Blocks), want)
			}
			for _, b := range doc.Blocks {
				want, ok := textByClauseID[b.ID]
				if !ok {
					t.Fatalf("block %s does not correspond to any clause id", b.ID)
				}
				if len(b.Content) != 1 || b.Content[0].Text != want {
					t.Fatalf("block %s content = %+v, want single text node %q", b.ID, b.Content, want)
				}
			}
		})
	}
}

// TestGroundingRoundTrip renders the richest single generated document (the
// enterprise 2026 standard, one clause per metric) through the existing CDM
// Typst pipeline and confirms every clause block gets a start and end anchor
// mark -- i.e. ADR 2026072901 DR25's "grounding is exact by construction"
// claim holds for content generated from this gold fixture, not only for the
// CDM package's own hand-written test fixtures.
func TestGroundingRoundTrip(t *testing.T) {
	typstBin, err := exec.LookPath("typst")
	if err != nil {
		t.Skip("typst not found on PATH")
	}

	f := loadFixture(t)
	docs := BuildDocuments(f)

	var subject *model.Document
	for i := range docs {
		if docs[i].Key == "doc:ent-q-syn-001-2026" {
			subject = &docs[i]
		}
	}
	if subject == nil {
		t.Fatal("gold.toml no longer has an authority_document doc:ent-q-syn-001-2026 -- update this test's target id")
	}
	if len(subject.Blocks) == 0 {
		t.Fatal("subject document has no blocks to ground")
	}

	r := &rendering.TypstRenderer{}
	src, err := r.RenderDocument(subject)
	if err != nil {
		t.Fatalf("render: %v", err)
	}

	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, "theme.typ"), rendering.DefaultTheme, 0o644); err != nil {
		t.Fatalf("write theme.typ: %v", err)
	}
	typPath := filepath.Join(tmp, "doc.typ")
	if err := os.WriteFile(typPath, src, 0o644); err != nil {
		t.Fatalf("write doc.typ: %v", err)
	}

	marks, err := rendering.ExtractAnchors(typstBin, typPath)
	if err != nil {
		t.Fatalf("extract anchors: %v", err)
	}

	starts := map[string]bool{}
	ends := map[string]bool{}
	for _, m := range marks {
		switch m.Kind {
		case "start":
			starts[m.ID] = true
		case "end":
			ends[m.ID] = true
		default:
			t.Fatalf("unexpected mark kind %q", m.Kind)
		}
	}

	for _, b := range subject.Blocks {
		if !starts[b.ID] {
			t.Errorf("clause block %q got no start anchor", b.ID)
		}
		if !ends[b.ID] {
			t.Errorf("clause block %q got no end anchor", b.ID)
		}
	}

	fragments, err := rendering.DeriveFragments(marks)
	if err != nil {
		t.Fatalf("derive fragments: %v", err)
	}
	fragmentsByUnit := map[string]int{}
	for _, frag := range fragments {
		fragmentsByUnit[frag.UnitID]++
	}
	for _, b := range subject.Blocks {
		if fragmentsByUnit[b.ID] == 0 {
			t.Errorf("clause block %q produced no highlight fragment", b.ID)
		}
	}
}
