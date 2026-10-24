package kbhandler

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	docprocessing "github.com/chendingplano/deepdoc/server/api/doc-processing"
)

const (
	questionGenerationMaxInputChars = 600000
	questionGenerationTimeout       = 10 * time.Minute
	questionGenerationBatchTimeout  = time.Hour
	questionGenerationPrompt        = "prompt-question-generator-search-metrics-v1.md"
)

type questionGenerationRequest struct {
	MetricsPerDocument *int   `json:"metrics_per_document"`
	TotalMetrics       *int   `json:"total_metrics"`
	InputRecordID      int64  `json:"input_record_id"`
	Purpose            string `json:"purpose"`
	Description        string `json:"description"`
	OneHopCount        *int   `json:"one_hop_count"`
	TwoHopCount        *int   `json:"two_hop_count"`
	MultiHopCount      *int   `json:"multi_hop_count"`
	TotalOneHopCount   *int   `json:"total_one_hop_count"`
	TotalTwoHopCount   *int   `json:"total_two_hop_count"`
	TotalMultiHopCount *int   `json:"total_multi_hop_count"`
	Lang               string `json:"lang"`
}

func (r *questionGenerationRequest) normalize() error {
	r.Purpose, r.Description = strings.TrimSpace(r.Purpose), strings.TrimSpace(r.Description)
	if r.InputRecordID < 0 {
		return fmt.Errorf("input_record_id must be positive or omitted")
	}
	if _, err := questionPurposePrompt(r.Purpose); err != nil {
		return err
	}
	if utf8.RuneCountInString(r.Description) > 10000 {
		return fmt.Errorf("description is too long")
	}
	// Question languages are independently chosen by the server, rather than
	// following the UI locale. Retain the request field for old clients.
	r.Lang = "mixed"
	// Keep the existing per-hop storage fields balanced for triplet runs. Legacy
	// clients may submit equal hop counts; unequal counts cannot describe triplets.
	resolve := func(value *int, legacy []*int, maximum int, name string) (int, error) {
		n := 10
		if value != nil {
			n = *value
		} else {
			found := false
			for _, count := range legacy {
				if count == nil {
					continue
				}
				if found && n != *count {
					return 0, fmt.Errorf("%s: legacy hop counts must be equal; each metric generates three questions", name)
				}
				n, found = *count, true
			}
		}
		if n < 1 || n > maximum {
			return 0, fmt.Errorf("%s must be between 1 and %d", name, maximum)
		}
		return n, nil
	}
	perDocument, err := resolve(r.MetricsPerDocument, []*int{r.OneHopCount, r.TwoHopCount, r.MultiHopCount}, 50, "metrics_per_document")
	if err != nil {
		return err
	}
	total := perDocument
	if r.InputRecordID == 0 {
		total, err = resolve(r.TotalMetrics, []*int{r.TotalOneHopCount, r.TotalTwoHopCount, r.TotalMultiHopCount}, 1000, "total_metrics")
		if err != nil {
			return err
		}
	}
	r.MetricsPerDocument, r.TotalMetrics = &perDocument, &total
	for _, field := range []**int{&r.OneHopCount, &r.TwoHopCount, &r.MultiHopCount} {
		n := perDocument
		*field = &n
	}
	for _, field := range []**int{&r.TotalOneHopCount, &r.TotalTwoHopCount, &r.TotalMultiHopCount} {
		n := total
		*field = &n
	}
	return nil
}

func questionPurposePrompt(purpose string) (string, error) {
	switch purpose {
	case "search_metrics":
		return envOrDefault("PROMPT_GENERATE_METRIC_QUESTIONS", questionGenerationPrompt), nil
	default:
		return "", fmt.Errorf("unsupported purpose: select Search Metrics")
	}
}

type questionGenerationRun struct {
	ID                     int64      `json:"id"`
	InputRecordID          int64      `json:"input_record_id"`
	Purpose                string     `json:"purpose"`
	Description            string     `json:"description"`
	OneHopCount            int        `json:"one_hop_count"`
	TwoHopCount            int        `json:"two_hop_count"`
	MultiHopCount          int        `json:"multi_hop_count"`
	TotalOneHopCount       int        `json:"total_one_hop_count"`
	TotalTwoHopCount       int        `json:"total_two_hop_count"`
	TotalMultiHopCount     int        `json:"total_multi_hop_count"`
	GeneratedOneHopCount   int        `json:"generated_one_hop_count"`
	GeneratedTwoHopCount   int        `json:"generated_two_hop_count"`
	GeneratedMultiHopCount int        `json:"generated_multi_hop_count"`
	DocumentsProcessed     int        `json:"documents_processed"`
	ExpiresAt              time.Time  `json:"expires_at"`
	Lang                   string     `json:"lang"`
	Status                 string     `json:"status"`
	ErrorMsg               string     `json:"error_msg"`
	ModelName              string     `json:"model_name"`
	PromptName             string     `json:"prompt_name"`
	SourceSHA256           string     `json:"source_sha256"`
	CreatedAt              time.Time  `json:"created_at"`
	FinishedAt             *time.Time `json:"finished_at,omitempty"`
}

type questionEvidence struct {
	LineID int    `json:"line_id"`
	Text   string `json:"text"`
}

type generatedQuestion struct {
	ID             int64              `json:"id"`
	RunID          int64              `json:"run_id"`
	InputRecordID  int64              `json:"input_record_id"`
	HopType        string             `json:"hop_type"`
	Question       string             `json:"question"`
	Answer         string             `json:"answer"`
	ReasoningSteps []string           `json:"reasoning_steps"`
	StepLineIDs    [][]int            `json:"step_line_ids"`
	Evidence       []questionEvidence `json:"evidence"`
	SetNo          int                `json:"set_no"`
	Lang           string             `json:"lang"`
	MetricID       string             `json:"metric_id"`
	MetricSnapshot json.RawMessage    `json:"metric_snapshot,omitempty"`
	SourceSHA256   string             `json:"source_sha256"`
}

// buildQuestionGenerationSource uses the entire canonical file and preserves its
// line IDs. Reject oversized input rather than silently dropping document sections.
func buildQuestionGenerationSource(lines []docprocessing.Line) (string, map[int]string, string, error) {
	source := make(map[int]string, len(lines))
	numbered := make([]questionEvidence, 0, len(lines))
	chars := 0
	for _, line := range lines {
		if line.LineNo <= 0 || strings.TrimSpace(line.Content) == "" {
			continue
		}
		if _, ok := source[line.LineNo]; ok {
			return "", nil, "", fmt.Errorf("duplicate source line %d", line.LineNo)
		}
		text := docprocessing.LLMLineContent(line.LineType, line.LineNo, line.Content)
		chars += utf8.RuneCountInString(text)
		if chars > questionGenerationMaxInputChars {
			return "", nil, "", fmt.Errorf("document too large (limit %d characters)", questionGenerationMaxInputChars)
		}
		source[line.LineNo] = text
		numbered = append(numbered, questionEvidence{LineID: line.LineNo, Text: text})
	}
	if len(numbered) == 0 {
		return "", nil, "", fmt.Errorf("document has no usable source lines")
	}
	raw, err := json.Marshal(numbered)
	if err != nil {
		return "", nil, "", err
	}
	if utf8.RuneCount(raw) > questionGenerationMaxInputChars {
		return "", nil, "", fmt.Errorf("document too large after formatting")
	}
	sum := sha256.Sum256(raw)
	return string(raw), source, hex.EncodeToString(sum[:]), nil
}

func validateGeneratedQuestions(payload map[string]any, req questionGenerationRequest, source map[int]string) ([]generatedQuestion, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	var output struct {
		Questions []struct {
			HopType        string   `json:"hop_type"`
			Question       string   `json:"question"`
			Answer         string   `json:"answer"`
			ReasoningSteps []string `json:"reasoning_steps"`
			LineIDs        []int    `json:"line_ids"`
			StepLineIDs    [][]int  `json:"step_line_ids"`
			SetNo          int      `json:"set_no"`
			Lang           string   `json:"lang"`
		} `json:"questions"`
	}
	if err := json.Unmarshal(raw, &output); err != nil {
		return nil, fmt.Errorf("invalid question output: %w", err)
	}
	expected := map[string]int{"one_hop": *req.OneHopCount, "two_hop": *req.TwoHopCount, "multi_hop": *req.MultiHopCount}
	minima := map[string]int{"one_hop": 1, "two_hop": 2, "multi_hop": 3}
	if len(output.Questions) != *req.OneHopCount+*req.TwoHopCount+*req.MultiHopCount {
		return nil, fmt.Errorf("model returned the wrong total question count")
	}
	counts := map[string]int{}
	seen := map[string]bool{}
	questions := make([]generatedQuestion, 0, len(output.Questions))
	for i, q := range output.Questions {
		minimum, ok := minima[q.HopType]
		if !ok {
			return nil, fmt.Errorf("question %d has an unknown hop_type", i+1)
		}
		q.Question, q.Answer = strings.TrimSpace(q.Question), strings.TrimSpace(q.Answer)
		key := strings.ToLower(strings.Join(strings.Fields(q.Question), " "))
		if key == "" || q.Answer == "" || seen[key] {
			return nil, fmt.Errorf("question %d is empty or duplicated, or has no answer", i+1)
		}
		seen[key] = true
		if len(q.ReasoningSteps) < minimum {
			return nil, fmt.Errorf("question %d (%s) has %d reasoning steps; requires at least %d", i+1, q.HopType, len(q.ReasoningSteps), minimum)
		}
		if len(q.StepLineIDs) != len(q.ReasoningSteps) {
			return nil, fmt.Errorf("question %d (%s) has %d reasoning steps but %d citation arrays; provide one step_line_ids array per reasoning step", i+1, q.HopType, len(q.ReasoningSteps), len(q.StepLineIDs))
		}
		cited := map[int]bool{}
		evidence := make([]questionEvidence, 0, len(q.LineIDs))
		for _, id := range q.LineIDs {
			text, exists := source[id]
			if !exists || cited[id] {
				return nil, fmt.Errorf("question %d cites an unknown or repeated line %d", i+1, id)
			}
			cited[id] = true
			evidence = append(evidence, questionEvidence{LineID: id, Text: text})
		}
		// Several independently evidenced table facts can share one canonical line.
		if len(cited) == 0 {
			return nil, fmt.Errorf("question %d lacks source evidence", i+1)
		}
		used := map[int]bool{}
		for step, text := range q.ReasoningSteps {
			q.ReasoningSteps[step] = strings.TrimSpace(text)
			if q.ReasoningSteps[step] == "" || len(q.StepLineIDs[step]) == 0 {
				return nil, fmt.Errorf("question %d has an unsupported reasoning step", i+1)
			}
			for _, id := range q.StepLineIDs[step] {
				if !cited[id] {
					return nil, fmt.Errorf("question %d step cites a line outside its evidence", i+1)
				}
				used[id] = true
			}
		}
		if len(used) != len(cited) {
			return nil, fmt.Errorf("question %d has evidence unused by its reasoning", i+1)
		}
		counts[q.HopType]++
		questions = append(questions, generatedQuestion{InputRecordID: req.InputRecordID, HopType: q.HopType, Question: q.Question, Answer: q.Answer, ReasoningSteps: q.ReasoningSteps, StepLineIDs: q.StepLineIDs, Evidence: evidence, SetNo: q.SetNo, Lang: q.Lang})
	}
	for hop, n := range expected {
		if counts[hop] != n {
			return nil, fmt.Errorf("model returned %d %s questions; expected %d", counts[hop], hop, n)
		}
	}
	return questions, nil
}
