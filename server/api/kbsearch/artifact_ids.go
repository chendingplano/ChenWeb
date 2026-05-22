package kbsearch

import (
	"fmt"
	"strings"
)

func ArtifactTypeCode(artifactType string) string {
	switch strings.TrimSpace(artifactType) {
	case "summary":
		return "sum"
	case "topic":
		return "tpc"
	case "scene_block":
		return "sbk"
	case "metric":
		return "mtc"
	case "provision":
		return "prv"
	case "product":
		return "prd"
	default:
		return "art"
	}
}

func BuildArtifactID(recordID int64, artifactType string, seq string) string {
	seq = strings.TrimSpace(seq)
	if seq == "" {
		seq = "0"
	}
	return fmt.Sprintf("%d_%s_%s", recordID, ArtifactTypeCode(artifactType), seq)
}
