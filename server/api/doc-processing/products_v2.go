package docprocessing

import (
	"bytes"
	"errors"
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
// A numeric comparison can be misplaced as an empty-valued parameter key (ChenWeb#62).
var productV2ComparisonKey = regexp.MustCompile(`^[<>≤≥]=?-?[0-9]+(?:\.[0-9]+)?$`)

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
	lineNumber := map[string]any{"type": "integer", "minimum": 1}
	spanList := map[string]any{
		"type": "array", "minItems": 1,
		"items": map[string]any{"anyOf": []any{
			lineNumber,
			map[string]any{"type": "string", "pattern": productV2Span.String()},
		}},
	}
	// A single line number carries the same evidence as a one-item array (ChenWeb#58).
	spans := map[string]any{"anyOf": []any{spanList, lineNumber}}
	params := schemaArrayOfObjects(map[string]any{
		"n": text, "v": schemaScalar("string", "number"), "u": schemaString(),
	}, []string{"n"}, false)
	params["items"].(map[string]any)["patternProperties"] = map[string]any{
		productV2ComparisonKey.String(): map[string]any{"const": ""},
	}
	related := schemaArrayOfObjects(map[string]any{
		"n": text, "r": provisionV2Enum(productV2RelatedRelationships),
	}, []string{"n", "r"}, false)
	products := schemaArrayOfObjects(map[string]any{
		"n": text, "c": schemaString(), "t": text, "r": schemaString(),
		"d": text, "s": spans, "o": provisionV2Enum(productV2Obligations), "x": schemaString(),
		"cnd": schemaStringArray(), "exc": schemaStringArray(), "p": params, "rp": related, "a": schemaString(),
		"q": map[string]any{"type": "number", "minimum": 0, "maximum": 1}, "cr": schemaString(),
	}, []string{"n", "t", "d", "s", "o", "q"}, false)
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
	// A relation code can land in t. Only recover a valid type when another
	// row for the same product supplies it consistently (ChenWeb#48).
	typesByName := make(map[string]string)
	for _, item := range items {
		row := item.(map[string]any)
		code := asString(row["t"])
		if productV2Types[code] == "" {
			continue
		}
		name := strings.TrimSpace(asString(row["n"]))
		if previous, exists := typesByName[name]; exists && previous != code {
			code = ""
		}
		typesByName[name] = code
	}
	// CheckParsed sends these errors to the primary model for its next attempt.
	// Report all invalid spans at once so fixing them doesn't use one retry each (ChenWeb#28).
	var validationErrs []error
	for i, item := range items {
		raw := item.(map[string]any)
		name := strings.TrimSpace(asString(raw["n"]))
		summary := strings.TrimSpace(asString(raw["d"]))
		if name == "" || summary == "" {
			return nil, fmt.Errorf("(MID_20261107_842) product %d has a blank name or summary", i+1)
		}
		// The relation code is validated against kb.product_relation_types after
		// decoding (ProductsProcessor.resolveProductRelations); an unlisted code is
		// proposed, not rejected. Only a blank code is a contract failure.
		relationCode := strings.ToLower(strings.TrimSpace(asString(raw["r"])))
		// Recover a relation misplaced in t before checking the omitted r (ChenWeb#48).
		if _, supplied := raw["r"]; !supplied && productV2Types[asString(raw["t"])] == "" && productV2Relations[asString(raw["t"])] != "" {
			relationCode = asString(raw["t"])
		}
		if relationCode == "" {
			return nil, fmt.Errorf("(MID_20261118_003) product %d has a blank relation code", i+1)
		}
		if (relationCode == "p" || relationCode == "r") && relationCode == asString(raw["o"]) {
			codes := provisionV2Enum(productV2Relations)["enum"].([]string)
			validationErrs = append(validationErrs, fmt.Errorf("product %d %q: r=%q repeats the obligation code from o. Keep the obligation in o; r describes how the document concerns the product. Choose the supported relation from %s (ChenWeb#65)", i+1, name, relationCode, strings.Join(codes, ", ")))
			continue
		}
		typeCode := asString(raw["t"])
		if productV2Types[typeCode] == "" {
			if typeCode == relationCode && productV2Relations[typeCode] != "" {
				typeCode = typesByName[name]
			}
			if productV2Types[typeCode] == "" {
				codes := provisionV2Enum(productV2Types)["enum"].([]string)
				validationErrs = append(validationErrs, fmt.Errorf("product %d %q: t describes what the product is; r describes its relationship to the requirement. Replace t=%q with one of %s (ChenWeb#48)",
					i+1, name, asString(raw["t"]), strings.Join(codes, ", ")))
				continue
			}
		}
		// Packaging requirements use the existing handling relation (ChenWeb#55).
		if relationCode == "pkg" {
			relationCode = "stor"
		}
		relationType := productV2Relations[relationCode]
		if relationType == "" {
			relationType = relationCode
		}
		evidence, ok := raw["s"].([]any)
		if !ok {
			evidence = []any{raw["s"]}
		}
		spans, quote, err := resolveProductV2Spans(evidence, lines)
		if err != nil {
			validationErrs = append(validationErrs, fmt.Errorf("(MID_20261107_843) product %d evidence_lines: %w", i+1, err))
			continue
		}
		spans, quote = groundProductV2Spans(name, spans, quote, lines)
		params := make([]any, 0)
		for _, param := range objectItemsFromValue(raw["p"]) {
			value, hasValue := param["v"]
			for key := range param {
				if productV2ComparisonKey.MatchString(key) {
					if hasValue {
						return nil, fmt.Errorf("parameter %q has conflicting values; return one value in v (ChenWeb#62)", asString(param["n"]))
					}
					value, hasValue = key, true
				}
			}
			if !hasValue {
				continue
			}
			params = append(params, map[string]any{
				"name": strings.TrimSpace(asString(param["n"])), "value": productV2ScalarText(value),
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
			"product_type": productV2Types[typeCode], "relation_type": relationType, "relation_code": relationCode,
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
	if len(validationErrs) > 0 {
		return nil, errors.Join(validationErrs...)
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
			var labels []string
			for _, row := range productV2LineRows(start, lines[start]) {
				if row.id == m[3] {
					found = true
					break
				}
				if row.id != "" {
					labels = append(labels, m[1]+"#"+row.id)
				}
			}
			if !found && len(labels) == 0 {
				return nil, "", fmt.Errorf("span %q cites a row that line %d does not have; line %d has no table rows, cite it as %d", span, start, start, start)
			}
			if !found {
				return nil, "", fmt.Errorf("span %q cites a row that line %d does not have; use the exact label of the row supporting the claim. Labels in the input: %s (ChenWeb#51)", span, start, strings.Join(labels, ", "))
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
