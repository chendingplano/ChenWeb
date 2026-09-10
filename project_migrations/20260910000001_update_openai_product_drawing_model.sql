-- +goose Up
ALTER TABLE IF EXISTS kb.product_drawings
    ALTER COLUMN model_name SET DEFAULT 'gpt-image-2.5-sunburst';

UPDATE kb.product_drawings
SET model_name = 'gpt-image-2.5-sunburst'
WHERE model = 'OpenAI' AND model_name = 'openai-image-2.5';

-- +goose Down
ALTER TABLE IF EXISTS kb.product_drawings
    ALTER COLUMN model_name SET DEFAULT 'openai-image-2.5';
