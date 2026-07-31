-- +goose Up
-- Governed ontology axioms, versioned. axiom_kind is restricted to
-- compiler-approved kinds in chunk B's module compiler; the column itself is
-- left open here so the compiler owns the approved-kinds vocabulary.
-- Subject/predicate/object reference governed terms by term_id; object_iri is
-- the external-target alternative when the object is not a governed term.
CREATE TABLE IF NOT EXISTS kb.ontology_axioms (
    id              BIGSERIAL PRIMARY KEY,
    axiom_id        TEXT NOT NULL,
    version         INT NOT NULL DEFAULT 1,
    axiom_kind      TEXT NOT NULL,
    subject_term_id TEXT NOT NULL,
    predicate_term_id TEXT,
    object_term_id  TEXT,
    object_iri      TEXT,
    module_id       TEXT NOT NULL,
    status          TEXT NOT NULL DEFAULT 'draft' CHECK (status IN (
                        'draft', 'in_review', 'approved', 'included_in_release',
                        'superseded', 'rejected'
                    )),
    create_time     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    create_by       TEXT,
    modify_time     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    modify_by       TEXT,
    CHECK (object_term_id IS NOT NULL OR object_iri IS NOT NULL)
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_kb_ontology_axioms_id_version
    ON kb.ontology_axioms (axiom_id, version);
CREATE INDEX IF NOT EXISTS idx_kb_ontology_axioms_module_status
    ON kb.ontology_axioms (module_id, status);
CREATE INDEX IF NOT EXISTS idx_kb_ontology_axioms_subject
    ON kb.ontology_axioms (subject_term_id);

-- +goose Down
DROP TABLE IF EXISTS kb.ontology_axioms;
