package docprocessing

import (
	"bytes"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

var provisionV2Types = map[string]string{"m": "mandatory", "r": "recommended", "o": "optional"}
var provisionV2Locations = map[string]string{
	"s": "sentence", "p": "paragraph", "b": "bullet", "tr": "table_row",
	"tc": "table_cell", "hc": "heading_context", "mx": "mixed", "li": "list-item",
}
var provisionV2ObjectTypes = map[string]string{
	"eq": "equipment", "mat": "material", "sys": "system", "proc": "process", "org": "organization",
	"per": "person", "loc": "place", "doc": "document", "con": "concept", "oth": "other",
}
var provisionV2ObjectRoles = map[string]string{
	"reg": "regulated_object", "tgt": "requirement_target", "part": "component", "par": "parent_system", "oth": "other",
}

// The same schema governs provider retries and decoding plain JSON clients.
func provisionV2Schema() map[string]any {
	text := map[string]any{"type": "string", "minLength": 1}
	score := map[string]any{"type": "number", "minimum": 0, "maximum": 1}
	spans := map[string]any{
		"type": "array", "minItems": 1,
		"items": map[string]any{"anyOf": []any{
			map[string]any{"type": "integer", "minimum": 1},
			map[string]any{"type": "string", "pattern": `^[1-9][0-9]*(-[1-9][0-9]*)?$`},
		}},
	}
	objects := schemaArrayOfObjects(map[string]any{
		"n": text, "t": map[string]any{"type": "string", "pattern": `\S`}, "r": provisionV2Enum(provisionV2ObjectRoles),
		"s": spans, "q": score, "a": schemaStringArray(), "y": schemaStringArray(), "d": schemaString(),
	}, []string{"n", "t", "r", "s", "q"}, false)
	provisions := schemaArrayOfObjects(map[string]any{
		"n": text, "t": provisionV2Enum(provisionV2Types), "x": text, "s": spans,
		"lt": provisionV2Enum(provisionV2Locations), "q": score,
		"f": map[string]any{"type": "integer", "minimum": 0, "maximum": 3},
		"d": schemaString(), "c": schemaString(), "u": schemaString(), "k": schemaStringArray(), "o": objects, "g": schemaArray(),
	}, []string{"n", "t", "x", "s", "lt", "q", "f"}, false)
	return map[string]any{
		"type": "object", "required": []string{"v", "l", "p"}, "additionalProperties": false,
		"properties": map[string]any{
			"v": map[string]any{"type": "integer", "const": 2}, "l": text, "p": provisions,
		},
	}
}

func provisionV2Enum(values map[string]string) map[string]any {
	codes := make([]string, 0, len(values))
	for code := range values {
		codes = append(codes, code)
	}
	sort.Strings(codes)
	return map[string]any{"type": "string", "enum": codes}
}

var compiledProvisionV2Schema = sync.OnceValues(func() (*jsonschema.Schema, error) {
	compiler := jsonschema.NewCompiler()
	const uri = "mem://provision-v2.json"
	document, err := jsonschema.UnmarshalJSON(bytes.NewReader(mustMarshalSchema(provisionV2Schema())))
	if err != nil {
		return nil, err
	}
	if err := compiler.AddResource(uri, document); err != nil {
		return nil, err
	}
	return compiler.Compile(uri)
})

func decodeProvisionV2(payload map[string]any) (map[string]any, error) {
	schema, err := compiledProvisionV2Schema()
	if err != nil {
		return nil, err
	}
	if err := schema.Validate(payload); err != nil {
		return nil, fmt.Errorf("(MID_20261107_762) invalid provision v2 response: %w", err)
	}
	language := strings.ToLower(strings.TrimSpace(asString(payload["l"])))
	isEnglish := language == "en" || strings.HasPrefix(language, "en-") || language == "english"
	isChinese := language == "zh" || strings.HasPrefix(language, "zh-") || language == "chinese"
	items := payload["p"].([]any)
	provisions := make([]any, 0, len(items))
	for i, item := range items {
		raw := item.(map[string]any)
		spans, err := decodeProvisionV2Spans(raw["s"])
		if err != nil {
			return nil, fmt.Errorf("provision %d source_line_spans: %w", i+1, err)
		}
		flags := int(toFloat(raw["f"]))
		provision := map[string]any{
			"name": raw["n"], "type": provisionV2Types[asString(raw["t"])], "provision": raw["x"],
			"source_line_spans": spans, "location_type": provisionV2Locations[asString(raw["lt"])], "confidence": raw["q"],
			"is_explicit": flags&1 != 0, "need_verify": flags&2 != 0,
			"provision_desc": asString(raw["d"]), "context": asString(raw["c"]), "subject": asString(raw["u"]),
			"keywords": append([]string{}, toStringSlice(raw["k"])...),
		}
		if categories, present := raw["g"]; present {
			provision["category_paths"] = categories
		}
		// Native English values fill existing English storage fields without an
		// LLM translation or duplicate wire fields. Other languages stay native.
		if isEnglish {
			for _, pair := range [][2]string{{"name", "name_en"}, {"provision", "provision_en"}, {"provision_desc", "provision_desc_en"}, {"context", "context_en"}, {"subject", "subject_en"}, {"keywords", "keywords_en"}, {"category_paths", "category_path_en"}} {
				if value, present := provision[pair[0]]; present {
					provision[pair[1]] = value
				}
			}
		}
		objects := make([]any, 0)
		for _, object := range objectItemsFromValue(raw["o"]) {
			objectSpans, err := decodeProvisionV2Spans(object["s"])
			if err != nil {
				return nil, fmt.Errorf("provision %d object source_line_spans: %w", i+1, err)
			}
			rawType := asString(object["t"])
			objectType := normalizeObjectToken(rawType)
			// These codes are backwards-compatible wire aliases only. The
			// database registry owns acceptance and approval of type names.
			if canonical, ok := provisionV2ObjectTypes[objectType]; ok {
				objectType = canonical
			}
			decoded := map[string]any{
				"object_name": object["n"], "object_type": objectType, "object_type_raw": rawType,
				"object_role": provisionV2ObjectRoles[asString(object["r"])], "source_line_spans": objectSpans, "confidence": object["q"],
				"description": asString(object["d"]), "aliases": append([]string{}, toStringSlice(object["a"])...),
				"acronyms": append([]string{}, toStringSlice(object["y"])...),
			}
			if isEnglish {
				decoded["object_name_en"] = object["n"]
			} else if isChinese {
				decoded["object_name_zh"] = object["n"]
			}
			objects = append(objects, decoded)
		}
		provision["objects"] = objects
		provisions = append(provisions, provision)
	}
	return map[string]any{"language": language, "provisions": provisions}, nil
}

func decodeProvisionV2Spans(value any) ([]string, error) {
	spans := make([]string, 0)
	for _, item := range value.([]any) {
		var span string
		if text, ok := item.(string); ok {
			span = text
		} else {
			span = strconv.FormatFloat(toFloat(item), 'f', 0, 64)
		}
		if _, _, ok := parseMetricLineSpan(span); !ok {
			return nil, fmt.Errorf("invalid line span %q", span)
		}
		spans = append(spans, span)
	}
	return spans, nil
}
