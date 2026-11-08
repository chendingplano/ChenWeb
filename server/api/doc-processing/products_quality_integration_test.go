package docprocessing

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"testing"
	"time"

	llmclients "github.com/chendingplano/shared/go/api/llm"
	"github.com/joho/godotenv"
)

// Opt-in evaluation of the merged pass (EXTRACT_PRODUCTS_MERGED_PASS=true) on
// one record's saved chunks. It runs ProcessChunk per chunk and the merged
// pass's Step B, and writes the rows to an isolated file in kb.products shape;
// it never changes kb.products, artifacts or benchmark runs, and skips
// translation and name resolution. No gold is sent to the model.
//
// With "responses" (chunk seq -> logged provider output body), it replays
// those responses instead of calling the model, so a Step B change can be
// measured on a past run exactly. Run under `mise exec -- go test` to use the
// configured model profile and prompt.
func TestProductsQualityLive(t *testing.T) {
	requestPath := os.Getenv("PRODUCTS_EVAL_REQUEST")
	if requestPath == "" {
		t.Skip("set PRODUCTS_EVAL_REQUEST for an isolated extract_products evaluation")
	}
	var request struct {
		RecordID  int64                  `json:"record_id"`
		Source    string                 `json:"source"`
		Chunks    string                 `json:"chunks"`
		Output    string                 `json:"output"`
		UserID    string                 `json:"user_id"`
		Record    DocMetadataInputRecord `json:"record"`
		Responses map[string]string      `json:"responses"`
	}
	body, err := os.ReadFile(requestPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(body, &request); err != nil {
		t.Fatal(err)
	}
	if request.RecordID <= 0 || request.Source == "" || request.Chunks == "" || request.Output == "" || request.UserID == "" {
		t.Fatal("evaluation requires record_id, source, chunks, output and user_id")
	}
	_ = godotenv.Load(filepath.Join("..", "..", "..", ".env"))
	body, err = os.ReadFile(request.Source)
	if err != nil {
		t.Fatal(err)
	}
	sourceHash := fmt.Sprintf("%x", sha256.Sum256(body))
	lines, err := ParseInputLinesIncludingTOC(body)
	if err != nil {
		t.Fatal(err)
	}
	// The artifact layout is <root>/<group>/<record>/<file>.chunks.
	root := filepath.Dir(filepath.Dir(filepath.Dir(request.Chunks)))
	chunks, err := loadChunksFromArtifactFile(root, request.RecordID, filepath.Base(request.Chunks), lines)
	if err != nil {
		t.Fatal(err)
	}
	var client LLMJSONExtractor = &llmclients.OpenAIJSONClient{HTTPClient: &http.Client{Timeout: 120 * time.Second}}
	if len(request.Responses) > 0 {
		client = loggedProductResponses(request.Responses)
	}
	p := NewProductsProcessor(nil, nil, client, nil)
	if !p.MergedPass {
		t.Fatal("this evaluation covers the merged pass; set EXTRACT_PRODUCTS_MERGED_PASS=true")
	}
	if p.MergedPromptErr != nil || p.MentionModelErr != nil {
		t.Fatalf("merged prompt configuration: %v; model configuration: %v", p.MergedPromptErr, p.MentionModelErr)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	ctx = withDocProcessorFlags(withLLMUserID(withLLMRecordID(ctx, request.RecordID), request.UserID), true, false)
	ctx = withEventID(ctx, fmt.Sprintf("products-eval-%d", time.Now().Unix()))
	if err := p.InitChunkBatch(ctx, request.RecordID, chunks, buildDocContextLine(request.Record)); err != nil {
		t.Fatal(err)
	}
	if _, err := runConcurrent(ctx, len(chunks), len(chunks), func(c context.Context, idx int) (struct{}, error) {
		return struct{}{}, p.ProcessChunk(c, idx)
	}); err != nil {
		t.Fatal(err)
	}
	if p.batchMergedFailures > 0 {
		t.Logf("%d of %d chunks failed", p.batchMergedFailures, len(chunks))
	}
	rows := dedupeFinalProductRows(settleMergedChunkRows(p.batchMergedRows, p.batchBlocks))
	model := firstNonEmptyTrimmed(p.batchMergedModel, p.MergedModelName)
	out := p.buildProductOutputRows(rows, p.Now(), len(chunks), model)
	for i := range out {
		out[i]["product_rel_id"] = fmt.Sprintf("%d_prd_%d", request.RecordID, i+1)
	}
	body, err = json.MarshalIndent(map[string]any{
		"record_id": request.RecordID, "prompt": p.MergedPromptRef, "model": model,
		"reasoning": p.Reasoning, "replayed": len(request.Responses) > 0, "source_sha256": sourceHash,
		"prompt_sha256": fmt.Sprintf("%x", sha256.Sum256([]byte(p.MergedPromptText))),
		"chunks":        len(chunks), "failed_chunks": p.batchMergedFailures, "predictions": out,
	}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(request.Output, body, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Logf("wrote %d rows to %s", len(out), request.Output)
}

// loggedProductResponses answers each merged call with the message content of
// a logged provider output body, chosen by the call's "Block index: N" suffix.
type loggedProductResponses map[string]string

var productBlockIndexSuffix = regexp.MustCompile(`Block index: ([0-9]+)\s*$`)

func (r loggedProductResponses) ExtractJSON(_ context.Context, in llmclients.JSONExtractionInput) (map[string]any, error) {
	match := productBlockIndexSuffix.FindStringSubmatch(in.PromptText)
	if match == nil {
		return nil, fmt.Errorf("no block index in the task prompt")
	}
	seq, _ := strconv.Atoi(match[1])
	path, ok := r[strconv.Itoa(seq)]
	if !ok {
		return map[string]any{"v": 2, "p": []any{}}, nil
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var provider struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(body, &provider); err != nil || len(provider.Choices) == 0 {
		return nil, fmt.Errorf("logged response %s: %v", path, err)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(provider.Choices[0].Message.Content), &payload); err != nil {
		return nil, fmt.Errorf("logged response %s content: %w", path, err)
	}
	return payload, nil
}
