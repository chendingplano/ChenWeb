-- +goose Up
CREATE SCHEMA IF NOT EXISTS kb;

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION kb.search_jsonb_array_text(arr JSONB)
RETURNS TEXT
LANGUAGE SQL
IMMUTABLE
AS $$
    SELECT CASE jsonb_typeof(COALESCE(arr, 'null'::jsonb))
        WHEN 'array' THEN (
            SELECT COALESCE(string_agg(value, ' '), '')
            FROM jsonb_array_elements_text(arr) AS t(value)
        )
        WHEN 'string' THEN trim(both '"' from arr::text)
        WHEN 'number' THEN arr::text
        WHEN 'boolean' THEN arr::text
        ELSE ''
    END;
$$;
-- +goose StatementEnd

CREATE TABLE IF NOT EXISTS kb.search_artifacts (
    artifact_type      TEXT        NOT NULL,
    artifact_id        TEXT        NOT NULL,
    input_record_id    BIGINT      NOT NULL REFERENCES kb.inputs(id) ON DELETE CASCADE,
    source_row_id      BIGINT,
    primary_label      TEXT        NOT NULL DEFAULT '',
    secondary_label    TEXT        NOT NULL DEFAULT '',
    search_document    TEXT        NOT NULL DEFAULT '',
    search_vector      TSVECTOR    NOT NULL DEFAULT ''::tsvector,
    snippet_basis      TEXT        NOT NULL DEFAULT '',
    source_title       TEXT        NOT NULL DEFAULT '',
    source_filename    TEXT        NOT NULL DEFAULT '',
    category_paths     JSONB       NOT NULL DEFAULT '[]'::jsonb,
    source_line_spans  JSONB       NOT NULL DEFAULT '[]'::jsonb,
    semantic_payload   JSONB       NOT NULL DEFAULT '{}'::jsonb,
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (artifact_type, artifact_id)
) PARTITION BY LIST (artifact_type);

CREATE TABLE IF NOT EXISTS kb.search_artifacts_summary PARTITION OF kb.search_artifacts
    FOR VALUES IN ('summary');
CREATE TABLE IF NOT EXISTS kb.search_artifacts_topic PARTITION OF kb.search_artifacts
    FOR VALUES IN ('topic');
CREATE TABLE IF NOT EXISTS kb.search_artifacts_scene_block PARTITION OF kb.search_artifacts
    FOR VALUES IN ('scene_block');
CREATE TABLE IF NOT EXISTS kb.search_artifacts_metric PARTITION OF kb.search_artifacts
    FOR VALUES IN ('metric');
CREATE TABLE IF NOT EXISTS kb.search_artifacts_provision PARTITION OF kb.search_artifacts
    FOR VALUES IN ('provision');
CREATE TABLE IF NOT EXISTS kb.search_artifacts_product PARTITION OF kb.search_artifacts
    FOR VALUES IN ('product');

CREATE INDEX IF NOT EXISTS idx_kb_search_artifacts_summary_search_vector
    ON kb.search_artifacts_summary USING GIN (search_vector);
CREATE INDEX IF NOT EXISTS idx_kb_search_artifacts_summary_record
    ON kb.search_artifacts_summary (input_record_id);

CREATE INDEX IF NOT EXISTS idx_kb_search_artifacts_topic_search_vector
    ON kb.search_artifacts_topic USING GIN (search_vector);
CREATE INDEX IF NOT EXISTS idx_kb_search_artifacts_topic_record
    ON kb.search_artifacts_topic (input_record_id);

CREATE INDEX IF NOT EXISTS idx_kb_search_artifacts_scene_block_search_vector
    ON kb.search_artifacts_scene_block USING GIN (search_vector);
CREATE INDEX IF NOT EXISTS idx_kb_search_artifacts_scene_block_record
    ON kb.search_artifacts_scene_block (input_record_id);

CREATE INDEX IF NOT EXISTS idx_kb_search_artifacts_metric_search_vector
    ON kb.search_artifacts_metric USING GIN (search_vector);
CREATE INDEX IF NOT EXISTS idx_kb_search_artifacts_metric_record
    ON kb.search_artifacts_metric (input_record_id);

CREATE INDEX IF NOT EXISTS idx_kb_search_artifacts_provision_search_vector
    ON kb.search_artifacts_provision USING GIN (search_vector);
CREATE INDEX IF NOT EXISTS idx_kb_search_artifacts_provision_record
    ON kb.search_artifacts_provision (input_record_id);

CREATE INDEX IF NOT EXISTS idx_kb_search_artifacts_product_search_vector
    ON kb.search_artifacts_product USING GIN (search_vector);
CREATE INDEX IF NOT EXISTS idx_kb_search_artifacts_product_record
    ON kb.search_artifacts_product (input_record_id);

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION kb.product_search_document(
    product_name TEXT,
    product_name_en TEXT,
    canonical_name TEXT,
    canonical_name_en TEXT,
    product_type TEXT,
    relation_type TEXT,
    relation_summary TEXT,
    relation_summary_en TEXT,
    evidence_quote TEXT,
    obligation_level TEXT,
    requirement_text TEXT,
    requirement_text_en TEXT,
    confidence_reason TEXT,
    confidence_reason_en TEXT,
    category_paths JSONB,
    category_paths_en JSONB
)
RETURNS TEXT
LANGUAGE SQL
IMMUTABLE
AS $$
    SELECT trim(concat_ws(
        ' ',
        COALESCE(product_name, ''),
        COALESCE(product_name_en, ''),
        COALESCE(canonical_name, ''),
        COALESCE(canonical_name_en, ''),
        COALESCE(product_type, ''),
        COALESCE(relation_type, ''),
        COALESCE(relation_summary, ''),
        COALESCE(relation_summary_en, ''),
        COALESCE(evidence_quote, ''),
        COALESCE(obligation_level, ''),
        COALESCE(requirement_text, ''),
        COALESCE(requirement_text_en, ''),
        COALESCE(confidence_reason, ''),
        COALESCE(confidence_reason_en, ''),
        kb.search_jsonb_array_text(category_paths),
        kb.search_jsonb_array_text(category_paths_en)
    ));
$$;
-- +goose StatementEnd

ALTER TABLE kb.products
    ADD COLUMN IF NOT EXISTS search_document TEXT,
    ADD COLUMN IF NOT EXISTS search_vector TSVECTOR;

UPDATE kb.products
SET search_document = kb.product_search_document(
        product_name, product_name_en,
        canonical_name, canonical_name_en,
        product_type, relation_type,
        relation_summary, relation_summary_en,
        evidence_quote, obligation_level,
        requirement_text, requirement_text_en,
        confidence_reason, confidence_reason_en,
        category_paths, category_paths_en
    ),
    search_vector = to_tsvector('simple', kb.product_search_document(
        product_name, product_name_en,
        canonical_name, canonical_name_en,
        product_type, relation_type,
        relation_summary, relation_summary_en,
        evidence_quote, obligation_level,
        requirement_text, requirement_text_en,
        confidence_reason, confidence_reason_en,
        category_paths, category_paths_en
    ));

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION kb.refresh_product_search_columns()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    NEW.search_document := kb.product_search_document(
        NEW.product_name, NEW.product_name_en,
        NEW.canonical_name, NEW.canonical_name_en,
        NEW.product_type, NEW.relation_type,
        NEW.relation_summary, NEW.relation_summary_en,
        NEW.evidence_quote, NEW.obligation_level,
        NEW.requirement_text, NEW.requirement_text_en,
        NEW.confidence_reason, NEW.confidence_reason_en,
        NEW.category_paths, NEW.category_paths_en
    );
    NEW.search_vector := to_tsvector('simple', COALESCE(NEW.search_document, ''));
    RETURN NEW;
END;
$$;
-- +goose StatementEnd

DROP TRIGGER IF EXISTS trg_refresh_product_search_columns ON kb.products;
CREATE TRIGGER trg_refresh_product_search_columns
BEFORE INSERT OR UPDATE OF
    product_name, product_name_en,
    canonical_name, canonical_name_en,
    product_type, relation_type,
    relation_summary, relation_summary_en,
    evidence_quote, obligation_level,
    requirement_text, requirement_text_en,
    confidence_reason, confidence_reason_en,
    category_paths, category_paths_en
ON kb.products
FOR EACH ROW
EXECUTE FUNCTION kb.refresh_product_search_columns();

CREATE INDEX IF NOT EXISTS idx_kb_products_search_vector
    ON kb.products USING GIN (search_vector);

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION kb.provision_search_document(
    prov_name TEXT,
    prov_name_en TEXT,
    provision_type TEXT,
    source_text TEXT,
    provision TEXT,
    provision_en TEXT,
    provision_subject TEXT,
    provision_subject_en TEXT,
    prov_desc TEXT,
    prov_desc_en TEXT,
    prov_context TEXT,
    prov_context_en TEXT,
    provision_keywords JSONB,
    provision_keywords_en JSONB,
    category_paths JSONB,
    category_paths_en JSONB
)
RETURNS TEXT
LANGUAGE SQL
IMMUTABLE
AS $$
    SELECT trim(concat_ws(
        ' ',
        COALESCE(prov_name, ''),
        COALESCE(prov_name_en, ''),
        COALESCE(provision_type, ''),
        COALESCE(source_text, ''),
        COALESCE(provision, ''),
        COALESCE(provision_en, ''),
        COALESCE(provision_subject, ''),
        COALESCE(provision_subject_en, ''),
        COALESCE(prov_desc, ''),
        COALESCE(prov_desc_en, ''),
        COALESCE(prov_context, ''),
        COALESCE(prov_context_en, ''),
        kb.search_jsonb_array_text(provision_keywords),
        kb.search_jsonb_array_text(provision_keywords_en),
        kb.search_jsonb_array_text(category_paths),
        kb.search_jsonb_array_text(category_paths_en)
    ));
$$;
-- +goose StatementEnd

ALTER TABLE kb.provisions
    ADD COLUMN IF NOT EXISTS search_document TEXT,
    ADD COLUMN IF NOT EXISTS search_vector TSVECTOR;

UPDATE kb.provisions
SET search_document = kb.provision_search_document(
        prov_name, prov_name_en,
        provision_type, source_text,
        provision, provision_en,
        provision_subject, provision_subject_en,
        prov_desc, prov_desc_en,
        prov_context, prov_context_en,
        provision_keywords, provision_keywords_en,
        category_paths, category_paths_en
    ),
    search_vector = to_tsvector('simple', kb.provision_search_document(
        prov_name, prov_name_en,
        provision_type, source_text,
        provision, provision_en,
        provision_subject, provision_subject_en,
        prov_desc, prov_desc_en,
        prov_context, prov_context_en,
        provision_keywords, provision_keywords_en,
        category_paths, category_paths_en
    ));

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION kb.refresh_provision_search_columns()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    NEW.search_document := kb.provision_search_document(
        NEW.prov_name, NEW.prov_name_en,
        NEW.provision_type, NEW.source_text,
        NEW.provision, NEW.provision_en,
        NEW.provision_subject, NEW.provision_subject_en,
        NEW.prov_desc, NEW.prov_desc_en,
        NEW.prov_context, NEW.prov_context_en,
        NEW.provision_keywords, NEW.provision_keywords_en,
        NEW.category_paths, NEW.category_paths_en
    );
    NEW.search_vector := to_tsvector('simple', COALESCE(NEW.search_document, ''));
    RETURN NEW;
END;
$$;
-- +goose StatementEnd

DROP TRIGGER IF EXISTS trg_refresh_provision_search_columns ON kb.provisions;
CREATE TRIGGER trg_refresh_provision_search_columns
BEFORE INSERT OR UPDATE OF
    prov_name, prov_name_en,
    provision_type, source_text,
    provision, provision_en,
    provision_subject, provision_subject_en,
    prov_desc, prov_desc_en,
    prov_context, prov_context_en,
    provision_keywords, provision_keywords_en,
    category_paths, category_paths_en
ON kb.provisions
FOR EACH ROW
EXECUTE FUNCTION kb.refresh_provision_search_columns();

CREATE INDEX IF NOT EXISTS idx_kb_provisions_search_vector
    ON kb.provisions USING GIN (search_vector);

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION kb.scene_block_search_document(
    scene_id TEXT,
    scene_type TEXT,
    title TEXT,
    summary TEXT,
    actors JSONB,
    resources JSONB,
    preconditions JSONB,
    triggers JSONB,
    states JSONB,
    actions JSONB,
    constraints JSONB,
    decisions JSONB,
    outcomes JSONB,
    failure_modes JSONB,
    root_causes JSONB,
    resolutions JSONB,
    relationships JSONB,
    discriminators JSONB,
    keywords JSONB
)
RETURNS TEXT
LANGUAGE SQL
IMMUTABLE
AS $$
    SELECT trim(concat_ws(
        ' ',
        COALESCE(scene_id, ''),
        COALESCE(scene_type, ''),
        COALESCE(title, ''),
        COALESCE(summary, ''),
        kb.search_jsonb_array_text(actors),
        kb.search_jsonb_array_text(resources),
        kb.search_jsonb_array_text(preconditions),
        kb.search_jsonb_array_text(triggers),
        kb.search_jsonb_array_text(states),
        kb.search_jsonb_array_text(actions),
        kb.search_jsonb_array_text(constraints),
        kb.search_jsonb_array_text(decisions),
        kb.search_jsonb_array_text(outcomes),
        kb.search_jsonb_array_text(failure_modes),
        kb.search_jsonb_array_text(root_causes),
        kb.search_jsonb_array_text(resolutions),
        kb.search_jsonb_array_text(relationships),
        kb.search_jsonb_array_text(discriminators),
        kb.search_jsonb_array_text(keywords)
    ));
$$;
-- +goose StatementEnd

ALTER TABLE kb.scene_objects
    ADD COLUMN IF NOT EXISTS search_document TEXT,
    ADD COLUMN IF NOT EXISTS search_vector TSVECTOR;

UPDATE kb.scene_objects
SET search_document = kb.scene_block_search_document(
        scene_id, scene_type, title, summary,
        actors, resources, preconditions, triggers, states, actions,
        constraints, decisions, outcomes, failure_modes, root_causes,
        resolutions, relationships, discriminators, keywords
    ),
    search_vector = to_tsvector('simple', kb.scene_block_search_document(
        scene_id, scene_type, title, summary,
        actors, resources, preconditions, triggers, states, actions,
        constraints, decisions, outcomes, failure_modes, root_causes,
        resolutions, relationships, discriminators, keywords
    ));

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION kb.refresh_scene_block_search_columns()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    NEW.search_document := kb.scene_block_search_document(
        NEW.scene_id, NEW.scene_type, NEW.title, NEW.summary,
        NEW.actors, NEW.resources, NEW.preconditions, NEW.triggers, NEW.states, NEW.actions,
        NEW.constraints, NEW.decisions, NEW.outcomes, NEW.failure_modes, NEW.root_causes,
        NEW.resolutions, NEW.relationships, NEW.discriminators, NEW.keywords
    );
    NEW.search_vector := to_tsvector('simple', COALESCE(NEW.search_document, ''));
    RETURN NEW;
END;
$$;
-- +goose StatementEnd

DROP TRIGGER IF EXISTS trg_refresh_scene_block_search_columns ON kb.scene_objects;
CREATE TRIGGER trg_refresh_scene_block_search_columns
BEFORE INSERT OR UPDATE OF
    scene_id, scene_type, title, summary,
    actors, resources, preconditions, triggers, states, actions,
    constraints, decisions, outcomes, failure_modes, root_causes,
    resolutions, relationships, discriminators, keywords
ON kb.scene_objects
FOR EACH ROW
EXECUTE FUNCTION kb.refresh_scene_block_search_columns();

CREATE INDEX IF NOT EXISTS idx_kb_scene_objects_search_vector
    ON kb.scene_objects USING GIN (search_vector);

-- +goose Down
DROP INDEX IF EXISTS idx_kb_scene_objects_search_vector;
DROP TRIGGER IF EXISTS trg_refresh_scene_block_search_columns ON kb.scene_objects;
DROP FUNCTION IF EXISTS kb.refresh_scene_block_search_columns();
ALTER TABLE kb.scene_objects DROP COLUMN IF EXISTS search_vector, DROP COLUMN IF EXISTS search_document;
DROP FUNCTION IF EXISTS kb.scene_block_search_document(TEXT, TEXT, TEXT, TEXT, JSONB, JSONB, JSONB, JSONB, JSONB, JSONB, JSONB, JSONB, JSONB, JSONB, JSONB, JSONB, JSONB, JSONB, JSONB);

DROP INDEX IF EXISTS idx_kb_provisions_search_vector;
DROP TRIGGER IF EXISTS trg_refresh_provision_search_columns ON kb.provisions;
DROP FUNCTION IF EXISTS kb.refresh_provision_search_columns();
ALTER TABLE kb.provisions DROP COLUMN IF EXISTS search_vector, DROP COLUMN IF EXISTS search_document;
DROP FUNCTION IF EXISTS kb.provision_search_document(TEXT, TEXT, TEXT, TEXT, TEXT, TEXT, TEXT, TEXT, TEXT, TEXT, TEXT, TEXT, JSONB, JSONB, JSONB, JSONB);

DROP INDEX IF EXISTS idx_kb_products_search_vector;
DROP TRIGGER IF EXISTS trg_refresh_product_search_columns ON kb.products;
DROP FUNCTION IF EXISTS kb.refresh_product_search_columns();
ALTER TABLE kb.products DROP COLUMN IF EXISTS search_vector, DROP COLUMN IF EXISTS search_document;
DROP FUNCTION IF EXISTS kb.product_search_document(TEXT, TEXT, TEXT, TEXT, TEXT, TEXT, TEXT, TEXT, TEXT, TEXT, TEXT, TEXT, TEXT, TEXT, JSONB, JSONB);

DROP INDEX IF EXISTS idx_kb_search_artifacts_product_record;
DROP INDEX IF EXISTS idx_kb_search_artifacts_product_search_vector;
DROP INDEX IF EXISTS idx_kb_search_artifacts_provision_record;
DROP INDEX IF EXISTS idx_kb_search_artifacts_provision_search_vector;
DROP INDEX IF EXISTS idx_kb_search_artifacts_metric_record;
DROP INDEX IF EXISTS idx_kb_search_artifacts_metric_search_vector;
DROP INDEX IF EXISTS idx_kb_search_artifacts_scene_block_record;
DROP INDEX IF EXISTS idx_kb_search_artifacts_scene_block_search_vector;
DROP INDEX IF EXISTS idx_kb_search_artifacts_topic_record;
DROP INDEX IF EXISTS idx_kb_search_artifacts_topic_search_vector;
DROP INDEX IF EXISTS idx_kb_search_artifacts_summary_record;
DROP INDEX IF EXISTS idx_kb_search_artifacts_summary_search_vector;
DROP TABLE IF EXISTS kb.search_artifacts_product;
DROP TABLE IF EXISTS kb.search_artifacts_provision;
DROP TABLE IF EXISTS kb.search_artifacts_metric;
DROP TABLE IF EXISTS kb.search_artifacts_scene_block;
DROP TABLE IF EXISTS kb.search_artifacts_topic;
DROP TABLE IF EXISTS kb.search_artifacts_summary;
DROP TABLE IF EXISTS kb.search_artifacts;
DROP FUNCTION IF EXISTS kb.search_jsonb_array_text(JSONB);
