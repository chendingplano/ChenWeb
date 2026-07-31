package docprocessing

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// DocPipelineModePlanOnly is the only pipeline mode P1 implements: the
// resolved pipeline/plan is always computed and recorded, but processor
// selection stays legacy-equivalent (driven by the request/required list,
// not by the resolved pipeline's Processors field). There is no "enforced"
// mode yet — that is P1 Chunk 4/5 follow-up work, not implemented here.
const DocPipelineModePlanOnly = "plan_only"

// DocPipelineModeFromEnv resolves the DOC_PIPELINE_PLAN_ONLY setting. Unset
// (or "true") resolves to the only mode P1 supports, DocPipelineModePlanOnly.
// Explicitly setting it to "false" is a request for enforced pipeline
// selection, which P1 does not implement yet; that request is rejected
// rather than silently downgraded to plan-only, so a misconfigured deploy
// fails fast instead of quietly running in a mode nobody asked for.
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
	if !planOnly {
		return "", fmt.Errorf("DOC_PIPELINE_PLAN_ONLY=false requests enforced pipeline selection, which P1 does not implement yet")
	}
	return DocPipelineModePlanOnly, nil
}
