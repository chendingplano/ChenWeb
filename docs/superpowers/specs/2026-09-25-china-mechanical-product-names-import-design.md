# China Mechanical Product Names Import

Add a page at **Development → System Admin → Resources → Mechanical Product Names** to import the supplied CSV into `kb.product_names` with `source='china-mechanical'`.

The page accepts CSV input and previews its required columns and row count before import. The server maps `code_class_large` to `sub_catalog`, `code_class_medium` to `category_l1`, `code_class_small` to `category_l2`, `product_name` to `product_name`, and `note` to `notes`. It also stores `code_group`, `child_code_field`, `industry_code`, `cpc`, `entry_no`, and `entry_no_new` in newly migrated columns. It translates each Chinese product name into `product_name_en` using the configured translation model.

Import is additive. It assigns `seq_no` from CSV record order and inserts rows with `ON CONFLICT DO NOTHING`; it never updates or deletes existing rows. The existing unique key `(source, seq_no, product_name)` provides idempotence for repeat uploads. The UI reports inserted and skipped counts and any translation or validation failures. Backend import routes require admin/root authorization; nav visibility is not an authorization boundary.

The companion Markdown file cited in the request was not found in KnowledgeStore; implementation relies on the supplied workbook/CSV and the user-provided mapping.
