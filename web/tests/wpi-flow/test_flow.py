"""Browser geometry regression: run with python3 web/tests/wpi-flow/test_flow.py.

Requires Bun, Python Playwright, and its Chromium browser.
"""
from http.server import SimpleHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path
from tempfile import TemporaryDirectory
from threading import Thread
import subprocess

from playwright.sync_api import sync_playwright

web = Path(__file__).resolve().parents[2]
html = '''<style>
#flow {display:flex;flex-wrap:wrap;position:relative;width:310px;gap:10px;align-items:flex-start;align-content:flex-start}
.item {flex:0 0 90px;height:50px;max-height:50px;overflow:auto;line-height:20px}
.body-row p {margin:0;overflow-wrap:anywhere}
</style><div id="flow">''' + ''.join('<div class="item" data-flow-item></div>' for _ in range(5)) + '''</div>
<script type="module">import {flowGeometry} from './flow.js';
window.geometry=flowGeometry(document.querySelector('#flow'), {config:{},count:5});</script>'''

with TemporaryDirectory() as tmp:
    folder = Path(tmp)
    (folder / 'index.html').write_text(html)
    subprocess.run(['bun', 'build', str(web / 'src/lib/wpi/layouters/flow-geometry.ts'),
                    '--outfile', str(folder / 'flow.js'), '--target', 'browser'], check=True)
    class Handler(SimpleHTTPRequestHandler):
        def __init__(self, *args, **kwargs):
            super().__init__(*args, directory=tmp, **kwargs)
        def log_message(self, *args):
            pass
    server = ThreadingHTTPServer(('127.0.0.1', 0), Handler)
    Thread(target=server.serve_forever, daemon=True).start()
    try:
        with sync_playwright() as playwright:
            browser = playwright.chromium.launch(headless=True)
            page = browser.new_page()
            page.goto(f'http://127.0.0.1:{server.server_port}/', wait_until='networkidle')
            for width, alignment, limit, expected in [
                (310, 'start', 2, [2, 2, 1]), (210, 'center', 3, [2, 2, 1]),
                (310, 'space-between', 2, [2, 2, 1]), (310, 'start', 0, [3, 2]),
                (80, 'start', 1, [1, 1, 1, 1, 1]),
            ]:
                page.evaluate('''([w,a,n]) => { let x = document.querySelector('#flow');
                  x.style.width = w + 'px'; x.style.justifyContent = a;
                  geometry.update({config:{max_num_elems:n,row_gap:'10px',
                    row_separator:'thin_solid_line',elem_separator:'bold_dash_line'},count:5}); }''',
                              [width, alignment, limit])
                page.wait_for_timeout(100)
                coords = page.locator('[data-flow-item]').evaluate_all('(xs) => xs.map(x => x.offsetTop)')
                rows = []
                for y in coords:
                    if not rows or rows[-1][0] != y:
                        rows.append([y, 0])
                    rows[-1][1] += 1
                assert [row[1] for row in rows] == expected, (width, alignment, rows)
                assert all(rows[i + 1][0] - rows[i][0] == 60 for i in range(len(rows) - 1)), rows
                lines = page.locator('[data-flow-decoration]').evaluate_all('''(xs) => xs
                  .filter(x => x.style.borderTop || x.style.borderLeft)
                  .map(x => [x.style.borderTop, x.style.borderLeft, x.offsetWidth, x.offsetHeight])''')
                assert sum(bool(line[0]) for line in lines) == len(rows) - 1, lines
                assert sum(bool(line[1]) for line in lines) == 5 - len(rows), lines
                assert all(line[3] == 50 for line in lines if line[1]), lines
                assert all('1px solid' in line[0] for line in lines if line[0]), lines
                assert all('3px dashed' in line[1] for line in lines if line[1]), lines
            # Separators cover each content-sized row, even when shorter items
            # are centered or bottom-aligned and adjacent rows have different heights.
            for alignment in ['flex-start', 'center', 'flex-end']:
                page.evaluate('''(alignment) => {
                  const flow = document.querySelector('#flow');
                  flow.style.width = '210px'; flow.style.height = '';
                  flow.style.alignItems = alignment; flow.style.justifyContent = 'start';
                  [...flow.querySelectorAll('[data-flow-item]')].forEach((item, i) => {
                    item.style.height = [80, 30, 40, 65, 20][i] + 'px';
                    item.style.maxHeight = '100px';
                  });
                  geometry.update({config:{max_num_elems:2,row_gap:'10px',
                    elem_separator:'thin_solid_line'},count:5});
                }''', alignment)
                page.wait_for_timeout(100)
                lines = page.locator('[data-flow-decoration]').evaluate_all('''xs => xs
                  .filter(x => x.style.borderLeft)
                  .map(x => [x.offsetTop, x.offsetHeight])''')
                items = page.locator('[data-flow-item]').evaluate_all('''xs => xs.map(x => [x.offsetTop, x.offsetHeight])''')
                for n, line in enumerate(lines):
                    row = items[n*2:n*2+2]
                    top = min(x[0] for x in row)
                    bottom = max(x[0] + x[1] for x in row)
                    assert line == [top, bottom - top], (alignment, line, row)
                assert len(lines) == 2, (alignment, lines)
            page.evaluate('''() => { document.querySelector('#flow').style.alignItems='flex-start';
              document.querySelectorAll('[data-flow-item]').forEach(item => { item.style.height='50px'; item.style.maxHeight='50px'; }); }''')
            page.evaluate('''() => { let x = document.querySelector('#flow'); x.style.width='310px';
              x.style.height='300px'; geometry.update({config:{max_num_elems:2,row_gap:'10px',
              rows_alignment:'space-between'},count:5}); }''')
            page.wait_for_timeout(100)
            assert page.locator('[data-flow-item]').evaluate_all('(xs) => xs.map(x => x.offsetTop)') == [0, 0, 125, 125, 250]
            page.evaluate('''() => { const x=document.querySelector('[data-flow-item]');
              x.classList.add('flow-text'); x.innerHTML='<div class="body-row"><p>This is a long piece of text that should overflow several lines in a narrow cell.</p></div>';
              geometry.update({config:{max_num_elems:2,row_gap:'10px'},count:5}); }''')
            page.wait_for_timeout(100)
            assert page.locator('.body-row p').evaluate('(e) => e.title.length > 0 && Number(e.style.webkitLineClamp) > 0')
            browser.close()
    finally:
        server.shutdown()
print('flow browser geometry passed')
