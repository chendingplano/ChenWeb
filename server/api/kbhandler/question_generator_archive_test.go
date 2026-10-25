package kbhandler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
)

func TestArchiveGeneratedQuestionsInvalidRequest(t *testing.T) {
	for _, body := range []string{`{`, `{}`, `{"ids":[]}`, `{"ids":[0]}`, `{"ids":[-1]}`, `{"ids":[1.5]}`, `{"ids":["1"]}`, `{"ids":[1]} {}`, `{"ids":[` + strings.Repeat("1,", 1000) + `1]}`} {
		rec := httptest.NewRecorder()
		c := echo.New().NewContext(httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body)), rec)
		if err := ArchiveGeneratedQuestions(c); err != nil || rec.Code != http.StatusBadRequest {
			t.Errorf("body=%s status=%d error=%v", body, rec.Code, err)
		}
	}
}
