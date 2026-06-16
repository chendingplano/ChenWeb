package docprocessing

import (
	"fmt"
	"sort"
)

// Connection is one edge in the Deep Wiki artifact graph. It mirrors a row of
// kb.artifact_connections. See
// KnowledgeStore/Capsules/coding-capsules/deep-wiki/artifact-graph-creation.md
type Connection struct {
	SourceRecordID    int64
	TargetRecordID    int64
	SourceType        string
	SourceID          string
	TargetType        string
	TargetID          string
	RelationName      string
	RelationMethod    string
	Confidence        float64
	Overlap           *OverlapInfo
	Provenance        map[string]any
	SemanticSignature string
	SourceDesc        string
	TargetDesc        string
	ExtraInfo         map[string]any
}

// OverlapInfo is the evidence stored on a line_overlap edge: the shared source
// lines and their count. Serialised into the JSONB `overlap` column.
type OverlapInfo struct {
	OverlapCount int   `json:"overlap_count"`
	OverlapLines []int `json:"overlap_lines"`
}

// ArtifactRef identifies a target artifact and the source line spans it covers.
// Spans use the same textual form as elsewhere in the pipeline ("5", "12:14").
type ArtifactRef struct {
	Type  string
	ID    string
	Spans []string
}

// relation_method values for kb.artifact_connections.
const (
	RelationMethodLLM         = "llm"
	RelationMethodLineOverlap = "line_overlap"
	RelationMethodStructural  = "structural"
	RelationMethodManual      = "manual"
)

// artifactTypeChunk is the source_type used for chunk endpoints.
const artifactTypeChunk = "chunk"

// relation_name values from the Deep Wiki connection catalog (+deep-wiki-rqmt.md).
const (
	RelationHasMetrics        = "has-metrics"
	RelationHasInventoryItems = "has-inventory-items"
	RelationHasTopic          = "has-topic"
	RelationHasProvision      = "has-provision"
	RelationHasPartComponent  = "has-part-component"
	RelationHasScene          = "has-scene"
	RelationHasSummary        = "has-summary"
	RelationEntityRelations   = "entity relations"
	RelationBelongTo          = "belong_to"
	RelationHasPredicate      = "has-predicate"
	RelationHasInstance       = "has-instance"
)

// Canonical relation store (ADR 2026061401). All edges produced by the
// entity-relation processor's graph step share one relation_method so they can be
// replaced idempotently as a group, and carry endpoints typed by the artifact-type
// registry (D2).
const (
	// RelationMethodEntityRelation tags every edge the relation-graph step writes
	// (subject->object and predicate-of), enabling a single method-scoped idempotent
	// replace per record.
	RelationMethodEntityRelation = "entity_relation"
	RelationMethodEntityName     = "entity_name"
	RelationMethodCategoryName   = "category_name"

	// artifactTypeCategory / artifactTypeRelationPredicate are the artifact_type
	// discriminators for the dictionary endpoints promoted to artifacts in D2.
	artifactTypeCategory          = "category"
	artifactTypeRelationPredicate = "relation_predicate"
	artifactTypeEntityName        = "entity_name"
)

// ChunkConnectionID returns the positional identifier used for a chunk endpoint.
// Chunks are not first-class DB rows; they are addressed by (record, index), and
// line_overlap edges are recomputed on every reprocess, so positional ids are safe.
func ChunkConnectionID(recordID int64, chunkIndex int) string {
	return fmt.Sprintf("%d_chk_%d", recordID, chunkIndex)
}

// DeriveLineOverlapConnections builds chunk -> artifact line_overlap edges: one
// edge per (chunk, target) pair whose source lines intersect. The result is
// linear because chunks are small and targets are chunk-scoped.
func DeriveLineOverlapConnections(recordID int64, targetType, relationName string, chunks []Block, targets []ArtifactRef) []Connection {
	var out []Connection
	for _, ch := range chunks {
		chunkLines := make(map[int]struct{}, len(ch.Lines))
		for _, l := range ch.Lines {
			if l.LineNumber > 0 {
				chunkLines[l.LineNumber] = struct{}{}
			}
		}
		if len(chunkLines) == 0 {
			continue
		}
		for _, t := range targets {
			overlap := overlapLines(chunkLines, t.Spans)
			if len(overlap) == 0 {
				continue
			}
			out = append(out, Connection{
				SourceRecordID: recordID,
				TargetRecordID: recordID,
				SourceType:     artifactTypeChunk,
				SourceID:       ChunkConnectionID(recordID, ch.Index),
				TargetType:     targetType,
				TargetID:       t.ID,
				RelationName:   relationName,
				RelationMethod: RelationMethodLineOverlap,
				SourceDesc:     connectionEndpointDesc(artifactTypeChunk, ChunkConnectionID(recordID, ch.Index)),
				TargetDesc:     connectionEndpointDesc(targetType, t.ID),
				Overlap:        &OverlapInfo{OverlapCount: len(overlap), OverlapLines: overlap},
			})
		}
	}
	return out
}

func connectionEndpointDesc(artifactType, artifactID string) string {
	return fmt.Sprintf("%s:%s", artifactType, artifactID)
}

// overlapLines returns the sorted set of line numbers present in both the chunk
// and the artifact's spans. Span parsing reuses parseMetricLineSpan (same package).
func overlapLines(chunkLines map[int]struct{}, spans []string) []int {
	seen := make(map[int]struct{})
	var out []int
	for _, span := range spans {
		start, end, ok := parseMetricLineSpan(span)
		if !ok {
			continue
		}
		for n := start; n <= end; n++ {
			if _, inChunk := chunkLines[n]; !inChunk {
				continue
			}
			if _, dup := seen[n]; dup {
				continue
			}
			seen[n] = struct{}{}
			out = append(out, n)
		}
	}
	sort.Ints(out)
	return out
}
