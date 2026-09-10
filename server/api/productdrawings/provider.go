package productdrawings

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
)

type openAIProvider struct {
	baseURL string
	apiKey  string
	client  *http.Client
}

type selectionProvider struct{ qwen, openAI ImageProvider }

func (p selectionProvider) Generate(ctx context.Context, model, prompt string) ([]byte, error) {
	if strings.EqualFold(model, "Qwen") {
		return p.qwen.Generate(ctx, qwenModelName(), prompt)
	}
	return p.openAI.Generate(ctx, openAIModelName(), prompt)
}

const qwenSelectionModel = "Qwen"
const openAISelectionModel = "OpenAI"

func qwenModelName() string {
	return envOr("QWEN_IMAGE_GEN_MODEL", envOr("IMAGE_GEN_MODEL", "wan2.2-t2i-flash"))
}
func openAIModelName() string { return envOr("OPENAI_IMAGE_GEN_MODEL", "gpt-image-2.5-sunburst") }

type openAIRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
	N      int    `json:"n"`
}

type openAIResponse struct {
	Data []struct {
		B64JSON string `json:"b64_json"`
		URL     string `json:"url"`
	} `json:"data"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

func newOpenAIProvider(baseURL, apiKey string) ImageProvider {
	return &openAIProvider{
		baseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		apiKey:  strings.TrimSpace(apiKey),
		client:  &http.Client{Timeout: 125 * time.Second},
	}
}

func (p *openAIProvider) Generate(ctx context.Context, model, prompt string) ([]byte, error) {
	if p.baseURL == "" || p.apiKey == "" {
		return nil, fmt.Errorf("image generation provider is not configured")
	}
	if strings.Contains(strings.ToLower(p.baseURL), "dashscope") || strings.EqualFold(strings.TrimSpace(getenv("IMAGE_GEN_STYLE")), "dashscope") {
		return p.generateDashScope(ctx, model, prompt)
	}
	body, err := json.Marshal(openAIRequest{Model: model, Prompt: prompt, N: 1})
	if err != nil {
		return nil, err
	}
	endpoint := p.baseURL + "/v1/images/generations"
	if strings.HasSuffix(p.baseURL, "/v1") {
		endpoint = p.baseURL + "/images/generations"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.apiKey)
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("provider returned status %d: %s", resp.StatusCode, truncateProviderError(raw, 300))
	}
	var parsed openAIResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("invalid provider response: %w", err)
	}
	if parsed.Error != nil {
		return nil, fmt.Errorf("provider returned an error")
	}
	if len(parsed.Data) == 0 {
		return nil, fmt.Errorf("provider returned no image data")
	}
	if parsed.Data[0].B64JSON != "" {
		decoded, err := base64.StdEncoding.DecodeString(parsed.Data[0].B64JSON)
		if err != nil {
			return nil, fmt.Errorf("decode provider image: %w", err)
		}
		return decoded, nil
	}
	if parsed.Data[0].URL == "" {
		return nil, fmt.Errorf("provider returned no image URL")
	}
	return fetchURL(ctx, p.client, parsed.Data[0].URL)
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

func (p *openAIProvider) generateDashScope(ctx context.Context, model, prompt string) ([]byte, error) {
	u, err := url.Parse(p.baseURL)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return nil, fmt.Errorf("invalid image generation base URL")
	}
	host := u.Scheme + "://" + u.Host
	body, err := json.Marshal(map[string]any{"model": model, "input": map[string]any{"prompt": prompt}, "parameters": map[string]any{"n": 1}})
	if err != nil {
		return nil, err
	}
	requestCtx, cancel := context.WithTimeout(ctx, 150*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(requestCtx, http.MethodPost, host+"/api/v1/services/aigc/text2image/image-synthesis", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.apiKey)
	req.Header.Set("X-DashScope-Async", "enable")
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("DashScope submit returned status %d: %s", resp.StatusCode, truncateProviderError(raw, 300))
	}
	var submitted dashScopeTaskResponse
	if err := json.Unmarshal(raw, &submitted); err != nil {
		return nil, fmt.Errorf("invalid DashScope response: %w", err)
	}
	if submitted.Output.TaskID == "" {
		return nil, fmt.Errorf("DashScope response did not include a task id")
	}
	for {
		select {
		case <-requestCtx.Done():
			return nil, requestCtx.Err()
		case <-time.After(3 * time.Second):
		}
		poll, err := http.NewRequestWithContext(requestCtx, http.MethodGet, host+"/api/v1/tasks/"+url.PathEscape(submitted.Output.TaskID), nil)
		if err != nil {
			return nil, err
		}
		poll.Header.Set("Authorization", "Bearer "+p.apiKey)
		pollResp, err := p.client.Do(poll)
		if err != nil {
			return nil, err
		}
		pollRaw, _ := io.ReadAll(io.LimitReader(pollResp.Body, 4<<20))
		pollResp.Body.Close()
		if pollResp.StatusCode < 200 || pollResp.StatusCode >= 300 {
			return nil, fmt.Errorf("DashScope task returned status %d: %s", pollResp.StatusCode, truncateProviderError(pollRaw, 300))
		}
		var result dashScopeTaskResponse
		if err := json.Unmarshal(pollRaw, &result); err != nil {
			return nil, fmt.Errorf("invalid DashScope task response: %w", err)
		}
		switch result.Output.TaskStatus {
		case "SUCCEEDED":
			if len(result.Output.Results) == 0 || result.Output.Results[0].URL == "" {
				return nil, fmt.Errorf("DashScope task succeeded without an image URL")
			}
			return fetchURL(requestCtx, p.client, result.Output.Results[0].URL)
		case "FAILED", "CANCELED", "UNKNOWN":
			msg := result.Output.Message
			if msg == "" {
				msg = result.Message
			}
			return nil, fmt.Errorf("DashScope task %s: %s", result.Output.TaskStatus, msg)
		}
	}
}

func getenv(name string) string { return os.Getenv(name) }
func truncateProviderError(raw []byte, n int) string {
	s := strings.TrimSpace(string(raw))
	if len(s) > n {
		return s[:n]
	}
	return s
}

func fetchURL(ctx context.Context, client *http.Client, imageURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, imageURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("image URL returned status %d", resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 64<<20))
}
