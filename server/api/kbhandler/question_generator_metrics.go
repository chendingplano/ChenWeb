package kbhandler

import (
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"strconv"

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

type questionGenerationMetric struct {
	metricReviewInputMetric
	// MetricNameEN lets the model avoid the English name too; two- and multi-hop questions may not
	// contain either name (validateMetricQuestions).
	MetricNameEN      string   `json:"metric_name_en,omitempty"`
	Context           string   `json:"context"`
	ContextLineIDs    []int    `json:"-"`
	ContextLineRanges []string `json:"context_line_ranges"`
}

type metricQuestionSet struct {
	SetNo     int                      `json:"set_no"`
	Metric    questionGenerationMetric `json:"metric"`
	Questions []questionSlot           `json:"questions"`
}

// Sample without replacement, refilling the pool only when all metrics were used.
func makeMetricQuestionSets(metrics []metricReviewInputMetric, counts questionCounts) ([]metricQuestionSet, error) {
	if len(metrics) == 0 {
		return nil, fmt.Errorf("document has no metrics")
	}
	if counts.OneHop < 1 || counts.OneHop != counts.TwoHop || counts.OneHop != counts.MultiHop {
		return nil, fmt.Errorf("each metric requires one question of each hop type")
	}
	sets := make([]metricQuestionSet, 0, counts.OneHop)
	var pool []int
	for range counts.OneHop {
		if len(pool) == 0 {
			pool = make([]int, len(metrics))
			for i := range pool {
				pool[i] = i
			}
		}
		index := questionGenerationRandomInt(len(pool))
		metric := metrics[pool[index]]
		pool = append(pool[:index], pool[index+1:]...)
		sets = append(sets, metricQuestionSet{SetNo: len(sets) + 1, Metric: questionGenerationMetric{metricReviewInputMetric: metric}, Questions: newQuestionSlots()})
	}
	return sets, nil
}

// newQuestionSlots requests one question of each hop type, each in a randomly assigned language.
func newQuestionSlots() []questionSlot {
	slots := make([]questionSlot, 0, 3)
	for _, hop := range []string{"one_hop", "two_hop", "multi_hop"} {
		lang := "en"
		if questionGenerationRandomInt(2) == 1 {
			lang = "zh-cn"
		}
		slots = append(slots, questionSlot{HopType: hop, Lang: lang})
	}
	return slots
}

// questionContextLineRanges preserves gaps in the sorted canonical line IDs.
func questionContextLineRanges(ids []int) []string {
	ranges := make([]string, 0)
	for i := 0; i < len(ids); {
		start, end := ids[i], ids[i]
		i++
		for i < len(ids) && ids[i] == end+1 {
			end = ids[i]
			i++
		}
		span := strconv.Itoa(start)
		if end != start {
			span += ":" + strconv.Itoa(end)
		}
		ranges = append(ranges, span)
	}
	return ranges
}

// buildMetricQuestionContexts fills each sampled metric's context from its source spans.
func buildMetricQuestionContexts(lines []docprocessing.Line, sets []metricQuestionSet) ([]metricQuestionSet, map[int]string, string, error) {
	targets := make([]questionContextTarget, len(sets))
	for i, set := range sets {
		targets[i] = questionContextTarget{Label: "metric " + set.Metric.MetricID, Spans: set.Metric.SourceLineSpans}
	}
	contextIDs, contexts, source, hash, err := buildQuestionTargetContexts(lines, targets)
	if err != nil {
		return nil, nil, "", err
	}
	result := append([]metricQuestionSet(nil), sets...)
	for i := range result {
		result[i].Metric.ContextLineIDs = contextIDs[i]
		result[i].Metric.ContextLineRanges = questionContextLineRanges(contextIDs[i])
		result[i].Metric.Context = contexts[i]
	}
	return result, source, hash, nil
}

func validateMetricQuestions(payload map[string]any, req questionGenerationRequest, source map[int]string, sets []metricQuestionSet, hash string) ([]generatedQuestion, error) {
	questions, err := collectMetricQuestions(payload, req, source, sets, hash, nil)
	if err != nil {
		return nil, err
	}
	return questions, nil
}

func collectMetricQuestions(payload map[string]any, req questionGenerationRequest, source map[int]string, sets []metricQuestionSet, hash string, retainedSlots map[string]bool) ([]generatedQuestion, error) {
	targets := make([]questionTargetSet, len(sets))
	for i, set := range sets {
		metric := set.Metric
		targets[i] = questionTargetSet{SetNo: set.SetNo, Slots: set.Questions, Spans: metric.SourceLineSpans, ContextIDs: metric.ContextLineIDs,
			LeakNames: []string{metric.MetricName, metric.MetricNameEN}, Noun: "metric",
			Stamp: func(q *generatedQuestion) error {
				snapshot, err := json.Marshal(metric)
				if err != nil {
					return err
				}
				q.MetricID, q.MetricSnapshot = metric.MetricID, snapshot
				q.ArtifactType, q.ArtifactID = "metric", metric.MetricID
				return nil
			}}
	}
	return collectTargetQuestions(payload, req, source, targets, hash, retainedSlots)
}

// checkMetricNameLeak rejects a two- or multi-hop question that contains its target metric's name
// (Chinese or English, case-insensitive).
func checkMetricNameLeak(q generatedQuestion, metric questionGenerationMetric) error {
	return checkTargetNameLeak(q, []string{metric.MetricName, metric.MetricNameEN}, "metric")
}
