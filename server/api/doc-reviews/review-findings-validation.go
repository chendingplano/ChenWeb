package docreviews

import (
	"fmt"
	"strings"
)

// validatedFindingsJSON distinguishes a completed empty assessment from a model
// response that omitted or corrupted its result. Normalization supplies optional
// display defaults only after validating the required result structure.
func validatedFindingsJSON(payload map[string]any, modelName string) ([]ReviewFinding, error) {
	raw, ok := payload["findings"]
	if !ok {
		return nil, fmt.Errorf("review model response: missing findings array")
	}
	arr, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("review model response: findings must be an array")
	}
	for i, item := range arr {
		finding, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("review model response: finding %d must be an object", i)
		}
		title, ok := finding["title"].(string)
		if !ok || strings.TrimSpace(title) == "" {
			return nil, fmt.Errorf("review model response: finding %d requires a title", i)
		}
	}
	return normalizeFindingsJSON(payload, modelName), nil
}
