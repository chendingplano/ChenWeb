package docbenchmarkadminhandler

import (
	"net/http"
	"strconv"

	docbenchmarkadmin "github.com/chendingplano/deepdoc/server/api/doc-benchmark-admin"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/labstack/echo/v4"
)

func service() docbenchmarkadmin.Service {
	return docbenchmarkadmin.NewService(ApiTypes.ProjectDBHandle)
}

func GetConfig(c echo.Context) error {
	cfg, err := service().GetConfig(c.Request().Context())
	if err != nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]any{"ok": false, "message": err.Error()})
	}
	return c.JSON(http.StatusOK, cfg)
}

func PutConfig(c echo.Context) error {
	var cfg docbenchmarkadmin.Config
	if err := c.Bind(&cfg); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{"ok": false, "message": "invalid json body"})
	}
	saved, err := service().SaveConfig(c.Request().Context(), cfg)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]any{"ok": false, "message": err.Error()})
	}
	return c.JSON(http.StatusOK, saved)
}

func GetSetupState(c echo.Context) error {
	state, err := service().SetupState(c.Request().Context())
	if err != nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]any{"ok": false, "message": err.Error()})
	}
	return c.JSON(http.StatusOK, state)
}

func RunStep(c echo.Context) error {
	job, err := service().RunStep(c.Request().Context(), c.Param("stepId"), "browser")
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{"ok": false, "message": err.Error()})
	}
	return c.JSON(http.StatusAccepted, job)
}

func RunNext(c echo.Context) error {
	job, err := service().RunNext(c.Request().Context(), "browser")
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{"ok": false, "message": err.Error()})
	}
	return c.JSON(http.StatusAccepted, job)
}

func ListJobs(c echo.Context) error {
	jobs, err := service().Store.ListJobs(c.Request().Context(), docbenchmarkadmin.DefaultScope, 50)
	if err != nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]any{"ok": false, "message": err.Error()})
	}
	return c.JSON(http.StatusOK, map[string]any{"jobs": jobs})
}

func GetJob(c echo.Context) error {
	id, err := strconv.ParseInt(c.Param("jobId"), 10, 64)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{"ok": false, "message": "invalid job id"})
	}
	job, err := service().Store.GetJob(c.Request().Context(), id)
	if err != nil {
		return c.JSON(http.StatusNotFound, map[string]any{"ok": false, "message": err.Error()})
	}
	return c.JSON(http.StatusOK, job)
}
