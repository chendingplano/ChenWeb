// Package classfoundation contains the additive ADR 2026081701 primitives.
package classfoundation

import (
	"os"
	"strconv"
	"strings"
)

// Caps bounds foundation read and diagnostic surfaces. They are deliberately
// independent of writer gates: callers can use them in shadow mode without
// admitting a new assertion or changing governance behavior.
type Caps struct {
	ProfileExamples        int
	RedirectDepth          int
	ClaimShadowReportRows  int
	ReviewSameClassResults int
}

func CapsFromEnv() Caps {
	return Caps{
		ProfileExamples:        positiveEnvInt("ONTOLOGY_PROFILE_MAX_EXAMPLES", 25),
		RedirectDepth:          positiveEnvInt("ONTOLOGY_REDIRECT_MAX_DEPTH", 16),
		ClaimShadowReportRows:  positiveEnvInt("ONTOLOGY_CLAIM_SHADOW_REPORT_MAX_ROWS", 1000),
		ReviewSameClassResults: positiveEnvInt("ONTOLOGY_REVIEW_SAME_CLASS_MAX_RESULTS", 20),
	}
}

func positiveEnvInt(name string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	n, err := strconv.Atoi(value)
	if err != nil || n < 1 {
		return fallback
	}
	return n
}
