package docreviews

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"unicode"

	docprocessing "github.com/chendingplano/deepdoc/server/api/doc-processing"
	"golang.org/x/text/unicode/norm"
)

type complianceArtifact struct {
	Type     string `json:"type"`
	ID       string `json:"id"`
	RecordID int64  `json:"record_id"`
}

func (a complianceArtifact) key() string { return a.Type + "/" + a.ID }

type complianceEvidence struct {
	RecordID  int64  `json:"record_id"`
	LineStart int    `json:"line_start"`
	LineEnd   int    `json:"line_end"`
	Quote     string `json:"quote"`
}

type complianceDocument struct {
	RecordID       int64                 `json:"record_id"`
	Title          string                `json:"title"`
	DocumentNumber string                `json:"document_number"`
	Context        string                `json:"context"`
	Lines          []complianceInputLine `json:"lines"`
	Hash           string                `json:"sha256"`
	Complete       bool                  `json:"complete"`
	Issues         []string              `json:"issues,omitempty"`
}
type complianceDocumentEntry struct {
	once sync.Once
	doc  complianceDocument
	err  error
}

// Freeze each canonical source once for this reviewer instance, including concurrent chunks.
func (r *complianceReviewer) document(ctx context.Context, id int64) (complianceDocument, error) {
	entry, _ := r.documents.LoadOrStore(id, &complianceDocumentEntry{})
	e := entry.(*complianceDocumentEntry)
	e.once.Do(func() {
		if r.loadDocument == nil {
			e.err = fmt.Errorf("compliance: canonical evidence loader is not configured")
			return
		}
		e.doc, e.err = r.loadDocument(ctx, id)
		if e.err == nil && (e.doc.RecordID != id || len(e.doc.Lines) == 0) {
			e.err = fmt.Errorf("compliance: empty or mismatched canonical evidence for record %d", id)
		}
	})
	return e.doc, e.err
}

func loadComplianceDocument(ctx context.Context, db *sql.DB, id int64) (complianceDocument, error) {
	rec, err := (&DocMetadataSQLStore{DB: db}).GetInputRecord(ctx, id)
	if err != nil {
		return complianceDocument{}, err
	}
	lines, err := loadPromptCacheReviewLines(id, rec)
	if err != nil {
		return complianceDocument{}, err
	}
	d := complianceDocument{RecordID: id, Title: rec.Title, DocumentNumber: rec.DocumentNumber, Context: buildDocContextLine(rec), Complete: true}
	// Use canonical lines rather than indexed snippets, including full table markup.
	for _, line := range lines {
		d.Lines = append(d.Lines, complianceInputLine{LineNumber: line.LineNo, Content: line.Content})
	}
	raw, _ := json.Marshal(d.Lines)
	sum := sha256.Sum256(raw)
	d.Hash = hex.EncodeToString(sum[:])
	// The canonical text alone cannot certify successful OCR or image interpretation.
	for _, line := range lines {
		if complianceUnreadableLine(line) {
			d.Complete = false
			d.Issues = append(d.Issues, "Image content requires independent evidence; text coverage is incomplete.")
			break
		}
	}
	return d, nil
}

func complianceUnreadableLine(line Line) bool {
	typ := strings.ToLower(line.LineType)
	s := strings.ToLower(strings.TrimSpace(line.Content))
	if strings.Contains(typ, "image") || strings.Contains(s, "![") || strings.Contains(s, "<img") || strings.Contains(s, "<svg") || strings.Contains(s, "data:image/") {
		return true
	}
	for _, ext := range []string{".png", ".jpg", ".jpeg", ".webp", ".gif", ".tif", ".tiff", ".svg"} {
		if strings.HasSuffix(s, ext) {
			return true
		}
	}
	return false
}

func matchesComplianceCitation(citation string, d complianceDocument) bool {
	key, ok := docprocessing.RefCitationIdentityKey(citation)
	if !ok {
		return strings.EqualFold(strings.TrimSpace(citation), strings.TrimSpace(d.Title))
	}
	own, ok := docprocessing.RefOwnIdentityKey(d.DocumentNumber, d.Title)
	return ok && own == key
}

func boundedComplianceDocument(d complianceDocument, limit int) complianceDocument {
	out := d
	out.Lines = nil
	n := 0
	for _, line := range d.Lines {
		size := len([]rune(line.Content))
		if n+size > limit {
			out.Complete = false
			out.Issues = append(out.Issues, "Canonical text exceeded the per-source character budget.")
			break
		}
		out.Lines = append(out.Lines, line)
		n += size
	}
	return out
}

// complianceLooseText keeps only letters and digits, so a model's copy that changes spacing,
// punctuation, full-width forms or markdown escapes (90%\~105% quoted as 90%~105%) still matches.
// It identifies where a quote came from; evidence itself must pass verifyComplianceEvidence.
func complianceLooseText(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return r
		}
		return -1
	}, norm.NFKC.String(s))
}

func verifyComplianceEvidence(e complianceEvidence, docs map[int64]complianceDocument) error {
	d, ok := docs[e.RecordID]
	if !ok || e.LineStart <= 0 || e.LineEnd < e.LineStart || strings.TrimSpace(e.Quote) == "" {
		return fmt.Errorf("compliance: invalid evidence anchor for record %d lines %d-%d", e.RecordID, e.LineStart, e.LineEnd)
	}
	var parts []string
	start, end := false, false
	for _, line := range d.Lines {
		if line.LineNumber == e.LineStart {
			start = true
		}
		if line.LineNumber == e.LineEnd {
			end = true
		}
		if line.LineNumber >= e.LineStart && line.LineNumber <= e.LineEnd {
			parts = append(parts, line.Content)
		}
	}
	if !start || !end || !strings.Contains(strings.Join(parts, "\n"), e.Quote) {
		return fmt.Errorf("compliance: quote does not match pinned record %d lines %d-%d", e.RecordID, e.LineStart, e.LineEnd)
	}
	return nil
}
