package kbhandler

import (
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"strconv"
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
type metricQuestionSet struct {
	SetNo     int                     `json:"set_no"`
	Metric    metricReviewInputMetric `json:"metric"`
	Questions []metricQuestionTarget  `json:"questions"`
}

// Each set chooses its own metric; reuse is allowed when a document has fewer
// metrics than requested sets. The model must still produce distinct questions.
func makeMetricQuestionSets(metrics []metricReviewInputMetric, counts questionCounts) ([]metricQuestionSet, error) {
	if len(metrics) == 0 {
		return nil, fmt.Errorf("document has no metrics")
	}
	setCount := max(counts.OneHop, counts.TwoHop, counts.MultiHop)
	sets := make([]metricQuestionSet, 0, setCount)
	for i := 0; i < setCount; i++ {
		set := metricQuestionSet{SetNo: i + 1, Metric: metrics[questionGenerationRandomInt(len(metrics))]}
		for _, target := range []struct {
			hop   string
			count int
		}{{"one_hop", counts.OneHop}, {"two_hop", counts.TwoHop}, {"multi_hop", counts.MultiHop}} {
			if i < target.count {
				lang := "en"
				if questionGenerationRandomInt(2) == 1 {
					lang = "zh-cn"
				}
				set.Questions = append(set.Questions, metricQuestionTarget{HopType: target.hop, Lang: lang})
			}
		}
		sets = append(sets, set)
	}
	return sets, nil
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
	anchors := map[int]metricReviewInputMetric{}
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
		anchorLines := metricEvidenceLineIDs(metric, source)
		anchored := false
		for _, e := range q.Evidence {
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
