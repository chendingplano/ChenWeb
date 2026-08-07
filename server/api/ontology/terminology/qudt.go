package terminology

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/deiu/rdf2go"

	"github.com/chendingplano/deepdoc/server/api/ontology/keywords"
)

// SIRPSourceID is the governed source identifier for BIPM SI Reference Point
// quantities. QUDT siExactMatch crosswalks reference these persistent IDs.
const SIRPSourceID = "bipm-sirp-quantity"

const (
	qudtQuantityKindClass = "http://qudt.org/schema/qudt/QuantityKind"
	qudtUnitClass         = "http://qudt.org/schema/qudt/Unit"
	qudtDimensionClass    = "http://qudt.org/schema/qudt/DimensionVector"

	qudtRDFType        = "http://www.w3.org/1999/02/22-rdf-syntax-ns#type"
	qudtRDFSLabel      = "http://www.w3.org/2000/01/rdf-schema#label"
	qudtSKOSPrefLabel  = "http://www.w3.org/2004/02/skos/core#prefLabel"
	qudtSKOSAltLabel   = "http://www.w3.org/2004/02/skos/core#altLabel"
	qudtOWLDeprecated  = "http://www.w3.org/2002/07/owl#deprecated"
	qudtDeprecated     = "http://qudt.org/schema/qudt/deprecated"
	qudtSymbol         = "http://qudt.org/schema/qudt/symbol"
	qudtSIExactMatch   = "http://qudt.org/schema/qudt/siExactMatch"
	qudtSKOSExactMatch = "http://www.w3.org/2004/02/skos/core#exactMatch"
	qudtWikidataMatch  = "http://qudt.org/schema/qudt/wikidataMatch"
	qudtSKOSBroader    = "http://www.w3.org/2004/02/skos/core#broader"
	qudtSKOSNarrower   = "http://www.w3.org/2004/02/skos/core#narrower"
	qudtSKOSRelated    = "http://www.w3.org/2004/02/skos/core#related"
	qudtReplaces       = "http://qudt.org/schema/qudt/replaces"
)

var qudtAllowedPredicates = map[string]bool{
	qudtRDFType: true, qudtRDFSLabel: true, qudtSKOSPrefLabel: true, qudtSKOSAltLabel: true,
	qudtOWLDeprecated: true, qudtDeprecated: true, qudtSymbol: true,
	qudtSIExactMatch: true, qudtSKOSExactMatch: true, qudtWikidataMatch: true,
	qudtSKOSBroader: true, qudtSKOSNarrower: true, qudtSKOSRelated: true, qudtReplaces: true,
}

// QUDTLabel preserves the lexical form, language tag, and role of one label.
type QUDTLabel struct {
	Value    string
	Language string
	Role     string
}

// QUDTRelation is one normalized identity-relevant relation from a QUDT
// graph. Exact crosswalks keep the object's source explicit so promotion can
// distinguish authoritative SIRP mappings from proposal-only Wikidata links.
type QUDTRelation struct {
	Relation       string
	ObjectSource   string
	ObjectRelease  string
	ObjectExternal string
}

// QUDTQuantityKind is the identity-relevant projection of one QUDT
// quantity-kind resource. Deprecation and replacement are retained rather
// than silently dropped.
type QUDTQuantityKind struct {
	IRI        string
	Deprecated bool
	Symbol     string
	Labels     []QUDTLabel
	Relations  []QUDTRelation
}

// ParseQUDTGraph parses a QUDT Turtle artifact into identity-relevant
// quantity-kind resources. Language tags are preserved; unknown relation
// predicates fail closed instead of being upgraded.
func ParseQUDTGraph(content []byte) ([]QUDTQuantityKind, error) {
	graph := rdf2go.NewGraph("")
	if err := graph.Parse(bufio.NewReader(strings.NewReader(string(content))), "text/turtle"); err != nil {
		return nil, fmt.Errorf("qudt turtle parse: %w", err)
	}

	type accumulator struct {
		deprecated bool
		symbol     string
		labels     map[string]bool
		relations  map[string]QUDTRelation
	}
	subjects := map[string]*accumulator{}
	var order []string
	for triple := range graph.IterTriples() {
		predicate := triple.Predicate.RawValue()
		if !qudtAllowedPredicates[predicate] {
			if strings.HasPrefix(predicate, "http://www.w3.org/2004/02/skos/core#") ||
				strings.HasPrefix(predicate, "http://www.w3.org/2002/07/owl#") {
				return nil, fmt.Errorf("unknown QUDT relation predicate %q", predicate)
			}
			continue
		}
		subject := triple.Subject.RawValue()
		acc, ok := subjects[subject]
		if !ok {
			acc = &accumulator{labels: map[string]bool{}, relations: map[string]QUDTRelation{}}
			subjects[subject] = acc
			order = append(order, subject)
		}
		switch predicate {
		case qudtRDFType:
			class := triple.Object.RawValue()
			if class != qudtQuantityKindClass && class != qudtUnitClass && class != qudtDimensionClass {
				return nil, fmt.Errorf("QUDT subject %q has unsupported class %q", subject, class)
			}
		case qudtOWLDeprecated, qudtDeprecated:
			if literal, ok := triple.Object.(*rdf2go.Literal); ok && strings.EqualFold(literal.Value, "true") {
				acc.deprecated = true
			}
		case qudtSymbol:
			if literal, ok := triple.Object.(*rdf2go.Literal); ok {
				acc.symbol = literal.Value
			}
		case qudtRDFSLabel, qudtSKOSPrefLabel, qudtSKOSAltLabel:
			literal, ok := triple.Object.(*rdf2go.Literal)
			if !ok {
				return nil, fmt.Errorf("QUDT label object for %q is not a literal", subject)
			}
			language := literal.Language
			if language == "" {
				language = "und"
			}
			role := "alternative"
			if predicate == qudtRDFSLabel || predicate == qudtSKOSPrefLabel {
				role = "preferred"
			}
			acc.labels[language+"\x00"+role+"\x00"+literal.Value] = true
		case qudtSIExactMatch, qudtSKOSExactMatch:
			relation := QUDTRelation{
				Relation: "exact_equivalent", ObjectRelease: "",
				ObjectExternal: triple.Object.RawValue(),
			}
			if predicate == qudtSIExactMatch {
				relation.ObjectSource = SIRPSourceID
			}
			acc.relations[relationKey(subject, relation.Relation, relation.ObjectSource, relation.ObjectRelease, relation.ObjectExternal)] = relation
		case qudtWikidataMatch:
			acc.relations[relationKey(subject, "wikidata_match", "wikidata", "", triple.Object.RawValue())] = QUDTRelation{
				Relation: "wikidata_match", ObjectSource: "wikidata", ObjectRelease: "",
				ObjectExternal: triple.Object.RawValue(),
			}
		case qudtSKOSBroader:
			acc.relations[relationKey(subject, "broader", "", "", triple.Object.RawValue())] = QUDTRelation{
				Relation: "broader", ObjectExternal: triple.Object.RawValue(),
			}
		case qudtSKOSNarrower:
			acc.relations[relationKey(subject, "narrower", "", "", triple.Object.RawValue())] = QUDTRelation{
				Relation: "narrower", ObjectExternal: triple.Object.RawValue(),
			}
		case qudtSKOSRelated:
			acc.relations[relationKey(subject, "related", "", "", triple.Object.RawValue())] = QUDTRelation{
				Relation: "related", ObjectExternal: triple.Object.RawValue(),
			}
		case qudtReplaces:
			acc.relations[relationKey(subject, "replaces", "", "", triple.Object.RawValue())] = QUDTRelation{
				Relation: "replaces", ObjectExternal: triple.Object.RawValue(),
			}
		}
	}
	if len(subjects) == 0 {
		return nil, errors.New("qudt turtle contains no quantity-kind resources")
	}

	out := make([]QUDTQuantityKind, 0, len(order))
	for _, subject := range order {
		acc := subjects[subject]
		resource := QUDTQuantityKind{IRI: subject, Deprecated: acc.deprecated, Symbol: acc.symbol}
		for key := range acc.labels {
			parts := strings.Split(key, "\x00")
			resource.Labels = append(resource.Labels, QUDTLabel{Value: parts[2], Language: parts[0], Role: parts[1]})
		}
		sort.Slice(resource.Labels, func(i, j int) bool {
			a, b := resource.Labels[i], resource.Labels[j]
			if a.Language != b.Language {
				return a.Language < b.Language
			}
			if a.Role != b.Role {
				return a.Role < b.Role
			}
			return a.Value < b.Value
		})
		for _, relation := range acc.relations {
			resource.Relations = append(resource.Relations, relation)
		}
		sort.Slice(resource.Relations, func(i, j int) bool {
			a, b := resource.Relations[i], resource.Relations[j]
			if a.Relation != b.Relation {
				return a.Relation < b.Relation
			}
			return a.ObjectExternal < b.ObjectExternal
		})
		out = append(out, resource)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].IRI < out[j].IRI })
	return out, nil
}

// QUDTAdapter imports QUDT quantity-kind Turtle into staging. Exact SIRP
// crosswalks normalize to exact_equivalent relations; wikidataMatch remains a
// proposal-only wikidata_match relation that can never authorize.
type QUDTAdapter struct{}

func (QUDTAdapter) ID() string      { return "qudt-quantity-kind" }
func (QUDTAdapter) Version() string { return "3.5.0" }

func (a QUDTAdapter) Convert(ctx context.Context, policy keywords.SourcePolicy, artifacts []VerifiedArtifact) (CatalogSnapshot, error) {
	if len(artifacts) != 1 {
		return CatalogSnapshot{}, errors.New("qudt adapter requires exactly one turtle artifact")
	}
	resources, err := ParseQUDTGraph(artifacts[0].Content)
	if err != nil {
		return CatalogSnapshot{}, err
	}
	snapshot := CatalogSnapshot{}
	for _, resource := range resources {
		entryStatus := "current"
		if resource.Deprecated {
			entryStatus = "deprecated"
		}
		payload, _ := json.Marshal(map[string]any{"deprecated": resource.Deprecated, "symbol": resource.Symbol})
		snapshot.Entries = append(snapshot.Entries, CatalogEntry{
			ExternalID: resource.IRI, EntryStatus: entryStatus, ProvenanceLocator: resource.IRI, NativePayload: payload,
		})
		for _, label := range resource.Labels {
			snapshot.Labels = append(snapshot.Labels, CatalogLabel{
				ExternalID: resource.IRI, Language: label.Language, LabelRole: label.Role, Label: label.Value,
				ProvenanceLocator: resource.IRI,
			})
		}
		for _, relation := range resource.Relations {
			objectSource := relation.ObjectSource
			if objectSource == "" {
				objectSource = policy.Source
			}
			objectRelease := relation.ObjectRelease
			if objectRelease == "" && objectSource == policy.Source {
				objectRelease = policy.Release
			}
			snapshot.Relations = append(snapshot.Relations, CatalogRelation{
				SubjectExternalID: resource.IRI, Relation: relation.Relation,
				ObjectSource: objectSource, ObjectRelease: objectRelease, ObjectExternalID: relation.ObjectExternal,
				ProvenanceLocator: resource.IRI,
			})
		}
	}
	normalizeSnapshot(&snapshot)
	return snapshot, nil
}

func init() { _ = RegisterAdapter(QUDTAdapter{}) }
