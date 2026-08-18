-- +goose Up
-- ADR 2026081701 DR9: redirects preserve a history of replacements while only
-- one target is active for any source. Recursive guards reject an active edge
-- that would make its source reachable from itself.

CREATE TABLE IF NOT EXISTS kb.ontology_term_redirects (
    id                          BIGSERIAL PRIMARY KEY,
    source_term_id              TEXT NOT NULL REFERENCES kb.ontology_term_headers (term_id),
    target_term_id              TEXT NOT NULL REFERENCES kb.ontology_term_headers (term_id),
    active                      BOOLEAN NOT NULL DEFAULT TRUE,
    superseded_by_redirect_id   BIGINT REFERENCES kb.ontology_term_redirects (id),
    reason                      TEXT NOT NULL,
    evidence                    JSONB NOT NULL DEFAULT '{}'::jsonb,
    create_time                 TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    create_by                   TEXT,
    CHECK (source_term_id <> target_term_id)
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_kb_ontology_term_redirects_active_source
    ON kb.ontology_term_redirects (source_term_id)
    WHERE active;

CREATE TABLE IF NOT EXISTS kb.semantic_assertion_redirects (
    id                          BIGSERIAL PRIMARY KEY,
    source_assertion_id         BIGINT NOT NULL REFERENCES kb.semantic_assertions (id),
    target_assertion_id         BIGINT NOT NULL REFERENCES kb.semantic_assertions (id),
    active                      BOOLEAN NOT NULL DEFAULT TRUE,
    superseded_by_redirect_id   BIGINT REFERENCES kb.semantic_assertion_redirects (id),
    reason                      TEXT NOT NULL,
    evidence                    JSONB NOT NULL DEFAULT '{}'::jsonb,
    create_time                 TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    create_by                   TEXT,
    CHECK (source_assertion_id <> target_assertion_id)
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_kb_semantic_assertion_redirects_active_source
    ON kb.semantic_assertion_redirects (source_assertion_id)
    WHERE active;

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION kb.guard_term_redirect_cycle()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
BEGIN
    IF NOT NEW.active THEN
        RETURN NEW;
    END IF;
    IF EXISTS (
        WITH RECURSIVE reachable(term_id, path) AS (
            SELECT target_term_id, ARRAY[source_term_id, target_term_id]
            FROM kb.ontology_term_redirects
            WHERE source_term_id = NEW.target_term_id AND active
            UNION ALL
            SELECT redirect.target_term_id, reachable.path || redirect.target_term_id
            FROM kb.ontology_term_redirects AS redirect
            JOIN reachable ON redirect.source_term_id = reachable.term_id
            WHERE redirect.active AND NOT redirect.target_term_id = ANY(reachable.path)
        )
        SELECT 1 FROM reachable WHERE term_id = NEW.source_term_id
    ) THEN
        RAISE EXCEPTION 'term redirect cycle detected';
    END IF;
    RETURN NEW;
END;
$$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION kb.guard_assertion_redirect_cycle()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
BEGIN
    IF NOT NEW.active THEN
        RETURN NEW;
    END IF;
    IF EXISTS (
        WITH RECURSIVE reachable(assertion_id, path) AS (
            SELECT target_assertion_id, ARRAY[source_assertion_id, target_assertion_id]
            FROM kb.semantic_assertion_redirects
            WHERE source_assertion_id = NEW.target_assertion_id AND active
            UNION ALL
            SELECT redirect.target_assertion_id, reachable.path || redirect.target_assertion_id
            FROM kb.semantic_assertion_redirects AS redirect
            JOIN reachable ON redirect.source_assertion_id = reachable.assertion_id
            WHERE redirect.active AND NOT redirect.target_assertion_id = ANY(reachable.path)
        )
        SELECT 1 FROM reachable WHERE assertion_id = NEW.source_assertion_id
    ) THEN
        RAISE EXCEPTION 'assertion redirect cycle detected';
    END IF;
    RETURN NEW;
END;
$$;
-- +goose StatementEnd

CREATE TRIGGER kb_term_redirect_cycle_guard
BEFORE INSERT OR UPDATE OF source_term_id, target_term_id, active ON kb.ontology_term_redirects
FOR EACH ROW EXECUTE FUNCTION kb.guard_term_redirect_cycle();

CREATE TRIGGER kb_assertion_redirect_cycle_guard
BEFORE INSERT OR UPDATE OF source_assertion_id, target_assertion_id, active ON kb.semantic_assertion_redirects
FOR EACH ROW EXECUTE FUNCTION kb.guard_assertion_redirect_cycle();

-- +goose Down
DROP TRIGGER IF EXISTS kb_assertion_redirect_cycle_guard ON kb.semantic_assertion_redirects;
DROP TRIGGER IF EXISTS kb_term_redirect_cycle_guard ON kb.ontology_term_redirects;
DROP FUNCTION IF EXISTS kb.guard_assertion_redirect_cycle();
DROP FUNCTION IF EXISTS kb.guard_term_redirect_cycle();
DROP TABLE IF EXISTS kb.semantic_assertion_redirects;
DROP TABLE IF EXISTS kb.ontology_term_redirects;
