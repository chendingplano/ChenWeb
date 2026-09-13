package main

import (
	"reflect"
	"testing"
)

func TestSplitExampleNames(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want []string
	}{
		{
			name: "dun hao delimited with trailing delimiter",
			raw:  "软组织超声手术仪、外科超声手术系统、超声手术系统、",
			want: []string{"软组织超声手术仪", "外科超声手术系统", "超声手术系统"},
		},
		{
			name: "single name no delimiter",
			raw:  "人工晶状体",
			want: []string{"人工晶状体"},
		},
		{
			name: "fullwidth comma delimiter",
			raw:  "甲，乙、丙",
			want: []string{"甲", "乙", "丙"},
		},
		{
			name: "duplicate names deduped preserving order",
			raw:  "甲、乙、甲",
			want: []string{"甲", "乙"},
		},
		{
			name: "parenthetical qualifier kept intact, not split",
			raw:  "一次性使用无菌注射器（带针）、无菌注射器",
			want: []string{"一次性使用无菌注射器（带针）", "无菌注射器"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := splitExampleNames(tc.raw)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("splitExampleNames(%q) = %#v, want %#v", tc.raw, got, tc.want)
			}
		})
	}
}

func TestExplodeRow(t *testing.T) {
	r := catalogRow{
		ExcelRow:        2,
		SeqNo:           1,
		SubCatalog:      "01 有源手术器械",
		CategoryL1:      "01 超声手术设备及附件",
		CategoryL2:      "01.1 超声手术设备",
		Description:     "desc",
		IntendedUse:     "use",
		ExampleNamesRaw: "甲、乙",
		RegulatoryClass: "Ⅲ",
	}
	got := explodeRow(r)
	if len(got) != 2 {
		t.Fatalf("expected 2 exploded rows, got %d", len(got))
	}
	if got[0].ProductName != "甲" || got[1].ProductName != "乙" {
		t.Errorf("unexpected product names: %+v", got)
	}
	if got[0].CategoryL2 != r.CategoryL2 || got[0].RegulatoryClass != r.RegulatoryClass {
		t.Errorf("category context not carried onto exploded row: %+v", got[0])
	}
}

func TestExplodeRowEmptyCellProducesNoRows(t *testing.T) {
	got := explodeRow(catalogRow{ExampleNamesRaw: ""})
	if len(got) != 0 {
		t.Errorf("expected 0 rows for empty cell, got %d", len(got))
	}
}

func TestBuildKeywordsWithoutTranslation(t *testing.T) {
	kw := buildKeywords("Luminance", "")
	if kw.Zh.Norm != "luminance" {
		t.Errorf("Zh.Norm = %q, want %q", kw.Zh.Norm, "luminance")
	}
	if kw.En != nil {
		t.Errorf("En should be nil when no translation is given, got %+v", kw.En)
	}
}

func TestBuildKeywordsWithTranslation(t *testing.T) {
	kw := buildKeywords("亮度", "Luminance")
	if kw.En == nil {
		t.Fatal("En should be populated when a translation is given")
	}
	if kw.En.Norm != "luminance" {
		t.Errorf("En.Norm = %q, want %q", kw.En.Norm, "luminance")
	}
}
