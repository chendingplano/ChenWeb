package wpi

import (
	"context"
	"os"
	"regexp"
	"testing"
)

// The customer-admin demo descriptors live only in the seed migration. This
// test reads them from there and checks that they form a valid app against the
// demo table's columns, so a broken seed fails here instead of at startup.
const seedMigration = "../../../project_migrations/20261026000003_seed_wpi_customer_admin.sql"

var seedRow = regexp.MustCompile(`(?s)\('(app|page|element|action)', '([a-z0-9-]+)', 1, '0\.1', \$wpi\$(.*?)\$wpi\$::jsonb`)

func TestSeedCustomerAdminIsValid(t *testing.T) {
	sqlText, err := os.ReadFile(seedMigration)
	if err != nil {
		t.Fatal(err)
	}
	all := map[Kind]map[string]Descriptor{}
	count := 0
	for _, m := range seedRow.FindAllStringSubmatch(string(sqlText), -1) {
		kind, id, body := Kind(m[1]), m[2], m[3]
		if all[kind] == nil {
			all[kind] = map[string]Descriptor{}
		}
		all[kind][id] = Descriptor{Kind: kind, ID: id, Version: 1, Raw: []byte(body), Body: decode(t, body)}
		count++
	}
	if count != 13 {
		t.Fatalf("found %d seed descriptors, want 13", count)
	}
	load := func(_ context.Context, kind Kind, ids []string) (map[string]Descriptor, error) {
		out := map[string]Descriptor{}
		for _, id := range ids {
			if d, ok := all[kind][id]; ok {
				out[id] = d
			}
		}
		return out, nil
	}
	demoColumns := func(_ context.Context, table string) (map[string]bool, error) {
		if table != "kb.wpi_demo_customers" {
			return nil, nil
		}
		return map[string]bool{"id": true, "user_name": true, "phone_number": true, "email": true, "created_at": true, "updated_at": true}, nil
	}
	res, err := BuildBundle(context.Background(), "customer-admin", load, demoColumns)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Errors) != 0 || len(res.Warnings) != 0 {
		t.Fatalf("errors %+v\nwarnings %+v", res.Errors, res.Warnings)
	}
	b := res.Bundle
	if reached := 1 + len(b.Pages) + len(b.Elements) + len(b.Actions); reached != count {
		t.Fatalf("app reaches %d descriptors, seed has %d", reached, count)
	}
}
