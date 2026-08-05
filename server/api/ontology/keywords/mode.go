package keywords

import (
	"os"
	"strings"
	"sync"
)

var (
	resolverMode     string
	resolverModeOnce sync.Once
)

// ResolverMode returns the current KEYWORD_RESOLVER_MODE. Valid values:
// "off" (default, no keyword resolution), "observe" (resolve, write
// mentions/surfaces/backlog, no downstream consumers), "on" (deferred).
// An unset or unrecognized value is always "off" — the gate fails closed.
func ResolverMode() string {
	resolverModeOnce.Do(func() {
		resolverMode = resolverModeFrom(os.Getenv("KEYWORD_RESOLVER_MODE"))
	})
	return resolverMode
}

// resolverModeFrom normalizes the raw environment value. Anything other than
// "observe" or "on" — including the unset value — maps to "off".
func resolverModeFrom(raw string) string {
	switch strings.TrimSpace(raw) {
	case "observe", "on":
		return strings.TrimSpace(raw)
	default:
		return "off"
	}
}
