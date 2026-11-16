package deployment

import "testing"

func TestArchiveNames(t *testing.T) {
	name := ArchiveName("20261116093000")
	if name != "runshen-deploy-20261116093000.tar.gz" || ArchiveBuildNumber(name) != "20261116093000" || !ValidArchiveName(name) {
		t.Fatalf("round trip failed for %q", name)
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
