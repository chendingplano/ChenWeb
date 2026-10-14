import { expect, test } from 'bun:test';
import { readFileSync } from 'node:fs';
import { transpileModule, ScriptTarget } from 'typescript';
import { init, use } from 'echarts/core';
import { GraphChart, TreeChart } from 'echarts/charts';
import { SVGRenderer } from 'echarts/renderers';
import { buildDocumentHierarchy, flattenHierarchy } from './graph-data';

use([GraphChart, TreeChart, SVGRenderer]);
const source = readFileSync(new URL('./+page.svelte', import.meta.url), 'utf8');
const body = source.match(/function handleChartClick\(event: ECElementEvent\) \{([\s\S]*?)\n\t\}/)![1];
const compiled = transpileModule(`function handleChartClick(event) {${body}}`, {
	compilerOptions: { target: ScriptTarget.ESNext }
}).outputText;

for (const mode of ['graph', 'LR', 'RL', 'TB', 'BT']) {
	test(`${mode} clicks select references and ignore other nodes`, () => {
		const chart = init(null, undefined, { renderer: 'svg', ssr: true, width: 1200, height: 900 });
		const references = [{ id: 1, name: 'Linked target', target_input_id: 500 }, { id: 2, name: 'Unlinked target' }];
		const hierarchy = buildDocumentHierarchy({ id: 416, name: 'Source' }, [{ id: 3, name: 'Metric' }],
			references, { metrics: 'Metrics', references: 'References' });
		const selection = new Function('record', 'references', `
			let selectedReferenceId = null;
			${compiled}
			return { click: handleChartClick, selected: () => selectedReferenceId };
		`)({ id: 416 }, references);
		try {
			chart.setOption({ series: [mode === 'graph'
				? { type: 'graph', layout: 'circular', data: flattenHierarchy(hierarchy).nodes, links: flattenHierarchy(hierarchy).links }
				: { type: 'tree', orient: mode, data: [hierarchy], initialTreeDepth: -1, expandAndCollapse: false }
			] });
			chart.on('click', selection.click);
			const data = (chart as any).getModel().getSeriesByIndex(0).getData();
			const click = (id: string) => {
				const index = Array.from({ length: data.count() }, (_, i) => i).find(i => data.getId(i) === id)!;
				const target = data.getItemGraphicEl(index).getSymbolPath();
				chart.getZr().trigger('click', { target, event: {} });
			};
			click('input:416:references:1');
			expect(selection.selected()).toBe(1);
			click('input:416:metrics:3');
			click('input:416:references');
			click('input:416');
			expect(selection.selected()).toBe(1);
			click('input:416:references:2');
			expect(selection.selected()).toBe(2);
			selection.click({ dataType: 'edge', data: { id: 'input:416:references:1' } });
			selection.click({ dataType: 'node', data: { id: 'input:417:references:1' } });
			expect(selection.selected()).toBe(2);
		} finally {
			chart.dispose();
		}
	});
}
