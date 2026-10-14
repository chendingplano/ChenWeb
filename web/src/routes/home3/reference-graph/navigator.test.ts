import { expect, test } from 'bun:test';
import { readFileSync } from 'node:fs';
import { transpileModule, ScriptTarget } from 'typescript';
import { init, use } from 'echarts/core';
import { GraphChart, TreeChart } from 'echarts/charts';
import { SVGRenderer } from 'echarts/renderers';
import { buildDocumentHierarchy, flattenHierarchy, buildNavigator } from './graph-data';

use([GraphChart, TreeChart, SVGRenderer]);

// Exercise the page's pan handler against real ECharts views: roam actions can
// update the model without moving the rendered canvas.
const source = readFileSync(new URL('./+page.svelte', import.meta.url), 'utf8');
const handler = source.match(/function dispatchNavigatorPan\(dx: number, dy: number\) \{([\s\S]*?)\n\t\}/)![1];
const compiledHandler = transpileModule(`function pan(dx: number, dy: number) {${handler}}`, {
	compilerOptions: { target: ScriptTarget.ESNext }
}).outputText;

for (const mode of ['graph', 'LR', 'TB', 'BT', 'RL']) {
	test(`minimap panning moves the rendered ${mode} view and preserves zoom`, () => {
		const chart = init(null, undefined, { renderer: 'svg', ssr: true, width: 800, height: 600 });
		try {
			chart.setOption({ animation: false, series: [{
				id: 'reference-graph-series', type: mode === 'graph' ? 'graph' : 'tree',
				layout: 'none', orient: mode, roam: true, zoom: 1.5,
				data: mode === 'graph'
					? [{ id: 'root', x: 0, y: 0 }, { id: 'leaf', x: 100, y: 100 }]
					: [{ name: 'root', children: [{ name: 'leaf' }] }],
				links: [{ source: 'root', target: 'leaf' }]
			}] });
			const pan = new Function('chartApi', 'chartWrap', 'viewMode', `${compiledHandler}; return pan;`)(chart, { clientWidth: 800, clientHeight: 600 }, mode === 'graph' ? 'graph' : 'left-right');
			const model = (chart as any).getModel().getSeriesByIndex(0);
			const group = (chart as any).getViewOfSeriesModel(model).group;
			const before = { x: group.x, y: group.y };
			pan(-80, -60);
			expect(group.x).toBeCloseTo(before.x - 80);
			expect(group.y).toBeCloseTo(before.y - 60);
			expect((chart.getOption().series as any[])[0].zoom).toBe(1.5);
			pan(80, 60);
			expect(group.x).toBeCloseTo(before.x);
			expect(group.y).toBeCloseTo(before.y);
		} finally {
			chart.dispose();
		}
	});
}

test('recreated graph charts refresh the minimap from the actual force layout after each tree mode', () => {
	const body = source.match(/function syncGraphPositions\(\) \{([\s\S]*?)\n\t\}/)![1];
	const compiled = transpileModule(`function sync() {${body}}`, {
		compilerOptions: { target: ScriptTarget.ESNext }
	}).outputText;
	const root = buildDocumentHierarchy({ id: 416, name: 'Document' },
		[{ id: 1, name: 'Metric' }, { id: 2, name: 'Another metric' }],
		[{ id: 1, name: 'Reference' }], { metrics: 'Metrics', references: 'References' });
	const { nodes, links } = flattenHierarchy(root);
	for (const mode of ['left-right', 'top-bottom', 'bottom-top', 'right-left'] as const) {
		const tree = buildNavigator(root, mode);
		const chart = init(null, undefined, { renderer: 'svg', ssr: true, width: 800, height: 600 });
		try {
			const sync = new Function('chartApi', 'viewMode', `let graphPositions = {}; ${compiled}; return () => { sync(); return graphPositions; };`)(chart, 'graph');
			let positions: Record<string, [number, number]> = {};
			chart.on('rendered', () => { positions = sync(); });
			chart.setOption({ animation: false, series: [{
				type: 'graph', layout: 'force', data: nodes, links,
				force: { repulsion: 800, layoutAnimation: false }
			}] });
			chart.getZr().flush();
			expect(Object.keys(positions)).toHaveLength(nodes.length);
			const minimap = buildNavigator(root, 'graph', positions);
			expect(minimap).not.toEqual(tree);
			const data = (chart as any).getModel().getSeriesByIndex(0).getData();
			for (let index = 0; index < data.count(); index++) {
				expect(positions[data.getId(index)]).toEqual(data.getItemLayout(index));
			}
		} finally {
			chart.dispose();
		}
	}
});
