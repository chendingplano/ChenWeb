package deployment

import "testing"

func TestArchiveNames(t *testing.T) {
	name := "runshen-deploy-20261116093000.tar.gz"
	if ArchiveBuildNumber(name) != "20261116093000" || !ValidArchiveName(name) {
		t.Fatalf("invalid archive or build number for %q", name)
	}
	if ArchiveBuildNumber("runshen-depoly-20261115-1103.tar.gz") != "" || !ValidArchiveName("runshen-depoly-20261115-1103.tar.gz") {
		t.Fatal("old archive name handling")
	}
	for _, bad := range []string{"", "x.tar.gz", "runshen-a.tar.gz;rm -rf ~", "runshen-$(id).tar.gz", "../runshen-a.tar.gz", "runshen-a b.tar.gz"} {
		if ValidArchiveName(bad) {
			t.Errorf("ValidArchiveName(%q) = true", bad)
		}
	}
}
