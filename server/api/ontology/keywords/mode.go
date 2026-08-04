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
func ResolverMode() string {
	resolverModeOnce.Do(func() {
		resolverMode = "off"
		raw := strings.TrimSpace(os.Getenv("KEYWORD_RESOLVER_MODE"))
		switch raw {
		case "observe", "on":
			resolverMode = raw
		}
	})
	return resolverMode
}

// IsObserveMode returns true when the resolver is in observe mode.
func IsObserveMode() bool {
	return ResolverMode() == "observe"
}
