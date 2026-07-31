package candidates

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
)

// Fingerprint returns the deterministic identity of an ontology proposal over
// its normalized payload, source, and intended module (spec §9.3). Two
// identical reprocessings produce the same fingerprint, so UNIQUE(fingerprint)
// in kb.ontology_candidates makes reprocessing reuse the candidate instead of
// creating duplicate review work (spec §16.3 item 1). json.Marshal sorts map
// keys deterministically, so remarshaling canonicalizes the payload.
func Fingerprint(payload []byte, sourceType, sourceRef, moduleID string) (string, error) {
	canonical, err := canonicalJSON(payload)
	if err != nil {
		return "", fmt.Errorf("fingerprint: %w", err)
	}
	digest := sha256.Sum256([]byte(strings.Join([]string{
		string(canonical),
		strings.TrimSpace(sourceType),
		strings.TrimSpace(sourceRef),
		strings.TrimSpace(moduleID),
	}, "\x00")))
	return hex.EncodeToString(digest[:]), nil
}

func canonicalJSON(raw []byte) ([]byte, error) {
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, err
	}
	return json.Marshal(v)
}
