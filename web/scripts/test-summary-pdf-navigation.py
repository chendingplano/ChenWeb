"""Summary PDF navigation regression test; requires a running web server and Playwright."""
import argparse,json,pathlib
from urllib.parse import urlparse
from playwright.sync_api import sync_playwright
ROOT=pathlib.Path(__file__).resolve().parents[1]
parser=argparse.ArgumentParser(description=__doc__)
parser.add_argument('--base-url',default='http://localhost:5173')
parser.add_argument('--locale',choices=['en','zh-cn'],default='en')
args=parser.parse_args()
BASE=args.base_url.rstrip('/')
labels=json.loads((ROOT/'messages'/f'{args.locale}.json').read_text())
# A real five-page PDF, rendered through PDF.js rather than a viewer stub.
objects=['<< /Type /Catalog /Pages 2 0 R >>','<< /Type /Pages /Kids [3 0 R 5 0 R 7 0 R 9 0 R 11 0 R] /Count 5 >>']
for i in range(5):
 objects += [f'<< /Type /Page /Parent 2 0 R /MediaBox [0 0 600 800] /Resources << >> /Contents {4+i*2} 0 R >>','<< /Length 0 >>\nstream\n\nendstream']
data=b'%PDF-1.4\n';offsets=[0]
for i,o in enumerate(objects,1):
 offsets.append(len(data));data+=f'{i} 0 obj\n{o}\nendobj\n'.encode()
xref=len(data);data+=f'xref\n0 {len(objects)+1}\n0000000000 65535 f \n'.encode()
for offset in offsets[1:]:data+=f'{offset:010} 00000 n \n'.encode()
data+=f'trailer\n<< /Size {len(objects)+1} /Root 1 0 R >>\nstartxref\n{xref}\n%%EOF'.encode()
record={'id':416,'title':'Summary PDF fixture','file_name':'source.pdf','type':'pdf'}
summaries=[{'id':i,'summary_id':f'416_sum_{i}','summary_seq_no':i,'summary_level':0,'summary_text':f'Summary {i}','source_line_spans':spans,'language':'en','skill_version':'1.0.0','model_name':'fixture','created_at':'2026-10-21T12:00:00Z'} for i,spans in [(1,['40']),(2,['80-81']),(3,['90']),(4,['80','100'])]]
lines=[{'line_number':n,'page_number':pg,'coords':[100,y,600,y+25],'content':f'Source line {n}','line_type':'text'} for n,pg,y in [(40,2,200),(80,4,500),(81,4,535),(90,4,800),(100,5,200)]]
with sync_playwright() as p:
 browser=p.chromium.launch(headless=True)
 ctx=browser.new_context(viewport={'width':1900,'height':1000})
 ctx.add_cookies([{'name':'PARAGLIDE_LOCALE','value':args.locale,'url':BASE}])
 page=ctx.new_page();errors=[]
 page.on('pageerror',lambda e:errors.append(str(e)))
 def handle(route):
  path=urlparse(route.request.url).path
  if not path.startswith('/api/v1/kb/'):
   route.continue_();return
  payload={'status':True,'results':[]}
  if path.endswith('/416/file'):route.fulfill(content_type='application/pdf',body=data);return
  if path.endswith('/gold-options'):payload.update(skill_versions=['1.0.0'],options=[])
  elif path.endswith('/summaries/gold'):payload.update(summaries=summaries,record_id=416)
  elif path.endswith('/inputs/416'):payload['record']=record
  elif path.endswith('/inputs'):payload.update(results=[record],total=1)
  elif path.endswith('/raw-lines'):payload.update(lines=lines,pages=5,input_id=416)
  route.fulfill(content_type='application/json',body=json.dumps(payload))
 ctx.route('**/api/**',handle)
 page.goto(BASE+'/development?view=system-admin&child=sysadmin-llm-summaries-gold')
 page.wait_for_load_state('networkidle')
 view=page.locator('.benchmark-page');view.wait_for()
 view.locator('input[inputmode="numeric"]').fill('416')
 view.get_by_role('button',name=labels['kb_input_record_browser_retrieve'],exact=False).click()
 view.locator('.summary-card').first.click()
 page.wait_for_function('document.querySelectorAll(".pdf-highlight").length > 0')
 for index,pg in [(0,2),(1,4),(2,4),(3,4),(0,2)]:
  view.locator('.summary-card').nth(index).click()
  expected_lines = [[40], [80,81], [90], [80,100]][index]
  page.wait_for_function('(expected)=>JSON.stringify([...document.querySelectorAll(".pdf-highlight")].map(el=>Number(el.title.split(" ")[1])))===JSON.stringify(expected)',arg=expected_lines)
  page.wait_for_function('(pg)=>document.querySelector(".page-input").value===String(pg)',arg=pg)
  host=view.locator('.pdf-canvas-host')
  dimensions=host.evaluate('el=>({height:el.clientHeight,scrollHeight:el.scrollHeight,scrollTop:el.scrollTop})')
  assert dimensions['height']<1000,dimensions
  assert dimensions['scrollHeight']>dimensions['height'],dimensions
  page.wait_for_function('pg=>{\n   const mark=document.querySelector(`[id$="-overlay-${pg}"] .pdf-highlight`);\n   const host=document.querySelector(\'.pdf-canvas-host\');\n   if(!mark||!host)return false;\n   const a=mark.getBoundingClientRect(),b=host.getBoundingClientRect();\n   return a.top>=b.top&&a.bottom<=b.bottom&&a.bottom<=innerHeight;\n  }',arg=pg)
 assert not errors,errors
 print(f'PASS ({args.locale}): summary selection scrolls the bounded PDF pane to visible source highlights, including same-page and multi-page summaries.')
 browser.close()
