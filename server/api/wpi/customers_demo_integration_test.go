package wpi

import (
	"database/sql"
	"encoding/json"
	"os"
	"testing"
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
