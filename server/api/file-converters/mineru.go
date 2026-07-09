package fileconverters

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type mineruDocument struct {
	Pages []mineruPage `json:"pages"`
}

type mineruPage struct {
	PageNumber int          `json:"page_number"`
	Items      []mineruItem `json:"items"`
}

type mineruItem struct {
	Type           string            `json:"type"`
	Text           string            `json:"text"`
	TextLevel      *int              `json:"text_level"`
	ListItems      []string          `json:"list_items"`
	ListItemBBoxes []json.RawMessage `json:"list_item_bboxes"`
	TableCaption   []string          `json:"table_caption"`
	TableFootnote  []string          `json:"table_footnote"`
	TableBody      string            `json:"table_body"`
	ImgPath        string            `json:"img_path"`
	ImageCaption   []string          `json:"image_caption"`
	ImageFootnote  []string          `json:"image_footnote"`
	BBox           json.RawMessage   `json:"bbox"`
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
				perItemBBoxes := item.ListItemBBoxes
				hasPerItemBBoxes := len(perItemBBoxes) == len(item.ListItems)
				for i, s := range item.ListItems {
					content := strings.TrimSpace(s)
					if content == "" {
						continue
					}
					itemBBox := bbox
					if hasPerItemBBoxes {
						if b := mineruBBoxStr(perItemBBoxes[i]); b != "" {
							itemBBox = b
						}
					}
					items = append(items, extractedOpenDataLine{
						Page:    pageStr,
						Type:    "list-item",
						BBox:    itemBBox,
						Content: content,
					})
				}

			case "equation":
				bbox := mineruBBoxStr(item.BBox)
				if content := strings.TrimSpace(item.Text); content != "" {
					items = append(items, extractedOpenDataLine{
						Page:    pageStr,
						Type:    "equation",
						BBox:    bbox,
						Content: content,
					})
				}
				if imgPath := strings.TrimSpace(item.ImgPath); imgPath != "" {
					items = append(items, extractedOpenDataLine{
						Page:    pageStr,
						Type:    "equation-image",
						BBox:    bbox,
						Content: imgPath,
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
				if imgPath := strings.TrimSpace(item.ImgPath); imgPath != "" {
					items = append(items, extractedOpenDataLine{
						Page:    pageStr,
						Type:    "table-image",
						BBox:    bbox,
						Content: imgPath,
					})
				}
				if tableBody := strings.TrimSpace(item.TableBody); tableBody != "" {
					items = append(items, extractedOpenDataLine{
						Page:    pageStr,
						Type:    "table",
						BBox:    bbox,
						Content: tableBody,
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

			case "image":
				bbox := mineruBBoxStr(item.BBox)
				for _, cap := range item.ImageCaption {
					if c := strings.TrimSpace(cap); c != "" {
						items = append(items, extractedOpenDataLine{
							Page:    pageStr,
							Type:    "image-caption",
							BBox:    bbox,
							Content: c,
						})
					}
				}
				if imgPath := strings.TrimSpace(item.ImgPath); imgPath != "" {
					items = append(items, extractedOpenDataLine{
						Page:    pageStr,
						Type:    "image",
						BBox:    bbox,
						Content: imgPath,
					})
				}
				for _, fn := range item.ImageFootnote {
					if f := strings.TrimSpace(fn); f != "" {
						items = append(items, extractedOpenDataLine{
							Page:    pageStr,
							Type:    "image-footnote",
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

func mineruBBoxStr(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var nums []json.Number
	if err := json.Unmarshal(raw, &nums); err != nil {
		return ""
	}
	parts := make([]string, 0, len(nums))
	for _, n := range nums {
		token := strings.TrimSpace(string(n))
		if _, err := strconv.ParseFloat(token, 64); err != nil {
			continue
		}
		parts = append(parts, token)
	}
	return "[" + strings.Join(parts, ", ") + "]"
}
