package deployment

import "regexp"

var (
	// archiveNamePattern guards the name passed to the deploy script, which hands it to
	// scp as a remote path, so only plain file names are accepted. Builds are named
	// chenweb-deploy-<build_number>.tar.gz; any <prefix>-deploy- name is accepted so
	// archives built under an earlier prefix stay deployable.
	archiveNamePattern = regexp.MustCompile(`^[A-Za-z0-9]+-deploy-[A-Za-z0-9._-]+\.tar\.gz$`)
	archiveBuildNumber = regexp.MustCompile(`^[A-Za-z0-9]+-deploy-(\d{14})\.tar\.gz$`)
)

// ValidArchiveName reports whether name is a plain relay archive file name.
func ValidArchiveName(name string) bool { return archiveNamePattern.MatchString(name) }

// ArchiveBuildNumber returns the build number in an archive name, or "" for older names.
func ArchiveBuildNumber(name string) string {
	if m := archiveBuildNumber.FindStringSubmatch(name); m != nil {
		return m[1]
	}
	return ""
}
