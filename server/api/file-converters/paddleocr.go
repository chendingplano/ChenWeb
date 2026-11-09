package fileconverters

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// paddleDocument is the PaddleOCR parser output written by
// python/pdf-parser/parser_paddle.py as <pdf-stem>_paddleocr.json:
//
//	{"pages": [{"page_number": 1, "parsing_res_list": [{block_label, block_content, block_bbox, page_number, image}]}], ...}
//
// Older PaddleOCR results nest the blocks under "res"; both shapes are read.
type paddleDocument struct {
	Pages []paddlePage `json:"pages"`
}

type paddlePage struct {
	PageNumber     *int           `json:"page_number"`
	ParsingResList []paddleBlock  `json:"parsing_res_list"`
	Res            *paddlePageRes `json:"res"`
}

type paddlePageRes struct {
	ParsingResList []paddleBlock `json:"parsing_res_list"`
}

type paddleBlock struct {
	BlockLabel   string          `json:"block_label"`
	BlockContent string          `json:"block_content"`
	BlockBBox    json.RawMessage `json:"block_bbox"`
}

// blocks returns the page's blocks in reading order, whichever shape was used.
func (p paddlePage) blocks() []paddleBlock {
	if len(p.ParsingResList) > 0 {
		return p.ParsingResList
	}
	if p.Res != nil {
		return p.Res.ParsingResList
	}
	return nil
}

// ConvertPaddleOCRFile turns <stem>_paddleocr.json into the Line File
// <stem>_paddleocr.txt, using the same line format as ConvertMineruFile.
func ConvertPaddleOCRFile(inputPath string) (string, error) {
	inputPath = strings.TrimSpace(inputPath)
	if inputPath == "" {
		return "", fmt.Errorf("input path is empty")
	}

	raw, err := os.ReadFile(inputPath)
	if err != nil {
		return "", fmt.Errorf("read input file: %w", err)
	}

	var doc paddleDocument
	if err := json.Unmarshal(raw, &doc); err != nil {
		return "", fmt.Errorf("parse paddleocr json: %w", err)
	}

	items := extractPaddleLineItems(doc.Pages)
	items = filterRepeatedContentLines(items, len(doc.Pages))
	lines := formatOpenDataLines(items)

	outputPath := paddleOCROutputPath(inputPath)
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

func paddleOCROutputPath(inputPath string) string {
	root := strings.TrimSuffix(inputPath, filepath.Ext(inputPath))
	if strings.HasSuffix(strings.ToLower(root), "_paddleocr") {
		return root + ".txt"
	}
	return root + "_paddleocr.txt"
}

func extractPaddleLineItems(pages []paddlePage) []extractedOpenDataLine {
	var items []extractedOpenDataLine
	for i, page := range pages {
		pageNo := i + 1
		if page.PageNumber != nil && *page.PageNumber > 0 {
			pageNo = *page.PageNumber
		}
		pageStr := strconv.Itoa(pageNo)
		for _, block := range page.blocks() {
			content := strings.TrimSpace(block.BlockContent)
			lineType, headingLevel, ok := paddleLineType(block.BlockLabel, content)
			if !ok {
				continue
			}
			if lineType == "equation" {
				content = capRepeatedLatexDots(content)
			}
			items = append(items, extractedOpenDataLine{
				Page:         pageStr,
				Type:         lineType,
				HeadingLevel: headingLevel,
				BBox:         mineruBBoxStr(block.BlockBBox),
				Content:      content,
			})
		}
	}
	return items
}

// paddleLineType maps a PaddleOCR layout label to a Line File type. It returns
// ok=false for page furniture and for blocks with no content to keep.
func paddleLineType(label, content string) (lineType, headingLevel string, ok bool) {
	switch strings.ToLower(strings.TrimSpace(label)) {
	case "doc_title":
		return "heading", "1", content != ""
	case "paragraph_title":
		return "heading", "2", content != ""
	case "table":
		return "table", "", content != ""
	case "table_title":
		return "table-caption", "", content != ""
	case "table_footnote":
		return "table-footnote", "", content != ""
	case "figure_title", "chart_title":
		return "image-caption", "", content != ""
	case "vision_footnote":
		return "image-footnote", "", content != ""
	case "image", "chart":
		// PaddleOCR does not save the cropped figure, so there is no image path
		// to record; an image block without text carries nothing to keep.
		return "image", "", content != ""
	case "formula":
		return "equation", "", content != ""
	case "header", "footer", "number", "formula_number", "seal":
		// Running headers/footers, page numbers, seals and equation numbers
		// are page furniture, not document content.
		return "", "", false
	default:
		// text, abstract, content, reference, reference_content, aside_text,
		// algorithm, footnote, and any label this mapping does not know.
		return "paragraph", "", content != ""
	}
}
