package docprocessing

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

//go:embed sumy_worker.py
var sumyWorkerSource string

type SumyConfig struct {
	Python        string
	Algorithm     string
	SentenceCount int
	Timeout       time.Duration
}

func sumyConfigFromEnv() (SumyConfig, error) {
	c := SumyConfig{Python: strings.TrimSpace(os.Getenv("SUMY_PYTHON")), Algorithm: strings.ToLower(strings.TrimSpace(os.Getenv("EXTRACT_SUMMARIES_ALGORITHM"))), SentenceCount: 3, Timeout: 2 * time.Minute}
	if c.Python == "" {
		c.Python = "python3"
	}
	if c.Algorithm == "" {
		c.Algorithm = "lexrank"
	}
	if !validSumyAlgorithm(c.Algorithm) {
		return c, fmt.Errorf("unsupported EXTRACT_SUMMARIES_ALGORITHM %q", c.Algorithm)
	}
	if raw := strings.TrimSpace(os.Getenv("EXTRACT_SUMMARIES_SENTENCES")); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > 100 {
			return c, fmt.Errorf("EXTRACT_SUMMARIES_SENTENCES must be 1..100")
		}
		c.SentenceCount = n
	}
	if raw := strings.TrimSpace(os.Getenv("EXTRACT_SUMMARIES_TIMEOUT")); raw != "" {
		d, err := time.ParseDuration(raw)
		if err != nil || d <= 0 {
			return c, fmt.Errorf("EXTRACT_SUMMARIES_TIMEOUT must be a positive duration")
		}
		c.Timeout = d
	}
	return c, nil
}

func validSumyAlgorithm(algorithm string) bool {
	switch algorithm {
	case "lexrank", "textrank", "lsa", "luhn", "sumbasic":
		return true
	}
	return false
}

type SumyLine struct {
	LineNumber int    `json:"line_number"`
	Content    string `json:"content"`
	LineType   string `json:"line_type"`
}
type SumyChunk struct {
	SeqNo int        `json:"seq_no"`
	Lines []SumyLine `json:"lines"`
}
type SumyRequest struct {
	Algorithm     string      `json:"algorithm"`
	SentenceCount int         `json:"sentence_count"`
	Language      string      `json:"language"`
	Chunks        []SumyChunk `json:"chunks"`
}
type ExtractSummarySpan struct {
	Line  int `json:"line"`
	Start int `json:"start"`
	End   int `json:"end"`
}
type ExtractSummarySentence struct {
	Text  string               `json:"text"`
	Spans []ExtractSummarySpan `json:"spans"`
}
type ExtractSummary struct {
	SeqNo           int                      `json:"seq_no"`
	Language        string                   `json:"language"`
	Summary         string                   `json:"summary_text"`
	Sentences       []ExtractSummarySentence `json:"sentences"`
	RankingFallback string                   `json:"ranking_fallback,omitempty"`
	ConfigJSON      string                   `json:"-"`
}
type SumyResponse struct {
	Algorithm string            `json:"algorithm"`
	Versions  map[string]string `json:"versions"`
	Summaries []ExtractSummary  `json:"summaries"`
}
type SumyRunner interface {
	Extract(context.Context, SumyRequest) (SumyResponse, error)
}
type PythonSumyRunner struct{ Config SumyConfig }

func (r PythonSumyRunner) Extract(ctx context.Context, request SumyRequest) (SumyResponse, error) {
	body, err := json.Marshal(request)
	if err != nil {
		return SumyResponse{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, r.Config.Timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, r.Config.Python, "-c", sumyWorkerSource)
	cmd.Stdin = bytes.NewReader(body)
	var output, stderr bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return SumyResponse{}, fmt.Errorf("Sumy worker interrupted: %w", ctx.Err())
		}
		return SumyResponse{}, fmt.Errorf("Sumy worker failed (provision python/sumy dependencies and punkt_tab): %w: %.2000s", err, stderr.String())
	}
	var response SumyResponse
	if err := json.Unmarshal(output.Bytes(), &response); err != nil {
		return response, fmt.Errorf("decode Sumy worker response: %w", err)
	}
	return response, nil
}
