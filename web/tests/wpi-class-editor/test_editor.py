"""Run from any directory; requires Bun and Python Playwright/Chromium."""
import json
import socket
import subprocess
import tempfile
import time
from pathlib import Path
from playwright.sync_api import sync_playwright, expect

web = Path(__file__).resolve().parents[2]
fixtures = {
 'element/input': {'descriptor_id': 'input', 'schema_version': '0.4', 'code_module_id': 'input', 'label': {'help_tip': {'en': 'Caption', 'zh-cn': '标题'}, 'default_value': 'Name'}, 'width': {'default_value': '200px'}, 'custom_count': {'default_value': 2}},
 'element/list': {'descriptor_id': 'list', 'schema_version': '0.4', 'code_module_id': 'list', 'source': {'default_value': {'table': 'kb.inputs', 'fields': ['id']}}},
 'page-layouter/single-pane': {'descriptor_id': 'single-pane', 'schema_version': '0.5', 'code_module_id': 'page-layouter', 'root': {'pane_id': 'main', 'width': '100%'}},
 'element-layouter/flow-layouter': {'descriptor_id': 'flow-layouter', 'schema_version': '0.5', 'code_module_id': 'flow-layouter', 'config': {'column_gap': '10px'}},
 'action/save': {'descriptor_id': 'save', 'schema_version': '0.2', 'table': None, 'mode': 'insert'}
}

with tempfile.TemporaryFile(mode='w+') as log:
 process = subprocess.Popen([str(web / 'node_modules/.bin/vite'), '--config', 'tests/wpi-class-editor/vite.config.ts'], cwd=web, stdout=log, stderr=log)
 try:
  deadline = time.monotonic() + 30
  while True:
   try:
    with socket.create_connection(('127.0.0.1', 5198), timeout=0.5): break
   except OSError:
    if process.poll() is not None or time.monotonic() > deadline:
     log.seek(0); raise RuntimeError(log.read())
    time.sleep(0.2)
  with sync_playwright() as p:
   browser = p.chromium.launch(headless=True)
   page = browser.new_page(viewport={'width': 1100, 'height': 1000})
   errors, writes, reads = [], [], []
   reject = [False]
   page.on('pageerror', lambda err: errors.append(str(err)))
   def api(route):
    key = route.request.url.split('/api/v1/wpi/classes/')[1]
    if route.request.method == 'GET':
     reads.append(key)
     route.fulfill(json={'version': 2, 'status': 'active', 'class': fixtures[key]})
    else:
     writes.append((key, route.request.post_data_json))
     if reject[0]:
      route.fulfill(status=400, json={'errors': [{'kind': 'element', 'descriptor_id': 'input', 'path': '/width', 'message': 'Width refused'}]})
     else:
      fixtures[key] = route.request.post_data_json
      route.fulfill(json={'version': 2, 'status': 'active', 'warnings': []})
   page.route('**/api/v1/wpi/classes/**', api)
   page.goto('http://127.0.0.1:5198/', wait_until='networkidle')
   expect(page.get_by_role('tab', name='Class', exact=True)).to_be_visible()
   expect(page.locator('#instance-properties')).to_be_visible()
   page.get_by_role('tab', name='Class', exact=True).click()
   expect(page.get_by_role('heading', name='Class: input')).to_be_visible()
   save = page.get_by_role('button', name='Save class', exact=True)
   expect(save).to_be_disabled()

   def attribute(name):
    return page.locator('button.attribute').filter(has=page.locator('span.name', has_text=name)).filter(has_text=name)
   def form(path):
    return page.locator(f'fieldset[id="class-form-{path}"]')
   def field(prefix, path):
    return page.locator(f'[id="class-{prefix}-{path}"]')
   label = page.locator('button.attribute').filter(has=page.locator('span.name', has_text='label'))
   label.click()
   expect(form('/label')).to_be_visible()
   expect(field('name', '/label')).to_have_value('label')
   expect(form('/label').get_by_role('group', name='Attribute scope')).to_be_visible()
   expect(field('description', '/label')).to_be_visible()
   expect(field('default', '/label')).to_have_value('Name')
   label.click(); expect(form('/label')).to_have_count(0)
   expect(save).to_be_disabled()
   label.click()
   field('default', '/label').fill('Full name'); field('default', '/label').blur()
   expect(save).to_be_enabled()
   expect(form('/label').get_by_role('alert')).to_have_count(0)

   # Cached drafts survive another selection and a trip through Properties.
   page.locator('#select-list').click()
   expect(page.get_by_role('heading', name='Class: list')).to_be_visible()
   page.locator('#select-input').click()
   page.locator('button.attribute').filter(has=page.locator('span.name', has_text='label')).click()
   expect(field('default', '/label')).to_have_value('Full name')
   page.get_by_role('tab', name='Properties', exact=True).click()
   expect(page.locator('#instance-properties')).to_be_visible()
   page.get_by_role('tab', name='Class', exact=True).click()
   expect(field('default', '/label')).to_have_value('Full name')
   assert reads.count('element/input') == 1, reads

   page.locator('[id="class-description-/label-en"]').fill('Full caption')
   page.locator('[id="class-description-/label-en"]').blur()

   # Number configuration and renaming preserve a separate attribute definition.
   page.locator('button.attribute').filter(has=page.locator('span.name', has_text='custom_count')).click()
   field('editor', '/custom_count').select_option('number')
   page.locator('[id="class-options-/custom_count-min"]').fill('1')
   page.locator('[id="class-options-/custom_count-min"]').blur()
   page.locator('[id="class-options-/custom_count-max"]').fill('10')
   page.locator('[id="class-options-/custom_count-max"]').blur()
   form('/custom_count').get_by_role('checkbox', name='Whole numbers only').check()
   field('name', '/custom_count').fill('counter'); field('name', '/custom_count').blur()
   expect(form('/counter')).to_be_visible()
   expect(field('default', '/counter')).to_have_value('2')

   # Scope and selected editor configure the stored class, keeping JSON types.
   form('/label').get_by_role('button', name='Class', exact=True).click()
   expect(form('/label').get_by_role('button', name='Class', exact=True)).to_have_attribute('aria-pressed', 'true')
   field('editor', '/label').select_option('value-picker')
   choices = page.locator('[id="class-options-/label-choices"]')
   expect(choices).to_be_visible()
   choices.fill('Full name'); choices.press('End'); choices.press('Enter'); choices.press_sequentially('Nickname')
   expect(choices).to_have_value('Full name\nNickname')
   choices.blur()
   save.click(); expect(save).to_be_disabled()
   assert writes[-1][1]['label']['instance_modifiable'] is False, writes
   assert writes[-1][1]['label']['default_value'] == 'Full name', writes
   assert writes[-1][1]['label']['help_tip'] == {'en': 'Full caption', 'zh-cn': '标题'}, writes
   assert 'custom_count' not in writes[-1][1], writes
   assert writes[-1][1]['counter']['default_value'] == 2, writes
   assert writes[-1][1]['attribute_definitions']['/counter']['value_editor'] == {'id': 'number', 'options': {'min': 1, 'max': 10, 'integer': True}}, writes
   assert writes[-1][1]['attribute_definitions']['/label']['value_editor'] == {'id': 'value-picker', 'options': {'choices': ['Full name', 'Nickname']}}, writes
   expect(page.locator('#saves')).to_have_text('1')

   # Server rejection retains the draft and gives the author the field issue.
   field('default', '/label').fill('Still editing'); field('default', '/label').blur()
   reject[0] = True; save.click()
   expect(page.get_by_role('alert')).to_contain_text('/width: Width refused')
   expect(field('default', '/label')).to_have_value('Still editing')
   expect(save).to_be_enabled()
   reject[0] = False; save.click(); expect(save).to_be_disabled()

   # Nested values are exposed as logical attribute paths.
   page.locator('#select-list').click()
   page.locator('button.attribute').filter(has=page.locator('span.name', has_text='source')).click()
   page.locator('button.attribute').filter(has=page.locator('span.name', has_text='table')).click()
   field('default', '/source/table').fill('kb.metrics'); field('default', '/source/table').blur()
   save.click(); expect(save).to_be_disabled()
   assert writes[-1][1]['source']['default_value'] == {'table': 'kb.metrics', 'fields': ['id']}, writes

   for control, key in [('select-pane', 'page-layouter/single-pane'), ('select-panel', 'element-layouter/flow-layouter'), ('select-action', 'action/save')]:
    page.locator('#'+control).click()
    expect(page.get_by_role('heading', name='Class: '+key.split('/')[1], exact=True)).to_be_visible()
    assert key in reads, reads
   page.locator('#select-page').click()
   expect(page.locator('.empty')).to_contain_text('Pages and flow-only nodes have no class')
   page.locator('#language-zh').click()
   expect(page.get_by_role('tab', name='类', exact=True)).to_be_visible()
   expect(page.locator('.empty')).to_contain_text('页面和流程专用节点没有类')
   page.locator('#select-input').click()
   expect(page.get_by_role('heading', name='类：input')).to_be_visible()
   page.locator('button.attribute').filter(has=page.locator('span.name', has_text='width')).click()
   expect(form('/width').get_by_text('属性名称', exact=True)).to_be_visible()
   expect(form('/width').get_by_text('编辑器配置', exact=True)).to_be_visible()
   assert not errors, errors
   browser.close()
   print('WPI Class browser checks passed: disclosures, nested values, class scope, configuration, saving, errors, cached drafts, class kinds and both languages.')
 finally:
  process.terminate(); process.wait(timeout=10)
