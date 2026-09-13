package productdrawings

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/EchoFactory"
	"github.com/labstack/echo/v4"
)

const pendingLifetime = 30 * time.Minute

func NewHandler(cfg Config) echo.HandlerFunc {
	if strings.TrimSpace(cfg.Model) == "" {
		cfg.Model = defaultModel
	}
	if cfg.Provider == nil {
		cfg.Provider = newOpenAIProvider(cfg.BaseURL, cfg.APIKey)
	}
	return func(c echo.Context) error {
		rc := EchoFactory.NewFromEcho(c, "CWB_DRAW_001")
		defer rc.Close()
		logger := rc.GetLogger()

		var req Request
		if c.Request().Body != nil && c.Request().ContentLength != 0 {
			if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
				return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request"})
			}
		}
		if strings.TrimSpace(req.Subject) == "" {
			req.Subject = "ventilator"
		}
		if strings.TrimSpace(req.Model) == "" {
			req.Model = cfg.Model
		}
		if req.Model != cfg.Model {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "unsupported model"})
		}
		prompt, err := loadPrompt(cfg.PromptDir, req.Subject)
		if err != nil {
			if strings.Contains(err.Error(), "unsupported subject") {
				return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
			}
			logger.Error("load product drawing prompt failed", "err", err)
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to load drawing prompt"})
		}
		img, err := cfg.Provider.Generate(c.Request().Context(), req.Model, prompt)
		if err != nil {
			logger.Error("product drawing generation failed", "err", err)
			return c.JSON(http.StatusBadGateway, map[string]string{"error": "image generation failed"})
		}
		if !bytes.HasPrefix(img, []byte("\x89PNG\r\n\x1a\n")) {
			return c.JSON(http.StatusBadGateway, map[string]string{"error": "provider did not return PNG data"})
		}
		if strings.TrimSpace(cfg.OutputDir) == "" {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "drawing output directory is not configured"})
		}
		if err := os.MkdirAll(cfg.OutputDir, 0o755); err != nil {
			logger.Error("create product drawing directory failed", "err", err)
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to save drawing"})
		}
		filename := drawingFilename()
		path := filepath.Join(cfg.OutputDir, filename)
		if err := writeExclusive(path, img); err != nil {
			logger.Error("save product drawing failed", "path", path, "err", err)
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to save drawing"})
		}
		c.Response().Header().Set(echo.HeaderContentType, "image/png")
		c.Response().Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
		c.Response().Header().Set("X-Drawing-Filename", filename)
		return c.Blob(http.StatusCreated, "image/png", img)
	}
}

func Generate(c echo.Context) error {
	return NewHandler(defaultConfig())(c)
}

func GeneratePending(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_DRAW_002")
	defer rc.Close()
	logger := rc.GetLogger()
	cfg := defaultConfig()
	var req Request
	if c.Request().Body != nil && c.Request().ContentLength != 0 {
		if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request"})
		}
	}
	if strings.TrimSpace(req.Subject) == "" {
		req.Subject = "ventilator"
	}
	selection := normalizeSelection(req.Model)
	if strings.TrimSpace(req.Model) == "" {
		selection = cfg.Model
	}
	if selection == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "unsupported model"})
	}
	req.Model = selection
	prompt := strings.TrimSpace(req.Prompt)
	var err error
	if prompt == "" {
		prompt, err = loadPrompt(cfg.PromptDir, req.Subject)
		if err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
		}
	}
	req.Prompt = prompt
	modelName := modelNameForSelection(selection)
	img, err := cfg.Provider.Generate(c.Request().Context(), selection, buildDrawingPrompt(prompt))
	if err != nil {
		logger.Error("pending product drawing generation failed", "err", err)
		return c.JSON(http.StatusBadGateway, map[string]string{"error": "image generation failed"})
	}
	if !bytes.HasPrefix(img, []byte("\x89PNG\r\n\x1a\n")) {
		return c.JSON(http.StatusBadGateway, map[string]string{"error": "provider did not return PNG data"})
	}
	if err := os.MkdirAll(cfg.PendingDir, 0o700); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to create pending drawing storage"})
	}
	token := pendingToken()
	if err := writeExclusive(filepath.Join(cfg.PendingDir, token+".png"), img); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to save pending drawing"})
	}
	meta, _ := json.Marshal(req)
	if err := os.WriteFile(filepath.Join(cfg.PendingDir, token+".json"), meta, 0o600); err != nil {
		_ = os.Remove(filepath.Join(cfg.PendingDir, token+".png"))
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to save drawing metadata"})
	}
	expires := time.Now().UTC().Add(pendingLifetime)
	return c.JSON(http.StatusCreated, PendingResponse{
		Token: token, ImageURL: "/api/v1/product-drawings/pending/" + token + "/content",
		Prompt: prompt, Model: selection, ModelName: modelName, Name: req.Name, Description: req.Description, Keywords: req.Keywords, Notes: req.Notes, ExpiresAt: expires.Format(time.RFC3339),
	})
}

func ServePendingContent(c echo.Context) error {
	path, ok := pendingPath(c.Param("token"))
	if !ok || expired(path) {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "pending drawing not found"})
	}
	f, err := os.Open(path)
	if err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "pending drawing not found"})
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "pending drawing not found"})
	}
	http.ServeContent(c.Response(), c.Request(), filepath.Base(path), info.ModTime(), f)
	return nil
}

func KeepPending(c echo.Context) error {
	token := c.Param("token")
	src, ok := pendingPath(token)
	if !ok || expired(src) {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "pending drawing not found"})
	}
	cfg := defaultConfig()
	if err := os.MkdirAll(cfg.OutputDir, 0o755); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to create drawing storage"})
	}
	filename := drawingFilename()
	dst := filepath.Join(cfg.OutputDir, filename)
	if err := os.Rename(src, dst); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to keep drawing"})
	}
	var req Request
	metaPath := filepath.Join(cfg.PendingDir, token+".json")
	if data, err := os.ReadFile(metaPath); err == nil {
		_ = json.Unmarshal(data, &req)
	}
	if strings.TrimSpace(req.Name) == "" {
		req.Name = strings.TrimSuffix(filename, filepath.Ext(filename))
	}
	if strings.TrimSpace(req.Prompt) == "" {
		req.Prompt = "Generated product drawing"
	}
	var id int64
	if ApiTypes.ProjectDBHandle != nil {
		row := ApiTypes.ProjectDBHandle.QueryRowContext(c.Request().Context(), `INSERT INTO kb.product_drawings (name,description,prompt,keywords,notes,filename,stored_path,model,model_name) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING id`, strings.TrimSpace(req.Name), strings.TrimSpace(req.Description), strings.TrimSpace(req.Prompt), strings.TrimSpace(req.Keywords), strings.TrimSpace(req.Notes), filename, dst, normalizeSelection(req.Model), modelNameForSelection(req.Model))
		if err := row.Scan(&id); err != nil {
			_ = os.Rename(dst, src)
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to save drawing metadata"})
		}
	}
	_ = os.Remove(metaPath)
	return c.JSON(http.StatusOK, KeepResponse{Status: true, Filename: filename, Path: "resources/product-drawings/" + filename, ID: id})
}

func IgnorePending(c echo.Context) error {
	path, ok := pendingPath(c.Param("token"))
	if !ok {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "pending drawing not found"})
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to ignore drawing"})
	}
	_ = os.Remove(strings.TrimSuffix(path, ".png") + ".json")
	return c.JSON(http.StatusOK, map[string]any{"status": true, "ignored": true})
}

func defaultConfig() Config {
	defaultSelection := openAISelectionModel
	if strings.TrimSpace(os.Getenv("IMAGE_GEN_BASE_URL")) != "" && strings.TrimSpace(os.Getenv("OPENAI_IMAGE_GEN_BASE_URL")) == "" {
		defaultSelection = qwenSelectionModel
	}
	cfg := Config{
		OutputDir: envOr("PRODUCT_DRAWINGS_DIR", "/Users/cding/Workspace/KnowledgeStore/doc-repo/resources/product-drawings"),
		PromptDir: envOr("PROMPTS_DIR", "prompts"), Model: defaultSelection,
		BaseURL: os.Getenv("IMAGE_GEN_BASE_URL"), APIKey: os.Getenv("IMAGE_GEN_API_KEY"),
		PendingDir: envOr("PRODUCT_DRAWINGS_PENDING_DIR", filepath.Join(os.TempDir(), "chenweb-product-drawings")),
	}
	qwenBase := envOr("IMAGE_GEN_BASE_URL", "https://dashscope.aliyuncs.com/compatible-mode/v1")
	qwenKey := os.Getenv("IMAGE_GEN_API_KEY")
	openAIBase := envOr("OPENAI_IMAGE_GEN_BASE_URL", "https://api.openai.com/v1")
	openAIKey := envOr("OPENAI_IMAGE_GEN_API_KEY", os.Getenv("OPENAI_API_KEY"))
	cfg.Provider = selectionProvider{qwen: newOpenAIProvider(qwenBase, qwenKey), openAI: newOpenAIProvider(openAIBase, openAIKey)}
	return cfg
}

func normalizeSelection(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "qwen":
		return qwenSelectionModel
	case "openai", "":
		return openAISelectionModel
	default:
		return ""
	}
}
func modelNameForSelection(selection string) string {
	if strings.EqualFold(selection, qwenSelectionModel) {
		return qwenModelName()
	}
	return openAIModelName()
}

func pendingToken() string {
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		return hex.EncodeToString([]byte(fmt.Sprintf("%d", time.Now().UnixNano())))
	}
	return hex.EncodeToString(id[:])
}

func pendingPath(token string) (string, bool) {
	if len(token) != 32 {
		return "", false
	}
	for _, ch := range token {
		if !((ch >= '0' && ch <= '9') || (ch >= 'a' && ch <= 'f')) {
			return "", false
		}
	}
	return filepath.Join(defaultConfig().PendingDir, token+".png"), true
}

func expired(path string) bool {
	info, err := os.Stat(path)
	return err != nil || time.Since(info.ModTime()) > pendingLifetime
}

func envOr(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}

func drawingFilename() string {
	var id [4]byte
	if _, err := rand.Read(id[:]); err != nil {
		return fmt.Sprintf("ventilator-exploded-view-%d.png", time.Now().UnixNano())
	}
	return fmt.Sprintf("ventilator-exploded-view-%s-%d.png", hex.EncodeToString(id[:]), time.Now().UnixNano())
}

func writeExclusive(path string, content []byte) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	if _, err := file.Write(content); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return err
	}
	return file.Close()
}

var _ ImageProvider = (*openAIProvider)(nil)
