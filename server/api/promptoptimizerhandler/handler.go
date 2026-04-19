// Package promptoptimizerhandler implements the Prompt Optimizer Studio
// endpoints: per-user LLM model registry (with encrypted API keys), prompt
// templates, run history, favorites, named variables, and the optimize /
// iterate / test / analyze operations that proxy to provider APIs through
// the shared llm client.
//
// Routes are mounted under /api/v1/prompt-optimizer/ by
// ChenWeb/server/api/routes.go. All endpoints require an authenticated
// session and scope every read/write by the caller's user id.
package promptoptimizerhandler

import (
	"errors"
	"net/http"
	"sync"

	"github.com/chendingplano/shared/go/api/EchoFactory"
	"github.com/chendingplano/shared/go/api/security"
	"github.com/labstack/echo/v4"
)

// ErrNotInitialized is returned by handlers when the package has not been
// initialized with an encryption key. main.go must call Init() before any
// request reaches these handlers.
var ErrNotInitialized = errors.New("promptoptimizerhandler: not initialized (call Init first)")

var (
	encKeyMu sync.RWMutex
	encKey   []byte
)

// Init stores the AES-256-GCM key used to encrypt/decrypt per-user API keys
// at rest. key MUST be exactly security.AESGCMKeyLen bytes. Typically main
// loads it via security.LoadKeyFromEnv("PROMPT_OPTIMIZER_ENCRYPTION_KEY").
func Init(key []byte) error {
	if len(key) != security.AESGCMKeyLen {
		return security.ErrKeyLength
	}
	encKeyMu.Lock()
	encKey = append(encKey[:0], key...)
	encKeyMu.Unlock()
	return nil
}

// getKey returns the key set by Init, or ErrNotInitialized if Init has not
// been called.
func getKey() ([]byte, error) {
	encKeyMu.RLock()
	defer encKeyMu.RUnlock()
	if len(encKey) != security.AESGCMKeyLen {
		return nil, ErrNotInitialized
	}
	k := make([]byte, len(encKey))
	copy(k, encKey)
	return k, nil
}

// requireUser resolves the authenticated user from the Echo context. On
// failure it writes a 403 JSON body and returns ok == false; callers
// should simply return nil.
func requireUser(c echo.Context, loc string) (userID string, ok bool) {
	rc := EchoFactory.NewFromEcho(c, loc)
	defer rc.Close()
	info := rc.IsAuthenticated()
	if info == nil {
		_ = c.JSON(http.StatusForbidden, ErrorResponse{Status: false, ErrorMsg: "login required (" + loc + ")"})
		return "", false
	}
	return info.UserId, true
}

// jsonError mirrors docgenhandler's ErrorResponse shape. Callers pass the
// module-location code as the suffix so logs and error bodies share a
// traceable identifier.
func jsonError(c echo.Context, status int, msg string) error {
	return c.JSON(status, ErrorResponse{Status: false, ErrorMsg: msg})
}
