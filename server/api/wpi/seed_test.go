package wpi

import (
	"context"
	"encoding/json"
	"os"
	"regexp"
	"slices"
	"testing"
)

// The classes and the demo descriptors live only in seed migrations. These tests
// read them from there and check that they are valid, and that the demo apps,
// as the newest migration converts them, are valid against the demo table's
// columns, so a broken seed fails here instead of at startup.
// TestCustomersDemoDatabase checks the migrated database itself.
const seedMigration = "../../../project_migrations/20261027000002_seed_wpi_classes_and_instances.sql"

// elementClassesMigration stores the element classes in format 0.3 as version
// 2, replacing those of seedMigration.
const elementClassesMigration = "../../../project_migrations/20261027000006_wpi_element_classes_0_3.sql"

// formatMigration moves every element class to format 0.4 by transforming its
// latest row; it has no bodies of its own (OpenSpec wpi-code-modules, design D7).
const formatMigration = "../../../project_migrations/20261028000001_wpi_element_classes_0_4.sql"

// codeModulesMigration creates kb.code_modules with a row for each element code module.
const codeModulesMigration = "../../../project_migrations/20261028000002_create_kb_code_modules.sql"

// The pages demo (wpi-pages) adds the query and load-file classes, stored
// statements and a stored file, and new page versions in kb.frontend_pages.
const pagesSeedMigration = "../../../project_migrations/20261027000005_seed_wpi_pages_demo.sql"

// customersDemoMigration stores the customers-demo app and its page.
const customersDemoMigration = "../../../project_migrations/20261028000005_isolate_wpi_customers_demo.sql"

// ownPanesMigration adds the layouter classes and modules, converts every page
// to format 0.5 and adds customer-admin-home (OpenSpec wpi-pages-own-panes).
const ownPanesMigration = "../../../project_migrations/20261030000001_wpi_pages_own_panes.sql"

var moduleSeedRow = regexp.MustCompile(`\('([a-z0-9-]+)',\s*'[^']+',\s*'[^']*',\s*'(element|action|page|layouter|page-layouter|element-layouter)',\s*'([^']*)'`)

func readFile(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// The layouter code modules with code are the ones the migrations register: the
// flow layouter (moved to type element-layouter) and the page layouter.
func TestSeedLayouterModules(t *testing.T) {
	text := readFile(t, ownPanesMigration)
	flow := readFile(t, "../../../project_migrations/20261028000003_register_flow_layouter.sql")
	for name, ok := range map[string]bool{
		"page-layouter registered": regexp.MustCompile(`(?s)\('page-layouter', 'Page Layouter',.*?'page-layouter', 'interpreter-layouter-page.md'`).MatchString(text),
		"flow-layouter registered": regexp.MustCompile(`\('flow-layouter',(?s:.*?)'interpreter-layouter-flow.md'`).MatchString(flow),
		"flow-layouter moved":      regexp.MustCompile(`(?s)SET module_type = 'element-layouter'.*?WHERE code_module_id = 'flow-layouter'`).MatchString(text),
	} {
		if !ok {
			t.Fatalf("%s: not found in the migrations", name)
		}
	}
	if !slices.Equal(pageLayouterModules, []string{"page-layouter"}) || len(elementLayouterModules) != 1 || elementLayouterModules["flow-layouter"] == nil {
		t.Fatalf("layouter modules with code: %v, %v", pageLayouterModules, elementLayouterModules)
	}
}

// seededModules returns the active modules the migrations store, by kind.
func seededModules(t *testing.T) map[string][]string {
	t.Helper()
	var ids []string
	for _, m := range moduleSeedRow.FindAllStringSubmatch(readFile(t, codeModulesMigration), -1) {
		if m[2] == "element" {
			ids = append(ids, m[1])
		}
	}
	return map[string][]string{
		storedModule:                ids,
		storedPageLayouterModule:    {"page-layouter"},
		storedElementLayouterModule: {"flow-layouter"},
	}
}

// Every element code module with code has its row in kb.code_modules, naming its
// document, and every seeded element module has code.
func TestSeedCodeModules(t *testing.T) {
	seeded := map[string]string{}
	for _, m := range moduleSeedRow.FindAllStringSubmatch(readFile(t, codeModulesMigration), -1) {
		if m[2] == "element" {
			seeded[m[1]] = m[3]
		}
	}
	if len(seeded) != len(codeModules) {
		t.Fatalf("seeded element modules %v; code modules with code %d", seeded, len(codeModules))
	}
	for id := range codeModules {
		if seeded[id] != id+".md" {
			t.Fatalf("code module %q: seeded document %q, want %q", id, seeded[id], id+".md")
		}
	}
}

// toFormat04 applies formatMigration's transform to the seeded element classes,
// all active: schema_version 0.4 and code_module_id equal to the class ID.
func toFormat04(t *testing.T, rows map[Kind]map[string]string) {
	t.Helper()
	if _, err := os.Stat(formatMigration); err != nil {
		t.Fatal(err)
	}
	for id, body := range rows[KindElement] {
		m := decode(t, body)
		m["schema_version"] = "0.4"
		m["code_module_id"] = id
		rows[KindElement][id] = marshal(t, m)
	}
}

func marshal(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

var (
	seedRow     = regexp.MustCompile(`(?s)\('(app|page|element|action|page-layouter|element-layouter)', '([a-z0-9-]+)', [0-9]+, '0\.[2-5]', \$wpi\$(.*?)\$wpi\$::jsonb`)
	pageSeedRow = regexp.MustCompile(`(?s)\('([a-z0-9-]+)', [0-9]+, '0\.2', \$wpi\$(.*?)\$wpi\$::jsonb`)
	newRow      = regexp.MustCompile(`(?s)\('([a-z0-9-]+)', \$wpi\$(.*?)\$wpi\$::jsonb\)`)
	stmtSeedRow = regexp.MustCompile(`\('([a-z0-9-]+)', \$sql\$`)
	fileSeedRow = regexp.MustCompile(`\('([a-z0-9-]+)', \$file\$`)
)

// seedRows reads the descriptor rows of seed migrations, in order; a later
// file's row replaces an earlier one of the same kind and ID, as a newer version
// does.
func seedRows(t *testing.T, files ...string) map[Kind]map[string]string {
	t.Helper()
	rows := map[Kind]map[string]string{}
	for _, f := range files {
		for _, m := range seedRow.FindAllStringSubmatch(readFile(t, f), -1) {
			kind, id, body := Kind(m[1]), m[2], m[3]
			if rows[kind] == nil {
				rows[kind] = map[string]string{}
			}
			rows[kind][id] = body
		}
	}
	return rows
}

// page05 mirrors pg_temp.wpi_page_05 of ownPanesMigration: a format 0.2 page as a
// single-pane page of format 0.5 whose panel main holds the page's elements.
func page05(t *testing.T, body string) string {
	t.Helper()
	b := decode(t, body)
	root := map[string]any{"pane_id": "main", "hori_align": "left"}
	for _, k := range []string{"left_margin", "right_margin", "top_margin", "bottom_margin"} {
		root[k] = "30px"
		if v, ok := b[k]; ok {
			root[k] = v
		}
		delete(b, k)
	}
	layouter := map[string]any{"layouter": "flow-layouter"}
	if c, ok := b["layouter_config"]; ok {
		layouter["config"] = c
	}
	elements, ok := b["elements"]
	if !ok {
		elements = []any{}
	}
	if a, ok := b["page_alignment"]; ok {
		if _, has := b["hori_align"]; !has {
			b["hori_align"] = a
		}
	}
	for _, k := range []string{"layouter", "layouter_config", "elements", "page_alignment"} {
		delete(b, k)
	}
	b["schema_version"] = "0.5"
	b["page_layouter"] = map[string]any{"layouter": "single-pane", "root": root}
	b["panels"] = map[string]any{"main": map[string]any{"elem_layouter": layouter, "elem_list": elements}}
	return marshal(t, b)
}

// demoRows are the classes, pages and apps the migrations leave active: the
// seeded classes, the newest seeded version of each page converted to format
// 0.5, and the rows ownPanesMigration adds. Pages get their class copies when
// fixtureLoader serves them, as the migration's pg_temp.wpi_page does.
func demoRows(t *testing.T) map[Kind]map[string]string {
	t.Helper()
	all := seedRows(t, seedMigration, pagesSeedMigration, elementClassesMigration, ownPanesMigration)
	toFormat04(t, all)
	pages := map[string]string{}
	for _, f := range []string{pagesSeedMigration} {
		for _, m := range pageSeedRow.FindAllStringSubmatch(readFile(t, f), -1) {
			pages[m[1]] = m[2]
		}
	}
	demo := regexp.MustCompile(`(?s)SELECT '([a-z0-9-]+)', COALESCE[^$]*\$wpi\$(.*?)\$wpi\$::jsonb`).FindAllStringSubmatch(readFile(t, customersDemoMigration), -1)
	for _, m := range demo {
		pages[m[1]] = m[2]
	}
	all[KindPage] = map[string]string{}
	for id, body := range pages {
		all[KindPage][id] = page05(t, body)
	}
	all[KindApp] = map[string]string{}
	for _, m := range newRow.FindAllStringSubmatch(readFile(t, ownPanesMigration), -1) {
		body := decode(t, m[2])
		if _, isApp := body["initial_page"]; isApp {
			all[KindApp][m[1]] = m[2]
		} else {
			all[KindPage][m[1]] = m[2]
		}
	}
	return all
}

func TestSeedLayouterClasses(t *testing.T) {
	rows := seedRows(t, ownPanesMigration)
	if len(rows[KindPageLayouter]) != 2 || len(rows[KindElementLayouter]) != 1 {
		t.Fatalf("layouter classes %v", rows)
	}
	for _, kind := range []Kind{KindPageLayouter, KindElementLayouter} {
		for id, body := range rows[kind] {
			expectIssue(t, ValidateDescriptor(kind, id, decode(t, body)), "")
		}
	}
}

// The converted demo apps are valid: every class, every page with its class
// copies, the panes their open-page instances name, the demo table's columns,
// the stored statements and files.
func TestSeedDemoAppsAreValid(t *testing.T) {
	all := demoRows(t)
	for _, kind := range []Kind{KindElement, KindAction} {
		for id, body := range all[kind] {
			expectIssue(t, ValidateDescriptor(kind, id, decode(t, body)), "")
		}
	}
	registered := seededModules(t)
	demo := readFile(t, pagesSeedMigration)
	for _, m := range stmtSeedRow.FindAllStringSubmatch(demo, -1) {
		registered[storedStatement] = append(registered[storedStatement], m[1])
	}
	for _, m := range fileSeedRow.FindAllStringSubmatch(demo, -1) {
		registered[storedFile] = append(registered[storedFile], m[1])
	}
	demoColumns := func(_ context.Context, table string) (map[string]bool, error) {
		if table != "kb.wpi_demo_customers" {
			return nil, nil
		}
		return map[string]bool{"id": true, "user_name": true, "phone_number": true, "email": true, "status": true}, nil
	}
	for app, want := range map[string][]string{
		"customer-admin": {"customer-admin-about", "customer-admin-home", "customer-directory", "customer-form", "customers"},
		"customers-demo": {"demo-customers"},
	} {
		t.Run(app, func(t *testing.T) {
			res, err := BuildBundle(context.Background(), app, fixtureLoader(t, all), demoColumns, registeredIn(registered))
			if err != nil {
				t.Fatal(err)
			}
			if len(res.Errors) != 0 || len(res.Warnings) != 0 {
				t.Fatalf("errors %+v\nwarnings %+v", res.Errors, res.Warnings)
			}
			var got []string
			for id := range res.Bundle.Pages {
				got = append(got, id)
			}
			slices.Sort(got)
			if !slices.Equal(got, want) {
				t.Fatalf("app reaches pages %v; want %v", got, want)
			}
		})
	}
}
