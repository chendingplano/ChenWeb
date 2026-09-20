package imagehandler

import (
	"bytes"
	"net/http"
	"os"
	"strings"

	"github.com/chendingplano/shared/go/api/EchoFactory"
	sharedllm "github.com/chendingplano/shared/go/api/llm"
	"github.com/labstack/echo/v4"
)

type generateRequest struct {
	Prompt string `json:"prompt"`
}

func GenerateImage(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_IMG_005")
	defer rc.Close()
	logger := rc.GetLogger()
	baseURL, apiKey, model := strings.TrimRight(strings.TrimSpace(os.Getenv("IMAGE_GEN_BASE_URL")), "/"), strings.TrimSpace(os.Getenv("IMAGE_GEN_API_KEY")), strings.TrimSpace(os.Getenv("IMAGE_GEN_MODEL"))
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
	userID := ""
	if user := rc.IsAuthenticated(); user != nil {
		userID = strings.TrimSpace(user.UserId)
	}
	style := sharedllm.ImageStyleOpenAICompatible
	if imageGenStyle(baseURL) == "dashscope" {
		style = sharedllm.ImageStyleDashScope
	}
	response, err := sharedllm.NewImageClient(baseURL, apiKey, nil).Generate(c.Request().Context(), sharedllm.ImageRequest{Style: style, Model: model, Prompt: prompt, Size: imageGenSize(), Capture: &sharedllm.RequestCapture{UserID: userID}})
	if err != nil {
		logger.Error("image generation failed", "err", err)
		return c.JSON(http.StatusBadGateway, errorResponse{false, "image generation failed (CWB_IMG_053)"})
	}
	meta, err := storeImage(dir, "generated.png", response.ContentType, "generated", prompt, currentUserEmail(rc), bytes.NewReader(response.Data), logger)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, errorResponse{false, "failed to save generated image (CWB_IMG_054)"})
	}
	return c.JSON(http.StatusOK, meta)
}
func imageGenStyle(baseURL string) string {
	if s := strings.ToLower(strings.TrimSpace(os.Getenv("IMAGE_GEN_STYLE"))); s != "" {
		return s
	}
	baseURL = strings.ToLower(baseURL)
	if strings.Contains(baseURL, "dashscope") || strings.Contains(baseURL, "aliyuncs") {
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
