package main

import (
	"archive/zip"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// ErrDocFormatNotSupported is returned when a legacy .doc binary file is passed to the parser.
var ErrDocFormatNotSupported = errors.New("(CWB_DOCX_001) .doc binary format is not supported; only .docx is supported")

// ErrDocImageOnly is returned when the docx contains no text — only embedded images.
// The caller should reroute the record to the PDF pipeline for OCR.
var ErrDocImageOnly = errors.New("(CWB_DOCX_006) docx contains no extractable text; all content is embedded images")

const wordNS = "http://schemas.openxmlformats.org/wordprocessingml/2006/main"

// DocxParser extracts text from .docx files and writes a tab-delimited line file.
type DocxParser struct{}

// Parse reads the .docx file at path, extracts paragraph text, and writes the
// tab-delimited line file to outputPath. It returns outputPath, the number of
// lines written, and any error.
//
// The caller is responsible for computing outputPath — typically via
// docxOutputPath(recordDir, stagingFilename). This keeps output placement
// explicit and tied to kb.inputs.staging_filename.
//
// .doc (legacy binary) files are rejected with ErrDocFormatNotSupported.
func (DocxParser) Parse(path, outputPath string) (lineFilePath string, lineCount int, err error) {
	ext := strings.ToLower(strings.TrimSpace(filepath.Ext(path)))
	if ext == ".doc" {
		return "", 0, ErrDocFormatNotSupported
	}
	if ext != ".docx" {
		return "", 0, fmt.Errorf("(CWB_DOCX_002) unsupported file extension: %q", ext)
	}

	xmlContent, err := readDocxXML(path)
	if err != nil {
		return "", 0, fmt.Errorf("(CWB_DOCX_003) read docx file: %w", err)
	}

	profile, err := probeDocxContent(xmlContent)
	if err != nil {
		return "", 0, fmt.Errorf("(CWB_DOCX_004) probe docx content: %w", err)
	}
	if profile.IsImageOnly() {
		return "", 0, ErrDocImageOnly
	}

	paragraphs, err := extractDocxParagraphs(xmlContent)
	if err != nil {
		return "", 0, fmt.Errorf("(CWB_DOCX_004) parse docx xml: %w", err)
	}

	lineCount, err = writeDocxLineFile(outputPath, paragraphs)
	if err != nil {
		return "", 0, fmt.Errorf("(CWB_DOCX_005) write line file: %w", err)
	}
	return outputPath, lineCount, nil
}

// readDocxXML opens the .docx zip archive and returns the raw XML of word/document.xml.
func readDocxXML(path string) (string, error) {
	r, err := zip.OpenReader(path)
	if err != nil {
		return "", err
	}
	defer r.Close()

	for _, f := range r.File {
		if f.Name != "word/document.xml" {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return "", err
		}
		defer rc.Close()
		b, err := io.ReadAll(rc)
		if err != nil {
			return "", err
		}
		return string(b), nil
	}
	return "", errors.New("word/document.xml not found in docx archive")
}

// DocxContentProfile summarises what types of content are present in word/document.xml.
type DocxContentProfile struct {
	TextRunes    int // total non-whitespace rune count across all w:t elements
	DrawingCount int // number of w:drawing elements
}

// IsImageOnly reports true when the document has embedded drawings but no text at all.
func (p DocxContentProfile) IsImageOnly() bool {
	return p.TextRunes == 0 && p.DrawingCount > 0
}

// probeDocxContent scans the Word document XML and returns a DocxContentProfile.
// It counts every w:t element regardless of nesting (including text boxes inside drawings).
func probeDocxContent(xmlContent string) (DocxContentProfile, error) {
	dec := xml.NewDecoder(strings.NewReader(xmlContent))
	var profile DocxContentProfile
	inText := false

	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return profile, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if t.Name.Space == wordNS {
				switch t.Name.Local {
				case "t":
					inText = true
				case "drawing":
					profile.DrawingCount++
				}
			}
		case xml.EndElement:
			if t.Name.Space == wordNS && t.Name.Local == "t" {
				inText = false
			}
		case xml.CharData:
			if inText {
				profile.TextRunes += len([]rune(strings.TrimSpace(string(t))))
			}
		}
	}
	return profile, nil
}

// docxOutputPath returns the *_docx.txt output path under dir, deriving the
// filename root from stagingFilename (kb.inputs.staging_filename). The result is:
//
//	<dir>/<staging_filename_root>_docx.txt
//
// where <dir> is typically ARTIFACT_DIR/<group_id>/<record_id>/.
func docxOutputPath(dir, stagingFilename string) string {
	base := filepath.Base(stagingFilename)
	root := strings.TrimSuffix(base, filepath.Ext(base))
	return filepath.Join(dir, root+"_docx.txt")
}

// extractDocxParagraphs streams the Word document XML and returns one string per
// non-empty paragraph. Text from multiple runs within a paragraph is concatenated.
func extractDocxParagraphs(xmlContent string) ([]string, error) {
	dec := xml.NewDecoder(strings.NewReader(xmlContent))

	var paragraphs []string
	var currentPara strings.Builder
	inParagraph := false
	inText := false

	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}

		switch t := tok.(type) {
		case xml.StartElement:
			if t.Name.Space == wordNS {
				switch t.Name.Local {
				case "p":
					inParagraph = true
					currentPara.Reset()
				case "t":
					if inParagraph {
						inText = true
					}
				}
			}
		case xml.EndElement:
			if t.Name.Space == wordNS {
				switch t.Name.Local {
				case "p":
					if inParagraph {
						text := strings.TrimSpace(currentPara.String())
						if text != "" {
							paragraphs = append(paragraphs, text)
						}
						currentPara.Reset()
						inParagraph = false
					}
				case "t":
					inText = false
				}
			}
		case xml.CharData:
			if inText {
				currentPara.Write(t)
			}
		}
	}

	return paragraphs, nil
}

// writeDocxLineFile writes paragraphs to outPath in the tab-delimited line file format:
// lineNum \t page \t type \t font \t fontSize \t coordinate \t content
func writeDocxLineFile(outPath string, paragraphs []string) (int, error) {
	var sb strings.Builder
	for i, para := range paragraphs {
		fields := []string{
			strconv.Itoa(i + 1),
			"1",
			"paragraph",
			"unknown-font",
			"12",
			"[]",
			escapeDocxLineContent(para),
		}
		sb.WriteString(strings.Join(fields, "\t"))
		sb.WriteByte('\n')
	}
	if err := os.WriteFile(outPath, []byte(sb.String()), 0o644); err != nil {
		return 0, err
	}
	return len(paragraphs), nil
}

// escapeDocxLineContent replaces literal newlines and tabs with their escape sequences
// so content stays on a single tab-delimited line.
func escapeDocxLineContent(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\\n")
	s = strings.ReplaceAll(s, "\n", "\\n")
	s = strings.ReplaceAll(s, "\r", "\\n")
	s = strings.ReplaceAll(s, "\t", "\\t")
	return s
}
