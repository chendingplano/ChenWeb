// Pure model for the Categories page: turns the flat category-tree node list into
// ECharts tree data, and holds the four layouts (LR, RL, TB, BT). Kept free of
// Svelte and ECharts imports so it can be unit-tested with node:test.
//
// Layouts follow the ECharts examples: tree-basic (LR), tree-orient-right-left (RL),
// tree-vertical (TB) and tree-orient-bottom-top (BT).

import type { EChartsOption } from 'echarts';
import type { CategoryTreeNode } from '$lib/services/categoryTreeService';

export const TREE_LAYOUTS = ['LR', 'RL', 'TB', 'BT'] as const;
export type TreeLayout = (typeof TREE_LAYOUTS)[number];
export const DEFAULT_TREE_LAYOUT: TreeLayout = 'LR';

// How a parent is joined to its children: right-angled elbows, or S-curves as in the
// ECharts tree examples.
export const EDGE_STYLES = ['broken', 'curve'] as const;
export type EdgeStyle = (typeof EDGE_STYLES)[number];
export const DEFAULT_EDGE_STYLE: EdgeStyle = 'curve';

// The datum ECharts receives for each node. `id` is the node ID the members
// endpoint needs; `leaf` says whether a click should open the artifact panel.
export type CategoryTreeDatum = {
	id: string;
	name: string;
	canonical_name: string;
	leaf: boolean;
	is_others: boolean;
	children?: CategoryTreeDatum[];
	// Set on a node that has children but is folded: those children are not drawn.
	collapsed?: boolean;
	child_count?: number;
};

// Label language keys in the stored labels are "en" and "zh" (categorytree/tree.go),
// while the app locale is "en" or "zh-cn".
export function labelLang(locale: string): 'en' | 'zh' {
	return locale === 'zh-cn' || locale.startsWith('zh') ? 'zh' : 'en';
}

export function nodeLabel(node: CategoryTreeNode, lang: 'en' | 'zh'): string {
	return node.pref_labels[lang] || node.pref_labels.en || node.canonical_name;
}

// Builds the nested datum tree from the depth-ordered node list the API returns.
// Returns null when the list is empty. Children keep the order they appear in the list.
export function buildTreeData(
	nodes: CategoryTreeNode[],
	lang: 'en' | 'zh'
): CategoryTreeDatum | null {
	if (nodes.length === 0) return null;
	const byId = new Map<string, CategoryTreeDatum>();
	for (const n of nodes) {
		byId.set(n.id, {
			id: n.id,
			name: nodeLabel(n, lang),
			canonical_name: n.canonical_name,
			leaf: n.is_leaf,
			is_others: n.is_others
		});
	}
	let root: CategoryTreeDatum | null = null;
	for (const n of nodes) {
		const datum = byId.get(n.id);
		if (!datum) continue;
		if (!n.parent_id) {
			root = datum;
			continue;
		}
		const parent = byId.get(n.parent_id);
		if (!parent) continue;
		(parent.children ??= []).push(datum);
	}
	return root;
}

// Label placement per layout. Horizontal trees put labels beside the node; vertical
// trees rotate them 90 degrees, as the ECharts vertical examples do.
// Node symbol and label size. The symbol is twice the original 6px.
export const NODE_SYMBOL_SIZE = 12;
export const NODE_FONT_SIZE = 14;

export type LabelStyle = {
	position: 'left' | 'right' | 'top' | 'bottom';
	distance: number;
	rotate?: number;
	verticalAlign: 'middle';
	align: 'left' | 'right';
};

// Label placement per layout, the same for folded and leaf nodes: right of the circle for
// left-right, left for right-left, below for top-bottom and above for bottom-top. Vertical
// layouts rotate the text 90 degrees so that it runs along the tree's main axis.
// The rotation pivots on the label's anchor point, so a vertical label is anchored just past
// the circle and aligned so that its text runs away from it (a pixel offset would be rotated
// along with the text and move the label sideways).
export function labelStyle(layout: TreeLayout, _leaf: boolean): LabelStyle {
	const distance = LABEL_DISTANCE;
	switch (layout) {
		case 'LR':
			return { position: 'right', distance, verticalAlign: 'middle', align: 'left' };
		case 'RL':
			return { position: 'left', distance, verticalAlign: 'middle', align: 'right' };
		case 'TB':
			// Rotated 90 degrees clockwise, the text reads top-down and starts at the circle.
			return { position: 'bottom', distance, rotate: -90, verticalAlign: 'middle', align: 'left' };
		case 'BT':
			// Rotated 90 degrees clockwise, the text reads top-down and ends at the circle.
			return { position: 'top', distance, rotate: -90, verticalAlign: 'middle', align: 'right' };
	}
}

// Gap between a node's symbol edge and its label, and from the node centre, in pixels.
const LABEL_DISTANCE = 10;
export const LABEL_GAP = NODE_SYMBOL_SIZE / 2 + LABEL_DISTANCE;

// Labels longer than this many width units are truncated with "...". A CJK character counts
// as two units, since it is about twice as wide as a Latin one at the same font size.
export const MAX_LABEL_UNITS = 40;
const ELLIPSIS = '...';
const CJK = /[\u2E80-\u9FFF\uF900-\uFAFF\uFF00-\uFFEF\u3000-\u303F]/;

export function labelUnits(text: string): number {
	let units = 0;
	for (const ch of text) units += CJK.test(ch) ? 2 : 1;
	return units;
}

export function truncateLabel(text: string, maxUnits = MAX_LABEL_UNITS): string {
	if (labelUnits(text) <= maxUnits) return text;
	const budget = maxUnits - labelUnits(ELLIPSIS);
	let used = 0;
	let out = '';
	for (const ch of text) {
		const w = CJK.test(ch) ? 2 : 1;
		if (used + w > budget) break;
		out += ch;
		used += w;
	}
	return out + ELLIPSIS;
}

// Text shown next to a node: its name, truncated, led for a folded node by the hidden child count.
export function nodeLabelText(n: {
	name: string;
	collapsed: boolean;
	child_count: number;
}): string {
	const base = truncateLabel(n.name);
	return n.collapsed ? `(+${n.child_count})  ${base}` : base;
}

// Fixed spacing, in CSS pixels at 100% zoom. Horizontal layouts run the tree
// left-right or right-left, so a level is a column and a leaf is a row; vertical
// layouts swap the roles, and their labels are rotated 90 degrees, so a row is narrow.
export const TREE_SPACING = {
	// 340px fits a label of MAX_LABEL_UNITS at 14px with room to spare.
	horizontal: { levelGap: 340, rowGap: 24 },
	// Vertical labels run along the main axis, so a level is as long as a label.
	vertical: { levelGap: 300, rowGap: 22 }
} as const;

// Two levels are shown by default: the root and its direct children. Only the root is expanded.
export function defaultExpanded(root: CategoryTreeDatum): Set<string> {
	return new Set([root.id]);
}

// The part of the tree that is drawn: a folded node keeps its own datum but loses its
// children, and is marked with how many children it hides.
export function visibleTree(root: CategoryTreeDatum, expanded: Set<string>): CategoryTreeDatum {
	const copy: CategoryTreeDatum = { ...root };
	delete copy.children;
	delete copy.collapsed;
	delete copy.child_count;
	const kids = root.children ?? [];
	if (kids.length === 0) return copy;
	if (expanded.has(root.id)) {
		copy.children = kids.map((c) => visibleTree(c, expanded));
	} else {
		copy.collapsed = true;
		copy.child_count = kids.length;
	}
	return copy;
}

export type PlacedNode = {
	id: string;
	name: string;
	canonical_name: string;
	leaf: boolean;
	is_others: boolean;
	depth: number;
	x: number;
	y: number;
	collapsed: boolean;
	child_count: number;
};

export type PlacedEdge = { parent: string; child: string; points: [number, number][] };

export type TreeLayoutResult = {
	nodes: PlacedNode[];
	edges: PlacedEdge[];
	bounds: { minX: number; maxX: number; minY: number; maxY: number };
};

// Places every node at a fixed distance from its neighbours. Leaves take consecutive
// rows in depth-first order; an inner node sits midway between its first and last child.
// Screen coordinates grow downwards: the root is at the top for TB, at the bottom for BT,
// on the left for LR and on the right for RL.
export function layoutTree(data: CategoryTreeDatum, layout: TreeLayout): TreeLayoutResult {
	const horizontal = layout === 'LR' || layout === 'RL';
	const { levelGap, rowGap } = horizontal ? TREE_SPACING.horizontal : TREE_SPACING.vertical;
	const sign = layout === 'RL' || layout === 'BT' ? -1 : 1;

	const rows = new Map<string, number>();
	const depths = new Map<string, number>();
	let nextRow = 0;
	const place = (d: CategoryTreeDatum, depth: number): number => {
		depths.set(d.id, depth);
		let row: number;
		if (!d.children || d.children.length === 0) {
			row = nextRow++;
		} else {
			const first = place(d.children[0], depth + 1);
			let last = first;
			for (const c of d.children.slice(1)) last = place(c, depth + 1);
			row = (first + last) / 2;
		}
		rows.set(d.id, row);
		return row;
	};
	place(data, 0);

	const at = (id: string): [number, number] => {
		const depth = depths.get(id) ?? 0;
		const cross = (rows.get(id) ?? 0) * rowGap;
		const main = sign * depth * levelGap;
		return horizontal ? [main, cross] : [cross, main];
	};

	const nodes: PlacedNode[] = [];
	const edges: PlacedEdge[] = [];
	const walk = (d: CategoryTreeDatum) => {
		const [x, y] = at(d.id);
		nodes.push({
			id: d.id,
			name: d.name,
			canonical_name: d.canonical_name,
			leaf: d.leaf,
			is_others: d.is_others,
			depth: depths.get(d.id) ?? 0,
			x,
			y,
			collapsed: d.collapsed ?? false,
			child_count: d.child_count ?? 0
		});
		for (const c of d.children ?? []) {
			const [cx, cy] = at(c.id);
			// Elbow connector: out along the main axis, across, then into the child.
			const points: [number, number][] = horizontal
				? [
						[x, y],
						[(x + cx) / 2, y],
						[(x + cx) / 2, cy],
						[cx, cy]
					]
				: [
						[x, y],
						[x, (y + cy) / 2],
						[cx, (y + cy) / 2],
						[cx, cy]
					];
			edges.push({ parent: d.id, child: c.id, points });
			walk(c);
		}
	};
	walk(data);

	const xs = nodes.map((n) => n.x);
	const ys = nodes.map((n) => n.y);
	return {
		nodes,
		edges,
		bounds: {
			minX: Math.min(...xs),
			maxX: Math.max(...xs),
			minY: Math.min(...ys),
			maxY: Math.max(...ys)
		}
	};
}

// The canvas is the tree's bounds plus a margin of the tree's own size on each side,
// so the view can be panned well past the outermost nodes. Zooming is limited to this canvas.
export function canvasExtent(bounds: TreeLayoutResult['bounds']) {
	const padX = Math.max(600, bounds.maxX - bounds.minX);
	const padY = Math.max(600, bounds.maxY - bounds.minY);
	return {
		xMin: bounds.minX - padX,
		xMax: bounds.maxX + padX,
		yMin: bounds.minY - padY,
		yMax: bounds.maxY + padY
	};
}

// Percent window (0-100 over the canvas extent) that shows `width` x `height` pixels at
// 100% zoom, anchored at the tree's top-left corner, so the root is in view at first.
export function initialWindow(
	extent: ReturnType<typeof canvasExtent>,
	bounds: TreeLayoutResult['bounds'],
	width: number,
	height: number
): { x: [number, number]; y: [number, number] } {
	const xSpan = extent.xMax - extent.xMin;
	const ySpan = extent.yMax - extent.yMin;
	const x0 = bounds.minX - 40;
	const y0 = bounds.minY - 40;
	const clamp = (v: number) => Math.min(100, Math.max(0, v));
	return {
		x: [
			clamp(((x0 - extent.xMin) / xSpan) * 100),
			clamp(((x0 + Math.max(width, 1) - extent.xMin) / xSpan) * 100)
		],
		y: [
			clamp(((y0 - extent.yMin) / ySpan) * 100),
			clamp(((y0 + Math.max(height, 1) - extent.yMin) / ySpan) * 100)
		]
	};
}

// ECharts label text treats { and } as rich-text markers, so they are removed from names.
function escapeLabel(text: string): string {
	return text.replace(/[{}]/g, '');
}

// Points drawn for an edge. A broken line is the elbow itself; a curve is the cubic Bézier whose
// control points are the elbow's four points (parent, both bends, child), sampled as a polyline.
const CURVE_SEGMENTS = 16;
export function edgePath(points: [number, number][], style: EdgeStyle): [number, number][] {
	if (style === 'broken' || points.length !== 4) return points;
	const [p0, p1, p2, p3] = points;
	const out: [number, number][] = [];
	for (let i = 0; i <= CURVE_SEGMENTS; i++) {
		const t = i / CURVE_SEGMENTS;
		const u = 1 - t;
		const a = u * u * u;
		const b = 3 * u * u * t;
		const c = 3 * u * t * t;
		const d = t * t * t;
		out.push([
			a * p0[0] + b * p1[0] + c * p2[0] + d * p3[0],
			a * p0[1] + b * p1[1] + c * p2[1] + d * p3[1]
		]);
	}
	return out;
}

// ECharts option: a cartesian canvas with fixed-spacing nodes (scatter), elbow edges
// (polyline lines) and pan/zoom through dataZoom. filterMode 'none' keeps every edge
// drawn while the view moves.
export function buildCanvasOption(
	result: TreeLayoutResult,
	layout: TreeLayout,
	window: { x: [number, number]; y: [number, number] },
	colors: { text: string; line: string; leaf: string; node: string; background: string },
	edgeStyle: EdgeStyle = DEFAULT_EDGE_STYLE
): EChartsOption {
	const extent = canvasExtent(result.bounds);
	const byId = new Map(result.nodes.map((n) => [n.id, n]));
	return {
		animation: false,
		grid: { left: 0, right: 0, top: 0, bottom: 0, containLabel: false },
		// Hover text for truncated labels is drawn by the chart component, not by ECharts.
		tooltip: { show: false },
		xAxis: { type: 'value', show: false, min: extent.xMin, max: extent.xMax },
		yAxis: { type: 'value', show: false, min: extent.yMin, max: extent.yMax, inverse: true },
		dataZoom: [
			{
				id: 'zx',
				type: 'inside',
				xAxisIndex: 0,
				filterMode: 'none',
				start: window.x[0],
				end: window.x[1],
				zoomLock: false,
				moveOnMouseMove: true,
				zoomOnMouseWheel: true
			},
			{
				id: 'zy',
				type: 'inside',
				yAxisIndex: 0,
				filterMode: 'none',
				start: window.y[0],
				end: window.y[1],
				zoomLock: false,
				moveOnMouseMove: true,
				zoomOnMouseWheel: true
			}
		],
		series: [
			{
				type: 'lines',
				coordinateSystem: 'cartesian2d',
				polyline: true,
				silent: true,
				symbol: ['none', 'none'],
				lineStyle: { color: colors.line, width: 1, opacity: 0.8 },
				data: result.edges.map((e) => ({ coords: edgePath(e.points, edgeStyle) }))
			},
			{
				type: 'scatter',
				symbolSize: NODE_SYMBOL_SIZE,
				itemStyle: { color: colors.node, borderColor: colors.line, borderWidth: 0.5 },
				data: result.nodes.map((n) => ({
					value: [n.x, n.y],
					name: n.name,
					id: n.id,
					canonical_name: n.canonical_name,
					leaf: n.leaf,
					is_others: n.is_others,
					collapsed: n.collapsed,
					// A folded node is hollow and shows how many children it hides.
					itemStyle: n.collapsed
						? {
								color: colors.background,
								borderColor: colors.node,
								borderWidth: 2
							}
						: n.leaf
							? { color: colors.leaf }
							: undefined,
					label: {
						show: true,
						fontSize: NODE_FONT_SIZE,
						color: n.leaf ? colors.leaf : colors.text,
						formatter: escapeLabel(nodeLabelText(n)),
						...labelStyle(layout, n.leaf)
					}
				}))
			}
		]
	};
}
