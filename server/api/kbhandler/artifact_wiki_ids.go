package kbhandler

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/chendingplano/deepdoc/server/api/kbsearch"
)

func parseArtifactSearchID(raw, artifactType string) (int64, string, error) {
	raw = strings.TrimSpace(raw)
	parts := strings.SplitN(raw, "_", 3)
	if len(parts) != 3 {
		return 0, "", fmt.Errorf("invalid %s artifact id %q", artifactType, raw)
	}

	recordID, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || recordID <= 0 {
		return 0, "", fmt.Errorf("invalid %s artifact id %q", artifactType, raw)
	}
	if want := kbsearch.ArtifactTypeCode(artifactType); strings.TrimSpace(parts[1]) != want {
		return 0, "", fmt.Errorf("invalid %s artifact id %q", artifactType, raw)
	}

	tail := strings.TrimSpace(parts[2])
	if tail == "" {
		return 0, "", fmt.Errorf("invalid %s artifact id %q", artifactType, raw)
	}
	return recordID, tail, nil
}

func parseArtifactSearchIDSeq(raw, artifactType string) (int64, int, error) {
	recordID, tail, err := parseArtifactSearchID(raw, artifactType)
	if err != nil {
		return 0, 0, err
	}
	seq, err := strconv.Atoi(lastArtifactIDToken(tail))
	if err != nil || seq <= 0 {
		return 0, 0, fmt.Errorf("invalid %s artifact id %q", artifactType, raw)
	}
	return recordID, seq, nil
}

func lastArtifactIDToken(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	parts := strings.Split(raw, "_")
	return strings.TrimSpace(parts[len(parts)-1])
}
