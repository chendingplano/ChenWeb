package docprocessing

import (
	"context"
	"database/sql"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/chendingplano/deepdoc/server/api/peakhourshandler"
	"github.com/chendingplano/shared/go/api/ApiTypes"
)

// ProcessingModeAutoOffPeak is the kb.inputs.processing_mode value ("Auto -
// off-peak only" in the upload UI) that gives the input's doc-processor task
// its own peak-hour mode auto_offpeak (task_engine_policy.go).
const ProcessingModeAutoOffPeak = "auto_offpeak"

const (
	peakWindowNameEnv         = "DOC_PROCESS_OFFPEAK_PEAK_HOURS_NAME"
	peakWindowPollSecondsEnv  = "DOC_PROCESS_OFFPEAK_POLL_SEC"
	defaultPeakWindowName     = "deepseek peak hours"
	defaultPeakWindowPollSecs = 30
)

// PeakWindow reports whether the named public.peak_hours record is active
// now. The answer, or the evaluation error, is cached for Poll and shared by
// every caller, so the DB is consulted at most once per poll interval.
type PeakWindow struct {
	Name   string
	Poll   time.Duration
	Logger ApiTypes.JimoLogger
	Now    func() time.Time

	// activeAt reports whether the window is active at an instant. Injected
	// so tests need no database.
	activeAt func(ctx context.Context, at time.Time) (bool, error)

	mu        sync.Mutex
	checkedAt time.Time
	active    bool
	err       error
}

// NewPeakWindowFromEnv builds the window from DOC_PROCESS_OFFPEAK_* settings,
// evaluating it with peakhourshandler against db. The record is re-read on
// every evaluation so edits in the Peak Hours admin page apply without a
// restart.
func NewPeakWindowFromEnv(db *sql.DB, logger ApiTypes.JimoLogger) *PeakWindow {
	name := strings.TrimSpace(os.Getenv(peakWindowNameEnv))
	if name == "" {
		name = defaultPeakWindowName
	}
	w := &PeakWindow{
		Name:   name,
		Poll:   time.Duration(peakWindowEnvInt(peakWindowPollSecondsEnv, defaultPeakWindowPollSecs, 1)) * time.Second,
		Logger: logger,
		Now:    time.Now,
	}
	w.activeAt = func(ctx context.Context, at time.Time) (bool, error) {
		rec, err := peakhourshandler.GetPeakHoursByName(ctx, db, name)
		if err != nil {
			return false, err
		}
		return peakhourshandler.EvaluateActive(ctx, db, rec, at)
	}
	return w
}

func peakWindowEnvInt(key string, fallback, min int) int {
	n, err := strconv.Atoi(strings.TrimSpace(os.Getenv(key)))
	if err != nil {
		return fallback
	}
	if n < min {
		return min
	}
	return n
}

func (w *PeakWindow) now() time.Time {
	if w.Now != nil {
		return w.Now()
	}
	return time.Now()
}

func (w *PeakWindow) poll() time.Duration {
	if w.Poll > 0 {
		return w.Poll
	}
	return defaultPeakWindowPollSecs * time.Second
}

// Active reports whether the window is active now. A missing record or an
// evaluation error is returned, with a WARN once per poll interval, so the
// caller can decide how to fail.
func (w *PeakWindow) Active(ctx context.Context) (bool, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	now := w.now()
	if !w.checkedAt.IsZero() && now.Sub(w.checkedAt) < w.poll() {
		return w.active, w.err
	}
	active, err := false, error(nil)
	if w.activeAt != nil {
		active, err = w.activeAt(ctx, now)
	}
	if err != nil && w.Logger != nil {
		w.Logger.Warn("peak hours evaluation failed; peak-hour control permits all work",
			"peak_hours_name", w.Name, "error", err)
	}
	w.active, w.err, w.checkedAt = active, err, now
	return active, err
}

// processingModeLoader is optionally implemented by the input store.
type processingModeLoader interface {
	GetProcessingMode(ctx context.Context, id int64) (string, error)
}

// GetProcessingMode returns kb.inputs.processing_mode, "auto" when blank.
func (s DocMetadataSQLStore) GetProcessingMode(ctx context.Context, id int64) (string, error) {
	var mode string
	err := s.DB.QueryRowContext(ctx,
		`SELECT COALESCE(NULLIF(BTRIM(processing_mode), ''), 'auto') FROM kb.inputs WHERE id = $1`,
		id,
	).Scan(&mode)
	return strings.ToLower(strings.TrimSpace(mode)), err
}

// pipelineSlotLease owns one pipeline slot for a pipeline so that a waiting
// pipeline can give its slot back and take one again before starting. The
// first waiter releases the slot; the last waiter to resume re-acquires it.
type pipelineSlotLease struct {
	acquire func(context.Context) (func(), error)

	mu      sync.Mutex
	release func()
	waiters int
}

func newPipelineSlotLease(acquire func(context.Context) (func(), error), release func()) *pipelineSlotLease {
	return &pipelineSlotLease{acquire: acquire, release: release}
}

// Release frees the slot if the lease still holds one. Safe to call twice.
func (l *pipelineSlotLease) Release() {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.release != nil {
		l.release()
		l.release = nil
	}
}

func (l *pipelineSlotLease) suspend() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.waiters++
	if l.waiters == 1 && l.release != nil {
		l.release()
		l.release = nil
	}
}

func (l *pipelineSlotLease) resume(ctx context.Context) error {
	l.mu.Lock()
	l.waiters--
	need := l.waiters == 0 && l.release == nil && l.acquire != nil
	l.mu.Unlock()
	if !need {
		return nil
	}
	release, err := l.acquire(ctx)
	if err != nil {
		return err
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.release == nil {
		l.release = release
	} else {
		release()
	}
	return nil
}

type pipelineSlotLeaseKey struct{}

func withPipelineSlotLease(ctx context.Context, l *pipelineSlotLease) context.Context {
	return context.WithValue(ctx, pipelineSlotLeaseKey{}, l)
}

func pipelineSlotLeaseFromContext(ctx context.Context) *pipelineSlotLease {
	l, _ := ctx.Value(pipelineSlotLeaseKey{}).(*pipelineSlotLease)
	return l
}
