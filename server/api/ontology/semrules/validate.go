package semrules

import (
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"sort"
	"strings"
)

// Analysis is the stable static projection of a predicate document.
type Analysis struct {
	RequiredPaths          []string `json:"required_paths"`
	RequiredDocumentFacets []string `json:"required_document_facets"`
	Specificity            int      `json:"specificity"`
	RequiresTier3          bool     `json:"requires_tier_3"`
}

// Validate checks a predicate document against grammar version 1 and the
// registered fact-path and operator metadata.
func Validate(doc Document) error {
	if doc.Version != 1 {
		return fmt.Errorf("version: unsupported grammar version %d", doc.Version)
	}
	return validatePredicate(doc.Expression, "expression")
}

func validatePredicate(predicate Predicate, location string) error {
	switch predicate.Kind {
	case "all", "any":
		if err := validateLogicalFields(predicate, location); err != nil {
			return err
		}
		for index, child := range predicate.Items {
			if err := validatePredicate(child, fmt.Sprintf("%s.items[%d]", location, index)); err != nil {
				return err
			}
		}
		return nil
	case "not":
		if err := validateLogicalFields(predicate, location); err != nil {
			return err
		}
		if len(predicate.Items) != 1 {
			return fmt.Errorf("%s.items: not requires exactly one child", location)
		}
		return validatePredicate(predicate.Items[0], location+".items[0]")
	case "fact":
		return validateFact(predicate, location)
	default:
		return fmt.Errorf("%s.kind: unknown predicate kind %q", location, predicate.Kind)
	}
}

func validateLogicalFields(predicate Predicate, location string) error {
	if predicate.Path != "" {
		return fmt.Errorf("%s.path: logical nodes cannot declare a fact path", location)
	}
	if predicate.Op != "" {
		return fmt.Errorf("%s.op: logical nodes cannot declare an operator", location)
	}
	if predicate.Value != nil {
		return fmt.Errorf("%s.value: logical nodes cannot declare a value", location)
	}
	if predicate.MinConfidence != nil {
		return fmt.Errorf("%s.min_confidence: logical nodes cannot declare a confidence threshold", location)
	}
	if predicate.Facet != "" {
		return fmt.Errorf("%s.facet: version 1 nodes cannot declare a legacy facet", location)
	}
	return nil
}

func validateFact(predicate Predicate, location string) error {
	if len(predicate.Items) != 0 {
		return fmt.Errorf("%s.items: fact nodes cannot have children", location)
	}
	if predicate.Facet != "" {
		return fmt.Errorf("%s.facet: version 1 fact nodes require path", location)
	}
	if predicate.Path == "" {
		return fmt.Errorf("%s.path: fact path is required", location)
	}
	spec, ok := LookupFactPath(predicate.Path)
	if !ok {
		return fmt.Errorf("%s.path: unknown fact path %q", location, predicate.Path)
	}
	if predicate.Op == "" {
		return fmt.Errorf("%s.op: fact operator is required", location)
	}
	if _, ok := lookupOperator(predicate.Op); !ok {
		return fmt.Errorf("%s.op: unknown operator %q", location, predicate.Op)
	}
	if !containsString(spec.Operators, predicate.Op) {
		return fmt.Errorf("%s.op: operator %q is not permitted for %s", location, predicate.Op, spec.Type)
	}
	if confidence := predicate.MinConfidence; confidence != nil {
		if math.IsNaN(*confidence) || math.IsInf(*confidence, 0) || *confidence < 0 || *confidence > 1 {
			return fmt.Errorf("%s.min_confidence: must be between 0 and 1", location)
		}
	}
	if predicate.Op == "exists" {
		if predicate.Value != nil {
			return fmt.Errorf("%s.value: exists does not accept a value", location)
		}
		return nil
	}
	if predicate.Value == nil {
		return fmt.Errorf("%s.value: operator %q requires a value", location, predicate.Op)
	}
	if predicate.Op == "in" || predicate.Op == "not_in" {
		return validateArrayValue(predicate.Value, spec.Type, location+".value")
	}
	return validateScalarValue(predicate.Value, spec.Type, location+".value")
}

func validateArrayValue(value any, factType FactType, location string) error {
	array := reflect.ValueOf(value)
	if !array.IsValid() || (array.Kind() != reflect.Slice && array.Kind() != reflect.Array) {
		return fmt.Errorf("%s: in and not_in require an array", location)
	}
	for index := 0; index < array.Len(); index++ {
		if err := validateScalarValue(array.Index(index).Interface(), factType, fmt.Sprintf("%s[%d]", location, index)); err != nil {
			return err
		}
	}
	return nil
}

func validateScalarValue(value any, factType FactType, location string) error {
	switch factType {
	case FactTypeString, FactTypeStringSet:
		if _, ok := value.(string); !ok {
			return fmt.Errorf("%s: expected string, got %T", location, value)
		}
	case FactTypeNumber:
		if _, err := numericValue(value); err != nil {
			return fmt.Errorf("%s: expected number: %w", location, err)
		}
	case FactTypeBoolean:
		if _, ok := value.(bool); !ok {
			return fmt.Errorf("%s: expected boolean, got %T", location, value)
		}
	case FactTypeDate:
		if _, err := dateValue(value); err != nil {
			return fmt.Errorf("%s: expected date: %w", location, err)
		}
	default:
		return fmt.Errorf("%s: unknown fact type %q", location, factType)
	}
	return nil
}

func containsString(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

// Analyze derives stable requirements from a predicate document. Callers use
// Validate before persisting the result; analysis itself is total so it can
// also support draft diagnostics.
func Analyze(doc Document) Analysis {
	paths := make(map[string]struct{})
	documentFacets := make(map[string]struct{})
	constraints := make(map[string]struct{})
	requiresTier3 := false

	var walk func(Predicate)
	walk = func(predicate Predicate) {
		if predicate.Kind == "fact" {
			paths[predicate.Path] = struct{}{}
			if strings.HasPrefix(predicate.Path, "document.") {
				documentFacets[predicate.Path] = struct{}{}
			}
			if spec, ok := LookupFactPath(predicate.Path); ok && spec.Tier3Producible {
				requiresTier3 = true
			}
			normalized, err := normalizeJSONValue(predicate.Value)
			if err == nil {
				predicate.Value = normalized
			}
			predicate.Items = nil
			encoded, err := json.Marshal(predicate)
			if err == nil {
				constraints[string(encoded)] = struct{}{}
			}
			return
		}
		for _, child := range predicate.Items {
			walk(child)
		}
	}
	walk(doc.Expression)

	return Analysis{
		RequiredPaths:          sortedKeys(paths),
		RequiredDocumentFacets: sortedKeys(documentFacets),
		Specificity:            len(constraints),
		RequiresTier3:          requiresTier3,
	}
}

func sortedKeys(values map[string]struct{}) []string {
	result := make([]string, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}
