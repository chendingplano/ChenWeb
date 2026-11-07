-- +goose Up
-- Move the table and its owned sequence and indexes without changing score history.
ALTER TABLE IF EXISTS kb.provision_score_runs SET SCHEMA testbed;

-- +goose Down
ALTER TABLE IF EXISTS testbed.provision_score_runs SET SCHEMA kb;
