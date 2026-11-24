"""Verify compliance-only intake, ordinary review and independent menu retention.

Run against the local Vite server: python3 scripts/test-standard-compliance-review.py.
API responses are mocked; no review jobs are started.
"""
import os
from playwright.sync_api import sync_playwright, expect

BASE = os.environ.get('CHENWEB_TEST_URL', 'http://localhost:5173')
STANDARD = 'apps-standard-compliance-review'

with sync_playwright() as p:
    browser = p.chromium.launch(headless=True)
    context = browser.new_context(viewport={'width': 1600, 'height': 1100})
    context.add_cookies([{'name': 'PARAGLIDE_LOCALE', 'value': 'en', 'url': BASE}])
    context.route('**/api/v1/page-config/**', lambda r: r.fulfill(json={'entries': [], 'hidden': []}))
    submissions = []

    def review_api(route):
        path = route.request.url.split('?')[0]
        if path.endswith('/aspects'):
            data = {'status': True, 'aspects': [
                {'name': 'compliance', 'label': 'Compliance Review', 'group': 'P5'},
                {'name': 'spelling', 'label': 'Spelling', 'group': 'P1'}],
                'packages': [], 'reviewer_packages': [
                    {'key': 'all', 'label': 'All reviewers', 'aspect_names': ['compliance', 'spelling']}]}
        elif path.endswith('/tiers'):
            data = {'status': True, 'tiers': [
                {'key': 'basic', 'label': 'Basic', 'aspect_names': ['spelling']}]}
        elif route.request.method == 'POST' and path.endswith('/requests'):
            submissions.append(route.request.post_data_json)
            data = {'status': False, 'error_msg': 'Test submission captured'}
        else:
            data = {'status': True, 'requests': [], 'runs': [], 'jobs': []}
        route.fulfill(json=data)

    context.route('**/api/v1/doc-review/**', review_api)
    page = context.new_page()
    page.goto(BASE + '/development?view=applications&child=' + STANDARD)
    standard = page.locator(f'[data-retained-page="{STANDARD}"]')
    expect(standard.get_by_role('heading', name='Standard Compliance Review', exact=True)).to_be_visible(timeout=30000)
    expect(standard.get_by_text('Check Level', exact=True)).to_have_count(0)
    expect(standard.get_by_text('References', exact=True)).to_have_count(0)

    # Search supplies a synthetic record through the ordinary document picker.
    context.route('**/api/v1/kb/inputs?*', lambda r: r.fulfill(json={
        'status': True, 'results': [{'id': 123, 'title': 'Test standard', 'file_name': 'test.pdf'}], 'total': 1}))
    standard.get_by_role('button', name='Search Library', exact=True).click()
    page.get_by_role('button', name='Search', exact=True).last.click()
    page.get_by_text('Test standard', exact=True).last.dblclick()
    expect(standard.get_by_role('heading', name='Step 2: Review Details', exact=True)).to_be_visible()
    standard.get_by_label('Your Name *', exact=True).fill('Test requester')
    standard.get_by_label('Notes (optional)', exact=True).fill('Keep these compliance notes')

    page.get_by_role('button', name='Document Review', exact=True).click()
    ordinary = page.locator('[data-retained-page="apps-document-review"]')
    expect(ordinary.get_by_role('heading', name='Step 1: Select Document')).to_be_visible()
    expect(ordinary.get_by_text('Check Level', exact=True)).to_be_visible()
    expect(ordinary.get_by_text('References', exact=True)).to_be_visible()
    expect(ordinary.get_by_text('Test standard', exact=True)).to_have_count(0)
    page.get_by_role('button', name='Standard Compliance Review', exact=True).click()
    expect(standard.get_by_label('Notes (optional)', exact=True)).to_have_value('Keep these compliance notes')
    expect(standard.get_by_label('Your Name *', exact=True)).to_have_value('Test requester')
    standard.get_by_role('button', name='← Back', exact=True).click()
    expect(standard.get_by_role('heading', name='Step 1: Select Document')).to_be_visible()
    standard.get_by_role('button', name='Next →', exact=True).click()
    standard.get_by_role('button', name='Start Review', exact=True).click()
    expect(standard.get_by_text('Test submission captured', exact=True)).to_be_visible()
    assert len(submissions) == 1, submissions
    assert submissions[0]['aspects'] == ['compliance'], submissions
    assert submissions[0]['input_record_id'] == 123, submissions
    assert submissions[0]['review_depth'] == 1, submissions
    browser.close()
    print('PASS: compliance-only submission, skipped steps, ordinary review and menu retention')
