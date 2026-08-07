package terminology

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/chendingplano/deepdoc/server/api/ontology/keywords"
)

// WikidataLine is one revision-pinned entity in a streaming JSONL snapshot.
// Every claim is proposal/context only; authority is decided by the source
// policy, never by this parser.
type WikidataLine struct {
	ID          string              `json:"id"`
	Revision    int64               `json:"revision"`
	Labels      map[string]string   `json:"labels"`
	Aliases     map[string][]string `json:"aliases,omitempty"`
	ExternalIDs []ExternalIDClaim   `json:"external_ids,omitempty"`
	Statements  []WikidataStatement `json:"statements,omitempty"`
	RetrievedAt time.Time           `json:"retrieved_at"`
}

type ExternalIDClaim struct {
	Property string `json:"property"`
	Value    string `json:"value"`
}

type WikidataStatement struct {
	Type   string `json:"type"`
	Object string `json:"object"`
}

var allowedWikidataStatementTypes = map[string]bool{
	"different_from": true, "broader": true, "narrower": true, "unit_statement": true,
}

// WikidataAdapter streams revision-pinned JSONL into proposal-only catalog
// rows. Unknown statement types fail closed; duplicate Q-IDs are rejected.
type WikidataAdapter struct{}

func (WikidataAdapter) ID() string      { return "wikidata" }
func (WikidataAdapter) Version() string { return "0.1.0" }

func (a WikidataAdapter) Convert(ctx context.Context, policy keywords.SourcePolicy, artifacts []VerifiedArtifact) (CatalogSnapshot, error) {
	if len(artifacts) != 1 {
		return CatalogSnapshot{}, errors.New("wikidata adapter requires exactly one jsonl artifact")
	}
	snapshot := CatalogSnapshot{}
	seen := map[string]bool{}

	reader := bufio.NewReader(bytes.NewReader(artifacts[0].Content))
	for {
		line, err := readJSONLLine(reader)
		if err != nil {
			if errors.Is(err, errJSONLEOF) {
				break
			}
			return CatalogSnapshot{}, err
		}
		if len(line) == 0 {
			continue
		}
		if err := rejectDuplicateJSONKeys(line); err != nil {
			return CatalogSnapshot{}, fmt.Errorf("wikidata line JSON: %w", err)
		}
		var entity WikidataLine
		decoder := json.NewDecoder(bytes.NewReader(line))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&entity); err != nil {
			return CatalogSnapshot{}, fmt.Errorf("decode wikidata line: %w", err)
		}
		if strings.TrimSpace(entity.ID) == "" || entity.Revision <= 0 {
			return CatalogSnapshot{}, errors.New("wikidata line requires id and positive revision")
		}
		if entity.RetrievedAt.IsZero() {
			return CatalogSnapshot{}, fmt.Errorf("wikidata %s requires retrieved_at", entity.ID)
		}
		if seen[entity.ID] {
			return CatalogSnapshot{}, fmt.Errorf("wikidata entity %s is duplicated", entity.ID)
		}
		seen[entity.ID] = true

		payload, _ := json.Marshal(map[string]any{
			"revision": entity.Revision, "retrieved_at": entity.RetrievedAt, "external_ids": entity.ExternalIDs,
		})
		snapshot.Entries = append(snapshot.Entries, CatalogEntry{
			ExternalID: entity.ID, EntryStatus: "proposal", ProvenanceLocator: entity.ID, NativePayload: payload,
		})
		for language, label := range entity.Labels {
			snapshot.Labels = append(snapshot.Labels, CatalogLabel{
				ExternalID: entity.ID, Language: language, LabelRole: "preferred", Label: label,
				ProvenanceLocator: entity.ID,
			})
		}
		for language, aliases := range entity.Aliases {
			for _, alias := range aliases {
				snapshot.Labels = append(snapshot.Labels, CatalogLabel{
					ExternalID: entity.ID, Language: language, LabelRole: "alternative", Label: alias,
					ProvenanceLocator: entity.ID,
				})
			}
		}
		for _, statement := range entity.Statements {
			if !allowedWikidataStatementTypes[statement.Type] {
				return CatalogSnapshot{}, fmt.Errorf("unknown wikidata statement type %q on %s", statement.Type, entity.ID)
			}
			if strings.TrimSpace(statement.Object) == "" {
				return CatalogSnapshot{}, fmt.Errorf("wikidata %s statement %q requires an object", entity.ID, statement.Type)
			}
			switch statement.Type {
			case "different_from":
				snapshot.NegativeDecisions = append(snapshot.NegativeDecisions, CatalogNegativeDecision{
					SubjectExternalID: entity.ID, ObjectSource: policy.Source, ObjectRelease: policy.Release,
					ObjectExternalID: statement.Object, Relation: "different_from",
					Reason: "wikidata proposal different_from", ProvenanceLocator: entity.ID,
				})
			case "broader":
				snapshot.Relations = append(snapshot.Relations, CatalogRelation{
					SubjectExternalID: entity.ID, Relation: "broader", ObjectSource: policy.Source,
					ObjectRelease: policy.Release, ObjectExternalID: statement.Object, ProvenanceLocator: entity.ID,
				})
			case "narrower":
				snapshot.Relations = append(snapshot.Relations, CatalogRelation{
					SubjectExternalID: entity.ID, Relation: "narrower", ObjectSource: policy.Source,
					ObjectRelease: policy.Release, ObjectExternalID: statement.Object, ProvenanceLocator: entity.ID,
				})
			case "unit_statement":
				snapshot.Relations = append(snapshot.Relations, CatalogRelation{
					SubjectExternalID: entity.ID, Relation: "unit_statement", ObjectSource: "ucum",
					ObjectRelease: "proposal", ObjectExternalID: statement.Object, ProvenanceLocator: entity.ID,
				})
			}
		}
	}
	if len(snapshot.Entries) == 0 {
		return CatalogSnapshot{}, errors.New("wikidata snapshot contains no entities")
	}
	normalizeSnapshot(&snapshot)
	return snapshot, nil
}

var errJSONLEOF = errors.New("jsonl end of stream")

func readJSONLLine(reader *bufio.Reader) ([]byte, error) {
	line, err := reader.ReadBytes('\n')
	if len(line) > 0 {
		return bytes.TrimSpace(line), nil
	}
	if err != nil {
		if errors.Is(err, io.EOF) {
			return nil, errJSONLEOF
		}
		return nil, err
	}
	return nil, errJSONLEOF
}

func init() { _ = RegisterAdapter(WikidataAdapter{}) }
