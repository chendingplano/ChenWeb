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

// SIRPRelease is the governed release pinned by the SIRP adapter. QUDT
// siExactMatch crosswalks carry this release so promotion can distinguish an
// authoritative SIRP mapping from proposal-only links.
const SIRPRelease = "1.0.0"

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

// QUDTQuantityKind is the identity-relevant projection of one QUDT resource.
// Class records the rdf:type so callers can project the subset they own; the
// QUDT adapter emits quantity kinds only, while the legacy quantity import
// command also consumes units and dimension vectors. Deprecation and
// replacement are retained rather than silently dropped.
type QUDTQuantityKind struct {
	IRI        string
	Class      string
	Deprecated bool
	Symbol     string
	Labels     []QUDTLabel
	Relations  []QUDTRelation
}

// ParseQUDTGraph parses a QUDT Turtle artifact into identity-relevant
// resources. Only subjects explicitly typed as a supported QUDT class are
// collected; class-less resources and unrelated classes (for example unit or
// dimension-vector definitions that leak into a quantity-kinds artifact) are
// ignored rather than emitted or rejected. Language tags are preserved;
// unknown relation predicates on collected subjects fail closed instead of
// being upgraded.
func ParseQUDTGraph(content []byte) ([]QUDTQuantityKind, error) {
	graph := rdf2go.NewGraph("")
	if err := graph.Parse(bufio.NewReader(strings.NewReader(string(content))), "text/turtle"); err != nil {
		return nil, fmt.Errorf("qudt turtle parse: %w", err)
	}

	type accumulator struct {
		class      string
		deprecated bool
		symbol     string
		labels     map[string]bool
		relations  map[string]QUDTRelation
	}
	subjects := map[string]*accumulator{}
	// Pass 1: collect only subjects typed as a supported QUDT class.
	for triple := range graph.IterTriples() {
		if triple.Predicate.RawValue() != qudtRDFType {
			continue
		}
		class := triple.Object.RawValue()
		if class != qudtQuantityKindClass && class != qudtUnitClass && class != qudtDimensionClass {
			continue
		}
		subject := triple.Subject.RawValue()
		if _, ok := subjects[subject]; !ok {
			subjects[subject] = &accumulator{class: class, labels: map[string]bool{}, relations: map[string]QUDTRelation{}}
		}
	}
	if len(subjects) == 0 {
		return nil, errors.New("qudt turtle contains no supported QUDT resources")
	}

	// Pass 2: gather labels/symbol/deprecation/relations only for collected
	// subjects. Unknown relation predicates fail closed instead of being
	// silently upgraded or dropped.
	for triple := range graph.IterTriples() {
		predicate := triple.Predicate.RawValue()
		subject := triple.Subject.RawValue()
		acc, ok := subjects[subject]
		if !ok {
			continue
		}
		if !qudtAllowedPredicates[predicate] {
			if strings.HasPrefix(predicate, "http://www.w3.org/2004/02/skos/core#") ||
				strings.HasPrefix(predicate, "http://www.w3.org/2002/07/owl#") {
				return nil, fmt.Errorf("unknown QUDT relation predicate %q", predicate)
			}
			continue
		}
		switch predicate {
		case qudtRDFType:
			// Pass 1 already decided membership; nothing to add here.
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

	out := make([]QUDTQuantityKind, 0, len(subjects))
	for subject := range subjects {
		acc := subjects[subject]
		resource := QUDTQuantityKind{IRI: subject, Class: acc.class, Deprecated: acc.deprecated, Symbol: acc.symbol}
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
		if resource.Class != qudtQuantityKindClass {
			continue
		}
		entryStatus := "current"
		if resource.Deprecated {
			entryStatus = "deprecated"
		}
		payload, err := json.Marshal(map[string]any{"deprecated": resource.Deprecated, "symbol": resource.Symbol})
		if err != nil {
			return CatalogSnapshot{}, fmt.Errorf("encode qudt payload: %w", err)
		}
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
			if objectRelease == "" && objectSource == SIRPSourceID {
				objectRelease = SIRPRelease
			}
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
	if len(snapshot.Entries) == 0 {
		return CatalogSnapshot{}, errors.New("qudt artifact contains no quantity-kind resources")
	}
	normalizeSnapshot(&snapshot)
	return snapshot, nil
}

func init() { _ = RegisterAdapter(QUDTAdapter{}) }
