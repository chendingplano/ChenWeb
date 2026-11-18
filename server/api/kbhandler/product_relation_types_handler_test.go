package kbhandler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/EchoFactory"
	"github.com/labstack/echo/v4"
)

type relationTypeStub struct {
	approveCalls, updateCalls int
	weight                    float64
	actor                     string
	deleteErr                 error
}

func (s *relationTypeStub) List(context.Context) ([]productRelationTypeRow, error) {
	return []productRelationTypeRow{{Code: "pkg", Status: "proposed"}}, nil
}
func (s *relationTypeStub) Approve(_ context.Context, code, actor string, _ productRelationTypeInput, w float64) (productRelationTypeRow, error) {
	s.approveCalls++
	s.weight, s.actor = w, actor
	return productRelationTypeRow{Code: code, Status: "approved", Weight: w}, nil
}
func (s *relationTypeStub) Update(_ context.Context, code string, _ productRelationTypeInput, w float64) (productRelationTypeRow, error) {
	s.updateCalls++
	return productRelationTypeRow{Code: code, Weight: w}, nil
}
func (s *relationTypeStub) Delete(context.Context, string) error { return s.deleteErr }

func relationTypeContext(t *testing.T, method, body string, user *ApiTypes.UserInfo) (echo.Context, *httptest.ResponseRecorder) {
	t.Helper()
	old := EchoFactory.DefaultAuthenticator
	EchoFactory.DefaultAuthenticator = func(ApiTypes.RequestContext) (*ApiTypes.UserInfo, error) { return user, nil }
	t.Cleanup(func() { EchoFactory.DefaultAuthenticator = old })
	req := httptest.NewRequest(method, "/api/v1/kb/product-relation-types/pkg", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	c.SetParamNames("code")
	c.SetParamValues("PKG")
	return c, rec
}

var relationAdmin = &ApiTypes.UserInfo{UserId: "admin1", Roles: []string{"admin"}}

func TestProductRelationTypeApproveValidation(t *testing.T) {
	for name, body := range map[string]string{
		"missing english": `{"description_en":"","description_zh":"包装","weight":0.5}`,
		"missing chinese": `{"description_en":"packaging","description_zh":"  ","weight":0.5}`,
		"weight too high": `{"description_en":"p","description_zh":"包","weight":1.5}`,
		"weight negative": `{"description_en":"p","description_zh":"包","weight":-0.1}`,
	} {
		t.Run(name, func(t *testing.T) {
			c, rec := relationTypeContext(t, http.MethodPost, body, relationAdmin)
			stub := &relationTypeStub{}
			_ = writeProductRelationType(c, stub, true)
			if rec.Code != http.StatusBadRequest || stub.approveCalls != 0 {
				t.Fatalf("code=%d approveCalls=%d, want 400 and no approval", rec.Code, stub.approveCalls)
			}
		})
	}
}

func TestProductRelationTypeApproveDefaultsWeightAndNormalizesCode(t *testing.T) {
	c, rec := relationTypeContext(t, http.MethodPost, `{"description_en":"packaging","description_zh":"包装"}`, relationAdmin)
	stub := &relationTypeStub{}
	_ = writeProductRelationType(c, stub, true)
	if rec.Code != http.StatusOK || stub.weight != 0.5 || stub.actor != "admin1" {
		t.Fatalf("code=%d weight=%v actor=%q", rec.Code, stub.weight, stub.actor)
	}
	if !strings.Contains(rec.Body.String(), `"code":"pkg"`) {
		t.Fatalf("code not lower-cased: %s", rec.Body.String())
	}
}

func TestProductRelationTypeAuthorization(t *testing.T) {
	for _, tc := range []struct {
		user *ApiTypes.UserInfo
		want int
	}{{nil, http.StatusUnauthorized}, {&ApiTypes.UserInfo{UserId: "u"}, http.StatusForbidden}} {
		c, rec := relationTypeContext(t, http.MethodPost, `{}`, tc.user)
		stub := &relationTypeStub{}
		_ = writeProductRelationType(c, stub, true)
		if rec.Code != tc.want || stub.approveCalls != 0 {
			t.Fatalf("code=%d want %d", rec.Code, tc.want)
		}
	}
}

func TestProductRelationTypeDeleteRefusals(t *testing.T) {
	for err, want := range map[error]int{
		errRelationTypeInUse:    http.StatusConflict,
		errRelationTypeApproved: http.StatusConflict,
		errRelationTypeNotFound: http.StatusNotFound,
		nil:                     http.StatusOK,
	} {
		c, rec := relationTypeContext(t, http.MethodDelete, ``, relationAdmin)
		_ = deleteProductRelationType(c, &relationTypeStub{deleteErr: err})
		if rec.Code != want {
			t.Fatalf("err=%v code=%d want %d", err, rec.Code, want)
		}
	}
}

func TestProductRelationTypeUpdateKeepsStatusOut(t *testing.T) {
	c, rec := relationTypeContext(t, http.MethodPut, `{"description_en":"e","description_zh":"中","weight":0.9}`, relationAdmin)
	stub := &relationTypeStub{}
	_ = writeProductRelationType(c, stub, false)
	if rec.Code != http.StatusOK || stub.updateCalls != 1 || stub.approveCalls != 0 {
		t.Fatalf("code=%d update=%d approve=%d", rec.Code, stub.updateCalls, stub.approveCalls)
	}
}
