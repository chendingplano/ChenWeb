-- +goose Up
-- +goose StatementBegin
-- The button class's label carries value_editor localized-text (set by
-- 20261102000002), and class metadata overrides the code default in elements.ts.
-- Give the button class a new version naming string-editor, as
-- 20261104000004 does for the value-editor classes. Instances placed before this
-- keep their own class copies until they are placed again.
INSERT INTO kb.wpi_descriptors
    (kind, descriptor_id, version, schema_version, body, status, created_by, updated_by)
SELECT kind, descriptor_id,
       (SELECT MAX(version) FROM kb.wpi_descriptors x
        WHERE x.kind = latest.kind AND x.descriptor_id = latest.descriptor_id) + 1,
       schema_version,
       jsonb_set(body, '{label,value_editor}',
                 '{"id":"string-editor","options":{"languages":["en","zh-cn"]}}'::jsonb),
       status, 'migration:wpi-button-label-string-editor', 'migration:wpi-button-label-string-editor'
FROM (
    SELECT DISTINCT ON (descriptor_id) *
    FROM kb.wpi_descriptors
    WHERE kind = 'element' AND status IN ('draft', 'active')
    ORDER BY descriptor_id, version DESC
) AS latest
WHERE latest.descriptor_id = 'button'
  AND body ? 'label'
  AND body->'label'->'value_editor'->>'id' IS DISTINCT FROM 'string-editor';
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DELETE FROM kb.wpi_descriptors
WHERE kind = 'element'
  AND created_by = 'migration:wpi-button-label-string-editor';
-- +goose StatementEnd
