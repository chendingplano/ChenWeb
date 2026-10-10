import argparse
import json
from pathlib import Path
from urllib.parse import urlparse,parse_qs
from playwright.sync_api import sync_playwright
ROOT=Path(__file__).resolve().parents[2]
parser=argparse.ArgumentParser(description='Benchmark gold/dropped smoke test with mocked APIs; requires a running ChenWeb web server and Python Playwright.')
parser.add_argument('--base-url',default='http://127.0.0.1:5173')
BASE=parser.parse_args().base_url.rstrip('/')
with sync_playwright() as p:
 b=p.chromium.launch(headless=True)
 for locale in ['en','zh-cn']:
  labels=json.loads((ROOT/'web/messages'/f'{locale}.json').read_text())
  ctx=b.new_context(viewport={'width':1600,'height':1000})
  ctx.add_cookies([{'name':'PARAGLIDE_LOCALE','value':locale,'url':BASE}])
  page=ctx.new_page();requests=[]
  record={'id':753,'title':'Benchmark drop fixture','name':'fixture.pdf','type':'txt','status':[], 'create_time':'2026-10-10T05:00:00-05:00','modify_time':'2026-10-10T05:00:00-05:00', 'gold_metric_runs':[{'skill_version':'6.0.0','model_name':'gold-model','run_id':'20261010_055329'}]}
  shared={'input_record_id':753,'input_filename':'fixture.pdf','run_id':'20261010_055329','skill_name':'extract-metrics-benchmark','skill_version':'6.0.0','model_name':'gold-model','source_line_spans':['1'],'source_table_rows':[]}
  gold=dict(shared,id=1,metric_id='753_gold_1',metric_name='Gold pressure',metric_value='0.5',metric_unit='MPa')
  drop=dict(shared,id=-2,metric_id='c2',metric_name='Benchmark exclusion',metric_desc='No value in source',dropped=True,drop_stage='benchmark_rule',drop_reason='X18',drop_decision={'reason':'No value in source'})
  def handle(route):
   url=urlparse(route.request.url);path=url.path;qs=parse_qs(url.query)
   payload={'status':True,'results':[],'total':0}
   if path.endswith('/metrics/gold-options'):
    payload.update(skill_versions=['6.0.0'],options=[{'skill_version':'6.0.0','model_name':'gold-model','document_count':1,'run_count':1}])
   elif path.endswith('/metrics'):
    requests.append(qs)
    assert qs['source']==['testbed'] and qs['include_dropped']==['true'] and qs['model_name']==['gold-model'] and qs['run_id']==[shared['run_id']],qs
    payload.update(results=[gold,drop],total=2,run_id=shared['run_id'])
   elif path.endswith('/inputs/753'):
    payload['record']=record
   elif path.endswith('/inputs'):
    payload.update(results=[record],total=1,page=1,page_size=20)
   elif path.endswith('/raw-lines'):
    payload['lines']=[{'page_number':1,'line_number':1,'line_type':'paragraph','content':'Pressure shall be 0.5 MPa.'}]
   route.fulfill(status=200,content_type='application/json',body=json.dumps(payload))
  ctx.route('**/api/v1/kb/**',handle)
  page.goto(BASE+'/development');page.wait_for_load_state('networkidle')
  for key in ['nav_system_admin','nav_sysadmin_llm','nav_sysadmin_llm_metrics','nav_sysadmin_llm_metrics_gold']:
   page.get_by_role('button',name=labels[key],exact=True).first.click()
  view=page.locator('.metric-mgmt');view.wait_for()
  assert view.locator('.record-card').filter(has_text=shared['run_id']).count()==1
  page.get_by_text('Benchmark drop fixture',exact=True).first.click()
  view.locator('.metric-card').get_by_text('Gold pressure',exact=True).first.wait_for()
  assert not view.locator('.metric-card').get_by_text('Benchmark exclusion',exact=True).count()
  requests_before=len(requests)
  toggle=view.locator('select').filter(has=page.locator('option[value="dropped"]'))
  view.locator('.metric-filter-toggle').click()
  toggle.select_option('dropped')
  view.locator('.metric-card').get_by_text('Benchmark exclusion',exact=True).first.wait_for()
  assert not view.locator('.metric-card').get_by_text('Gold pressure',exact=True).count()
  assert len(requests)==requests_before,'Switching views unexpectedly reselected a run'
  view.locator('.metric-card').get_by_text('Benchmark exclusion',exact=True).first.click()
  page.get_by_text(shared['run_id'],exact=True).first.wait_for()
  assert page.get_by_text(labels['metric_mgmt_run_id'],exact=True).count()>0
  assert view.evaluate('(el)=>getComputedStyle(el).userSelect')=='text'
  page.screenshot(path=f'/tmp/benchmark-drops-{locale}.png',full_page=True)
  print(locale, 'benchmark source + same run + cached toggle + run details: passed')
  ctx.close()
 b.close()
