package docprocessing

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// DocPipelineModePlanOnly means the resolved pipeline/plan is always
// computed and recorded, but processor selection stays legacy-equivalent
// (driven by the request/required list, not by the resolved pipeline's
// Processors field). This is P1's default and the safe fallback whenever
// the mode can't be determined.
const DocPipelineModePlanOnly = "plan_only"

// DocPipelineModeEnforced means a resolved pipeline with a non-empty
// Processors list actually constrains what runs: BuildProductionProcessorPlanFromFacts
// intersects the requested processors with the pipeline's declared
// Processors (see ExcludedByPolicy on ProductionProcessorPlan for what that
// excludes). A pipeline with an empty Processors list has no effect even in
// enforced mode — it hasn't opted into governing selection.
const DocPipelineModeEnforced = "enforced"

// DocPipelineModeFromEnv resolves the DOC_PIPELINE_PLAN_ONLY setting. Unset
// (or "true") resolves to DocPipelineModePlanOnly; "false" resolves to
// DocPipelineModeEnforced.
func DocPipelineModeFromEnv() (string, error) {
	return normalizeDocPipelineMode(os.Getenv("DOC_PIPELINE_PLAN_ONLY"))
}

func normalizeDocPipelineMode(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return DocPipelineModePlanOnly, nil
	}
	planOnly, err := strconv.ParseBool(raw)
	if err != nil {
		return "", fmt.Errorf("invalid DOC_PIPELINE_PLAN_ONLY %q: must be a boolean", raw)
	}
	if planOnly {
		return DocPipelineModePlanOnly, nil
	}
	return DocPipelineModeEnforced, nil
}
