import asyncio,json,pathlib,argparse,tempfile
from urllib.parse import urlparse,parse_qs
from playwright.async_api import async_playwright
ROOT=pathlib.Path(__file__).resolve().parents[2]
parser=argparse.ArgumentParser(description='Benchmark page smoke test with mocked APIs; requires a running ChenWeb web server and Python Playwright.')
parser.add_argument('--base-url',default='http://localhost:5173')
BASE=parser.parse_args().base_url.rstrip('/')
SCREENSHOTS=pathlib.Path(tempfile.mkdtemp(prefix='chenweb-metric-score-smoke-'))

def fixture():
 summary={'score':70,'soft_precision':.7,'soft_recall':.7,'precision':1,'recall':1,'gold_rows':1,'predicted_rows':1,'matched':1}
 return {'id':91,'input_record_id':416,'title':'Benchmark fixture','lang':'en','status':'done','model_name':'scorer-exact','prompt_name':'prompt-score-extract-metrics-v1.md','created_by':'smoke','created_at':'2026-10-07T18:00:00Z','finished_at':'2026-10-07T18:01:00Z','input':{'gold_run':{'skill_version':'3.0.0','model_name':'gold-a','benchmark_run_id':'20261007_120000','rows':1},'extraction':{'prompts_models_events':[['extract-v11','extractor','event1']],'created_at':['2026-10-07','2026-10-07']},'warnings':['Fixture source changed; line checks disabled'],'gold':[{'metric_id':'g1','metric_name':'Temperature','metric_value':'20','metric_unit':'C'}],'predictions':[{'metric_id':'p1','metric_name':'Temperature','metric_value':'30','metric_unit':'C'}],'provenance':{'implementation_sha256':{'score_io.py':'fixture-hash'},'model_profile':'scorer-profile'}},'score':{'score':70,'main':summary,'without_test_parameters':dict(summary,score=75),'field_accuracy':{'value':0,'range_type':1,'kind':1,'unit':1,'lines':None},'pairs':[{'gold':'g1','pred':'p1','note':'same assertion','checks':{'value':False,'range_type':True,'kind':True,'unit':True,'lines':None},'credit':.7}],'missed':[],'false_positives':[],'overrides':[]},'matches':{'pairs':[{'gold':'g1','pred':'p1'}]},'report':'# Canonical scorer report\nScore: 70 / 100'}

async def run():
 async with async_playwright() as p:
  browser=await p.chromium.launch(headless=True)
  for locale in ['en','zh-cn']:
   labels=json.loads((ROOT/'web/messages'/f'{locale}.json').read_text())
   ctx=await browser.new_context(viewport={'width':1600,'height':1100})
   await ctx.add_cookies([{'name':'PARAGLIDE_LOCALE','value':locale,'url':BASE}])
   page=await ctx.new_page();errors=[];requests=[];running=False;polls=0
   page.on('pageerror',lambda e:errors.append(str(e)))
   done=fixture();done['lang']=locale
   # Exercise each failure reason, multiple failures, and unchecked fields at full credit.
   for field in ['kind','unit','lines','range_type']:
    checks=dict(value=True,range_type=True,kind=True,unit=True,lines=True);checks[field]=False
    done['score']['pairs'].append({'gold':'g-'+field,'pred':'p-'+field,'checks':checks,'credit':.8})
   done['score']['pairs'].append({'gold':'g-multi','pred':'p-multi','checks':{'kind':False,'unit':False},'credit':.65})
   done['score']['pairs'].append({'gold':'g-full','pred':'p-full','checks':{'value':True,'range_type':True,'kind':True,'unit':True,'lines':None},'credit':1})
   gold=[{'skill_version':'3.0.0','model_name':'gold-a','benchmark_run_id':'20261007_120000','rows':1},{'skill_version':'4.0.0','model_name':'gold-b','benchmark_run_id':'20261007_120000','rows':1}]
   async def handle(route):
    nonlocal running,polls
    req=route.request;u=urlparse(req.url);path=u.path;q=parse_qs(u.query)
    requests.append((req.method,path,q,req.post_data))
    payload={'status':True}
    if path.endswith('/models'):payload['models']=['scorer-profile']
    elif path.endswith('/gold-runs'):payload['runs']=gold
    elif '/artifacts/' in path:
     await route.fulfill(status=200,content_type='application/json',body='{"fixture":true}',headers={'Content-Disposition':'attachment; filename="fixture.json"'});return
    elif req.method=='POST':
     body=json.loads(req.post_data)
     assert body['lang']==locale and body['record_id']==416
     assert body['gold_version']=='4.0.0' and body['gold_model']=='gold-b'
     running=True;payload['run']=dict(done,status='running',input=None,matches=None,score=None,report='')
    elif path.endswith('/91'):
     polls+=1
     if running:
      # Longer than one polling interval: the completed result must still be displayed.
      await asyncio.sleep(5);running=False
     payload['run']=done
    else:
     payload.update(runs=[dict(done,status='running',score=None) if running else done],total=21)
    await route.fulfill(status=202 if req.method=='POST' else 200,content_type='application/json',body=json.dumps(payload))
   await ctx.route('**/api/v1/kb/metric-scores**',handle)
   await page.route('**/api/v1/kb/inputs?*',lambda route:route.fulfill(status=200,content_type='application/json',body=json.dumps({'status':True,'results':[{'id':416,'title':'Benchmark fixture'}]})))
   await page.goto(BASE+'/development');await page.wait_for_load_state('networkidle')
   for key in ['nav_system_admin','nav_sysadmin_llm','nav_sysadmin_llm_metrics','nav_sysadmin_llm_metrics_benchmark']:
    await page.get_by_role('button',name=labels[key],exact=True).first.click()
   view=page.locator('.benchmark');await view.wait_for()
   assert await view.evaluate('(el)=>getComputedStyle(el).userSelect')=='text'
   await view.get_by_role('button',name=labels['msc_open'],exact=True).click()
   await view.get_by_text('Temperature',exact=False).first.wait_for()
   await view.get_by_role('button',name=labels['msc_close'],exact=True).click()
   await view.get_by_role('textbox').fill('416')
   await view.get_by_role('button',name=labels['msc_search'],exact=True).click()
   await view.locator('.documents button').click()
   await view.locator('.gold select').select_option(json.dumps(['4.0.0','gold-b','20261007_120000'],separators=(',',':')))
   await view.get_by_role('button',name=labels['msc_run'],exact=True).click()
   await view.locator('.results').get_by_text(labels['msc_background'],exact=True).wait_for()
   await view.locator('.results').get_by_text('Temperature',exact=False).first.wait_for(timeout=16000)
   assert polls>=2
   await view.get_by_role('button',name=labels['msc_next'],exact=True).click()
   await asyncio.sleep(.25)
   assert any(q.get('offset')==['20'] for _,_,q,_ in requests)
   await view.locator('.section-head select').select_option('selected')
   await asyncio.sleep(.25)
   assert any(q.get('record_id')==['416'] for _,path,q,_ in requests if path.endswith('/metric-scores'))
   results=view.locator('.results');pairs=results.locator('.matched-metrics article')
   match_filter=results.locator('.pair-filter select')
   assert await pairs.count()==7
   for choice,count in [('full',1),('partial',6),('kind',2),('unit',2),('lines',1),('value',1),('range_type',1),('all',7)]:
    await match_filter.select_option(choice)
    assert await pairs.count()==count,(choice,await pairs.count())
   for kind,key in [('input','msc_input_download'),('matches','msc_matches_download'),('score','msc_score_download'),('report','msc_report_download')]:
    menu=results.get_by_role('combobox',name=labels[key],exact=True)
    await menu.select_option('view')
    preview=results.locator('.evidence-view pre');await preview.wait_for()
    data=done[kind]
    if kind=='report':assert await preview.inner_text()==data
    else:assert json.loads(await preview.inner_text())==data
    assert await menu.input_value()==''
    async with page.expect_download() as download_info:
     await menu.select_option('download')
    download=await download_info.value
    assert download.suggested_filename==f'benchmark-91-{kind}.'+('md' if kind=='report' else 'json')
    assert await download.failure() is None
    assert pathlib.Path(await download.path()).read_text()=='{"fixture":true}'
    assert any(path.endswith('/artifacts/'+kind) for _,path,_,_ in requests)
   await results.get_by_role('button',name=labels['msc_close_evidence'],exact=True).click()
   assert await results.locator('.evidence-view').count()==0
   await match_filter.select_option('unit')
   await view.get_by_role('button',name=labels['msc_open'],exact=True).click()
   assert await match_filter.input_value()=='all'
   assert await pairs.count()==7
   # A full-credit-only result gives an explicit empty state for a failed-field filter.
   done['score']['pairs']=[done['score']['pairs'][-1]]
   await view.get_by_role('button',name=labels['msc_open'],exact=True).click()
   await match_filter.select_option('lines')
   await results.get_by_text(labels['msc_no_matching_pairs'],exact=True).wait_for()
   assert await pairs.count()==0
   await match_filter.select_option('all')
   await page.screenshot(path=str(SCREENSHOTS/f'benchmark-{locale}.png'),full_page=True)
   assert not errors,errors
   print(f'{locale}: navigation, search, composite gold, launch, slow polling, details, pagination/filter, evidence previews/downloads, all credit/reason filters, empty state/reset, text selection passed')
   await ctx.close()
  await browser.close()
asyncio.run(run())
print(f'Screenshots: {SCREENSHOTS}')
