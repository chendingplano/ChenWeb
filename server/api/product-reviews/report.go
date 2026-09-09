package productreviews

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// NodeCoverage is one row of a run report's per-node coverage table. ArtifactCount
// counts attributed results only (tiers direct / part / aspect), never
// document_scope (spec: Coverage and gap reporting).
type NodeCoverage struct {
	NodeID        int64  `json:"node_id"`
	Label         string `json:"label"`
	Kind          string `json:"kind"`
	Grounding     string `json:"grounding"`
	ArtifactCount int    `json:"artifact_count"`
	DocumentCount int    `json:"document_count"`
}

// GapEntry is an accepted scope node that matched zero artifacts.
type GapEntry struct {
	NodeID    int64  `json:"node_id"`
	Label     string `json:"label"`
	Kind      string `json:"kind"`
	Grounding string `json:"grounding"`
}

// Report is the structured run report stored as report_json.
type Report struct {
	ProfileID            int64          `json:"profile_id"`
	ProfileVersion       int            `json:"profile_version"`
	ArtifactTypes        []string       `json:"artifact_types"`
	Coverage             []NodeCoverage `json:"coverage"`
	Gaps                 []GapEntry     `json:"gaps"`
	AttributedCount      int            `json:"attributed_count"`
	DocumentScopeCount   int            `json:"document_scope_count"`
	ScopedDocumentCount  int            `json:"scoped_document_count"`
	ResultCount          int            `json:"result_count"`
	TruncatedCount       int            `json:"truncated_count"`
	PerDocumentTruncated map[int64]int  `json:"per_document_truncated,omitempty"`
	GeneratedAt          time.Time      `json:"generated_at"`
}

// BuildReport assembles the coverage table, gap list, and counts for a
// completed run. Deterministic given identical inputs.
func BuildReport(profile *Profile, nodes []ProfileNode, artifactTypes []string, scoped []ScopedDoc, res RetrieveResult) Report {
	countByNode := map[int64]int{}
	docsByNode := map[int64]map[int64]bool{}
	for _, r := range res.Results {
		if r.NodeID == nil {
			continue
		}
		nid := *r.NodeID
		countByNode[nid]++
		if docsByNode[nid] == nil {
			docsByNode[nid] = map[int64]bool{}
		}
		docsByNode[nid][r.InputRecordID] = true
	}

	rep := Report{
		ProfileID:            profile.ID,
		ProfileVersion:       profile.Version,
		ArtifactTypes:        append([]string(nil), artifactTypes...),
		AttributedCount:      res.AttributedCount,
		DocumentScopeCount:   res.DocumentScopeCount,
		ScopedDocumentCount:  len(scoped),
		ResultCount:          len(res.Results),
		TruncatedCount:       res.TruncatedCount,
		PerDocumentTruncated: nonEmptyIntMap(res.PerDocumentTruncated),
		GeneratedAt:          time.Now().UTC(),
	}

	for _, n := range nodes {
		if n.Status != StatusAccepted {
			continue
		}
		cov := NodeCoverage{
			NodeID: n.ID, Label: n.Label, Kind: n.NodeKind, Grounding: n.Grounding,
			ArtifactCount: countByNode[n.ID], DocumentCount: len(docsByNode[n.ID]),
		}
		rep.Coverage = append(rep.Coverage, cov)
		if cov.ArtifactCount == 0 {
			rep.Gaps = append(rep.Gaps, GapEntry{NodeID: n.ID, Label: n.Label, Kind: n.NodeKind, Grounding: n.Grounding})
		}
	}
	sort.SliceStable(rep.Coverage, func(i, j int) bool { return rep.Coverage[i].NodeID < rep.Coverage[j].NodeID })
	sort.SliceStable(rep.Gaps, func(i, j int) bool { return rep.Gaps[i].NodeID < rep.Gaps[j].NodeID })
	return rep
}

// RenderReportMarkdown renders a Report as the report_md text.
func RenderReportMarkdown(r Report) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Product Metric Review — profile %d (version %d)\n\n", r.ProfileID, r.ProfileVersion)
	fmt.Fprintf(&b, "- Artifact types: %s\n", strings.Join(r.ArtifactTypes, ", "))
	fmt.Fprintf(&b, "- Attributed results: **%d** · document-scope results: **%d** · total: **%d**\n",
		r.AttributedCount, r.DocumentScopeCount, r.ResultCount)
	fmt.Fprintf(&b, "- Documents in scope: %d\n", r.ScopedDocumentCount)
	if r.TruncatedCount > 0 {
		fmt.Fprintf(&b, "- ⚠️ %d result(s) truncated against the configured caps\n", r.TruncatedCount)
	}
	b.WriteString("\n## Coverage by scope node\n\n")
	b.WriteString("| Node | Kind | Grounding | Metrics | Documents |\n|---|---|---|---:|---:|\n")
	for _, c := range r.Coverage {
		fmt.Fprintf(&b, "| %s | %s | %s | %d | %d |\n", c.Label, c.Kind, c.Grounding, c.ArtifactCount, c.DocumentCount)
	}
	b.WriteString("\n## Coverage gaps (accepted nodes with zero metrics)\n\n")
	if len(r.Gaps) == 0 {
		b.WriteString("_None — every accepted scope node matched at least one metric._\n")
	} else {
		for _, g := range r.Gaps {
			fmt.Fprintf(&b, "- **%s** (%s, grounding: %s)\n", g.Label, g.Kind, g.Grounding)
		}
	}
	return b.String()
}

func nonEmptyIntMap(m map[int64]int) map[int64]int {
	if len(m) == 0 {
		return nil
	}
	return m
}
