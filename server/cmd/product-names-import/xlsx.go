package main

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/xuri/excelize/v2"
)

// catalogRow is one row of the 医疗器械分类目录产品列表 sheet: one product
// category, whose 品名举例 (ExampleNamesRaw) cell packs several example
// product names into one 、/，-delimited string.
type catalogRow struct {
	ExcelRow        int
	SeqNo           int
	SubCatalog      string
	CategoryL1      string
	CategoryL2      string
	Description     string
	IntendedUse     string
	ExampleNamesRaw string
	RegulatoryClass string
}

// productNameRow is one exploded example name, still carrying its source
// row's category context.
type productNameRow struct {
	catalogRow
	ProductName   string
	ProductNameEN string
}

// readCatalogRows reads the first sheet of the medical-device classification
// catalog xlsx. Column order is fixed: 序号, 子目录, 一级产品类别,
// 二级产品类别, 产品描述, 预期用途, 品名举例, 管理类别.
func readCatalogRows(path string) ([]catalogRow, error) {
	f, err := excelize.OpenFile(path)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()

	sheet := f.GetSheetName(0)
	rows, err := f.GetRows(sheet)
	if err != nil {
		return nil, fmt.Errorf("read sheet %q: %w", sheet, err)
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("sheet %q is empty", sheet)
	}

	out := make([]catalogRow, 0, len(rows)-1)
	for i, row := range rows[1:] {
		excelRow := i + 2 // header is row 1
		cell := func(col int) string {
			if col < len(row) {
				return strings.TrimSpace(row[col])
			}
			return ""
		}
		seqNo, _ := strconv.Atoi(cell(0))
		r := catalogRow{
			ExcelRow:        excelRow,
			SeqNo:           seqNo,
			SubCatalog:      cell(1),
			CategoryL1:      cell(2),
			CategoryL2:      cell(3),
			Description:     cell(4),
			IntendedUse:     cell(5),
			ExampleNamesRaw: cell(6),
			RegulatoryClass: cell(7),
		}
		if r.ExampleNamesRaw == "" {
			continue
		}
		out = append(out, r)
	}
	return out, nil
}

// splitExampleNames splits a 品名举例 cell on the enumeration delimiters
// observed in the source file (、 and ，), trims whitespace, drops empty
// pieces (a trailing delimiter is common), and dedupes exact repeats within
// the same cell while preserving first-seen order.
func splitExampleNames(raw string) []string {
	parts := strings.FieldsFunc(raw, func(r rune) bool {
		return r == '、' || r == '，'
	})
	seen := make(map[string]bool, len(parts))
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		name := strings.TrimSpace(p)
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, name)
	}
	return out
}

// explodeRow turns one catalog row into one productNameRow per example name.
func explodeRow(r catalogRow) []productNameRow {
	names := splitExampleNames(r.ExampleNamesRaw)
	out := make([]productNameRow, 0, len(names))
	for _, name := range names {
		out = append(out, productNameRow{catalogRow: r, ProductName: name})
	}
	return out
}

// explodeAll applies explodeRow to every catalog row.
func explodeAll(rows []catalogRow) []productNameRow {
	out := make([]productNameRow, 0, len(rows)*4)
	for _, r := range rows {
		out = append(out, explodeRow(r)...)
	}
	return out
}
