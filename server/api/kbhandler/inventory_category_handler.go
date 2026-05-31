package kbhandler

import (
	"maps"
	"net/http"
	"strings"

	docprocessing "github.com/chendingplano/deepdoc/server/api/doc-processing"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/EchoFactory"
	"github.com/labstack/echo/v4"
)

type inventoryCategoryResponse struct {
	CategoryKey     string                                       `json:"category_key"`
	Status          string                                       `json:"status"`
	CanonicalOf     string                                       `json:"canonical_of,omitempty"`
	DisplayNames    []string                                     `json:"display_names"`
	RequiredAttrs   []string                                     `json:"required_attrs"`
	Specs           map[string]docprocessing.InventorySpecSchema `json:"specs"`
	PlausibleRanges map[string]inventoryPlausibleRangeJSON       `json:"plausible_ranges"`
	SeenCount       int64                                        `json:"seen_count"`
}

type inventoryPlausibleRangeJSON struct {
	Min  *float64 `json:"min,omitempty"`
	Max  *float64 `json:"max,omitempty"`
	Unit string   `json:"unit,omitempty"`
}

type listInventoryCategoriesResponse struct {
	Status     bool                        `json:"status"`
	Results    []inventoryCategoryResponse `json:"results"`
	Total      int                         `json:"total"`
}

type patchInventoryCategoryRequest struct {
	Status          *string                                       `json:"status"`
	CanonicalOf     *string                                       `json:"canonical_of"`
	RequiredAttrs   []string                                      `json:"required_attrs"`
	Specs           map[string]docprocessing.InventorySpecSchema  `json:"specs"`
	PlausibleRanges map[string]inventoryPlausibleRangeJSON        `json:"plausible_ranges"`
}

// ListInventoryCategories handles GET /api/v1/kb/inventory-categories
// Query params: status (default: all), limit (default: 100)
func ListInventoryCategories(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_ICAT_001")
	defer rc.Close()
	logger := rc.GetLogger()

	db := ApiTypes.ProjectDBHandle
	if db == nil {
		return c.JSON(http.StatusInternalServerError, errorResponse{
			Status:   false,
			ErrorMsg: "database unavailable (CWB_KB_ICAT_011)",
		})
	}

	statusFilter := strings.TrimSpace(c.QueryParam("status"))
	limit := 100
	if lStr := strings.TrimSpace(c.QueryParam("limit")); lStr != "" {
		if n, err := parseInt(lStr); err == nil && n > 0 {
			limit = n
		}
	}

	registry := docprocessing.InventoryCategoryRegistry{DB: db}
	var (
		records []docprocessing.InventoryCategoryRecord
		err     error
	)
	if statusFilter == docprocessing.InventoryCategoryStatusPending {
		records, err = registry.ListPendingForReview(c.Request().Context(), limit)
	} else {
		records, err = registry.ListCategoriesByStatus(c.Request().Context(), statusFilter, limit)
	}
	if err != nil {
		logger.Error("list inventory categories failed", "error", err, "status", statusFilter)
		return c.JSON(http.StatusInternalServerError, errorResponse{
			Status:   false,
			ErrorMsg: "failed to load inventory categories (CWB_KB_ICAT_020)",
		})
	}

	results := make([]inventoryCategoryResponse, 0, len(records))
	for _, rec := range records {
		results = append(results, toInventoryCategoryResponse(rec))
	}
	return c.JSON(http.StatusOK, listInventoryCategoriesResponse{
		Status:  true,
		Results: results,
		Total:   len(results),
	})
}

// UpdateInventoryCategory handles PATCH /api/v1/kb/inventory-categories/:key
// All fields in the request body are optional; only supplied fields are updated.
func UpdateInventoryCategory(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_ICAT_002")
	defer rc.Close()
	logger := rc.GetLogger()

	key := docprocessing.NormalizeInventoryToken(strings.TrimSpace(c.Param("key")))
	if key == "" {
		return c.JSON(http.StatusBadRequest, errorResponse{
			Status:   false,
			ErrorMsg: "missing or invalid category key (CWB_KB_ICAT_030)",
		})
	}

	var req patchInventoryCategoryRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{
			Status:   false,
			ErrorMsg: "invalid request body (CWB_KB_ICAT_031)",
		})
	}

	if req.Status != nil {
		allowed := map[string]bool{
			docprocessing.InventoryCategoryStatusApproved: true,
			docprocessing.InventoryCategoryStatusRejected: true,
			docprocessing.InventoryCategoryStatusMerged:   true,
			docprocessing.InventoryCategoryStatusPending:  true,
		}
		if !allowed[*req.Status] {
			return c.JSON(http.StatusBadRequest, errorResponse{
				Status:   false,
				ErrorMsg: "invalid status value (CWB_KB_ICAT_032)",
			})
		}
	}

	db := ApiTypes.ProjectDBHandle
	if db == nil {
		return c.JSON(http.StatusInternalServerError, errorResponse{
			Status:   false,
			ErrorMsg: "database unavailable (CWB_KB_ICAT_033)",
		})
	}

	registry := docprocessing.InventoryCategoryRegistry{DB: db}
	ctx := c.Request().Context()

	patch := docprocessing.InventoryCategoryPatch{
		Status:      req.Status,
		CanonicalOf: req.CanonicalOf,
	}
	if req.RequiredAttrs != nil {
		patch.RequiredAttrs = req.RequiredAttrs
	}
	if req.Specs != nil {
		patch.Specs = req.Specs
	}
	if req.PlausibleRanges != nil {
		ranges := make(map[string]docprocessing.InventoryPlausibleRange, len(req.PlausibleRanges))
		for k, v := range req.PlausibleRanges {
			ranges[k] = docprocessing.InventoryPlausibleRange{Min: v.Min, Max: v.Max, Unit: v.Unit}
		}
		patch.PlausibleRanges = ranges
	}

	updated, err := registry.PatchCategory(ctx, key, patch)
	if err != nil {
		if err == docprocessing.ErrInventoryCategoryNotFound {
			return c.JSON(http.StatusNotFound, errorResponse{
				Status:   false,
				ErrorMsg: "category not found (CWB_KB_ICAT_040)",
			})
		}
		logger.Error("patch inventory category failed", "key", key, "error", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{
			Status:   false,
			ErrorMsg: "failed to update category (CWB_KB_ICAT_041)",
		})
	}
	return c.JSON(http.StatusOK, map[string]any{
		"status": true,
		"result": toInventoryCategoryResponse(updated),
	})
}

func toInventoryCategoryResponse(rec docprocessing.InventoryCategoryRecord) inventoryCategoryResponse {
	specs := make(map[string]docprocessing.InventorySpecSchema, len(rec.Specs))
	maps.Copy(specs, rec.Specs)
	ranges := make(map[string]inventoryPlausibleRangeJSON, len(rec.PlausibleRanges))
	for k, v := range rec.PlausibleRanges {
		ranges[k] = inventoryPlausibleRangeJSON{Min: v.Min, Max: v.Max, Unit: v.Unit}
	}
	displayNames := rec.DisplayNames
	if displayNames == nil {
		displayNames = []string{}
	}
	requiredAttrs := rec.RequiredAttrs
	if requiredAttrs == nil {
		requiredAttrs = []string{}
	}
	return inventoryCategoryResponse{
		CategoryKey:     rec.CategoryKey,
		Status:          rec.Status,
		CanonicalOf:     rec.CanonicalOf,
		DisplayNames:    displayNames,
		RequiredAttrs:   requiredAttrs,
		Specs:           specs,
		PlausibleRanges: ranges,
		SeenCount:       rec.SeenCount,
	}
}

