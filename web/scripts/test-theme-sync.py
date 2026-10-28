"""Run with a local Vite server: python3 scripts/test-theme-sync.py.

Mocks WPI responses to check theme changes across the dashboard and its iframe
without a login or database. Theme changes must preserve the running WPI page.
"""
import os
import re
from playwright.sync_api import sync_playwright, expect

BASE = os.environ.get('CHENWEB_TEST_URL', 'http://localhost:5173')
BUNDLE = {
    'app': {'descriptor_id': 'customers-demo', 'schema_version': '0.2',
            'layout_template': 'single-pane', 'panes': {'main': {'page': 'demo-customers'}}},
    'pages': {'demo-customers': {
        'descriptor_id': 'demo-customers', 'schema_version': '0.2', 'layouter': 'flow-layouter',
        'elements': [
            {'element': 'text', 'text': 'Demo - Customers', 'width': '100%'},
            {'element': 'list', 'width': '100%',
             'source': {'action': 'load', 'table': 'kb.wpi_demo_customers', 'result': 'list'},
             'columns': [{'field': 'user_name', 'label': 'User name'}]}
        ]}},
    'elements': {name: {'descriptor_id': name, 'schema_version': '0.4', 'code_module_id': name}
                 for name in ['text', 'list']},
    'actions': {}, 'warnings': []
}
DARK = re.compile(r'\bdark\b')

with sync_playwright() as p:
    browser = p.chromium.launch(headless=True)
    context = browser.new_context(viewport={'width': 1440, 'height': 1000}, color_scheme='light')
    context.add_cookies([{'name': 'PARAGLIDE_LOCALE', 'value': 'en', 'url': BASE}])
    context.add_init_script("if (!localStorage.getItem('chenweb-theme')) localStorage.setItem('chenweb-theme', 'dark')")
    context.route('**/api/v1/wpi/apps/customers-demo', lambda route: route.fulfill(json=BUNDLE))
    context.route('**/api/v1/wpi/actions/run', lambda route: route.fulfill(json={'result': [{'user_name': 'Alice'}]}))
    context.route('**/api/v1/page-config/**', lambda route: route.fulfill(json={'entries': [], 'hidden': []}))
    page = context.new_page()
    errors = []
    page.on('pageerror', lambda error: errors.append(str(error)))
    page.goto(BASE + '/development?view=development&child=dev-demos-customers')
    frame = page.frame_locator('iframe[src="/wpi/customers-demo"]')
    expect(frame.get_by_text('Alice', exact=True)).to_be_visible(timeout=30000)
    expect(page.locator('html')).to_have_class(DARK)
    expect(frame.locator('html')).to_have_class(DARK)
    pane = frame.locator('[data-pane="main"]')
    pane.evaluate('(el) => el.dataset.themeTest = "preserved"')
    dark_background = frame.locator('main').evaluate('(el) => getComputedStyle(el).backgroundColor')
    other = context.new_page()
    other.goto(BASE + '/wpi/customers-demo')
    expect(other.get_by_text('Alice', exact=True)).to_be_visible()
    expect(other.locator('html')).to_have_class(DARK)
    page.get_by_role('button', name='Toggle dark mode', exact=True).click()
    expect(page.locator('html')).not_to_have_class(DARK)
    expect(frame.locator('html')).not_to_have_class(DARK)
    expect(other.locator('html')).not_to_have_class(DARK)
    light_background = frame.locator('main').evaluate('(el) => getComputedStyle(el).backgroundColor')
    assert dark_background != light_background, (dark_background, light_background)
    page.get_by_role('button', name='Toggle dark mode', exact=True).click()
    expect(frame.locator('html')).to_have_class(DARK)
    expect(other.locator('html')).to_have_class(DARK)
    expect(pane).to_have_attribute('data-theme-test', 'preserved')
    expect(frame.get_by_text('Alice', exact=True)).to_be_visible()
    assert not errors, errors
    print('PASS: initial dark mode and live light/dark changes synchronize across the iframe and another tab without remounting the WPI page')
    browser.close()
