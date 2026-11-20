package deployment

import "testing"

func TestArchiveNames(t *testing.T) {
	name := "chenweb-deploy-20261116093000.tar.gz"
	if ArchiveBuildNumber(name) != "20261116093000" || !ValidArchiveName(name) {
		t.Fatalf("invalid archive or build number for %q", name)
	}
	// Archives built under an earlier prefix stay deployable.
	if old := "legacy-deploy-20261115093000.tar.gz"; ArchiveBuildNumber(old) != "20261115093000" || !ValidArchiveName(old) {
		t.Fatalf("earlier-prefix archive %q rejected", old)
	}
	for _, bad := range []string{"", "x.tar.gz", "backup-20261115.tar.gz", "chenweb-deploy-a.tar.gz;rm -rf ~",
		"chenweb-deploy-$(id).tar.gz", "../chenweb-deploy-a.tar.gz", "chenweb-deploy-a b.tar.gz"} {
		if ValidArchiveName(bad) {
			t.Errorf("ValidArchiveName(%q) = true", bad)
		}
	}
}
