package systemmonitor

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/chendingplano/deepdoc/server/buildinfo"
	appconfig "github.com/chendingplano/deepdoc/server/cmd/config"
)

// fakeMachine stands in for a development machine: its processes, the instances that
// answer build-info requests over NATS, and the mise tasks.
type fakeMachine struct {
	root       string
	names      map[int]string // ps command name of each live pid
	answering  map[int]bool   // pids that answer build-info requests
	ignoreTerm map[int]bool   // pids that do not exit on SIGTERM
	signals    []int
	tasks      []string
	onStart    func(m *fakeMachine) // what the start task launches
}

func newFakeMachine(t *testing.T) *fakeMachine {
	t.Helper()
	m := &fakeMachine{root: t.TempDir(), names: map[int]string{}, answering: map[int]bool{}, ignoreTerm: map[int]bool{}}
	if err := os.MkdirAll(filepath.Join(m.root, ".cache"), 0o755); err != nil {
		t.Fatal(err)
	}
	oldRun, oldAlive, oldList, oldName, oldTerm := runMise, pidAlive, listInstances, processName, signalTerm
	oldStop, oldStart, oldSettle, oldPoll := stopWait, startWait, startupSettle, pollInterval
	t.Cleanup(func() {
		runMise, pidAlive, listInstances, processName, signalTerm = oldRun, oldAlive, oldList, oldName, oldTerm
		stopWait, startWait, startupSettle, pollInterval = oldStop, oldStart, oldSettle, oldPoll
	})
	stopWait, startWait, startupSettle, pollInterval = 100*time.Millisecond, 200*time.Millisecond, 0, time.Millisecond

	listInstances = func(_ context.Context, executable string) ([]buildinfo.Process, error) {
		var out []buildinfo.Process
		for pid := range m.answering {
			out = append(out, buildinfo.Process{Executable: executable, Status: "running", PID: pid})
		}
		slices.SortFunc(out, func(a, b buildinfo.Process) int { return a.PID - b.PID })
		return out, nil
	}
	processName = func(pid int) string { return m.names[pid] }
	pidAlive = func(pid int) bool { _, ok := m.names[pid]; return ok }
	signalTerm = func(pid int) error {
		m.signals = append(m.signals, pid)
		if !m.ignoreTerm[pid] {
			delete(m.names, pid)
			delete(m.answering, pid)
		}
		return nil
	}
	runMise = func(_ context.Context, _ string, task string) ([]byte, error) {
		m.tasks = append(m.tasks, task)
		if strings.HasSuffix(task, "-start") && m.onStart != nil {
			m.onStart(m)
		}
		return []byte(task + " ok\n"), nil
	}
	return m
}

func (m *fakeMachine) run(pid int, name string, answers bool) {
	m.names[pid] = name
	if answers {
		m.answering[pid] = true
	}
}

func (m *fakeMachine) writePIDFile(t *testing.T, name string, pid int) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(m.root, ".cache", name+".pid"), []byte(strconv.Itoa(pid)+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

// startsInstance makes the start task launch a `go run` wrapper (pid file) and the
// service binary under it.
func startsInstance(t *testing.T, name string, wrapper, pid int) func(*fakeMachine) {
	return func(m *fakeMachine) {
		m.writePIDFile(t, name, wrapper)
		m.run(wrapper, "go", false)
		m.run(pid, "/var/folders/kz/T/go-build2/b001/exe/"+name, true)
	}
}

// An instance started from a terminal (start.sh → `mise doc-processor-run`) writes no
// pid file; the restart must still stop it instead of starting a second one.
func TestRestartStopsInstanceStartedOutsideMise(t *testing.T) {
	m := newFakeMachine(t)
	m.run(501, "/var/folders/kz/T/go-build1/b001/exe/doc-processor", true)
	m.onStart = startsInstance(t, "doc-processor", 700, 777)

	out, err := restartService(context.Background(), m.root, restartTargets["doc-processor"])
	if err != nil {
		t.Fatalf("restart: %v\n%s", err, out)
	}
	if !slices.Equal(m.signals, []int{501}) {
		t.Fatalf("signals = %v, want SIGTERM to the running binary 501", m.signals)
	}
	if !slices.Equal(m.tasks, []string{"doc-processor-stop", "doc-processor-start"}) {
		t.Fatalf("tasks = %v", m.tasks)
	}
	if !strings.Contains(string(out), "pid 777") {
		t.Fatalf("output does not name the new instance:\n%s", out)
	}
}

// An earlier broken restart can leave several instances running; all are stopped.
func TestRestartStopsEveryRunningInstance(t *testing.T) {
	m := newFakeMachine(t)
	m.run(501, "/var/folders/kz/T/go-build1/b001/exe/doc-service", true)
	m.run(502, "/Users/me/ChenWeb/.cache/doc-service.exe", true)
	m.onStart = startsInstance(t, "doc-service", 700, 777)

	if out, err := restartService(context.Background(), m.root, restartTargets["doc-service"]); err != nil {
		t.Fatalf("restart: %v\n%s", err, out)
	}
	if !slices.Equal(m.signals, []int{501, 502}) {
		t.Fatalf("signals = %v", m.signals)
	}
}

func TestRestartStartsServiceThatIsNotRunning(t *testing.T) {
	m := newFakeMachine(t)
	m.writePIDFile(t, "parser-result-converter", 4242) // stale
	m.onStart = startsInstance(t, "parser-result-converter", 700, 777)

	if out, err := restartService(context.Background(), m.root, restartTargets["parser-result-converter"]); err != nil {
		t.Fatalf("restart: %v\n%s", err, out)
	}
	if len(m.signals) != 0 {
		t.Fatalf("signals = %v, want none", m.signals)
	}
	if !slices.Equal(m.tasks, []string{"parser-result-converter-stop", "parser-result-converter-start"}) {
		t.Fatalf("tasks = %v", m.tasks)
	}
}

// The start task returns as soon as it has launched the process in the background, so
// a process that exits during startup must be reported as a failure, with its log.
func TestRestartReportsInstanceThatExitsDuringStartup(t *testing.T) {
	m := newFakeMachine(t)
	m.onStart = func(m *fakeMachine) {
		m.writePIDFile(t, "doc-processor", 700) // the wrapper has already exited
		log := "\x1b[91mERROR\x1b[0m subscribe failed: consumer is already bound to a subscription\ndoc processor stopped\n"
		if err := os.WriteFile(filepath.Join(m.root, ".cache", "doc-processor.log"), []byte(log), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	out, err := restartService(context.Background(), m.root, restartTargets["doc-processor"])
	if err == nil {
		t.Fatalf("restart succeeded although the new instance exited:\n%s", out)
	}
	if !strings.Contains(string(out), "ERROR subscribe failed: consumer is already bound") {
		t.Fatalf("output lacks the startup log:\n%s", out)
	}
	if strings.Contains(string(out), "\x1b") {
		t.Fatalf("output keeps terminal colour codes:\n%q", out)
	}
}

func TestRestartReportsInstanceThatExitsAfterAnswering(t *testing.T) {
	m := newFakeMachine(t)
	m.onStart = func(m *fakeMachine) {
		m.writePIDFile(t, "doc-processor", 700)
		m.run(700, "go", false)
		m.answering[777] = true // answers once, but is no longer alive
	}
	if out, err := restartService(context.Background(), m.root, restartTargets["doc-processor"]); err == nil {
		t.Fatalf("restart succeeded although the new instance exited:\n%s", out)
	}
}

// A NATS reply names a pid; only a local process of that service may be signalled.
func TestRestartRefusesPIDOfAnotherProgram(t *testing.T) {
	m := newFakeMachine(t)
	m.run(501, "/usr/bin/vim", true)

	if _, err := restartService(context.Background(), m.root, restartTargets["doc-processor"]); err == nil {
		t.Fatal("restart succeeded although the reported pid is not doc-processor")
	}
	if len(m.signals) != 0 || len(m.tasks) != 0 {
		t.Fatalf("signals = %v, tasks = %v; want neither", m.signals, m.tasks)
	}
}

func TestRestartServiceWaitsForOldProcess(t *testing.T) {
	m := newFakeMachine(t)
	m.run(99, "/var/folders/kz/T/go-build1/b001/exe/doc-processor", true)
	m.ignoreTerm[99] = true // still finishing in-flight work

	if _, err := restartService(context.Background(), m.root, restartTargets["doc-processor"]); err == nil {
		t.Fatal("restart succeeded while the old process was still alive")
	}
	if len(m.tasks) != 0 {
		t.Fatalf("tasks ran after a failed stop wait: %v", m.tasks)
	}
}

func TestSameExecutable(t *testing.T) {
	cases := []struct {
		comm, executable string
		want             bool
	}{
		{"/var/folders/kz/T/go-build1/b001/exe/doc-processor", "doc-processor", true},
		{"/Users/me/ChenWeb/.cache/doc-processor.exe", "doc-processor", true},
		{"doc-processor\n", "doc-processor", true},
		{"parser-result-c", "parser-result-converter", true}, // Linux truncates to 15
		{"doc-processor.e", "doc-processor", true},           // Linux, .exe build
		{"doc-processor-old", "doc-processor", false},
		{"doc-service", "doc-processor", false},
		{"/usr/bin/vim", "doc-processor", false},
		{"", "doc-processor", false},
	}
	for _, c := range cases {
		if got := sameExecutable(c.comm, c.executable); got != c.want {
			t.Errorf("sameExecutable(%q, %q) = %v, want %v", c.comm, c.executable, got, c.want)
		}
	}
}

func TestRestartTargetsAreNotTheWholeList(t *testing.T) {
	for _, id := range []string{"chenweb", "kratos", "nats", "caddy", "pdf-parser", "deepseek-watcher", "sshd", "doc-service;rm"} {
		if _, ok := restartTargets[id]; ok {
			t.Errorf("%q should not be restartable from the monitor", id)
		}
	}
	for id, target := range restartTargets {
		if _, ok := restartLocks[id]; !ok {
			t.Errorf("no restart lock for %q", id)
		}
		if target.name != id {
			t.Errorf("%q: task prefix %q must match the source id", id, target.name)
		}
	}
}

func TestRestartServiceOnDeploymentMachine(t *testing.T) {
	dir := t.TempDir()
	sudo := filepath.Join(dir, "sudo")
	if err := os.WriteFile(sudo, []byte("#!/bin/sh\nprintf '%s\n' \"$*\"\n"), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	oldRole := appconfig.AppConfig.Deployment.Role
	appconfig.AppConfig.Deployment.Role = "deploy"
	t.Cleanup(func() { appconfig.AppConfig.Deployment.Role = oldRole })
	out, err := restartService(context.Background(), t.TempDir(), restartTargets["doc-processor"])
	if err != nil {
		t.Fatalf("restart: %v", err)
	}
	if string(out) != "-n /bin/systemctl restart doc-processor\n" {
		t.Fatalf("output = %q", out)
	}
}
