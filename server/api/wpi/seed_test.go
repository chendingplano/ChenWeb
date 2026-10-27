package wpi

import (
	"context"
	"os"
	"regexp"
	"testing"
)

// The classes and the customer-admin demo descriptors live only in the seed
// migration. This test reads them from there and checks that every class row
// is valid and that they form a valid app against the demo table's columns,
// so a broken seed fails here instead of at startup.
const seedMigration = "../../../project_migrations/20261027000002_seed_wpi_classes_and_instances.sql"

var seedRow = regexp.MustCompile(`(?s)\('(app|page|element|action)', '([a-z0-9-]+)', [0-9]+, '0\.2', \$wpi\$(.*?)\$wpi\$::jsonb`)

func TestSeedCustomerAdminIsValid(t *testing.T) {
	sqlText, err := os.ReadFile(seedMigration)
	if err != nil {
		t.Fatal(err)
	}
	all := map[Kind]map[string]string{}
	count := 0
	for _, m := range seedRow.FindAllStringSubmatch(string(sqlText), -1) {
		kind, id, body := Kind(m[1]), m[2], m[3]
		if all[kind] == nil {
			all[kind] = map[string]string{}
		}
		all[kind][id] = body
		count++
	}
	if len(all[KindElement]) != 6 || len(all[KindAction]) != 5 || len(all[KindPage]) != 4 || count != 16 {
		t.Fatalf("seed has %d element and %d action classes, %d pages, %d rows; want 6, 5, 4, 16",
			len(all[KindElement]), len(all[KindAction]), len(all[KindPage]), count)
	}
	for _, kind := range []Kind{KindElement, KindAction} {
		for id, body := range all[kind] {
			expectIssue(t, ValidateDescriptor(kind, id, decode(t, body)), "")
		}
	}
	demoColumns := func(_ context.Context, table string) (map[string]bool, error) {
		if table != "kb.wpi_demo_customers" {
			return nil, nil
		}
		return map[string]bool{"id": true, "user_name": true, "phone_number": true, "email": true, "created_at": true, "updated_at": true}, nil
	}
	res, err := BuildBundle(context.Background(), "customer-admin", fixtureLoader(t, all), demoColumns)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Errors) != 0 || len(res.Warnings) != 0 {
		t.Fatalf("errors %+v\nwarnings %+v", res.Errors, res.Warnings)
	}
	if b := res.Bundle; len(b.Pages) != 4 {
		t.Fatalf("app reaches %d pages, seed has 4", len(b.Pages))
	}
}
