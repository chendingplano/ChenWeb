package docprocessing

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	llmclients "github.com/chendingplano/shared/go/api/llm"
	"github.com/joho/godotenv"
)

// Opt-in live evaluation. Writes predictions to an isolated file; never changes
// production provisions, their artifacts, or benchmark runs. No gold is sent to
// the model. Run under `mise exec -- go test` to use the configured model profile.
func TestProvisionsQualityLive(t *testing.T) {
	requestPath := os.Getenv("PROVISIONS_EVAL_REQUEST")
	if requestPath == "" {
		t.Skip("set PROVISIONS_EVAL_REQUEST for a live, isolated model evaluation")
	}
	var request struct {
		RecordID int64                  `json:"record_id"`
		Source   string                 `json:"source"`
		Chunks   string                 `json:"chunks"`
		Output   string                 `json:"output"`
		UserID   string                 `json:"user_id"`
		Record   DocMetadataInputRecord `json:"record"`
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
	client := &llmclients.OpenAIJSONClient{HTTPClient: &http.Client{Timeout: 120 * time.Second}}
	p := NewProvisionsProcessor(nil, nil, client, nil)
	if p.PromptErr != nil || p.ModelErr != nil {
		t.Fatalf("prompt configuration: %v; model configuration: %v", p.PromptErr, p.ModelErr)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	ctx = withLLMUserID(withLLMRecordID(ctx, request.RecordID), request.UserID)
	result, err := p.extractProvisionsFromChunksWithLLM(ctx, chunks, request.RecordID, buildDocContextLine(request.Record))
	if err != nil {
		t.Fatal(err)
	}
	rows := p.buildProvisionOutputRows(request.RecordID, result.Provisions, p.Now(), len(chunks), 0, result.ModelName)
	body, err = json.MarshalIndent(map[string]any{
		"record_id": request.RecordID, "prompt": p.PromptRef, "model": result.ModelName,
		"review_prompt": p.ReviewPromptRef, "source_sha256": sourceHash,
		"prompt_sha256":        fmt.Sprintf("%x", sha256.Sum256([]byte(p.PromptText))),
		"review_prompt_sha256": fmt.Sprintf("%x", sha256.Sum256([]byte(p.ReviewPromptText))),
		"extract_id":           result.ExtractID, "calls": result.LLMCallCount,
		"fallback_count": result.FallbackCount, "predictions": rows,
	}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(request.Output, body, 0600); err != nil {
		t.Fatal(err)
	}
	t.Logf("record=%d model=%s prompt=%s chunks=%d predictions=%d output=%s", request.RecordID, result.ModelName, p.PromptRef, len(chunks), len(rows), request.Output)
}
