package classfoundation

import (
	"context"
	"strings"
)

const (
	RedirectResolved   = "resolved"
	RedirectUnresolved = "unresolved"
	RedirectDepthLimit = "depth_limit"
)

// RedirectLookup lets the same resolver operate over term or assertion
// redirect stores without making one identity family depend on the other.
type RedirectLookup interface {
	ActiveRedirect(context.Context, string) (target string, found bool, err error)
}

type RedirectResolution struct {
	TerminalTarget string
	Traversal      []string
	Status         string
	Reason         string
}

type RedirectResolver struct {
	Lookup   RedirectLookup
	DepthCap int
}

func (r RedirectResolver) Resolve(ctx context.Context, source string) RedirectResolution {
	source = strings.TrimSpace(source)
	if source == "" {
		return RedirectResolution{Status: RedirectUnresolved, Reason: "empty_source"}
	}
	if r.Lookup == nil {
		return RedirectResolution{Status: RedirectUnresolved, Reason: "lookup_unavailable", Traversal: []string{source}}
	}
	cap := r.DepthCap
	if cap < 1 {
		cap = CapsFromEnv().RedirectDepth
	}
	current := source
	traversal := []string{source}
	seen := map[string]bool{source: true}
	for step := 0; ; step++ {
		target, found, err := r.Lookup.ActiveRedirect(ctx, current)
		if err != nil {
			return RedirectResolution{Status: RedirectUnresolved, Reason: "lookup_error", Traversal: traversal}
		}
		if !found {
			return RedirectResolution{Status: RedirectResolved, TerminalTarget: current, Traversal: traversal}
		}
		target = strings.TrimSpace(target)
		if target == "" {
			return RedirectResolution{Status: RedirectUnresolved, Reason: "empty_target", Traversal: traversal}
		}
		if step >= cap {
			return RedirectResolution{Status: RedirectDepthLimit, Reason: "depth_cap_reached", Traversal: traversal}
		}
		if seen[target] {
			return RedirectResolution{Status: RedirectUnresolved, Reason: "cycle_detected", Traversal: append(traversal, target)}
		}
		seen[target] = true
		traversal = append(traversal, target)
		current = target
	}
}
