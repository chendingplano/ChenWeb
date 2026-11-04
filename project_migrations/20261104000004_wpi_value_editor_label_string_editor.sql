-- +goose Up
-- +goose StatementBegin
-- The Properties panel edits a value-editor element's label with the string editor,
-- multilingual by default (interpreter-value-editor.md §9). The class copy's
-- value_editor wins over the code default, so every value-editor class gets a new
-- version naming it; instances placed before this keep their own copies.
INSERT INTO kb.wpi_descriptors
    (kind, descriptor_id, version, schema_version, body, status, created_by, updated_by)
SELECT kind, descriptor_id,
       (SELECT MAX(version) FROM kb.wpi_descriptors x
        WHERE x.kind = latest.kind AND x.descriptor_id = latest.descriptor_id) + 1,
       schema_version,
       jsonb_set(body, '{label,value_editor}',
                 '{"id":"string-editor","options":{"mode":"multilingual","languages":["en","zh-cn"]}}'::jsonb),
       status, 'migration:wpi-value-editor-label-string-editor', 'migration:wpi-value-editor-label-string-editor'
FROM (
    SELECT DISTINCT ON (descriptor_id) *
    FROM kb.wpi_descriptors
    WHERE kind = 'element' AND status IN ('draft', 'active')
    ORDER BY descriptor_id, version DESC
) AS latest
WHERE body->>'code_module_id' = 'value-editor'
  AND body ? 'label'
  AND body->'label'->'value_editor'->>'id' IS DISTINCT FROM 'string-editor';
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DELETE FROM kb.wpi_descriptors
WHERE kind = 'element'
  AND created_by = 'migration:wpi-value-editor-label-string-editor';
-- +goose StatementEnd
