package wpi

import (
	"database/sql"
	"encoding/json"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"
)

// Run against a migrated database with WPI_TEST_DB=1 and standard PG* env vars.
// Unlike the seed fixture, this checks the versions the endpoint really loads.
func TestCustomersDemoDatabase(t *testing.T) {
	if os.Getenv("WPI_TEST_DB") != "1" {
		t.Skip("set WPI_TEST_DB=1 to check the migrated database")
	}
	db, err := sql.Open("postgres", "sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	h := testHandler(db, adminUser)
	for _, appID := range []string{"customers-demo", "customer-admin"} {
		t.Run(appID, func(t *testing.T) {
			res := call(t, h.GetApp, "GET", "", "appId", appID)
			if res.Code != 200 {
				t.Fatalf("app returned %d: %s", res.Code, res.Body.String())
			}
			if appID == "customers-demo" {
				var bundle Bundle
				if err := json.Unmarshal(res.Body.Bytes(), &bundle); err != nil {
					t.Fatal(err)
				}
				if len(bundle.Pages) != 1 || bundle.Pages["demo-customers"] == nil {
					t.Fatalf("demo must load only its isolated page, got %v", bundle.Pages)
				}
			}
		})
	}
}

// Exercise PostgreSQL's parameter typing and latest-version filtering, which
// sqlmock cannot check. This test only reads the migrated database.
func TestPageSearchDatabase(t *testing.T) {
	if os.Getenv("WPI_TEST_DB") != "1" {
		t.Skip("set WPI_TEST_DB=1 to check the migrated database")
	}
	db, err := sql.Open("postgres", "sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	h := testHandler(db, plainUser)
	type result struct {
		Pages []PageSummary `json:"pages"`
		More  bool          `json:"has_more"`
	}
	search := func(params url.Values) result {
		t.Helper()
		rec := callQuery(t, h.SearchPages, params.Encode())
		if rec.Code != 200 {
			t.Fatalf("%d: %s", rec.Code, rec.Body)
		}
		var got result
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		return got
	}
	first := search(url.Values{"limit": {"1"}})
	if len(first.Pages) != 1 {
		t.Fatal("expected a migrated page")
	}
	p := first.Pages[0]
	if first.More {
		next := search(url.Values{"limit": {"1"}, "offset": {"1"}})
		if len(next.Pages) != 1 || next.Pages[0].DescriptorID == p.DescriptorID {
			t.Fatal("offset did not advance")
		}
	}
	filtered := search(url.Values{"descriptor_id": {strings.ToUpper(p.DescriptorID)}, "created_from": {p.CreatedAt.Format(time.RFC3339Nano)}, "updated_from": {p.UpdatedAt.Format(time.RFC3339Nano)}})
	if len(filtered.Pages) == 0 || filtered.Pages[0].DescriptorID != p.DescriptorID {
		t.Fatal("inclusive filter missed the page")
	}
	excluded := search(url.Values{"descriptor_id": {p.DescriptorID}, "created_to": {p.CreatedAt.Format(time.RFC3339Nano)}})
	for _, row := range excluded.Pages {
		if row.DescriptorID == p.DescriptorID {
			t.Fatal("exclusive end included the page")
		}
	}
	if len(search(url.Values{"descriptor_id": {"does-not-exist-%_"}}).Pages) != 0 {
		t.Fatal("wildcards must be literal")
	}
}
