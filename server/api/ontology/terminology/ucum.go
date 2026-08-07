package terminology

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"strings"

	"github.com/chendingplano/deepdoc/server/api/ontology/keywords"
)

type ucumUnitsXML struct {
	XMLName xml.Name      `xml:"units"`
	Units   []ucumUnitXML `xml:"unit"`
}

type ucumUnitXML struct {
	Code        string `xml:"Code,attr"`
	PrintSymbol string `xml:"printSymbol"`
	Dim         string `xml:"dim,attr"`
}

// UCUMAdapter reads the official UCUM essence XML into the unit-code staging
// registry. It emits unit codes and supporting evidence only; it never
// creates or merges keyword concepts and never emits quantity identity.
type UCUMAdapter struct{}

func (UCUMAdapter) ID() string      { return "ucum" }
func (UCUMAdapter) Version() string { return "0.1.0" }

func (a UCUMAdapter) Convert(ctx context.Context, policy keywords.SourcePolicy, artifacts []VerifiedArtifact) (CatalogSnapshot, error) {
	if len(artifacts) != 1 {
		return CatalogSnapshot{}, errors.New("ucum adapter requires exactly one xml artifact")
	}
	var essence ucumUnitsXML
	if err := xml.Unmarshal(artifacts[0].Content, &essence); err != nil {
		return CatalogSnapshot{}, fmt.Errorf("parse ucum essence xml: %w", err)
	}
	if len(essence.Units) == 0 {
		return CatalogSnapshot{}, errors.New("ucum essence xml contains no units")
	}
	seen := map[string]bool{}
	snapshot := CatalogSnapshot{}
	for _, unit := range essence.Units {
		if strings.TrimSpace(unit.Code) == "" {
			return CatalogSnapshot{}, errors.New("ucum unit is missing its Code attribute")
		}
		if seen[unit.Code] {
			return CatalogSnapshot{}, fmt.Errorf("ucum unit code %q is duplicated", unit.Code)
		}
		seen[unit.Code] = true
		snapshot.UCUMCodes = append(snapshot.UCUMCodes, UCUMCode{
			Code: unit.Code, PrintSymbol: unit.PrintSymbol, Dimension: unit.Dim,
			ProvenanceLocator: artifacts[0].ProvenanceLocator,
		})
	}
	normalizeSnapshot(&snapshot)
	return snapshot, nil
}

func init() { _ = RegisterAdapter(UCUMAdapter{}) }
