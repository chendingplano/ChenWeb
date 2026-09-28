package releaseshandler

import (
	"errors"
	"sort"
	"testing"
)

func TestNaturalVersionOrder(t *testing.T) {
	versions := []string{"2", "12", "3", "2a", "2b", "10"}
	sort.Slice(versions, func(i, j int) bool { return naturalCompare(versions[i], versions[j]) > 0 })
	want := []string{"12", "10", "3", "2b", "2a", "2"}
	for i := range want {
		if versions[i] != want[i] {
			t.Fatalf("version %d = %q, want %q", i, versions[i], want[i])
		}
	}
}

func TestValidateRelease(t *testing.T) {
	r := Release{MajorVersion: " 12 ", MinorVersion: "2a", ReleaseDate: "2026-09-28", Items: []Item{{ItemType: "bug fix", Description: " Fix crash "}}}
	if err := validate(&r); err != nil {
		t.Fatal(err)
	}
	if r.MajorVersion != "12" || r.Items[0].Description != "Fix crash" {
		t.Fatalf("fields were not trimmed: %+v", r)
	}
	r.ReleaseDate = "2026-02-30"
	if err := validate(&r); !errors.Is(err, ErrInvalid) {
		t.Fatalf("invalid date: %v", err)
	}
	r.ReleaseDate = "2026-09-28"
	r.Items[0].ItemType = "other"
	if err := validate(&r); !errors.Is(err, ErrInvalid) {
		t.Fatalf("invalid item type: %v", err)
	}
}
