package comparison

import (
	"context"
	"encoding/json"
	"testing"
)

type fakeAssertionWatermarkLoader struct {
	byObjectID map[string]int64
	gotIDs     []string
}

func (f *fakeAssertionWatermarkLoader) HighestAcceptedAssertionID(_ context.Context, objectID string) (int64, error) {
	f.gotIDs = append(f.gotIDs, objectID)
	return f.byObjectID[objectID], nil
}

func TestComputeAssertionWatermarkTracksHighestAcrossTargets(t *testing.T) {
	loader := &fakeAssertionWatermarkLoader{byObjectID: map[string]int64{"obj-1": 5, "obj-2": 90}}
	got, err := ComputeAssertionWatermark(context.Background(), ComparisonScope{TargetObjectIDs: json.RawMessage(`["obj-1", "obj-2"]`)}, loader)
	if err != nil {
		t.Fatal(err)
	}
	if got != "assertion:90" {
		t.Fatalf("watermark = %q, want assertion:90", got)
	}
	if len(loader.gotIDs) != 2 || loader.gotIDs[0] != "obj-1" || loader.gotIDs[1] != "obj-2" {
		t.Fatalf("loader queried = %v, want exactly the scope's target_object_ids", loader.gotIDs)
	}
}

func TestComputeAssertionWatermarkReturnsNoneWhenNothingAccepted(t *testing.T) {
	loader := &fakeAssertionWatermarkLoader{}
	got, err := ComputeAssertionWatermark(context.Background(), ComparisonScope{TargetObjectIDs: json.RawMessage(`["obj-1"]`)}, loader)
	if err != nil {
		t.Fatal(err)
	}
	if got != "none" {
		t.Fatalf("watermark = %q, want none", got)
	}
}
