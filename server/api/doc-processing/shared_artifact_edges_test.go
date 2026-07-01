package docprocessing

import "testing"

func TestBuildSharedArtifactEdges(t *testing.T) {
	overlaps := []sharedArtifactOverlap{
		{anchorType: "metric", anchorID: "100_mtc_3", selfID: "100_prv_1"},
		{anchorType: "entity", anchorID: "100_ent_5", selfID: "100_prv_1"},
		{anchorType: "topic", anchorID: "100_tpc_2", selfID: "100_prv_2"},
	}

	conns := buildSharedArtifactEdges(100, searchArtifactProvision, "extract_provisions", overlaps)
	if len(conns) != 3 {
		t.Fatalf("edges = %d, want 3", len(conns))
	}

	c := conns[0]
	// Anchor is the SOURCE, provision is the TARGET; both endpoints intra-document.
	if c.SourceType != "metric" || c.SourceID != "100_mtc_3" {
		t.Errorf("source = %s/%s, want metric/100_mtc_3", c.SourceType, c.SourceID)
	}
	if c.TargetType != searchArtifactProvision || c.TargetID != "100_prv_1" {
		t.Errorf("target = %s/%s, want provision/100_prv_1", c.TargetType, c.TargetID)
	}
	if c.SourceRecordID != 100 || c.TargetRecordID != 100 {
		t.Errorf("record ids = %d/%d, want 100/100", c.SourceRecordID, c.TargetRecordID)
	}
	if c.RelationName != RelationSharedArtifact || c.RelationMethod != RelationMethodLineOverlapArtifact {
		t.Errorf("relation = %s/%s, want %s/%s", c.RelationName, c.RelationMethod, RelationSharedArtifact, RelationMethodLineOverlapArtifact)
	}
	if c.Confidence != 1.0 {
		t.Errorf("confidence = %v, want 1.0", c.Confidence)
	}
	if c.ExtraInfo["source"] != "extract_provisions" || c.ExtraInfo["anchor_type"] != "metric" {
		t.Errorf("extra_info = %#v, want source=extract_provisions anchor_type=metric", c.ExtraInfo)
	}

	// anchor_type is per-edge (mirrors the source type).
	if conns[1].ExtraInfo["anchor_type"] != "entity" || conns[2].ExtraInfo["anchor_type"] != "topic" {
		t.Errorf("anchor_type not per-edge: %v, %v", conns[1].ExtraInfo["anchor_type"], conns[2].ExtraInfo["anchor_type"])
	}
}

func TestBuildSharedArtifactEdges_Empty(t *testing.T) {
	if got := buildSharedArtifactEdges(1, searchArtifactProvision, "extract_provisions", nil); len(got) != 0 {
		t.Fatalf("edges = %d, want 0", len(got))
	}
}
