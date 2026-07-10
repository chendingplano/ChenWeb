package docreviews

import (
	"slices"
	"strings"
	"sync"
)

type reviewOutputSnapshot struct {
	Findings int
	Analyses int
	Queue    []int
	Claimed  []int
}

type reviewWorkGate struct {
	mu                     sync.Mutex
	maxFindings, maxAnalyses int
	findings, analyses     int
	queue                  []int
	claimed                map[int]struct{}
}

func newReviewWorkGate(maxFindings, maxAnalyses int, queue []int) *reviewWorkGate {
	copiedQueue := append([]int(nil), queue...)
	return &reviewWorkGate{
		maxFindings: maxFindings,
		maxAnalyses: maxAnalyses,
		queue:       copiedQueue,
		claimed:     make(map[int]struct{}, len(copiedQueue)),
	}
}

func classifyReviewOutput(finding ReviewFinding) string {
	if strings.EqualFold(strings.TrimSpace(finding.FindingType), "analysis") {
		return "analysis"
	}
	return "finding"
}

func (g *reviewWorkGate) claimNext() (int, bool) {
	g.mu.Lock()
	defer g.mu.Unlock()

	if g.findings >= g.maxFindings || g.analyses >= g.maxAnalyses || len(g.queue) == 0 {
		return 0, false
	}

	next := g.queue[0]
	g.queue = g.queue[1:]
	g.claimed[next] = struct{}{}
	return next, true
}

func (g *reviewWorkGate) replaceQueue(queue []int) {
	g.mu.Lock()
	defer g.mu.Unlock()

	g.queue = append([]int(nil), queue...)
}

func (g *reviewWorkGate) complete(findings []ReviewFinding) {
	g.mu.Lock()
	defer g.mu.Unlock()

	for _, finding := range findings {
		if classifyReviewOutput(finding) == "analysis" {
			g.analyses++
			continue
		}
		g.findings++
	}
}

func (g *reviewWorkGate) reached() bool {
	g.mu.Lock()
	defer g.mu.Unlock()

	return g.findings >= g.maxFindings || g.analyses >= g.maxAnalyses
}

func (g *reviewWorkGate) unclaimedIndexes(total int) []int {
	g.mu.Lock()
	defer g.mu.Unlock()

	unclaimed := make([]int, 0, total)
	for i := range total {
		if _, ok := g.claimed[i]; ok {
			continue
		}
		unclaimed = append(unclaimed, i)
	}
	return unclaimed
}

func (g *reviewWorkGate) snapshot() reviewOutputSnapshot {
	g.mu.Lock()
	defer g.mu.Unlock()

	claimed := make([]int, 0, len(g.claimed))
	for idx := range g.claimed {
		claimed = append(claimed, idx)
	}
	slices.Sort(claimed)

	return reviewOutputSnapshot{
		Findings: g.findings,
		Analyses: g.analyses,
		Queue:    append([]int(nil), g.queue...),
		Claimed:  claimed,
	}
}
