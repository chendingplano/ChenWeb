package terminology

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/deiu/rdf2go"

	"github.com/chendingplano/deepdoc/server/api/ontology/keywords"
)

const (
	sirpQuantityClass = "https://si-digital-framework.org/ont#Quantity"

	sirpRDFType        = "http://www.w3.org/1999/02/22-rdf-syntax-ns#type"
	sirpRDFSLabel      = "http://www.w3.org/2000/01/rdf-schema#label"
	sirpOWLDeprecated  = "http://www.w3.org/2002/07/owl#deprecated"
	sirpSKOSPrefLabel  = "http://www.w3.org/2004/02/skos/core#prefLabel"
	sirpSKOSAltLabel   = "http://www.w3.org/2004/02/skos/core#altLabel"
	sirpSKOSExactMatch = "http://www.w3.org/2004/02/skos/core#exactMatch"
	sirpSKOSBroader    = "http://www.w3.org/2004/02/skos/core#broader"
	sirpSKOSNarrower   = "http://www.w3.org/2004/02/skos/core#narrower"
	sirpSKOSRelated    = "http://www.w3.org/2004/02/skos/core#related"
)

var sirpAllowedPredicates = map[string]bool{
	sirpRDFType: true, sirpRDFSLabel: true, sirpOWLDeprecated: true,
	sirpSKOSPrefLabel: true, sirpSKOSAltLabel: true,
	sirpSKOSExactMatch: true, sirpSKOSBroader: true, sirpSKOSNarrower: true, sirpSKOSRelated: true,
}

// relationPredicateNamespaces lists predicate namespaces whose unknown local
// names must fail closed instead of being silently upgraded or dropped.
var relationPredicateNamespaces = []string{
	"http://www.w3.org/2004/02/skos/core#",
	"http://www.w3.org/2002/07/owl#",
	"http://www.w3.org/2000/01/rdf-schema#",
}

// SIRPAdapter imports BIPM SI Reference Point quantity Turtle. Persistent
// resource identifiers are preserved as opaque external IDs; exact identity
// claims are only ever authorized by the source policy, never by the parser.
type SIRPAdapter struct{}

func (SIRPAdapter) ID() string      { return "bipm-sirp-quantity" }
func (SIRPAdapter) Version() string { return "1.0.0" }

func (a SIRPAdapter) Convert(ctx context.Context, policy keywords.SourcePolicy, artifacts []VerifiedArtifact) (CatalogSnapshot, error) {
	if len(artifacts) != 1 {
		return CatalogSnapshot{}, errors.New("sirp adapter requires exactly one turtle artifact")
	}
	graph := rdf2go.NewGraph("")
	if err := graph.Parse(bufio.NewReader(strings.NewReader(string(artifacts[0].Content))), "text/turtle"); err != nil {
		return CatalogSnapshot{}, fmt.Errorf("sirp turtle parse: %w", err)
	}

	type quantity struct {
		deprecated bool
		labels     map[string]bool // key: language\x00role\x00label
		relations  map[string]bool
	}
	quantities := map[string]*quantity{}
	for triple := range graph.IterTriples() {
		predicate := triple.Predicate.RawValue()
		if !sirpAllowedPredicates[predicate] {
			for _, namespace := range relationPredicateNamespaces {
				if strings.HasPrefix(predicate, namespace) {
					return CatalogSnapshot{}, fmt.Errorf("unknown SIRP relation predicate %q", predicate)
				}
			}
			continue
		}
		subject := triple.Subject.RawValue()
		q, ok := quantities[subject]
		if !ok {
			q = &quantity{labels: map[string]bool{}, relations: map[string]bool{}}
			quantities[subject] = q
		}
		switch predicate {
		case sirpRDFType:
			if triple.Object.RawValue() != sirpQuantityClass {
				return CatalogSnapshot{}, fmt.Errorf("SIRP subject %q is not a quantity", subject)
			}
		case sirpOWLDeprecated:
			q.deprecated = true
		case sirpRDFSLabel, sirpSKOSPrefLabel, sirpSKOSAltLabel:
			literal, ok := triple.Object.(*rdf2go.Literal)
			if !ok {
				return CatalogSnapshot{}, fmt.Errorf("SIRP label object for %q is not a literal", subject)
			}
			language := literal.Language
			if language == "" {
				language = "und"
			}
			role := "alternative"
			if predicate == sirpRDFSLabel || predicate == sirpSKOSPrefLabel {
				role = "preferred"
			}
			q.labels[language+"\x00"+role+"\x00"+literal.Value] = true
		case sirpSKOSExactMatch:
			q.relations[relationKey(subject, "exact_equivalent", policy.Source, policy.Release, triple.Object.RawValue())] = true
		case sirpSKOSBroader:
			q.relations[relationKey(subject, "broader", policy.Source, policy.Release, triple.Object.RawValue())] = true
		case sirpSKOSNarrower:
			q.relations[relationKey(subject, "narrower", policy.Source, policy.Release, triple.Object.RawValue())] = true
		case sirpSKOSRelated:
			q.relations[relationKey(subject, "related", policy.Source, policy.Release, triple.Object.RawValue())] = true
		}
	}
	if len(quantities) == 0 {
		return CatalogSnapshot{}, errors.New("sirp turtle contains no quantities")
	}

	snapshot := CatalogSnapshot{}
	for subject, q := range quantities {
		entryStatus := "current"
		if q.deprecated {
			entryStatus = "deprecated"
		}
		payload, _ := json.Marshal(map[string]any{"deprecated": q.deprecated})
		snapshot.Entries = append(snapshot.Entries, CatalogEntry{
			ExternalID: subject, EntryStatus: entryStatus, ProvenanceLocator: subject, NativePayload: payload,
		})
		for key := range q.labels {
			parts := strings.Split(key, "\x00")
			snapshot.Labels = append(snapshot.Labels, CatalogLabel{
				ExternalID: subject, Language: parts[0], LabelRole: parts[1], Label: parts[2],
				ProvenanceLocator: subject,
			})
		}
		for key := range q.relations {
			parts := strings.Split(key, "\x00")
			snapshot.Relations = append(snapshot.Relations, CatalogRelation{
				SubjectExternalID: subject, Relation: parts[1],
				ObjectSource: parts[2], ObjectRelease: parts[3], ObjectExternalID: parts[4],
				ProvenanceLocator: subject,
			})
		}
	}
	normalizeSnapshot(&snapshot)
	return snapshot, nil
}

func init() { _ = RegisterAdapter(SIRPAdapter{}) }
