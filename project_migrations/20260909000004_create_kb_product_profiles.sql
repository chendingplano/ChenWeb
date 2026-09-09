-- +goose Up
-- Product Metric Reviewer (openspec change product-metric-reviewer), capability
-- product-scope-profile. A profile is a versioned tree describing a product:
-- one `product` root, `module`/`part` nodes nested under it, and `aspect` nodes
-- attached to the root. Nodes are app-local; their `object_id`/`concept_id`/
-- `term_id` only *reference* governed rows (kb.object_nodes, kb.keyword_concepts
-- scope metric_subject, kb.ontology_terms) and carry no FK so a merge or delete
-- upstream never cascades into a curated profile.
--
-- Requires the "vector" extension (installed by 20260603000001); a node's label
-- embedding is stored once at grounding time for reuse across runs. Node budgets
-- are small (a couple hundred per profile), so nodes are loaded wholesale and
-- need no ANN index on the embedding.

CREATE SCHEMA IF NOT EXISTS kb;
CREATE EXTENSION IF NOT EXISTS vector;

CREATE TABLE IF NOT EXISTS kb.product_profiles (
    id                  BIGSERIAL    PRIMARY KEY,
    tenant_id           VARCHAR(128) NOT NULL DEFAULT '-',
    name                TEXT         NOT NULL,
    product_description  TEXT         NOT NULL DEFAULT '',
    version             INT          NOT NULL DEFAULT 1,
    status              TEXT         NOT NULL DEFAULT 'draft'
                                     CHECK (status IN ('draft', 'ready')),
    truncated           BOOLEAN      NOT NULL DEFAULT FALSE,
    truncated_count     INT          NOT NULL DEFAULT 0,
    created_at          TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_kb_product_profiles_tenant
    ON kb.product_profiles (tenant_id, updated_at DESC);

CREATE TABLE IF NOT EXISTS kb.product_profile_nodes (
    id                BIGSERIAL   PRIMARY KEY,
    profile_id        BIGINT      NOT NULL REFERENCES kb.product_profiles(id) ON DELETE CASCADE,
    parent_node_id    BIGINT      REFERENCES kb.product_profile_nodes(id) ON DELETE CASCADE,
    node_kind         TEXT        NOT NULL
                                  CHECK (node_kind IN ('product', 'module', 'part', 'aspect')),
    label             TEXT        NOT NULL DEFAULT '',
    label_en          TEXT        NOT NULL DEFAULT '',
    aliases           JSONB       NOT NULL DEFAULT '[]'::jsonb,
    depth             INT         NOT NULL DEFAULT 0,
    origin            TEXT        NOT NULL
                                  CHECK (origin IN ('llm_proposed', 'graph_expanded', 'user_added')),
    status            TEXT        NOT NULL DEFAULT 'proposed'
                                  CHECK (status IN ('proposed', 'accepted', 'rejected')),
    confidence        DOUBLE PRECISION NOT NULL DEFAULT 0,
    rationale         TEXT        NOT NULL DEFAULT '',
    object_id         TEXT,
    concept_id        TEXT,
    term_id           TEXT,
    grounding         TEXT        NOT NULL DEFAULT 'ungrounded'
                                  CHECK (grounding IN ('object_node', 'keyword_concept',
                                                       'ontology_term', 'ungrounded')),
    reconcile_status  TEXT        NOT NULL DEFAULT '',
    aspect_key        TEXT        NOT NULL DEFAULT '',
    relation_types    JSONB       NOT NULL DEFAULT '[]'::jsonb,
    match_mode        TEXT        NOT NULL DEFAULT '',
    embedding         vector(1536),
    source_refs       JSONB       NOT NULL DEFAULT '[]'::jsonb,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_kb_product_profile_nodes_profile
    ON kb.product_profile_nodes (profile_id);
CREATE INDEX IF NOT EXISTS idx_kb_product_profile_nodes_parent
    ON kb.product_profile_nodes (profile_id, parent_node_id);
CREATE INDEX IF NOT EXISTS idx_kb_product_profile_nodes_kind
    ON kb.product_profile_nodes (profile_id, node_kind);
CREATE INDEX IF NOT EXISTS idx_kb_product_profile_nodes_object
    ON kb.product_profile_nodes (object_id) WHERE object_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_kb_product_profile_nodes_concept
    ON kb.product_profile_nodes (concept_id) WHERE concept_id IS NOT NULL;

-- +goose Down
DROP TABLE IF EXISTS kb.product_profile_nodes;
DROP TABLE IF EXISTS kb.product_profiles;
-- NOTE: the "vector" extension is shared infrastructure; not dropped here.
