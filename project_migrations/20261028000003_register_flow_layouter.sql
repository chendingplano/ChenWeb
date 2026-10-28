-- Register the implemented flow layouter without changing existing catalogue entries.
-- +goose Up
INSERT INTO kb.code_modules
    (code_module_id, module_name, description, module_type, module_doc, notes, creator)
VALUES
    ('flow-layouter', 'Flow Layouter',
     'Wraps page elements into rows with configurable gaps, horizontal and vertical alignment, and elastic sizing.',
     'layouter', 'interpreter-layouter-flow.md',
     'Selected by page.layouter; configured through page.layouter_config. Built into the WPI frontend registry.',
     'migration:wpi-flow-layouter')
ON CONFLICT (code_module_id) DO NOTHING;

-- +goose Down
DELETE FROM kb.code_modules
WHERE code_module_id = 'flow-layouter' AND creator = 'migration:wpi-flow-layouter';
