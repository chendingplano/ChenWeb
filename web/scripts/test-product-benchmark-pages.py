"""Smoke test for System Admin > Benchmark > Products (Gold Products and Benchmark) with mocked APIs,
in English and Chinese. Requires a running ChenWeb web server and Python Playwright; no login or
database access is needed."""
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
SCREENSHOTS = pathlib.Path(tempfile.mkdtemp(prefix='chenweb-product-benchmark-'))
RUN = {'skill_version': '1.0.0', 'model_name': 'claude-opus-5-5', 'run_id': '20261108_110147'}
RECORD = {'id': 416, 'title': '农村生活垃圾分类处理规范', 'doc_no': 'DB33/T 2030—2018', 'type': 'pdf',
          'file_name': 'std_1503937.pdf', 'parser_name': 'mineru', 'gold_metric_runs': [RUN]}
LINES = [{'line_number': 89, 'page_number': 5, 'line_type': 'list-item', 'coords': [100, 100, 900, 120],
          'content': '7.1.1 垃圾分类收集容器应有盖，应在显著位置印制垃圾分类标志。'},
         {'line_number': 50, 'page_number': 4, 'line_type': 'list-item', 'coords': [100, 200, 900, 220],
          'content': '5.2 乡镇（街道）应科学配置垃圾转运站或垃圾处理终端设施，配备密闭的垃圾转运车辆。'}]


def row(rid, key, name, relation, lines, text=''):
    return {'id': int(rid.rsplit('_', 1)[1]), 'product_rel_id': rid, 'product_key': key, 'product_name': name,
            'product_name_en': 'Container', 'canonical_name': name, 'product_type': 'equipment', 'relation_type': relation,
            'relation_summary': text or name, 'evidence_lines': lines, 'obligation_level': 'mandatory' if text else 'descriptive',
            'requirement_text': text, 'conditions': [], 'exceptions': [], 'parameters': [], 'related_products': [],
            'confidence': 0.95, 'model_name': RUN['model_name'], 'prompt_name': '', 'create_time': '', 'modify_time': '',
            'rule_id': 'R1', 'section': '7.1 投放要求'}


GOLD = [row('416_gprd_117', '416_gp_99', '垃圾分类收集容器', 'design_requirement', ['89'], '垃圾分类收集容器应有盖'),
        row('416_gprd_118', '416_gp_99', '垃圾分类收集容器', 'certification_requirement', ['89'], '应在显著位置印制垃圾分类标志')]
DROPPED = [{'id': 7, 'candidate_id': 'c140', 'decision': 'excluded', 'drop_reason': 'X1', 'reason': 'A station built at a site.',
            'text': '垃圾转运站', 'source_line_spans': ['50'], 'product_keys': []}]


def score_fixture(locale):
    summary = {'score': 66, 'soft_precision': .78, 'soft_recall': .57, 'precision': .92, 'recall': .68,
               'gold_rows': 2, 'predicted_rows': 3, 'matched': 1}
    detection = {'gold_products': 2, 'predicted_products': 2, 'detected': 1, 'correct_predicted': 1,
                 'precision': .5, 'recall': .5, 'f1': .5, 'missed_products': [{'product_key': '416_gp_1', 'product_name': '垃圾转运车辆'}]}
    gold = [dict(GOLD[0]), dict(row('416_gprd_8', '416_gp_1', '垃圾转运车辆', 'requirement_target', ['50'], '配备密闭的垃圾转运车辆'))]
    preds = [dict(row('416_prd_14', '', '垃圾分类收集容器', 'design_requirement', ['89'], '垃圾分类收集容器应有盖')),
             dict(row('416_prd_2', '', '垃圾转运站', 'requirement_target', ['50'], '科学配置垃圾转运站')),
             dict(row('416_prd_3', '', '垃圾分类收集容器', 'usage_condition', ['89'], '印制垃圾分类标志'))]
    return {'id': 61, 'input_record_id': 416, 'title': '农村生活垃圾分类处理规范', 'lang': locale, 'status': 'done',
            'model_name': 'scorer-exact', 'prompt_name': 'prompt-score-extract-products-v1.md', 'created_by': 'smoke',
            'created_at': '2026-11-08T18:00:00Z', 'finished_at': '2026-11-08T18:01:00Z',
            'input': {'gold_run': {'skill_version': '1.0.0', 'model_name': RUN['model_name'],
                                   'benchmark_run_id': RUN['run_id'], 'rows': 2},
                      'extraction': {'model_names': ['deepseek-flash'], 'prompt_names': ['prompt-enrich-product-mention-v4.md']},
                      'warnings': [], 'gold': gold, 'predictions': preds},
            'score': {'score': 66, 'main': summary, 'product_detection': detection,
                      'groups': {'design_requirement': dict(summary, score=80), 'requirement_target': dict(summary, score=1)},
                      'groups_by_product_type': {'equipment': dict(summary, score=66)},
                      'field_accuracy': {'relation_type': 1, 'requirement_text': .9, 'product_type': 1, 'name': 1,
                                         'lines': 1, 'obligation_level': 1, 'parameters': None, 'conditions': None,
                                         'related_products': None, 'responsible_actor': None},
                      'pairs': [{'gold': '416_gprd_117', 'pred': '416_prd_14', 'note': '',
                                 'checks': {'relation_type': True, 'requirement_text': .9, 'product_type': True, 'name': True,
                                            'lines': 1, 'obligation_level': True, 'parameters': None, 'conditions': None,
                                            'related_products': None, 'responsible_actor': None}, 'credit': .98}],
                      'missed': [{'gold': '416_gprd_8', 'note': 'No prediction names 垃圾转运车辆', 'product_missed': True}],
                      'false_positives': [{'pred': '416_prd_2', 'cause': 'gold_excluded', 'note': 'facility'},
                                          {'pred': '416_prd_3', 'cause': 'extra_relation', 'gold': '416_gprd_117',
                                           'product_key': '416_gp_99', 'note': 'extra relation'}],
                      'overrides': []},
            'matches': {'pairs': []}, 'report': '# extract_products score — record 416\n**Score: 66 / 100**'}


async def json_route(route, payload, status=200):
    await route.fulfill(status=status, content_type='application/json', body=json.dumps(payload, ensure_ascii=False))


async def check_gold_page(ctx, labels, locale):
    page = await ctx.new_page()
    errors, calls = [], []
    page.on('pageerror', lambda e: errors.append(str(e)))

    async def products(route):
        q = parse_qs(urlparse(route.request.url).query)
        calls.append(q)
        assert q.get('source') == ['testbed'] and q.get('run_id') == [RUN['run_id']], q
        await json_route(route, {'status': True, 'input_id': 416, 'results': GOLD, 'total': len(GOLD), 'dropped': DROPPED})

    await page.route('**/api/v1/kb/products/gold-options', lambda r: json_route(r, {
        'status': True, 'skill_versions': ['1.0.0'],
        'options': [{'skill_version': '1.0.0', 'model_name': RUN['model_name'], 'record_count': 1, 'run_count': 1}]}))
    await page.route('**/api/v1/kb/products?*', products)
    await page.route('**/api/v1/kb/inputs?*', lambda r: json_route(r, {
        'status': True, 'results': [RECORD], 'page': 1, 'page_size': 20, 'total': 1}))
    await page.route('**/api/v1/kb/inputs/416', lambda r: json_route(r, {'status': True, 'record': RECORD}))
    await page.route('**/api/v1/kb/raw-lines?*', lambda r: json_route(r, {'status': True, 'lines': LINES}))
    await page.goto(f'{BASE}/development?view=system-admin&child=sysadmin-llm-products-gold')
    await page.get_by_text(labels['products_gold_title'], exact=True).first.wait_for(timeout=20000)
    await page.locator('.gold-view select').wait_for()
    assert await page.locator('.gold-view option[value="dropped"]').inner_text() == labels['products_gold_view_dropped']
    await page.get_by_text(RECORD['title'], exact=False).first.click()
    await page.locator('.scene-row').first.wait_for()
    assert calls, 'gold products were not requested'
    assert await page.locator('.scene-row').count() == 2
    await page.get_by_text(GOLD[0]['relation_summary'], exact=False).first.wait_for()
    await page.locator('.gold-view select').select_option('dropped')
    await page.get_by_text('垃圾转运站', exact=False).first.wait_for()
    assert await page.locator('.scene-row').count() == 1
    assert len(calls) == 2, 'switching Gold/Dropped must reload the selected run'
    await page.locator('.gold-view select').select_option('gold')
    await page.wait_for_function("document.querySelectorAll('.scene-row').length === 2")
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
                                'benchmark_run_id': RUN['run_id'], 'rows': 188}]
        elif req.method == 'POST':
            posts.append(json.loads(req.post_data))
            payload['run'] = done
        elif path.endswith('/61'):
            payload['run'] = done
        else:
            payload.update(runs=[done], total=1)
        await json_route(route, payload, 202 if req.method == 'POST' else 200)

    await page.route('**/api/v1/kb/product-scores**', scores)
    await page.route('**/api/v1/kb/inputs?*', lambda r: json_route(r, {
        'status': True, 'results': [RECORD], 'page': 1, 'page_size': 50, 'total': 1}))
    await page.route('**/api/v1/kb/raw-lines?*', lambda r: json_route(r, {'status': True, 'lines': LINES}))
    await page.goto(f'{BASE}/development?view=system-admin&child=sysadmin-llm-products-benchmark')
    view = page.locator('.benchmark').first
    await view.get_by_text(labels['prsc_title'], exact=True).first.wait_for(timeout=20000)
    assert await view.evaluate('(el) => getComputedStyle(el).userSelect') == 'text'
    await view.locator('.documents button').first.click()
    await view.get_by_role('button', name=labels['msc_run'], exact=True).click()
    results = view.locator('.results')
    await results.get_by_text(labels['prsc_detection'], exact=True).wait_for()
    assert posts and posts[0]['record_id'] == 416 and posts[0]['lang'] == locale, posts
    await results.get_by_text(labels['prsc_by_relation'], exact=True).wait_for()
    await results.get_by_text(labels['prsc_by_product_type'], exact=True).wait_for()
    await results.get_by_text(labels['prsc_cause_extra_relation'], exact=False).first.wait_for()
    await view.get_by_text(labels['prsc_detection_f1'], exact=True).first.wait_for()
    assert await results.locator('.matched-metrics .result-entry').count() == 1
    missed = results.locator('details').filter(has_text=labels['prsc_missed'].split('{')[0].strip()).locator('.result-entry')
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
