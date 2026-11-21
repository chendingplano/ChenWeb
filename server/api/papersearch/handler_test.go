package papersearch

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
)

func TestSearchRejectsEmptyAndReturnsResults(t *testing.T) {
	e := echo.New()
	h := Handler{Search: func(_ context.Context, query string) ([]byte, error) {
		return []byte(`{"papers":[{"title":"` + query + `"}],"errors":{}}`), nil
	}}
	for _, tc := range []struct {
		body     string
		status   int
		contains string
	}{{`{"query":"  "}`, 400, ""}, {`{"query":"test"}`, 200, `"title":"test"`}} {
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(tc.body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		if err := h.Handle(e.NewContext(req, rec)); err != nil {
			t.Fatal(err)
		}
		if rec.Code != tc.status || !strings.Contains(rec.Body.String(), tc.contains) {
			t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
		}
	}
}
