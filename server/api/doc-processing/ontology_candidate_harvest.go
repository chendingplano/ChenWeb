package docprocessing

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
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

type testMethodMention struct {
	ProcedureName string
	Definition    string
	MetricNames   []string
	LineNumbers   []int
}

// buildTestMethodCandidates proposes a procedure concept and, when a metric
// is named explicitly, a governed measured_by axiom candidate. Both sides are
// proposals: no relation becomes active without curator approval and release.
func buildTestMethodCandidates(recordID int64, m testMethodMention) ([]candidates.Candidate, error) {
	procedure := strings.TrimSpace(m.ProcedureName)
	if recordID <= 0 || procedure == "" {
		return nil, fmt.Errorf("input record id and procedure name are required")
	}
	spans := normalizedLineNumbers(m.LineNumbers)
	procedureID := "measurement:" + candidateIdentifier(procedure)
	termPayload, err := json.Marshal(map[string]any{
		"term_id": procedureID, "term_kind": "concept", "module_id": "measurement",
		"definition": strings.TrimSpace(m.Definition), "scope": "document-derived candidate", "label": procedure,
	})
	if err != nil {
		return nil, err
	}
	out := []candidates.Candidate{{CandidateKind: "term", ProposedPayload: termPayload, ProposedModuleID: "measurement", SourceType: "document", SourceRef: fmt.Sprintf("input_record:%d", recordID), SourceLineSpans: spans, DiscoveryMethod: "routed_extraction", ProposedBy: "extract_test_methods"}}
	for _, metric := range normalizedStrings(m.MetricNames) {
		metricID := "measurement:" + candidateIdentifier(metric)
		payload, err := json.Marshal(map[string]any{
			"axiom_id": "measurement:" + candidateIdentifier(metric+" measured by "+procedure), "axiom_kind": "object_property_assertion",
			"subject_term_id": metricID, "predicate_term_id": "mea:measured_by", "object_term_id": procedureID, "module_id": "measurement",
		})
		if err != nil {
			return nil, err
		}
		out = append(out, candidates.Candidate{CandidateKind: "axiom", ProposedPayload: payload, ProposedModuleID: "measurement", SourceType: "document", SourceRef: fmt.Sprintf("input_record:%d", recordID), SourceLineSpans: spans, DiscoveryMethod: "routed_extraction", ProposedBy: "extract_test_methods"})
	}
	return out, nil
}

// OntologyCandidateSink makes the harvesting boundary testable while the
// production runtime uses candidates.CandidateStore directly.
type OntologyCandidateSink interface {
	CreateCandidate(context.Context, candidates.Candidate) (candidates.Candidate, error)
}

// harvestMetricDefinitions submits candidates only for metrics that carry a
// definition. A threshold/value alone is an assertion, not a definition, and
// must remain on the assertion-normalization path.
func harvestMetricDefinitions(ctx context.Context, recordID int64, metrics []map[string]any, sink OntologyCandidateSink) error {
	if sink == nil {
		return fmt.Errorf("ontology candidate sink is nil")
	}
	for _, metric := range metrics {
		definition := strings.TrimSpace(asString(metric["formula_or_definition"]))
		if definition == "" {
			continue
		}
		candidate, err := buildMetricDefinitionCandidate(recordID, metricDefinitionMention{
			CanonicalName: strings.TrimSpace(asString(metric["metric_name"])),
			Definition:    definition,
			ValueType:     strings.TrimSpace(asString(metric["value_data_type"])),
			RangeType:     strings.TrimSpace(asString(metric["value_range_type"])),
			LineNumbers:   sourceSpanLineNumbers(metric["source_line_spans"]),
		})
		if err != nil {
			return err
		}
		if _, err := sink.CreateCandidate(ctx, candidate); err != nil {
			return fmt.Errorf("create metric definition candidate: %w", err)
		}
	}
	return nil
}

func sourceSpanLineNumbers(raw any) []int {
	spans := normalizeSourceLineSpans(raw)
	out := make([]int, 0, len(spans))
	for _, span := range spans {
		for _, part := range strings.Split(span, ":") {
			if n, err := strconv.Atoi(strings.TrimSpace(part)); err == nil && n > 0 {
				out = append(out, n)
			}
		}
	}
	return out
}

// parseTestMethodMentions is deliberately strict about names and forgiving
// about optional enrichment fields, so malformed LLM entries are skipped
// rather than producing unnamed ontology content.
func parseTestMethodMentions(payload map[string]any) []testMethodMention {
	raw, _ := payload["procedures"].([]any)
	out := make([]testMethodMention, 0, len(raw))
	for _, item := range raw {
		m, ok := item.(map[string]any)
		if !ok || strings.TrimSpace(asString(m["procedure_name"])) == "" {
			continue
		}
		metricNames := []string{}
		if values, ok := m["metric_names"].([]any); ok {
			for _, value := range values {
				metricNames = append(metricNames, asString(value))
			}
		}
		out = append(out, testMethodMention{
			ProcedureName: strings.TrimSpace(asString(m["procedure_name"])),
			Definition:    strings.TrimSpace(asString(m["definition"])),
			MetricNames:   normalizedStrings(metricNames),
			LineNumbers:   sourceSpanLineNumbers(m["source_line_spans"]),
		})
	}
	return out
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
