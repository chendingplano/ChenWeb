package classfoundation

import (
	"context"
	"testing"
)

type mapRedirectLookup map[string]string

func (m mapRedirectLookup) ActiveRedirect(_ context.Context, source string) (string, bool, error) {
	target, ok := m[source]
	return target, ok, nil
}

func TestRedirectResolverReturnsTerminalAndTraversal(t *testing.T) {
	result := (RedirectResolver{Lookup: mapRedirectLookup{"a": "b", "b": "c"}, DepthCap: 3}).Resolve(context.Background(), "a")
	if result.Status != RedirectResolved || result.TerminalTarget != "c" || len(result.Traversal) != 3 {
		t.Fatalf("resolution = %#v", result)
	}
}

func TestRedirectResolverReportsDepthLimitWithoutInferredTarget(t *testing.T) {
	result := (RedirectResolver{Lookup: mapRedirectLookup{"a": "b", "b": "c"}, DepthCap: 1}).Resolve(context.Background(), "a")
	if result.Status != RedirectDepthLimit || result.TerminalTarget != "" || result.Reason == "" {
		t.Fatalf("resolution = %#v", result)
	}
}

func TestRedirectResolverReportsCycleAsUnresolved(t *testing.T) {
	result := (RedirectResolver{Lookup: mapRedirectLookup{"a": "b", "b": "a"}, DepthCap: 3}).Resolve(context.Background(), "a")
	if result.Status != RedirectUnresolved || result.Reason != "cycle_detected" || result.TerminalTarget != "" {
		t.Fatalf("resolution = %#v", result)
	}
}
