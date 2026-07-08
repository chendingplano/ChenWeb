package kbhandler

import (
	"context"
	"fmt"

	docprocessing "github.com/chendingplano/deepdoc/server/api/doc-processing"
)

const (
	defaultSimilarArtifactTopN  = 10
	defaultObjectChartRecursion = 3
	defaultObjectChartMaxNodes  = 300
)

// ObjectGraphNode is one node in the Object Relation Chart graph.
type ObjectGraphNode struct {
	Key             string `json:"key"`
	Type            string `json:"type"` // object_node | artifact_object | artifact
	Label           string `json:"label"`
	ObjectID        string `json:"object_id,omitempty"`
	ArtifactType    string `json:"artifact_type,omitempty"`
	ArtifactID      string `json:"artifact_id,omitempty"`
	ReconcileStatus string `json:"reconcile_status,omitempty"`
}

// ObjectGraphEdge is one directed edge in the Object Relation Chart graph.
type ObjectGraphEdge struct {
	From string `json:"from"`
	To   string `json:"to"`
	Type string `json:"type"` // mentions | about | similar | same_object
}

// ObjectGraph is the response payload for the relation graph.
type ObjectGraph struct {
	SeedObjectID string            `json:"seed_object_id"`
	Truncated    bool              `json:"truncated"`
	Nodes        []ObjectGraphNode `json:"nodes"`
	Edges        []ObjectGraphEdge `json:"edges"`
}

// objectGraphSource abstracts the data access the traversal needs, so the
// bounded BFS can be unit-tested with an in-memory fake.
type objectGraphSource interface {
	LoadArtifactObjectByID(ctx context.Context, id int64) (docprocessing.ArtifactObject, bool, error)
	ObjectsByObjectID(ctx context.Context, objectID string) ([]docprocessing.ArtifactObject, error)
	ObjectsByArtifact(ctx context.Context, artifactType, artifactID string) ([]docprocessing.ArtifactObject, error)
	ObjectNodeByID(ctx context.Context, objectID string) (docprocessing.ObjectNode, bool, error)
	SimilarArtifacts(ctx context.Context, artifactType, artifactID string, topN int) ([]docprocessing.OnTheFlySemanticMatch, error)
}

// objectGraphSeed selects where the traversal starts. Prefer ObjectID; use
// Mention when seeding from a specific (possibly unresolved) mention row.
type objectGraphSeed struct {
	ObjectID string
	Mention  *docprocessing.ArtifactObject
}

// objectGraphOptions bounds the traversal.
type objectGraphOptions struct {
	SimilarTopN    int
	RecursiveLevel int
	MaxNodes       int
}

// graphBuilder accumulates deduplicated nodes and edges in insertion order and
// enforces the global node cap.
type graphBuilder struct {
	nodes     map[string]ObjectGraphNode
	order     []string
	edges     map[string]bool
	edgeList  []ObjectGraphEdge
	maxNodes  int
	truncated bool
}

func newGraphBuilder(maxNodes int) *graphBuilder {
	return &graphBuilder{nodes: map[string]ObjectGraphNode{}, edges: map[string]bool{}, maxNodes: maxNodes}
}

// addNode inserts a node once. When the cap is reached it flips truncated and
// refuses further nodes so edges never dangle to a missing endpoint.
func (g *graphBuilder) addNode(n ObjectGraphNode) {
	if _, ok := g.nodes[n.Key]; ok {
		return
	}
	if len(g.nodes) >= g.maxNodes {
		g.truncated = true
		return
	}
	g.nodes[n.Key] = n
	g.order = append(g.order, n.Key)
}

// addEdge inserts a directed edge once, but only if both endpoints exist.
func (g *graphBuilder) addEdge(from, to, typ string) {
	if _, ok := g.nodes[from]; !ok {
		return
	}
	if _, ok := g.nodes[to]; !ok {
		return
	}
	key := from + "|" + to + "|" + typ
	if g.edges[key] {
		return
	}
	g.edges[key] = true
	g.edgeList = append(g.edgeList, ObjectGraphEdge{From: from, To: to, Type: typ})
}

func objectKey(objectID string) string { return "object:" + objectID }
func mentionKey(id int64) string       { return fmt.Sprintf("ao:%d", id) }
func artifactNodeKey(t, id string) string {
	return "artifact:" + t + ":" + id
}

func objectNodeLabel(n docprocessing.ObjectNode, objectID string) string {
	if n.CanonicalName != "" {
		return n.CanonicalName
	}
	return objectID
}

func mentionLabel(m docprocessing.ArtifactObject) string {
	if m.ObjectName != "" {
		return m.ObjectName
	}
	if m.ObjectNameEn != "" {
		return m.ObjectNameEn
	}
	return m.ArtifactType + ":" + m.ArtifactID
}

// expandMention adds the mention node, its artifact, similar artifacts, and the
// objects of those similar artifacts. It returns the object_ids discovered via
// similar artifacts (Node Set D) for the caller to enqueue. ownerObjectID is the
// canonical object the mention belongs to, or "" for an unresolved seed mention
// (which is then rendered as a terminal node with no object edges).
func (g *graphBuilder) expandMention(ctx context.Context, src objectGraphSource, m docprocessing.ArtifactObject, opts objectGraphOptions, ownerObjectID string) ([]string, error) {
	aoKey := mentionKey(m.ID)
	g.addNode(ObjectGraphNode{
		Key: aoKey, Type: "artifact_object", Label: mentionLabel(m),
		ObjectID: m.ObjectID, ArtifactType: m.ArtifactType, ArtifactID: m.ArtifactID,
		ReconcileStatus: m.ReconcileStatus,
	})
	if ownerObjectID != "" {
		g.addEdge(objectKey(ownerObjectID), aoKey, "mentions")
		g.addEdge(aoKey, objectKey(ownerObjectID), "same_object")
	}

	artKey := artifactNodeKey(m.ArtifactType, m.ArtifactID)
	g.addNode(ObjectGraphNode{Key: artKey, Type: "artifact", Label: m.ArtifactType + ":" + m.ArtifactID, ArtifactType: m.ArtifactType, ArtifactID: m.ArtifactID})
	g.addEdge(aoKey, artKey, "about")

	sims, err := src.SimilarArtifacts(ctx, m.ArtifactType, m.ArtifactID, opts.SimilarTopN)
	if err != nil {
		return nil, err
	}
	var next []string
	for _, s := range sims {
		simKey := artifactNodeKey(s.ArtifactType, s.ArtifactID)
		g.addNode(ObjectGraphNode{Key: simKey, Type: "artifact", Label: s.ArtifactType + ":" + s.ArtifactID, ArtifactType: s.ArtifactType, ArtifactID: s.ArtifactID})
		g.addEdge(artKey, simKey, "similar")

		dObjs, err := src.ObjectsByArtifact(ctx, s.ArtifactType, s.ArtifactID)
		if err != nil {
			return nil, err
		}
		for _, d := range dObjs {
			if d.ObjectID != "" {
				next = append(next, d.ObjectID)
			}
		}
	}
	return next, nil
}

// expandObject adds the canonical object node and expands each of its mention
// rows. It returns the object_ids reachable via similar artifacts.
func (g *graphBuilder) expandObject(ctx context.Context, src objectGraphSource, objectID string, opts objectGraphOptions) ([]string, error) {
	node, found, err := src.ObjectNodeByID(ctx, objectID)
	if err != nil {
		return nil, err
	}
	label := objectID
	if found {
		label = objectNodeLabel(node, objectID)
	}
	g.addNode(ObjectGraphNode{Key: objectKey(objectID), Type: "object_node", Label: label, ObjectID: objectID})

	mentions, err := src.ObjectsByObjectID(ctx, objectID)
	if err != nil {
		return nil, err
	}
	var next []string
	for _, m := range mentions {
		n, err := g.expandMention(ctx, src, m, opts, objectID)
		if err != nil {
			return nil, err
		}
		next = append(next, n...)
	}
	return next, nil
}

// BuildObjectGraph builds the Object Relation Chart graph with a bounded,
// deduplicated breadth-first traversal (see ADR object-manager). It maintains a
// visited set on object_id (never expands the same object twice), stops and
// marks the result truncated at MaxNodes, and never recurses beyond
// RecursiveLevel. Unresolved seed mentions (object_id == "") are rendered as
// terminal nodes rather than recursion seeds.
func BuildObjectGraph(ctx context.Context, src objectGraphSource, seed objectGraphSeed, opts objectGraphOptions) (ObjectGraph, error) {
	if opts.SimilarTopN <= 0 {
		opts.SimilarTopN = defaultSimilarArtifactTopN
	}
	if opts.RecursiveLevel < 0 {
		opts.RecursiveLevel = 0
	}
	if opts.MaxNodes <= 0 {
		opts.MaxNodes = defaultObjectChartMaxNodes
	}

	g := newGraphBuilder(opts.MaxNodes)
	result := ObjectGraph{SeedObjectID: seed.ObjectID}

	type frontier struct {
		objectID string
		depth    int
	}
	visited := map[string]bool{}
	var queue []frontier

	switch {
	case seed.Mention != nil && seed.ObjectID == "":
		next, err := g.expandMention(ctx, src, *seed.Mention, opts, "")
		if err != nil {
			return result, err
		}
		if opts.RecursiveLevel >= 1 {
			for _, oid := range next {
				queue = append(queue, frontier{objectID: oid, depth: 1})
			}
		}
	case seed.ObjectID != "":
		queue = append(queue, frontier{objectID: seed.ObjectID, depth: 0})
	}

	for len(queue) > 0 {
		f := queue[0]
		queue = queue[1:]
		if visited[f.objectID] {
			continue
		}
		if len(g.nodes) >= g.maxNodes {
			g.truncated = true
			break
		}
		visited[f.objectID] = true

		next, err := g.expandObject(ctx, src, f.objectID, opts)
		if err != nil {
			return result, err
		}
		if f.depth < opts.RecursiveLevel {
			for _, oid := range next {
				if !visited[oid] {
					queue = append(queue, frontier{objectID: oid, depth: f.depth + 1})
				}
			}
		}
	}

	result.Truncated = g.truncated
	result.Nodes = make([]ObjectGraphNode, 0, len(g.order))
	for _, key := range g.order {
		result.Nodes = append(result.Nodes, g.nodes[key])
	}
	result.Edges = g.edgeList
	if result.Edges == nil {
		result.Edges = []ObjectGraphEdge{}
	}
	return result, nil
}
