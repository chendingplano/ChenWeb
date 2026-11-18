-- +goose Up
ALTER TABLE IF EXISTS kb.deployment_builds ALTER COLUMN build_number DROP NOT NULL;

-- +goose Down
UPDATE kb.deployment_builds SET build_number = 'unassigned-' || id WHERE build_number IS NULL;
ALTER TABLE IF EXISTS kb.deployment_builds ALTER COLUMN build_number SET NOT NULL;
