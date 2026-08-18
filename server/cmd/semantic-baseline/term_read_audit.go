package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// TermReaderAudit is a source-level inventory used before replacing current
// term reads with kb.ontology_terms_current. It deliberately reports every
// application source file containing a base-table reference so unusual SQL is
// visible for manual migration rather than silently ignored.
type TermReaderAudit struct {
	References   []TermReaderReference `json:"references"`
	CurrentState int                   `json:"current_state"`
	Historical   int                   `json:"historical"`
	WritePath    int                   `json:"write_path"`
}

type TermReaderReference struct {
	Path            string `json:"path"`
	Classification  string `json:"classification"`
	MigrationTarget string `json:"migration_target"`
}

// AuditTermReaders scans Go application files under root. Test files and
// generated/vendor trees are excluded because the migration gate concerns
// production readers, not fixture SQL. A file is classified historical when
// its base-table access mentions revision/history; all remaining SELECT-like
// access is current-state and therefore must migrate to the compatibility
// view before base-table reshaping.
func AuditTermReaders(root string) (TermReaderAudit, error) {
	var audit TermReaderAudit
	root = strings.TrimSpace(root)
	if root == "" {
		return audit, fmt.Errorf("source root is required")
	}
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", "vendor", "node_modules":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			return err
		}
		ast.Inspect(file, func(node ast.Node) bool {
			literal, ok := node.(*ast.BasicLit)
			if !ok || literal.Kind != token.STRING {
				return true
			}
			value, err := strconv.Unquote(literal.Value)
			if err != nil {
				return true
			}
			classification, ok := classifyTermSQL(value)
			if !ok {
				return true
			}
			audit.References = append(audit.References, TermReaderReference{
				Path:            path,
				Classification:  classification,
				MigrationTarget: termReaderMigrationTarget(classification),
			})
			switch classification {
			case "write_path":
				audit.WritePath++
			case "historical":
				audit.Historical++
			default:
				audit.CurrentState++
			}
			return true
		})
		return nil
	})
	return audit, err
}

func classifyTermSQL(raw string) (string, bool) {
	stmt := strings.TrimSpace(strings.ToLower(raw))
	if !strings.Contains(stmt, "kb.ontology_terms") {
		return "", false
	}
	if !strings.HasPrefix(stmt, "select") && !strings.HasPrefix(stmt, "with") &&
		!strings.HasPrefix(stmt, "insert") && !strings.HasPrefix(stmt, "update") &&
		!strings.HasPrefix(stmt, "delete") {
		return "", false
	}
	if strings.HasPrefix(stmt, "insert") || strings.HasPrefix(stmt, "update") || strings.HasPrefix(stmt, "delete") {
		return "write_path", true
	}
	if strings.Contains(stmt, "revision") || strings.Contains(stmt, "history") {
		return "historical", true
	}
	return "current_state", true
}

func termReaderMigrationTarget(classification string) string {
	switch classification {
	case "historical":
		return "append-only term revisions"
	case "write_path":
		return "stable term header and revision stores"
	default:
		return "kb.ontology_terms_current"
	}
}
