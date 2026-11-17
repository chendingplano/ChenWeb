package attention

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/EchoFactory"
	"github.com/labstack/echo/v4"
)

type stubStore struct {
	items      []Item
	item       Item
	err        error
	filter     ListFilter
	closedID   int64
	closedAs   string
	actor      string
	resolution string
}

func (s *stubStore) Get(_ context.Context, id int64) (Item, error) {
	return s.item, s.err
}

func (s *stubStore) List(_ context.Context, f ListFilter) ([]Item, int, error) {
	s.filter = f
	return s.items, len(s.items), s.err
}

func (s *stubStore) Close(_ context.Context, id int64, status, actor, resolution string) (Item, error) {
	s.closedID, s.closedAs, s.actor, s.resolution = id, status, actor, resolution
	return s.item, s.err
}

func testContext(t *testing.T, method, target, body, id string, user *ApiTypes.UserInfo) (echo.Context, *httptest.ResponseRecorder) {
	t.Helper()
	old := EchoFactory.DefaultAuthenticator
	EchoFactory.DefaultAuthenticator = func(ApiTypes.RequestContext) (*ApiTypes.UserInfo, error) { return user, nil }
	t.Cleanup(func() { EchoFactory.DefaultAuthenticator = old })
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	if body != "" {
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	}
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	if id != "" {
		c.SetParamNames("id")
		c.SetParamValues(id)
	}
	return c, rec
}

var alice = &ApiTypes.UserInfo{UserId: "u1", UserName: "alice"}

func TestListItemsPassesFilters(t *testing.T) {
	store := &stubStore{items: []Item{{ID: 7, ItemType: TypeAmbiguousObjectName}}}
	c, rec := testContext(t, http.MethodGet, "/api/v1/kb/attention-items?type=ambiguous-object-name&status=open&limit=20&offset=40", "", "", alice)
	if err := listItems(c, store); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d body = %s", rec.Code, rec.Body)
	}
	want := ListFilter{ItemType: TypeAmbiguousObjectName, Status: StatusOpen, Limit: 20, Offset: 40}
	if store.filter != want {
		t.Fatalf("filter = %+v", store.filter)
	}
	var body struct {
		Items []Item `json:"items"`
		Total int    `json:"total"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || len(body.Items) != 1 || body.Total != 1 {
		t.Fatalf("body = %s", rec.Body)
	}
}

func TestListItemsRejectsBadFilters(t *testing.T) {
	for _, q := range []string{"type=nope", "status=closed"} {
		c, rec := testContext(t, http.MethodGet, "/api/v1/kb/attention-items?"+q, "", "", alice)
		if err := listItems(c, &stubStore{}); err != nil {
			t.Fatal(err)
		}
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("%s: code = %d", q, rec.Code)
		}
	}
}

func TestRequiresAuthentication(t *testing.T) {
	c, rec := testContext(t, http.MethodGet, "/api/v1/kb/attention-items", "", "", nil)
	if err := listItems(c, &stubStore{}); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("code = %d", rec.Code)
	}
}

func TestResolveItemRecordsActorAndNote(t *testing.T) {
	store := &stubStore{item: Item{ID: 5, Status: StatusResolved}}
	c, rec := testContext(t, http.MethodPost, "/api/v1/kb/attention-items/5/resolve", `{"resolution":"merged obj_285 into obj_279"}`, "5", alice)
	if err := closeItem(c, store, StatusResolved); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusOK || store.closedID != 5 || store.closedAs != StatusResolved || store.actor != "alice" || store.resolution != "merged obj_285 into obj_279" {
		t.Fatalf("code = %d store = %+v", rec.Code, store)
	}
}

func TestDismissWithoutBody(t *testing.T) {
	store := &stubStore{item: Item{ID: 5, Status: StatusDismissed}}
	c, rec := testContext(t, http.MethodPost, "/api/v1/kb/attention-items/5/dismiss", "", "5", alice)
	if err := closeItem(c, store, StatusDismissed); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusOK || store.closedAs != StatusDismissed || store.resolution != "" {
		t.Fatalf("code = %d store = %+v", rec.Code, store)
	}
}

func TestCloseErrors(t *testing.T) {
	for _, tc := range []struct {
		err  error
		want int
	}{
		{ErrNotFound, http.StatusNotFound},
		{ErrNotOpen, http.StatusConflict},
	} {
		c, rec := testContext(t, http.MethodPost, "/api/v1/kb/attention-items/5/resolve", "", "5", alice)
		if err := closeItem(c, &stubStore{err: tc.err}, StatusResolved); err != nil {
			t.Fatal(err)
		}
		if rec.Code != tc.want {
			t.Fatalf("%v: code = %d", tc.err, rec.Code)
		}
	}
}

func TestGetItemNotFoundAndBadID(t *testing.T) {
	c, rec := testContext(t, http.MethodGet, "/api/v1/kb/attention-items/9", "", "9", alice)
	if err := getItem(c, &stubStore{err: ErrNotFound}); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusNotFound {
		t.Fatalf("code = %d", rec.Code)
	}
	c, rec = testContext(t, http.MethodGet, "/api/v1/kb/attention-items/x", "", "x", alice)
	if err := getItem(c, &stubStore{}); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("code = %d", rec.Code)
	}
}
