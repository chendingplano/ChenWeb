package fileconverters

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"golang.org/x/net/html"
)

type mineruDocument struct {
	Pages []mineruPage `json:"pages"`
}

type mineruPage struct {
	PageNumber int          `json:"page_number"`
	Items      []mineruItem `json:"items"`
}

type mineruItem struct {
	Type          string          `json:"type"`
	Text          string          `json:"text"`
	TextLevel     *int            `json:"text_level"`
	ListItems     []string        `json:"list_items"`
	TableCaption  []string        `json:"table_caption"`
	TableFootnote []string        `json:"table_footnote"`
	TableBody     string          `json:"table_body"`
	BBox          json.RawMessage `json:"bbox"`
}

func ConvertMineruFile(inputPath string) (string, error) {
	inputPath = strings.TrimSpace(inputPath)
	if inputPath == "" {
		return "", fmt.Errorf("input path is empty")
	}

	raw, err := os.ReadFile(inputPath)
	if err != nil {
		return "", fmt.Errorf("read input file: %w", err)
	}

	var doc mineruDocument
	if err := json.Unmarshal(raw, &doc); err != nil {
		return "", fmt.Errorf("parse mineru json: %w", err)
	}

	items := extractMineruLineItems(doc.Pages)
	items = filterRepeatedContentLines(items, len(doc.Pages))
	lines := formatOpenDataLines(items)

	outputPath := mineruOutputPath(inputPath)
	content := strings.Join(lines, "\n")
	if len(lines) > 0 {
		content += "\n"
	}
	if err := os.WriteFile(outputPath, []byte(content), 0o644); err != nil {
		return "", fmt.Errorf("write output file: %w", err)
	}
	originPath := strings.TrimSuffix(outputPath, filepath.Ext(outputPath)) + ".origin"
	if err := writeReadOnlyFile(originPath, []byte(content), 0o444); err != nil {
		return "", fmt.Errorf("write origin file: %w", err)
	}

	return outputPath, nil
}

func mineruOutputPath(inputPath string) string {
	root := strings.TrimSuffix(inputPath, filepath.Ext(inputPath))
	if strings.HasSuffix(strings.ToLower(root), "_mineru") {
		return root + ".txt"
	}
	return root + "_mineru.txt"
}

func extractMineruLineItems(pages []mineruPage) []extractedOpenDataLine {
	var items []extractedOpenDataLine
	for _, page := range pages {
		pageStr := strconv.Itoa(page.PageNumber)
		for _, item := range page.Items {
			switch strings.ToLower(strings.TrimSpace(item.Type)) {
			case "header", "footer", "page_number":
				// page furniture; skip

			case "text":
				content := strings.TrimSpace(item.Text)
				if content == "" {
					continue
				}
				lineType, headingLevel := "paragraph", ""
				if item.TextLevel != nil && *item.TextLevel > 0 {
					lineType = "heading"
					headingLevel = strconv.Itoa(*item.TextLevel)
				}
				items = append(items, extractedOpenDataLine{
					Page:         pageStr,
					Type:         lineType,
					HeadingLevel: headingLevel,
					BBox:         mineruBBoxStr(item.BBox),
					Content:      content,
				})

			case "list":
				bbox := mineruBBoxStr(item.BBox)
				for _, s := range item.ListItems {
					if content := strings.TrimSpace(s); content != "" {
						items = append(items, extractedOpenDataLine{
							Page:    pageStr,
							Type:    "list-item",
							BBox:    bbox,
							Content: content,
						})
					}
				}

			case "equation":
				if content := strings.TrimSpace(item.Text); content != "" {
					items = append(items, extractedOpenDataLine{
						Page:    pageStr,
						Type:    "equation",
						BBox:    mineruBBoxStr(item.BBox),
						Content: content,
					})
				}

			case "table":
				bbox := mineruBBoxStr(item.BBox)
				for _, cap := range item.TableCaption {
					if c := strings.TrimSpace(cap); c != "" {
						items = append(items, extractedOpenDataLine{
							Page:    pageStr,
							Type:    "table-caption",
							BBox:    bbox,
							Content: c,
						})
					}
				}
				for _, row := range parseMineruHTMLTableRows(item.TableBody) {
					items = append(items, extractedOpenDataLine{
						Page:    pageStr,
						Type:    "table-row",
						BBox:    bbox,
						Content: markdownRow(row),
					})
				}
				for _, fn := range item.TableFootnote {
					if f := strings.TrimSpace(fn); f != "" {
						items = append(items, extractedOpenDataLine{
							Page:    pageStr,
							Type:    "table-footnote",
							BBox:    bbox,
							Content: f,
						})
					}
				}
			}
		}
	}
	return items
}

func parseMineruHTMLTableRows(htmlBody string) [][]string {
	if strings.TrimSpace(htmlBody) == "" {
		return nil
	}
	z := html.NewTokenizer(strings.NewReader(htmlBody))
	var rows [][]string
	var currentRow []string
	var cellBuf strings.Builder
	inCell := false
	for {
		tt := z.Next()
		if tt == html.ErrorToken {
			break
		}
		switch tt {
		case html.StartTagToken:
			name, _ := z.TagName()
			switch string(name) {
			case "tr":
				currentRow = []string{}
			case "td", "th":
				inCell = true
				cellBuf.Reset()
			}
		case html.EndTagToken:
			name, _ := z.TagName()
			switch string(name) {
			case "td", "th":
				if inCell {
					currentRow = append(currentRow, strings.TrimSpace(cellBuf.String()))
					inCell = false
				}
			case "tr":
				if currentRow != nil {
					rows = append(rows, currentRow)
					currentRow = nil
				}
			}
		case html.TextToken:
			if inCell {
				cellBuf.Write(z.Text())
			}
		}
	}
	return rows
}

func mineruBBoxStr(raw json.RawMessage) string {
	s := strings.TrimSpace(string(raw))
	if s == "" || s == "null" {
		return "[]"
	}
	return s
}
