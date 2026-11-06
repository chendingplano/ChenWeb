"""Smoke test for System Admin > Benchmark > Provisions (Gold Provisions and Benchmark) with mocked
APIs, in English and Chinese. Requires a running ChenWeb web server and Python Playwright; no login
or database access is needed."""
import argparse
import asyncio
import json
import pathlib
import tempfile
from urllib.parse import urlparse, parse_qs

from playwright.async_api import async_playwright

ROOT = pathlib.Path(__file__).resolve().parents[2]
parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument('--base-url', default='http://localhost:5173')
BASE = parser.parse_args().base_url.rstrip('/')
SCREENSHOTS = pathlib.Path(tempfile.mkdtemp(prefix='chenweb-provision-benchmark-'))
RUN = {'skill_version': '1.0.0', 'model_name': 'claude-opus-5-5', 'run_id': '20261106_143017'}
RECORD = {'id': 416, 'title': '农村生活垃圾分类处理规范', 'doc_no': 'DB33/T 2030—2018', 'type': 'pdf',
          'file_name': 'std_1503937.pdf', 'parser_name': 'mineru', 'gold_metric_runs': [RUN]}
LINES = [{'line_number': 89, 'page_number': 5, 'line_type': 'list-item', 'coords': [100, 100, 900, 120],
          'content': '7.1.1 垃圾分类收集容器应有盖，应在显著位置印制垃圾分类标志。'},
         {'line_number': 94, 'page_number': 5, 'line_type': 'list-item', 'coords': [100, 200, 900, 220],
          'content': '7.1.6 其他垃圾投放至户分类垃圾容器，或村分类垃圾投放点。'},
         {'line_number': 19, 'page_number': 3, 'line_type': 'paragraph', 'coords': [100, 300, 900, 320],
          'content': '本标准适用于农村生活垃圾分类处理。'}]
GOLD = [
    {'id': 1, 'input_record_id': 416, 'prov_id': '416_gprv_8', 'prov_name': '分类收集容器有盖并印制标志',
     'prov_name_en': 'Collection containers have lids', 'provision_type': 'mandatory', 'provision_kind': 'requirement',
     'source_line_spans': ['89'], 'source_table_rows': [], 'provision': '垃圾分类收集容器应有盖，应在显著位置印制垃圾分类标志。',
     'provision_subject': '垃圾分类收集容器', 'location_type': 'bullet', 'confidence': 0.95, 'is_explicit': True,
     'section': '7.1 投放要求', 'evidence': '垃圾分类收集容器应有盖'},
    {'id': 2, 'input_record_id': 416, 'prov_id': '416_gprv_15', 'prov_name': '其他垃圾投放至户容器或村投放点',
     'prov_name_en': 'Other waste disposal points', 'provision_type': 'mandatory', 'provision_kind': 'requirement',
     'source_line_spans': ['94'], 'source_table_rows': [], 'provision': '其他垃圾投放至户分类垃圾容器，或村分类垃圾投放点。',
     'provision_subject': '其他垃圾', 'location_type': 'bullet', 'confidence': 0.95, 'is_explicit': False,
     'section': '7.1 投放要求', 'evidence': '其他垃圾投放至户分类垃圾容器'},
]
DROPPED = [{'id': 7, 'candidate_id': 'c6', 'decision': 'excluded', 'drop_reason': 'X2', 'reason': 'Scope: applicability.',
            'text': '本标准适用于农村生活垃圾分类处理。', 'source_line_spans': ['19'], 'prov_ids': []}]


def score_fixture(locale):
    summary = {'score': 61, 'soft_precision': .6, 'soft_recall': .62, 'precision': .75, 'recall': .67,
               'gold_rows': 3, 'predicted_rows': 4, 'matched': 2}
    return {'id': 51, 'input_record_id': 416, 'title': '农村生活垃圾分类处理规范', 'lang': locale, 'status': 'done',
            'model_name': 'scorer-exact', 'prompt_name': 'prompt-score-extract-provisions-v1.md', 'created_by': 'smoke',
            'created_at': '2026-11-06T18:00:00Z', 'finished_at': '2026-11-06T18:01:00Z',
            'input': {'gold_run': {'skill_version': '1.0.0', 'model_name': 'claude-opus-5-5',
                                   'benchmark_run_id': '20261106_143017', 'rows': 3},
                      'extraction': {'model_names': ['deepseek-v4-flash'], 'prompt_names': ['prompt-extract-provisions-v3.md']},
                      'warnings': ['Fixture warning'],
                      'gold': [dict(g, prov_id=g['prov_id']) for g in GOLD] + [
                          {'prov_id': '416_gprv_3', 'provision_kind': 'requirement', 'provision_type': 'mandatory',
                           'prov_name': '建制村配备分类垃圾桶', 'provision': '建制村应合理配备分类垃圾桶。',
                           'source_line_spans': ['89']}],
                      'predictions': [
                          {'prov_id': '416_prv_1', 'provision_type': 'mandatory', 'prov_name': '容器有盖',
                           'provision': '7.1.1 垃圾分类收集容器应有盖', 'provision_subject': '垃圾分类收集容器',
                           'location_type': 'list-item', 'is_explicit': True, 'source_line_spans': ['89']},
                          {'prov_id': '416_prv_2', 'provision_type': 'recommended', 'prov_name': '其他垃圾投放',
                           'provision': '其他垃圾投放至户分类垃圾容器', 'provision_subject': '其他垃圾',
                           'location_type': 'bullet', 'is_explicit': True, 'source_line_spans': ['94']},
                          {'prov_id': '416_prv_3', 'provision_type': 'mandatory', 'prov_name': '标志',
                           'provision': '应在显著位置印制垃圾分类标志', 'source_line_spans': ['89']},
                          {'prov_id': '416_prv_4', 'provision_type': 'mandatory', 'prov_name': '适用范围',
                           'provision': '本标准适用于农村生活垃圾分类处理。', 'source_line_spans': ['19']}]},
            'score': {'score': 61, 'main': summary,
                      'groups': {'mandatory': dict(summary, score=70), 'recommended': dict(summary, score=1),
                                 'optional': dict(summary, gold_rows=0, predicted_rows=0, matched=0, score=1)},
                      'recall_by_kind': {'requirement': {'gold_rows': 3, 'matched': 2, 'soft_recall': .62}},
                      'field_accuracy': {'provision_type': .5, 'text': .9, 'lines': 1, 'subject': 1,
                                         'location_type': .5, 'is_explicit': .5},
                      'pairs': [{'gold': '416_gprv_8', 'pred': '416_prv_1', 'note': '',
                                 'checks': {'provision_type': True, 'text': .8, 'lines': 1, 'subject': True,
                                            'location_type': False, 'is_explicit': True}, 'credit': .86},
                                {'gold': '416_gprv_15', 'pred': '416_prv_2', 'note': '',
                                 'checks': {'provision_type': False, 'text': .9, 'lines': 1, 'subject': True,
                                            'location_type': True, 'is_explicit': False}, 'credit': .63}],
                      'missed': [{'gold': '416_gprv_3', 'note': 'not extracted'}],
                      'false_positives': [{'pred': '416_prv_3', 'cause': 'split', 'gold': '416_gprv_8', 'note': 'second clause'},
                                          {'pred': '416_prv_4', 'cause': 'gold_excluded', 'note': 'scope'}],
                      'overrides': []},
            'matches': {'pairs': []}, 'report': '# extract_provisions score — record 416\n**Score: 61 / 100**'}


async def json_route(route, payload, status=200):
    await route.fulfill(status=status, content_type='application/json', body=json.dumps(payload, ensure_ascii=False))


async def check_gold_page(ctx, labels, locale):
    page = await ctx.new_page()
    errors, calls = [], []
    page.on('pageerror', lambda e: errors.append(str(e)))

    async def provisions(route):
        q = parse_qs(urlparse(route.request.url).query)
        calls.append(q)
        assert q.get('source') == ['testbed'] and q.get('run_id') == [RUN['run_id']], q
        await json_route(route, {'status': True, 'results': GOLD, 'total': len(GOLD), 'dropped': DROPPED})

    await page.route('**/api/v1/kb/provisions/gold-options', lambda r: json_route(r, {
        'status': True, 'skill_versions': ['1.0.0'],
        'options': [{'skill_version': '1.0.0', 'model_name': RUN['model_name'], 'record_count': 1, 'run_count': 1}]}))
    await page.route('**/api/v1/kb/provisions?*', provisions)
    await page.route('**/api/v1/kb/inputs?*', lambda r: json_route(r, {
        'status': True, 'results': [RECORD], 'page': 1, 'page_size': 20, 'total': 1}))
    await page.route('**/api/v1/kb/inputs/416', lambda r: json_route(r, {'status': True, 'record': RECORD}))
    await page.route('**/api/v1/kb/raw-lines?*', lambda r: json_route(r, {'status': True, 'lines': LINES}))
    await page.goto(f'{BASE}/development?view=system-admin&child=sysadmin-llm-provisions-gold')
    await page.get_by_text(labels['provision_mgmt_gold_title'], exact=True).first.wait_for(timeout=20000)
    root = page.locator('.metric-mgmt')
    assert await root.evaluate('(el) => getComputedStyle(el).userSelect') == 'text'
    gold_filter = page.get_by_text('gold-provisions', exact=False)
    del gold_filter
    await page.get_by_text(RECORD['title'], exact=False).first.click()
    await page.get_by_text(GOLD[0]['prov_name'], exact=True).first.wait_for()
    assert calls, 'gold provisions were not requested'
    assert await page.locator('.metric-card').count() == 2
    await page.locator('.gold-view-select').select_option('dropped')
    await page.get_by_text('c6 · X2', exact=True).first.wait_for()
    assert await page.locator('.metric-card').count() == 1
    await page.locator('.gold-view-select').select_option('gold')
    await page.locator('.metric-card').first.click()
    # Read-only: no line edit/delete/add tools in the PDF toolbar.
    for key in ['provision_mgmt_edit_lines', 'provision_mgmt_delete_lines', 'provision_mgmt_add_line']:
        assert await page.locator(f'button[title="{labels[key]}"]').count() == 0, key
    await page.screenshot(path=str(SCREENSHOTS / f'gold-{locale}.png'), full_page=True)
    assert not errors, errors
    await page.close()


async def check_score_page(ctx, labels, locale):
    page = await ctx.new_page()
    errors, posts = [], []
    page.on('pageerror', lambda e: errors.append(str(e)))
    done = score_fixture(locale)

    async def scores(route):
        req = route.request
        path = urlparse(req.url).path
        payload = {'status': True}
        if path.endswith('/models'):
            payload['models'] = ['scorer-profile']
        elif path.endswith('/gold-runs'):
            payload['runs'] = [{'skill_version': '1.0.0', 'model_name': RUN['model_name'],
                                'benchmark_run_id': RUN['run_id'], 'rows': 84}]
        elif req.method == 'POST':
            posts.append(json.loads(req.post_data))
            payload['run'] = done
        elif path.endswith('/51'):
            payload['run'] = done
        else:
            payload.update(runs=[done], total=1)
        await json_route(route, payload, 202 if req.method == 'POST' else 200)

    await page.route('**/api/v1/kb/provision-scores**', scores)
    await page.route('**/api/v1/kb/inputs?*', lambda r: json_route(r, {
        'status': True, 'results': [RECORD], 'page': 1, 'page_size': 50, 'total': 1}))
    await page.route('**/api/v1/kb/raw-lines?*', lambda r: json_route(r, {'status': True, 'lines': LINES}))
    await page.goto(f'{BASE}/development?view=system-admin&child=sysadmin-llm-provisions-benchmark')
    view = page.locator('.benchmark').first
    await view.get_by_text(labels['psc_title'], exact=True).first.wait_for(timeout=20000)
    assert await view.evaluate('(el) => getComputedStyle(el).userSelect') == 'text'
    await view.locator('.documents button').first.click()
    await view.get_by_role('button', name=labels['msc_run'], exact=True).click()
    results = view.locator('.results')
    await results.get_by_text(labels['psc_by_type'], exact=True).wait_for()
    assert posts and posts[0]['record_id'] == 416 and posts[0]['lang'] == locale, posts
    await results.get_by_text(labels['psc_recall_by_kind'], exact=True).wait_for()
    await results.get_by_text(labels['psc_cause_split'], exact=False).first.wait_for()
    assert await results.locator('.matched-metrics .result-entry').count() == 2
    await results.locator('.pair-filter select').select_option('location_type')
    assert await results.locator('.matched-metrics .result-entry').count() == 1
    await results.locator('.pair-filter select').select_option('all')
    # Selecting a missed gold provision highlights its source line.
    missed = results.locator('details').filter(has_text=labels['psc_missed'].split('{')[0].strip()).locator('.result-entry')
    await missed.first.click()
    assert await missed.first.get_attribute('aria-pressed') == 'true'
    await page.screenshot(path=str(SCREENSHOTS / f'benchmark-{locale}.png'), full_page=True)
    assert not errors, errors
    await page.close()


async def run():
    async with async_playwright() as p:
        browser = await p.chromium.launch(headless=True)
        for locale in ['en', 'zh-cn']:
            labels = json.loads((ROOT / 'web/messages' / f'{locale}.json').read_text())
            ctx = await browser.new_context(viewport={'width': 1600, 'height': 1100})
            await ctx.add_cookies([{'name': 'PARAGLIDE_LOCALE', 'value': locale, 'url': BASE}])
            await check_gold_page(ctx, labels, locale)
            await check_score_page(ctx, labels, locale)
            await ctx.close()
        await browser.close()
    print(f'ok; screenshots in {SCREENSHOTS}')


asyncio.run(run())
