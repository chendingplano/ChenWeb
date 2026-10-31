"""Run from the repository root; requires Bun and Python Playwright/Chromium."""
import json
import socket
import subprocess
import tempfile
import time
from pathlib import Path
from playwright.sync_api import sync_playwright, expect

web = Path(__file__).resolve().parents[2]
with tempfile.TemporaryFile(mode='w+') as log:
 process = subprocess.Popen([str(web / 'node_modules/.bin/vite'), '--config', 'tests/wpi-flow-editor/vite.config.ts'], cwd=web, stdout=log, stderr=log)
 try:
  deadline = time.monotonic() + 30
  while True:
   try:
    with socket.create_connection(('127.0.0.1', 5197), timeout=0.5): break
   except OSError:
    if process.poll() is not None or time.monotonic() > deadline:
     log.seek(0); raise RuntimeError(log.read())
    time.sleep(0.2)
  with sync_playwright() as p:
   browser = p.chromium.launch(headless=True)
   page = browser.new_page(viewport={'width': 1000, 'height': 800})
   errors=[]
   page.on('pageerror', lambda err: errors.append(str(err)))
   page.goto('http://127.0.0.1:5197/', wait_until='networkidle')
   count = page.locator('#wpi-flow-max_num_elems')
   expect(count).to_have_value('50')
   expect(page.locator('#wpi-flow-column_gap')).to_have_value('2rem')
   items = page.locator('[data-wpi-page-preview] [data-flow-item]')
   expect(items).to_have_count(4)
   def column_gap():
    return items.evaluate_all('(xs) => xs[1].getBoundingClientRect().left - xs[0].getBoundingClientRect().right')
   assert abs(column_gap() - 32) < 1, column_gap()
   page.locator('#wpi-flow-column_gap').fill('60px'); page.locator('#wpi-flow-column_gap').blur()
   assert abs(column_gap() - 60) < 1, column_gap()
   page.locator('#wpi-flow-column_gap').fill('2rem'); page.locator('#wpi-flow-column_gap').blur()
   column = page.locator('#wpi-flow-column_gap')
   column.fill('100'); column.blur()
   expect(column).to_have_value('2rem')
   expect(page.locator('#wpi-flow-column_gap-error')).to_contain_text('px')
   expect(page.locator('#wpi-flow-column_gap-error')).to_contain_text('not applied')
   page.locator('#wpi-flow-row_gap').fill('3px'); page.locator('#wpi-flow-row_gap').blur()
   expect(page.locator('#wpi-flow-column_gap-error')).to_be_visible()
   column.fill('2rem'); column.blur()
   page.locator('#wpi-flow-row_gap').fill(''); page.locator('#wpi-flow-row_gap').blur()
   expect(page.locator('#wpi-flow-max_row_height')).to_have_value('200px')
   minimum = page.locator('#wpi-flow-min_elem_width')
   minimum.fill('500px'); minimum.blur()
   items = page.locator('[data-wpi-page-preview] [data-flow-item]')
   expect(items).to_have_count(4)
   assert items.evaluate_all('(xs) => xs.map(x => x.offsetWidth)') == [500] * 4
   minimum.fill('100px'); minimum.blur()
   page.locator('#wpi-flow-max_num_elems').fill('2'); page.locator('#wpi-flow-max_num_elems').blur()
   page.locator('#wpi-flow-elem_separator').select_option('thin_solid_line')
   page.wait_for_timeout(200)
   positions = items.evaluate_all('(xs) => xs.map(x => x.offsetTop)')
   assert positions[0] == positions[1] and positions[2] == positions[3] and positions[2] > positions[1], positions
   assert page.locator('[data-wpi-page-preview] [data-flow-decoration]').count() >= 2
   count.fill('0'); count.blur()
   config = json.loads(page.locator('#page').inner_text())['panels']['main']['elem_layouter']['config']
   assert config == {'column_gap':'2rem', 'custom':'retained', 'min_elem_width':'100px', 'max_num_elems':0, 'elem_separator':'thin_solid_line'}, config
   page.locator('#wpi-flow-row_separator').select_option('bold_dash_line')
   config = json.loads(page.locator('#page').inner_text())['panels']['main']['elem_layouter']['config']
   assert config['row_separator'] == 'bold_dash_line'
   gap = page.locator('#wpi-flow-row_gap'); gap.fill('-1px'); gap.blur()
   expect(gap).to_have_value('0.75rem')
   expect(page.locator('#wpi-flow-row_gap-error')).to_be_visible()
   assert 'row_gap' not in json.loads(page.locator('#page').inner_text())['panels']['main']['elem_layouter']['config']
   gap.fill('1rem'); gap.blur()
   assert json.loads(page.locator('#page').inner_text())['panels']['main']['elem_layouter']['config']['row_gap'] == '1rem'
   page.locator('#locale').click()
   expect(page.get_by_label('每行最多元素数')).to_have_value('0')
   expect(page.get_by_label('行分隔线').locator('option:checked')).to_have_text('粗虚线')
   assert page.locator('aside').evaluate("e=>getComputedStyle(e).userSelect") == 'text'
   assert not errors, errors
   browser.close()
 finally:
  process.terminate(); process.wait(timeout=10)
print('flow property editor browser checks passed')
