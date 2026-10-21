import test from 'node:test';
import assert from 'node:assert/strict';

import {
	buildCanvasOption,
	buildTreeData,
	edgePath,
	EDGE_STYLES,
	defaultExpanded,
	visibleTree,
	canvasExtent,
	initialWindow,
	labelLang,
	labelStyle,
	layoutTree,
	LABEL_GAP,
	labelUnits,
	MAX_LABEL_UNITS,
	NODE_SYMBOL_SIZE,
	nodeLabelText,
	truncateLabel,
	TREE_LAYOUTS,
	TREE_SPACING
} from './category-tree-model.js';

const node = (id: string, parent: string | undefined, isLeaf: boolean, en: string, zh = '') => ({
	id,
	tree_id: 't',
	parent_id: parent,
	canonical_name: id,
	is_others: false,
	pref_labels: { en, ...(zh ? { zh } : {}) },
	definitions: {},
	depth: 0,
	iri: '',
	is_leaf: isLeaf
});

// root -> a -> (a1, a2); root -> b (leaf)
const sample = () =>
	buildTreeData(
		[
			node('root', undefined, false, 'Root', '根'),
			node('a', 'root', false, 'A'),
			node('a1', 'a', true, 'A1'),
			node('a2', 'a', true, 'A2'),
			node('b', 'root', true, 'B')
		],
		'en'
	);

const colors = { text: '#000', line: '#111', leaf: '#222', node: '#333', background: '#FFF' };

test('labelLang maps app locales to stored label keys', () => {
	assert.equal(labelLang('zh-cn'), 'zh');
	assert.equal(labelLang('en'), 'en');
	assert.equal(labelLang('fr'), 'en');
});

test('buildTreeData nests children under their parent in list order', () => {
	const data = sample();
	assert.ok(data);
	assert.equal(data.id, 'root');
	assert.deepEqual(
		data.children?.map((c) => c.id),
		['a', 'b']
	);
	assert.deepEqual(
		data.children?.[0].children?.map((c) => c.id),
		['a1', 'a2']
	);
	assert.equal(data.children?.[1].leaf, true);
});

test('buildTreeData uses the requested language and falls back to English', () => {
	const nodes = [
		node('root', undefined, false, 'Root', '根'),
		node('x', 'root', true, 'Only English')
	];
	assert.equal(buildTreeData(nodes, 'zh')?.name, '根');
	assert.equal(buildTreeData(nodes, 'zh')?.children?.[0].name, 'Only English');
	assert.equal(buildTreeData([], 'en'), null);
});

test('labelStyle places leaf labels on the far side of each layout', () => {
	// Same side for leaves and folded nodes, in every layout.
	for (const leaf of [true, false]) {
		assert.equal(labelStyle('LR', leaf).position, 'right');
		assert.equal(labelStyle('RL', leaf).position, 'left');
		assert.equal(labelStyle('TB', leaf).position, 'bottom');
		assert.equal(labelStyle('BT', leaf).position, 'top');
	}
	// The rotated text runs away from the circle: TB starts at the anchor, BT ends there.
	assert.equal(labelStyle('TB', true).align, 'left');
	assert.equal(labelStyle('BT', true).align, 'right');
	// The hover hit-box in the chart starts LABEL_GAP from the node centre.
	for (const l of TREE_LAYOUTS)
		assert.equal(labelStyle(l, true).distance + NODE_SYMBOL_SIZE / 2, LABEL_GAP);
	assert.equal(labelStyle('LR', true).rotate, undefined);
	assert.equal(labelStyle('TB', false).rotate, -90);
	// Both vertical layouts read top-down (-90).
	assert.equal(labelStyle('BT', true).rotate, -90);
});

test('layoutTree places leaves on consecutive rows at a fixed gap', () => {
	const data = sample();
	assert.ok(data);
	const { nodes } = layoutTree(data, 'LR');
	const byId = new Map(nodes.map((n) => [n.id, n]));
	const { rowGap, levelGap } = TREE_SPACING.horizontal;
	// Leaves in depth-first order: a1, a2, b -> rows 0, 1, 2.
	assert.equal(byId.get('a1')?.y, 0);
	assert.equal(byId.get('a2')?.y, rowGap);
	assert.equal(byId.get('b')?.y, 2 * rowGap);
	// Inner node "a" sits midway between its children; root midway between a and b.
	assert.equal(byId.get('a')?.y, rowGap / 2);
	// Root is midway between a (row 0.5) and b (row 2): row 1.25.
	assert.equal(byId.get('root')?.y, 1.25 * rowGap);
	// Depth sets the x spacing: root 0, a and b at one level, a1 and a2 at two.
	assert.equal(byId.get('root')?.x, 0);
	assert.equal(byId.get('a')?.x, levelGap);
	assert.equal(byId.get('a1')?.x, 2 * levelGap);
});

test('layoutTree mirrors and rotates the tree for each orientation', () => {
	const data = sample();
	assert.ok(data);
	const lr = new Map(layoutTree(data, 'LR').nodes.map((n) => [n.id, n]));
	const rl = new Map(layoutTree(data, 'RL').nodes.map((n) => [n.id, n]));
	assert.equal(rl.get('a1')?.x, -(lr.get('a1')?.x ?? 0));
	assert.equal(rl.get('a1')?.y, lr.get('a1')?.y);

	const v = TREE_SPACING.vertical;
	const tb = new Map(layoutTree(data, 'TB').nodes.map((n) => [n.id, n]));
	// Vertical: rows run across (x), depth runs down (y). a1 is row 0 at depth 2.
	assert.equal(tb.get('a1')?.x, 0);
	assert.equal(tb.get('a1')?.y, 2 * v.levelGap);
	assert.equal(tb.get('b')?.x, 2 * v.rowGap);
	const bt = new Map(layoutTree(data, 'BT').nodes.map((n) => [n.id, n]));
	assert.equal(bt.get('a1')?.x, tb.get('a1')?.x);
	assert.equal(bt.get('a1')?.y, -(tb.get('a1')?.y ?? 0));
});

test('layoutTree emits one elbow connector per parent-child pair', () => {
	const data = sample();
	assert.ok(data);
	const { edges } = layoutTree(data, 'LR');
	assert.equal(edges.length, 4);
	for (const e of edges) assert.equal(e.points.length, 4);
});

test('canvas extent pads the tree and the initial window shows the root', () => {
	const data = sample();
	assert.ok(data);
	const { bounds } = layoutTree(data, 'LR');
	const extent = canvasExtent(bounds);
	assert.ok(extent.xMin < bounds.minX && extent.xMax > bounds.maxX);
	assert.ok(extent.yMin < bounds.minY && extent.yMax > bounds.maxY);
	const w = initialWindow(extent, bounds, 800, 600);
	assert.ok(w.x[0] >= 0 && w.x[1] <= 100 && w.x[0] < w.x[1]);
	assert.ok(w.y[0] >= 0 && w.y[1] <= 100 && w.y[0] < w.y[1]);
});

test('buildCanvasOption uses a fixed canvas with pan and zoom and no fitting', () => {
	const data = sample();
	assert.ok(data);
	const result = layoutTree(data, 'TB');
	const window = initialWindow(canvasExtent(result.bounds), result.bounds, 800, 600);
	const option = buildCanvasOption(result, 'TB', window, colors) as {
		xAxis: { min: number; max: number };
		dataZoom: { id: string; filterMode: string }[];
		series: { type: string; data: unknown[] }[];
	};
	assert.equal(option.dataZoom.map((z) => z.id).join(','), 'zx,zy');
	assert.ok(option.dataZoom.every((z) => z.filterMode === 'none'));
	assert.equal(option.series[0].type, 'lines');
	assert.equal(option.series[1].type, 'scatter');
	assert.equal(option.series[1].data.length, result.nodes.length);
	assert.equal(option.series[0].data.length, result.edges.length);
	assert.ok(option.xAxis.min < option.xAxis.max);
});

test('buildCanvasOption sets no labelLayout offset in any layout', () => {
	const data = sample();
	assert.ok(data);
	for (const layout of TREE_LAYOUTS) {
		const result = layoutTree(data, layout);
		const window = initialWindow(canvasExtent(result.bounds), result.bounds, 800, 600);
		const option = buildCanvasOption(result, layout, window, colors) as {
			series: { labelLayout?: unknown; data: object[] }[];
		};
		assert.equal(option.series[1].labelLayout, undefined);
		assert.ok(option.series[1].data.every((d) => !('labelLayout' in d)));
	}
});

test('EDGE_STYLES lists broken lines and curves', () => {
	assert.deepEqual([...EDGE_STYLES], ['broken', 'curve']);
});

test('edgePath keeps the elbow for broken lines and bends it into a curve', () => {
	const elbow: [number, number][] = [
		[0, 0],
		[0, 50],
		[100, 50],
		[100, 100]
	];
	assert.deepEqual(edgePath(elbow, 'broken'), elbow);
	const curve = edgePath(elbow, 'curve');
	// Starts at the parent, ends at the child, passes the midpoint, and has no right angles.
	assert.deepEqual(curve[0], [0, 0]);
	assert.deepEqual(curve[curve.length - 1], [100, 100]);
	assert.ok(curve.length > elbow.length);
	const mid = curve[Math.floor(curve.length / 2)];
	assert.ok(Math.abs(mid[0] - 50) < 1e-9 && Math.abs(mid[1] - 50) < 1e-9);
	assert.ok(curve.every(([x, y]) => x >= 0 && x <= 100 && y >= 0 && y <= 100));
});

test('buildCanvasOption draws the chosen edge style', () => {
	const data = sample();
	assert.ok(data);
	const result = layoutTree(data, 'TB');
	const window = initialWindow(canvasExtent(result.bounds), result.bounds, 800, 600);
	const coords = (style: 'broken' | 'curve') =>
		(
			buildCanvasOption(result, 'TB', window, colors, style) as {
				series: { data: { coords: [number, number][] }[] }[];
			}
		).series[0].data.map((d) => d.coords);
	assert.deepEqual(
		coords('broken'),
		result.edges.map((e) => e.points)
	);
	assert.deepEqual(
		coords('curve'),
		result.edges.map((e) => edgePath(e.points, 'curve'))
	);
});

test('TREE_LAYOUTS lists the four orientations', () => {
	assert.deepEqual([...TREE_LAYOUTS], ['LR', 'RL', 'TB', 'BT']);
});

test('default expansion shows two levels: the root and its direct children', () => {
	const data = sample();
	assert.ok(data);
	const shown = visibleTree(data, defaultExpanded(data));
	assert.deepEqual(
		shown.children?.map((c) => c.id),
		['a', 'b']
	);
	// "a" has children that are hidden, so it is folded and says how many.
	assert.equal(shown.children?.[0].collapsed, true);
	assert.equal(shown.children?.[0].child_count, 2);
	assert.equal(shown.children?.[0].children, undefined);
	// Leaves are never folded.
	assert.equal(shown.children?.[1].collapsed, undefined);
});

test('expanding a node reveals its children without touching the rest', () => {
	const data = sample();
	assert.ok(data);
	const expanded = new Set([data.id, 'a']);
	const shown = visibleTree(data, expanded);
	assert.deepEqual(
		shown.children?.[0].children?.map((c) => c.id),
		['a1', 'a2']
	);
	assert.equal(shown.children?.[0].collapsed, undefined);
});

test('visibleTree does not mutate the source tree', () => {
	const data = sample();
	assert.ok(data);
	visibleTree(data, new Set());
	assert.equal(data.children?.[0].children?.length, 2);
});

test('truncateLabel shortens long labels with an ellipsis and keeps short ones', () => {
	assert.equal(truncateLabel('Short'), 'Short');
	const long = 'Information technology applications in building and construction industry';
	const cut = truncateLabel(long);
	assert.ok(cut.endsWith('...'));
	assert.ok(labelUnits(cut) <= MAX_LABEL_UNITS);
	assert.ok(long.startsWith(cut.slice(0, -3)));
});

test('truncateLabel counts CJK characters as two units', () => {
	const zh = '电子与电信设备用机电元件通则和检验方法的详细说明文件';
	assert.ok(labelUnits(zh) > MAX_LABEL_UNITS);
	const cut = truncateLabel(zh);
	assert.ok(cut.endsWith('...'));
	assert.ok(labelUnits(cut) <= MAX_LABEL_UNITS);
});

test('nodeLabelText adds the hidden child count to a folded node', () => {
	assert.equal(nodeLabelText({ name: 'Optics', collapsed: false, child_count: 0 }), 'Optics');
	assert.equal(nodeLabelText({ name: 'Optics', collapsed: true, child_count: 7 }), '(+7)  Optics');
});
