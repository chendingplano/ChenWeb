// Command qudt-import imports the published QUDT catalog (units, quantity
// kinds, dimension vectors) into the `quantity` core module as governed,
// approved content, per DR13 (adopt selectively: compile the published TTL
// into our module format via the compiler). The TTL files are transient
// generator input; the database is the system of record (2026-07-31 storage
// decision).
//
// Usage:
//
//	qudt-import --units <units.ttl> --quantity-kinds <qk.ttl> --dimensions <dim.ttl> [--release]
//
// Re-running is safe: existing term_ids are skipped. With --release the module
// is released and activated at the end.
package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"strings"

	_ "github.com/lib/pq"

	"github.com/chendingplano/deepdoc/server/api/ontology/modules"
	"github.com/chendingplano/deepdoc/server/api/ontology/terminology"
	"github.com/chendingplano/deepdoc/server/api/ontology/terms"
)

const (
	unitClass = "http://qudt.org/schema/qudt/Unit"
	qkClass   = "http://qudt.org/schema/qudt/QuantityKind"
	dimClass  = "http://qudt.org/schema/qudt/DimensionVector"
)

// importedTerm is one governed term produced by the import.
type importedTerm struct {
	TermID    string
	Kind      string
	SourceIRI string
	PrefLabel string
	Symbol    string
}

func main() {
	log.SetFlags(0)
	unitsPath := flag.String("units", "", "QUDT units TTL")
	qkPath := flag.String("quantity-kinds", "", "QUDT quantity-kinds TTL")
	dimPath := flag.String("dimensions", "", "QUDT dimension-vectors TTL")
	doRelease := flag.Bool("release", false, "release and activate quantity after import")
	flag.Parse()

	if *unitsPath == "" || *qkPath == "" || *dimPath == "" {
		log.Fatalf("--units, --quantity-kinds, and --dimensions are all required")
	}

	db := connect()
	defer db.Close()
	ctx := context.Background()

	terms_ := []importedTerm{}
	terms_ = append(terms_, parseTerms(*unitsPath, unitClass, "unit_")...)
	terms_ = append(terms_, parseTerms(*qkPath, qkClass, "qk_")...)
	terms_ = append(terms_, parseTerms(*dimPath, dimClass, "dim_")...)
	fmt.Printf("parsed %d terms from QUDT TTL\n", len(terms_))

	importTerms(ctx, db, terms_)

	if *doRelease {
		rs := modules.ReleaseStore{DB: db}
		if _, err := rs.GetRelease(ctx, "quantity", "1.0.0"); err == nil {
			fmt.Printf("release quantity@1.0.0 already exists; skipping\n")
			return
		}
		rel, err := rs.CreateRelease(ctx, "quantity", "1.0.0", "qudt-import")
		if err != nil {
			log.Fatalf("release quantity: %v", err)
		}
		if _, err := rs.Activate(ctx, "quantity", rel.ID, "qudt-import"); err != nil {
			log.Fatalf("activate quantity: %v", err)
		}
		fmt.Printf("released + activated quantity@1.0.0 (release id=%d, checksum=%s)\n", rel.ID, short(rel.ContentChecksum))
	}
	fmt.Println("QUDT import complete")
}

func parseTerms(path, class, idPrefix string) []importedTerm {
	f, err := os.Open(path)
	if err != nil {
		log.Fatalf("open %s: %v", path, err)
	}
	defer f.Close()
	content, err := io.ReadAll(f)
	if err != nil {
		log.Fatalf("read %s: %v", path, err)
	}
	resources, err := terminology.ParseQUDTGraph(content)
	if err != nil {
		log.Fatalf("parse %s: %v", path, err)
	}
	out := make([]importedTerm, 0, len(resources))
	for _, resource := range resources {
		if resource.Class != class {
			continue
		}
		if resource.Deprecated {
			continue // skip deprecated QUDT entries
		}
		label := pickLabel(resource.Labels)
		out = append(out, importedTerm{
			TermID:    "quantity:" + idPrefix + localName(resource.IRI),
			Kind:      termKindFor(idPrefix),
			SourceIRI: resource.IRI,
			PrefLabel: label,
			Symbol:    resource.Symbol,
		})
	}
	return out
}

func termKindFor(prefix string) string {
	switch prefix {
	case "unit_":
		return "unit"
	case "qk_":
		return "quantity_kind"
	default:
		return "dimension"
	}
}

// pickLabel selects the en preferred label for the legacy quantity module
// prefLabel, falling back to any preferred label and then the first label.
func pickLabel(labels []terminology.QUDTLabel) string {
	for _, l := range labels {
		if l.Role == "preferred" && l.Language == "en" {
			return l.Value
		}
	}
	for _, l := range labels {
		if l.Role == "preferred" {
			return l.Value
		}
	}
	if len(labels) > 0 {
		return labels[0].Value
	}
	return ""
}

func localName(iri string) string {
	if i := strings.LastIndex(iri, "/"); i >= 0 {
		return iri[i+1:]
	}
	return iri
}

// importTerms batch-inserts terms, their prefLabels, and exact mappings back
// to the source IRIs, skipping term_ids that already exist.
func importTerms(ctx context.Context, db *sql.DB, list []importedTerm) {
	ms := modules.ModuleStore{DB: db}
	if _, err := ms.GetModule(ctx, "quantity"); err != nil {
		if _, err := ms.CreateModule(ctx, modules.Module{
			ModuleID: "quantity", Title: "Quantity kinds, units, and dimensions (QUDT)",
			Owner: "platform", DependsOn: []string{"core"}, Status: "active",
			CreateBy: "qudt-import", ModifyBy: "qudt-import",
		}); err != nil {
			log.Fatalf("register quantity module: %v", err)
		}
		fmt.Printf("quantity module registered\n")
	} else {
		fmt.Printf("quantity module already registered\n")
	}

	existing := existingTermIDs(ctx, db)
	ts := terms.TermStore{DB: db}
	ls := terms.LabelStore{DB: db}
	ms2 := terms.MappingStore{DB: db}

	imported := 0
	for _, it := range list {
		if existing[it.TermID] {
			continue
		}
		if _, err := ts.CreateTerm(ctx, terms.Term{
			TermID: it.TermID, TermKind: it.Kind, ModuleID: "quantity",
			Definition: "QUDT import; source " + it.SourceIRI, Status: "approved",
			CreateBy: "qudt-import", ModifyBy: "qudt-import",
		}); err != nil {
			log.Fatalf("import term %s: %v", it.TermID, err)
		}
		if it.PrefLabel != "" {
			if _, err := ls.CreateLabel(ctx, terms.TermLabel{
				TermID: it.TermID, Label: it.PrefLabel, Lang: "en", LabelRole: "prefLabel",
				Status: "approved", CreateBy: "qudt-import", ModifyBy: "qudt-import",
			}); err != nil {
				log.Fatalf("import label for %s: %v", it.TermID, err)
			}
		}
		if it.Symbol != "" {
			if _, err := ls.CreateLabel(ctx, terms.TermLabel{
				TermID: it.TermID, Label: it.Symbol, Lang: "en", LabelRole: "altLabel",
				Status: "approved", CreateBy: "qudt-import", ModifyBy: "qudt-import",
			}); err != nil {
				log.Fatalf("import symbol for %s: %v", it.TermID, err)
			}
		}
		if _, err := ms2.CreateMapping(ctx, terms.Mapping{
			MappingID: "quantity:map_" + strings.TrimPrefix(it.TermID, "quantity:"), FromTermID: it.TermID,
			ToIRI: it.SourceIRI, Relation: "exact", ApprovalStatus: "approved",
			ModuleID: "quantity", Status: "approved",
			CreateBy: "qudt-import", ModifyBy: "qudt-import",
		}); err != nil {
			log.Fatalf("import mapping for %s: %v", it.TermID, err)
		}
		existing[it.TermID] = true
		imported++
		if imported%500 == 0 {
			fmt.Printf("  imported %d terms so far\n", imported)
		}
	}
	fmt.Printf("imported %d new terms (%d already present)\n", imported, len(list)-imported)
}

func existingTermIDs(ctx context.Context, db *sql.DB) map[string]bool {
	rows, err := db.QueryContext(ctx, "SELECT term_id FROM kb.ontology_terms WHERE module_id = 'quantity'")
	if err != nil {
		return map[string]bool{}
	}
	defer rows.Close()
	out := make(map[string]bool)
	for rows.Next() {
		var id string
		if rows.Scan(&id) == nil {
			out[id] = true
		}
	}
	return out
}

func connect() *sql.DB {
	dsn := fmt.Sprintf("host=%s port=%s user=%s dbname=%s sslmode=disable",
		envOr("PG_HOST", "/tmp"), envOr("PG_PORT", "5432"), envOr("PG_USER", "cding"),
		envOr("PG_DB_NAME", "chenweb_test"))
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	if err := db.Ping(); err != nil {
		log.Fatalf("ping db: %v", err)
	}
	return db
}

func envOr(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

func short(s string) string {
	if len(s) > 12 {
		return s[:12]
	}
	return s
}
