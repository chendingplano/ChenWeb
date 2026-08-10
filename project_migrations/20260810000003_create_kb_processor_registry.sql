-- +goose Up
-- ADR 2026081001 DR7: incremental adoption of processor dependency metadata.
-- Existing processors stay hardcoded in productionProcessorSpecs untouched;
-- only new processors, going forward, get a row here instead of a new line
-- in the Go literal.
CREATE TABLE IF NOT EXISTS kb.processor_registry (
    name             VARCHAR(128) PRIMARY KEY,
    phase            VARCHAR(1) NOT NULL,          -- A | B | C
    class            VARCHAR(16) NOT NULL,         -- mandatory | routed | on_demand
    cost             VARCHAR(16) NOT NULL,         -- free | cheap_llm | expensive_llm
    on_undetermined  VARCHAR(8) NOT NULL DEFAULT 'run',
    idempotent       BOOLEAN NOT NULL DEFAULT false,
    requires         TEXT[] NOT NULL DEFAULT '{}', -- artifact kinds needed as input
    produces         TEXT[] NOT NULL DEFAULT '{}', -- artifact kinds emitted
    create_time      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    modify_time      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- +goose Down
DROP TABLE IF EXISTS kb.processor_registry;
