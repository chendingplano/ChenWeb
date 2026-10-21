-- Normalized MeSH in the resources schema: one row per MeSH record (entity), its
-- literal attributes, and its links to other records. Replaces querying the raw
-- triples in resources.mesh, which has no indexes. Loaded from resources.mesh by
-- scripts/mesh-import.sql (a one-time job, not part of this migration).

-- +goose Up
CREATE SCHEMA IF NOT EXISTS resources;

CREATE TABLE IF NOT EXISTS resources.mesh_import (
    release     TEXT        PRIMARY KEY,
    source_rows BIGINT      NOT NULL,
    imported_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- kind is the RDF class (TopicalDescriptor, Concept, Term, TreeNumber, ...).
-- id is the last segment of the MeSH IRI (D000001, M0000001, D02.355.291.933.125).
CREATE TABLE IF NOT EXISTS resources.mesh_entity (
    id    TEXT PRIMARY KEY,
    kind  TEXT NOT NULL,
    label TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_mesh_entity_kind_label ON resources.mesh_entity (kind, label);

CREATE TABLE IF NOT EXISTS resources.mesh_property (
    entity_id TEXT NOT NULL REFERENCES resources.mesh_entity (id),
    predicate TEXT NOT NULL,
    value     TEXT NOT NULL,
    lang      TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_mesh_property_entity ON resources.mesh_property (entity_id);

-- predicate is the vocabulary local name (broaderDescriptor, treeNumber, concept, ...).
CREATE TABLE IF NOT EXISTS resources.mesh_link (
    subject_id TEXT NOT NULL REFERENCES resources.mesh_entity (id),
    predicate  TEXT NOT NULL,
    object_id  TEXT NOT NULL,
    PRIMARY KEY (subject_id, predicate, object_id)
);
CREATE INDEX IF NOT EXISTS idx_mesh_link_object ON resources.mesh_link (object_id, predicate);

-- +goose Down
DROP TABLE IF EXISTS resources.mesh_link;
DROP TABLE IF EXISTS resources.mesh_property;
DROP TABLE IF EXISTS resources.mesh_entity;
DROP TABLE IF EXISTS resources.mesh_import;
