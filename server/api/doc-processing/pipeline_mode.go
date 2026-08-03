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

// DocPipelineOnConflictFromEnv resolves the DOC_PIPELINE_ON_CONFLICT
// setting spec 2026080102 sections 5.1/5.2 define for both pipeline-binding
// and processor-gate conflict/indeterminacy resolution -- previously never
// read anywhere, leaving bindings hardcoded to PipelineBindingOnConflictBlock
// and gates hardcoded to PipelineBindingOnConflictFallback (P5 review
// 2026080302 finding P5-19). Unset defaults to block (fail closed before any
// processor runs), matching bindings' prior hardcoded default and the
// spec's fail-closed philosophy; this is a deliberate default-mode change
// for gates, which previously always used fallback.
func DocPipelineOnConflictFromEnv() (string, error) {
	return normalizeDocPipelineOnConflict(os.Getenv("DOC_PIPELINE_ON_CONFLICT"))
}

func normalizeDocPipelineOnConflict(raw string) (string, error) {
	raw = strings.ToLower(strings.TrimSpace(raw))
	switch raw {
	case "":
		return PipelineBindingOnConflictBlock, nil
	case PipelineBindingOnConflictBlock, PipelineBindingOnConflictFallback:
		return raw, nil
	default:
		return "", fmt.Errorf("invalid DOC_PIPELINE_ON_CONFLICT %q: must be %q or %q", raw, PipelineBindingOnConflictBlock, PipelineBindingOnConflictFallback)
	}
}
