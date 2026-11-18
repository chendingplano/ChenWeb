package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

type levelRecordingLogger struct{ levels []string }

func (l *levelRecordingLogger) Debug(string, ...any) { l.levels = append(l.levels, "debug") }
func (l *levelRecordingLogger) Line(string, ...any)  { l.levels = append(l.levels, "line") }
func (l *levelRecordingLogger) Info(string, ...any)  { l.levels = append(l.levels, "info") }
func (l *levelRecordingLogger) Warn(string, ...any)  { l.levels = append(l.levels, "warn") }
func (l *levelRecordingLogger) Error(string, ...any) { l.levels = append(l.levels, "error") }
func (l *levelRecordingLogger) Trace(string)         { l.levels = append(l.levels, "trace") }
func (l *levelRecordingLogger) Close()               {}

func TestDevFrontendProxyErrorHandler(t *testing.T) {
	cases := []struct {
		name      string
		err       error
		wantLevel string
	}{
		{"client canceled", context.Canceled, "debug"},
		{"client canceled, wrapped", fmt.Errorf("read upstream: %w", context.Canceled), "debug"},
		{"upstream unreachable", errors.New("dial tcp 127.0.0.1:5173: connect: connection refused"), "error"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			logger := &levelRecordingLogger{}
			rec := httptest.NewRecorder()

			devFrontendProxyErrorHandler(logger)(rec, httptest.NewRequest(http.MethodGet, "/src/app.ts", nil), tc.err)

			if rec.Code != http.StatusBadGateway {
				t.Errorf("status = %d, want %d", rec.Code, http.StatusBadGateway)
			}
			if len(logger.levels) != 1 || logger.levels[0] != tc.wantLevel {
				t.Errorf("log levels = %v, want [%s]", logger.levels, tc.wantLevel)
			}
		})
	}
}
