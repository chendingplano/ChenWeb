package terminology

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/chendingplano/deepdoc/server/api/ontology/keywords"
	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/transform"
)

type ucumUnitsXML struct {
	XMLName xml.Name      `xml:"root"`
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
	decoder := xml.NewDecoder(bytes.NewReader(artifacts[0].Content))
	decoder.CharsetReader = ucumCharsetReader
	if err := decoder.Decode(&essence); err != nil {
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

// ucumCharsetReader lets encoding/xml decode the official UCUM essence,
// which declares encoding="ascii" while carrying a pure-ASCII payload.
// Unknown legacy labels fail closed rather than guessing at the encoding.
func ucumCharsetReader(charset string, input io.Reader) (io.Reader, error) {
	switch strings.ToLower(strings.TrimSpace(charset)) {
	case "", "utf-8", "utf8", "us-ascii", "ascii":
		return input, nil
	case "iso-8859-1", "latin1":
		return transform.NewReader(input, charmap.ISO8859_1.NewDecoder()), nil
	case "windows-1252", "cp1252":
		return transform.NewReader(input, charmap.Windows1252.NewDecoder()), nil
	default:
		return nil, fmt.Errorf("unsupported xml charset %q", charset)
	}
}

func init() { _ = RegisterAdapter(UCUMAdapter{}) }
