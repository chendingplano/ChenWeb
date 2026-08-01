package docprocessing

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"unicode"

	"github.com/chendingplano/deepdoc/server/api/ontology/candidates"
)

// metricDefinitionMention is the normalized, document-derived input for a
// governed metric-definition proposal.  It deliberately carries document
// evidence rather than treating an extracted value as a definition.
type metricDefinitionMention struct {
	CanonicalName string
	Aliases       []string
	Definition    string
	ValueType     string
	RangeType     string
	Confidence    float64
	LineNumbers   []int
}

// buildMetricDefinitionCandidate turns one extracted definition into the
// reviewable ontology candidate required by ADR §8.2.  Promotion remains an
// explicit curator action; this helper never writes governed content itself.
func buildMetricDefinitionCandidate(recordID int64, m metricDefinitionMention) (candidates.Candidate, error) {
	name := strings.TrimSpace(m.CanonicalName)
	if recordID <= 0 {
		return candidates.Candidate{}, fmt.Errorf("input record id is required")
	}
	if name == "" {
		return candidates.Candidate{}, fmt.Errorf("metric definition canonical name is required")
	}
	spans := normalizedLineNumbers(m.LineNumbers)
	payload, err := json.Marshal(map[string]any{
		"term_id":    "measurement:" + candidateIdentifier(name),
		"term_kind":  "metric_definition",
		"module_id":  "measurement",
		"definition": strings.TrimSpace(m.Definition),
		"scope":      "document-derived candidate",
		"label":      name,
		"aliases":    normalizedStrings(m.Aliases),
		"value_type": strings.TrimSpace(m.ValueType),
		"range_type": strings.TrimSpace(m.RangeType),
	})
	if err != nil {
		return candidates.Candidate{}, fmt.Errorf("marshal metric definition candidate: %w", err)
	}
	confidence := m.Confidence
	if confidence < 0 || confidence > 1 {
		confidence = 0
	}
	return candidates.Candidate{
		CandidateKind:    "term",
		ProposedPayload:  payload,
		ProposedModuleID: "measurement",
		SourceType:       "document",
		SourceRef:        fmt.Sprintf("input_record:%d", recordID),
		SourceLineSpans:  spans,
		DiscoveryMethod:  "routed_extraction",
		Confidence:       &confidence,
		ProposedBy:       "extract_metric_definitions",
	}, nil
}

func normalizedLineNumbers(lines []int) json.RawMessage {
	set := map[int]bool{}
	for _, line := range lines {
		if line > 0 {
			set[line] = true
		}
	}
	out := make([]int, 0, len(set))
	for line := range set {
		out = append(out, line)
	}
	sort.Ints(out)
	b, _ := json.Marshal(out)
	return b
}

func normalizedStrings(in []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, raw := range in {
		v := strings.TrimSpace(raw)
		if v != "" && !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	sort.Strings(out)
	return out
}

func candidateIdentifier(label string) string {
	var b strings.Builder
	lastUnderscore := false
	for _, r := range strings.ToLower(strings.TrimSpace(label)) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			if r <= unicode.MaxASCII {
				b.WriteRune(r)
				lastUnderscore = false
			}
			continue
		}
		if b.Len() > 0 && !lastUnderscore {
			b.WriteByte('_')
			lastUnderscore = true
		}
	}
	base := strings.Trim(b.String(), "_")
	if base != "" {
		return base
	}
	digest := sha256.Sum256([]byte(label))
	return "term_" + hex.EncodeToString(digest[:4])
}
