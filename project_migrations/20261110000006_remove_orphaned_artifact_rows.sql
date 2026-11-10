-- Rows derived from metrics and provisions that no longer exist.
-- scripts/clear_artifact_data.sql deleted all of kb.metrics and kb.provisions
-- without the rows built from them (fixed in the same change). Migration
-- 20261110000005 removed the provision search rows and their connections; this
-- one removes the rest:
--
-- 1. Metric rows in the search index (kb.search_artifacts) whose metric is gone
--    (on 2026-11-10, 5,621 of 8,021; the index covered 64 records, kb.metrics 15).
-- 2. Connections whose metric end is gone. Only ends holding a metric id count:
--    category_name connections carry a category label on the target end, and
--    object_id connections carry an object id ("obj_416_…") on both ends.
-- 3. kb.artifact_objects rows (object mentions) of metrics and provisions that
--    are gone.
-- 4. object_id connections ("belong_to" an object node) of metrics and
--    provisions none of whose anchoring artifacts (extra_info.artifact_ids)
--    exists any more.
--
-- Not touched: kb.semantic_processing_outcomes / unresolved_semantic_occurrences
-- rows of deleted metrics (processing history with dependent findings), and
-- kb.keyword_occurrences (its artifact_id holds metric names, not metric ids).
--
-- Down is a no-op: the deleted rows pointed at nothing.

-- +goose Up

-- 1. Orphaned metric index rows.
DELETE FROM kb.search_artifacts sa
WHERE sa.artifact_type = 'metric'
  AND NOT EXISTS (
      SELECT 1 FROM kb.metrics m
      WHERE m.input_record_id = sa.input_record_id
        AND (m.id = sa.source_row_id OR m.metric_id = sa.artifact_id));

-- 2. Connections whose metric end no longer exists.
DELETE FROM kb.artifact_connections c
WHERE (c.source_type = 'metric'
       AND c.source_id ~ '^[0-9]+_mtc_'
       AND NOT EXISTS (
           SELECT 1 FROM kb.metrics m
           WHERE m.input_record_id = c.source_record_id AND m.metric_id = c.source_id))
   OR (c.target_type = 'metric'
       AND c.target_id ~ '^[0-9]+_mtc_'
       AND NOT EXISTS (
           SELECT 1 FROM kb.metrics m
           WHERE m.input_record_id = c.target_record_id AND m.metric_id = c.target_id));

-- 3. Object mentions of metrics and provisions that no longer exist.
DELETE FROM kb.artifact_objects ao
WHERE (ao.artifact_type = 'metric'
       AND NOT EXISTS (
           SELECT 1 FROM kb.metrics m
           WHERE m.input_record_id = ao.input_record_id AND m.metric_id = ao.artifact_id))
   OR (ao.artifact_type = 'provision'
       AND NOT EXISTS (
           SELECT 1 FROM kb.provisions p
           WHERE p.input_record_id = ao.input_record_id AND p.prov_id = ao.artifact_id));

-- 4. Object-node connections left without any anchoring metric or provision.
DELETE FROM kb.artifact_connections c
WHERE c.relation_method = 'object_id'
  AND c.source_type IN ('metric', 'provision')
  AND NOT EXISTS (
      SELECT 1
      FROM jsonb_array_elements_text(COALESCE(c.extra_info -> 'artifact_ids', '[]'::jsonb)) AS a(id)
      WHERE (c.source_type = 'metric'
             AND EXISTS (SELECT 1 FROM kb.metrics m WHERE m.metric_id = a.id))
         OR (c.source_type = 'provision'
             AND EXISTS (SELECT 1 FROM kb.provisions p WHERE p.prov_id = a.id)));

-- +goose Down
SELECT 1;
