package deployment

import (
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestExecuteFailingScript(t *testing.T) {
	script := `echo '::step::check'; echo checking
echo '::step::binary::a'; echo 'compile error' >&2; exit 3
echo '::step::upload'`
	tr := NewTracker([]PlanStep{{Key: "check"}, {Key: "binary", Arg: "a"}, {Key: "upload"}}, time.Now)
	var mu sync.Mutex
	saved := map[int]Step{}
	save := func(steps []Step) {
		mu.Lock()
		defer mu.Unlock()
		for _, s := range steps {
			saved[s.Seq] = s
		}
	}
	code, err := Execute(exec.Command("bash", "-c", script), tr, save, 10*time.Millisecond)
	if code != 3 || err != nil {
		t.Fatalf("Execute = %d, %v", code, err)
	}
	status, msg := tr.Finish(code, err)
	save(tr.TakeDirty())
	if status != StatusFailed || !strings.Contains(msg, "compile error") {
		t.Fatalf("Finish = %q, %q", status, msg)
	}
	if saved[1].Status != StatusSucceeded || saved[1].Output != "checking\n" ||
		saved[2].Status != StatusFailed || saved[3].Status != StatusSkipped {
		t.Fatalf("saved = %+v", saved)
	}
}

func TestExecuteMissingCommand(t *testing.T) {
	tr := NewTracker([]PlanStep{{Key: "check"}}, time.Now)
	code, err := Execute(exec.Command("/nonexistent/script.sh"), tr, func([]Step) {}, time.Second)
	if code != -1 || err == nil {
		t.Fatalf("Execute = %d, %v", code, err)
	}
}
