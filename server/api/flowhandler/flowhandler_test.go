package flowhandler_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/chendingplano/deepdoc/server/api/flowhandler"
	"github.com/labstack/echo/v4"
)

func newEcho() *echo.Echo { return echo.New() }

func TestListFlows_ReturnsOK(t *testing.T) {
	e := newEcho()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/flows?scope=mine", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	if err := flowhandler.ListFlows(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"flows"`) {
		t.Fatalf("expected flows key in response, got: %s", rec.Body.String())
	}
}

func TestCreateFlow_ReturnsCreated(t *testing.T) {
	e := newEcho()
	body := `{"flow_name":"Test","flow_data":{"nodes":[],"edges":[]},"is_shared":false}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/flows", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	if err := flowhandler.CreateFlow(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"flow"`) {
		t.Fatalf("expected flow key in response, got: %s", rec.Body.String())
	}
}

func TestGetFlow_ReturnsOK(t *testing.T) {
	e := newEcho()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/flows/1", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("1")
	if err := flowhandler.GetFlow(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestDeleteFlow_ReturnsOK(t *testing.T) {
	e := newEcho()
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/flows/1", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("1")
	if err := flowhandler.DeleteFlow(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestGetDefaultFlow_Returns200(t *testing.T) {
	e := newEcho()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/flows/default", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	if err := flowhandler.GetDefaultFlow(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestSetDefaultFlow_Returns200(t *testing.T) {
	e := newEcho()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/flows/1/default", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("1")
	if err := flowhandler.SetDefaultFlow(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestForkFlow_Returns201(t *testing.T) {
	e := newEcho()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/flows/1/fork", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("1")
	if err := flowhandler.ForkFlow(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", rec.Code)
	}
}

func TestSaveAsTemplate_Returns201(t *testing.T) {
	e := newEcho()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/flows/1/template", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("1")
	if err := flowhandler.SaveAsTemplate(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", rec.Code)
	}
}

func TestGetNodeTypes_Returns11Types(t *testing.T) {
	e := newEcho()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/flow-node-types", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	if err := flowhandler.GetNodeTypes(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"nodeTypes"`) {
		t.Fatalf("missing nodeTypes key: %s", rec.Body.String())
	}
	// Count occurrences of "id" to verify 11 node types
	count := strings.Count(rec.Body.String(), `"id":"`)
	if count != 11 {
		t.Fatalf("expected 11 node types, found %d", count)
	}
}
