package assertions

import (
	"context"
	"fmt"
	"strings"

	appconfig "github.com/chendingplano/deepdoc/server/cmd/config"
)

// PropertyMapping is one parsed entry from an operator-configured
// "artifact_type:table_field_name:property_name" map -- the shared shape
// behind both [ontology_term_property_map] (kb.ontology_terms.properties,
// class-level facts) and [semantic_assertion_property_map]
// (kb.semantic_assertions.qualifiers, instance-level facts; bug 2026082101
// finding 6/8). Lives here rather than in doc-processing (the original,
// single-purpose home) because assertions cannot import doc-processing --
// doc-processing already imports assertions -- so a helper shared by a
// doc-processing call site (extract_metrics) and an assertions call site
// (metric_normalizer.go) must live on the assertions side of that boundary.
type PropertyMapping struct {
	Field    string // dotted path into the artifact's field map, e.g. "ext_info.object_name"
	Property string // key written into the target JSONB bag
}

// ParsePropertyMap groups the configured "artifact_type:field:property"
// strings by artifact_type. Malformed entries (not exactly 3 ':'-separated
// parts, or an empty artifact_type/field/property) are skipped.
func ParsePropertyMap(entries []string) map[string][]PropertyMapping {
	byArtifact := map[string][]PropertyMapping{}
	for _, entry := range entries {
		parts := strings.SplitN(entry, ":", 3)
		if len(parts) != 3 {
			continue
		}
		artifactType := strings.TrimSpace(parts[0])
		field := strings.TrimSpace(parts[1])
		property := strings.TrimSpace(parts[2])
		if artifactType == "" || field == "" || property == "" {
			continue
		}
		byArtifact[artifactType] = append(byArtifact[artifactType], PropertyMapping{Field: field, Property: property})
	}
	return byArtifact
}

// BuildMappedProperties applies mappings against an artifact's field map
// (dotted paths resolve into nested maps, e.g. "ext_info.object_name"),
// returning the properties to add to the target JSONB bag. Absent or
// empty-valued fields are omitted rather than written as null/"".
func BuildMappedProperties(mappings []PropertyMapping, m map[string]any) map[string]any {
	if len(mappings) == 0 || len(m) == 0 {
		return nil
	}
	out := map[string]any{}
	for _, mapping := range mappings {
		val := lookupFieldPath(m, mapping.Field)
		if isEmptyFieldValue(val) {
			continue
		}
		out[mapping.Property] = val
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// BuildSignatureProperties is BuildMappedProperties' class-identity-aware
// sibling (ADR 2026082203 DR2/DR4), used only for [ontology_term_property_map]
// entries -- [semantic_assertion_property_map] keeps using BuildMappedProperties
// unchanged, since instance-level fields never participate in class-identity
// signature matching. For each configured entry: an empty field value is
// skipped, matching BuildMappedProperties. A non-identity entry is written as
// a plain value, unchanged from today. An identity entry resolves its raw
// value against resolver.Lookup for the configured dimension and is written
// as {"raw": <value>, "term_id": <term_id-or-nil>} -- term_id is nil (not an
// absent key) whenever resolution did not return an approved term, so a
// reader can distinguish "resolution attempted, unresolved" from "field not
// configured at all" (ADR 2026082203 DR2).
func BuildSignatureProperties(ctx context.Context, resolver GovernedPropertyResolver, entries []appconfig.OntologyTermPropertyMapEntry, fieldMap map[string]any, recordID int64) (map[string]any, error) {
	if len(entries) == 0 || len(fieldMap) == 0 {
		return nil, nil
	}
	out := map[string]any{}
	for _, entry := range entries {
		val := lookupFieldPath(fieldMap, entry.Field)
		if isEmptyFieldValue(val) {
			continue
		}
		if !entry.Identity {
			out[entry.Property] = val
			continue
		}
		raw := fieldValueAsString(val)
		termID, _, err := resolver.Lookup(ctx, entry.Dimension, raw, recordID)
		if err != nil {
			return nil, fmt.Errorf("resolve identity dimension %q for field %q: %w", entry.Dimension, entry.Field, err)
		}
		var termIDOrNil any
		if termID != "" {
			termIDOrNil = termID
		}
		out[entry.Property] = map[string]any{"raw": raw, "term_id": termIDOrNil}
	}
	if len(out) == 0 {
		return nil, nil
	}
	return out, nil
}

// fieldValueAsString coerces a field-map value to the string form
// GovernedPropertyResolver.Lookup normalizes and stores as raw_value. Every
// identity-configured metric field is already string-valued
// (metricFieldMap), so the string case is the common path; fmt.Sprint is a
// defensive fallback rather than a real code path.
func fieldValueAsString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprint(v)
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
