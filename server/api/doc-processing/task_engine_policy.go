package docprocessing

import (
	"context"
	"encoding/json"
	"errors"
	"maps"
	"time"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	sharedllm "github.com/chendingplano/shared/go/api/llm"
)

// Peak-hour modes: the engine's default_mode, or a task's own mode.
const (
	PeakModeNone        = "none"
	PeakModeAutoPeak    = "auto_peak"
	PeakModeAutoOffPeak = "auto_offpeak"
)

// Cross-peak boundary rules (cross_peak_mode).
const (
	CrossPeakIgnore   = "ignore"
	CrossPeakPauseLLM = "pause-llm"
	CrossPeakAbort    = "abort"
)

// llm_usage_event.event_kind values of a held LLM call.
const (
	llmEventKindHold          = "llm_hold"
	llmEventKindResume        = "llm_resume"
	llmEventKindHoldCancelled = "llm_hold_cancelled"
)

const llmLifecycleEventTimeout = 5 * time.Second

// pipelineAbortedError is the cancellation cause of a task that
// cross_peak_mode abort ends at a peak boundary. It is a stop
// (errors.Is(err, ErrPipelineStopped)), so every existing stop check ends the
// work, and its message records why on the status entry and run row.
type pipelineAbortedError struct{}

func (pipelineAbortedError) Error() string        { return "aborted at peak-hour boundary" }
func (pipelineAbortedError) Is(target error) bool { return target == ErrPipelineStopped }

var errPipelineAborted error = pipelineAbortedError{}

// taskEnginePolicy is the doc-processor's peak-hour control, configured by
// [[task-engines.doc-processor]] (spec doc-processor-peak-controls). The
// zero value disables it.
type taskEnginePolicy struct {
	DefaultMode string
	CrossPeak   string
	Window      *PeakWindow
	Logger      ApiTypes.JimoLogger
	// Events receives the llm_usage_event lifecycle rows of held LLM calls.
	// Nil means the holds are only logged.
	Events sharedllm.UsageCaptureSink
}

// modeFor returns a task's peak-hour mode: its own mode when it has one,
// otherwise the engine's default_mode.
func (p taskEnginePolicy) modeFor(taskMode string) string {
	if taskMode != "" {
		return taskMode
	}
	if p.DefaultMode != "" {
		return p.DefaultMode
	}
	return PeakModeNone
}

// permitted reports whether a task in mode may work now. A failed peak-hours
// evaluation permits work (the window logs it): holding on a
// misconfiguration would silently stall every task.
func (p taskEnginePolicy) permitted(ctx context.Context, mode string) bool {
	if mode == PeakModeNone || p.Window == nil {
		return true
	}
	active, err := p.Window.Active(ctx)
	if err != nil {
		return true
	}
	if mode == PeakModeAutoPeak {
		return active
	}
	return !active
}

// waitPermitted blocks until mode is permitted, re-checking every poll
// interval. It returns context.Cause(ctx) if ctx ends first.
func (p taskEnginePolicy) waitPermitted(ctx context.Context, mode string) error {
	for !p.permitted(ctx, mode) {
		timer := time.NewTimer(p.Window.poll())
		select {
		case <-ctx.Done():
			timer.Stop()
			return context.Cause(ctx)
		case <-timer.C:
		}
	}
	return context.Cause(ctx)
}

func (p taskEnginePolicy) windowName() string {
	if p.Window == nil {
		return ""
	}
	return p.Window.Name
}

// waitingNote is the doc_processing progress of a task waiting for mode.
func (p taskEnginePolicy) waitingNote(mode string) string {
	if mode == PeakModeAutoPeak {
		return "waiting for peak hours (" + p.windowName() + ")"
	}
	return "waiting for off-peak hours (" + p.windowName() + ")"
}

// abortAtBoundary cancels an admitted task with errPipelineAborted once its
// mode stops being permitted. It returns when ctx ends.
func (p taskEnginePolicy) abortAtBoundary(ctx context.Context, cancel context.CancelCauseFunc, recordID int64, mode string) {
	ticker := time.NewTicker(p.Window.poll())
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if p.permitted(ctx, mode) {
				continue
			}
			if p.Logger != nil {
				p.Logger.Info("aborting task at peak-hour boundary",
					"record_id", recordID, "peak_mode", mode, "cross_peak_mode", p.CrossPeak, "peak_hours_name", p.windowName())
			}
			cancel(errPipelineAborted)
			return
		}
	}
}

// peakRequestGate holds each provider request of a task while the task's
// mode is not permitted (cross_peak_mode pause-llm). A hold is logged and
// written to llm_usage_event as an llm_hold row, closed by an llm_resume or
// llm_hold_cancelled row with the same hold_id.
type peakRequestGate struct {
	policy   taskEnginePolicy
	mode     string
	recordID int64
}

func (g *peakRequestGate) Open(ctx context.Context) bool {
	return g.policy.permitted(ctx, g.mode)
}

func (g *peakRequestGate) Wait(ctx context.Context, call sharedllm.RequestGateCall) error {
	if g.policy.permitted(ctx, g.mode) {
		return nil
	}
	usage := g.attribute(ctx, call.Usage)
	holdID := newHoldID()
	heldAt := time.Now().UTC()
	g.log("llm call held until its peak-hour span returns", usage, call.Kind, holdID, nil)
	g.recordLifecycle(ctx, usage, llmEventKindHold, call.Kind, holdID, heldAt, heldAt, nil)

	err := g.policy.waitPermitted(ctx, g.mode)
	kind, msg := llmEventKindResume, "held llm call resumed"
	if err != nil {
		kind, msg = llmEventKindHoldCancelled, "held llm call cancelled"
	}
	g.log(msg, usage, call.Kind, holdID, err)
	g.recordLifecycle(ctx, usage, kind, call.Kind, holdID, heldAt, time.Now().UTC(), err)
	return err
}

// attribute fills the task's user, run and record into a call's identity
// when the call does not carry them itself.
func (g *peakRequestGate) attribute(ctx context.Context, usage sharedllm.UsageCaptureInput) sharedllm.UsageCaptureInput {
	if usage.UserID == "" {
		usage.UserID = llmUserIDFromContext(ctx)
	}
	if usage.RunID == 0 {
		usage.RunID, _ = runIDFromContext(ctx)
	}
	if usage.RecordID == 0 {
		usage.RecordID = g.recordID
	}
	return usage
}

func (g *peakRequestGate) log(msg string, usage sharedllm.UsageCaptureInput, callKind, holdID string, err error) {
	if g.policy.Logger == nil {
		return
	}
	args := []any{
		"record_id", usage.RecordID, "run_id", usage.RunID, "hold_id", holdID,
		"peak_mode", g.mode, "cross_peak_mode", g.policy.CrossPeak, "peak_hours_name", g.policy.windowName(),
		"call_kind", callKind, "model", usage.ModelName, "call_reason", usage.CallReason, "call_loc", usage.CallLoc,
	}
	if err != nil {
		args = append(args, "cause", err.Error())
	}
	g.policy.Logger.Info(msg, args...)
}

// recordLifecycle writes one lifecycle row for a held call. A failed write
// is logged and never blocks or fails the call.
func (g *peakRequestGate) recordLifecycle(ctx context.Context, usage sharedllm.UsageCaptureInput, kind, callKind, holdID string, startedAt, finishedAt time.Time, cause error) {
	if g.policy.Events == nil {
		return
	}
	metadata := maps.Clone(usage.Metadata)
	if metadata == nil {
		metadata = map[string]any{}
	}
	metadata["hold_id"] = holdID
	metadata["call_kind"] = callKind
	metadata["peak_mode"] = g.mode
	metadata["cross_peak_mode"] = g.policy.CrossPeak
	metadata["peak_hours_name"] = g.policy.windowName()
	if kind != llmEventKindHold {
		metadata["held_ms"] = finishedAt.Sub(startedAt).Milliseconds()
	}
	usage.Metadata = metadata
	usage.EventKind = kind
	usage.RequestStartedAt = startedAt
	usage.RequestFinishedAt = finishedAt
	if cause != nil {
		usage.ErrorMessage = cause.Error()
	}
	writeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), llmLifecycleEventTimeout)
	defer cancel()
	if _, err := g.policy.Events.Capture(writeCtx, sharedllm.NewUsageCaptureRecord(usage)); err != nil && g.policy.Logger != nil {
		g.policy.Logger.Warn("failed writing llm hold lifecycle event",
			"event_kind", kind, "hold_id", holdID, "record_id", usage.RecordID, "run_id", usage.RunID, "error", err)
	}
}

func newHoldID() string {
	return "hold-" + newEventID()[len("evt-"):]
}

// taskPeakMode returns a task's own peak-hour mode: auto_offpeak for an
// input uploaded as "Auto - off-peak only", otherwise "" (the engine default
// applies), including when the processing mode cannot be read.
func (s *ControlService) taskPeakMode(ctx context.Context, recordID int64) string {
	if s.TaskPolicy.Window == nil {
		return ""
	}
	loader, ok := s.InputStore.(processingModeLoader)
	if !ok {
		return ""
	}
	mode, err := loader.GetProcessingMode(ctx, recordID)
	if err != nil {
		if s.Logger != nil {
			s.Logger.Warn("load processing_mode failed; task uses the engine peak-hour mode", "record_id", recordID, "error", err)
		}
		return ""
	}
	if mode == ProcessingModeAutoOffPeak {
		return PeakModeAutoOffPeak
	}
	return ""
}

// admitTask holds a task until its peak-hour mode permits work. While
// waiting, the task gives up its pipeline slot and its doc_processing status
// shows the wait. After taking a slot back it checks again, since waiting for
// the slot can cross a boundary. It returns context.Cause(ctx) if the task is
// stopped while waiting.
func (s *ControlService) admitTask(ctx context.Context, recordID int64, mode string) error {
	p := s.TaskPolicy
	lease := pipelineSlotLeaseFromContext(ctx)
	waited := false
	for !p.permitted(ctx, mode) {
		if !waited {
			waited = true
			if s.Logger != nil {
				s.Logger.Info("task waiting for its permitted peak-hour span",
					"record_id", recordID, "peak_mode", mode, "cross_peak_mode", p.CrossPeak, "peak_hours_name", p.windowName())
			}
			s.persistPipelineWaiting(ctx, recordID, p.waitingNote(mode))
		}
		if lease != nil {
			lease.suspend()
		}
		err := p.waitPermitted(ctx, mode)
		if lease != nil {
			if resumeErr := lease.resume(ctx); err == nil {
				err = resumeErr
			}
		}
		if err != nil {
			return err
		}
	}
	if waited && s.Logger != nil {
		s.Logger.Info("task starts after peak-hour wait",
			"record_id", recordID, "peak_mode", mode, "cross_peak_mode", p.CrossPeak)
	}
	return nil
}

// applyCrossPeakRule arranges what an admitted task does when its permitted
// span ends: nothing (ignore), hold its provider requests (pause-llm), or
// cancel it (abort). A task in mode none has no boundary.
func (s *ControlService) applyCrossPeakRule(ctx context.Context, cancel context.CancelCauseFunc, recordID int64, mode string) context.Context {
	p := s.TaskPolicy
	if mode == PeakModeNone || p.Window == nil {
		return ctx
	}
	switch p.CrossPeak {
	case CrossPeakAbort:
		go p.abortAtBoundary(ctx, cancel, recordID, mode)
	case CrossPeakPauseLLM:
		return sharedllm.WithRequestGate(ctx, &peakRequestGate{policy: p, mode: mode, recordID: recordID})
	}
	return ctx
}

// finishStoppedEarly ends a task that was stopped, by the user or a
// cross-peak abort, before its run row exists: while waiting for admission or
// during routing.
func (s *ControlService) finishStoppedEarly(ctx context.Context, recordID int64, requestStart time.Time) {
	var reason error
	if cause := context.Cause(ctx); errors.Is(cause, errPipelineAborted) {
		reason = cause
	}
	s.persistPipelineStatus(context.Background(), recordID, "stopped", "", reason)
	if s.StopStore != nil {
		_ = s.StopStore.ClearStopRequested(context.Background(), recordID)
	}
	if s.Logger != nil {
		s.Logger.Info("finish processing request",
			"record_id", recordID,
			"proc_status", "stopped",
			"ms_used", time.Since(requestStart).Milliseconds(),
		)
	}
}

// persistPipelineWaiting marks recordID's doc_processing entry running, with
// note as its progress, while the task waits for admission. It has no
// processor name, so the stuck-pipeline auto-heal leaves it alone.
func (s *ControlService) persistPipelineWaiting(ctx context.Context, recordID int64, note string) {
	if s.InputStore == nil || recordID <= 0 {
		return
	}
	err := updateInputStatusAtomic(ctx, s.InputStore, recordID, func(current string) (DocMetadataUpdate, error) {
		raw, err := appendPipelineStatus(current, s.now(), "running", "", nil)
		if err != nil {
			return DocMetadataUpdate{}, err
		}
		raw, err = setPipelineProgress(raw, note)
		if err != nil {
			return DocMetadataUpdate{}, err
		}
		return DocMetadataUpdate{StatusRaw: raw}, nil
	})
	if err != nil && s.Logger != nil {
		s.Logger.Error("failed persisting doc pipeline waiting status", "record_id", recordID, "error", err)
	}
}

// setPipelineProgress sets the progress of raw's doc_processing entry.
func setPipelineProgress(raw, progress string) (string, error) {
	entries := decodeDocMetaStatus(raw)
	for _, e := range entries {
		if asString(e["operation"]) == "doc_processing" {
			e["progress"] = progress
		}
	}
	bs, err := json.Marshal(entries)
	if err != nil {
		return "", err
	}
	return string(bs), nil
}
