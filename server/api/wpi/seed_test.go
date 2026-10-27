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

// elementClassesMigration stores the element classes in format 0.3 as version
// 2, replacing those of seedMigration.
const elementClassesMigration = "../../../project_migrations/20261027000006_wpi_element_classes_0_3.sql"

var seedRow = regexp.MustCompile(`(?s)\('(app|page|element|action)', '([a-z0-9-]+)', [0-9]+, '0\.[23]', \$wpi\$(.*?)\$wpi\$::jsonb`)

// seedRows reads the descriptor rows of seed migrations, in order; a later
// file's row replaces an earlier one of the same kind and ID, as a newer version
// does. count is the number of rows read.
func seedRows(t *testing.T, files ...string) (rows map[Kind]map[string]string, count int) {
	t.Helper()
	rows = map[Kind]map[string]string{}
	for _, f := range files {
		sqlText, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range seedRow.FindAllStringSubmatch(string(sqlText), -1) {
			kind, id, body := Kind(m[1]), m[2], m[3]
			if rows[kind] == nil {
				rows[kind] = map[string]string{}
			}
			rows[kind][id] = body
			count++
		}
	}
	return rows, count
}

func TestSeedCustomerAdminIsValid(t *testing.T) {
	first, count := seedRows(t, seedMigration)
	if len(first[KindElement]) != 6 || len(first[KindAction]) != 5 || len(first[KindPage]) != 4 || count != 16 {
		t.Fatalf("seed has %d element and %d action classes, %d pages, %d rows; want 6, 5, 4, 16",
			len(first[KindElement]), len(first[KindAction]), len(first[KindPage]), count)
	}
	all, _ := seedRows(t, seedMigration, elementClassesMigration)
	if classes, n := seedRows(t, elementClassesMigration); n != 6 || len(classes[KindElement]) != 6 {
		t.Fatalf("%s has %d rows; want the 6 element classes", elementClassesMigration, n)
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
	res, err := BuildBundle(context.Background(), "customer-admin", fixtureLoader(t, all), demoColumns, registeredIn(nil))
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

// The pages demo (wpi-pages) adds the query and load-file classes, stored
// statements and a stored file, and new page versions in kb.frontend_pages.
const pagesSeedMigration = "../../../project_migrations/20261027000005_seed_wpi_pages_demo.sql"

var (
	pageSeedRow = regexp.MustCompile(`(?s)\('([a-z0-9-]+)', [0-9]+, '0\.2', \$wpi\$(.*?)\$wpi\$::jsonb`)
	stmtSeedRow = regexp.MustCompile(`\('([a-z0-9-]+)', \$sql\$`)
	fileSeedRow = regexp.MustCompile(`\('([a-z0-9-]+)', \$file\$`)
)

func TestSeedPagesDemoIsValid(t *testing.T) {
	demo, err := os.ReadFile(pagesSeedMigration)
	if err != nil {
		t.Fatal(err)
	}
	all, _ := seedRows(t, seedMigration, pagesSeedMigration, elementClassesMigration)
	// The pages of seedMigration are replaced by those of the demo, in kb.frontend_pages.
	all[KindPage] = map[string]string{}
	for _, m := range pageSeedRow.FindAllStringSubmatch(string(demo), -1) {
		all[KindPage][m[1]] = m[2]
	}
	for _, cls := range []string{"query", "load-file"} {
		expectIssue(t, ValidateDescriptor(KindAction, cls, decode(t, all[KindAction][cls])), "")
	}
	registered := map[string][]string{}
	for _, m := range stmtSeedRow.FindAllStringSubmatch(string(demo), -1) {
		registered[storedStatement] = append(registered[storedStatement], m[1])
	}
	for _, m := range fileSeedRow.FindAllStringSubmatch(string(demo), -1) {
		registered[storedFile] = append(registered[storedFile], m[1])
	}
	if len(all[KindPage]) != 6 || len(registered[storedStatement]) != 2 || len(registered[storedFile]) != 1 {
		t.Fatalf("seed has %d pages, %v; want 6 pages, 2 statements, 1 file", len(all[KindPage]), registered)
	}
	demoColumns := func(_ context.Context, table string) (map[string]bool, error) {
		if table != "kb.wpi_demo_customers" {
			return nil, nil
		}
		return map[string]bool{"id": true, "user_name": true, "phone_number": true, "email": true}, nil
	}
	res, err := BuildBundle(context.Background(), "customer-admin", fixtureLoader(t, all), demoColumns, registeredIn(registered))
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Errors) != 0 || len(res.Warnings) != 0 {
		t.Fatalf("errors %+v\nwarnings %+v", res.Errors, res.Warnings)
	}
	if b := res.Bundle; len(b.Pages) != 6 {
		t.Fatalf("app reaches %d pages, seed has 6", len(b.Pages))
	}
}
