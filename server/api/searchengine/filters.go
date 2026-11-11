package searchengine

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/lib/pq"
)

// argList numbers query arguments in the order they are added.
type argList struct{ vals []any }

func (a *argList) add(v any) string {
	a.vals = append(a.vals, v)
	return "$" + strconv.Itoa(len(a.vals))
}

// partitionTypes are the artifact types with a kb.search_artifacts_<type> partition. pg_search accepts
// BM25 operators only on these partitions, never on the partitioned parent.
var partitionTypes = []string{
	"chunk", "entity", "inventory_item", "knowledge", "metric", "product", "provision",
	"relation", "scene_block", "semantic_projection", "summary", "topic",
}

// PartitionTypes lists the artifact types that have a kb.search_artifacts_<type> partition.
func PartitionTypes() []string { return slices.Clone(partitionTypes) }

// partitionsFor returns the partition tables a BM25 query reads. An unknown type is an error: names
// are rendered into SQL, so they must come from partitionTypes.
func partitionsFor(types []string) ([]string, error) {
	if len(types) == 0 {
		types = partitionTypes
	}
	tables := make([]string, 0, len(types))
	for _, t := range types {
		t = strings.TrimSpace(t)
		if !slices.Contains(partitionTypes, t) {
			return nil, fmt.Errorf("searchengine: unknown artifact type %q", t)
		}
		tables = append(tables, "kb.search_artifacts_"+t)
	}
	return tables, nil
}

// filterClauses renders every filter against alias `sa`. The referenced artifact (reference queries)
// is excluded here too, so it never takes a slot. withTypes adds the artifact_type condition, which a
// partition branch does not need.
func filterClauses(a *argList, f Filters, self *ArtifactRef, withTypes bool) []string {
	var c []string
	if withTypes && len(f.ArtifactTypes) > 0 {
		c = append(c, "sa.artifact_type = ANY("+a.add(pq.Array(f.ArtifactTypes))+")")
	}
	if len(f.RecordIDs) > 0 {
		c = append(c, "sa.input_record_id = ANY("+a.add(pq.Array(f.RecordIDs))+")")
	}
	if len(f.ExcludeRecordIDs) > 0 {
		c = append(c, "NOT (sa.input_record_id = ANY("+a.add(pq.Array(f.ExcludeRecordIDs))+"))")
	}
	if len(f.ExcludeArtifactIDs) > 0 {
		c = append(c, "NOT (sa.artifact_id = ANY("+a.add(pq.Array(f.ExcludeArtifactIDs))+"))")
	}
	if self != nil {
		c = append(c, fmt.Sprintf("NOT (sa.artifact_type = %s AND sa.artifact_id = %s)", a.add(self.Type), a.add(self.ID)))
	}
	if f.KnowledgeStoreID != "" {
		c = append(c, fmt.Sprintf(`EXISTS (
			SELECT 1 FROM kb.inputs agent_input
			JOIN kb.knowledge_store agent_store ON agent_store.id=agent_input.ks_store_id
			WHERE agent_input.id=sa.input_record_id AND agent_store.status='active' AND agent_store.id::text=%s
		)`, a.add(f.KnowledgeStoreID)))
	}
	if f.DocumentGroup != "" {
		c = append(c, fmt.Sprintf(`EXISTS (
			SELECT 1 FROM kb.inputs agent_input
			WHERE agent_input.id=sa.input_record_id AND agent_input.type=%s
		)`, a.add(f.DocumentGroup)))
	}
	s := f.Structural
	if s.CategoryPath != "" {
		c = append(c, "sa.category_paths::text ILIKE "+a.add("%"+s.CategoryPath+"%"))
	}
	if s.InventoryCategory != "" {
		c = append(c, fmt.Sprintf("COALESCE(sa.semantic_payload->'item_categories', '[]'::jsonb) @> to_jsonb(%s::text)", a.add(s.InventoryCategory)))
	}
	for _, p := range []struct{ key, val string }{
		{"topic_type", s.TopicType}, {"scene_type", s.SceneType}, {"provision_type", s.ProvisionType},
		{"product_type", s.ProductType}, {"relation_type", s.RelationType}, {"manufacturer", s.Manufacturer},
		{"brand", s.Brand}, {"model_number", s.ModelNumber}, {"part_number", s.PartNumber},
		{"validation_status", s.ValidationStatus},
	} {
		if p.val != "" {
			c = append(c, fmt.Sprintf("COALESCE(sa.semantic_payload->>'%s', '') = %s", p.key, a.add(p.val)))
		}
	}
	return c
}

func andJoin(clauses []string) string {
	if len(clauses) == 0 {
		return "TRUE"
	}
	return strings.Join(clauses, " AND ")
}
