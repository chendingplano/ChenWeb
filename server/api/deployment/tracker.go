package deployment

import (
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"
)

const (
	StatusPending   = "pending"
	StatusQueued    = "queued"
	StatusRunning   = "running"
	StatusSucceeded = "succeeded"
	StatusFailed    = "failed"
	StatusSkipped   = "skipped"

	// MaxStepOutput is how much output a step keeps; older lines are dropped first.
	MaxStepOutput = 256 * 1024
	// errorTailLines is how many of the failed step's last lines go into the run's error.
	errorTailLines = 20

	markerPrefix = "::step::"
)

var releaseVersionPattern = regexp.MustCompile(`^\d{2}\.\d{2}\.\d{2}$`)

// Step is one row of kb.deployment_steps.
type Step struct {
	Seq             int        `json:"seq"`
	Key             string     `json:"key"`
	Arg             string     `json:"arg"`
	Status          string     `json:"status"`
	StartedAt       *time.Time `json:"started_at,omitempty"`
	FinishedAt      *time.Time `json:"finished_at,omitempty"`
	Output          string     `json:"output"`
	OutputTruncated bool       `json:"output_truncated"`
}

// ParseMarker reads `::step::<key>` or `::step::<key>::<arg>`.
func ParseMarker(line string) (key, arg string, ok bool) {
	rest, found := strings.CutPrefix(strings.TrimRight(line, "\r\n"), markerPrefix)
	if !found || rest == "" {
		return "", "", false
	}
	key, arg, _ = strings.Cut(rest, "::")
	return key, arg, key != ""
}

// Tracker turns a script's output into step states. It is safe for concurrent use.
type Tracker struct {
	mu             sync.Mutex
	steps          []*Step
	current        int
	dirty          map[int]bool
	now            func() time.Time
	archiveName    string
	releaseVersion string
}

// NewTracker creates every planned step as pending and starts the first one, so
// output printed before the first marker belongs to it.
func NewTracker(plan []PlanStep, now func() time.Time) *Tracker {
	t := &Tracker{current: -1, dirty: map[int]bool{}, now: now}
	for i, p := range plan {
		t.steps = append(t.steps, &Step{Seq: i + 1, Key: p.Key, Arg: p.Arg, Status: StatusPending})
	}
	if len(t.steps) > 0 {
		t.start(0)
	}
	return t
}

func (t *Tracker) start(i int) {
	at := t.now()
	t.steps[i].Status = StatusRunning
	t.steps[i].StartedAt = &at
	t.current = i
	t.dirty[i] = true
}

func (t *Tracker) end(i int, status string) {
	at := t.now()
	t.steps[i].Status = status
	t.steps[i].FinishedAt = &at
	t.dirty[i] = true
}

// Line handles one output line: a marker starts its step, anything else is output of
// the running step.
func (t *Tracker) Line(line string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if key, arg, ok := ParseMarker(line); ok {
		t.marker(key, arg)
		return
	}
	if name, ok := strings.CutPrefix(strings.TrimRight(line, "\r\n"), "::archive::"); ok && ArchiveBuildNumber(name) != "" {
		t.archiveName = name
		return
	}
	if v, ok := strings.CutPrefix(strings.TrimRight(line, "\r\n"), "::release::"); ok && releaseVersionPattern.MatchString(v) {
		t.releaseVersion = v
		return
	}
	if t.current < 0 {
		return
	}
	s := t.steps[t.current]
	s.Output += strings.TrimRight(line, "\r\n") + "\n"
	if len(s.Output) > MaxStepOutput {
		cut := len(s.Output) - MaxStepOutput
		if nl := strings.IndexByte(s.Output[cut:], '\n'); nl >= 0 {
			cut += nl + 1
		}
		s.Output = s.Output[cut:]
		s.OutputTruncated = true
	}
	t.dirty[t.current] = true
}

// ReleaseVersion returns the release version the build script committed, if any.
func (t *Tracker) ReleaseVersion() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.releaseVersion
}

// ArchiveName returns the uploaded archive name reported by the build script.
func (t *Tracker) ArchiveName() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.archiveName
}

func (t *Tracker) marker(key, arg string) {
	if t.current >= 0 && t.steps[t.current].Key == key && t.steps[t.current].Arg == arg {
		return
	}
	next := -1
	for i, s := range t.steps {
		if s.Status == StatusPending && s.Key == key && s.Arg == arg {
			next = i
			break
		}
	}
	if next < 0 {
		// Not in the plan (or already run): show it as an extra step instead of failing.
		t.steps = append(t.steps, &Step{Seq: len(t.steps) + 1, Key: key, Arg: arg, Status: StatusPending})
		next = len(t.steps) - 1
	}
	if t.current >= 0 {
		t.end(t.current, StatusSucceeded)
	}
	t.start(next)
}

// Finish records how the script ended and returns the run's status and error message.
// exitCode 0 is success.
func (t *Tracker) Finish(exitCode int, cause error) (status, errorMessage string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	status = StatusSucceeded
	if exitCode != 0 || cause != nil {
		status = StatusFailed
	}
	if t.current >= 0 {
		t.end(t.current, status)
	}
	for i, s := range t.steps {
		if s.Status == StatusPending {
			t.end(i, StatusSkipped)
		}
	}
	if status == StatusSucceeded {
		return status, ""
	}
	reason := fmt.Sprintf("exit status %d", exitCode)
	if cause != nil {
		reason = cause.Error()
	}
	if t.current < 0 {
		return status, reason
	}
	return status, reason + "\n" + lastLines(t.steps[t.current].Output, errorTailLines)
}

// TakeDirty returns copies of the steps changed since the last call.
func (t *Tracker) TakeDirty() []Step {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make([]Step, 0, len(t.dirty))
	for i := range t.steps {
		if t.dirty[i] {
			out = append(out, *t.steps[i])
		}
	}
	t.dirty = map[int]bool{}
	return out
}

func lastLines(s string, n int) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}
