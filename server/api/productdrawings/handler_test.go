package productdrawings

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
)

func TestPendingDrawingCanBeKept(t *testing.T) {
	promptDir := t.TempDir()
	outputDir := t.TempDir()
	pendingDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(promptDir, promptFileName), []byte("prompt"), 0o644); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"data":[{"b64_json":"iVBORw0KGgo="}]}`)
	}))
	defer server.Close()
	t.Setenv("PROMPTS_DIR", promptDir)
	t.Setenv("PRODUCT_DRAWINGS_DIR", outputDir)
	t.Setenv("PRODUCT_DRAWINGS_PENDING_DIR", pendingDir)
	t.Setenv("IMAGE_GEN_BASE_URL", server.URL)
	t.Setenv("IMAGE_GEN_API_KEY", "test-key")
	t.Setenv("IMAGE_GEN_MODEL", "test-model")

	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/product-drawings/generate", nil)
	rec := httptest.NewRecorder()
	if err := GeneratePending(e.NewContext(req, rec)); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusCreated {
		t.Fatalf("generate status = %d: %s", rec.Code, rec.Body.String())
	}
	var pending PendingResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &pending); err != nil {
		t.Fatal(err)
	}
	if pending.Token == "" || pending.Prompt != "prompt" {
		t.Fatalf("unexpected pending response: %+v", pending)
	}
	entries, err := os.ReadDir(outputDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("permanent files before keep = %d, want 0", len(entries))
	}
	keepReq := httptest.NewRequest(http.MethodPost, "/api/v1/product-drawings/pending/"+pending.Token+"/keep", nil)
	keepReq = keepReq.WithContext(context.Background())
	keepRec := httptest.NewRecorder()
	keepCtx := e.NewContext(keepReq, keepRec)
	keepCtx.SetPath("/api/v1/product-drawings/pending/:token/keep")
	keepCtx.SetParamNames("token")
	keepCtx.SetParamValues(pending.Token)
	if err := KeepPending(keepCtx); err != nil {
		t.Fatal(err)
	}
	if keepRec.Code != http.StatusOK {
		t.Fatalf("keep status = %d: %s", keepRec.Code, keepRec.Body.String())
	}
	entries, err = os.ReadDir(outputDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("permanent files after keep = %d, want 1", len(entries))
	}
}

type fakeProvider struct {
	model  string
	prompt string
	err    error
}

func (f *fakeProvider) Generate(_ context.Context, model, prompt string) ([]byte, error) {
	f.model = model
	f.prompt = prompt
	if f.err != nil {
		return nil, f.err
	}
	return []byte("\x89PNG\r\n\x1a\n generated"), nil
}

func TestGenerateDefaultsToVentilatorAndReturnsPNG(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	promptDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(promptDir, promptFileName), []byte("canonical ventilator prompt"), 0o644); err != nil {
		t.Fatal(err)
	}
	provider := &fakeProvider{}
	h := NewHandler(Config{
		OutputDir: tmp,
		PromptDir: promptDir,
		Model:     "openai-image-2.5",
		Provider:  provider,
	})

	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/product-drawings", nil)
	rec := httptest.NewRecorder()
	if err := h(e.NewContext(req, rec)); err != nil {
		t.Fatal(err)
	}

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	if got := rec.Header().Get(echo.HeaderContentType); !strings.HasPrefix(got, "image/png") {
		t.Fatalf("content type = %q, want image/png", got)
	}
	if got := rec.Header().Get("X-Drawing-Filename"); got == "" {
		t.Fatal("missing X-Drawing-Filename")
	}
	if !bytes.HasPrefix(rec.Body.Bytes(), []byte("\x89PNG\r\n\x1a\n")) {
		t.Fatal("response is not PNG data")
	}
	if provider.model != "openai-image-2.5" || provider.prompt != "canonical ventilator prompt" {
		t.Fatalf("provider request = (%q, %q)", provider.model, provider.prompt)
	}
	entries, err := os.ReadDir(tmp)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("saved files = %d, want 1", len(entries))
	}
}

func TestGenerateRejectsUnsupportedSubject(t *testing.T) {
	t.Parallel()

	h := NewHandler(Config{OutputDir: t.TempDir(), PromptDir: t.TempDir(), Model: "model", Provider: &fakeProvider{}})
	e := echo.New()
	body, _ := json.Marshal(map[string]string{"subject": "camera"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/product-drawings", bytes.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	if err := h(e.NewContext(req, rec)); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestGeneratePendingSendsPromptVerbatim(t *testing.T) {
	promptDir := t.TempDir()
	outputDir := t.TempDir()
	pendingDir := t.TempDir()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"data":[{"b64_json":"iVBORw0KGgo="}]}`)
	}))
	defer server.Close()
	t.Setenv("PROMPTS_DIR", promptDir)
	t.Setenv("PRODUCT_DRAWINGS_DIR", outputDir)
	t.Setenv("PRODUCT_DRAWINGS_PENDING_DIR", pendingDir)
	t.Setenv("IMAGE_GEN_BASE_URL", server.URL)
	t.Setenv("IMAGE_GEN_API_KEY", "test-key")
	t.Setenv("IMAGE_GEN_MODEL", "test-model")

	const customPrompt = "Draw a 3D exploded technical illustration of 血压计. custom template text"
	body, _ := json.Marshal(map[string]string{"name": "血压计", "prompt": customPrompt})
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/product-drawings/generate", bytes.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	if err := GeneratePending(e.NewContext(req, rec)); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusCreated {
		t.Fatalf("generate status = %d: %s", rec.Code, rec.Body.String())
	}
	var pending PendingResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &pending); err != nil {
		t.Fatal(err)
	}
	if pending.Prompt != customPrompt {
		t.Fatalf("prompt = %q, want it unchanged from the request: %q", pending.Prompt, customPrompt)
	}
}

func TestComposePromptEndpoint(t *testing.T) {
	promptDir := t.TempDir()
	writeExplodedViewTemplate(t, promptDir)
	t.Setenv("PROMPTS_DIR", promptDir)

	body, _ := json.Marshal(ComposePromptRequest{ProductName: "血压计", Components: []string{"控制按钮", "电路板"}})
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/product-drawings/compose-prompt", bytes.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	if err := ComposePrompt(e.NewContext(req, rec)); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	var resp ComposePromptResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(resp.Prompt, "控制按钮, 电路板") {
		t.Fatalf("composed prompt missing components: %q", resp.Prompt)
	}
}

func TestComposePromptRequiresProductName(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/product-drawings/compose-prompt", bytes.NewReader([]byte(`{}`)))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	if err := ComposePrompt(e.NewContext(req, rec)); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestGenerateMapsProviderFailure(t *testing.T) {
	t.Parallel()

	promptDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(promptDir, promptFileName), []byte("prompt"), 0o644); err != nil {
		t.Fatal(err)
	}
	h := NewHandler(Config{OutputDir: t.TempDir(), PromptDir: promptDir, Model: "model", Provider: &fakeProvider{err: io.ErrUnexpectedEOF}})
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/product-drawings", nil)
	rec := httptest.NewRecorder()
	if err := h(e.NewContext(req, rec)); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadGateway)
	}
}
