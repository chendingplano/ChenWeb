package productreviews

import (
	"encoding/json"
	"strings"
	"testing"
)

func acceptedNode(id int64, kind, label, grounding string) ProfileNode {
	return ProfileNode{ID: id, NodeKind: kind, Label: label, Grounding: grounding, Status: StatusAccepted}
}

// Scenario: Node with no metrics is reported as a gap + Coverage separates
// attributed from scoped-only counts (spec: Coverage and gap reporting).
func TestBuildReportGapsAndCounts(t *testing.T) {
	profile := &Profile{ID: 7, Version: 3}
	nodes := []ProfileNode{
		acceptedNode(1, KindProduct, "Ventilator", GroundingObjectNode),
		acceptedNode(2, KindPart, "Display", GroundingObjectNode),
		acceptedNode(3, KindPart, "Humidifier", GroundingUngrounded),
	}
	n2 := int64(2)
	res := RetrieveResult{
		AttributedCount:    2,
		DocumentScopeCount: 1,
		TruncatedCount:     4,
		Results: []Result{
			{ArtifactType: "metric", ArtifactID: "42_mtc_1", InputRecordID: 42, NodeID: &n2, Tier: TierPart},
			{ArtifactType: "metric", ArtifactID: "43_mtc_1", InputRecordID: 43, NodeID: &n2, Tier: TierPart},
			{ArtifactType: "metric", ArtifactID: "42_mtc_9", InputRecordID: 42, Tier: TierDocumentScope}, // no node
		},
	}
	rep := BuildReport(profile, nodes, []string{"metric"}, []ScopedDoc{{InputRecordID: 42}, {InputRecordID: 43}}, res)

	if rep.ProfileVersion != 3 || rep.ScopedDocumentCount != 2 {
		t.Fatalf("report meta = v%d / %d scoped, want v3 / 2", rep.ProfileVersion, rep.ScopedDocumentCount)
	}
	byNode := map[int64]NodeCoverage{}
	for _, c := range rep.Coverage {
		byNode[c.NodeID] = c
	}
	if byNode[2].ArtifactCount != 2 || byNode[2].DocumentCount != 2 {
		t.Fatalf("Display coverage = %+v, want 2 artifacts / 2 docs (document_scope excluded)", byNode[2])
	}
	if byNode[1].ArtifactCount != 0 || byNode[3].ArtifactCount != 0 {
		t.Fatalf("root/humidifier should have 0 attributed artifacts: %+v / %+v", byNode[1], byNode[3])
	}
	gapLabels := map[string]string{}
	for _, g := range rep.Gaps {
		gapLabels[g.Label] = g.Grounding
	}
	if _, ok := gapLabels["Humidifier"]; !ok {
		t.Fatalf("Humidifier missing from gaps: %+v", rep.Gaps)
	}
	if gapLabels["Humidifier"] != GroundingUngrounded {
		t.Fatalf("gap should carry grounding state, got %q", gapLabels["Humidifier"])
	}
	if _, ok := gapLabels["Display"]; ok {
		t.Fatal("Display matched 2 metrics; must not be a gap")
	}
}

// Scenario: Report is available in both forms (spec) — structured + markdown,
// with the truncation disclosed.
func TestRenderReportMarkdown(t *testing.T) {
	rep := BuildReport(&Profile{ID: 7, Version: 2}, []ProfileNode{acceptedNode(1, KindProduct, "Ventilator", GroundingObjectNode)},
		[]string{"metric"}, nil, RetrieveResult{TruncatedCount: 5})
	raw, err := json.Marshal(rep)
	if err != nil || len(raw) == 0 {
		t.Fatalf("report_json marshal failed: %v", err)
	}
	md := RenderReportMarkdown(rep)
	if !strings.Contains(md, "Coverage by scope node") || !strings.Contains(md, "Coverage gaps") {
		t.Fatalf("markdown missing sections:\n%s", md)
	}
	if !strings.Contains(md, "5 result(s) truncated") {
		t.Fatalf("markdown does not disclose truncation:\n%s", md)
	}
}
