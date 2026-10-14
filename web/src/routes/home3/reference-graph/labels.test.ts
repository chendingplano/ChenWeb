import { expect, test } from 'bun:test';
import { readFileSync } from 'node:fs';
import { transpileModule, ScriptTarget } from 'typescript';
import { init, use } from 'echarts/core';
import { TreeChart } from 'echarts/charts';
import { TooltipComponent } from 'echarts/components';
import { SVGRenderer } from 'echarts/renderers';

use([TreeChart, TooltipComponent, SVGRenderer]);

// Render the page's actual tree options to catch label rotation and anchoring
// regressions, including the mirrored bottom-to-top view.
const source = readFileSync(new URL('./+page.svelte', import.meta.url), 'utf8');
const body = source.match(/const treeOption = \$derived\.by<EChartsOption>\(\(\) => \{([\s\S]*?)\n\t\}\);/)![1];
const compiled = transpileModule(`function buildOption(record, references, viewMode, palette, recordTitle, referenceTitle) {${body}}`, {
	compilerOptions: { target: ScriptTarget.ESNext }
}).outputText;
const buildOption = new Function(`${compiled}; return buildOption;`)();

for (const mode of ['top-bottom', 'bottom-top', 'left-right', 'right-left']) {
	test(`${mode} labels rotate and anchor away from the tree`, () => {
		const chart = init(null, undefined, { renderer: 'svg', ssr: true, width: 1200, height: 900 });
		try {
			chart.setOption(buildOption(
				{ title: 'Document 文档' }, [{ title: 'Reference 引用', ref_id: 'ref-1' }], mode,
				{ text: '#333', root: '#a87518', node: '#ddd', edge: '#aaa' },
				(value: { title: string }) => value.title, (value: { title: string }) => value.title
			));
			const series = (chart as any).getModel().getSeriesByIndex(0);
			const data = series.getData();
			const vertical = mode === 'top-bottom' || mode === 'bottom-top';
			const rootSymbol = data.getItemGraphicEl(1).getSymbolPath();
			const leafSymbol = data.getItemGraphicEl(2).getSymbolPath();
			const rootLabel = rootSymbol.getTextContent();
			const leafLabel = leafSymbol.getTextContent();
			expect(rootSymbol.textConfig.rotation ?? 0).toBeCloseTo(vertical ? -Math.PI / 2 : 0);
			expect(leafSymbol.textConfig.rotation ?? 0).toBeCloseTo(vertical ? -Math.PI / 2 : 0);
			const rootAlign = mode === 'top-bottom' || mode === 'left-right' ? 'right' : 'left';
			expect(rootLabel.style.align).toBe(rootAlign);
			expect(leafLabel.style.align).toBe(rootAlign === 'right' ? 'left' : 'right');
			expect(rootLabel.style.text).toBe('Document 文档');
			expect(leafLabel.style.text).toBe('Reference 引用');
		} finally {
			chart.dispose();
		}
	});
}
