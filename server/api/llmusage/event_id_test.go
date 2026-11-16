package llmusage

import (
	"regexp"
	"testing"
)

// ChenWeb#12: usage event IDs were 32-bit random and collided on llm_usage_event_pkey.
func TestNewUsageEventIDIs128BitAndUnique(t *testing.T) {
	re := regexp.MustCompile(`^llm-[0-9a-f]{32}$`)
	seen := make(map[string]struct{}, 200000)
	for i := 0; i < 200000; i++ {
		id := newUsageEventID()
		if !re.MatchString(id) {
			t.Fatalf("id %q is not llm-<32 hex>", id)
		}
		if _, dup := seen[id]; dup {
			t.Fatalf("duplicate id %q after %d draws", id, i)
		}
		seen[id] = struct{}{}
	}
}
