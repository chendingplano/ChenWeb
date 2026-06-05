package kbhandler

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/EchoFactory"
	"github.com/labstack/echo/v4"
)

func SearchSummaries(c echo.Context) error {
	return searchRegistryArtifacts(c, "summary")
}

func SearchTopics(c echo.Context) error {
	return searchRegistryArtifacts(c, "topic")
}

func SearchSceneBlocks(c echo.Context) error {
	return searchRegistryArtifacts(c, "scene_block")
}

func SearchProvisions(c echo.Context) error {
	return searchRegistryArtifacts(c, "provision")
}

func SearchProducts(c echo.Context) error {
	return searchRegistryArtifacts(c, "product")
}

func SearchAllArtifacts(c echo.Context) error {
	return searchRegistryArtifacts(c, "all")
}

func searchRegistryArtifacts(c echo.Context, artifactType string) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_GSR_001")
	defer rc.Close()
	logger := rc.GetLogger()

	queryText, err := parseRequiredSearchQuery(c.QueryParam("q"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{
			Status:   false,
			ErrorMsg: fmt.Sprintf("%v (CWB_KB_GSR_010)", err),
		})
	}

	cfg := loadRegistrySearchConfig()
	page := parsePositiveInt(c.QueryParam("page"), 1)
	pageSize := parsePositiveInt(c.QueryParam("page_size"), cfg.defaultPageSize)
	if pageSize > cfg.maxPageSize {
		pageSize = cfg.maxPageSize
	}

	inputRecordID, err := parseOptionalPositiveInt64(c.QueryParam("input_record_id"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{
			Status:   false,
			ErrorMsg: fmt.Sprintf("invalid input_record_id: %v (CWB_KB_GSR_011)", err),
		})
	}
	filters := artifactSearchFilters{
		InputRecordID:     inputRecordID,
		CategoryPath:      strings.TrimSpace(c.QueryParam("category_path")),
		TopicType:         strings.TrimSpace(c.QueryParam("topic_type")),
		SceneType:         strings.TrimSpace(c.QueryParam("scene_type")),
		ProvisionType:     strings.TrimSpace(c.QueryParam("provision_type")),
		ProductType:       strings.TrimSpace(c.QueryParam("product_type")),
		RelationType:      strings.TrimSpace(c.QueryParam("relation_type")),
		InventoryCategory: strings.TrimSpace(c.QueryParam("item_categories")),
		Manufacturer:      strings.TrimSpace(c.QueryParam("manufacturer")),
		Brand:             strings.TrimSpace(c.QueryParam("brand")),
		ModelNumber:       strings.TrimSpace(c.QueryParam("model_number")),
		PartNumber:        strings.TrimSpace(c.QueryParam("part_number")),
		ValidationStatus:  strings.TrimSpace(c.QueryParam("validation_status")),
	}
	if strings.TrimSpace(artifactType) == "all" {
		filters.ArtifactTypes = parseCSVValues(c.QueryParam("artifact_types"))
	}

	total, err := countRegistrySearchResults(ApiTypes.ProjectDBHandle, artifactType, queryText, filters, cfg)
	if err != nil {
		logger.Error("count registry search failed", "artifact_type", artifactType, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{
			Status:   false,
			ErrorMsg: "failed to count search results (CWB_KB_GSR_012)",
		})
	}

	results, err := queryRegistrySearchResults(ApiTypes.ProjectDBHandle, artifactType, queryText, filters, page, pageSize, cfg)
	if err != nil {
		logger.Error("query registry search failed", "artifact_type", artifactType, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{
			Status:   false,
			ErrorMsg: "failed to search artifacts (CWB_KB_GSR_013)",
		})
	}

	return c.JSON(http.StatusOK, artifactSearchResponse{
		Status:         true,
		Query:          queryText,
		ArtifactType:   strings.TrimSpace(artifactType),
		Page:           page,
		PageSize:       pageSize,
		Total:          total,
		AppliedFilters: filters,
		Results:        results,
	})
}
