package processhealth

import (
	"context"
	"testing"
	"time"
)

func TestFunctionalProbeReportsOperatorIssueThenRecovery(t *testing.T) {
	r, err := NewReporterAt(t.TempDir(), "worker")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	issues := make(chan *Issue, 2)
	issues <- &Issue{Reason: "model_unavailable", Resource: "MODEL_CONFIG", Message: "Configured model is not available", Recovery: OperatorAction}
	r.GoCheck(ctx, 10*time.Millisecond, func(ctx context.Context) *Issue {
		select {
		case x := <-issues:
			return x
		case <-ctx.Done():
			return nil
		}
	}, nil)
	awaitState := func(want State) {
		t.Helper()
		deadline := time.After(time.Second)
		for r.Snapshot().State != want {
			select {
			case <-deadline:
				t.Fatalf("state=%s, want %s", r.Snapshot().State, want)
			case <-time.After(time.Millisecond):
			}
		}
	}
	awaitState(Blocked)
	if r.Snapshot().Reason != "model_unavailable" {
		t.Fatal(r.Snapshot())
	}
	issues <- nil
	awaitState(Healthy)
}

func TestHungFunctionalProbeDoesNotRefresh(t *testing.T) {
	r, err := NewReporterAt(t.TempDir(), "worker")
	if err != nil {
		t.Fatal(err)
	}
	initial := r.Snapshot().ObservedAt
	ctx, cancel := context.WithCancel(context.Background())
	entered, release := make(chan struct{}), make(chan struct{})
	r.GoCheck(ctx, time.Millisecond, func(ctx context.Context) *Issue { close(entered); <-release; return nil }, nil)
	<-entered
	time.Sleep(5 * time.Millisecond)
	cancel()
	if !r.Snapshot().ObservedAt.Equal(initial) {
		t.Fatal("hung check refreshed health")
	}
	close(release)
}

func TestCapturePanicPersistsSafeDiagnosis(t *testing.T) {
	r, err := NewReporterAt(t.TempDir(), "worker")
	if err != nil {
		t.Fatal(err)
	}
	func() {
		defer func() {
			if recover() == nil {
				t.Error("panic was swallowed")
			}
		}()
		defer r.CapturePanic()
		panic("private database credential")
	}()
	if got := r.Snapshot(); got.Reason != "process_panic" || got.Recovery != Restart || got.Message == "private database credential" {
		t.Fatal(got)
	}
}

func TestApplicationEnvironmentReportsVariableWithoutValue(t *testing.T) {
	t.Setenv("APP_HOST", "local")
	t.Setenv("APP_PORT", "secret-invalid-port")
	issue := ApplicationEnvironment()
	if issue == nil || issue.Resource != "APP_PORT" || issue.Recovery != OperatorAction {
		t.Fatal(issue)
	}
	t.Setenv("APP_PORT", "8090")
	if issue := ApplicationEnvironment(); issue != nil {
		t.Fatal(issue)
	}
}

func TestWritableDirectoryCleansProbeAndReportsMissingDirectory(t *testing.T) {
	dir := t.TempDir()
	if issue := WritableDirectory(dir); issue != nil {
		t.Fatal(issue)
	}
	if issue := WritableDirectory(dir + "/missing"); issue == nil || issue.Recovery != OperatorAction {
		t.Fatal(issue)
	}
}
