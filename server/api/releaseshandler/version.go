package releaseshandler

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
)

// MaxPart is the largest value of a version part; parts are shown as two digits.
const MaxPart = 99

// ErrLimit means a version part would pass MaxPart.
var ErrLimit = errors.New("version limit reached")

// Version is a release number <major>.<minor>.<revision>. Zero parts mean "none created yet".
type Version struct {
	Major, Minor, Revision int
}

var versionPattern = regexp.MustCompile(`^(\d{2})\.(\d{2})\.(\d{2})$`)

// ParseVersion reads "MM.mm.rr".
func ParseVersion(s string) (Version, error) {
	m := versionPattern.FindStringSubmatch(s)
	if m == nil {
		return Version{}, fmt.Errorf("%w: version must look like 01.02.03, got %q", ErrInvalid, s)
	}
	major, _ := strconv.Atoi(m[1])
	minor, _ := strconv.Atoi(m[2])
	revision, _ := strconv.Atoi(m[3])
	return Version{major, minor, revision}, nil
}

// String formats the version as "MM.mm.rr".
func (v Version) String() string {
	return fmt.Sprintf("%02d.%02d.%02d", v.Major, v.Minor, v.Revision)
}

// Compare returns -1, 0 or 1.
func (v Version) Compare(o Version) int {
	for _, d := range [3]int{v.Major - o.Major, v.Minor - o.Minor, v.Revision - o.Revision} {
		if d < 0 {
			return -1
		}
		if d > 0 {
			return 1
		}
	}
	return 0
}

// NextMajor returns (M+1).00.00.
func (v Version) NextMajor() (Version, error) {
	if v.Major >= MaxPart {
		return v, fmt.Errorf("%w: major %02d is the last major", ErrLimit, v.Major)
	}
	return Version{v.Major + 1, 0, 0}, nil
}

// NextMinor returns M.(m+1).00.
func (v Version) NextMinor() (Version, error) {
	if v.Minor >= MaxPart {
		return v, fmt.Errorf("%w: minor %02d.%02d is the last minor of this major; create a new major", ErrLimit, v.Major, v.Minor)
	}
	return Version{v.Major, v.Minor + 1, 0}, nil
}

// NextRevision returns M.m.(r+1), the version of the next build.
func (v Version) NextRevision() (Version, error) {
	if v.Revision >= MaxPart {
		return v, fmt.Errorf("%w: %s is the last revision of %02d.%02d; create a new minor on the Releases page", ErrLimit, v, v.Major, v.Minor)
	}
	return Version{v.Major, v.Minor, v.Revision + 1}, nil
}
