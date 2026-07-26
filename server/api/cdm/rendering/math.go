package rendering

import (
	"fmt"
	"strings"

	"github.com/chendingplano/deepdoc/server/api/cdm/model"
)

// mathOpSymbols maps normalized-AST binary/n-ary operators to their Typst
// infix spelling (spec §6.4 operator vocabulary).
var mathOpSymbols = map[string]string{
	"add":      "+",
	"subtract": "-",
	"multiply": " ",
	"divide":   "/",
	"equal":    "=",
}

// renderMath implements the spec §6.4 fallback rule: prefer the equation's
// normalized AST, and fall back to converting its original source otherwise.
func renderMath(eq *model.Equation) (string, error) {
	if eq.Normalized != nil {
		return renderTypstMath(eq.Normalized)
	}

	if eq.Original == nil {
		return "", fmt.Errorf("equation has neither normalized nor original")
	}

	switch eq.Original.Format {
	case "typst":
		return eq.Original.Source, nil
	case "latex":
		return convertLatexToTypst(eq.Original.Source)
	default:
		return "", fmt.Errorf("unsupported math source format %q", eq.Original.Format)
	}
}

// renderTypstMath walks a normalized math AST (spec §6.4) and emits Typst
// math syntax.
func renderTypstMath(expr *model.MathExpr) (string, error) {
	if expr == nil {
		return "", fmt.Errorf("nil math expression")
	}

	switch expr.Type {
	case "symbol":
		return expr.Name, nil
	case "number":
		return expr.Value, nil
	}

	switch expr.Op {
	case "add", "subtract", "equal":
		parts, err := renderMathArgs(expr.Args)
		if err != nil {
			return "", err
		}
		return strings.Join(parts, " "+mathOpSymbols[expr.Op]+" "), nil

	case "multiply":
		parts, err := renderMathArgs(expr.Args)
		if err != nil {
			return "", err
		}
		return strings.Join(parts, " "), nil

	case "divide":
		if len(expr.Args) != 2 {
			return "", fmt.Errorf("divide expects 2 args, got %d", len(expr.Args))
		}
		num, err := renderMathOperand(expr.Args[0])
		if err != nil {
			return "", err
		}
		den, err := renderMathOperand(expr.Args[1])
		if err != nil {
			return "", err
		}
		return num + "/" + den, nil

	case "length":
		if len(expr.Args) != 1 {
			return "", fmt.Errorf("length expects 1 arg, got %d", len(expr.Args))
		}
		inner, err := renderTypstMath(&expr.Args[0])
		if err != nil {
			return "", err
		}
		return "abs(" + inner + ")", nil

	case "abs":
		if len(expr.Args) != 1 {
			return "", fmt.Errorf("abs expects 1 arg, got %d", len(expr.Args))
		}
		inner, err := renderTypstMath(&expr.Args[0])
		if err != nil {
			return "", err
		}
		return "abs(" + inner + ")", nil

	case "power":
		if len(expr.Args) != 2 {
			return "", fmt.Errorf("power expects 2 args, got %d", len(expr.Args))
		}
		base, err := renderMathOperand(expr.Args[0])
		if err != nil {
			return "", err
		}
		exp, err := renderMathOperand(expr.Args[1])
		if err != nil {
			return "", err
		}
		return base + "^" + exp, nil

	case "root":
		if len(expr.Args) != 1 {
			return "", fmt.Errorf("root expects 1 arg, got %d", len(expr.Args))
		}
		inner, err := renderTypstMath(&expr.Args[0])
		if err != nil {
			return "", err
		}
		return "sqrt(" + inner + ")", nil

	case "sum", "product", "min", "max", "function":
		parts, err := renderMathArgs(expr.Args)
		if err != nil {
			return "", err
		}
		return expr.Op + "(" + strings.Join(parts, ", ") + ")", nil
	}

	return "", fmt.Errorf("unsupported math operator %q", expr.Op)
}

func renderMathArgs(args []model.MathExpr) ([]string, error) {
	parts := make([]string, len(args))
	for i := range args {
		s, err := renderTypstMath(&args[i])
		if err != nil {
			return nil, err
		}
		parts[i] = s
	}
	return parts, nil
}

// renderMathOperand wraps a rendered operand in parentheses when it is a
// compound expression, so precedence is preserved once embedded in a divide
// or power expression.
func renderMathOperand(expr model.MathExpr) (string, error) {
	s, err := renderTypstMath(&expr)
	if err != nil {
		return "", err
	}
	if expr.Op == "add" || expr.Op == "subtract" || expr.Op == "equal" {
		return "(" + s + ")", nil
	}
	return s, nil
}

// convertLatexToTypst is the best-effort fallback used when no normalized
// AST is available (spec §6.4). Phase 1 equations always take this path
// (design D6: parse_status "skipped"). This is intentionally a thin,
// syntactic pass-through for the common LaTeX constructs used in
// technical-document formulas; it is not a full LaTeX parser.
func convertLatexToTypst(src string) (string, error) {
	s := convertLatexFracs(src)

	replacer := strings.NewReplacer(
		`\left(`, "(",
		`\right)`, ")",
		`\left[`, "[",
		`\right]`, "]",
		`\left|`, "|",
		`\right|`, "|",
		`\cdot`, "*",
		`\times`, "*",
		`\,`, " ",
	)
	s = replacer.Replace(s)
	return strings.TrimSpace(s), nil
}

// convertLatexFracs rewrites every \frac{num}{den} to (num)/(den), including
// nested fracs in either group. A regex cannot balance the braces reliably,
// so this scans manually for \frac followed by two brace groups.
func convertLatexFracs(s string) string {
	const marker = `\frac`

	var out strings.Builder
	i := 0
	for {
		j := strings.Index(s[i:], marker)
		if j == -1 {
			out.WriteString(s[i:])
			break
		}
		j += i
		out.WriteString(s[i:j])

		num, afterNum, ok := readBraceGroup(s, j+len(marker))
		if !ok {
			// Not a well-formed \frac{...}: emit the literal marker and move on.
			out.WriteString(marker)
			i = j + len(marker)
			continue
		}
		den, afterDen, ok := readBraceGroup(s, afterNum)
		if !ok {
			out.WriteString(marker)
			i = j + len(marker)
			continue
		}

		out.WriteString("(" + convertLatexFracs(num) + ")/(" + convertLatexFracs(den) + ")")
		i = afterDen
	}
	return out.String()
}

// readBraceGroup reads a brace-delimited group "{...}" starting at s[pos],
// respecting nested braces, and returns its inner content and the index
// just past the closing brace.
func readBraceGroup(s string, pos int) (content string, next int, ok bool) {
	for pos < len(s) && s[pos] == ' ' {
		pos++
	}
	if pos >= len(s) || s[pos] != '{' {
		return "", pos, false
	}
	depth := 0
	start := pos + 1
	for k := pos; k < len(s); k++ {
		switch s[k] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return s[start:k], k + 1, true
			}
		}
	}
	return "", pos, false
}
