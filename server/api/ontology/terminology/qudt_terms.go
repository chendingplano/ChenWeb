package terminology

import "strings"

// QUDTImportedTerm is one governed quantity/unit/dimension term derived from
// a parsed QUDT resource, ready to write into kb.ontology_terms.
type QUDTImportedTerm struct {
	TermID    string
	Kind      string
	SourceIRI string
	PrefLabel string
	Symbol    string
}

// qudtClassInfo maps a QUDT RDF class to the governed quantity-module term_id
// prefix and term_kind it produces.
var qudtClassInfo = map[string]struct{ Prefix, Kind string }{
	qudtUnitClass:         {"unit_", "unit"},
	qudtQuantityKindClass: {"qk_", "quantity_kind"},
	qudtDimensionClass:    {"dim_", "dimension"},
}

// ClassifyQUDTTerms converts parsed QUDT resources -- of any mix of unit,
// quantity-kind, and dimension classes, e.g. the full combined qudt-all.ttl
// graph -- into governed quantity-module terms, skipping deprecated entries.
func ClassifyQUDTTerms(resources []QUDTQuantityKind) []QUDTImportedTerm {
	out := make([]QUDTImportedTerm, 0, len(resources))
	for _, r := range resources {
		info, ok := qudtClassInfo[r.Class]
		if !ok || r.Deprecated {
			continue
		}
		out = append(out, QUDTImportedTerm{
			TermID:    "quantity:" + info.Prefix + qudtLocalName(r.IRI),
			Kind:      info.Kind,
			SourceIRI: r.IRI,
			PrefLabel: pickQUDTLabel(r.Labels),
			Symbol:    r.Symbol,
		})
	}
	return out
}

// pickQUDTLabel selects the en preferred label for the quantity module's
// prefLabel, falling back to any preferred label and then the first label.
func pickQUDTLabel(labels []QUDTLabel) string {
	for _, l := range labels {
		if l.Role == "preferred" && l.Language == "en" {
			return l.Value
		}
	}
	for _, l := range labels {
		if l.Role == "preferred" {
			return l.Value
		}
	}
	if len(labels) > 0 {
		return labels[0].Value
	}
	return ""
}

func qudtLocalName(iri string) string {
	if i := strings.LastIndex(iri, "/"); i >= 0 {
		return iri[i+1:]
	}
	return iri
}
