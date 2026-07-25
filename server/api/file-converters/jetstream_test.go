package fileconverters

import (
	"errors"
	"sync"
	"testing"
	"time"
)

func TestIsNonFatalEnsureStreamErr(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "nil",
			err:  nil,
			want: false,
		},
		{
			name: "already in use",
			err:  errors.New("nats: stream name already in use"),
			want: true,
		},
		{
			name: "subjects overlap",
			err:  errors.New("nats: subjects overlap with an existing stream"),
			want: false,
		},
		{
			name: "other error",
			err:  errors.New("nats: permission violation"),
			want: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := isNonFatalEnsureStreamErr(tc.err); got != tc.want {
				t.Fatalf("isNonFatalEnsureStreamErr() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestShouldAckOnHandlerError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "missing parsed success",
			err:  ErrMissingParsedSuccess,
			want: true,
		},
		{
			name: "not found",
			err:  ErrNotFound,
			want: true,
		},
		{
			name: "result file not found",
			err:  ErrResultFileNotFound,
			want: true,
		},
		{
			name: "unsupported parser wrapped",
			err:  errors.New(`(MID_26041110) unsupported parser_name="mineru": unsupported parser_name`),
			want: false,
		},
		{
			name: "unsupported parser via sentinel",
			err:  errors.Join(ErrUnsupportedParserName, errors.New("details")),
			want: true,
		},
		{
			name: "parser not implemented",
			err:  ErrParserNotImplemented,
			want: true,
		},
		{
			name: "transient error",
			err:  errors.New("db timeout"),
			want: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := shouldAckOnHandlerError(tc.err); got != tc.want {
				t.Fatalf("shouldAckOnHandlerError() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestWaitWithGraceDrainsBeforeGracePeriod(t *testing.T) {
	var wg sync.WaitGroup
	wg.Go(func() {
		time.Sleep(10 * time.Millisecond)
	})

	start := time.Now()
	if !waitWithGrace(&wg, time.Second) {
		t.Fatal("waitWithGrace() = false, want true (wg drained well within grace)")
	}
	if elapsed := time.Since(start); elapsed >= time.Second {
		t.Fatalf("waitWithGrace() took %v, want well under the 1s grace period", elapsed)
	}
}

func TestDefaultStreamNameForSubjectAvoidsCollisionAcrossSubjects(t *testing.T) {
	// doc-processor's primary subject and parser-result-converter's subject
	// both fall back to EnsureStream's default when their stream env vars
	// are unset. They must not resolve to the same stream name.
	docProcessorSubject := "kb.pdf.start-doc-processing"
	parserResultSubject := "kb.pdf.parsed"

	got1 := defaultStreamNameForSubject(docProcessorSubject)
	got2 := defaultStreamNameForSubject(parserResultSubject)

	if got1 == "" || got2 == "" {
		t.Fatalf("defaultStreamNameForSubject returned empty name: got1=%q got2=%q", got1, got2)
	}
	if got1 == got2 {
		t.Fatalf("defaultStreamNameForSubject collided for different subjects %q and %q: both resolved to %q",
			docProcessorSubject, parserResultSubject, got1)
	}
}

func TestWaitWithGraceTimesOutOnStuckWork(t *testing.T) {
	var wg sync.WaitGroup
	wg.Add(1) // never Done() — simulates a handler stuck past shutdown

	if waitWithGrace(&wg, 20*time.Millisecond) {
		t.Fatal("waitWithGrace() = true, want false (wg never drains)")
	}
}
