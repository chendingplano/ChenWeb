-- +goose Up
-- Change prov_id from INTEGER to TEXT to support the standard artifact ID format
-- <record_id>_prv_<seqno>. Existing data is wiped before this migration runs.

ALTER TABLE kb.provisions DROP CONSTRAINT IF EXISTS uq_kb_provisions_input_prov_id;

ALTER TABLE kb.provisions ALTER COLUMN prov_id TYPE TEXT USING prov_id::text;

ALTER TABLE kb.provisions ADD CONSTRAINT uq_kb_provisions_input_prov_id UNIQUE (input_record_id, prov_id);

-- +goose Down
ALTER TABLE kb.provisions DROP CONSTRAINT IF EXISTS uq_kb_provisions_input_prov_id;

ALTER TABLE kb.provisions ALTER COLUMN prov_id TYPE INTEGER USING prov_id::integer;

ALTER TABLE kb.provisions ADD CONSTRAINT uq_kb_provisions_input_prov_id UNIQUE (input_record_id, prov_id);
