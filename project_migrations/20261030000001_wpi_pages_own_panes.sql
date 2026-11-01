-- WPI pages own their panes (devdoc 2026102601, revised 2026-10-29, clarified
-- 2026-10-30; OpenSpec change wpi-pages-own-panes, design D11).
--
-- 1. kb.wpi_descriptors holds no pages (the one kind 'page' row, a non-WPI body
--    inserted by hand on 2026-10-29, is deleted) and gains the kinds
--    page-layouter and element-layouter.
-- 2. kb.code_modules: module types page-layouter and element-layouter replace
--    layouter; flow-layouter becomes an element layouter; the page-layouter
--    module, which draws pane trees, is added.
-- 3. The layouter classes single-pane, top-left-middle-right-bottom and
--    flow-layouter.
-- 4. Every page whose latest active version is format 0.2 gets a new version in
--    format 0.5: a single-pane page whose root pane main carries the page's
--    margins (30px each, the former default), and whose panel main holds the
--    page's elements with a flow-layouter copy configured by layouter_config.
-- 5. Page customer-admin-home takes over the panes of app customer-admin: the
--    header in top, the menu in left, pages opened in middle and right. Both demo
--    apps name their initial page.
--
-- Every element instance, action instance and layouter copy of a page written
-- here holds a copy of the active class (design D2), made by the pg_temp
-- functions below. They walk the same places as actionPositions in
-- server/api/wpi/validate.go. Down deletes what Up added, so the previous
-- versions are in use again; the code that reads them is the previous code.

-- +goose Up
-- +goose StatementBegin
DELETE FROM kb.wpi_descriptors WHERE kind = 'page';
ALTER TABLE kb.wpi_descriptors DROP CONSTRAINT IF EXISTS wpi_descriptors_kind_check;
ALTER TABLE kb.wpi_descriptors
    ADD CONSTRAINT wpi_descriptors_kind_check CHECK (kind IN ('app', 'element', 'action', 'page-layouter', 'element-layouter'));
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE kb.code_modules DROP CONSTRAINT IF EXISTS code_modules_module_type_check;
UPDATE kb.code_modules
SET module_type = 'element-layouter',
    notes = 'Named by a panel''s elem_layouter copy; configured through its config. Built into the WPI frontend registry.',
    updated_at = NOW()
WHERE code_module_id = 'flow-layouter';
ALTER TABLE kb.code_modules
    ADD CONSTRAINT code_modules_module_type_check CHECK (module_type IN ('element', 'action', 'page', 'page-layouter', 'element-layouter'));
COMMENT ON COLUMN kb.code_modules.module_type IS
    'What the module implements: element, action, page, page-layouter or element-layouter.';
INSERT INTO kb.code_modules (code_module_id, module_name, description, module_type, module_doc, notes, creator) VALUES
    ('page-layouter', 'Page Layouter',
     'Draws a page''s pane tree: panes split into rows or columns, with sizes, margins, alignment and scroll bars.',
     'page-layouter', 'interpreter-layouter-page.md',
     'Named by a page''s page_layouter copy. Built into the WPI frontend registry.',
     'migration:wpi-pages-own-panes')
ON CONFLICT (code_module_id) DO NOTHING;
-- +goose StatementEnd

-- +goose StatementBegin
INSERT INTO kb.wpi_descriptors (kind, descriptor_id, version, schema_version, body, status, created_by, updated_by) VALUES
('page-layouter', 'single-pane', 1, '0.5', $wpi${
  "descriptor_id": "single-pane", "schema_version": "0.5",
  "description": {"en": "One pane: the whole page", "zh-cn": "单个窗格：整个页面"},
  "code_module_id": "page-layouter",
  "root": {"pane_id": "main"}
}$wpi$::jsonb, 'active', 'migration:wpi-pages-own-panes', 'migration:wpi-pages-own-panes'),
('page-layouter', 'top-left-middle-right-bottom', 1, '0.5', $wpi${
  "descriptor_id": "top-left-middle-right-bottom", "schema_version": "0.5",
  "description": {"en": "A header, three columns and a footer", "zh-cn": "页眉、三栏和页脚"},
  "code_module_id": "page-layouter",
  "root": {"pane_id": "page", "split": "rows", "children": [
    {"pane_id": "top", "height": "48px"},
    {"pane_id": "middle-row", "height": "elastic", "split": "columns", "children": [
      {"pane_id": "left", "width": "240px", "min_width": "160px", "resizable": true},
      {"pane_id": "middle", "width": "elastic"},
      {"pane_id": "right", "width": "elastic", "min_width": "400px"}]},
    {"pane_id": "bottom", "height": "32px"}]}
}$wpi$::jsonb, 'active', 'migration:wpi-pages-own-panes', 'migration:wpi-pages-own-panes'),
('element-layouter', 'flow-layouter', 1, '0.5', $wpi${
  "descriptor_id": "flow-layouter", "schema_version": "0.5",
  "description": {"en": "Places elements left to right and wraps to the next row", "zh-cn": "从左到右排列元素，放不下时换行"},
  "code_module_id": "flow-layouter",
  "config": {"column_gap": "0.75rem", "row_gap": "0.75rem", "horizontal_alignment": "start", "vertical_alignment": "top", "rows_alignment": "top"}
}$wpi$::jsonb, 'active', 'migration:wpi-pages-own-panes', 'migration:wpi-pages-own-panes')
ON CONFLICT (kind, descriptor_id, version) DO NOTHING;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION pg_temp.wpi_copy(holder jsonb, class_kind text, name_key text) RETURNS jsonb AS $fn$
DECLARE
    row jsonb;
BEGIN
    IF jsonb_typeof(holder) <> 'object' OR holder ? 'class' OR NOT holder ? name_key THEN
        RETURN holder;
    END IF;
    SELECT body INTO row FROM kb.wpi_descriptors
    WHERE kind = class_kind AND descriptor_id = holder ->> name_key AND status = 'active'
    ORDER BY version DESC LIMIT 1;
    IF row IS NULL THEN
        RAISE EXCEPTION 'no active % class %', class_kind, holder ->> name_key;
    END IF;
    RETURN holder || jsonb_build_object('class', row);
END
$fn$ LANGUAGE plpgsql;
-- +goose StatementEnd

-- +goose StatementBegin
-- An action instance, and the one an open-page holds in input. An object without
-- action is static data and gets no copy.
CREATE FUNCTION pg_temp.wpi_action(a jsonb) RETURNS jsonb AS $fn$
BEGIN
    IF jsonb_typeof(a) <> 'object' OR NOT a ? 'action' THEN
        RETURN a;
    END IF;
    a := pg_temp.wpi_copy(a, 'action', 'action');
    IF a ->> 'action' = 'open-page' AND a ? 'input' THEN
        a := jsonb_set(a, '{input}', pg_temp.wpi_action(a -> 'input'));
    END IF;
    RETURN a;
END
$fn$ LANGUAGE plpgsql;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION pg_temp.wpi_items(items jsonb) RETURNS jsonb AS $fn$
BEGIN
    RETURN (SELECT COALESCE(jsonb_agg(
        CASE
            WHEN it ? 'on_click' THEN jsonb_set(it, '{on_click}', pg_temp.wpi_action(it -> 'on_click'))
            WHEN it ? 'children' THEN jsonb_set(it, '{children}', pg_temp.wpi_items(it -> 'children'))
            ELSE it
        END ORDER BY ord), '[]'::jsonb)
        FROM jsonb_array_elements(items) WITH ORDINALITY AS x(it, ord));
END
$fn$ LANGUAGE plpgsql;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION pg_temp.wpi_element(e jsonb) RETURNS jsonb AS $fn$
DECLARE
    attr text;
BEGIN
    e := pg_temp.wpi_copy(e, 'element', 'element');
    FOREACH attr IN ARRAY ARRAY['on_click', 'source', 'on_row_click'] LOOP
        IF e ? attr THEN
            e := jsonb_set(e, ARRAY[attr], pg_temp.wpi_action(e -> attr));
        END IF;
    END LOOP;
    IF jsonb_typeof(e -> 'items') = 'array' THEN
        e := jsonb_set(e, '{items}', pg_temp.wpi_items(e -> 'items'));
    END IF;
    RETURN e;
END
$fn$ LANGUAGE plpgsql;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION pg_temp.wpi_page(p jsonb) RETURNS jsonb AS $fn$
DECLARE
    pane text;
    panel jsonb;
BEGIN
    p := jsonb_set(p, '{page_layouter}', pg_temp.wpi_copy(p -> 'page_layouter', 'page-layouter', 'layouter'));
    IF p ? 'input' THEN
        p := jsonb_set(p, '{input}', pg_temp.wpi_action(p -> 'input'));
    END IF;
    IF p ? 'save_action' THEN
        p := jsonb_set(p, '{save_action}', pg_temp.wpi_action(p -> 'save_action'));
    END IF;
    FOR pane, panel IN SELECT key, value FROM jsonb_each(COALESCE(p -> 'panels', '{}'::jsonb)) LOOP
        panel := jsonb_set(panel, '{elem_layouter}', pg_temp.wpi_copy(panel -> 'elem_layouter', 'element-layouter', 'layouter'));
        IF jsonb_typeof(panel -> 'elem_list') = 'array' THEN
            panel := jsonb_set(panel, '{elem_list}', (
                SELECT COALESCE(jsonb_agg(pg_temp.wpi_element(e) ORDER BY ord), '[]'::jsonb)
                FROM jsonb_array_elements(panel -> 'elem_list') WITH ORDINALITY AS x(e, ord)));
        END IF;
        p := jsonb_set(p, ARRAY['panels', pane], panel);
    END LOOP;
    RETURN p;
END
$fn$ LANGUAGE plpgsql;
-- +goose StatementEnd

-- +goose StatementBegin
-- A format 0.2 page as a single-pane page of format 0.5.
CREATE FUNCTION pg_temp.wpi_page_05(b jsonb) RETURNS jsonb AS $fn$
DECLARE
    root jsonb := jsonb_build_object(
        'pane_id', 'main', 'hori_align', 'left',
        'left_margin', COALESCE(b ->> 'left_margin', '30px'), 'right_margin', COALESCE(b ->> 'right_margin', '30px'),
        'top_margin', COALESCE(b ->> 'top_margin', '30px'), 'bottom_margin', COALESCE(b ->> 'bottom_margin', '30px'));
    layouter jsonb := jsonb_strip_nulls(jsonb_build_object('layouter', 'flow-layouter', 'config', b -> 'layouter_config'));
BEGIN
    RETURN (b - 'layouter' - 'layouter_config' - 'elements' - 'page_alignment'
              - 'left_margin' - 'right_margin' - 'top_margin' - 'bottom_margin')
        || CASE WHEN b ? 'page_alignment' AND NOT b ? 'hori_align'
                THEN jsonb_build_object('hori_align', b -> 'page_alignment') ELSE '{}'::jsonb END
        || jsonb_build_object(
            'schema_version', '0.5',
            'page_layouter', jsonb_build_object('layouter', 'single-pane', 'root', root),
            'panels', jsonb_build_object('main', jsonb_build_object(
                'elem_layouter', layouter, 'elem_list', COALESCE(b -> 'elements', '[]'::jsonb))));
END
$fn$ LANGUAGE plpgsql;
-- +goose StatementEnd

-- +goose StatementBegin
INSERT INTO kb.frontend_pages (descriptor_id, version, schema_version, body, status, created_by, updated_by)
SELECT l.descriptor_id,
       (SELECT COALESCE(MAX(f.version), 0) + 1
        FROM kb.frontend_pages f
        WHERE f.descriptor_id = l.descriptor_id),
       '0.5', pg_temp.wpi_page(pg_temp.wpi_page_05(l.body)), 'active',
       'migration:wpi-pages-own-panes', 'migration:wpi-pages-own-panes'
FROM (
    SELECT DISTINCT ON (descriptor_id) descriptor_id, version, body
    FROM kb.frontend_pages WHERE status = 'active'
    ORDER BY descriptor_id, version DESC
) l
WHERE l.body ->> 'schema_version' = '0.2';
-- +goose StatementEnd

-- +goose StatementBegin
INSERT INTO kb.frontend_pages (descriptor_id, version, schema_version, body, status, created_by, updated_by)
SELECT p.id, COALESCE((SELECT MAX(version) FROM kb.frontend_pages f WHERE f.descriptor_id = p.id), 0) + 1, '0.5',
       pg_temp.wpi_page(p.body), 'active', 'migration:wpi-pages-own-panes', 'migration:wpi-pages-own-panes'
FROM (VALUES
('customer-admin-home', $wpi${
  "descriptor_id": "customer-admin-home", "schema_version": "0.5",
  "page_name": {"en": "Customer admin", "zh-cn": "客户管理"},
  "description": {"en": "Home page of the customer admin demo: a header, a menu, and panes in which the menu opens pages", "zh-cn": "客户管理演示的首页：页眉、菜单以及菜单打开页面所用的窗格"},
  "input": {"title": "Customer admin"},
  "page_layouter": {"layouter": "top-left-middle-right-bottom", "root": {"pane_id": "page", "split": "rows", "children": [
    {"pane_id": "top", "height": "48px", "left_margin": "16px", "right_margin": "16px", "top_margin": "12px", "bottom_margin": "12px", "hori_align": "left"},
    {"pane_id": "middle-row", "height": "elastic", "split": "columns", "children": [
      {"pane_id": "left", "width": "240px", "min_width": "160px", "resizable": true,
       "left_margin": "8px", "right_margin": "8px", "top_margin": "8px", "bottom_margin": "8px", "hori_align": "left"},
      {"pane_id": "middle", "width": "elastic", "min_width": "320px",
       "left_margin": "30px", "right_margin": "30px", "top_margin": "30px", "bottom_margin": "30px"},
      {"pane_id": "right", "width": "elastic", "min_width": "360px",
       "left_margin": "30px", "right_margin": "30px", "top_margin": "30px", "bottom_margin": "30px"}]}]}},
  "panels": {
    "top": {"elem_layouter": {"layouter": "flow-layouter"}, "elem_list": [
      {"element": "text", "data_binding": "$input_doc.title"}]},
    "left": {"elem_layouter": {"layouter": "flow-layouter"}, "elem_list": [
      {"element": "menu", "initial_item": "customers", "items": [
        {"item_id": "customer-records", "label": "Customer records", "children": [
          {"item_id": "customers", "label": "Customers", "on_click": {"action": "open-page", "page": "customers", "pane": "middle"}},
          {"item_id": "new-customer", "label": "New customer", "on_click": {"action": "open-page", "page": "customer-form", "pane": "middle", "input": {}}},
          {"item_id": "customer-directory", "label": "Directory", "on_click": {"action": "open-page", "page": "customer-directory", "pane": "middle"}}]},
        {"item_id": "about", "label": "About", "on_click": {"action": "open-page", "page": "customer-admin-about", "pane": "middle"}}]}]}}
}$wpi$::jsonb)
) AS p(id, body);
-- +goose StatementEnd

-- +goose StatementBegin
INSERT INTO kb.wpi_descriptors (kind, descriptor_id, version, schema_version, body, status, created_by, updated_by)
SELECT 'app', a.id, COALESCE((SELECT MAX(version) FROM kb.wpi_descriptors d WHERE d.kind = 'app' AND d.descriptor_id = a.id), 0) + 1,
       '0.5', a.body, 'active', 'migration:wpi-pages-own-panes', 'migration:wpi-pages-own-panes'
FROM (VALUES
('customer-admin', $wpi${"descriptor_id": "customer-admin", "schema_version": "0.5", "initial_page": "customer-admin-home"}$wpi$::jsonb),
('customers-demo', $wpi${"descriptor_id": "customers-demo", "schema_version": "0.5", "initial_page": "demo-customers"}$wpi$::jsonb)
) AS a(id, body);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DELETE FROM kb.wpi_descriptors WHERE created_by = 'migration:wpi-pages-own-panes';
DELETE FROM kb.frontend_pages WHERE created_by = 'migration:wpi-pages-own-panes';
DELETE FROM kb.code_modules WHERE creator = 'migration:wpi-pages-own-panes';
UPDATE kb.code_modules SET module_type = 'layouter', updated_at = NOW() WHERE code_module_id = 'flow-layouter';
ALTER TABLE kb.code_modules DROP CONSTRAINT IF EXISTS code_modules_module_type_check;
ALTER TABLE kb.code_modules
    ADD CONSTRAINT code_modules_module_type_check CHECK (module_type IN ('element', 'action', 'page', 'layouter'));
ALTER TABLE kb.wpi_descriptors DROP CONSTRAINT IF EXISTS wpi_descriptors_kind_check;
ALTER TABLE kb.wpi_descriptors
    ADD CONSTRAINT wpi_descriptors_kind_check CHECK (kind IN ('app', 'element', 'action'));
-- +goose StatementEnd
