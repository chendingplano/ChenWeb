package kbhandler

import (
	docprocessing "github.com/chendingplano/deepdoc/server/api/doc-processing"
	"github.com/labstack/echo/v4"
	"net/http"
)

func GetProductsConfig(c echo.Context) error {
	rc, err := requirePendingFilesAdmin(c, "CWB_PRD_CFG_001")
	defer rc.Close()
	if err != nil {
		return err
	}
	cfg, err := docprocessing.LoadProductsConfig(docprocessing.ProductsConfigPath())
	if err != nil {
		rc.GetLogger().Warn("load products config failed", "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "cannot read configuration"})
	}
	return c.JSON(http.StatusOK, map[string]any{"config": cfg})
}
func PutProductsConfig(c echo.Context) error {
	rc, err := requirePendingFilesAdmin(c, "CWB_PRD_CFG_002")
	defer rc.Close()
	if err != nil {
		return err
	}
	var cfg docprocessing.ProductsConfig
	if err := c.Bind(&cfg); err != nil || cfg == nil {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid configuration"})
	}
	if err := docprocessing.SaveProductsConfig(docprocessing.ProductsConfigPath(), cfg); err != nil {
		rc.GetLogger().Warn("save products config failed", "err", err)
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: err.Error()})
	}
	return c.JSON(http.StatusOK, map[string]any{"status": true})
}
