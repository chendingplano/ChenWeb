package kbhandler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	docprocessing "github.com/chendingplano/deepdoc/server/api/doc-processing"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	llmclients "github.com/chendingplano/shared/go/api/llm"
)

// The question core shared by metric targets (kb.metrics) and search registry targets
// (kb.search_artifacts): source contexts from line spans, per-target validation, and the model call
// with one correction attempt (openspec question-generator-artifact-types, design D4).

// questionLineIndex holds a document's non-empty canonical lines in ID order.
type questionLineIndex struct {
	byID map[int]docprocessing.Line
	ids  []int
}

func newQuestionLineIndex(lines []docprocessing.Line) (questionLineIndex, error) {
	index := questionLineIndex{byID: make(map[int]docprocessing.Line, len(lines)), ids: make([]int, 0, len(lines))}
	for _, line := range lines {
		if line.LineNo <= 0 || strings.TrimSpace(line.Content) == "" {
			continue
		}
		if _, ok := index.byID[line.LineNo]; ok {
			return index, fmt.Errorf("duplicate source line %d", line.LineNo)
		}
		index.byID[line.LineNo] = line
		index.ids = append(index.ids, line.LineNo)
	}
	sort.Ints(index.ids)
	return index, nil
}

// contextIDs returns a target's context: its span lines plus up to ten actual lines before the first
// and after the last, counted by position because canonical IDs need not be contiguous. Gaps between
// disjoint spans stay out. label names the target in errors ("metric 416_mtc_7").
func (x questionLineIndex) contextIDs(label string, spans json.RawMessage) ([]int, error) {
	parts := metricReviewSpansFromJSON(spans)
	if len(parts) == 0 {
		return nil, fmt.Errorf("%s has no source spans", label)
	}
	evidence := map[int]bool{}
	first, last := len(x.ids), -1
	for _, span := range parts {
		match := metricReviewLineSpanPattern.FindStringSubmatch(span)
		if match == nil {
			return nil, fmt.Errorf("%s has invalid source span %q", label, span)
		}
		start, _ := strconv.Atoi(match[1])
		end := start
		if match[2] != "" {
			end, _ = strconv.Atoi(match[2])
		}
		_, startOK := x.byID[start]
		_, endOK := x.byID[end]
		if !startOK || !endOK || end < start {
			return nil, fmt.Errorf("%s references missing source span %q", label, span)
		}
		// Walk real IDs from the span start; never expand an untrusted or very large range.
		for pos := sort.SearchInts(x.ids, start); pos < len(x.ids) && x.ids[pos] <= end; pos++ {
			evidence[x.ids[pos]] = true
			first, last = min(first, pos), max(last, pos)
		}
	}
	ids := make([]int, 0, len(evidence)+20)
	for pos := max(0, first-10); pos <= min(len(x.ids)-1, last+10); pos++ {
		if id := x.ids[pos]; evidence[id] || pos < first || pos > last {
			ids = append(ids, id)
		}
	}
	return ids, nil
}

// contextChars is the length of the context text sent to the model: the formatted lines joined by
// newlines, as buildQuestionTargetContexts produces it.
func (x questionLineIndex) contextChars(ids []int) int {
	chars := max(0, len(ids)-1)
	for _, id := range ids {
		line := x.byID[id]
		chars += utf8.RuneCountInString(docprocessing.LLMLineContent(line.LineType, line.LineNo, line.Content))
	}
	return chars
}

type questionContextTarget struct {
	Label string
	Spans json.RawMessage
}

// buildQuestionTargetContexts returns each target's context line IDs and text, plus one source map
// and hash over the union of all contexts. Only these lines are formatted and sent to the model.
func buildQuestionTargetContexts(lines []docprocessing.Line, targets []questionContextTarget) ([][]int, []string, map[int]string, string, error) {
	index, err := newQuestionLineIndex(lines)
	if err != nil {
		return nil, nil, nil, "", err
	}
	contextIDs := make([][]int, len(targets))
	selected := map[int]bool{}
	for i, target := range targets {
		if contextIDs[i], err = index.contextIDs(target.Label, target.Spans); err != nil {
			return nil, nil, nil, "", err
		}
		for _, id := range contextIDs[i] {
			selected[id] = true
		}
	}
	subset := make([]docprocessing.Line, 0, len(selected))
	for _, id := range index.ids {
		if selected[id] {
			subset = append(subset, index.byID[id])
		}
	}
	_, source, hash, err := buildQuestionGenerationSource(subset)
	if err != nil {
		return nil, nil, nil, "", err
	}
	contexts := make([]string, len(targets))
	for i, ids := range contextIDs {
		contents := make([]string, 0, len(ids))
		for _, id := range ids {
			contents = append(contents, source[id])
		}
		contexts[i] = strings.Join(contents, "\n")
		if strings.TrimSpace(contexts[i]) == "" {
			return nil, nil, nil, "", fmt.Errorf("%s has empty context", targets[i].Label)
		}
	}
	return contextIDs, contexts, source, hash, nil
}

// spanEvidenceLineIDs returns the source IDs that fall inside the spans.
func spanEvidenceLineIDs(spans json.RawMessage, source map[int]string) map[int]bool {
	out := map[int]bool{}
	for _, span := range metricReviewSpansFromJSON(spans) {
		match := metricReviewLineSpanPattern.FindStringSubmatch(span)
		if match == nil {
			continue
		}
		start, _ := strconv.Atoi(match[1])
		end := start
		if match[2] != "" {
			end, _ = strconv.Atoi(match[2])
		}
		// Iterate real source IDs, never expand an untrusted or very large range.
		for id := range source {
			if id >= start && id <= end {
				out[id] = true
			}
		}
	}
	return out
}

// questionSlot is one requested question of a set: its hop type and assigned language.
type questionSlot struct {
	HopType string `json:"hop_type"`
	Lang    string `json:"lang"`
}

// questionTargetSet is one sampled target as the shared validator sees it. Noun names the target
// kind in validation messages ("metric", "provision"); AnchorHint, when set, tells a correction
// which lines the target owns; Stamp copies the target's identity onto an accepted question.
type questionTargetSet struct {
	SetNo      int
	Slots      []questionSlot
	Spans      json.RawMessage
	ContextIDs []int
	LeakNames  []string
	Noun       string
	AnchorHint string
	Stamp      func(*generatedQuestion) error
}

func collectTargetQuestions(payload map[string]any, req questionGenerationRequest, source map[int]string, sets []questionTargetSet, hash string, retainedSlots map[string]bool) ([]generatedQuestion, error) {
	questions, positions, structuralErr := collectGeneratedQuestions(payload, req, source, false)
	validationErrors := []error{structuralErr}
	accepted := make([]generatedQuestion, 0, len(questions))
	seenText := map[string]int{}
	expected := map[string]questionSlot{}
	anchors := map[int]questionTargetSet{}
	noun := "" // the sets' common noun, or "target" when they differ (Mix)
	for _, set := range sets {
		anchors[set.SetNo] = set
		if noun == "" {
			noun = set.Noun
		} else if noun != set.Noun {
			noun = "target"
		}
		for _, slot := range set.Slots {
			key := fmt.Sprintf("%d/%s", set.SetNo, slot.HopType)
			if !retainedSlots[key] {
				expected[key] = slot
			}
		}
	}
	for i := range questions {
		q := &questions[i]
		// A correction cannot replace an earlier accepted slot. Ignore its replacement
		// before checking duplicate text, so it cannot disqualify an unresolved slot.
		if retainedSlots[fmt.Sprintf("%d/%s", q.SetNo, q.HopType)] {
			continue
		}
		err := func() error {
			key := fmt.Sprintf("%d/%s", q.SetNo, q.HopType)
			slot, ok := expected[key]
			if !ok || q.Lang != slot.Lang {
				return fmt.Errorf("question %d has an unexpected set, hop type, or language", positions[i])
			}
			set := anchors[q.SetNo]
			anchorLines := spanEvidenceLineIDs(set.Spans, source)
			allowed := make(map[int]bool, len(set.ContextIDs))
			for _, id := range set.ContextIDs {
				allowed[id] = true
			}
			anchored := false
			for _, e := range q.Evidence {
				if !allowed[e.LineID] {
					return fmt.Errorf("question %d cites line %d outside its selected %s context", positions[i], e.LineID, set.Noun)
				}
				if anchorLines[e.LineID] {
					anchored = true
				}
			}
			if !anchored {
				return fmt.Errorf("question %d does not cite its selected %s's source lines%s", positions[i], set.Noun, set.AnchorHint)
			}
			if err := checkTargetNameLeak(*q, set.LeakNames, set.Noun); err != nil {
				return fmt.Errorf("question %d %w", positions[i], err)
			}
			text := strings.ToLower(strings.Join(strings.Fields(q.Question), " "))
			if previous := seenText[text]; previous != 0 {
				return fmt.Errorf("question %d duplicates question %d (ignoring case and whitespace); provide a distinct question", positions[i], previous)
			}
			if err := set.Stamp(q); err != nil {
				return err
			}
			q.SourceSHA256 = hash
			delete(expected, key)
			accepted = append(accepted, *q)
			seenText[text] = positions[i]
			return nil
		}()
		if err != nil {
			validationErrors = append(validationErrors, err)
		}
	}
	if len(expected) != 0 {
		if noun == "" {
			noun = "target"
		}
		validationErrors = append(validationErrors, fmt.Errorf("%d requested %s questions are missing or rejected", len(expected), noun))
	}
	return accepted, errors.Join(validationErrors...)
}

// checkTargetNameLeak rejects a two- or multi-hop question that contains one of its target's names
// (case-insensitive). Those questions must describe the need, so a searcher has to work out which
// target to look up. Names under two characters are too generic to check.
func checkTargetNameLeak(q generatedQuestion, names []string, noun string) error {
	if q.HopType == "one_hop" {
		return nil
	}
	question := strings.ToLower(q.Question)
	for _, name := range names {
		name = strings.ToLower(strings.TrimSpace(name))
		if utf8.RuneCountInString(name) >= 2 && strings.Contains(question, name) {
			return fmt.Errorf("(%s) contains its target %s name %q; describe the need or situation instead of naming the %s", q.HopType, noun, name, noun)
		}
	}
	return nil
}

// questionModelRequest is the model input of one document. Field order keeps document metadata
// ahead of the sets in both the model input and the archived LLM request body.
type questionModelRequest struct {
	DocumentMetadata  questionDocumentMetadata `json:"document_metadata"`
	RunID             int64                    `json:"run_id"`
	InputRecordID     int64                    `json:"input_record_id"`
	Purpose           string                   `json:"purpose"`
	Description       string                   `json:"description"`
	Sets              any                      `json:"sets"`
	PreviousQuestions []string                 `json:"previous_questions"`
	ValidationError   string                   `json:"validation_error,omitempty"`
	PreviousResponse  map[string]any           `json:"previous_response,omitempty"`
}

type questionModelCall struct {
	RunID      int64
	Noun       string // "metric" or "target", for messages
	Wanted     int    // questions requested across all sets
	Collect    func(payload map[string]any, retainedSlots map[string]bool) ([]generatedQuestion, error)
	Extraction llmclients.JSONExtractionInput // call template; InputText is filled per attempt
}

// runQuestionModel keeps every valid question across attempts, saves accepted questions before the
// single correction attempt, and gives malformed output one correction without changing the sampled
// targets, language assignments or evidence. Validation is never relaxed.
func runQuestionModel(ctx context.Context, client metricJSONExtractor, request questionModelRequest, call questionModelCall, logger ApiTypes.JimoLogger, persistPartial ...func([]generatedQuestion) error) ([]generatedQuestion, error) {
	accepted := []generatedQuestion{}
	acceptedSlots := map[string]bool{}
	acceptedText := map[string]bool{}
	var lastValidationErr error
	for attempt := 0; ; attempt++ {
		if err := ctx.Err(); err != nil {
			return accepted, errors.Join(lastValidationErr, err)
		}
		input, err := json.Marshal(request)
		if err != nil {
			return accepted, fmt.Errorf("cannot format %s question input", call.Noun)
		}
		if utf8.RuneCount(input) > questionGenerationMaxInputChars {
			return accepted, fmt.Errorf("document and %s context too large after formatting", call.Noun)
		}
		extraction := call.Extraction
		extraction.InputText = string(input)
		payload, err := client.ExtractJSON(ctx, extraction)
		if err != nil {
			logger.Error(call.Noun+" question model call failed", "run_id", call.RunID, "record_id", request.InputRecordID, "error", err)
			return accepted, errors.Join(lastValidationErr, fmt.Errorf("question generation model request failed or timed out"))
		}
		questions, validationErr := call.Collect(payload, acceptedSlots)
		for _, q := range questions {
			key := fmt.Sprintf("%d/%s", q.SetNo, q.HopType)
			if acceptedSlots[key] {
				continue
			}
			text := strings.ToLower(strings.Join(strings.Fields(q.Question), " "))
			if acceptedText[text] {
				validationErr = errors.Join(validationErr, fmt.Errorf("question for set %d (%s) duplicates an accepted question", q.SetNo, q.HopType))
				continue
			}
			accepted = append(accepted, q)
			acceptedSlots[key], acceptedText[text] = true, true
		}
		if len(accepted) == call.Wanted {
			return accepted, nil
		}
		lastValidationErr = validationErr
		// Commit successful rows before a correction call can fail, time out, or be interrupted.
		if attempt == 0 && len(accepted) > 0 && len(persistPartial) > 0 {
			if err := persistPartial[0](accepted); err != nil {
				return accepted, err
			}
		}
		if attempt == 1 {
			return accepted, validationErr
		}
		logger.Warn(call.Noun+" question output invalid; requesting correction", "run_id", call.RunID, "record_id", request.InputRecordID, "accepted_questions", len(accepted), "error", validationErr)
		request.ValidationError, request.PreviousResponse = validationErr.Error(), payload
	}
}
