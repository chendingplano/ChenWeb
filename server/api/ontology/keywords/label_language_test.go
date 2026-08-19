package keywords

import "testing"

func TestAutoPromotedLabelLanguage(t *testing.T) {
	tests := []struct {
		name  string
		label string
		want  string
	}{
		{name: "Han label", label: "每户配备分类垃圾容器的数量", want: "zh"},
		{name: "Latin label", label: "Display luminance", want: "en"},
		{name: "Han wins mixed label", label: "显示 luminance", want: "zh"},
		{name: "blank label", label: "", want: "und"},
		{name: "numeric label", label: "250", want: "und"},
		{name: "punctuation label", label: "---", want: "und"},
		{name: "Arabic script label", label: "السطوع", want: "und"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := autoPromotedLabelLanguage(tt.label); got != tt.want {
				t.Errorf("autoPromotedLabelLanguage(%q) = %q, want %q", tt.label, got, tt.want)
			}
		})
	}
}
