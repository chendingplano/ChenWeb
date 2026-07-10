package docreviews

import (
	"reflect"
	"testing"
)

func TestClassifyReviewOutput(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		findingType string
		want        string
	}{
		{name: "exact analysis", findingType: "analysis", want: "analysis"},
		{name: "trimmed analysis", findingType: " Analysis ", want: "analysis"},
		{name: "case-insensitive analysis", findingType: "ANALYSIS", want: "analysis"},
		{name: "blank finding type", findingType: "", want: "finding"},
		{name: "whitespace finding type", findingType: "   ", want: "finding"},
		{name: "known non-analysis type", findingType: "issue", want: "finding"},
		{name: "unknown non-analysis type", findingType: "custom_note", want: "finding"},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := classifyReviewOutput(ReviewFinding{FindingType: tc.findingType})
			if got != tc.want {
				t.Fatalf("classifyReviewOutput(%q) = %q, want %q", tc.findingType, got, tc.want)
			}
		})
	}
}

func TestReviewWorkGateClaimAndLimits(t *testing.T) {
	t.Parallel()

	gate := newReviewWorkGate(1, 1, []int{5, 1, 4})

	first, ok := gate.claimNext()
	if !ok || first != 5 {
		t.Fatalf("first claimNext() = (%d, %t), want (5, true)", first, ok)
	}
	second, ok := gate.claimNext()
	if !ok || second != 1 {
		t.Fatalf("second claimNext() = (%d, %t), want (1, true)", second, ok)
	}

	if got := gate.unclaimedIndexes(6); !reflect.DeepEqual(got, []int{0, 2, 3, 4}) {
		t.Fatalf("unclaimedIndexes(6) after claims = %v, want [0 2 3 4]", got)
	}

	gate.complete([]ReviewFinding{{FindingType: "issue"}})

	if !gate.reached() {
		t.Fatalf("reached() = false, want true after completing first finding")
	}
	if third, ok := gate.claimNext(); ok {
		t.Fatalf("claimNext() after limit reached = (%d, %t), want (_, false)", third, ok)
	}

	gate.complete([]ReviewFinding{{FindingType: "analysis"}})

	snapshot := gate.snapshot()
	if snapshot.Findings != 1 || snapshot.Analyses != 1 {
		t.Fatalf("snapshot counts = (%d findings, %d analyses), want (1, 1)", snapshot.Findings, snapshot.Analyses)
	}
}

func TestReviewWorkGateAlreadyClaimedTasksMayOvershoot(t *testing.T) {
	t.Parallel()

	gate := newReviewWorkGate(1, 2, []int{2, 0, 1})

	first, ok := gate.claimNext()
	if !ok || first != 2 {
		t.Fatalf("first claimNext() = (%d, %t), want (2, true)", first, ok)
	}
	second, ok := gate.claimNext()
	if !ok || second != 0 {
		t.Fatalf("second claimNext() = (%d, %t), want (0, true)", second, ok)
	}

	gate.complete([]ReviewFinding{{FindingType: "issue"}})
	if _, ok := gate.claimNext(); ok {
		t.Fatalf("claimNext() after finding limit reached = (_, true), want (_, false)")
	}

	gate.complete([]ReviewFinding{{FindingType: "unknown_type"}})

	snapshot := gate.snapshot()
	if snapshot.Findings != 2 || snapshot.Analyses != 0 {
		t.Fatalf("snapshot counts after overshoot = (%d findings, %d analyses), want (2, 0)", snapshot.Findings, snapshot.Analyses)
	}
}

func TestReviewWorkGateReplaceQueueUsesCopiedRemainderOrder(t *testing.T) {
	t.Parallel()

	gate := newReviewWorkGate(4, 4, []int{2, 0, 4})

	first, ok := gate.claimNext()
	if !ok || first != 2 {
		t.Fatalf("first claimNext() = (%d, %t), want (2, true)", first, ok)
	}
	second, ok := gate.claimNext()
	if !ok || second != 0 {
		t.Fatalf("second claimNext() = (%d, %t), want (0, true)", second, ok)
	}

	remainder := []int{4, 1, 3}
	gate.replaceQueue(remainder)
	remainder[0] = 99

	third, ok := gate.claimNext()
	if !ok || third != 4 {
		t.Fatalf("third claimNext() after replaceQueue = (%d, %t), want (4, true)", third, ok)
	}

	if got := gate.unclaimedIndexes(5); !reflect.DeepEqual(got, []int{1, 3}) {
		t.Fatalf("unclaimedIndexes(5) after replaceQueue = %v, want [1 3]", got)
	}
}
