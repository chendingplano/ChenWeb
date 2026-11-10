package docprocessing

import "testing"

func TestProvisionArtifactIDIsProvID(t *testing.T) {
	cases := []struct {
		name   string
		record int64
		provID string
		rowID  int64
		want   string
	}{
		{"prov_id already prefixed is kept", 416, "416_prv_1", 52930, "416_prv_1"},
		{"surrounding spaces trimmed", 416, " 416_prv_12 ", 1, "416_prv_12"},
		{"bare prov_id is wrapped", 416, "7", 1, "416_prv_7"},
		{"other record's prefix is wrapped", 416, "415_prv_3", 1, "416_prv_415_prv_3"},
		{"empty prov_id falls back to row id", 416, "", 52930, "416_prv_52930"},
	}
	for _, c := range cases {
		if got := provisionArtifactID(c.record, c.provID, c.rowID); got != c.want {
			t.Errorf("%s: provisionArtifactID(%d, %q, %d) = %q, want %q", c.name, c.record, c.provID, c.rowID, got, c.want)
		}
	}
}
