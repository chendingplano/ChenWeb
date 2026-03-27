package docgenhandler

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/chendingplano/deepdoc/server/cmd/config"
	"github.com/chendingplano/shared/go/api/EchoFactory"
	"github.com/labstack/echo/v4"
)

// ListTemplates handles GET /api/v1/docgen/templates
func ListTemplates(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_DGH_010")
	defer rc.Close()
	logger := rc.GetLogger()

	templateDir := config.GetDocGenConfig().TemplateDir
	entries, err := os.ReadDir(templateDir)
	if err != nil {
		if os.IsNotExist(err) {
			return c.JSON(http.StatusOK, TemplateListResponse{Status: true, Templates: []string{}})
		}
		logger.Error("list templates failed", "err", err)
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Status: false, ErrorMsg: "failed to list templates (CWB_DGH_011)"})
	}

	var names []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(strings.ToLower(e.Name()), ".docx") {
			names = append(names, e.Name())
		}
	}
	if names == nil {
		names = []string{}
	}
	return c.JSON(http.StatusOK, TemplateListResponse{Status: true, Templates: names})
}

// UploadTemplate handles POST /api/v1/docgen/templates — admin only
func UploadTemplate(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_DGH_020")
	defer rc.Close()
	logger := rc.GetLogger()

	userInfo := rc.IsAuthenticated()
	if userInfo == nil || !userInfo.Admin {
		return c.JSON(http.StatusForbidden, ErrorResponse{Status: false, ErrorMsg: "admin only (CWB_DGH_021)"})
	}

	file, err := c.FormFile("file")
	if err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Status: false, ErrorMsg: "file field required (CWB_DGH_022)"})
	}
	baseName := filepath.Base(file.Filename)
	if !strings.HasSuffix(strings.ToLower(baseName), ".docx") {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Status: false, ErrorMsg: "only .docx files are accepted (CWB_DGH_023)"})
	}

	templateDir := config.GetDocGenConfig().TemplateDir
	if err := os.MkdirAll(templateDir, 0755); err != nil {
		logger.Error("mkdir template dir failed", "err", err)
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Status: false, ErrorMsg: "failed to create template directory (CWB_DGH_024)"})
	}

	src, err := file.Open()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Status: false, ErrorMsg: "failed to open upload (CWB_DGH_025)"})
	}
	defer src.Close()

	dst, err := os.Create(filepath.Join(templateDir, baseName))
	if err != nil {
		logger.Error("create template file failed", "err", err)
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Status: false, ErrorMsg: "failed to save template (CWB_DGH_026)"})
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		logger.Error("copy template failed", "err", err)
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Status: false, ErrorMsg: "failed to write template (CWB_DGH_027)"})
	}

	return c.JSON(http.StatusOK, map[string]any{"status": true, "filename": baseName})
}
