-- +goose Up
-- +goose StatementBegin
DO $$
BEGIN
  IF EXISTS (
    SELECT 1 FROM information_schema.tables
    WHERE table_schema = 'kb' AND table_name = 'inputs'
  ) THEN
    EXECUTE 'ALTER TABLE kb.inputs ADD COLUMN IF NOT EXISTS result_filename TEXT';
  ELSIF EXISTS (
    SELECT 1 FROM information_schema.tables
    WHERE table_schema = 'public' AND table_name = 'inputs'
  ) THEN
    EXECUTE 'ALTER TABLE public.inputs ADD COLUMN IF NOT EXISTS result_filename TEXT';
  ELSE
    RAISE EXCEPTION 'inputs table not found in kb/public schema';
  END IF;
END
$$;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DO $$
BEGIN
  IF EXISTS (
    SELECT 1 FROM information_schema.tables
    WHERE table_schema = 'kb' AND table_name = 'inputs'
  ) THEN
    EXECUTE 'ALTER TABLE kb.inputs DROP COLUMN IF EXISTS result_filename';
  ELSIF EXISTS (
    SELECT 1 FROM information_schema.tables
    WHERE table_schema = 'public' AND table_name = 'inputs'
  ) THEN
    EXECUTE 'ALTER TABLE public.inputs DROP COLUMN IF EXISTS result_filename';
  END IF;
END
$$;
-- +goose StatementEnd
