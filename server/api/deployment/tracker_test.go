package deployment

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func fixedNow() time.Time { return time.Date(2026, 11, 16, 9, 30, 0, 0, time.UTC) }

func statuses(t *Tracker) []string {
	out := make([]string, 0, len(t.steps))
	for _, s := range t.steps {
		out = append(out, s.Key+"/"+s.Arg+"="+s.Status)
	}
	return out
}

func TestParseMarker(t *testing.T) {
	cases := []struct {
		line, key, arg string
		ok             bool
	}{
		{"::step::web", "web", "", true},
		{"::step::binary::doc-processor\n", "binary", "doc-processor", true},
		{"::step::", "", "", false},
		{"==> building server-linux", "", "", false},
		{"  ::step::web", "", "", false},
	}
	for _, c := range cases {
		key, arg, ok := ParseMarker(c.line)
		if key != c.key || arg != c.arg || ok != c.ok {
			t.Errorf("ParseMarker(%q) = %q,%q,%v want %q,%q,%v", c.line, key, arg, ok, c.key, c.arg, c.ok)
		}
	}
}

func TestTrackerSuccessSkipsStepsThatNeverRan(t *testing.T) {
	tr := NewTracker([]PlanStep{{Key: "check"}, {Key: "binary", Arg: "a"}, {Key: "binary", Arg: "b"}, {Key: "upload"}}, fixedNow)
	tr.Line("before any marker")
	tr.Line("::step::check")
	tr.Line("checking")
	tr.Line("::step::binary::a")
	tr.Line("::step::upload")
	status, msg := tr.Finish(0, nil)
	if status != StatusSucceeded || msg != "" {
		t.Fatalf("Finish = %q,%q", status, msg)
	}
	want := "check/=succeeded binary/a=succeeded binary/b=skipped upload/=succeeded"
	if got := strings.Join(statuses(tr), " "); got != want {
		t.Fatalf("steps = %s\nwant    %s", got, want)
	}
	if got := tr.steps[0].Output; got != "before any marker\nchecking\n" {
		t.Fatalf("check output = %q", got)
	}
}

func TestTrackerFailureMarksRunningStepFailedAndRestSkipped(t *testing.T) {
	tr := NewTracker(Plan(KindBuild), fixedNow)
	tr.Line("::step::web")
	tr.Line("::step::binary::server")
	for i := 0; i < 30; i++ {
		tr.Line("line")
	}
	tr.Line("server/api/x.go:1: undefined: y")
	status, msg := tr.Finish(1, nil)
	if status != StatusFailed {
		t.Fatalf("status = %q", status)
	}
	if !strings.HasPrefix(msg, "exit status 1\n") || !strings.HasSuffix(msg, "undefined: y") {
		t.Fatalf("error message = %q", msg)
	}
	if n := strings.Count(msg, "\n"); n != errorTailLines {
		t.Fatalf("error message has %d newlines, want %d", n, errorTailLines)
	}
	byKey := map[string]string{}
	for _, s := range tr.steps {
		byKey[s.Key+"/"+s.Arg] = s.Status
	}
	if byKey["check/"] != StatusSucceeded || byKey["web/"] != StatusSucceeded || byKey["prompt-defaults/"] != StatusSkipped ||
		byKey["binary/server"] != StatusFailed || byKey["upload/"] != StatusSkipped {
		t.Fatalf("steps = %v", byKey)
	}
}

func TestTrackerFailureBeforeAnyMarkerFailsFirstStep(t *testing.T) {
	tr := NewTracker(Plan(KindDeploy), fixedNow)
	tr.Line("!! PROJECT_WORKSPACE_DIR is not a directory")
	status, msg := tr.Finish(0, errors.New("start script: no such file"))
	if status != StatusFailed || !strings.HasPrefix(msg, "start script: no such file") {
		t.Fatalf("Finish = %q,%q", status, msg)
	}
	if tr.steps[0].Status != StatusFailed || tr.steps[1].Status != StatusSkipped {
		t.Fatalf("steps = %v", statuses(tr))
	}
}

func TestTrackerUnknownMarkerIsAppended(t *testing.T) {
	tr := NewTracker([]PlanStep{{Key: "check"}, {Key: "upload"}}, fixedNow)
	tr.Line("::step::surprise::x")
	tr.Line("::step::upload")
	tr.Finish(0, nil)
	want := "check/=succeeded upload/=succeeded surprise/x=succeeded"
	if got := strings.Join(statuses(tr), " "); got != want {
		t.Fatalf("steps = %s\nwant    %s", got, want)
	}
	if tr.steps[2].Seq != 3 {
		t.Fatalf("appended seq = %d", tr.steps[2].Seq)
	}
}

func TestTrackerCapsOutputAtLineBoundary(t *testing.T) {
	tr := NewTracker([]PlanStep{{Key: "archive"}}, fixedNow)
	line := strings.Repeat("x", 1000)
	for i := 0; i < 400; i++ {
		tr.Line(line)
	}
	s := tr.steps[0]
	if len(s.Output) > MaxStepOutput || !s.OutputTruncated || !strings.HasPrefix(s.Output, "x") {
		t.Fatalf("len=%d truncated=%v prefix=%q", len(s.Output), s.OutputTruncated, s.Output[:5])
	}
}

func TestTakeDirtyReturnsOnlyChangedSteps(t *testing.T) {
	tr := NewTracker([]PlanStep{{Key: "a"}, {Key: "b"}, {Key: "c"}}, fixedNow)
	if got := len(tr.TakeDirty()); got != 1 {
		t.Fatalf("initial dirty = %d", got)
	}
	tr.Line("::step::b")
	dirty := tr.TakeDirty()
	if len(dirty) != 2 || dirty[0].Key != "a" || dirty[1].Key != "b" {
		t.Fatalf("dirty = %+v", dirty)
	}
	if got := len(tr.TakeDirty()); got != 0 {
		t.Fatalf("dirty after take = %d", got)
	}
}

func TestBuildArchiveMarkerKeepsUploadStep(t *testing.T) {
	tr := NewTracker([]PlanStep{{Key: "upload"}}, time.Now)
	tr.TakeDirty()
	tr.Line("::archive::runshen-deploy-20261118010604.tar.gz\n")
	if got := tr.ArchiveName(); got != "runshen-deploy-20261118010604.tar.gz" {
		t.Fatalf("archive name = %q", got)
	}
	if steps := tr.TakeDirty(); len(steps) != 0 {
		t.Fatalf("archive marker changed upload step: %+v", steps)
	}
}
