package comparison

import (
	"context"
	"encoding/json"
	"fmt"
)

const noAssertionsWatermark = "none"

// AssertionWatermarkLoader resolves the highest accepted assertion id for one
// target object. ComputeAssertionWatermark uses it to derive a comparison
// run's watermark from governed state, never from a caller-supplied value.
type AssertionWatermarkLoader interface {
	HighestAcceptedAssertionID(ctx context.Context, objectID string) (int64, error)
}

// ComputeAssertionWatermark derives a reproducible watermark from the scope's
// own pinned target_object_ids: the highest accepted assertion id across all
// of them, or "none" if none exist yet.
func ComputeAssertionWatermark(ctx context.Context, scope ComparisonScope, loader AssertionWatermarkLoader) (string, error) {
	var targetObjectIDs []string
	if err := json.Unmarshal(scope.TargetObjectIDs, &targetObjectIDs); err != nil {
		return "", fmt.Errorf("target_object_ids: %w", err)
	}
	var maxID int64
	for _, objectID := range targetObjectIDs {
		id, err := loader.HighestAcceptedAssertionID(ctx, objectID)
		if err != nil {
			return "", err
		}
		if id > maxID {
			maxID = id
		}
	}
	if maxID == 0 {
		return noAssertionsWatermark, nil
	}
	return fmt.Sprintf("assertion:%d", maxID), nil
}
