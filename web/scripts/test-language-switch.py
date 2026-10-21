"""Run against the local Vite server: python3 scripts/test-language-switch.py.

Uses bilingual category fixtures so no login or database mutation is required.
"""
import os
from playwright.sync_api import sync_playwright, expect

BASE = os.environ.get('CHENWEB_TEST_URL', 'http://localhost:5173')
TREE = {'id': 'tree', 'canonical_name': 'test-tree', 'pref_labels': {'en': 'Test tree', 'zh': '测试树'}}
NODES = [
    {'id': 'root', 'tree_id': 'tree', 'canonical_name': 'root', 'pref_labels': {'en': 'Root', 'zh': '根'}, 'is_leaf': False},
    {'id': 'branch', 'parent_id': 'root', 'tree_id': 'tree', 'canonical_name': 'branch', 'pref_labels': {'en': 'Branch', 'zh': '分支'}, 'is_leaf': False},
    {'id': 'leaf', 'parent_id': 'branch', 'tree_id': 'tree', 'canonical_name': 'leaf', 'pref_labels': {'en': 'Leaf', 'zh': '叶子'}, 'is_leaf': True},
]

with sync_playwright() as p:
    browser = p.chromium.launch(headless=True)
    context = browser.new_context(viewport={'width': 1600, 'height': 1000})
    context.add_cookies([{'name': 'PARAGLIDE_LOCALE', 'value': 'en', 'url': BASE}])
    page = context.new_page()
    documents = []
    errors = []
    page.on('pageerror', lambda error: errors.append(str(error)))
    page.on('request', lambda req: documents.append(req.url) if req.is_navigation_request() and req.frame == page.main_frame else None)

    def categories(route):
        if '/members?' in route.request.url:
            payload = {'members': [{'node_id': 'leaf', 'artifact_type': 'metric', 'artifact_id': 'selected-record', 'record_id': 42, 'stale': False}]}
        elif route.request.url.endswith('/category-trees'):
            payload = {'trees': [TREE]}
        else:
            payload = {'tree': TREE, 'nodes': NODES, 'valid': True}
        route.fulfill(json=payload)

    def config(route):
        chinese = 'lang=zh-cn' in route.request.url
        entries = [{'entry_key': 'kb-metric-ontology', 'content': {'label': '测试本体' if chinese else 'Test ontology'}}] if '/home3-knowledge?' in route.request.url else []
        route.fulfill(json={'entries': entries, 'hidden': []})

    page.route('**/api/v1/kb/category-trees**', categories)
    page.route('**/api/v1/page-config/**', config)
    page.goto(BASE + '/home3/knowledge?section=kb-ontology-categories')
    page.get_by_role('button', name='Test ontology', exact=False).click()
    tree = page.locator('.cat-tree-item')
    tree.click()
    expect(tree).to_have_attribute('aria-pressed', 'true')
    expect(page.locator('.cat-chart canvas').first).to_be_visible()
    page.locator('.cat-layout select').first.select_option('TB')
    page.locator('.cat-layout select').last.select_option('broken')
    page.evaluate('''async () => {
        const {getInstanceByDom} = await import('/node_modules/.vite/deps/echarts_core.js');
        window.languageTestChart = getInstanceByDom(document.querySelector('[_echarts_instance_]'));
    }''')

    def click_node(node_id):
        point = page.evaluate('''id => {
            const chart = window.languageTestChart;
            const nodes = chart.getOption().series.find(s => s.type === 'scatter');
            const node = nodes.data.find(n => n.id === id);
            return chart.convertToPixel({seriesIndex: 1}, node.value);
        }''', node_id)
        bounds = page.locator('[_echarts_instance_]').bounding_box()
        page.mouse.click(bounds['x'] + point[0], bounds['y'] + point[1])

    click_node('branch')
    page.wait_for_function("window.languageTestChart.getOption().series[1].data.some(n => n.id === 'leaf')")
    click_node('leaf')
    expect(page.locator('.cat-panel h2')).to_have_text('Leaf')
    expect(page.locator('.cat-table')).to_contain_text('selected-record')
    page.evaluate("window.languageTestChart.dispatchAction({type: 'dataZoom', dataZoomId: 'zy', start: 20, end: 60})")
    zoom_before = page.evaluate('window.languageTestChart.getOption().dataZoom.map(z => [z.startValue, z.endValue])')
    # A DOM marker proves the existing page survives instead of being remounted.
    page.locator('.categories-page').evaluate('(el) => el.dataset.preserved = "yes"')
    page.get_by_role('button', name='Switch language', exact=True).click()
    page.get_by_role('menuitem', name='中文', exact=True).click()
    expect(page.locator('.cat-heading').first).to_have_text('类目树')
    expect(tree).to_have_attribute('aria-pressed', 'true')
    expect(tree).to_contain_text('测试树')
    expect(page.locator('.categories-page')).to_have_attribute('data-preserved', 'yes')
    expect(page.locator('.cat-layout select').first).to_have_value('TB')
    expect(page.locator('.cat-layout select').last).to_have_value('broken')
    expect(page.get_by_role('button', name='测试本体', exact=False).first).to_be_visible()
    expect(page.get_by_role('button', name='类目', exact=False).first).to_be_visible()
    expect(page.locator('.cat-panel h2')).to_have_text('叶子')
    expect(page.locator('.cat-table')).to_contain_text('selected-record')
    assert page.evaluate("window.languageTestChart.getOption().series[1].data.some(n => n.id === 'leaf' && n.name === '叶子')"), 'Lost expanded branch or translated labels'
    zoom_after = page.evaluate('window.languageTestChart.getOption().dataZoom.map(z => [z.startValue, z.endValue])')
    assert all(abs(a - b) < 0.001 for before, after in zip(zoom_before, zoom_after) for a, b in zip(before, after)), (zoom_before, zoom_after)
    assert len(documents) == 1, f'Language switch navigated: {documents}'
    assert page.evaluate('document.documentElement.lang') == 'zh-cn'
    assert next(c['value'] for c in context.cookies() if c['name'] == 'PARAGLIDE_LOCALE') == 'zh-cn'
    page.get_by_role('button', name='切换语言', exact=True).click()
    page.get_by_role('menuitem', name='English', exact=True).click()
    expect(page.locator('.cat-panel h2')).to_have_text('Leaf')
    expect(tree).to_have_attribute('aria-pressed', 'true')
    assert len(documents) == 1
    print('PASS: English/Chinese switches retain the selected tree and record, expanded branches, zoom, layout, line style and expanded menu without navigation')

    page.route('**/api/v1/system-admin/build-info', lambda route: route.fulfill(json={'status': True, 'processes': []}))
    page.goto(BASE + '/development?view=system-admin&child=sysadmin-system-build-info')
    expect(page.locator('.build-page')).to_be_visible()
    page.locator('.build-page').evaluate('(el) => el.dataset.preserved = "yes"')
    breadcrumb = page.locator('main nav').filter(has_text='Build Info')
    expect(breadcrumb).to_be_visible()
    document_count = len(documents)
    page.get_by_role('button', name='Switch language', exact=True).click()
    page.get_by_role('menuitem', name='中文', exact=True).click()
    expect(page.locator('.build-page')).to_have_attribute('data-preserved', 'yes')
    expect(page.locator('main nav').filter(has_text='构建信息')).to_be_visible()
    assert len(documents) == document_count
    assert not errors, errors
    print('PASS: dashboard nested selection and breadcrumb translate without remounting the view')
    browser.close()
