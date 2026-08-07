package terminology

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/chendingplano/deepdoc/server/api/ontology/keywords"
)

const IECSeedSchemaVersion = 1

// IECSeed is a reviewed, minimal bilingual seed. It carries local surfaces,
// stable IEV references, scope/constraint metadata, and the reviewer
// decision; it never contains copied definitions.
type IECSeed struct {
	SchemaVersion     int               `json:"schema_version"`
	Entries           []IECSeedEntry    `json:"entries"`
	NegativeDecisions []IECSeedNegative `json:"negative_decisions,omitempty"`
}

type IECSeedEntry struct {
	ExternalID           string       `json:"external_id"`
	Status               string       `json:"status"`
	PublicationDate      string       `json:"publication_date,omitempty"`
	RetrievedAt          time.Time    `json:"retrieved_at"`
	Scope                string       `json:"scope,omitempty"`
	UnitConstraints      []string     `json:"unit_constraints,omitempty"`
	DimensionConstraints []string     `json:"dimension_constraints,omitempty"`
	Decision             string       `json:"decision"`
	Reviewer             string       `json:"reviewer"`
	ReviewedAt           time.Time    `json:"reviewed_at"`
	Surfaces             []IECSurface `json:"surfaces"`
	ProvenanceLocator    string       `json:"provenance_locator"`
	Notes                string       `json:"notes,omitempty"`
}

type IECSurface struct {
	Language string `json:"language"`
	Surface  string `json:"surface"`
	Role     string `json:"role,omitempty"`
}

type IECSeedNegative struct {
	SubjectExternalID string    `json:"subject_external_id"`
	ObjectExternalID  string    `json:"object_external_id"`
	Relation          string    `json:"relation"`
	Reason            string    `json:"reason"`
	Scope             string    `json:"scope"`
	Reviewer          string    `json:"reviewer"`
	ReviewedAt        time.Time `json:"reviewed_at"`
	ProvenanceLocator string    `json:"provenance_locator"`
}

func (s *IECSeed) Validate() error {
	if s.SchemaVersion != IECSeedSchemaVersion {
		return fmt.Errorf("unsupported iec seed schema_version %d", s.SchemaVersion)
	}
	if len(s.Entries) == 0 {
		return errors.New("iec seed requires at least one entry")
	}
	seen := map[string]bool{}
	for i, entry := range s.Entries {
		if strings.TrimSpace(entry.ExternalID) == "" {
			return fmt.Errorf("iec seed entry[%d] external_id is required", i)
		}
		if seen[entry.ExternalID] {
			return fmt.Errorf("iec seed entry %q is duplicated", entry.ExternalID)
		}
		seen[entry.ExternalID] = true
		if strings.TrimSpace(entry.Reviewer) == "" || entry.ReviewedAt.IsZero() {
			return fmt.Errorf("iec seed entry %q requires reviewer and reviewed_at", entry.ExternalID)
		}
		if entry.Decision != "exact" && entry.Decision != "contextual" && entry.Decision != "none" {
			return fmt.Errorf("iec seed entry %q decision must be exact, contextual, or none", entry.ExternalID)
		}
		if entry.Decision == "exact" {
			if strings.TrimSpace(entry.Scope) == "" {
				return fmt.Errorf("iec seed entry %q exact decision requires a scope", entry.ExternalID)
			}
			if len(entry.Surfaces) == 0 {
				return fmt.Errorf("iec seed entry %q exact decision requires reviewed surfaces", entry.ExternalID)
			}
		}
		if strings.TrimSpace(entry.ProvenanceLocator) == "" {
			return fmt.Errorf("iec seed entry %q requires provenance_locator", entry.ExternalID)
		}
		if entry.RetrievedAt.IsZero() {
			return fmt.Errorf("iec seed entry %q requires retrieved_at", entry.ExternalID)
		}
		for _, surface := range entry.Surfaces {
			if strings.TrimSpace(surface.Language) == "" || strings.TrimSpace(surface.Surface) == "" {
				return fmt.Errorf("iec seed entry %q surfaces require language and surface", entry.ExternalID)
			}
		}
	}
	for i, neg := range s.NegativeDecisions {
		if strings.TrimSpace(neg.SubjectExternalID) == "" || strings.TrimSpace(neg.ObjectExternalID) == "" ||
			strings.TrimSpace(neg.Relation) == "" || strings.TrimSpace(neg.Reason) == "" {
			return fmt.Errorf("iec seed negative[%d] requires subject, object, relation, and reason", i)
		}
		if strings.TrimSpace(neg.Reviewer) == "" || neg.ReviewedAt.IsZero() || strings.TrimSpace(neg.Scope) == "" {
			return fmt.Errorf("iec seed negative[%d] requires scope, reviewer, and reviewed_at", i)
		}
		if strings.TrimSpace(neg.ProvenanceLocator) == "" {
			return fmt.Errorf("iec seed negative[%d] requires provenance_locator", i)
		}
	}
	return nil
}

// IECSeedAdapter imports a reviewed IEC seed into staging. Reviewed exact
// decisions surface as catalog entries and labels; the adapter never invents
// identity from labels and rejects unreviewed or copied content.
type IECSeedAdapter struct{}

func (IECSeedAdapter) ID() string      { return "iec-seed" }
func (IECSeedAdapter) Version() string { return "0.1.0" }

func (a IECSeedAdapter) Convert(ctx context.Context, policy keywords.SourcePolicy, artifacts []VerifiedArtifact) (CatalogSnapshot, error) {
	if len(artifacts) != 1 {
		return CatalogSnapshot{}, errors.New("iec seed adapter requires exactly one json artifact")
	}
	content := artifacts[0].Content
	if err := rejectDuplicateJSONKeys(content); err != nil {
		return CatalogSnapshot{}, fmt.Errorf("iec seed JSON: %w", err)
	}
	var seed IECSeed
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&seed); err != nil {
		return CatalogSnapshot{}, fmt.Errorf("decode iec seed: %w", err)
	}
	if err := requireJSONEOF(decoder); err != nil {
		return CatalogSnapshot{}, err
	}
	if err := seed.Validate(); err != nil {
		return CatalogSnapshot{}, err
	}

	snapshot := CatalogSnapshot{}
	for _, entry := range seed.Entries {
		payload, _ := json.Marshal(map[string]any{
			"decision": entry.Decision, "scope": entry.Scope,
			"unit_constraints": entry.UnitConstraints, "dimension_constraints": entry.DimensionConstraints,
			"reviewer": entry.Reviewer, "reviewed_at": entry.ReviewedAt,
			"publication_date": entry.PublicationDate, "retrieved_at": entry.RetrievedAt, "notes": entry.Notes,
		})
		snapshot.Entries = append(snapshot.Entries, CatalogEntry{
			ExternalID: entry.ExternalID, EntryStatus: entry.Status, ProvenanceLocator: entry.ProvenanceLocator,
			NativePayload: payload,
		})
		for _, surface := range entry.Surfaces {
			role := surface.Role
			if role == "" {
				role = "preferred"
			}
			snapshot.Labels = append(snapshot.Labels, CatalogLabel{
				ExternalID: entry.ExternalID, Language: surface.Language, LabelRole: role, Label: surface.Surface,
				ProvenanceLocator: entry.ProvenanceLocator,
			})
		}
	}
	for _, neg := range seed.NegativeDecisions {
		snapshot.NegativeDecisions = append(snapshot.NegativeDecisions, CatalogNegativeDecision{
			SubjectExternalID: neg.SubjectExternalID, ObjectSource: policy.Source, ObjectRelease: policy.Release,
			ObjectExternalID: neg.ObjectExternalID, Relation: neg.Relation, Reason: neg.Reason,
			ProvenanceLocator: neg.ProvenanceLocator,
		})
	}
	normalizeSnapshot(&snapshot)
	return snapshot, nil
}

func init() { _ = RegisterAdapter(IECSeedAdapter{}) }
