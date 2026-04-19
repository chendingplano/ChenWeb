package fileconverters

import (
	"errors"
	"testing"
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
