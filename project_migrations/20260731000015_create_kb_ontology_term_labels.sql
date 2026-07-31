-- +goose Up
-- Language-tagged labels on governed terms, versioned like their term.
-- label_role follows the SKOS label discipline adopted in DR13:
-- prefLabel / altLabel / hiddenLabel. The "one released prefLabel per
-- configured language" rule is enforced at write time in Go (it cannot be a
-- partial unique index because multiple versions of the same term coexist).
CREATE TABLE IF NOT EXISTS kb.ontology_term_labels (
    id              BIGSERIAL PRIMARY KEY,
    term_id         TEXT NOT NULL,
    version         INT NOT NULL DEFAULT 1,
    label           TEXT NOT NULL,
    lang            TEXT NOT NULL DEFAULT 'en',
    label_role      TEXT NOT NULL DEFAULT 'prefLabel' CHECK (label_role IN (
                        'prefLabel', 'altLabel', 'hiddenLabel'
                    )),
    status          TEXT NOT NULL DEFAULT 'draft' CHECK (status IN (
                        'draft', 'in_review', 'approved', 'included_in_release',
                        'superseded', 'rejected'
                    )),
    create_time     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    create_by       TEXT,
    modify_time     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    modify_by       TEXT
);

CREATE INDEX IF NOT EXISTS idx_kb_ontology_term_labels_term_version
    ON kb.ontology_term_labels (term_id, version);
CREATE INDEX IF NOT EXISTS idx_kb_ontology_term_labels_term_lang
    ON kb.ontology_term_labels (term_id, lang);

-- +goose Down
DROP TABLE IF EXISTS kb.ontology_term_labels;
