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
   items.first.locator('button.overlay').click()
   toolbar = page.get_by_role('group', name='Element actions')
   expect(toolbar).to_be_visible()
   toolbar.get_by_label('Edit class').click()
   expect(page.locator('#action')).to_have_text('edit:0')
   toolbar.get_by_label('Duplicate').click()
   expect(page.locator('#action')).to_have_text('duplicate:0')
   toolbar.get_by_label('Delete').click()
   expect(page.locator('#action')).to_have_text('remove:0')
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
   panel_rect = page.locator('[data-wpi-page-preview] [data-flow-layout]').bounding_box()
   pane_rect = page.locator('[data-editor-pane="main"]').bounding_box()
   row_rect = items.evaluate_all("xs => { const r = xs.slice(0, 2).map(x => x.getBoundingClientRect()); return {left: r[0].left, right: r[1].right}; }")
   assert abs((row_rect['left'] + row_rect['right']) / 2 - (pane_rect['x'] + pane_rect['width'] / 2)) < 2, (row_rect, pane_rect, panel_rect)
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
   background_mode = page.locator('#wpi-color-background_color-mode')
   expect(background_mode).to_have_value('none')
   background_mode.select_option('theme')
   page.locator('#wpi-color-background_color-light-picker').fill('#ffffff')
   page.locator('#wpi-color-background_color-dark-picker').fill('#000000')
   panel = page.locator('[data-wpi-page-preview] [data-flow-layout]')
   assert panel.evaluate('(e) => getComputedStyle(e).backgroundColor') == 'rgb(255, 255, 255)'
   page.locator('#theme').click()
   assert panel.evaluate('(e) => getComputedStyle(e).backgroundColor') == 'rgb(0, 0, 0)'
   page.locator('#mode').click()
   page.locator('#wpi-color-background_color-mode').select_option('single')
   page.locator('#wpi-color-background_color-single-picker').fill('#ff0000')
   page.locator('#wpi-color-foreground_color-mode').select_option('single')
   page.locator('#wpi-color-foreground_color-single-picker').fill('#ffffff')
   descriptor = json.loads(page.locator('#page').inner_text())
   assert descriptor['page_layouter']['root']['background_color'] == '#ff0000'
   assert descriptor['panels']['main']['background_color'] == {'light':'#ffffff', 'dark':'#000000'}
   assert panel.evaluate('(e) => getComputedStyle(e).backgroundColor') == 'rgba(0, 0, 0, 0)'
   assert panel.evaluate('(e) => getComputedStyle(e).color') == 'rgb(255, 255, 255)'
   page.locator('#mode').click()
   page.locator('#wpi-color-foreground_color-mode').select_option('single')
   page.locator('#wpi-color-foreground_color-single-value').fill('rgba(0,0,255,0.5)')
   page.locator('#wpi-color-foreground_color-single-value').blur()
   assert json.loads(page.locator('#page').inner_text())['panels']['main']['foreground_color'] == 'rgba(0,0,255,0.5)'
   expect(page.locator('#wpi-color-foreground_color-single-picker')).to_have_value('#0000ff')
   page.locator('#wpi-color-foreground_color-mode').select_option('none')
   assert 'foreground_color' not in json.loads(page.locator('#page').inner_text())['panels']['main']
   page.locator('#wpi-color-background_color-mode').select_option('single')
   assert json.loads(page.locator('#page').inner_text())['panels']['main']['background_color'] == '#ffffff'
   page.locator('#wpi-color-background_color-mode').select_option('none')
   assert 'background_color' not in json.loads(page.locator('#page').inner_text())['panels']['main']
   page.locator('#nested').click()
   pane = page.locator('[data-pane-id="main"]')
   assert pane.evaluate('(e) => getComputedStyle(e).backgroundColor') == 'rgb(0, 0, 255)'
   assert pane.evaluate('(e) => getComputedStyle(e).color') == 'rgb(255, 255, 0)'
   assert pane.locator('[data-flow-layout]').evaluate('(e) => getComputedStyle(e).backgroundColor') == 'rgba(0, 0, 0, 0)'
   sibling = page.locator('[data-pane-id="second"]')
   assert sibling.evaluate('(e) => getComputedStyle(e).backgroundColor') == 'rgb(0, 128, 0)'
   assert sibling.evaluate('(e) => getComputedStyle(e).color') == 'rgb(0, 0, 0)'
   bottom = page.locator('[data-pane-id="bottom"]')
   assert bottom.evaluate('(e) => getComputedStyle(e).backgroundColor') == 'rgb(255, 0, 0)'
   assert bottom.evaluate('(e) => getComputedStyle(e).color') == 'rgb(255, 255, 255)'
   runtime_pane = page.locator('#runtime [data-pane="main"]')
   expect(runtime_pane).to_be_visible()
   assert runtime_pane.evaluate('(e) => getComputedStyle(e).backgroundColor') == 'rgb(0, 0, 255)'
   assert runtime_pane.evaluate('(e) => getComputedStyle(e).color') == 'rgb(255, 255, 0)'
   assert runtime_pane.locator('[data-flow-layout]').evaluate('(e) => getComputedStyle(e.parentElement).backgroundColor') == 'rgba(0, 0, 0, 0)'
   runtime_items = runtime_pane.locator('[data-flow-item]')
   expect(runtime_items).to_have_count(4)
   page.wait_for_timeout(200)
   runtime_bounds = runtime_items.evaluate_all('xs => xs.slice(0, 2).map(x => x.getBoundingClientRect().toJSON())')
   runtime_box = runtime_pane.bounding_box()
   assert abs((runtime_bounds[0]['left'] + runtime_bounds[1]['right']) / 2 - (runtime_box['x'] + runtime_box['width'] / 2)) < 2, (runtime_bounds, runtime_box)
   assert not errors, errors
   browser.close()
 finally:
  process.terminate(); process.wait(timeout=10)
print('flow property editor browser checks passed')
