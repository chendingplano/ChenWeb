package buildinfo

import (
	"context"
	"encoding/json"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
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

// ListInstances must return every responder, not only the first; skipped without NATS.
func TestListInstancesFindsEveryResponder(t *testing.T) {
	nc, err := connectNATS()
	if err != nil {
		t.Skipf("NATS not reachable: %v", err)
	}
	nc.Close()
	executable := "buildinfo-test-" + strconv.Itoa(os.Getpid())
	for range 2 {
		stop, err := StartResponder(executable)
		if err != nil {
			t.Fatalf("start responder: %v", err)
		}
		t.Cleanup(stop)
	}
	processes, err := ListInstances(context.Background(), executable, 500*time.Millisecond)
	if err != nil {
		t.Fatalf("list instances: %v", err)
	}
	if len(processes) != 2 {
		t.Fatalf("got %d instances, want 2: %+v", len(processes), processes)
	}
	for _, p := range processes {
		if p.PID != os.Getpid() || p.Executable != executable {
			t.Fatalf("unexpected instance %+v", p)
		}
	}
}
