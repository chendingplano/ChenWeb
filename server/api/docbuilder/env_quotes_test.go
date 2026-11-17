package docbuilder

import (
	"context"
	"strings"
	"testing"
)

// ChenWeb#14: a DOCUMENT_ROOT_DIR with literal quotes was resolved under the working directory.
func TestNewStoreFromEnvTrimsLiteralQuotes(t *testing.T) {
	dir := t.TempDir()
	for _, v := range []string{`"` + dir + `"`, `'` + dir + `'`, ` "` + dir + `" `} {
		t.Setenv("DOCUMENT_ROOT_DIR", v)
		_, err := NewStoreFromEnv(context.Background(), nil, nil, "test", nil)
		if err == nil || strings.Contains(err.Error(), "is not a directory") {
			t.Fatalf("value %q: want a later-stage error, got %v", v, err)
		}
	}
}
