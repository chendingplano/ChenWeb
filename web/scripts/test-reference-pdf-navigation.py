"""References PDF and Prev/Next navigation smoke test; requires a running web server and Playwright."""
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
record={'id':416,'title':'Source PDF fixture','file_name':'source.pdf','type':'pdf'}
refs=[{'id':i,'ref_id':f'416_REF_{i}','target_title':title,'source_line_spans':spans,'target_type':'standard','target_subtype':'national_standard','target_content':'general','normativity':'normative','identifiers':[]} for i,title,spans in [(1,'First reference',['40']),(2,'Second reference',['80-81']),(3,'Same page reference',['90']),(4,'Multiple page reference',['80','100'])]]
lines=[{'line_number':n,'page_number':pg,'coords':[100,y,600,y+25],'content':f'Source line {n}','line_type':'text'} for n,pg,y in [(40,2,200),(80,4,500),(81,4,535),(90,4,800),(100,5,200)]]
source_failed = True
with sync_playwright() as p:
 b=p.chromium.launch(headless=True);ctx=b.new_context(viewport={'width':1900,'height':1000});ctx.add_cookies([{'name':'PARAGLIDE_LOCALE','value':args.locale,'url':BASE}]);page=ctx.new_page();errors=[];page.on('pageerror',lambda e:errors.append(str(e)))
 diagnostics=[]
 page.on('console',lambda msg:diagnostics.append(msg.text) if '[References PDF 20261011-731]' in msg.text else None)
 def handle(route):
  path=urlparse(route.request.url).path
  if not path.startswith('/api/v1/kb/'):
   route.continue_();return
  payload={'status':True,'results':[]}
  if path.endswith('/416/file'):route.fulfill(content_type='application/pdf',body=data);return
  if path.endswith('/stores'):payload['results']=[{'id':1,'ks_name':'Navigation fixture','description':'Fixture','is_active':True}]
  elif path.endswith('/inputs/416'):payload['record']=record
  elif path.endswith('/inputs'):payload.update(results=[record],total=1)
  elif path.endswith('/references'):payload['results']=refs
  elif path.endswith('/raw-lines'):
   if source_failed:
    route.fulfill(status=500,content_type='application/json',body=json.dumps({'status':False,'error_msg':'Source lines unavailable'}));return
   payload.update(lines=lines,pages=5,input_id=416)
  route.fulfill(content_type='application/json',body=json.dumps(payload))
 ctx.route('**/api/**',handle)
 page.goto(BASE+'/home3/knowledge');page.wait_for_load_state('networkidle');page.get_by_text('Navigation fixture',exact=True).first.click()
 page.get_by_role('button',name=labels['home3_knowledge_wiki'],exact=False).first.click();page.get_by_role('button',name=labels['references_mgmt_title'],exact=False).first.click()
 view=page.locator('.references-page');view.wait_for();view.locator('.records input[inputmode="numeric"]').fill('416');view.get_by_role('button',name=labels['kb_input_record_browser_retrieve'],exact=False).click()
 view.locator('.notice.error').wait_for()
 assert labels['references_mgmt_source_lines_load_failed'].split('{error}')[0] in view.locator('.notice.error').inner_text()
 assert view.locator('.mention-card').count()==0
 source_failed=False
 view.get_by_role('button',name=labels['kb_input_record_browser_retrieve'],exact=False).click()
 page.locator('.pdf-page canvas').first.wait_for(timeout=20000)
 for index,pg,expected in [(0,2,1),(1,4,2),(2,4,1),(3,4,2),(0,2,1),(1,4,2)]:
  view.locator('.mention-card').nth(index).click()
  page.wait_for_function('(count)=>document.querySelectorAll(".document .pdf-highlight").length===count',arg=expected)
  marks=page.locator('.document .pdf-highlight')
  assert marks.count()==expected
  mark=page.locator(f'.document [id$="-overlay-{pg}"] .pdf-highlight').first
  page.wait_for_function('''page => {
   const mark=document.querySelector(`[id$="-overlay-${page}"] .pdf-highlight`);
   const host=document.querySelector('.pdf-canvas-host');
   if(!mark || !host) return false;
   const a=mark.getBoundingClientRect(), b=host.getBoundingClientRect();
   return a.top>=b.top && a.bottom<=b.bottom;
  }''',arg=pg)
  box=mark.bounding_box();host=page.locator('.pdf-canvas-host').bounding_box()
  document=view.locator('.document').bounding_box()
  assert host and document and host['height']<=document['height'],(host,document)
  assert box and document and box['y']>=document['y'] and box['y']+box['height']<=document['y']+document['height'],(box,document)
  assert page.locator('.pdf-canvas-host').evaluate('el=>el.scrollHeight>el.clientHeight')
  if pg>=4: assert page.locator('.pdf-canvas-host').evaluate('el=>el.scrollTop>0')
  assert box and host and box['y']>=host['y'] and box['y']+box['height']<=host['y']+host['height'],(box,host)
 prev=view.get_by_role('button',name=labels['references_mgmt_prev'],exact=True)
 next_button=view.get_by_role('button',name=labels['references_mgmt_next'],exact=True)
 assert abs(prev.bounding_box()['y']-next_button.bounding_box()['y'])<1
 for direction,index in [('prev',0),('next',1),('next',2),('next',3),('prev',2),('next',3)]:
  (prev if direction=='prev' else next_button).click()
  assert view.locator('.mention-card.active').count()==1
  assert view.locator('.mention-card').nth(index).get_attribute('aria-pressed')=='true'
  expected_lines=[line['line_number'] for line in lines if str(line['line_number']) in refs[index]['source_line_spans'] or (index==1 and line['line_number'] in [80,81])]
  page.wait_for_function('(expected)=>JSON.stringify([...document.querySelectorAll(".document .pdf-highlight")].map(el=>Number(el.title.split(" ")[1])))===JSON.stringify(expected)',arg=expected_lines)
  if index==0: assert prev.is_disabled()
  if index==len(refs)-1: assert next_button.is_disabled()
 view.locator('.reference-filters summary').click()
 text_filter=view.get_by_role('textbox',name=labels['references_mgmt_filter_by_text'],exact=True)
 text_filter.fill('Second reference')
 assert view.locator('.mention-card').count()==1
 assert prev.is_disabled() and next_button.is_enabled()
 next_button.click()
 assert view.locator('.mention-card.active').count()==1
 assert prev.is_disabled() and next_button.is_disabled()
 text_filter.fill('no matching reference')
 assert prev.is_disabled() and next_button.is_disabled()
 assert not diagnostics,diagnostics
 assert not errors,errors
 print(f'PASS ({args.locale}): PDF highlighting, Prev/Next boundaries and filters, source-load failure and retry; no reference debug logs.')
 b.close()
