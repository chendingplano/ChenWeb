"""Integration checks against the real WPI Editor with HTTP fixtures only."""
import socket
import json
import subprocess
import tempfile
import time
from pathlib import Path
from urllib.parse import urlsplit
from playwright.sync_api import sync_playwright, expect

web = Path(__file__).resolve().parents[2]
text_class = {'descriptor_id': 'text', 'schema_version': '0.4', 'code_module_id': 'text', 'text': {'default_value': 'Original class value', 'scope': 'instance', 'value_editor': {'id': 'localized-text', 'options': {'rows': 2}}}}
page_descriptor = {
 'descriptor_id': '', 'schema_version': '0.5',
 'page_layouter': {'layouter': 'single-pane', 'root': {'pane_id': 'main'}},
 'panels': {'main': {'elem_layouter': {'layouter': 'flow-layouter'}, 'elem_list': []}}
}
node = {'id': 'text-fixture', 'type': 'wpi-element', 'hidden': True, 'position': {'x': 20, 'y': 20}, 'data': {'label': 'Text fixture', 'element': 'text', 'pane': 'main', 'attrs': {'text': 'Instance text'}, 'class': text_class}}
flow = {'flow_id': 1, 'user_id': 1, 'flow_name': 'Fixture page', 'flow_desc': '', 'is_default': True, 'is_shared': False, 'is_template': False, 'template_category': '', 'flow_data': {'nodes': [node], 'edges': [], 'wpi_page': page_descriptor}, 'thumbnail_svg': None, 'created_at': '2026-11-02T00:00:00Z', 'updated_at': '2026-11-02T00:00:00Z'}
preview_page = {**page_descriptor, 'descriptor_id': 'preview-page'}
preview_bundle = {'app': {'descriptor_id': 'preview-app', 'schema_version': '0.5', 'initial_page': 'preview-page'}, 'pages': {'preview-page': preview_page}, 'warnings': []}

with tempfile.TemporaryFile(mode='w+') as log:
 process = subprocess.Popen([str(web / 'node_modules/.bin/vite'), '--config', 'tests/wpi-class-editor/vite.config.ts'], cwd=web, stdout=log, stderr=log)
 try:
  deadline = time.monotonic() + 30
  while True:
   try:
    with socket.create_connection(('127.0.0.1', 5198), timeout=.5): break
   except OSError:
    if process.poll() is not None or time.monotonic() > deadline:
     log.seek(0); raise RuntimeError(log.read())
    time.sleep(.2)
  with sync_playwright() as p:
   browser = p.chromium.launch(headless=True)
   page = browser.new_page(viewport={'width': 1600, 'height': 1100})
   page.set_default_timeout(15000)
   errors, requests, saved_classes = [], [], []
   page.on('pageerror', lambda error: errors.append(str(error)))
   page.on('requestfailed', lambda request: print('Failed request:', request.url, request.failure))
   page.on('response', lambda response: print('HTTP error:', response.status, response.url) if response.status >= 400 else None)
   page.on('console', lambda message: print('Console error:', message.text) if message.type == 'error' else None)
   def api(route):
    path = urlsplit(route.request.url).path
    if not path.startswith('/api/v1/'):
     route.continue_(); return
    requests.append(route.request.url)
    if path == '/api/v1/flow-node-types': body = {'nodeTypes': []}
    elif path == '/api/v1/flows/default': body = {'flow': flow}
    elif path == '/api/v1/wpi/recoveries/unsaved': body = {'recoveries': []}
    elif path == '/api/v1/wpi/classes/element': body = {'classes': [{'descriptor_id': 'text', 'version': 1, 'status': 'active', 'description': None, 'code_module_id': 'text', 'attributes': {'text': {'default_value': 'Original class value', 'scope': 'instance', 'value_editor': {'id': 'localized-text', 'options': {'rows': 2}}}}}]}
    elif path == '/api/v1/wpi/classes/action': body = {'actions': ['save', 'open-page', 'load']}
    elif path == '/api/v1/wpi/classes/page-layouter': body = {'classes': [{'descriptor_id': 'single-pane', 'schema_version': '0.5', 'code_module_id': 'page-layouter', 'root': {'pane_id': 'main'}}]}
    elif path == '/api/v1/wpi/classes/element/text':
     if route.request.method == 'PUT':
      saved_classes.append(route.request.post_data_json)
      body = {'version': 7, 'status': 'active', 'warnings': []}
     else: body = {'version': 7, 'status': 'active', 'class': saved_classes[-1] if saved_classes else text_class}
    elif path == '/api/v1/wpi/classes/action/save': body = {'version': 1, 'status': 'active', 'class': {'descriptor_id': 'save', 'schema_version': '0.2', 'table': None, 'mode': 'insert'}}
    elif path == '/api/v1/wpi/pages/preview': body = preview_bundle
    elif path == '/api/v1/flows': body = {'flows': []}
    elif path.endswith('/recovery'): body = {'page': None}
    else: body = {}
    route.fulfill(json=body)
   page.route('**/api/**', api)
   page.goto('http://127.0.0.1:5198/?full=1', wait_until='networkidle')
   try:
    expect(page.get_by_role('tab', name='Class', exact=True)).to_be_visible()
    page.locator('[data-flow-item] button.overlay').click()
    page.get_by_role('tab', name='Class', exact=True).click()
    expect(page.get_by_role('heading', name='Class: text')).to_be_visible()
    expect(page.locator('[data-class-version]')).to_have_text('7')
    expect(page.locator('[data-class-schema-version]')).to_have_text('0.4')
    page.locator('button.attribute').filter(has=page.locator('span.name', has_text='text')).click()
    default = page.locator('[id="class-default-/text"]')
    default.fill('Unsaved class draft'); default.blur()
    page.locator('[id="class-editor-/text"]').select_option('multiline-text')
    page.locator('[id="class-options-/text-rows"]').fill('3')
    page.locator('[id="class-options-/text-rows"]').blur()
    page.get_by_text('Editor options as JSON', exact=True).click()
    expect(page.get_by_text('Options for the multiline-text value editor only. An empty object uses its defaults.', exact=True)).to_be_visible()
    assert json.loads(page.locator('[id="class-options-/text-json"]').input_value()) == {'rows': 3}
    expect(page.get_by_role('button', name='Save class', exact=True)).to_be_enabled()
    # Code/View JSON must show the same unsaved class body, not instance values.
    page.get_by_role('button', name='Code', exact=True).click()
    page.get_by_role('menuitem', name='View JSON', exact=True).click()
    dialog = page.get_by_role('dialog')
    expect(dialog).to_contain_text('Class JSON: text')
    body = json.loads(dialog.locator('pre.json-text').inner_text())
    assert body['descriptor_id'] == 'text' and body['text']['default_value'] == 'Unsaved class draft', body
    assert 'instance_id' not in body and 'element' not in body, body
    dialog.get_by_role('button', name='Edit', exact=True).click()
    invalid_body = {**body, 'schema_version': 'unsupported'}
    dialog.locator('textarea.json-text').fill(json.dumps(invalid_body))
    dialog.get_by_role('button', name='Apply', exact=True).click()
    expect(dialog).to_be_visible()
    expect(page.locator('[data-class-schema-version]')).to_have_text('0.4')
    body['text']['default_value'] = 'Edited class JSON'
    dialog.locator('textarea.json-text').fill(json.dumps(body))
    dialog.get_by_role('button', name='Apply', exact=True).click()
    expect(default).to_have_value('Edited class JSON')
    page.get_by_role('button', name='Save class', exact=True).click()
    expect(page.get_by_role('button', name='Save class', exact=True)).to_be_disabled()
    assert saved_classes[-1] == body, saved_classes
    expect(page.locator('[data-class-version]')).to_have_text('7')
    # Properties retains the instance JSON, separately from the stored class.
    page.get_by_role('tab', name='Properties', exact=True).click()
    page.get_by_role('button', name='Code', exact=True).click()
    page.get_by_role('menuitem', name='View JSON', exact=True).click()
    instance_body = json.loads(page.get_by_role('dialog').locator('pre.json-text').inner_text())
    assert instance_body['instance_id'] == 'text-fixture' and instance_body['text'] == 'Instance text', instance_body
    assert 'descriptor_id' not in instance_body, instance_body
    page.get_by_role('dialog').get_by_role('button', name='Close', exact=True).click()
    page.get_by_role('tab', name='Class', exact=True).click()
    default.fill('Unsaved class draft'); default.blur()
    panel = page.locator('.properties-container')
    panel.screenshot(path='/tmp/wpi-class-editor.png')
    assert panel.evaluate('(el) => el.scrollWidth <= el.clientWidth'), 'Class panel overflows horizontally'
    page.get_by_role('button', name='Preview', exact=True).click()
    expect(page.get_by_role('button', name='Edit', exact=True)).to_be_visible()
    expect(page.get_by_role('tab', name='Class', exact=True)).not_to_be_visible()
    page.get_by_role('button', name='Edit', exact=True).click()
    expect(page.get_by_role('heading', name='Class: text')).to_be_visible()
    expect(default).to_have_value('Unsaved class draft')
    expect(page.get_by_role('button', name='Save class', exact=True)).to_be_enabled()

    page.get_by_role('button', name='Create', exact=True).click()
    page.get_by_role('menuitem', name='Create Action', exact=True).click()
    page.get_by_role('tab', name='Class', exact=True).click()
    # The new action must be assignable without editing JSON or invoking a class API by hand.
    picker = page.locator('#wpi-action-class')
    expect(picker).to_be_visible()
    picker.select_option('save')
    expect(page.get_by_role('heading', name='Class: save')).to_be_visible()
    assert any('/classes/action?scope=all' in url for url in requests), requests
    assert not errors, errors
    print('Full WPI Editor checks passed: record/schema versions, class-body JSON editing and saving, separate instance JSON, editor-options help, preview drafts and action classes.')
   except Exception:
    page.screenshot(path='/tmp/wpi-class-full-failure.png', full_page=True)
    print('Browser errors:', errors)
    print('Visible text:', page.locator('body').inner_text()[:5000])
    log.seek(0); print(log.read()[-6000:])
    raise
   finally:
    browser.close()
 finally:
  process.terminate(); process.wait(timeout=10)
