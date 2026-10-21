package categorytree

import (
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/EchoFactory"
	"github.com/chendingplano/shared/go/api/loggerutil"
	"github.com/labstack/echo/v4"
)

// Handler serves the category-tree API (design D9). The store and service are created on
// first use, after the project database is open.
type Handler struct {
	once sync.Once
	svc  *Service
}

// NewHandler returns a handler over ApiTypes.ProjectDBHandle.
func NewHandler() *Handler { return &Handler{} }

func (h *Handler) service() *Service {
	h.once.Do(func() {
		logger := loggerutil.CreateDefaultLogger("20261021-583")
		h.svc = NewService(&Store{DB: ApiTypes.ProjectDBHandle}, logger)
	})
	return h.svc
}

// Register adds the routes to an authenticated group.
func (h *Handler) Register(g *echo.Group) {
	g.GET("/kb/category-trees", h.ListTrees)
	g.POST("/kb/category-trees", h.CreateTree)
	g.GET("/kb/category-trees/:name", h.GetTree)
	g.POST("/kb/category-trees/:name/change-sets", h.ApplyChangeSet)
	g.GET("/kb/category-trees/:name/changes", h.ListChanges)
	g.GET("/kb/category-trees/:name/revisions", h.ListRevisions)
	g.POST("/kb/category-trees/:name/revisions", h.CreateRevision)
	g.GET("/kb/category-trees/:name/nodes/:node_id/members", h.Members)
	g.GET("/kb/category-trees/:name/others-report", h.OthersReport)
	g.GET("/kb/category-tree-types", h.ListTreeTypes)
	g.PUT("/kb/category-tree-types/:artifact_type", h.SetTreeType)
	g.POST("/kb/category-classifications/run", h.RunRecord)
	g.POST("/kb/category-classifications/reclassify-stale", h.ReclassifyStale)
	g.GET("/kb/category-classifications", h.ListClassifications)
	g.GET("/kb/category-classifications/review", h.ReviewQueue)
	g.POST("/kb/category-classifications/:id/review", h.Review)
	g.GET("/kb/category-bindings", h.ArtifactBindings)
}

// fail maps an error to a status and logs it.
func fail(c echo.Context, rc ApiTypes.RequestContext, msg string, err error) error {
	status := http.StatusInternalServerError
	body := map[string]any{"message": msg, "error": err.Error()}
	var ve *ValidationError
	var ie *InputError
	switch {
	case errors.As(err, &ve):
		status = http.StatusUnprocessableEntity
		body["problems"] = ve.Problems
	case errors.Is(err, ErrNotFound):
		status = http.StatusNotFound
	case errors.Is(err, ErrConflict):
		status = http.StatusConflict
	case errors.As(err, &ie):
		status = http.StatusBadRequest
	}
	logger := rc.GetLogger()
	if status >= 500 {
		logger.Error(msg, "path", c.Path(), "user_id", rc.GetUserID(), "error", err)
	} else {
		logger.Warn(msg, "path", c.Path(), "user_id", rc.GetUserID(), "status", status, "error", err)
	}
	return c.JSON(status, body)
}

func badRequest(c echo.Context, rc ApiTypes.RequestContext, msg string) error {
	rc.GetLogger().Warn("category tree request rejected", "path", c.Path(), "reason", msg)
	return c.JSON(http.StatusBadRequest, map[string]any{"message": msg})
}

func (h *Handler) ListTrees(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_CAT_101")
	defer rc.Close()
	trees, err := h.service().Store.ListTrees(c.Request().Context())
	if err != nil {
		return fail(c, rc, "list category trees failed", err)
	}
	return c.JSON(http.StatusOK, map[string]any{"trees": trees})
}

func (h *Handler) CreateTree(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_CAT_102")
	defer rc.Close()
	var in CreateTreeInput
	if err := c.Bind(&in); err != nil {
		return badRequest(c, rc, "invalid tree: "+err.Error())
	}
	info, cs, err := h.service().Store.CreateTree(c.Request().Context(), in, rc.GetUserID())
	if err != nil {
		return fail(c, rc, "create category tree failed", err)
	}
	rc.GetLogger().Info("category tree created", "tree", info.CanonicalName, "tree_id", info.ID, "seq", cs.ToSeq, "user_id", rc.GetUserID())
	return c.JSON(http.StatusCreated, map[string]any{"tree": info, "change_set": cs})
}

// treeNodeView is a node in the tree listing, root first, with its depth and IRI.
type treeNodeView struct {
	*Node
	Depth int    `json:"depth"`
	IRI   string `json:"iri"`
	Leaf  bool   `json:"is_leaf"`
}

// GetTree returns the current tree, or the tree at ?revision= or ?seq= by replay.
func (h *Handler) GetTree(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_CAT_103")
	defer rc.Close()
	ctx := c.Request().Context()
	store := h.service().Store
	info, err := store.GetTree(ctx, treeRef(c))
	if err != nil {
		return fail(c, rc, "category tree not found", err)
	}
	revision := strings.TrimSpace(c.QueryParam("revision"))
	seq := info.CurrentSeq
	if revision != "" {
		if seq, err = store.RevisionSeq(ctx, info.ID, revision); err != nil {
			return fail(c, rc, "category tree revision not found", err)
		}
	} else if s := strings.TrimSpace(c.QueryParam("seq")); s != "" {
		if seq, err = strconv.ParseInt(s, 10, 64); err != nil {
			return badRequest(c, rc, "seq must be an integer")
		}
	}
	var tree *Tree
	if seq == info.CurrentSeq {
		tree, err = store.LoadCurrent(ctx, info)
	} else {
		tree, err = store.TreeAt(ctx, info, seq)
	}
	if err != nil {
		return fail(c, rc, "load category tree failed", err)
	}
	body := map[string]any{"tree": info, "seq": seq, "revision": revision, "nodes": nodeViews(tree), "valid": true}
	if verr := Validate(tree); verr != nil {
		body["valid"] = false
		if ve, ok := verr.(*ValidationError); ok {
			body["problems"] = ve.Problems
		}
	}
	return c.JSON(http.StatusOK, body)
}

func nodeViews(t *Tree) []treeNodeView {
	root := t.Root()
	if root == nil {
		var out []treeNodeView
		for _, n := range t.Nodes {
			out = append(out, treeNodeView{Node: n, IRI: t.IRI(n.ID), Leaf: t.IsLeaf(n.ID)})
		}
		return out
	}
	var out []treeNodeView
	var walk func(n *Node, depth int)
	walk = func(n *Node, depth int) {
		out = append(out, treeNodeView{Node: n, Depth: depth, IRI: t.IRI(n.ID), Leaf: t.IsLeaf(n.ID)})
		for _, ch := range t.Children(n.ID) {
			walk(ch, depth+1)
		}
	}
	walk(root, 0)
	return out
}

func (h *Handler) ApplyChangeSet(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_CAT_104")
	defer rc.Close()
	var in struct {
		Operations []Op `json:"operations"`
	}
	if err := c.Bind(&in); err != nil {
		return badRequest(c, rc, "invalid change set: "+err.Error())
	}
	res, err := h.service().Store.ApplyChangeSet(c.Request().Context(), treeRef(c), in.Operations, rc.GetUserID())
	if err != nil {
		return fail(c, rc, "change set rejected", err)
	}
	rc.GetLogger().Info("category tree change set applied", "tree_id", res.TreeID, "change_set_id", res.ChangeSetID,
		"from_seq", res.FromSeq, "to_seq", res.ToSeq, "entries", res.Entries, "stale_bindings", res.StaleBindings, "user_id", rc.GetUserID())
	return c.JSON(http.StatusOK, map[string]any{"change_set": res})
}

// treeRef returns the :name path parameter. Echo does not decode path parameters, so a
// client that percent-encodes the colon in "cat:…" sends "cat%3A…", which must be unescaped.
func treeRef(c echo.Context) string {
	name, err := url.PathUnescape(c.Param("name"))
	if err != nil {
		return c.Param("name")
	}
	return name
}

func queryInt(c echo.Context, key string) int64 {
	n, _ := strconv.ParseInt(strings.TrimSpace(c.QueryParam(key)), 10, 64)
	return n
}

func (h *Handler) ListChanges(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_CAT_105")
	defer rc.Close()
	ctx := c.Request().Context()
	store := h.service().Store
	info, err := store.GetTree(ctx, treeRef(c))
	if err != nil {
		return fail(c, rc, "category tree not found", err)
	}
	limit := int(queryInt(c, "limit"))
	if limit <= 0 || limit > 2000 {
		limit = 500
	}
	changes, err := store.Changes(ctx, info.ID, queryInt(c, "after_seq"), 0, limit)
	if err != nil {
		return fail(c, rc, "list category tree changes failed", err)
	}
	return c.JSON(http.StatusOK, map[string]any{"changes": changes, "current_seq": info.CurrentSeq})
}

func (h *Handler) ListRevisions(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_CAT_106")
	defer rc.Close()
	ctx := c.Request().Context()
	store := h.service().Store
	info, err := store.GetTree(ctx, treeRef(c))
	if err != nil {
		return fail(c, rc, "category tree not found", err)
	}
	revs, err := store.Revisions(ctx, info.ID)
	if err != nil {
		return fail(c, rc, "list revisions failed", err)
	}
	return c.JSON(http.StatusOK, map[string]any{"revisions": revs, "current_seq": info.CurrentSeq})
}

func (h *Handler) CreateRevision(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_CAT_107")
	defer rc.Close()
	var in struct {
		Name string `json:"name"`
		Note string `json:"note"`
	}
	if err := c.Bind(&in); err != nil {
		return badRequest(c, rc, "invalid revision: "+err.Error())
	}
	ctx := c.Request().Context()
	store := h.service().Store
	info, err := store.GetTree(ctx, treeRef(c))
	if err != nil {
		return fail(c, rc, "category tree not found", err)
	}
	rev, err := store.CreateRevision(ctx, info, in.Name, in.Note, rc.GetUserID())
	if err != nil {
		return fail(c, rc, "create revision failed", err)
	}
	rc.GetLogger().Info("category tree revision created", "tree", info.CanonicalName, "revision", rev.Name, "seq", rev.Seq, "user_id", rc.GetUserID())
	return c.JSON(http.StatusCreated, map[string]any{"revision": rev})
}

func (h *Handler) Members(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_CAT_108")
	defer rc.Close()
	direct := c.QueryParam("direct") == "true"
	members, err := h.service().Store.Members(c.Request().Context(), c.Param("node_id"), direct, int(queryInt(c, "limit")))
	if err != nil {
		return fail(c, rc, "list category members failed", err)
	}
	return c.JSON(http.StatusOK, map[string]any{"members": members, "direct": direct})
}

func (h *Handler) OthersReport(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_CAT_109")
	defer rc.Close()
	ctx := c.Request().Context()
	store := h.service().Store
	info, err := store.GetTree(ctx, treeRef(c))
	if err != nil {
		return fail(c, rc, "category tree not found", err)
	}
	rows, err := store.OthersReport(ctx, info.ID)
	if err != nil {
		return fail(c, rc, "others report failed", err)
	}
	return c.JSON(http.StatusOK, map[string]any{"others": rows})
}

func (h *Handler) ListTreeTypes(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_CAT_110")
	defer rc.Close()
	types, err := h.service().Store.TreeTypes(c.Request().Context())
	if err != nil {
		return fail(c, rc, "list category tree types failed", err)
	}
	return c.JSON(http.StatusOK, map[string]any{"tree_types": types})
}

func (h *Handler) SetTreeType(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_CAT_111")
	defer rc.Close()
	var in struct {
		Tree string `json:"tree"`
	}
	if err := c.Bind(&in); err != nil {
		return badRequest(c, rc, "invalid binding: "+err.Error())
	}
	ctx := c.Request().Context()
	store := h.service().Store
	info, err := store.GetTree(ctx, in.Tree)
	if err != nil {
		return fail(c, rc, "category tree not found", err)
	}
	artifactType := c.Param("artifact_type")
	if err := store.SetTreeType(ctx, artifactType, info, rc.GetUserID()); err != nil {
		return fail(c, rc, "bind artifact type failed", err)
	}
	rc.GetLogger().Info("artifact type bound to category tree", "artifact_type", artifactType, "tree", info.CanonicalName, "user_id", rc.GetUserID())
	return c.JSON(http.StatusOK, map[string]any{"artifact_type": artifactType, "tree": info})
}

func (h *Handler) RunRecord(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_CAT_112")
	defer rc.Close()
	var in struct {
		ArtifactType string `json:"artifact_type"`
		RecordID     int64  `json:"record_id"`
	}
	if err := c.Bind(&in); err != nil || in.RecordID <= 0 || strings.TrimSpace(in.ArtifactType) == "" {
		return badRequest(c, rc, "artifact_type and record_id are required")
	}
	start, err := h.service().StartRecord(c.Request().Context(), in.ArtifactType, in.RecordID)
	if err != nil {
		return fail(c, rc, "category classification not started", err)
	}
	return c.JSON(http.StatusAccepted, map[string]any{"run": start})
}

func (h *Handler) ReclassifyStale(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_CAT_113")
	defer rc.Close()
	var in struct {
		ArtifactType string `json:"artifact_type"`
		Limit        int    `json:"limit"`
	}
	if err := c.Bind(&in); err != nil || strings.TrimSpace(in.ArtifactType) == "" {
		return badRequest(c, rc, "artifact_type is required")
	}
	start, err := h.service().StartStale(c.Request().Context(), in.ArtifactType, in.Limit)
	if err != nil {
		return fail(c, rc, "reclassification not started", err)
	}
	return c.JSON(http.StatusAccepted, map[string]any{"run": start})
}

func (h *Handler) ListClassifications(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_CAT_114")
	defer rc.Close()
	list, err := h.service().Store.ListClassifications(c.Request().Context(), ClassificationFilter{
		ArtifactType: c.QueryParam("artifact_type"),
		ArtifactID:   c.QueryParam("artifact_id"),
		RecordID:     queryInt(c, "record_id"),
		Status:       c.QueryParam("status"),
		LatestOnly:   c.QueryParam("latest") == "true",
		Limit:        int(queryInt(c, "limit")),
	})
	if err != nil {
		return fail(c, rc, "list classifications failed", err)
	}
	return c.JSON(http.StatusOK, map[string]any{"classifications": list})
}

func (h *Handler) ReviewQueue(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_CAT_115")
	defer rc.Close()
	list, err := h.service().Store.ListClassifications(c.Request().Context(), ClassificationFilter{
		ArtifactType: c.QueryParam("artifact_type"), ReviewQueue: true, Limit: int(queryInt(c, "limit")),
	})
	if err != nil {
		return fail(c, rc, "list review queue failed", err)
	}
	return c.JSON(http.StatusOK, map[string]any{"classifications": list})
}

// Review applies a reviewer action: retry, close or resolve (with leaf node IDs).
func (h *Handler) Review(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_CAT_116")
	defer rc.Close()
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return badRequest(c, rc, "classification id must be an integer")
	}
	var in struct {
		Action  string   `json:"action"`
		NodeIDs []string `json:"node_ids"`
		Note    string   `json:"note"`
	}
	if err := c.Bind(&in); err != nil {
		return badRequest(c, rc, "invalid review: "+err.Error())
	}
	ctx := c.Request().Context()
	svc := h.service()
	user := rc.GetUserID()
	switch in.Action {
	case "retry":
		start, err := svc.StartRetry(ctx, id)
		if err != nil {
			return fail(c, rc, "retry not started", err)
		}
		rc.GetLogger().Info("category classification retry", "classification_id", id, "user_id", user)
		return c.JSON(http.StatusAccepted, map[string]any{"run": start})
	case "close":
		err = svc.Store.CloseClassification(ctx, id, user, in.Note)
	case "resolve":
		err = svc.Store.ResolveClassification(ctx, id, in.NodeIDs, user, in.Note)
	default:
		return badRequest(c, rc, "action must be retry, close or resolve")
	}
	if err != nil {
		return fail(c, rc, "review action failed", err)
	}
	rc.GetLogger().Info("category classification reviewed", "classification_id", id, "action", in.Action, "nodes", len(in.NodeIDs), "user_id", user)
	cl, err := svc.Store.GetClassification(ctx, id)
	if err != nil {
		return fail(c, rc, "load classification failed", err)
	}
	return c.JSON(http.StatusOK, map[string]any{"classification": cl})
}

func (h *Handler) ArtifactBindings(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_CAT_117")
	defer rc.Close()
	artifactType, artifactID := c.QueryParam("artifact_type"), c.QueryParam("artifact_id")
	if artifactType == "" || artifactID == "" {
		return badRequest(c, rc, "artifact_type and artifact_id are required")
	}
	assoc, err := h.service().Store.ArtifactBindings(c.Request().Context(), artifactType, artifactID)
	if err != nil {
		return fail(c, rc, "load artifact bindings failed", err)
	}
	return c.JSON(http.StatusOK, assoc)
}
