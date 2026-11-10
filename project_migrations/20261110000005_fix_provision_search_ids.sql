-- Provision rows in the search index (kb.search_artifacts) and the connections
-- that point at provisions (kb.artifact_connections).
--
-- 1. Orphans: delete rows whose provision no longer exists in kb.provisions.
--    Provisions deleted outside re-extraction (scripts/clear_artifact_data.sql
--    deletes all of kb.provisions) left their index rows and connections behind:
--    on 2026-11-10, 13,754 of 13,959 provision index rows.
-- 2. Doubled ids: rewrite "416_prv_416_prv_1" to the prov_id "416_prv_1". The
--    indexers passed the prov_id to kbsearch.BuildArtifactID as the sequence, so
--    every provision id carried the record prefix twice and no lookup by prov_id
--    matched (the provisions reviewer found no cross-document matches). Fixed in
--    provisionArtifactID (server/api/doc-processing/search_indexing.go).
--
-- Down is a no-op: the deleted rows pointed at nothing and the doubled ids were
-- a bug.

-- +goose Up

-- 1. Orphaned provision index rows.
DELETE FROM kb.search_artifacts sa
WHERE sa.artifact_type = 'provision'
  AND NOT EXISTS (
      SELECT 1 FROM kb.provisions p
      WHERE p.input_record_id = sa.input_record_id
        AND (p.id = sa.source_row_id
             OR p.prov_id = regexp_replace(sa.artifact_id, '^[0-9]+_prv_([0-9]+_prv_)', '\1')));

-- 1. Connections whose provision end no longer exists. Only ends holding a
--    provision id count: the object_id connections carry source_type
--    'provision' but an object id ("obj_416_…") on both ends.
DELETE FROM kb.artifact_connections c
WHERE (c.source_type = 'provision'
       AND c.source_id ~ '^[0-9]+_prv_'
       AND NOT EXISTS (
           SELECT 1 FROM kb.provisions p
           WHERE p.input_record_id = c.source_record_id
             AND p.prov_id = regexp_replace(c.source_id, '^[0-9]+_prv_([0-9]+_prv_)', '\1')))
   OR (c.target_type = 'provision'
       AND c.target_id ~ '^[0-9]+_prv_'
       AND NOT EXISTS (
           SELECT 1 FROM kb.provisions p
           WHERE p.input_record_id = c.target_record_id
             AND p.prov_id = regexp_replace(c.target_id, '^[0-9]+_prv_([0-9]+_prv_)', '\1')));

-- 2. Doubled ids in the index. A doubled row whose corrected id already exists
--    is a duplicate and is dropped first, so the rewrite cannot collide.
DELETE FROM kb.search_artifacts d
WHERE d.artifact_type = 'provision'
  AND d.artifact_id ~ '^[0-9]+_prv_[0-9]+_prv_'
  AND EXISTS (
      SELECT 1 FROM kb.search_artifacts k
      WHERE k.artifact_type = 'provision'
        AND k.artifact_id = regexp_replace(d.artifact_id, '^[0-9]+_prv_([0-9]+_prv_)', '\1'));

UPDATE kb.search_artifacts
SET artifact_id = regexp_replace(artifact_id, '^[0-9]+_prv_([0-9]+_prv_)', '\1')
WHERE artifact_type = 'provision'
  AND artifact_id ~ '^[0-9]+_prv_[0-9]+_prv_';

-- 2. Doubled ids in connections, source side then target side, each with the
--    same duplicate guard against the connections' unique key.
DELETE FROM kb.artifact_connections d
WHERE d.source_type = 'provision'
  AND d.source_id ~ '^[0-9]+_prv_[0-9]+_prv_'
  AND EXISTS (
      SELECT 1 FROM kb.artifact_connections k
      WHERE k.relation_method = d.relation_method
        AND k.source_type = d.source_type
        AND k.source_id = regexp_replace(d.source_id, '^[0-9]+_prv_([0-9]+_prv_)', '\1')
        AND k.target_type = d.target_type
        AND k.target_id = d.target_id
        AND k.relation_name = d.relation_name);

UPDATE kb.artifact_connections
SET source_id = regexp_replace(source_id, '^[0-9]+_prv_([0-9]+_prv_)', '\1')
WHERE source_type = 'provision'
  AND source_id ~ '^[0-9]+_prv_[0-9]+_prv_';

DELETE FROM kb.artifact_connections d
WHERE d.target_type = 'provision'
  AND d.target_id ~ '^[0-9]+_prv_[0-9]+_prv_'
  AND EXISTS (
      SELECT 1 FROM kb.artifact_connections k
      WHERE k.relation_method = d.relation_method
        AND k.source_type = d.source_type
        AND k.source_id = d.source_id
        AND k.target_type = d.target_type
        AND k.target_id = regexp_replace(d.target_id, '^[0-9]+_prv_([0-9]+_prv_)', '\1')
        AND k.relation_name = d.relation_name);

UPDATE kb.artifact_connections
SET target_id = regexp_replace(target_id, '^[0-9]+_prv_([0-9]+_prv_)', '\1')
WHERE target_type = 'provision'
  AND target_id ~ '^[0-9]+_prv_[0-9]+_prv_';

-- +goose Down
SELECT 1;
