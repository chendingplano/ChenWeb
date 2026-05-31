package kbhandler

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode"
)

type artifactSearchFilters struct {
	InputRecordID     *int64   `json:"input_record_id,omitempty"`
	ArtifactTypes     []string `json:"artifact_types,omitempty"`
	CategoryPath      string   `json:"category_path,omitempty"`
	TopicType         string   `json:"topic_type,omitempty"`
	SceneType         string   `json:"scene_type,omitempty"`
	ProvisionType     string   `json:"provision_type,omitempty"`
	ProductType       string   `json:"product_type,omitempty"`
	RelationType      string   `json:"relation_type,omitempty"`
	InventoryCategory string   `json:"item_category,omitempty"`
	Manufacturer      string   `json:"manufacturer,omitempty"`
	Brand             string   `json:"brand,omitempty"`
	ModelNumber       string   `json:"model_number,omitempty"`
	PartNumber        string   `json:"part_number,omitempty"`
	ValidationStatus  string   `json:"validation_status,omitempty"`
}

type artifactSearchResult struct {
	ArtifactType    string          `json:"artifact_type,omitempty"`
	ArtifactID      string          `json:"artifact_id"`
	InputRecordID   int64           `json:"input_record_id"`
	PrimaryLabel    string          `json:"primary_label"`
	SecondaryLabel  string          `json:"secondary_label,omitempty"`
	Snippet         string          `json:"snippet"`
	Score           float64         `json:"score"`
	SourceTitle     string          `json:"source_title,omitempty"`
	SourceFilename  string          `json:"source_filename,omitempty"`
	SourceLineSpans json.RawMessage `json:"source_line_spans,omitempty"`
	SemanticPayload json.RawMessage `json:"semantic_payload,omitempty"`
}

type artifactSearchResponse struct {
	Status         bool                   `json:"status"`
	Query          string                 `json:"query"`
	ArtifactType   string                 `json:"artifact_type"`
	Page           int                    `json:"page"`
	PageSize       int                    `json:"page_size"`
	Total          int64                  `json:"total"`
	AppliedFilters artifactSearchFilters  `json:"applied_filters"`
	Results        []artifactSearchResult `json:"results"`
}

func parseRequiredSearchQuery(raw string) (string, error) {
	queryText := strings.TrimSpace(raw)
	if queryText == "" {
		return "", fmt.Errorf("q is required")
	}
	return queryText, nil
}

func parseCSVValues(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		out = append(out, part)
	}
	return out
}

func containsCJKText(raw string) bool {
	for _, r := range raw {
		if unicode.In(r, unicode.Han, unicode.Hiragana, unicode.Katakana, unicode.Hangul) {
			return true
		}
	}
	return false
}
