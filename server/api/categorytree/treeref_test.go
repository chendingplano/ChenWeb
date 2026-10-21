package categorytree

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
)

// A client that percent-encodes the colon in "cat:…" must reach the same tree as a raw colon.
func TestTreeRefUnescapesEncodedColon(t *testing.T) {
	e := echo.New()
	for _, path := range []string{
		"/api/v1/kb/category-trees/cat:general-tech-categories",
		"/api/v1/kb/category-trees/cat%3Ageneral-tech-categories",
	} {
		var got string
		e.GET("/api/v1/kb/category-trees/:name", func(c echo.Context) error {
			got = treeRef(c)
			return c.NoContent(http.StatusOK)
		})
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if got != "cat:general-tech-categories" {
			t.Fatalf("%s: treeRef = %q", path, got)
		}
		e = echo.New()
	}
}
