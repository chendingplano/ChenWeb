-- +goose Up
INSERT INTO kb.wpi_descriptors (kind, descriptor_id, version, schema_version, body, status, created_by, updated_by)
SELECT 'element-layouter', 'flow-layouter',
       (SELECT COALESCE(MAX(version), 0) + 1 FROM kb.wpi_descriptors WHERE kind = 'element-layouter' AND descriptor_id = 'flow-layouter'),
       schema_version,
       jsonb_set(body, '{config}', COALESCE(body->'config', '{}'::jsonb) ||
         '{"min_row_height":"30px","max_row_height":"200px","min_elem_width":"20px","max_elem_width":"300px","max_num_elems":50,"row_separator":"none","elem_separator":"none"}'::jsonb),
       'active', 'migration:flow-constraints', 'migration:flow-constraints'
FROM kb.wpi_descriptors
WHERE kind = 'element-layouter' AND descriptor_id = 'flow-layouter' AND status = 'active'
ORDER BY version DESC LIMIT 1;

-- +goose Down
DELETE FROM kb.wpi_descriptors WHERE kind = 'element-layouter' AND descriptor_id = 'flow-layouter' AND created_by = 'migration:flow-constraints';
