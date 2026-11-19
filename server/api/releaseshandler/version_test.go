package releaseshandler

import (
	"errors"
	"testing"
)

func TestVersionFormatAndParse(t *testing.T) {
	v := Version{1, 2, 3}
	if v.String() != "01.02.03" {
		t.Fatalf("String = %q", v.String())
	}
	got, err := ParseVersion("01.02.03")
	if err != nil || got != v {
		t.Fatalf("ParseVersion = %v, %v", got, err)
	}
	for _, bad := range []string{"1.2.3", "01.02", "01.02.03.04", "aa.bb.cc", ""} {
		if _, err := ParseVersion(bad); !errors.Is(err, ErrInvalid) {
			t.Errorf("ParseVersion(%q) err = %v, want ErrInvalid", bad, err)
		}
	}
}

func TestVersionArithmetic(t *testing.T) {
	cur := Version{1, 2, 5}
	if v, _ := cur.NextMajor(); v != (Version{2, 0, 0}) {
		t.Errorf("NextMajor = %v", v)
	}
	if v, _ := cur.NextMinor(); v != (Version{1, 3, 0}) {
		t.Errorf("NextMinor = %v", v)
	}
	if v, _ := cur.NextRevision(); v != (Version{1, 2, 6}) {
		t.Errorf("NextRevision = %v", v)
	}
	if v, _ := (Version{2, 0, 0}).NextRevision(); v != (Version{2, 0, 1}) {
		t.Errorf("build after a new major = %v", v)
	}
	if v, _ := (Version{}).NextRevision(); v.String() != "00.00.01" {
		t.Errorf("first build = %v", v)
	}
}

func TestVersionLimits(t *testing.T) {
	if _, err := (Version{1, 2, 99}).NextRevision(); !errors.Is(err, ErrLimit) {
		t.Errorf("revision 99: %v", err)
	}
	if _, err := (Version{1, 99, 3}).NextMinor(); !errors.Is(err, ErrLimit) {
		t.Errorf("minor 99: %v", err)
	}
	if _, err := (Version{99, 1, 3}).NextMajor(); !errors.Is(err, ErrLimit) {
		t.Errorf("major 99: %v", err)
	}
}

func TestVersionCompare(t *testing.T) {
	a, b := Version{1, 2, 10}, Version{1, 3, 0}
	if a.Compare(b) != -1 || b.Compare(a) != 1 || a.Compare(a) != 0 {
		t.Fatal("Compare order wrong")
	}
}
