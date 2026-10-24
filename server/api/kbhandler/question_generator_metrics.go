package kbhandler

import (
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"sort"
	"strconv"
	"strings"

	docprocessing "github.com/chendingplano/deepdoc/server/api/doc-processing"
)

var questionGenerationRandomInt = rand.IntN

type questionCounts struct{ OneHop, TwoHop, MultiHop int }

func (c questionCounts) total() int { return c.OneHop + c.TwoHop + c.MultiHop }
func (c questionCounts) add(other questionCounts) questionCounts {
	return questionCounts{c.OneHop + other.OneHop, c.TwoHop + other.TwoHop, c.MultiHop + other.MultiHop}
}
func questionDocumentCounts(req questionGenerationRequest, generated questionCounts) questionCounts {
	return questionCounts{
		min(*req.OneHopCount, max(0, *req.TotalOneHopCount-generated.OneHop)),
		min(*req.TwoHopCount, max(0, *req.TotalTwoHopCount-generated.TwoHop)),
		min(*req.MultiHopCount, max(0, *req.TotalMultiHopCount-generated.MultiHop)),
	}
}

type metricQuestionTarget struct {
	HopType string `json:"hop_type"`
	Lang    string `json:"lang"`
}
type questionGenerationMetric struct {
	metricReviewInputMetric
	Context        string `json:"context"`
	ContextLineIDs []int  `json:"context_line_ids"`
}

type metricQuestionSet struct {
	SetNo     int                      `json:"set_no"`
	Metric    questionGenerationMetric `json:"metric"`
	Questions []metricQuestionTarget   `json:"questions"`
}

// Sample without replacement, refilling the pool only when all metrics were used.
func makeMetricQuestionSets(metrics []metricReviewInputMetric, counts questionCounts) ([]metricQuestionSet, error) {
	if len(metrics) == 0 {
		return nil, fmt.Errorf("document has no metrics")
	}
	sets := make([]metricQuestionSet, 0, counts.total())
	var pool []int
	for _, target := range []struct {
		hop   string
		count int
	}{{"one_hop", counts.OneHop}, {"two_hop", counts.TwoHop}, {"multi_hop", counts.MultiHop}} {
		for range target.count {
			if len(pool) == 0 {
				pool = make([]int, len(metrics))
				for i := range pool {
					pool[i] = i
				}
			}
			index := questionGenerationRandomInt(len(pool))
			metric := metrics[pool[index]]
			pool = append(pool[:index], pool[index+1:]...)
			lang := "en"
			if questionGenerationRandomInt(2) == 1 {
				lang = "zh-cn"
			}
			sets = append(sets, metricQuestionSet{SetNo: len(sets) + 1, Metric: questionGenerationMetric{metricReviewInputMetric: metric}, Questions: []metricQuestionTarget{{HopType: target.hop, Lang: lang}}})
		}
	}
	return sets, nil
}

// Build contexts from actual canonical lines, counting neighbors by position rather
// than assuming canonical IDs are contiguous. Disjoint metric spans keep their gaps out.
func buildMetricQuestionContexts(lines []docprocessing.Line, sets []metricQuestionSet) ([]metricQuestionSet, map[int]string, string, error) {
	byID := make(map[int]docprocessing.Line, len(lines))
	ids := make([]int, 0, len(lines))
	for _, line := range lines {
		if line.LineNo <= 0 || strings.TrimSpace(line.Content) == "" {
			continue
		}
		if _, ok := byID[line.LineNo]; ok {
			return nil, nil, "", fmt.Errorf("duplicate source line %d", line.LineNo)
		}
		byID[line.LineNo] = line
		ids = append(ids, line.LineNo)
	}
	sort.Ints(ids)
	available := make(map[int]string, len(ids))
	for _, id := range ids {
		available[id] = ""
	}
	selected := map[int]bool{}
	result := append([]metricQuestionSet(nil), sets...)
	for i, set := range result {
		spans := metricReviewSpansFromJSON(set.Metric.SourceLineSpans)
		if len(spans) == 0 {
			return nil, nil, "", fmt.Errorf("metric %s has no source spans", set.Metric.MetricID)
		}
		for _, span := range spans {
			match := metricReviewLineSpanPattern.FindStringSubmatch(span)
			if match == nil {
				return nil, nil, "", fmt.Errorf("metric %s has invalid source span %q", set.Metric.MetricID, span)
			}
			start, _ := strconv.Atoi(match[1])
			end := start
			if match[2] != "" {
				end, _ = strconv.Atoi(match[2])
			}
			_, startOK := byID[start]
			_, endOK := byID[end]
			if !startOK || !endOK || end < start {
				return nil, nil, "", fmt.Errorf("metric %s references missing source span %q", set.Metric.MetricID, span)
			}
		}
		evidence := metricEvidenceLineIDs(set.Metric.metricReviewInputMetric, available)
		first, last := -1, -1
		for pos, id := range ids {
			if evidence[id] {
				if first < 0 {
					first = pos
				}
				last = pos
			}
		}
		if first < 0 {
			return nil, nil, "", fmt.Errorf("metric %s has no usable source lines", set.Metric.MetricID)
		}
		contextIDs := make([]int, 0, len(evidence)+20)
		for pos, id := range ids {
			if evidence[id] || (pos >= max(0, first-10) && pos < first) || (pos > last && pos <= min(len(ids)-1, last+10)) {
				contextIDs = append(contextIDs, id)
				selected[id] = true
			}
		}
		result[i].Metric.ContextLineIDs = contextIDs
	}
	subset := make([]docprocessing.Line, 0, len(selected))
	for _, id := range ids {
		if selected[id] {
			subset = append(subset, byID[id])
		}
	}
	_, source, hash, err := buildQuestionGenerationSource(subset)
	if err != nil {
		return nil, nil, "", err
	}
	for i := range result {
		contents := make([]string, 0, len(result[i].Metric.ContextLineIDs))
		for _, id := range result[i].Metric.ContextLineIDs {
			contents = append(contents, source[id])
		}
		result[i].Metric.Context = strings.Join(contents, "\n")
		if strings.TrimSpace(result[i].Metric.Context) == "" {
			return nil, nil, "", fmt.Errorf("metric %s has empty context", result[i].Metric.MetricID)
		}
	}
	return result, source, hash, nil
}

func metricEvidenceLineIDs(metric metricReviewInputMetric, source map[int]string) map[int]bool {
	out := map[int]bool{}
	for _, span := range metricReviewSpansFromJSON(metric.SourceLineSpans) {
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

func validateMetricQuestions(payload map[string]any, req questionGenerationRequest, source map[int]string, sets []metricQuestionSet, hash string) ([]generatedQuestion, error) {
	questions, err := validateGeneratedQuestions(payload, req, source)
	if err != nil {
		return nil, err
	}
	expected := map[string]metricQuestionTarget{}
	anchors := map[int]questionGenerationMetric{}
	for _, set := range sets {
		anchors[set.SetNo] = set.Metric
		for _, target := range set.Questions {
			expected[fmt.Sprintf("%d/%s", set.SetNo, target.HopType)] = target
		}
	}
	for i := range questions {
		q := &questions[i]
		key := fmt.Sprintf("%d/%s", q.SetNo, q.HopType)
		target, ok := expected[key]
		if !ok || q.Lang != target.Lang {
			return nil, fmt.Errorf("question %d has an unexpected set, hop type, or language", i+1)
		}
		delete(expected, key)
		metric := anchors[q.SetNo]
		anchorLines := metricEvidenceLineIDs(metric.metricReviewInputMetric, source)
		allowed := make(map[int]bool, len(metric.ContextLineIDs))
		for _, id := range metric.ContextLineIDs {
			allowed[id] = true
		}
		anchored := false
		for _, e := range q.Evidence {
			if !allowed[e.LineID] {
				return nil, fmt.Errorf("question %d cites line %d outside its selected metric context", i+1, e.LineID)
			}
			if anchorLines[e.LineID] {
				anchored = true
			}
		}
		if !anchored {
			return nil, fmt.Errorf("question %d does not cite its selected metric's source lines", i+1)
		}
		q.MetricID = metric.MetricID
		q.MetricSnapshot, err = json.Marshal(metric)
		if err != nil {
			return nil, err
		}
		q.SourceSHA256 = hash
	}
	if len(expected) != 0 {
		return nil, fmt.Errorf("model omitted requested metric question sets")
	}
	return questions, nil
}
