package main

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/chendingplano/deepdoc/server/api/docsync/transfer"
)

func TestForcedCommandKeepsDiagnosticsOffStdout(t *testing.T) {
	if os.Getenv("DOCSYNC_TEST_SUBPROCESS") == "1" {
		os.Args = os.Args[:1]
		os.Exit(runExport())
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestForcedCommandKeepsDiagnosticsOffStdout$")
	cmd.Dir = t.TempDir()
	cmd.Env = []string{
		"DOCSYNC_TEST_SUBPROCESS=1",
		"SSH_ORIGINAL_COMMAND=export status --run 1",
	}
	cmd.Stdin = strings.NewReader("{}")
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr

	var exit *exec.ExitError
	if err := cmd.Run(); !errors.As(err, &exit) || exit.ExitCode() != transfer.SessionExitCode {
		t.Fatalf("status without authentication was not rejected: %v", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("export stdout contains %d diagnostic bytes", stdout.Len())
	}
	if !strings.Contains(stderr.String(), "authentication denied") {
		t.Fatal("missing authentication failure diagnostic on stderr")
	}
}
