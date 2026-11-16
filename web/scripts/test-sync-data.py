"""Exercise sync selection, partial failures, in-place updates and menu retention.
Run against the local Vite server; all sync requests are mocked.
"""
import os
from urllib.parse import urlparse
from playwright.sync_api import sync_playwright, expect

BASE = os.environ.get('CHENWEB_TEST_URL', 'http://localhost:5173')

with sync_playwright() as p:
    browser = p.chromium.launch(headless=True)
    for locale in ['en', 'zh-cn']:
        context = browser.new_context(viewport={'width': 1500, 'height': 850})
        context.add_cookies([{'name': 'PARAGLIDE_LOCALE', 'value': locale, 'url': BASE}])
        page = context.new_page()
        errors = []
        calls = []
        lists = []
        items = [{'item_id': f'item_{i}', 'table': f'public.table_{i}', 'kind': 'table',
                  'origin': 'compiled', 'cursor_col': 'update_time', 'natural_key': ['name'],
                  'columns': ['name', 'update_time'], 'json_columns': [], 'last_row_count': 0, 'target_row_count': 41}
                 for i in range(30)]

        def sync_api(route):
            path = urlparse(route.request.url).path
            if path.endswith('/items'):
                lists.append(path)
                route.fulfill(json={'ok': True, 'items': items})
            elif path.endswith('/schema/tables'):
                route.fulfill(json={'ok': True, 'tables': []})
            elif path.endswith('/preview') or path.endswith('/apply'):
                item_id, action = path.split('/')[-2:]
                calls.append((item_id, action))
                if item_id == 'item_1':
                    route.fulfill(status=502, json={'message': 'Fixture source error'})
                else:
                    route.fulfill(json={'ok': True, 'item_id': item_id, 'changed_row_count': 7,
                                        'synced_row_count': 0 if item_id == 'item_0' else 7, 'target_row_count': 41, 'last_synced_at': '2026-11-16T12:00:00Z'})
            else:
                raise AssertionError(path)

        page.on('pageerror', lambda err: errors.append(str(err)))
        page.route('**/api/v1/data-sync/**', sync_api)
        page.route('**/api/v1/page-config/**', lambda r: r.fulfill(json={'entries': [], 'hidden': []}))
        page.goto(BASE + '/development?view=system-admin&child=sysadmin-resources-sync-data')
        root = page.locator('[data-retained-page="sysadmin-resources-sync-data"]')
        expect(root.locator('tbody tr')).to_have_count(30, timeout=30000)
        preview = root.get_by_role('button', name='Preview selected' if locale == 'en' else '预览所选项', exact=True)
        sync = root.get_by_role('button', name='Sync selected' if locale == 'en' else '同步所选项', exact=True)
        expect(root.locator('tbody tr').first.locator('td').nth(6)).to_have_text('41')
        checkboxes = root.get_by_role('checkbox')
        expect(preview).to_be_disabled()
        checkboxes.first.check()
        expect(root.locator('tbody input:checked')).to_have_count(30)
        checkboxes.first.uncheck()
        expect(root.locator('tbody input:checked')).to_have_count(0)
        checkboxes.nth(1).check()
        checkboxes.nth(2).check()
        checkboxes.nth(3).check()
        assert checkboxes.first.evaluate('(el) => el.indeterminate')
        preview.click()
        expect(preview).to_be_enabled()
        assert calls == [('item_0', 'preview'), ('item_1', 'preview'), ('item_2', 'preview')], calls
        expect(root.locator('tbody tr').nth(1)).to_contain_text('Fixture source error')
        expect(root.locator('tbody tr').nth(2)).to_contain_text('7')
        # Keep the same DOM row and viewport after applying a single table.
        row = root.locator('tbody tr').nth(20)
        row.evaluate('(el) => el.dataset.syncTest = "preserved"')
        row.get_by_role('button', name='Sync' if locale == 'en' else '同步', exact=True).scroll_into_view_if_needed()
        row_top = row.bounding_box()['y']
        row.get_by_role('button', name='Sync' if locale == 'en' else '同步', exact=True).click()
        expect(row.get_by_role('button', name='Sync' if locale == 'en' else '同步', exact=True)).to_be_enabled()
        expect(row).to_have_attribute('data-sync-test', 'preserved')
        expect(row.locator('td').nth(6)).to_have_text('41')
        assert len(lists) == 1, lists
        assert abs(row.bounding_box()['y'] - row_top) <= 2, (row_top, row.bounding_box()['y'])
        sync.click()
        expect(sync).to_be_enabled()
        assert calls[-3:] == [('item_0', 'apply'), ('item_1', 'apply'), ('item_2', 'apply')]
        expect(root.locator('tbody tr').first.locator('td').nth(6)).to_have_text('41')
        assert len(lists) == 1
        expect(root.locator('tbody input:checked')).to_have_count(3)
        root.get_by_role('button', name='+ New Data Syncher' if locale == 'en' else '+ 新建数据同步器', exact=True).click()
        root.locator('form input').first.fill('unsaved-sync-item')
        row.scroll_into_view_if_needed()
        menu_scroll = root.evaluate('(el) => el.closest("main").scrollTop')
        # Menu changes update the existing dashboard without remounting the page.
        page.get_by_role('button', name='System' if locale == 'en' else '系统', exact=True).click()
        page.get_by_role('button', name='Build Info' if locale == 'en' else '构建信息', exact=True).click()
        expect(root).to_be_hidden()
        page.get_by_role('button', name='Sync Data' if locale == 'en' else '数据同步', exact=True).click()
        expect(root).to_be_visible()
        expect(root.locator('tbody input:checked')).to_have_count(3)
        expect(row).to_have_attribute('data-sync-test', 'preserved')
        expect(root.locator('form input').first).to_have_value('unsaved-sync-item')
        assert root.evaluate('(el) => el.closest("main").scrollTop') == menu_scroll
        assert len(lists) == 1, lists
        with page.expect_response(lambda response: urlparse(response.url).path == '/api/v1/data-sync/items'):
            root.get_by_role('button', name='Refresh' if locale == 'en' else '刷新', exact=True).click()
        expect(root.locator('tbody tr').first.locator('td').nth(6)).to_have_text('41')
        assert len(lists) == 2, lists
        assert not errors, errors
        print(f'PASS {locale}: selection, ordered batch, partial errors, row/scroll preservation, menu return')
        context.close()
    browser.close()
