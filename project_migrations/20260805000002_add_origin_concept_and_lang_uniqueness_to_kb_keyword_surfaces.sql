-- +goose Up
-- Spec 2026080403 §19 step 6:
--  - §14.1/§14.4: origin_concept records the concept a surface was re-pointed
--    FROM when a merge moved it. This is what makes an un-merge
--    reconstructible — without it a merge is irreversible (D11).
--  - §10.2 item 5: the uniqueness key omitted lang, so one concept could not
--    hold a pref surface per language. The key gains lang.

ALTER TABLE kb.keyword_surfaces
    ADD COLUMN IF NOT EXISTS origin_concept TEXT
    REFERENCES kb.keyword_concepts(concept_id);

CREATE INDEX IF NOT EXISTS idx_keyword_surfaces_origin_concept
    ON kb.keyword_surfaces (origin_concept);

DROP INDEX IF EXISTS kb.uq_keyword_surfaces_norm_concept_scope_role;

CREATE UNIQUE INDEX IF NOT EXISTS uq_keyword_surfaces_norm_concept_scope_role_lang
    ON kb.keyword_surfaces (norm_key, concept_id, scope, label_role, lang);

-- +goose Down
DROP INDEX IF EXISTS kb.uq_keyword_surfaces_norm_concept_scope_role_lang;

CREATE UNIQUE INDEX IF NOT EXISTS uq_keyword_surfaces_norm_concept_scope_role
    ON kb.keyword_surfaces (norm_key, concept_id, scope, label_role);

DROP INDEX IF EXISTS kb.idx_keyword_surfaces_origin_concept;

ALTER TABLE kb.keyword_surfaces
    DROP COLUMN IF EXISTS origin_concept;
