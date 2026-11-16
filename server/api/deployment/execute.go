package deployment

import (
	"bufio"
	"errors"
	"io"
	"os/exec"
	"time"
)

// Execute runs cmd, feeds its combined stdout and stderr to tr line by line, and calls
// save with the changed steps at most once per flushEvery. It returns the exit code
// (-1 when the command could not run) and the start or wait error, if any.
func Execute(cmd *exec.Cmd, tr *Tracker, save func([]Step), flushEvery time.Duration) (int, error) {
	pr, pw := io.Pipe()
	cmd.Stdout, cmd.Stderr = pw, pw
	// Stop waiting for output a few seconds after the script exits, even if a child
	// it left behind still holds the pipe.
	cmd.WaitDelay = 5 * time.Second
	if err := cmd.Start(); err != nil {
		return -1, err
	}

	readDone := make(chan struct{})
	go func() {
		defer close(readDone)
		r := bufio.NewReader(pr)
		for {
			line, err := r.ReadString('\n')
			if line != "" {
				tr.Line(line)
			}
			if err != nil {
				return
			}
		}
	}()

	stopFlush := make(chan struct{})
	flushDone := make(chan struct{})
	go func() {
		defer close(flushDone)
		ticker := time.NewTicker(flushEvery)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if steps := tr.TakeDirty(); len(steps) > 0 {
					save(steps)
				}
			case <-stopFlush:
				return
			}
		}
	}()

	waitErr := cmd.Wait()
	pw.Close()
	<-readDone
	close(stopFlush)
	<-flushDone

	var exitErr *exec.ExitError
	if errors.As(waitErr, &exitErr) {
		return exitErr.ExitCode(), nil
	}
	if waitErr != nil {
		return -1, waitErr
	}
	return 0, nil
}
