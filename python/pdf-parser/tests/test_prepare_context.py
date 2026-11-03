import subprocess
import sys
from pathlib import Path

SCRIPT = Path(__file__).resolve().parent.parent / 'container' / 'prepare-context.py'
ADAPTERS = ('parser_mineru.py', 'mineru_v4_adapter.py', 'mineru_table_repair.py',
            'mineru_cell_repair.py', 'stacked_cells.py', 'table_geometry.py')


def test_staged_worker_has_every_module_the_mineru_parser_imports(tmp_path):
    worker, adapters, mineru = tmp_path / 'worker', tmp_path / 'adapters', tmp_path / 'mineru'
    worker.mkdir()
    (worker / 'pdf_parser.py').write_text('make_ocr_parser(_ocr_fallback_lang())\n')
    adapters.mkdir()
    for name in ADAPTERS:
        (adapters / name).write_text(f'# {name}\n')
    (mineru / 'mineru').mkdir(parents=True)
    for name in ('pyproject.toml', 'README.md', 'LICENSE.md'):
        (mineru / name).write_text('')
    out = tmp_path / 'out'
    subprocess.run([sys.executable, str(SCRIPT), '--worker', str(worker), '--adapters', str(adapters),
                    '--mineru', str(mineru), '--output', str(out)], check=True)
    for name in ADAPTERS:
        assert (out / 'worker' / name).read_text() == f'# {name}\n'
