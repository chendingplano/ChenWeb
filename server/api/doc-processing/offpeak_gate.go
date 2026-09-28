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
// off-peak only" in the upload UI) whose LLM processors are held while the
// configured peak-hours window is active or about to start.
const ProcessingModeAutoOffPeak = "auto_offpeak"

const (
	offPeakWindowNameEnv       = "DOC_PROCESS_OFFPEAK_PEAK_HOURS_NAME"
	offPeakLeadMinutesEnv      = "DOC_PROCESS_OFFPEAK_LEAD_MINUTES"
	offPeakPollSecondsEnv      = "DOC_PROCESS_OFFPEAK_POLL_SEC"
	defaultOffPeakWindowName   = "deepseek peak hours"
	defaultOffPeakLeadMinutes  = 10
	defaultOffPeakPollSeconds  = 30
	offPeakHeldProgressPrefix  = "held: waiting for off-peak hours"
	nonLLMBlockingProcessorKey = "blocking"
)

// OffPeakGate decides whether auto_offpeak work must wait. The window is
// "held" when the named public.peak_hours record is active at any minute in
// [now, now+Lead]. The decision is cached for Poll and shared by every
// waiter, so the DB is consulted at most once per poll interval.
type OffPeakGate struct {
	WindowName string
	Lead       time.Duration
	Poll       time.Duration
	Logger     ApiTypes.JimoLogger
	Now        func() time.Time

	// peakSoon reports whether the window is active at any minute in
	// [from, from+lead]. Injected so tests need no database.
	peakSoon func(ctx context.Context, from time.Time, lead time.Duration) (bool, error)

	mu        sync.Mutex
	checkedAt time.Time
	held      bool
}

// NewOffPeakGateFromEnv builds the gate from DOC_PROCESS_OFFPEAK_* settings,
// evaluating the window with peakhourshandler against db. The record is
// re-read on every check so edits in the Peak Hours admin page apply
// without a restart.
func NewOffPeakGateFromEnv(db *sql.DB, logger ApiTypes.JimoLogger) *OffPeakGate {
	name := strings.TrimSpace(os.Getenv(offPeakWindowNameEnv))
	if name == "" {
		name = defaultOffPeakWindowName
	}
	g := &OffPeakGate{
		WindowName: name,
		Lead:       time.Duration(offPeakEnvInt(offPeakLeadMinutesEnv, defaultOffPeakLeadMinutes, 0)) * time.Minute,
		Poll:       time.Duration(offPeakEnvInt(offPeakPollSecondsEnv, defaultOffPeakPollSeconds, 1)) * time.Second,
		Logger:     logger,
		Now:        time.Now,
	}
	g.peakSoon = func(ctx context.Context, from time.Time, lead time.Duration) (bool, error) {
		rec, err := peakhourshandler.GetPeakHoursByName(ctx, db, name)
		if err != nil {
			return false, err
		}
		for d := time.Duration(0); d <= lead; d += time.Minute {
			active, err := peakhourshandler.EvaluateActive(ctx, db, rec, from.Add(d))
			if err != nil {
				return false, err
			}
			if active {
				return true, nil
			}
		}
		return false, nil
	}
	return g
}

func offPeakEnvInt(key string, fallback, min int) int {
	n, err := strconv.Atoi(strings.TrimSpace(os.Getenv(key)))
	if err != nil {
		return fallback
	}
	if n < min {
		return min
	}
	return n
}

func (g *OffPeakGate) now() time.Time {
	if g.Now != nil {
		return g.Now()
	}
	return time.Now()
}

// HeldNow reports whether auto_offpeak LLM work must wait right now. A
// missing record or evaluation error fails open (not held) with a WARN log:
// holding forever on a misconfiguration would silently stall every upload.
func (g *OffPeakGate) HeldNow(ctx context.Context) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	now := g.now()
	if !g.checkedAt.IsZero() && now.Sub(g.checkedAt) < g.Poll {
		return g.held
	}
	held := false
	if g.peakSoon != nil {
		var err error
		held, err = g.peakSoon(ctx, now, g.Lead)
		if err != nil {
			held = false
			if g.Logger != nil {
				g.Logger.Warn("off-peak gate: peak hours evaluation failed, not holding",
					"peak_hours_name", g.WindowName, "error", err)
			}
		}
	}
	g.held, g.checkedAt = held, now
	return held
}

// Wait blocks while HeldNow is true, re-checking every Poll. onHold runs
// once, before the first wait. It returns whether it waited at all, and
// context.Cause(ctx) if ctx ends first (ErrPipelineStopped on a user stop).
func (g *OffPeakGate) Wait(ctx context.Context, onHold func()) (bool, error) {
	waited := false
	for {
		if err := context.Cause(ctx); err != nil {
			return waited, err
		}
		if !g.HeldNow(ctx) {
			return waited, nil
		}
		if !waited {
			waited = true
			if onHold != nil {
				onHold()
			}
		}
		t := time.NewTimer(g.Poll)
		select {
		case <-ctx.Done():
			t.Stop()
			return waited, context.Cause(ctx)
		case <-t.C:
		}
	}
}

func (g *OffPeakGate) heldProgress() string {
	return offPeakHeldProgressPrefix + " (" + g.WindowName + ")"
}

type offPeakHoldKey struct{}

// withOffPeakHold marks ctx's pipeline as auto_offpeak.
func withOffPeakHold(ctx context.Context) context.Context {
	return context.WithValue(ctx, offPeakHoldKey{}, true)
}

func offPeakHoldFromContext(ctx context.Context) bool {
	v, _ := ctx.Value(offPeakHoldKey{}).(bool)
	return v
}

// processorUsesLLM reports whether an off-peak pipeline must hold before
// running the processor. Every processor except the blocking one calls an
// LLM somewhere (even static_analyzer's TOC detection and chunking).
func processorUsesLLM(name string) bool {
	return canonicalOperationName(name) != nonLLMBlockingProcessorKey
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

// pipelineSlotLease owns one pipeline slot for a pipeline so that a held
// pipeline can give its slot back while it waits and take one again before
// resuming. The first waiter releases the slot; the last waiter to resume
// re-acquires it.
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

// offPeakOnly reports whether recordID's pipeline must honour the off-peak
// gate. A lookup failure falls back to plain auto (no hold).
func (s *ControlService) offPeakOnly(ctx context.Context, recordID int64) bool {
	if s.OffPeak == nil {
		return false
	}
	loader, ok := s.InputStore.(processingModeLoader)
	if !ok {
		return false
	}
	mode, err := loader.GetProcessingMode(ctx, recordID)
	if err != nil {
		if s.Logger != nil {
			s.Logger.Warn("off-peak gate: load processing_mode failed, not holding", "record_id", recordID, "error", err)
		}
		return false
	}
	return mode == ProcessingModeAutoOffPeak
}

// holdForOffPeak blocks an auto_offpeak pipeline before LLM work while the
// peak window is active or about to start. processorNames get status
// "active" with a held progress note while waiting. The pipeline slot is
// released while held. It returns context.Cause(ctx) (ErrPipelineStopped
// on a user stop) if the pipeline ends while held.
func (s *ControlService) holdForOffPeak(ctx context.Context, recordID int64, processorNames ...string) error {
	if s.OffPeak == nil || !offPeakHoldFromContext(ctx) {
		return nil
	}
	lease := pipelineSlotLeaseFromContext(ctx)
	suspended := false
	waited, err := s.OffPeak.Wait(ctx, func() {
		if s.Logger != nil {
			s.Logger.Info("holding doc processors until off-peak hours",
				"record_id", recordID, "processors", processorNames, "peak_hours_name", s.OffPeak.WindowName)
		}
		for _, name := range processorNames {
			s.persistProcessorRuntimeStatus(ctx, recordID, name, "active", s.OffPeak.heldProgress())
		}
		if lease != nil {
			lease.suspend()
			suspended = true
		}
	})
	if suspended {
		if resumeErr := lease.resume(ctx); resumeErr != nil && err == nil {
			err = resumeErr
		}
	}
	if err != nil {
		return err
	}
	if waited {
		if s.Logger != nil {
			s.Logger.Info("resuming doc processors in off-peak hours", "record_id", recordID, "processors", processorNames)
		}
		for _, name := range processorNames {
			s.persistProcessorRuntimeStatus(ctx, recordID, name, "active", "")
		}
	}
	return nil
}

func hasPostProcessIndexers(processors []Processor) bool {
	for _, p := range processors {
		if _, ok := p.(PostProcessIndexer); ok {
			return true
		}
	}
	return false
}
