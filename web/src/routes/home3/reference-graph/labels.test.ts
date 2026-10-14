import { expect, test } from 'bun:test';
import { readFileSync } from 'node:fs';
import { transpileModule, ScriptTarget } from 'typescript';
import { init, use } from 'echarts/core';
import { TreeChart } from 'echarts/charts';
import { TooltipComponent } from 'echarts/components';
import { SVGRenderer } from 'echarts/renderers';
import { buildDocumentHierarchy } from './graph-data';

use([TreeChart, TooltipComponent, SVGRenderer]);

// Render the page's actual tree options to catch label rotation and anchoring
// regressions, including the mirrored bottom-to-top view.
const source = readFileSync(new URL('./+page.svelte', import.meta.url), 'utf8');
const body = source.match(/const treeOption = \$derived\.by<EChartsOption>\(\(\) => \{([\s\S]*?)\n\t\}\);/)![1];
const compiled = transpileModule(`function buildOption(hierarchy, viewMode, palette) {${body}}`, {
	compilerOptions: { target: ScriptTarget.ESNext }
}).outputText;
const buildOption = new Function(`${compiled}; return buildOption;`)();

for (const mode of ['top-bottom', 'bottom-top', 'left-right', 'right-left']) {
	test(`${mode} labels retain their orientation and mouse wheel zooms over blank canvas`, () => {
		const chart = init(null, undefined, { renderer: 'svg', ssr: true, width: 1200, height: 900 });
		try {
			chart.setOption(buildOption(
				buildDocumentHierarchy({ id: 416, name: 'Document 文档' }, [{ id: 1, name: 'pH', value: '5.5–8.5' }],
					[{ id: 1, name: 'Reference 引用', value: 'ref-1' }], { metrics: 'Metrics', references: 'References' }),
				mode, { text: '#333', root: '#a87518', node: '#ddd', edge: '#aaa' }
			));
			const series = (chart as any).getModel().getSeriesByIndex(0);
			const configureBody = source.match(/function configureTreeRoam\(\) \{([\s\S]*?)\n\t\}/)?.[1];
			if (configureBody) {
				const compiled = transpileModule(`function configure() {${configureBody}}`, {
					compilerOptions: { target: ScriptTarget.ESNext }
				}).outputText;
				new Function('chartApi', `${compiled}; configure();`)(chart);
			}
			const wheel = (x: number, y: number, wheelDelta: number) => chart.getZr().trigger('mousewheel', {
				offsetX: x, offsetY: y, wheelDelta,
				event: { preventDefault() {}, stopPropagation() {} }
			});
			// Blank canvas space must zoom too, independent of tree orientation.
			wheel(60, 60, 1);
			expect(series.get('zoom')).toBeCloseTo(1.1);
			wheel(60, 60, -1);
			expect(series.get('zoom')).toBeCloseTo(1);
			wheel(-10, 60, 1);
			expect(series.get('zoom')).toBeCloseTo(1);
			const data = series.getData();
			const vertical = mode === 'top-bottom' || mode === 'bottom-top';
			const rootSymbol = data.getItemGraphicEl(1).getSymbolPath();
			const groupSymbol = data.getItemGraphicEl(2).getSymbolPath();
			const leafSymbol = data.getItemGraphicEl(5).getSymbolPath();
			const rootLabel = rootSymbol.getTextContent();
			const leafLabel = leafSymbol.getTextContent();
			expect(rootSymbol.textConfig.rotation ?? 0).toBeCloseTo(vertical ? -Math.PI / 2 : 0);
			expect(groupSymbol.textConfig.rotation ?? 0).toBeCloseTo(vertical ? -Math.PI / 2 : 0);
			expect(leafSymbol.textConfig.rotation ?? 0).toBeCloseTo(vertical ? -Math.PI / 2 : 0);
			const rootAlign = mode === 'top-bottom' || mode === 'left-right' ? 'right' : 'left';
			expect(rootLabel.style.align).toBe(rootAlign);
			expect(groupSymbol.getTextContent().style.align).toBe(rootAlign);
			expect(leafLabel.style.align).toBe(rootAlign === 'right' ? 'left' : 'right');
			expect(rootLabel.style.text).toBe('Document 文档');
			expect(leafLabel.style.text).toBe('Reference 引用');
		} finally {
			chart.dispose();
		}
	});
}
