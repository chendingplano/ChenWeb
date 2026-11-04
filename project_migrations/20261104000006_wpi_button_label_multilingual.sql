-- +goose Up
-- +goose StatementBegin
-- A plain-string label always shows the string editor in Single mode, so the
-- button's label default becomes a language map, and its string editor opens in
-- Multilingual, matching the value-editor labels (20261104000004). Each new
-- version copies the latest active or draft button class.
INSERT INTO kb.wpi_descriptors
    (kind, descriptor_id, version, schema_version, body, status, created_by, updated_by)
SELECT kind, descriptor_id,
       (SELECT MAX(version) FROM kb.wpi_descriptors x
        WHERE x.kind = latest.kind AND x.descriptor_id = latest.descriptor_id) + 1,
       schema_version,
       jsonb_set(
         jsonb_set(body, '{label,value_editor}',
                   '{"id":"string-editor","options":{"mode":"multilingual","languages":["en","zh-cn"]}}'::jsonb),
         '{label,default_value}', '{"en":"Button","zh-cn":"按钮"}'::jsonb),
       status, 'migration:wpi-button-label-multilingual', 'migration:wpi-button-label-multilingual'
FROM (
    SELECT DISTINCT ON (descriptor_id) *
    FROM kb.wpi_descriptors
    WHERE kind = 'element' AND status IN ('draft', 'active')
    ORDER BY descriptor_id, version DESC
) AS latest
WHERE latest.descriptor_id = 'button';
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DELETE FROM kb.wpi_descriptors
WHERE kind = 'element'
  AND created_by = 'migration:wpi-button-label-multilingual';
-- +goose StatementEnd
