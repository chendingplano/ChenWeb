package docprocessing

import (
	"context"
	"testing"
)

func TestDocProcessorFlagsContext_RoundTrip(t *testing.T) {
	ctx := withDocProcessorFlags(context.Background(), false, true)
	force, forceClear := docProcessorFlagsFromContext(ctx)
	if force != false || forceClear != true {
		t.Fatalf("got force=%v forceClear=%v, want false,true", force, forceClear)
	}
}

func TestDocProcessorFlagsContext_DefaultsWhenAbsent(t *testing.T) {
	force, forceClear := docProcessorFlagsFromContext(context.Background())
	if force != true || forceClear != false {
		t.Fatalf("got force=%v forceClear=%v, want true,false (defaults)", force, forceClear)
	}
}
