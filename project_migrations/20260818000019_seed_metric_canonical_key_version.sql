-- +goose Up
-- ADR 2026081801 Phase 3 task 6.5: kb.semantic_claim_identities.key_version
-- is a foreign key into kb.semantic_canonical_key_versions, so the metric
-- lossless writer's canonical claim registration has no row to reference
-- until one is seeded. "identity/v1" is the serializer this writer and the
-- shadow-mode foundation comparison (metric_foundation_shadow.go) already
-- use; registering it active is what lets a real (non-shadow) claim commit.
INSERT INTO kb.semantic_canonical_key_versions (key_version, serializer_name, serializer_version, status, definition, create_by)
VALUES ('identity/v1', 'metric_canonical_claim_key', '1', 'active', '{}'::jsonb, 'metric_lossless_writer_migration')
ON CONFLICT (key_version) DO NOTHING;

-- +goose Down
DELETE FROM kb.semantic_canonical_key_versions WHERE key_version = 'identity/v1';
