package docbuilder

import (
	"context"
	"errors"
	"net/http"
	"os"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/chendingplano/deepdoc/server/api/pageconfighandler"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/EchoFactory"
	"github.com/labstack/echo/v4"
)

// API serves document groups. Reads need a login and a role of the group; administration needs
// the admin role (the same check as page-config).
type API struct {
	Store         *Store
	ValidRoles    func() []string                                // configured role keys
	NewClassifier func() (c Classifier, model string, err error) // built lazily, only when classifying
}

type menuItem struct {
	Key    string `json:"group_key"`
	Label  string `json:"label"`
	Parent string `json:"parent_menu_id"`
	Order  int    `json:"sort_order"`
}

func pickLabel(l map[string]string, lang, key string) string {
	for _, k := range []string{lang, "en"} {
		if v := strings.TrimSpace(l[k]); v != "" {
			return v
		}
	}
	return key
}

func lang(c echo.Context) string {
	if strings.HasPrefix(strings.ToLower(c.QueryParam("lang")), "zh") {
		return "zh-cn"
	}
	return "en"
}

func isAdmin(u *ApiTypes.UserInfo) bool {
	return u.Admin || u.IsOwner || slices.Contains(u.Roles, "admin")
}

func (a *API) user(c echo.Context) (*ApiTypes.UserInfo, func(), bool) {
	rc := EchoFactory.NewFromEcho(c, "CWB_DGR_001")
	u := rc.IsAuthenticated()
	if u == nil {
		rc.Close()
		_ = c.JSON(http.StatusUnauthorized, map[string]string{"error": "Authentication required"})
		return nil, nil, false
	}
	return u, rc.Close, true
}

func (a *API) admin(c echo.Context) (*ApiTypes.UserInfo, func(), bool) {
	u, closeFn, ok := a.user(c)
	if !ok {
		return nil, nil, false
	}
	if !isAdmin(u) {
		closeFn()
		_ = c.JSON(http.StatusForbidden, map[string]string{"error": "Admin access required"})
		return nil, nil, false
	}
	return u, closeFn, true
}

func (a *API) fail(c echo.Context, err error) error {
	switch {
	case errors.Is(err, ErrNotFound):
		return c.JSON(http.StatusNotFound, map[string]string{"error": "not found"})
	case errors.Is(err, ErrBadPath), errors.Is(err, ErrUnsupported):
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}
	a.Store.logf("docbuilder: %v", err)
	return c.JSON(http.StatusInternalServerError, map[string]string{"error": "request failed"})
}

// visibleGroup loads a group the caller may read: enabled and holding one of its roles (admins
// see every group). Anything else is reported as not found.
func (a *API) visibleGroup(c echo.Context, u *ApiTypes.UserInfo) (*Group, error) {
	g, err := a.Store.GroupByKey(c.Request().Context(), c.Param("key"))
	if err != nil {
		return nil, err
	}
	if isAdmin(u) {
		return g, nil
	}
	if !g.Enabled || !pageconfighandler.RoleAllowed(g.Roles, u.Roles, a.ValidRoles()) {
		return nil, ErrNotFound
	}
	return g, nil
}

// Register adds the routes to an authenticated group.
func (a *API) Register(g *echo.Group) {
	g.GET("/doc-groups/menu", a.menu)
	g.GET("/doc-groups/:key/tree", a.tree)
	g.GET("/doc-groups/:key/documents/:docId/file", a.file)
	g.GET("/doc-groups/:key/documents/:docId/pdf", a.pdf)

	g.GET("/doc-groups/admin/groups", a.adminList)
	g.POST("/doc-groups/admin/groups", a.adminSave)
	g.PUT("/doc-groups/admin/groups/:key", a.adminSave)
	g.DELETE("/doc-groups/admin/groups/:key", a.adminDelete)
	g.POST("/doc-groups/admin/groups/:key/members", a.adminAddMember)
	g.DELETE("/doc-groups/admin/groups/:key/members/:memberId", a.adminRemoveMember)
	g.GET("/doc-groups/admin/groups/:key/documents", a.adminDocs)
	g.POST("/doc-groups/admin/groups/:key/documents/:docId/remove", a.adminRemoveDoc)
	g.POST("/doc-groups/admin/groups/:key/rescan", a.adminRescan)
	g.POST("/doc-groups/admin/groups/:key/classify", a.adminClassify)
}

// menuFor lists the enabled groups a user with these roles may see, ordered by parent menu, then
// position number, then key. The label is resolved for lang, falling back to English, then the key.
func (a *API) menuFor(ctx context.Context, roles []string, lang string) ([]menuItem, error) {
	groups, err := a.Store.ListGroups(ctx)
	if err != nil {
		return nil, err
	}
	out := []menuItem{}
	for _, g := range groups {
		if !g.Enabled || !pageconfighandler.RoleAllowed(g.Roles, roles, a.ValidRoles()) {
			continue
		}
		out = append(out, menuItem{g.Key, pickLabel(g.Label, lang, g.Key), g.Parent, g.Order})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Parent != out[j].Parent {
			return out[i].Parent < out[j].Parent
		}
		if out[i].Order != out[j].Order {
			return out[i].Order < out[j].Order
		}
		return out[i].Key < out[j].Key
	})
	return out, nil
}

func (a *API) menu(c echo.Context) error {
	u, done, ok := a.user(c)
	if !ok {
		return nil
	}
	defer done()
	out, err := a.menuFor(c.Request().Context(), u.Roles, lang(c))
	if err != nil {
		return a.fail(c, err)
	}
	return c.JSON(http.StatusOK, out)
}

func (a *API) tree(c echo.Context) error {
	u, done, ok := a.user(c)
	if !ok {
		return nil
	}
	defer done()
	g, err := a.visibleGroup(c, u)
	if err != nil {
		return a.fail(c, err)
	}
	ctx := c.Request().Context()
	if err := a.Store.SyncGroup(ctx, g.ID); err != nil {
		return a.fail(c, err)
	}
	entries, err := a.Store.GroupDocs(ctx, g.ID)
	if err != nil {
		return a.fail(c, err)
	}
	pending := 0
	for _, e := range entries {
		if e.Category == "" {
			pending++
		}
	}
	return c.JSON(http.StatusOK, map[string]any{"title": pickLabel(g.Label, lang(c), g.Key), "tree": BuildTree(entries), "uncategorized": pending})
}

func docID(c echo.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("docId"), 10, 64)
	if err != nil || id <= 0 {
		_ = c.JSON(http.StatusNotFound, map[string]string{"error": "not found"})
		return 0, false
	}
	return id, true
}

func (a *API) file(c echo.Context) error {
	u, done, ok := a.user(c)
	if !ok {
		return nil
	}
	defer done()
	id, ok := docID(c)
	if !ok {
		return nil
	}
	g, err := a.visibleGroup(c, u)
	if err != nil {
		return a.fail(c, err)
	}
	rel, kind, err := a.Store.CurrentFile(c.Request().Context(), g.ID, id)
	if err != nil {
		return a.fail(c, err)
	}
	raw, err := os.ReadFile(a.Store.abs(rel))
	if err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "not found"})
	}
	ct := "text/markdown; charset=utf-8"
	if kind == KindPDF {
		ct = "application/pdf"
	}
	c.Response().Header().Set("Content-Disposition", "inline")
	return c.Blob(http.StatusOK, ct, raw)
}

func (a *API) pdf(c echo.Context) error {
	u, done, ok := a.user(c)
	if !ok {
		return nil
	}
	defer done()
	id, ok := docID(c)
	if !ok {
		return nil
	}
	g, err := a.visibleGroup(c, u)
	if err != nil {
		return a.fail(c, err)
	}
	p, err := a.Store.ExportPDF(c.Request().Context(), g.ID, id)
	if err != nil {
		return a.fail(c, err)
	}
	c.Response().Header().Set("Content-Disposition", "inline")
	return c.File(p)
}

// ---- administration ----

type adminGroup struct {
	Group
	Members       []Member `json:"members"`
	Documents     int      `json:"documents"`
	Uncategorized int      `json:"uncategorized"`
}

func (a *API) adminList(c echo.Context) error {
	_, done, ok := a.admin(c)
	if !ok {
		return nil
	}
	defer done()
	ctx := c.Request().Context()
	groups, err := a.Store.ListGroups(ctx)
	if err != nil {
		return a.fail(c, err)
	}
	out := []adminGroup{}
	for _, g := range groups {
		members, err := a.Store.Members(ctx, g.ID)
		if err != nil {
			return a.fail(c, err)
		}
		entries, err := a.Store.GroupDocs(ctx, g.ID)
		if err != nil {
			return a.fail(c, err)
		}
		for i := range members {
			members[i].Exists = a.Store.memberReachable(members[i])
			if members[i].Type == "file" {
				_, err := os.Stat(a.Store.abs(members[i].Path))
				members[i].Exists = err == nil
			}
		}
		ag := adminGroup{Group: g, Members: members, Documents: len(entries)}
		if ag.Members == nil {
			ag.Members = []Member{}
		}
		for _, e := range entries {
			if e.Category == "" {
				ag.Uncategorized++
			}
		}
		out = append(out, ag)
	}
	return c.JSON(http.StatusOK, out)
}

func (a *API) adminSave(c echo.Context) error {
	u, done, ok := a.admin(c)
	if !ok {
		return nil
	}
	defer done()
	var g Group
	if err := c.Bind(&g); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid body"})
	}
	if key := c.Param("key"); key != "" {
		cur, err := a.Store.GroupByKey(c.Request().Context(), key)
		if err != nil {
			return a.fail(c, err)
		}
		g.ID = cur.ID
	} else {
		g.ID = 0
	}
	valid := a.ValidRoles()
	roles := []string{}
	for _, r := range g.Roles {
		r = strings.ToLower(strings.TrimSpace(r))
		if r != "" && slices.Contains(valid, r) && !slices.Contains(roles, r) {
			roles = append(roles, r)
		}
	}
	g.Roles = roles
	if err := a.Store.SaveGroup(c.Request().Context(), &g, u.Email); err != nil {
		if strings.Contains(err.Error(), "group_key") {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
		}
		if strings.Contains(err.Error(), "duplicate key") {
			return c.JSON(http.StatusConflict, map[string]string{"error": "a group with this key exists"})
		}
		return a.fail(c, err)
	}
	return c.JSON(http.StatusOK, g)
}

func (a *API) groupByParam(c echo.Context) (*Group, bool) {
	g, err := a.Store.GroupByKey(c.Request().Context(), c.Param("key"))
	if err != nil {
		_ = a.fail(c, err)
		return nil, false
	}
	return g, true
}

func (a *API) adminDelete(c echo.Context) error {
	_, done, ok := a.admin(c)
	if !ok {
		return nil
	}
	defer done()
	g, ok := a.groupByParam(c)
	if !ok {
		return nil
	}
	if err := a.Store.DeleteGroup(c.Request().Context(), g.ID); err != nil {
		return a.fail(c, err)
	}
	return c.NoContent(http.StatusNoContent)
}

func (a *API) adminAddMember(c echo.Context) error {
	_, done, ok := a.admin(c)
	if !ok {
		return nil
	}
	defer done()
	g, ok := a.groupByParam(c)
	if !ok {
		return nil
	}
	var body struct {
		Type string `json:"member_type"`
		Path string `json:"path"`
	}
	if err := c.Bind(&body); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid body"})
	}
	m, err := a.Store.AddMember(c.Request().Context(), g.ID, body.Type, body.Path)
	if err != nil {
		return a.fail(c, err)
	}
	return c.JSON(http.StatusOK, m)
}

func (a *API) adminRemoveMember(c echo.Context) error {
	_, done, ok := a.admin(c)
	if !ok {
		return nil
	}
	defer done()
	g, ok := a.groupByParam(c)
	if !ok {
		return nil
	}
	id, err := strconv.ParseInt(c.Param("memberId"), 10, 64)
	if err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "not found"})
	}
	if err := a.Store.RemoveMember(c.Request().Context(), g.ID, id); err != nil {
		return a.fail(c, err)
	}
	return c.NoContent(http.StatusNoContent)
}

func (a *API) adminDocs(c echo.Context) error {
	_, done, ok := a.admin(c)
	if !ok {
		return nil
	}
	defer done()
	g, ok := a.groupByParam(c)
	if !ok {
		return nil
	}
	ctx := c.Request().Context()
	if err := a.Store.SyncGroup(ctx, g.ID); err != nil {
		return a.fail(c, err)
	}
	entries, err := a.Store.GroupDocs(ctx, g.ID)
	if err != nil {
		return a.fail(c, err)
	}
	if entries == nil {
		entries = []Entry{}
	}
	return c.JSON(http.StatusOK, entries)
}

func (a *API) adminRemoveDoc(c echo.Context) error {
	_, done, ok := a.admin(c)
	if !ok {
		return nil
	}
	defer done()
	id, ok := docID(c)
	if !ok {
		return nil
	}
	g, ok := a.groupByParam(c)
	if !ok {
		return nil
	}
	var body struct {
		Delete bool `json:"delete"`
	}
	_ = c.Bind(&body)
	blocked, err := a.Store.RemoveDocument(c.Request().Context(), g.ID, id, body.Delete)
	if errors.Is(err, ErrInUse) {
		return c.JSON(http.StatusConflict, map[string]any{"error": "the document belongs to other groups, so it cannot be deleted", "groups": blocked})
	}
	if err != nil {
		return a.fail(c, err)
	}
	return c.NoContent(http.StatusNoContent)
}

func (a *API) adminRescan(c echo.Context) error {
	_, done, ok := a.admin(c)
	if !ok {
		return nil
	}
	defer done()
	g, ok := a.groupByParam(c)
	if !ok {
		return nil
	}
	if err := a.Store.SyncGroup(c.Request().Context(), g.ID); err != nil {
		return a.fail(c, err)
	}
	return c.NoContent(http.StatusNoContent)
}

func (a *API) adminClassify(c echo.Context) error {
	_, done, ok := a.admin(c)
	if !ok {
		return nil
	}
	defer done()
	g, ok := a.groupByParam(c)
	if !ok {
		return nil
	}
	var body struct {
		Force bool `json:"force"`
	}
	_ = c.Bind(&body)
	cl, model, err := a.NewClassifier()
	if err != nil {
		a.Store.logf("docbuilder: classifier: %v", err)
		return c.JSON(http.StatusServiceUnavailable, map[string]string{"error": "classification model is not available"})
	}
	n, err := a.Store.ClassifyGroup(c.Request().Context(), g, cl, model, body.Force)
	if err != nil {
		return a.fail(c, err)
	}
	return c.JSON(http.StatusOK, map[string]int{"classified": n})
}
