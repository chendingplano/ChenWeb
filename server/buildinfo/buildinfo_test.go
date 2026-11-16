package buildinfo

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestCurrentProcessWithoutEmbeddedMetadata(t *testing.T) {
	old := EmbeddedMetadataBase64
	EmbeddedMetadataBase64 = ""
	t.Cleanup(func() { EmbeddedMetadataBase64 = old })
	process, err := CurrentProcess("server")
	if err != nil || process.Status != "running" || process.PID != os.Getpid() || process.StartedAt.IsZero() || process.BuildNumber != "" {
		t.Fatalf("expected running process without build metadata, got %+v, err %v", process, err)
	}
}

func TestUnreportedProcessOmitsStartTime(t *testing.T) {
	data, err := json.Marshal(Process{Executable: "worker", Status: "unreported"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "started_at") {
		t.Fatalf("unexpected start time: %s", data)
	}
}
