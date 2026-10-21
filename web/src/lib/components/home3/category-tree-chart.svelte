<script lang="ts">
	// Category tree canvas. Nodes sit at fixed spacing on an ECharts cartesian canvas, panned and
	// zoomed with the mouse. Clicking a node folds or expands it; clicking a leaf reports it.
	// A miniature of the whole visible tree sits in the bottom-right corner; dragging its
	// viewport rectangle moves the view.
	import { m } from '$lib/paraglide/messages.js';
	import { Chart } from 'svelte-echarts';
	import { init, use, type EChartsType } from 'echarts/core';
	import { LinesChart, ScatterChart } from 'echarts/charts';
	import { DataZoomComponent, GridComponent, TooltipComponent } from 'echarts/components';
	import { CanvasRenderer } from 'echarts/renderers';
	import { untrack } from 'svelte';
	import {
		buildCanvasOption,
		canvasExtent,
		defaultExpanded,
		labelUnits,
		LABEL_GAP,
		layoutTree,
		MAX_LABEL_UNITS,
		NODE_FONT_SIZE,
		NODE_SYMBOL_SIZE,
		nodeLabelText,
		visibleTree,
		type CategoryTreeDatum,
		type EdgeStyle,
		type TreeLayout
	} from './category-tree-model.js';

	use([
		LinesChart,
		ScatterChart,
		DataZoomComponent,
		GridComponent,
		TooltipComponent,
		CanvasRenderer
	]);

	let {
		data,
		layout,
		edgeStyle,
		darkMode = true,
		onleafclick
	}: {
		data: CategoryTreeDatum;
		layout: TreeLayout;
		edgeStyle: EdgeStyle;
		darkMode?: boolean;
		onleafclick?: (datum: CategoryTreeDatum) => void;
	} = $props();

	const MINIMAP_WIDTH = 200;
	const MINIMAP_HEIGHT = 240;
	// Space around the tree inside the miniature, in layout units.
	const MINIMAP_MARGIN = 20;

	let clientWidth = $state(0);
	let clientHeight = $state(0);
	let chart = $state<EChartsType | undefined>(undefined);
	let expanded = $state<Set<string>>(new Set());

	// Labels change with the locale; reset folding only when another tree is loaded.
	const treeId = $derived(data.id);
	$effect(() => {
		void treeId;
		expanded = untrack(() => defaultExpanded(data));
	});

	const colors = $derived(
		darkMode
			? {
					text: '#E2E8F0',
					line: '#64748B',
					leaf: '#A5B4FC',
					node: '#818CF8',
					background: '#0E1116'
				}
			: {
					text: '#1F2937',
					line: '#9CA3AF',
					leaf: '#4F46E5',
					node: '#6366F1',
					background: '#FFFFFF'
				}
	);

	const placed = $derived(layoutTree(visibleTree(data, expanded), layout));
	const extent = $derived(canvasExtent(placed.bounds));
	const xSpan = $derived(extent.xMax - extent.xMin);
	const ySpan = $derived(extent.yMax - extent.yMin);

	// The view is kept in data units, so folding a node does not move what the user is looking at.
	type View = { x0: number; x1: number; y0: number; y1: number };
	type Extent = { xMin: number; xMax: number; yMin: number; yMax: number };
	let view: View | null = null;
	let lastExtent: Extent = { xMin: 0, xMax: 1, yMin: 0, yMax: 1 };
	let lastTreeId: string | null = null;
	let lastLayout: TreeLayout | null = null;
	let lastMeasured = false;

	const clampPct = (v: number) => Math.min(100, Math.max(0, v));

	function viewAt(bounds: typeof placed.bounds, width: number, height: number): View {
		// One data unit is one CSS pixel at 100% zoom; the view starts just above-left of the root.
		const x0 = bounds.minX - 40;
		const y0 = bounds.minY - 40;
		return { x0, x1: x0 + width, y0, y1: y0 + height };
	}

	function percentOf(v: View, ext: Extent): { x: [number, number]; y: [number, number] } {
		const xs = ext.xMax - ext.xMin;
		const ys = ext.yMax - ext.yMin;
		return {
			x: [clampPct(((v.x0 - ext.xMin) / xs) * 100), clampPct(((v.x1 - ext.xMin) / xs) * 100)],
			y: [clampPct(((v.y0 - ext.yMin) / ys) * 100), clampPct(((v.y1 - ext.yMin) / ys) * 100)]
		};
	}

	// Rebuilt when the tree, the layout, the edge style, the folding or the colours change, and once when the
	// container is first measured. A resize alone keeps the zoom (only the measured flag is read).
	const option = $derived.by(() => {
		const measured = clientWidth > 0 && clientHeight > 0;
		const ext = canvasExtent(placed.bounds);
		const fresh = treeId !== lastTreeId || layout !== lastLayout || (measured && !lastMeasured);
		lastTreeId = treeId;
		lastLayout = layout;
		lastMeasured = measured;
		lastExtent = ext;
		if (fresh || !view) {
			const w = untrack(() => clientWidth || 800);
			const h = untrack(() => clientHeight || 560);
			view = viewAt(placed.bounds, w, h);
		}
		const win = percentOf(view, ext);
		return buildCanvasOption(placed, layout, win, colors, edgeStyle);
	});

	// Current window as percent of the canvas, for the miniature's viewport rectangle.
	let zx = $state<[number, number]>([0, 100]);
	let zy = $state<[number, number]>([0, 100]);

	$effect(() => {
		void option;
		if (!view) return;
		const p = percentOf(view, lastExtent);
		zx = p.x;
		zy = p.y;
	});

	// Records a window (percent of the canvas) on one axis, in data units, and mirrors it for the miniature.
	function setAxis(axis: 'zx' | 'zy', start: number, end: number) {
		if (!view) return;
		if (axis === 'zx') {
			view = {
				...view,
				x0: lastExtent.xMin + (start / 100) * (lastExtent.xMax - lastExtent.xMin),
				x1: lastExtent.xMin + (end / 100) * (lastExtent.xMax - lastExtent.xMin)
			};
			zx = [start, end];
		} else {
			view = {
				...view,
				y0: lastExtent.yMin + (start / 100) * (lastExtent.yMax - lastExtent.yMin),
				y1: lastExtent.yMin + (end / 100) * (lastExtent.yMax - lastExtent.yMin)
			};
			zy = [start, end];
		}
	}

	$effect(() => {
		const c = chart;
		if (!c) return;
		const onZoom = (e: unknown) => {
			const ev = e as {
				batch?: { dataZoomId?: string; start?: number; end?: number }[];
				dataZoomId?: string;
				start?: number;
				end?: number;
			};
			for (const it of ev.batch ?? [ev]) {
				if (it.start === undefined || it.end === undefined) continue;
				if (it.dataZoomId === 'zx') setAxis('zx', it.start, it.end);
				else if (it.dataZoomId === 'zy') setAxis('zy', it.start, it.end);
			}
		};
		c.on('datazoom', onZoom);
		return () => {
			c.off('datazoom', onZoom);
		};
	});

	function toggleNode(id: string) {
		const next = new Set(expanded);
		if (next.has(id)) next.delete(id);
		else next.add(id);
		expanded = next;
	}

	function handleClick(event: { componentType?: string; seriesType?: string; data?: unknown }) {
		if (event.componentType !== 'series' || event.seriesType !== 'scatter') return;
		const d = event.data as
			| { id: string; name: string; canonical_name: string; leaf: boolean; is_others: boolean }
			| undefined;
		if (!d) return;
		if (d.leaf) {
			onleafclick?.({
				id: d.id,
				name: d.name,
				canonical_name: d.canonical_name,
				leaf: true,
				is_others: d.is_others
			});
			return;
		}
		toggleNode(d.id);
	}

	// ---- Hover text for truncated labels -----------------------------------------
	// ECharts only reports hovers on the symbol, so the label box is hit-tested here. A truncated
	// label shows its full name next to the pointer.
	type Hover = { name: string; x: number; y: number } | null;
	let hover = $state<Hover>(null);
	const truncatedNodes = $derived(placed.nodes.filter((n) => labelUnits(n.name) > MAX_LABEL_UNITS));

	let measureCtx: CanvasRenderingContext2D | null | undefined;
	function textWidthPx(text: string): number {
		if (measureCtx === undefined) {
			measureCtx =
				typeof document === 'undefined' ? null : document.createElement('canvas').getContext('2d');
		}
		if (!measureCtx) return text.length * NODE_FONT_SIZE * 0.6;
		measureCtx.font = `${NODE_FONT_SIZE}px sans-serif`;
		return measureCtx.measureText(text).width;
	}

	// Pixel box of a node's label at the current zoom, mirroring labelStyle's placement.
	function labelBoxPx(
		ax: number,
		ay: number,
		text: string
	): { x0: number; x1: number; y0: number; y1: number } {
		const w = textWidthPx(text);
		const h = NODE_FONT_SIZE * 1.4;
		const gap = LABEL_GAP;
		switch (layout) {
			case 'LR':
				return { x0: ax + gap, x1: ax + gap + w, y0: ay - h / 2, y1: ay + h / 2 };
			case 'RL':
				return { x0: ax - gap - w, x1: ax - gap, y0: ay - h / 2, y1: ay + h / 2 };
			case 'TB':
				return { x0: ax - h / 2, x1: ax + h / 2, y0: ay + gap, y1: ay + gap + w };
			case 'BT':
				return { x0: ax - h / 2, x1: ax + h / 2, y0: ay - gap - w, y1: ay - gap };
		}
	}

	$effect(() => {
		const c = chart;
		if (!c) return;
		const zr = c.getZr();
		const onMove = (e: { offsetX: number; offsetY: number }) => {
			let found: Hover = null;
			for (const n of truncatedNodes) {
				const [ax, ay] = c.convertToPixel({ xAxisIndex: 0, yAxisIndex: 0 }, [n.x, n.y]) as number[];
				const box = labelBoxPx(ax, ay, nodeLabelText(n));
				const pad = 3;
				if (
					e.offsetX >= box.x0 - pad &&
					e.offsetX <= box.x1 + pad &&
					e.offsetY >= box.y0 - pad &&
					e.offsetY <= box.y1 + pad
				) {
					found = { name: n.name, x: e.offsetX, y: e.offsetY };
					break;
				}
			}
			hover = found;
		};
		const onOut = () => {
			hover = null;
		};
		zr.on('mousemove', onMove);
		zr.on('globalout', onOut);
		return () => {
			zr.off('mousemove', onMove);
			zr.off('globalout', onOut);
		};
	});

	// ---- Miniature -------------------------------------------------------------
	// The miniature maps the whole visible tree onto its rectangle. Each axis has its own scale,
	// so the tree fills the rectangle; a viewport rectangle outside the tree is clipped.
	const mm = $derived.by(() => {
		const b = placed.bounds;
		const x0 = b.minX - MINIMAP_MARGIN;
		const y0 = b.minY - MINIMAP_MARGIN;
		const spanX = Math.max(b.maxX - b.minX, 1) + 2 * MINIMAP_MARGIN;
		const spanY = Math.max(b.maxY - b.minY, 1) + 2 * MINIMAP_MARGIN;
		return { x0, y0, sx: MINIMAP_WIDTH / spanX, sy: MINIMAP_HEIGHT / spanY };
	});

	const viewRect = $derived.by(() => {
		const dx0 = extent.xMin + (zx[0] / 100) * xSpan;
		const dx1 = extent.xMin + (zx[1] / 100) * xSpan;
		const dy0 = extent.yMin + (zy[0] / 100) * ySpan;
		const dy1 = extent.yMin + (zy[1] / 100) * ySpan;
		return {
			x: (dx0 - mm.x0) * mm.sx,
			y: (dy0 - mm.y0) * mm.sy,
			w: (dx1 - dx0) * mm.sx,
			h: (dy1 - dy0) * mm.sy
		};
	});

	let mapEl = $state<SVGSVGElement | undefined>(undefined);
	let drag: { x: number; y: number; zx: [number, number]; zy: [number, number] } | null = null;

	// Moves one axis of the view so that its window keeps its span, starting at `start` (percent).
	function moveAxis(axis: 'zx' | 'zy', start: number, span: number) {
		const s = Math.min(Math.max(start, 0), 100 - span);
		setAxis(axis, s, s + span);
		chart?.dispatchAction({ type: 'dataZoom', dataZoomId: axis, start: s, end: s + span });
	}

	function pointerToMap(e: PointerEvent): { x: number; y: number } {
		const r = mapEl?.getBoundingClientRect();
		return { x: e.clientX - (r?.left ?? 0), y: e.clientY - (r?.top ?? 0) };
	}

	function onMapDown(e: PointerEvent) {
		const p = pointerToMap(e);
		const inside =
			p.x >= viewRect.x &&
			p.x <= viewRect.x + viewRect.w &&
			p.y >= viewRect.y &&
			p.y <= viewRect.y + viewRect.h;
		if (!inside) {
			// A press outside the viewport centres the view on that point.
			const cx = mm.x0 + p.x / mm.sx;
			const cy = mm.y0 + p.y / mm.sy;
			const spanX = zx[1] - zx[0];
			const spanY = zy[1] - zy[0];
			moveAxis('zx', ((cx - extent.xMin) / xSpan) * 100 - spanX / 2, spanX);
			moveAxis('zy', ((cy - extent.yMin) / ySpan) * 100 - spanY / 2, spanY);
		}
		drag = {
			x: e.clientX,
			y: e.clientY,
			zx: [...zx] as [number, number],
			zy: [...zy] as [number, number]
		};
		(e.currentTarget as Element).setPointerCapture(e.pointerId);
	}

	function onMapMove(e: PointerEvent) {
		if (!drag) return;
		// Miniature pixels -> layout units -> percent of the canvas extent.
		const dx = ((e.clientX - drag.x) / mm.sx / xSpan) * 100;
		const dy = ((e.clientY - drag.y) / mm.sy / ySpan) * 100;
		moveAxis('zx', drag.zx[0] + dx, drag.zx[1] - drag.zx[0]);
		moveAxis('zy', drag.zy[0] + dy, drag.zy[1] - drag.zy[0]);
	}

	function onMapUp(e: PointerEvent) {
		drag = null;
		(e.currentTarget as Element).releasePointerCapture?.(e.pointerId);
	}
</script>

<div class="category-tree-chart" bind:clientWidth bind:clientHeight>
	<Chart
		bind:chart
		{init}
		options={option}
		notMerge
		style="width:100%; height:100%;"
		onclick={handleClick}
	/>

	{#if hover}
		<div
			class="category-tree-hover"
			class:light={!darkMode}
			style="left:{hover.x + 14}px; top:{hover.y + 14}px"
		>
			{hover.name}
		</div>
	{/if}

	<svg
		class="category-tree-minimap"
		bind:this={mapEl}
		width={MINIMAP_WIDTH}
		height={MINIMAP_HEIGHT}
		viewBox={`0 0 ${MINIMAP_WIDTH} ${MINIMAP_HEIGHT}`}
		role="application"
		aria-label={m.home3_categories_minimap_label()}
		onpointerdown={onMapDown}
		onpointermove={onMapMove}
		onpointerup={onMapUp}
		onpointercancel={onMapUp}
	>
		<rect class="mm-bg" x="0" y="0" width={MINIMAP_WIDTH} height={MINIMAP_HEIGHT} rx="4" />
		{#each placed.nodes as n (n.id)}
			<rect
				class={n.leaf ? 'mm-node mm-leaf' : 'mm-node'}
				x={(n.x - mm.x0) * mm.sx - 1}
				y={(n.y - mm.y0) * mm.sy - 1}
				width="2"
				height="2"
			/>
		{/each}
		<rect
			class="mm-view"
			x={viewRect.x}
			y={viewRect.y}
			width={Math.max(viewRect.w, 4)}
			height={Math.max(viewRect.h, 4)}
			rx="2"
		/>
	</svg>
</div>

<style>
	.category-tree-chart {
		position: relative;
		width: 100%;
		height: 100%;
		min-height: 0;
		overflow: hidden;
	}
	.category-tree-minimap {
		position: absolute;
		right: 8px;
		bottom: 8px;
		width: 200px;
		height: 240px;
		overflow: hidden;
		touch-action: none;
		cursor: grab;
		border-radius: 4px;
	}
	.category-tree-hover {
		position: absolute;
		z-index: 2;
		max-width: 480px;
		padding: 4px 8px;
		border-radius: 4px;
		font-size: 13px;
		line-height: 1.4;
		pointer-events: none;
		color: #e2e8f0;
		background: rgba(15, 23, 42, 0.95);
		border: 1px solid rgba(148, 163, 184, 0.35);
	}
	.category-tree-hover.light {
		color: #1f2937;
		background: rgba(255, 255, 255, 0.97);
	}
	.category-tree-minimap:active {
		cursor: grabbing;
	}
	.mm-bg {
		fill: rgba(15, 23, 42, 0.8);
		stroke: rgba(148, 163, 184, 0.45);
	}
	.mm-node {
		fill: #818cf8;
	}
	.mm-leaf {
		fill: #a5b4fc;
	}
	.mm-view {
		fill: rgba(129, 140, 248, 0.2);
		stroke: #c7d2fe;
		stroke-width: 1.5;
	}
</style>
