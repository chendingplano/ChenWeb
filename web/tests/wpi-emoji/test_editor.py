"""Browser contract for the real WPI emoji adapter with local API fixtures."""
from playwright.sync_api import sync_playwright, expect

def item(value, name, zh):
    return {'unicode_id': format(ord(value), 'x'), 'unicode_value': value, 'name_en': name, 'name_zh': zh,
            'description_en': name, 'description_zh': zh, 'keywords_en': 'celebration', 'keywords_zh': '庆祝',
            'category_id': 'activities', 'category_en': 'Activities', 'category_zh': '活动', 'version': 1, 'score': 1}

with sync_playwright() as playwright:
    browser = playwright.chromium.launch(headless=True, executable_path='/Applications/Google Chrome.app/Contents/MacOS/Google Chrome')
    page = browser.new_page()
    errors = []
    external_requests = []
    page.on('pageerror', lambda e: errors.append(str(e)))
    def local_only(route):
        if route.request.url.startswith('http://127.0.0.1:5199/'):
            route.continue_()
        else:
            external_requests.append(route.request.url)
            route.abort()
    page.route('**/*', local_only)
    def api(route):
        path = route.request.url.split('?')[0]
        if path.endswith('/categories'):
            route.fulfill(json={'status': True, 'results': [{'id':'activities','name_en':'Activities','name_zh':'活动'}]})
        elif path.endswith('/search'):
            route.fulfill(json={'status': True, 'results': {'items':[item('🎉','party popper','拉炮彩带'),item('🎈','balloon','气球')]}})
        elif path.endswith('/svg'):
            route.fulfill(status=200, content_type='image/svg+xml', body='<svg xmlns="http://www.w3.org/2000/svg"/>')
        elif '/by-id/' in path:
            route.fulfill(json={'status': True, 'results': item('🎉','party popper','拉炮彩带')})
        else:
            route.fulfill(status=404)
    page.route('**/shared_api/v1/emojis/**', api)
    page.goto('http://127.0.0.1:5199/', wait_until='networkidle')
    page.evaluate("localStorage.removeItem('wpi-emoji-test')")
    page.reload(wait_until='networkidle')
    trigger = page.locator('#emoji')
    trigger.click()
    page.get_by_role('button', name='balloon').click()
    expect(page.get_by_role('alert')).to_have_text('Rejected')
    expect(page.get_by_test_id('value')).to_have_text('')
    # The user can return to the search field without applying the invalid value.
    page.get_by_role('button', name='Stay Edit').click()
    expect(page.get_by_role('searchbox')).to_be_focused()
    page.get_by_role('button', name='balloon').click()
    page.get_by_role('button', name='Abandon Edit').click()
    expect(trigger).to_be_focused()
    trigger.click()
    page.get_by_role('button', name='party popper').click()
    expect(page.get_by_test_id('value')).to_have_text('🎉')
    page.reload(wait_until='networkidle')
    expect(page.get_by_test_id('value')).to_have_text('🎉')
    trigger.click()
    page.keyboard.press('Escape')
    expect(trigger).to_be_focused()
    expect(trigger).to_have_attribute('aria-expanded', 'false')
    page.goto('http://127.0.0.1:5199/?zh=1', wait_until='networkidle')
    page.locator('#emoji').click()
    expect(page.get_by_placeholder('按关键词或描述搜索表情')).to_be_visible()
    expect(page.get_by_role('button', name='拉炮彩带')).to_be_visible()
    assert not errors, errors
    assert not external_requests, external_requests
    browser.close()
    print('WPI emoji adapter browser checks passed')
