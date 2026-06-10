package main

import (
	"archive/zip"
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const minimalDocxXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
  <w:body>
    <w:p><w:r><w:t>First line</w:t></w:r></w:p>
    <w:p><w:r><w:t>Second line</w:t></w:r></w:p>
    <w:p></w:p>
    <w:p><w:r><w:t>Third line</w:t></w:r></w:p>
  </w:body>
</w:document>`

func makeTestDocx(t *testing.T, dir, name, xmlContent string) string {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create("word/document.xml")
	if err != nil {
		t.Fatalf("create document.xml entry: %v", err)
	}
	if _, err := w.Write([]byte(xmlContent)); err != nil {
		t.Fatalf("write document.xml: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatalf("write test docx: %v", err)
	}
	return path
}

func TestDocxParser_RejectsDotDocFormat(t *testing.T) {
	var p DocxParser
	_, _, err := p.Parse("/some/path/document.doc", "/some/path/document_docx.txt")
	if err == nil {
		t.Fatal("expected error for .doc file, got nil")
	}
	if !errors.Is(err, ErrDocFormatNotSupported) {
		t.Fatalf("expected ErrDocFormatNotSupported, got %v", err)
	}
}

func TestDocxParser_RejectsUnknownExtension(t *testing.T) {
	var p DocxParser
	_, _, err := p.Parse("/some/path/document.txt", "/some/path/document_docx.txt")
	if err == nil {
		t.Fatal("expected error for unsupported extension, got nil")
	}
	if errors.Is(err, ErrDocFormatNotSupported) {
		t.Fatal("should not return ErrDocFormatNotSupported for .txt, got wrong sentinel")
	}
}

func TestExtractDocxParagraphs_BasicParagraphs(t *testing.T) {
	paras, err := extractDocxParagraphs(minimalDocxXML)
	if err != nil {
		t.Fatalf("extractDocxParagraphs: %v", err)
	}
	if len(paras) != 3 {
		t.Fatalf("expected 3 non-empty paragraphs, got %d: %v", len(paras), paras)
	}
	if paras[0] != "First line" {
		t.Errorf("paras[0] = %q, want %q", paras[0], "First line")
	}
	if paras[1] != "Second line" {
		t.Errorf("paras[1] = %q, want %q", paras[1], "Second line")
	}
	if paras[2] != "Third line" {
		t.Errorf("paras[2] = %q, want %q", paras[2], "Third line")
	}
}

func TestExtractDocxParagraphs_SkipsEmptyParagraphs(t *testing.T) {
	xmlWithEmpties := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
  <w:body>
    <w:p></w:p>
    <w:p><w:r><w:t>Content</w:t></w:r></w:p>
    <w:p></w:p>
  </w:body>
</w:document>`
	paras, err := extractDocxParagraphs(xmlWithEmpties)
	if err != nil {
		t.Fatalf("extractDocxParagraphs: %v", err)
	}
	if len(paras) != 1 || paras[0] != "Content" {
		t.Fatalf("expected [\"Content\"], got %v", paras)
	}
}

func TestExtractDocxParagraphs_MultiRunParagraph(t *testing.T) {
	xmlMultiRun := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
  <w:body>
    <w:p>
      <w:r><w:t xml:space="preserve">Hello </w:t></w:r>
      <w:r><w:t>World</w:t></w:r>
    </w:p>
  </w:body>
</w:document>`
	paras, err := extractDocxParagraphs(xmlMultiRun)
	if err != nil {
		t.Fatalf("extractDocxParagraphs: %v", err)
	}
	if len(paras) != 1 {
		t.Fatalf("expected 1 paragraph, got %d", len(paras))
	}
	if paras[0] != "Hello World" {
		t.Errorf("paras[0] = %q, want %q", paras[0], "Hello World")
	}
}

func TestWriteDocxLineFile_ProducesTabDelimitedLines(t *testing.T) {
	dir := t.TempDir()
	outPath := filepath.Join(dir, "doc_docx.txt")
	paragraphs := []string{"Hello World", "Second\tparagraph", "Third\nparagraph"}

	count, err := writeDocxLineFile(outPath, paragraphs)
	if err != nil {
		t.Fatalf("writeDocxLineFile: %v", err)
	}
	if count != 3 {
		t.Fatalf("expected count=3, got %d", count)
	}

	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	lines := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d: %v", len(lines), lines)
	}

	fields := strings.Split(lines[0], "\t")
	if len(fields) != 7 {
		t.Fatalf("expected 7 tab-separated fields in line 0, got %d: %q", len(fields), lines[0])
	}
	if fields[0] != "1" {
		t.Errorf("field[0] (lineNum) = %q, want \"1\"", fields[0])
	}
	if fields[1] != "1" {
		t.Errorf("field[1] (page) = %q, want \"1\"", fields[1])
	}
	if fields[2] != "paragraph" {
		t.Errorf("field[2] (type) = %q, want \"paragraph\"", fields[2])
	}
	if fields[3] != "unknown-font" {
		t.Errorf("field[3] (font) = %q, want \"unknown-font\"", fields[3])
	}
	if fields[4] != "12" {
		t.Errorf("field[4] (fontSize) = %q, want \"12\"", fields[4])
	}
	if fields[5] != "[]" {
		t.Errorf("field[5] (coord) = %q, want \"[]\"", fields[5])
	}
	if fields[6] != "Hello World" {
		t.Errorf("field[6] (content) = %q, want \"Hello World\"", fields[6])
	}

	// tab escape in second line
	fields2 := strings.Split(lines[1], "\t")
	if len(fields2) != 7 {
		t.Fatalf("line 1 has %d fields, expected 7", len(fields2))
	}
	if fields2[6] != `Second\tparagraph` {
		t.Errorf("tab escape: got %q, want %q", fields2[6], `Second\tparagraph`)
	}

	// newline escape in third line
	fields3 := strings.Split(lines[2], "\t")
	if len(fields3) != 7 {
		t.Fatalf("line 2 has %d fields, expected 7", len(fields3))
	}
	if fields3[6] != `Third\nparagraph` {
		t.Errorf("newline escape: got %q, want %q", fields3[6], `Third\nparagraph`)
	}
}

func TestWriteDocxLineFile_EmptyParagraphsProducesEmptyFile(t *testing.T) {
	dir := t.TempDir()
	outPath := filepath.Join(dir, "empty_docx.txt")

	count, err := writeDocxLineFile(outPath, nil)
	if err != nil {
		t.Fatalf("writeDocxLineFile with nil: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected count=0, got %d", count)
	}

	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	if len(data) != 0 {
		t.Errorf("expected empty file, got %d bytes", len(data))
	}
}

func TestDocxParser_Parse_ProducesLineFile(t *testing.T) {
	dir := t.TempDir()
	docxPath := makeTestDocx(t, dir, "report.docx", minimalDocxXML)

	var p DocxParser
	// Caller computes outputPath from record dir + staging filename, as the worker does.
	outputPath := docxOutputPath(dir, "report.docx")
	lineFilePath, count, err := p.Parse(docxPath, outputPath)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if count != 3 {
		t.Fatalf("expected 3 lines, got %d", count)
	}

	expectedPath := filepath.Join(dir, "report_docx.txt")
	if lineFilePath != expectedPath {
		t.Fatalf("lineFilePath = %q, want %q", lineFilePath, expectedPath)
	}
	if _, err := os.Stat(lineFilePath); err != nil {
		t.Fatalf("line file not found at %q: %v", lineFilePath, err)
	}

	data, err := os.ReadFile(lineFilePath)
	if err != nil {
		t.Fatalf("read line file: %v", err)
	}
	lines := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 output lines, got %d", len(lines))
	}
}

func TestDocxOutputPath(t *testing.T) {
	// dir = ARTIFACT_DIR/<group_id>/<record_id>/, stagingFilename = kb.inputs.staging_filename
	cases := []struct {
		dir      string
		filename string
		want     string
	}{
		{"/data/Artifacts/0/53", "report.docx", "/data/Artifacts/0/53/report_docx.txt"},
		{"/data/Artifacts/0/53", "REPORT.DOCX", "/data/Artifacts/0/53/REPORT_docx.txt"},
		{"/data/Artifacts/0/53", "file.doc", "/data/Artifacts/0/53/file_docx.txt"},
		{"/data/Artifacts/1/1042", "quarterly report.docx", "/data/Artifacts/1/1042/quarterly report_docx.txt"},
	}
	for _, tc := range cases {
		got := docxOutputPath(tc.dir, tc.filename)
		if got != tc.want {
			t.Errorf("docxOutputPath(%q, %q) = %q, want %q", tc.dir, tc.filename, got, tc.want)
		}
	}
}

func TestAppendParsedStatus_AddsSuccessEntry(t *testing.T) {
	start := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)
	got, err := appendParsedStatus("[]", start, 150, nil)
	if err != nil {
		t.Fatalf("appendParsedStatus: %v", err)
	}
	if !strings.Contains(got, `"parsed"`) {
		t.Errorf("expected operation=parsed in %q", got)
	}
	if !strings.Contains(got, `"success"`) {
		t.Errorf("expected proc-status=success in %q", got)
	}
}

func TestAppendParsedStatus_AddsFailedEntry(t *testing.T) {
	start := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)
	got, err := appendParsedStatus("[]", start, 50, errors.New("parse error"))
	if err != nil {
		t.Fatalf("appendParsedStatus: %v", err)
	}
	if !strings.Contains(got, `"failed"`) {
		t.Errorf("expected proc-status=failed in %q", got)
	}
	if !strings.Contains(got, "parse error") {
		t.Errorf("expected error message in %q", got)
	}
}

func TestAppendParsedStatus_ReplacesExistingParsedEntry(t *testing.T) {
	existing := `[{"operation":"parsed","proc-status":"failed","error":"old"}]`
	start := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)
	got, err := appendParsedStatus(existing, start, 100, nil)
	if err != nil {
		t.Fatalf("appendParsedStatus: %v", err)
	}
	// Should have exactly one "parsed" entry
	count := strings.Count(got, `"parsed"`)
	if count != 1 {
		t.Errorf("expected exactly 1 parsed entry, got %d in %q", count, got)
	}
	if strings.Contains(got, "old") {
		t.Errorf("expected old error to be replaced in %q", got)
	}
}

func TestAppendParsedStatus_PreservesOtherEntries(t *testing.T) {
	existing := `[{"operation":"converted","proc-status":"success"}]`
	start := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)
	got, err := appendParsedStatus(existing, start, 100, nil)
	if err != nil {
		t.Fatalf("appendParsedStatus: %v", err)
	}
	if !strings.Contains(got, "converted") {
		t.Errorf("expected converted entry to be preserved in %q", got)
	}
	if !strings.Contains(got, `"parsed"`) {
		t.Errorf("expected parsed entry to be added in %q", got)
	}
}

func TestMaxDocGoroutines_DefaultsToFour(t *testing.T) {
	t.Setenv("MAX_DOC_GOROUTINE", "")
	if got := maxDocGoroutines(); got != 4 {
		t.Errorf("maxDocGoroutines() = %d, want 4", got)
	}
}

func TestMaxDocGoroutines_ReadsEnvVar(t *testing.T) {
	t.Setenv("MAX_DOC_GOROUTINE", "8")
	if got := maxDocGoroutines(); got != 8 {
		t.Errorf("maxDocGoroutines() = %d, want 8", got)
	}
}

func TestMaxDocGoroutines_InvalidFallsBackToFour(t *testing.T) {
	t.Setenv("MAX_DOC_GOROUTINE", "bad")
	if got := maxDocGoroutines(); got != 4 {
		t.Errorf("maxDocGoroutines() = %d, want 4 for invalid input", got)
	}
}
