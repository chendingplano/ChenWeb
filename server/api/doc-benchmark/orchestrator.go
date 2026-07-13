package docbenchmark

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"sort"
	"sync"
)

type WorkerProcess interface {
	Send(WorkerRequest) error
	Messages() <-chan WorkerMessage
	Wait() error
	Kill() error
}
type WorkerLauncher func(context.Context, ExperimentVariant) (WorkerProcess, error)

type OrchestratorConfig struct {
	Variants            []ExperimentVariant
	MaxParallelVariants int
	MaxParallelCases    int
	Launcher            WorkerLauncher
	RunID               string
	AllowDirty          bool
	Cases               []string
}

type Orchestrator struct{ Config OrchestratorConfig }

func (o Orchestrator) Run(ctx context.Context) error {
	if o.Config.Launcher == nil {
		return errors.New("orchestrator: launcher is required")
	}
	variants := append([]ExperimentVariant(nil), o.Config.Variants...)
	sort.SliceStable(variants, func(i, j int) bool { return variants[i].Name < variants[j].Name })
	limit := o.Config.MaxParallelVariants
	if limit <= 0 {
		limit = 1
	}
	sem := make(chan struct{}, limit)
	var wg sync.WaitGroup
	errs := make(chan error, len(variants))
	for _, v := range variants {
		v := v
		select {
		case sem <- struct{}{}:
		case <-ctx.Done():
			return ctx.Err()
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			if err := o.runVariant(ctx, v); err != nil {
				errs <- err
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		return err
	}
	return nil
}
func (o Orchestrator) runVariant(ctx context.Context, v ExperimentVariant) error {
	p, err := o.Config.Launcher(ctx, v)
	if err != nil {
		return err
	}
	defer p.Kill()
	if err = p.Send(WorkerRequest{Version: WorkerProtocolVersion, Type: "initialize", Variant: v}); err != nil {
		return err
	}
	var ready WorkerMessage
	select {
	case ready = <-p.Messages():
	case <-ctx.Done():
		_ = p.Kill()
		return ctx.Err()
	}
	if ready.Type != "ready" || ready.ConfigHash == "" || ready.Config.Hash == "" || ready.ConfigHash != ready.Config.Hash {
		return errors.New("orchestrator: invalid ready handshake")
	}
	if err = p.Send(WorkerRequest{Version: WorkerProtocolVersion, Type: "authorize", RunID: o.Config.RunID}); err != nil {
		return err
	}
	select {
	case m := <-p.Messages():
		if m.Type != "authorized" {
			return errors.New("orchestrator: authorization failed")
		}
	case <-ctx.Done():
		_ = p.Kill()
		return ctx.Err()
	}
	cases := append([]string(nil), o.Config.Cases...)
	sort.Strings(cases)
	for _, caseID := range cases {
		if err := p.Send(WorkerRequest{Version: WorkerProtocolVersion, Type: "case", RunID: o.Config.RunID, CaseID: caseID}); err != nil {
			return err
		}
		for {
			select {
			case m, ok := <-p.Messages():
				if !ok {
					return errors.New("orchestrator: worker exited before case result")
				}
				if m.Type == "heartbeat" {
					continue
				}
				if m.Type != "result" || m.CaseID != caseID {
					return errors.New("orchestrator: invalid case result")
				}
				if m.Error != nil {
					return fmt.Errorf("case %s: %s", caseID, m.Error.Message)
				}
				goto nextCase
			case <-ctx.Done():
				_ = p.Kill()
				return ctx.Err()
			}
		}
	nextCase:
	}
	if err := p.Send(WorkerRequest{Version: WorkerProtocolVersion, Type: "shutdown"}); err != nil {
		return err
	}
	return p.Wait()
}

// ExecWorkerProcess is a bounded, protocol-only subprocess transport.
type ExecWorkerProcess struct {
	cmd      *exec.Cmd
	in       io.WriteCloser
	messages chan WorkerMessage
	done     chan error
	maxLine  int
}

func LaunchExecWorker(ctx context.Context, executable string, maxOutputBytes int) (WorkerLauncher, error) {
	if executable == "" {
		return nil, errors.New("worker executable is required")
	}
	return func(ctx context.Context, _ ExperimentVariant) (WorkerProcess, error) {
		cmd := exec.CommandContext(ctx, executable, "worker")
		in, err := cmd.StdinPipe()
		if err != nil {
			return nil, err
		}
		out, err := cmd.StdoutPipe()
		if err != nil {
			return nil, err
		}
		cmd.Stderr = &boundedBuffer{max: maxOutputBytes}
		p := &ExecWorkerProcess{cmd: cmd, in: in, messages: make(chan WorkerMessage, 16), done: make(chan error, 1), maxLine: maxOutputBytes}
		if err = cmd.Start(); err != nil {
			return nil, err
		}
		go func() {
			sc := bufio.NewScanner(out)
			lim := maxOutputBytes
			if lim <= 0 {
				lim = 1 << 20
			}
			sc.Buffer(make([]byte, 4096), lim)
			for sc.Scan() {
				var m WorkerMessage
				if json.Unmarshal(sc.Bytes(), &m) == nil {
					p.messages <- m
				}
			}
			close(p.messages)
			p.done <- sc.Err()
		}()
		return p, nil
	}, nil
}
func (p *ExecWorkerProcess) Send(r WorkerRequest) error {
	b, e := json.Marshal(r)
	if e != nil {
		return e
	}
	_, e = p.in.Write(append(b, '\n'))
	return e
}
func (p *ExecWorkerProcess) Messages() <-chan WorkerMessage { return p.messages }
func (p *ExecWorkerProcess) Wait() error                    { return p.cmd.Wait() }
func (p *ExecWorkerProcess) Kill() error                    { _ = p.in.Close(); return p.cmd.Process.Kill() }

type boundedBuffer struct {
	max int
	b   []byte
}

func (b *boundedBuffer) Write(p []byte) (int, error) {
	if b.max <= 0 {
		return len(p), nil
	}
	n := len(p)
	if len(b.b)+n > b.max {
		n = b.max - len(b.b)
		if n < 0 {
			n = 0
		}
	}
	b.b = append(b.b, p[:n]...)
	return len(p), nil
}

var _ = fmt.Sprintf
