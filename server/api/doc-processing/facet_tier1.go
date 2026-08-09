package docprocessing

import (
	"bufio"
	"context"
	"os"
	"regexp"
	"strings"
	"unicode"

	"github.com/chendingplano/deepdoc/server/api/ontology/semrules"
)

// FacetTier1Name is facet_tier1's ProcessorSpec name (processor_plan.go).
const FacetTier1Name = "facet_tier1"

// facetTier1GatedOff reports whether an authored processor gate
// (kb.pipeline_rules, target_processor="facet_tier1") resolves to Skip.
// Absent a registered spec or any matching gate row it returns false (not
// gated off), matching ResolveProcessorGate's "processor_default" Enable
// behavior for a processor no rule targets yet -- same contract as
// classify_document's classifyDocumentGatedOff (applicability_resolver.go).
// facts is deliberately not the full binding fact set: the common case (no
// authored gate row) never evaluates a predicate at all, and the intended
// use -- an unconditional debug/test skip, predicate left empty -- needs no
// facts either.
func facetTier1GatedOff(facts semrules.FactSet) (bool, error) {
	spec := findProductionProcessorSpec(FacetTier1Name)
	if spec == nil {
		return false, nil
	}
	decision, err := ResolveProcessorGate(*spec, currentProductionPipelineGates(), facts, GateResolutionOptions{})
	if err != nil {
		return false, err
	}
	return decision.Effect == GateEffectSkip, nil
}

// Tier-1 deterministic facets (spec ADR 2026072901 S3.5 DR4, S16.1 "Facet
// tiers 1-2"): computed once from the static analyzer's already-written line
// file, no doc_metadata and no LLM call. Paths must match semrules'
// registered document.xxx fact paths (semrules/facts.go), not an ontology
// term id -- the ontology terms seeded alongside these (da:page_count etc.,
// server/cmd/ontology-seed/content.go) are the governance/documentation
// layer DR4 describes; semrules' PathSpec registry is the separate, actual
// runtime layer predicates evaluate against, the same split document.doc_kind
// (semrules) / da:doc_kind (ontology) already has. All observations use
// FacetMethodDeterministic and reuse deterministicConfidence()
// (pipeline_bindings.go) for an explicit Confidence (P5 review 2026080302
// finding P5-16 -- a nil Confidence makes an observation spuriously
// indeterminate against any min_confidence predicate). file_type is
// deliberately not produced here: document.input_doc_type already exists
// and is already populated by BuildPipelineBindingFactSet.
var (
	// numericUnitPattern matches a number immediately followed by (optional
	// space then) a common measurement unit. Deliberately a representative
	// set, not exhaustive -- this is a density heuristic, not a unit parser
	// (that job belongs to the quantity/measurement ontology).
	numericUnitPattern = regexp.MustCompile(`\d+(\.\d+)?\s*(mm|cm|km|kg|g|mg|V|kV|mV|A|mA|W|kW|Hz|kHz|MHz|GHz|°C|℃|°F|%|dB|N|Pa|kPa|MPa|s|ms|min|h|Ω|lm|lx|cd)\b`)
	// modalVerbPattern matches the normative modal verbs DR4 names, English
	// case-insensitive and Chinese literal.
	modalVerbPattern = regexp.MustCompile(`(?i)\b(shall|must)\b|应|必须`)
	// figureCaptionPattern matches a figure/table caption line, English or
	// Chinese, at the start of the (trimmed) line content.
	figureCaptionPattern = regexp.MustCompile(`(?i)^\s*(figure|fig\.?|table|图|表)\s*\d+`)
)

// ComputeTier1Facets reads recordID's line file (the artifact
// static_analyzer already wrote) and returns its deterministic facet
// observations. Takes the same DocMetadataStore interface ControlService
// already injects (InputStore) rather than a raw *sql.DB, consistent with
// this package's store-interface convention. Requires only that the line
// file exist -- no doc_metadata, no LLM. Returns (nil, nil) if the record or
// its line file can't be resolved yet (the caller should treat this as "not
// ready", not an error).
func ComputeTier1Facets(ctx context.Context, metaStore DocMetadataStore, recordID int64) ([]FacetObservation, error) {
	rec, err := metaStore.GetInputRecord(ctx, recordID)
	if err != nil {
		return nil, err
	}
	path, err := ResolveInputFilePath(LineFileGeneratedEvent{RecordID: recordID}, rec.ResultFilename, rec.ParserName, rec.StagingFilename)
	if err != nil {
		return nil, nil
	}
	lines, err := readAllLines(path)
	if err != nil {
		return nil, nil
	}
	return tier1FacetsFromLines(recordID, lines), nil
}

// readAllLines parses every line of the line file at path (parseLine's
// format, fix-size-chunking.go), skipping malformed lines the same way
// readBoundedDocumentSample does. Unlike that function, this reads the
// whole file, unbounded -- density/ratio facets need the full document, not
// an LLM-context-sized sample.
func readAllLines(path string) ([]Line, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var lines []Line
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line, err := parseLine(sc.Text())
		if err != nil {
			continue
		}
		lines = append(lines, line)
	}
	return lines, sc.Err()
}

func tier1FacetsFromLines(recordID int64, lines []Line) []FacetObservation {
	obs := make([]FacetObservation, 0, 8)
	add := func(path string, value any) {
		obs = append(obs, FacetObservation{
			RecordID:   recordID,
			Path:       path,
			Value:      value,
			Method:     FacetMethodDeterministic,
			Confidence: deterministicConfidence(),
		})
	}

	if len(lines) == 0 {
		// No content to measure -- an empty ratio would be misleading, not
		// informative, so report nothing rather than zeros.
		return obs
	}

	maxPage := 0
	headingCount := 0
	tocPresent := false
	tableLines := 0
	numericUnitLines := 0
	modalVerbLines := 0
	figureLines := 0
	var cjkRunes, latinRunes int

	for _, line := range lines {
		if line.PageNo > maxPage {
			maxPage = line.PageNo
		}
		switch {
		case line.LineType == "heading":
			headingCount++
		case line.LineType == "toc":
			tocPresent = true
		}
		if strings.Contains(line.LineType, "table") {
			tableLines++
		}
		if numericUnitPattern.MatchString(line.Content) {
			numericUnitLines++
		}
		if modalVerbPattern.MatchString(line.Content) {
			modalVerbLines++
		}
		if figureCaptionPattern.MatchString(line.Content) {
			figureLines++
		}
		for _, r := range line.Content {
			switch {
			case unicode.Is(unicode.Han, r):
				cjkRunes++
			case unicode.IsLetter(r) && r < unicode.MaxASCII:
				latinRunes++
			}
		}
	}

	total := float64(len(lines))
	add("document.page_count", maxPage)
	add("document.toc_presence", tocPresent)
	add("document.heading_count", headingCount)
	add("document.table_line_ratio", float64(tableLines)/total)
	add("document.numeric_unit_density", float64(numericUnitLines)/total)
	add("document.modal_verb_density", float64(modalVerbLines)/total)
	add("document.figure_density", float64(figureLines)/total)
	if cjkRunes+latinRunes > 0 {
		add("document.language_mix", float64(cjkRunes)/float64(cjkRunes+latinRunes))
	}
	return obs
}
