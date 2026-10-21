package categorytree

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/chendingplano/shared/go/api/loggerutil"
	"github.com/labstack/echo/v4"
)

func TestHandler_TreeLifecycle(t *testing.T) {
	db := testDB(t)
	name := fmt.Sprintf("handler-tree-%d", time.Now().UnixNano())
	t.Cleanup(func() {
		q := `(SELECT id FROM kb.category_trees WHERE canonical_name = '` + name + `')`
		db.Exec(`DELETE FROM kb.category_tree_revisions WHERE tree_id IN ` + q)
		db.Exec(`DELETE FROM kb.category_tree_changes WHERE tree_id IN ` + q)
		db.Exec(`UPDATE kb.category_nodes SET parent_id = NULL WHERE tree_id IN ` + q)
		db.Exec(`DELETE FROM kb.category_nodes WHERE tree_id IN ` + q)
		db.Exec(`DELETE FROM kb.category_trees WHERE canonical_name = $1`, name)
	})
	h := &Handler{}
	h.once.Do(func() { h.svc = NewService(&Store{DB: db}, loggerutil.CreateDefaultLogger("20261021-662")) })
	e := echo.New()
	h.Register(e.Group(""))

	call := func(method, path, body string) (int, map[string]any) {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		var out map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &out)
		return rec.Code, out
	}
	node := func(n string, children string) string {
		return fmt.Sprintf(`{"canonical_name":%q,"pref_labels":{"en":"%s","zh":"中%s"},"definitions":{"en":"d","zh":"定义"},"children":[%s]}`, n, n, n, children)
	}

	code, out := call(http.MethodPost, "/kb/category-trees", fmt.Sprintf(
		`{"canonical_name":%q,"namespace":"https://example.org/cat/","pref_labels":{"en":"T","zh":"树"},"definitions":{"en":"t","zh":"树"},"root":%s}`,
		name, node("root", node("a", "")+","+node("b", ""))))
	if code != http.StatusCreated {
		t.Fatalf("create: %d %v", code, out)
	}
	if code, out = call(http.MethodPost, "/kb/category-trees/"+name+"/revisions", `{"name":"v1"}`); code != http.StatusCreated {
		t.Fatalf("revision: %d %v", code, out)
	}
	code, out = call(http.MethodPost, "/kb/category-trees/"+name+"/change-sets",
		`{"operations":[{"op":"add","parent":"a","fields":{"canonical_name":"a1","pref_labels":{"en":"A1"},"definitions":{"en":"x","zh":"x"}}}]}`)
	if code != http.StatusUnprocessableEntity || out["problems"] == nil {
		t.Errorf("invalid change set: %d %v", code, out)
	}
	code, out = call(http.MethodPost, "/kb/category-trees/"+name+"/change-sets",
		`{"operations":[{"op":"move","node":"root","parent":"a"}]}`)
	if code != http.StatusBadRequest {
		t.Errorf("moving the root: %d %v", code, out)
	}
	if code, out = call(http.MethodPost, "/kb/category-trees/"+name+"/change-sets",
		`{"operations":[{"op":"add","parent":"a","fields":{"canonical_name":"a1","pref_labels":{"en":"A1","zh":"甲一"},"definitions":{"en":"x","zh":"x"}}}]}`); code != http.StatusOK {
		t.Fatalf("change set: %d %v", code, out)
	}
	_, cur := call(http.MethodGet, "/kb/category-trees/"+name, "")
	_, old := call(http.MethodGet, "/kb/category-trees/"+name+"?revision=v1", "")
	if n := len(cur["nodes"].([]any)); n != 6 { // root, a, a1, a-others, b, root-others
		t.Errorf("current nodes = %d", n)
	}
	if n := len(old["nodes"].([]any)); n != 4 || old["valid"] != true {
		t.Errorf("v1 nodes = %d valid=%v", n, old["valid"])
	}
	if code, _ := call(http.MethodGet, "/kb/category-trees/"+name+"?revision=nope", ""); code != http.StatusNotFound {
		t.Errorf("missing revision: %d", code)
	}
	if code, out := call(http.MethodGet, "/kb/category-trees/"+name+"/changes?after_seq=4", ""); code != http.StatusOK || len(out["changes"].([]any)) != 2 {
		t.Errorf("changes: %d %v", code, out)
	}
}
