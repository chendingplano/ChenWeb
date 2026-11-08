package docprocessing

import (
	"bytes"
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

// Codes of the compact merged response (prompt-extract-products-merged-v2.md),
// expanded to the long values the rest of extract_products stores.
var productV2Types = map[string]string{
	"sp": "specific_product", "pc": "product_class", "cmp": "component", "mat": "material", "sw": "software",
	"sys": "system", "eq": "equipment", "con": "consumable", "pkg": "packaging", "oth": "other",
}
var productV2Relations = map[string]string{
	"sc": "scope", "reg": "regulated_object", "req": "requirement_target", "perf": "performance_requirement",
	"des": "design_requirement", "mat": "material_requirement", "test": "testing_requirement",
	"cert": "certification_requirement", "use": "usage_condition", "inst": "installation_requirement",
	"maint": "maintenance_requirement", "stor": "storage_requirement", "ban": "prohibited_product",
	"ex": "exempted_product", "part": "component_of", "has": "contains_product", "compat": "compatible_with",
	"alt": "replacement_or_alternative", "meas": "measurement_object", "risk": "risk_source", "oth": "other",
}
var productV2Obligations = map[string]string{
	"m": "mandatory", "r": "recommended", "p": "permitted", "b": "prohibited", "c": "conditional",
	"d": "descriptive", "unk": "unknown",
}
var productV2RelatedRelationships = map[string]string{
	"part": "component_of", "has": "contains_product", "compat": "compatible_with",
	"alt": "replacement_or_alternative", "vs": "compared_with", "oth": "other",
}

// productV2Span matches a line (143), an inclusive range ("150-151"), or one row
// of a table line as the input labels it ("116#r3", "116#h0").
var productV2Span = regexp.MustCompile(`^([1-9][0-9]*)(?:-([1-9][0-9]*)|#([hr][0-9]+))?$`)

// productSourceLines maps each line number a compact merged call sent to the
// model to its content exactly as sent (llmLineContent: a table line is rendered
// as "<line>#<row>: …" rows). Image lines are never sent, so they are absent.
type productSourceLines map[int]string

func productSourceLinesFromChunk(lines []MarkedLine) productSourceLines {
	out := make(productSourceLines, len(lines))
	for _, l := range lines {
		if l.Line.LineType == "image" {
			continue
		}
		out[l.Line.LineNo] = llmLineContent(l.Line.LineType, l.Line.LineNo, l.Line.Content)
	}
	return out
}

// productMergedPromptUsesV2 reports whether the merged prompt asks for the
// compact v2 response. v3 keeps the v2 wire format and changes only the rules.
func productMergedPromptUsesV2(ref string) bool {
	switch filepath.Base(ref) {
	case "prompt-extract-products-merged-v2.md", "prompt-extract-products-merged-v3.md":
		return true
	}
	return false
}

// The same schema governs provider retries and decoding plain JSON clients.
func productMergedV2Schema() map[string]any {
	text := map[string]any{"type": "string", "minLength": 1}
	spans := map[string]any{
		"type": "array", "minItems": 1,
		"items": map[string]any{"anyOf": []any{
			map[string]any{"type": "integer", "minimum": 1},
			map[string]any{"type": "string", "pattern": productV2Span.String()},
		}},
	}
	params := schemaArrayOfObjects(map[string]any{
		"n": text, "v": schemaScalar("string", "number"), "u": schemaString(),
	}, []string{"n", "v"}, false)
	related := schemaArrayOfObjects(map[string]any{
		"n": text, "r": provisionV2Enum(productV2RelatedRelationships),
	}, []string{"n", "r"}, false)
	products := schemaArrayOfObjects(map[string]any{
		"n": text, "c": schemaString(), "t": provisionV2Enum(productV2Types), "r": provisionV2Enum(productV2Relations),
		"d": text, "s": spans, "o": provisionV2Enum(productV2Obligations), "x": schemaString(),
		"cnd": schemaStringArray(), "exc": schemaStringArray(), "p": params, "rp": related, "a": schemaString(),
		"q": map[string]any{"type": "number", "minimum": 0, "maximum": 1}, "cr": schemaString(),
	}, []string{"n", "t", "r", "d", "s", "o", "q"}, false)
	return map[string]any{
		"type": "object", "required": []string{"v", "p"}, "additionalProperties": false,
		"properties": map[string]any{
			"v": map[string]any{"type": "integer", "const": 2}, "p": products,
		},
	}
}

var compiledProductMergedV2Schema = sync.OnceValues(func() (*jsonschema.Schema, error) {
	compiler := jsonschema.NewCompiler()
	const uri = "mem://product-merged-v2.json"
	document, err := jsonschema.UnmarshalJSON(bytes.NewReader(mustMarshalSchema(productMergedV2Schema())))
	if err != nil {
		return nil, err
	}
	if err := compiler.AddResource(uri, document); err != nil {
		return nil, err
	}
	return compiler.Compile(uri)
})

// decodeProductMergedV2 validates a compact merged response against the lines
// the call sent and expands it into fresh legacy-shaped rows for
// normalizeProductList. evidence_quote is rebuilt from the cited lines.
func decodeProductMergedV2(payload map[string]any, lines productSourceLines) (map[string]any, error) {
	schema, err := compiledProductMergedV2Schema()
	if err != nil {
		return nil, err
	}
	if err := schema.Validate(payload); err != nil {
		return nil, fmt.Errorf("(MID_20261107_841) invalid product merged v2 response: %w", err)
	}
	items := payload["p"].([]any)
	products := make([]any, 0, len(items))
	for i, item := range items {
		raw := item.(map[string]any)
		name := strings.TrimSpace(asString(raw["n"]))
		summary := strings.TrimSpace(asString(raw["d"]))
		if name == "" || summary == "" {
			return nil, fmt.Errorf("(MID_20261107_842) product %d has a blank name or summary", i+1)
		}
		spans, quote, err := resolveProductV2Spans(raw["s"].([]any), lines)
		if err != nil {
			return nil, fmt.Errorf("(MID_20261107_843) product %d evidence_lines: %w", i+1, err)
		}
		spans, quote = groundProductV2Spans(name, spans, quote, lines)
		params := make([]any, 0)
		for _, param := range objectItemsFromValue(raw["p"]) {
			params = append(params, map[string]any{
				"name": strings.TrimSpace(asString(param["n"])), "value": productV2ScalarText(param["v"]),
				"unit": strings.TrimSpace(asString(param["u"])),
			})
		}
		related := make([]any, 0)
		for _, rel := range objectItemsFromValue(raw["rp"]) {
			related = append(related, map[string]any{
				"product_name": strings.TrimSpace(asString(rel["n"])), "relationship": productV2RelatedRelationships[asString(rel["r"])],
			})
		}
		products = append(products, map[string]any{
			"product_name": name, "canonical_name": firstNonEmptyTrimmed(asString(raw["c"]), name),
			"product_type": productV2Types[asString(raw["t"])], "relation_type": productV2Relations[asString(raw["r"])],
			"product_summary": summary, "evidence_quote": quote, "evidence_lines": spans,
			"relation_details": map[string]any{
				"obligation_level":         productV2Obligations[asString(raw["o"])],
				"requirement_text":         strings.TrimSpace(asString(raw["x"])),
				"conditions":               append([]string{}, toStringSlice(raw["cnd"])...),
				"exceptions":               append([]string{}, toStringSlice(raw["exc"])...),
				"thresholds_or_parameters": params,
				"related_products":         related,
				"responsible_actor":        strings.TrimSpace(asString(raw["a"])),
			},
			"confidence": toFloat(raw["q"]), "confidence_reason": strings.TrimSpace(asString(raw["cr"])),
		})
	}
	return map[string]any{"products": products}, nil
}

// productV2ScalarText keeps a numeric parameter value exact; asString would
// truncate 2.5 to "2".
func productV2ScalarText(v any) string {
	if f, ok := v.(float64); ok {
		return strconv.FormatFloat(f, 'f', -1, 64)
	}
	return strings.TrimSpace(asString(v))
}

// resolveProductV2Spans checks every span against the lines the call sent and
// returns the spans as strings plus the cited text: whole lines, or single
// table rows, in line order and without the "<line>#<row>: " labels.
func resolveProductV2Spans(items []any, lines productSourceLines) ([]string, string, error) {
	spans := make([]string, 0, len(items))
	whole := map[int]bool{}
	rows := map[int]map[string]bool{}
	for _, item := range items {
		span, ok := item.(string)
		if !ok {
			span = strconv.FormatFloat(toFloat(item), 'f', 0, 64)
		}
		m := productV2Span.FindStringSubmatch(span)
		if m == nil {
			return nil, "", fmt.Errorf("invalid line span %q", span)
		}
		start, _ := strconv.Atoi(m[1])
		if _, ok := lines[start]; !ok {
			return nil, "", fmt.Errorf("span %q cites line %d, which was not in the input", span, start)
		}
		switch {
		case m[2] != "":
			end, _ := strconv.Atoi(m[2])
			if end < start {
				return nil, "", fmt.Errorf("span %q is reversed", span)
			}
			if _, ok := lines[end]; !ok {
				return nil, "", fmt.Errorf("span %q cites line %d, which was not in the input", span, end)
			}
			// Both ends exist in this call's input, so the walk is bounded by it;
			// lines it skips (images) were never sent.
			for n := start; n <= end; n++ {
				if _, ok := lines[n]; ok {
					whole[n] = true
				}
			}
		case m[3] != "":
			found := false
			for _, row := range productV2LineRows(start, lines[start]) {
				found = found || row.id == m[3]
			}
			if !found {
				return nil, "", fmt.Errorf("span %q cites a row that line %d does not have", span, start)
			}
			if rows[start] == nil {
				rows[start] = map[string]bool{}
			}
			rows[start][m[3]] = true
		default:
			whole[start] = true
		}
		spans = append(spans, span)
	}

	cited := make([]int, 0, len(whole)+len(rows))
	for n := range whole {
		cited = append(cited, n)
	}
	for n := range rows {
		if !whole[n] {
			cited = append(cited, n)
		}
	}
	sort.Ints(cited)
	var parts []string
	for _, n := range cited {
		for _, row := range productV2LineRows(n, lines[n]) {
			if whole[n] || rows[n][row.id] {
				parts = append(parts, row.text)
			}
		}
	}
	return spans, strings.Join(parts, "\n"), nil
}

// groundProductV2Spans adds the line just before or after a cited line when
// none of the cited text contains the product's name but that neighbour does.
// With reasoning off, deepseek-flash sometimes cites a whole chunk's rows one
// line off (record 416: every test-method row cited the heading above its
// step). The cited spans are kept, so a row that rightly cites a line without
// the name, such as a definition under its term, only gains the term's line.
// Names the source never writes out whole (expanded conjuncts) match no line
// and are left alone; table-row spans are not widened.
func groundProductV2Spans(name string, spans []string, quote string, lines productSourceLines) ([]string, string) {
	key := productV2NameKey(name)
	if key == "" || strings.Contains(productV2NameKey(quote), key) {
		return spans, quote
	}
	cited := map[string]bool{}
	for _, span := range spans {
		cited[span] = true
	}
	widened := stringsToAny(spans)
	added := false
	for _, span := range spans {
		m := productV2Span.FindStringSubmatch(span)
		if m == nil || m[3] != "" {
			continue
		}
		start, _ := strconv.Atoi(m[1])
		end := start
		if m[2] != "" {
			end, _ = strconv.Atoi(m[2])
		}
		for _, n := range []int{start - 1, end + 1} {
			text, ok := lines[n]
			if ok && !cited[strconv.Itoa(n)] && strings.Contains(productV2NameKey(text), key) {
				cited[strconv.Itoa(n)] = true
				widened = append(widened, strconv.Itoa(n))
				added = true
			}
		}
	}
	if !added {
		return spans, quote
	}
	if out, text, err := resolveProductV2Spans(widened, lines); err == nil {
		return out, text
	}
	return spans, quote
}

// productV2NameKey compares names and cited text without case or whitespace.
func productV2NameKey(s string) string {
	return strings.ToLower(strings.Join(strings.Fields(s), ""))
}

func stringsToAny(values []string) []any {
	out := make([]any, len(values))
	for i, v := range values {
		out[i] = v
	}
	return out
}

type productV2LineRow struct {
	id   string // "r3"/"h0" for a rendered table row, "" otherwise
	text string // content without the "<line>#<row>: " label
}

// productV2LineRows splits a line's sent content into its rendered table rows
// (see TableGrid.RenderNumbered); any other content is one unlabeled row.
func productV2LineRows(lineNo int, content string) []productV2LineRow {
	prefix := strconv.Itoa(lineNo) + "#"
	pieces := strings.Split(content, "\n")
	out := make([]productV2LineRow, 0, len(pieces))
	for _, piece := range pieces {
		if label, text, ok := strings.Cut(strings.TrimPrefix(piece, prefix), ": "); ok && strings.HasPrefix(piece, prefix) && productV2RowID(label) {
			out = append(out, productV2LineRow{id: label, text: text})
			continue
		}
		out = append(out, productV2LineRow{text: piece})
	}
	return out
}

func productV2RowID(label string) bool {
	if len(label) < 2 || (label[0] != 'h' && label[0] != 'r') {
		return false
	}
	_, err := strconv.Atoi(label[1:])
	return err == nil
}
