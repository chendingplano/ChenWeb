package imagehandler

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/chendingplano/shared/go/api/EchoFactory"
)

type generateRequest struct {
	Prompt string `json:"prompt"`
}

// openAIImageRequest is the standard OpenAI-compatible /v1/images/generations body.
type openAIImageRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
	N      int    `json:"n"`
}

// openAIImageResponse covers both response shapes: base64 or URL.
type openAIImageResponse struct {
	Data []struct {
		B64JSON string `json:"b64_json"`
		URL     string `json:"url"`
	} `json:"data"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// GenerateImage handles POST /api/v1/images/generate. It calls an OpenAI-compatible
// text-to-image endpoint configured via env, saves the result into kb.images
// (origin='generated'), and returns the new image's metadata. Fail-soft: missing
// config or a provider error returns a clear message with no partial row/file.
func GenerateImage(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_IMG_005")
	defer rc.Close()
	logger := rc.GetLogger()

	baseURL := strings.TrimRight(strings.TrimSpace(os.Getenv("IMAGE_GEN_BASE_URL")), "/")
	apiKey := strings.TrimSpace(os.Getenv("IMAGE_GEN_API_KEY"))
	model := strings.TrimSpace(os.Getenv("IMAGE_GEN_MODEL"))
	if baseURL == "" || apiKey == "" || model == "" {
		return c.JSON(http.StatusServiceUnavailable, errorResponse{false, "image generation is not configured (CWB_IMG_050)"})
	}

	dir := imageDir()
	if dir == "" {
		return c.JSON(http.StatusInternalServerError, errorResponse{false, "IMAGE_DIR is not configured (CWB_IMG_051)"})
	}

	var req generateRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{false, "invalid request (CWB_IMG_052)"})
	}
	prompt := strings.TrimSpace(req.Prompt)
	if prompt == "" {
		prompt = "An abstract, professional training-video cover image"
	}

	imgBytes, contentType, err := callImageProvider(baseURL, apiKey, model, prompt)
	if err != nil {
		logger.Error("image generation failed", "err", err)
		return c.JSON(http.StatusBadGateway, errorResponse{false, "image generation failed (CWB_IMG_053)"})
	}

	meta, err := storeImage(dir, "generated.png", contentType, "generated", prompt, currentUserEmail(rc), bytes.NewReader(imgBytes), logger)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, errorResponse{false, "failed to save generated image (CWB_IMG_054)"})
	}
	return c.JSON(http.StatusOK, meta)
}

// callImageProvider posts to {base}/v1/images/generations and returns the decoded
// image bytes. Handles both b64_json and url response shapes.
func callImageProvider(baseURL, apiKey, model, prompt string) ([]byte, string, error) {
	body, err := json.Marshal(openAIImageRequest{Model: model, Prompt: prompt, N: 1})
	if err != nil {
		return nil, "", err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	// Accept a base URL that already includes the OpenAI-compatible `/v1`
	// segment (e.g. DashScope's .../compatible-mode/v1) as well as a bare host.
	endpoint := baseURL + "/v1/images/generations"
	if strings.HasSuffix(baseURL, "/v1") {
		endpoint = baseURL + "/images/generations"
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, "", err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)

	client := &http.Client{Timeout: 125 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	if err != nil {
		return nil, "", err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, "", fmt.Errorf("provider returned status %d: %s", resp.StatusCode, truncate(string(raw), 300))
	}

	var parsed openAIImageResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, "", fmt.Errorf("invalid provider response: %w", err)
	}
	if parsed.Error != nil && parsed.Error.Message != "" {
		return nil, "", fmt.Errorf("provider error: %s", parsed.Error.Message)
	}
	if len(parsed.Data) == 0 {
		return nil, "", fmt.Errorf("provider returned no image data")
	}

	first := parsed.Data[0]
	if first.B64JSON != "" {
		decoded, err := base64.StdEncoding.DecodeString(first.B64JSON)
		if err != nil {
			return nil, "", fmt.Errorf("failed to decode image: %w", err)
		}
		return decoded, "image/png", nil
	}
	if first.URL != "" {
		return fetchImageURL(ctx, first.URL)
	}
	return nil, "", fmt.Errorf("provider response had neither b64_json nor url")
}

func fetchImageURL(ctx context.Context, url string) ([]byte, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, "", err
	}
	resp, err := (&http.Client{Timeout: 60 * time.Second}).Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, "", fmt.Errorf("image url returned status %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	if err != nil {
		return nil, "", err
	}
	ct := strings.TrimSpace(resp.Header.Get("Content-Type"))
	if ct == "" {
		ct = "image/png"
	}
	return data, ct, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
