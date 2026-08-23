package assertions

import (
	"strings"

	appconfig "github.com/chendingplano/deepdoc/server/cmd/config"
)

// BuildConfiguredProperties applies configured field->property mappings
// against an artifact's field map (dotted paths resolve into nested maps,
// e.g. "ext_info.object_name"), returning the properties to add to the
// target properties/qualifiers bag -- shared by both
// [ontology_term_property_map] (kb.ontology_terms.properties) and
// [semantic_assertion_property_map] (kb.semantic_assertions.qualifiers)
// (openspec change governed-property-normalization, collapsing the former
// BuildSignatureProperties/BuildMappedProperties/ParsePropertyMap into one
// function now that both config sections share the same entry shape).
//
// Absent or empty-valued fields are omitted rather than written as null/"".
// A non-normalized entry (Normalize == "") is written as a plain value,
// unchanged. A normalized entry is written as {"raw": <value>, "resolved":
// <resolved value>} -- resolved is looked up from the caller-supplied
// resolved map by field name, never computed here. This function is
// mechanism-agnostic: it never branches on which of "system"/"simple"/
// "moderate"/"strong" produced a resolved value, only on whether Normalize
// is non-empty. Lives here rather than in doc-processing (the original,
// single-purpose home) because assertions cannot import doc-processing --
// doc-processing already imports assertions -- so a helper shared by a
// doc-processing call site (extract_metrics) and an assertions call site
// (metric_normalizer.go) must live on the assertions side of that boundary.
func BuildConfiguredProperties(entries []appconfig.OntologyTermPropertyMapEntry, fieldMap map[string]any, resolved map[string]string) map[string]any {
	if len(entries) == 0 || len(fieldMap) == 0 {
		return nil
	}
	out := map[string]any{}
	for _, entry := range entries {
		val := lookupFieldPath(fieldMap, entry.Field)
		if isEmptyFieldValue(val) {
			continue
		}
		if entry.Normalize == "" {
			out[entry.Property] = val
			continue
		}
		out[entry.Property] = map[string]any{"raw": val, "resolved": resolved[entry.Field]}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// lookupFieldPath resolves a dot-separated path (e.g. "ext_info.object_name")
// against nested map[string]any values. A missing key or a non-map
// intermediate value yields nil.
func lookupFieldPath(m map[string]any, path string) any {
	var cur any = m
	for _, part := range strings.Split(path, ".") {
		asMap, ok := cur.(map[string]any)
		if !ok {
			return nil
		}
		cur = asMap[part]
	}
	return cur
}

func isEmptyFieldValue(v any) bool {
	switch val := v.(type) {
	case nil:
		return true
	case string:
		return strings.TrimSpace(val) == ""
	case []string:
		return len(val) == 0
	case []any:
		return len(val) == 0
	default:
		return false
	}
}
