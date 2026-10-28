-- WPI element class rows in format 0.4 (OpenSpec change wpi-code-modules; devdoc
-- 2026102601 §3.5.2). An element class now names the code that draws it, its
-- code module, in code_module_id, so several classes can share one module.
--
-- For every element class whose latest draft or active row is still format 0.3,
-- this stores the next version with the same status and body, schema_version
-- 0.4 and, for an active row, code_module_id equal to its class ID (until now a
-- class ID was also the name of its code). Rows are transformed, not seeded
-- again, so attributes authors added through the WPI editor are kept; one the
-- module does not support is now a warning. A draft gets no code_module_id: it
-- may not have code yet. Running it again stores nothing: the latest rows are
-- 0.4 by then, and a concurrent run hits the (kind, descriptor_id, version) key.
-- server/api/wpi/seed_test.go applies the same transform to the seeded classes.

-- +goose Up
-- +goose StatementBegin
INSERT INTO kb.wpi_descriptors (kind, descriptor_id, version, schema_version, body, status, created_by, updated_by)
SELECT 'element', d.descriptor_id,
       (SELECT MAX(w.version) FROM kb.wpi_descriptors w WHERE w.kind = 'element' AND w.descriptor_id = d.descriptor_id) + 1,
       '0.4',
       d.body || jsonb_build_object('schema_version', '0.4')
              || CASE WHEN d.status = 'active' THEN jsonb_build_object('code_module_id', d.descriptor_id) ELSE '{}'::jsonb END,
       d.status, 'migration:wpi-code-modules', 'migration:wpi-code-modules'
FROM (
    SELECT DISTINCT ON (descriptor_id) descriptor_id, schema_version, body, status
    FROM kb.wpi_descriptors
    WHERE kind = 'element' AND status IN ('draft', 'active')
    ORDER BY descriptor_id, version DESC
) d
WHERE d.schema_version = '0.3'
ON CONFLICT (kind, descriptor_id, version) DO NOTHING;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DELETE FROM kb.wpi_descriptors
WHERE kind = 'element' AND created_by = 'migration:wpi-code-modules';
-- +goose StatementEnd
