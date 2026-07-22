package imagehandler

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/chendingplano/shared/go/api/EchoFactory"
)

type generateRequest struct {
	Prompt string `json:"prompt"`
}

// GenerateImage handles POST /api/v1/images/generate. It generates an image from
// a text prompt via a configured provider, saves the result into kb.images
// (origin='generated'), and returns the new image's metadata. Fail-soft: missing
// config or a provider error returns a clear message with no partial row/file.
//
// Provider styles (IMAGE_GEN_STYLE, auto-detected from the base URL when unset):
//   - "openai":    synchronous OpenAI-compatible POST {base}/[v1/]images/generations
//   - "dashscope": Alibaba DashScope async task API (submit + poll + fetch URL)
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

	imgBytes, contentType, err := callImageProvider(imageGenStyle(baseURL), baseURL, apiKey, model, prompt)
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

// imageGenStyle picks the provider protocol: explicit IMAGE_GEN_STYLE wins,
// otherwise DashScope/Aliyun bases use the async task API and everything else
// the synchronous OpenAI-compatible endpoint.
func imageGenStyle(baseURL string) string {
	if s := strings.ToLower(strings.TrimSpace(os.Getenv("IMAGE_GEN_STYLE"))); s != "" {
		return s
	}
	lb := strings.ToLower(baseURL)
	if strings.Contains(lb, "dashscope") || strings.Contains(lb, "aliyuncs") {
		return "dashscope"
	}
	return "openai"
}

func imageGenSize() string {
	if s := strings.TrimSpace(os.Getenv("IMAGE_GEN_SIZE")); s != "" {
		return s
	}
	return "1024*1024"
}

func callImageProvider(style, baseURL, apiKey, model, prompt string) ([]byte, string, error) {
	if style == "dashscope" {
		return callDashScope(baseURL, apiKey, model, prompt)
	}
	return callOpenAICompatible(baseURL, apiKey, model, prompt)
}

// --- OpenAI-compatible (synchronous) ---

type openAIImageRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
	N      int    `json:"n"`
}

type openAIImageResponse struct {
	Data []struct {
		B64JSON string `json:"b64_json"`
		URL     string `json:"url"`
	} `json:"data"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

func callOpenAICompatible(baseURL, apiKey, model, prompt string) ([]byte, string, error) {
	body, err := json.Marshal(openAIImageRequest{Model: model, Prompt: prompt, N: 1})
	if err != nil {
		return nil, "", err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	// Accept a base URL that already includes the `/v1` segment as well as a bare host.
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

	resp, err := (&http.Client{Timeout: 125 * time.Second}).Do(httpReq)
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

// --- DashScope (asynchronous task API) ---

type dashScopeSubmitRequest struct {
	Model      string         `json:"model"`
	Input      map[string]any `json:"input"`
	Parameters map[string]any `json:"parameters"`
}

type dashScopeTaskResponse struct {
	Output struct {
		TaskID     string `json:"task_id"`
		TaskStatus string `json:"task_status"`
		Message    string `json:"message"`
		Results    []struct {
			URL string `json:"url"`
		} `json:"results"`
	} `json:"output"`
	Message string `json:"message"`
}

func callDashScope(baseURL, apiKey, model, prompt string) ([]byte, string, error) {
	host, err := hostRoot(baseURL)
	if err != nil {
		return nil, "", err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Second)
	defer cancel()

	// Submit the async generation task.
	submitBody, err := json.Marshal(dashScopeSubmitRequest{
		Model:      model,
		Input:      map[string]any{"prompt": prompt},
		Parameters: map[string]any{"size": imageGenSize(), "n": 1},
	})
	if err != nil {
		return nil, "", err
	}
	submitReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		host+"/api/v1/services/aigc/text2image/image-synthesis", bytes.NewReader(submitBody))
	if err != nil {
		return nil, "", err
	}
	submitReq.Header.Set("Content-Type", "application/json")
	submitReq.Header.Set("Authorization", "Bearer "+apiKey)
	submitReq.Header.Set("X-DashScope-Async", "enable")

	client := &http.Client{Timeout: 30 * time.Second}
	submitResp, err := client.Do(submitReq)
	if err != nil {
		return nil, "", err
	}
	rawSubmit, _ := io.ReadAll(io.LimitReader(submitResp.Body, 1<<20))
	submitResp.Body.Close()
	if submitResp.StatusCode < 200 || submitResp.StatusCode >= 300 {
		return nil, "", fmt.Errorf("submit returned status %d: %s", submitResp.StatusCode, truncate(string(rawSubmit), 300))
	}

	var submitParsed dashScopeTaskResponse
	if err := json.Unmarshal(rawSubmit, &submitParsed); err != nil {
		return nil, "", fmt.Errorf("invalid submit response: %w", err)
	}
	taskID := submitParsed.Output.TaskID
	if taskID == "" {
		return nil, "", fmt.Errorf("no task_id in submit response: %s", truncate(string(rawSubmit), 200))
	}

	// Poll until the task terminates.
	taskURL := host + "/api/v1/tasks/" + taskID
	deadline := time.Now().Add(140 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return nil, "", ctx.Err()
		case <-time.After(3 * time.Second):
		}

		pollReq, err := http.NewRequestWithContext(ctx, http.MethodGet, taskURL, nil)
		if err != nil {
			return nil, "", err
		}
		pollReq.Header.Set("Authorization", "Bearer "+apiKey)
		pollResp, err := client.Do(pollReq)
		if err != nil {
			return nil, "", err
		}
		rawPoll, _ := io.ReadAll(io.LimitReader(pollResp.Body, 4<<20))
		pollResp.Body.Close()

		var poll dashScopeTaskResponse
		if err := json.Unmarshal(rawPoll, &poll); err != nil {
			return nil, "", fmt.Errorf("invalid task response: %w", err)
		}
		switch poll.Output.TaskStatus {
		case "SUCCEEDED":
			if len(poll.Output.Results) == 0 || poll.Output.Results[0].URL == "" {
				return nil, "", fmt.Errorf("task succeeded but returned no image url")
			}
			return fetchImageURL(ctx, poll.Output.Results[0].URL)
		case "FAILED", "CANCELED", "UNKNOWN":
			msg := poll.Output.Message
			if msg == "" {
				msg = poll.Message
			}
			return nil, "", fmt.Errorf("task %s: %s", poll.Output.TaskStatus, msg)
		}
		// PENDING / RUNNING: keep polling.
	}
	return nil, "", fmt.Errorf("image generation timed out")
}

// hostRoot returns scheme://host for a URL, dropping any path (DashScope's async
// endpoints live at the host root, not under the compatible-mode base path).
func hostRoot(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", err
	}
	if u.Scheme == "" || u.Host == "" {
		return "", fmt.Errorf("invalid IMAGE_GEN_BASE_URL: %s", raw)
	}
	return u.Scheme + "://" + u.Host, nil
}

// --- shared ---

func fetchImageURL(ctx context.Context, imageURL string) ([]byte, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, imageURL, nil)
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
