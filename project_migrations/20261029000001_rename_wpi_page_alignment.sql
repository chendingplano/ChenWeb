-- Page attribute page_alignment is now hori_align; the old name is rejected.
-- +goose Up
-- +goose StatementBegin
UPDATE kb.frontend_pages
SET body = (body - 'page_alignment') || jsonb_build_object('hori_align', body -> 'page_alignment')
WHERE body ->> 'page_alignment' IS NOT NULL;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
UPDATE kb.frontend_pages
SET body = (body - 'hori_align') || jsonb_build_object('page_alignment', body -> 'hori_align')
WHERE body ->> 'hori_align' IS NOT NULL;
-- +goose StatementEnd
