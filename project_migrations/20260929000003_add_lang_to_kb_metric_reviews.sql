-- +goose Up
-- openspec change metric-review-i18n-export: reviews are per language. lang is the
-- language of the report prose ('en' | 'zh-cn'); every review before this change was
-- written in English. translated_from_id is the kb.metric_reviews row a translation
-- was made from (NULL for an LLM review).
ALTER TABLE kb.metric_reviews ADD COLUMN IF NOT EXISTS lang TEXT NOT NULL DEFAULT 'en';
ALTER TABLE kb.metric_reviews ADD COLUMN IF NOT EXISTS translated_from_id BIGINT;

DROP INDEX IF EXISTS kb.idx_kb_metric_reviews_record;
CREATE INDEX IF NOT EXISTS idx_kb_metric_reviews_record_lang
    ON kb.metric_reviews (input_record_id, lang, created_at DESC);

-- +goose Down
DROP INDEX IF EXISTS kb.idx_kb_metric_reviews_record_lang;
CREATE INDEX IF NOT EXISTS idx_kb_metric_reviews_record
    ON kb.metric_reviews (input_record_id, created_at DESC);
ALTER TABLE kb.metric_reviews DROP COLUMN IF EXISTS translated_from_id;
ALTER TABLE kb.metric_reviews DROP COLUMN IF EXISTS lang;
