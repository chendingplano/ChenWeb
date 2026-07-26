package rendering

import (
	"testing"

	"github.com/chendingplano/deepdoc/server/api/cdm/model"
)

func TestConvertLatexToTypst_SimpleFrac(t *testing.T) {
	got, err := convertLatexToTypst(`\frac{1}{3}`)
	if err != nil {
		t.Fatalf("convert: %v", err)
	}
	want := "(1)/(3)"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestConvertLatexToTypst_NestedFrac(t *testing.T) {
	got, err := convertLatexToTypst(`\frac{1}{3}\left(\frac{m}{s_1} + \frac{m}{s_2}\right)`)
	if err != nil {
		t.Fatalf("convert: %v", err)
	}
	want := "(1)/(3)((m)/(s_1) + (m)/(s_2))"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestRenderMath_NormalizedPreferredOverOriginal(t *testing.T) {
	eq := &model.Equation{
		ParseStatus: "success",
		Original:    &model.MathSource{Format: "latex", Source: `\frac{1}{2}`},
		Normalized: &model.MathExpr{
			Op: "equal",
			Args: []model.MathExpr{
				{Type: "symbol", Name: "x"},
				{Type: "number", Value: "1"},
			},
		},
	}
	got, err := renderMath(eq)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if got != "x = 1" {
		t.Fatalf("got %q, want %q", got, "x = 1")
	}
}

func TestRenderMath_FallsBackToTypstOriginal(t *testing.T) {
	eq := &model.Equation{
		ParseStatus: "skipped",
		Original:    &model.MathSource{Format: "typst", Source: "a + b = c"},
	}
	got, err := renderMath(eq)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if got != "a + b = c" {
		t.Fatalf("got %q, want %q", got, "a + b = c")
	}
}

func TestRenderMath_FallsBackAndConvertsLatex(t *testing.T) {
	eq := &model.Equation{
		ParseStatus: "skipped",
		Original:    &model.MathSource{Format: "latex", Source: `\frac{m}{n}`},
	}
	got, err := renderMath(eq)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if got != "(m)/(n)" {
		t.Fatalf("got %q, want %q", got, "(m)/(n)")
	}
}

func TestRenderMath_UnsupportedFormatErrors(t *testing.T) {
	eq := &model.Equation{
		ParseStatus: "skipped",
		Original:    &model.MathSource{Format: "mathml", Source: "<math/>"},
	}
	if _, err := renderMath(eq); err == nil {
		t.Fatal("expected error for unsupported format")
	}
}

func TestRenderMath_LengthUsesAbs(t *testing.T) {
	expr := &model.MathExpr{
		Op:   "length",
		Args: []model.MathExpr{{Type: "symbol", Name: "s"}},
	}
	got, err := renderTypstMath(expr)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if got != "abs(s)" {
		t.Fatalf("got %q, want %q", got, "abs(s)")
	}
}
