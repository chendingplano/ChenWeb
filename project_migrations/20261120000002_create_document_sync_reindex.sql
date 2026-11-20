-- +goose Up
CREATE TABLE IF NOT EXISTS kb.doc_sync_reindex (
    input_id BIGINT NOT NULL REFERENCES kb.inputs(id) ON DELETE CASCADE,
    processor TEXT NOT NULL,
    PRIMARY KEY (input_id, processor)
);

-- +goose Down
DROP TABLE IF EXISTS kb.doc_sync_reindex;
