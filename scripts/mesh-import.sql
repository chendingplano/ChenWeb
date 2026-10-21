-- Load resources.mesh (raw N-Triples rows) into the normalized resources.mesh_* tables.
-- Requires the migration 20261022000001_create_resources_mesh_normalized.sql.
--
-- Run once per MeSH release (idempotent: everything is truncated and reloaded in one
-- transaction):
--   psql -h 127.0.0.1 -U <user> -d miner -v ON_ERROR_STOP=1 -f scripts/mesh-import.sql
--
-- Mapping from RDF roles to tables:
--   rdf:type and rdfs:label         -> mesh_entity.kind / mesh_entity.label
--   literal objects (and other IRIs) -> mesh_property
--   IRI objects inside the MeSH namespace -> mesh_link (vocabulary IRIs excluded)

BEGIN;

TRUNCATE resources.mesh_link, resources.mesh_property, resources.mesh_entity, resources.mesh_import;

-- Entities: one row per subject. A subject's kind is its rdf:type; its label is rdfs:label,
-- falling back to vocab prefLabel (Concepts and Terms carry only prefLabel).
INSERT INTO resources.mesh_entity (id, kind, label)
SELECT sid,
       COALESCE(MAX(CASE WHEN pred = 'type'
                         THEN regexp_replace(object, '^<.*vocab#([^>]*)>$', '\1') END), ''),
       COALESCE(MAX(CASE WHEN pred = 'label' AND object LIKE '"%'
                         THEN regexp_replace(object, '^"(.*)"(@[A-Za-z0-9-]+|\^\^<[^>]*>)?$', '\1') END),
                MAX(CASE WHEN pred = 'prefLabel' AND object LIKE '"%'
                         THEN regexp_replace(object, '^"(.*)"(@[A-Za-z0-9-]+|\^\^<[^>]*>)?$', '\1') END),
                '')
FROM (
    SELECT regexp_replace(subject, '^<.*/([^/]*)>$', '\1')   AS sid,
           regexp_replace(predicate, '^<.*[#/]([^#/]*)>$', '\1') AS pred,
           object
    FROM resources.mesh
) t
GROUP BY sid;

-- Links: relations between MeSH records (rdf:type and vocabulary objects are not links).
INSERT INTO resources.mesh_link (subject_id, predicate, object_id)
SELECT DISTINCT sid, pred, regexp_replace(object, '^<.*/([^/]*)>$', '\1')
FROM (
    SELECT regexp_replace(subject, '^<.*/([^/]*)>$', '\1')   AS sid,
           regexp_replace(predicate, '^<.*[#/]([^#/]*)>$', '\1') AS pred,
           object
    FROM resources.mesh
) t
WHERE object LIKE '<http://id.nlm.nih.gov/mesh/%'
  AND object NOT LIKE '<http://id.nlm.nih.gov/mesh/vocab#%'
  AND pred NOT IN ('type', 'label');

-- Properties: literal attributes and any non-MeSH IRI. Language tag is kept separately.
INSERT INTO resources.mesh_property (entity_id, predicate, value, lang)
SELECT DISTINCT sid, pred,
       CASE WHEN object LIKE '"%'
            THEN replace(regexp_replace(object, '^"(.*)"(@[A-Za-z0-9-]+|\^\^<[^>]*>)?$', '\1'), '\"', '"')
            ELSE regexp_replace(object, '^<(.*)>$', '\1') END,
       CASE WHEN object LIKE '"%'
            THEN COALESCE((regexp_match(object, '"@([A-Za-z0-9-]+)$'))[1], '')
            ELSE '' END
FROM (
    SELECT regexp_replace(subject, '^<.*/([^/]*)>$', '\1')   AS sid,
           regexp_replace(predicate, '^<.*[#/]([^#/]*)>$', '\1') AS pred,
           object
    FROM resources.mesh
) t
WHERE pred NOT IN ('type', 'label')
  AND object NOT LIKE '<http://id.nlm.nih.gov/mesh/%';

INSERT INTO resources.mesh_import (release, source_rows)
SELECT regexp_replace(subject, '^<http://id.nlm.nih.gov/mesh/([^/]*)/.*$', '\1'), (SELECT count(*) FROM resources.mesh)
FROM resources.mesh
LIMIT 1;

COMMIT;

ANALYZE resources.mesh_entity;
ANALYZE resources.mesh_property;
ANALYZE resources.mesh_link;

SELECT kind, count(*) FROM resources.mesh_entity GROUP BY kind ORDER BY count(*) DESC;
SELECT count(*) AS properties FROM resources.mesh_property;
SELECT count(*) AS links FROM resources.mesh_link;
