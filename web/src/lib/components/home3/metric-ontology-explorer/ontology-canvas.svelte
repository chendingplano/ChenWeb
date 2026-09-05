<script lang="ts">
	// Hand-rolled SVG orrery. No graph library.
	//
	// Layout invariants:
	//   satellites on an ellipse  rx = clamp(hostW*0.32, 240, 430)
	//                             ry = clamp(hostH*0.30,  88, 150)
	//   satellite i angle         base(i) = (-90 + i*40)deg
	//   open chain: `rotTarget` brings the active satellite to angle 0 (3 o'clock);
	//               chain nodes hang in a vertical column STEP px to its right,
	//               ROW px apart, centred on the satellite's y.
	//   pan/zoom is a translate(x,y) scale(k) on the root <g>; k clamped to [0.42, 1.35]
	//   node <g> transforms always CSS-transition; the root only while `animating`.

	import { scale } from 'svelte/transition';
	import { CENTER, SATELLITES, chainFor, CHAINED_SATELLITES, type Glyph } from './model';
	import type { ExplorerTokens } from './theme';

	let {
		tokens,
		focusId,
		openChain,
		onfocus,
		ontogglechain,
		onopenrecord
	}: {
		tokens: ExplorerTokens;
		focusId: string;
		openChain: string | null;
		onfocus: (id: string) => void;
		ontogglechain: (satelliteId: string) => void;
		onopenrecord: (chainNodeId: string) => void;
	} = $props();

	const GLYPH: Record<Glyph, string> = {
		metric: 'M8 2.2a5.8 5.8 0 100 11.6 5.8 5.8 0 000-11.6ZM8 5v6M5 8h6',
		document: 'M4.2 2.2h5L12 5v8.8H4.2ZM9.2 2.2V5H12M5.8 8.2h4.4M5.8 10.6h4.4',
		object: 'M8 2.2l5.4 3.1v5.4L8 13.8l-5.4-3.1V5.3ZM2.6 5.3L8 8.4l5.4-3.1M8 8.4v5.4',
		keyword: 'M2.4 8.6l6-6h3.4l.1 3.4-6 6ZM10.2 4.1h.01',
		evidence: 'M6.7 2.6a3.8 3.8 0 100 7.6 3.8 3.8 0 000-7.6ZM9.5 9.5l3.9 3.9',
		mdef: 'M2.4 2.4h11.2v11.2H2.4ZM2.4 6h11.2M6 2.4v11.2',
		processor: 'M3.2 3l4.4 5-4.4 5M8 3l4.4 5L8 13',
		product: 'M2.6 3.2h4v9.6h-4ZM8.4 4.6l3.2 3.4-3.2 3.4M6.8 8h4.6',
		analysis: 'M2.6 13.4V2.6M2.6 13.4h10.8M5.4 10.2l2.6-2.8 2 1.8 2.8-3.6',
		misc: 'M5.6 3v10M10.4 3v10',
		entity: 'M3 3h10v10H3ZM6 6h4v4H6Z',
		stage: 'M5.2 3.2l7 4.8-7 4.8Z'
	};

	const STEP = 212;
	const ROW = 48;
	const DEG = Math.PI / 180;
	const base = (i: number) => (-90 + i * (360 / SATELLITES.length)) * DEG;
	const clamp = (v: number, lo: number, hi: number) => Math.max(lo, Math.min(hi, v));
	const satIndex = (id: string) => SATELLITES.findIndex((s) => s.id === id);

	let hostEl: HTMLDivElement | null = $state(null);
	let hostW = $state(900);
	let hostH = $state(360);
	function measure() {
		if (hostEl) {
			hostW = hostEl.clientWidth || hostW;
			hostH = hostEl.clientHeight || hostH;
		}
	}
	$effect(() => {
		if (!hostEl) return;
		measure();
		const ro = new ResizeObserver(measure);
		ro.observe(hostEl);
		return () => ro.disconnect();
	});
	const RX = $derived(clamp(hostW * 0.32, 240, 430));
	const RY = $derived(clamp(hostH * 0.3, 88, 150));
	const rotTarget = $derived(openChain ? -base(satIndex(openChain)) : 0);

	type LNode = {
		id: string;
		kind: 'core' | 'sat' | 'chain';
		label: string;
		sub?: string;
		glyph: Glyph;
		x: number;
		y: number;
		w: number;
		h: number;
	};

	function satW(label: string) {
		return clamp(38 + 14 + 8 + label.length * 7.2 + 16, 108, 210);
	}
	function chainW(label: string) {
		return clamp(30 + 12 + 7 + label.length * 6.6 + 14, 96, 214);
	}

	const nodes = $derived.by<LNode[]>(() => {
		const out: LNode[] = [
			{ id: CENTER.id, kind: 'core', label: 'Metric', sub: 'ONTOLOGY CORE', glyph: 'metric', x: 0, y: 0, w: 178, h: 66 }
		];
		SATELLITES.forEach((s, i) => {
			const a = base(i) + rotTarget;
			out.push({
				id: s.id,
				kind: 'sat',
				label: s.label,
				sub: s.kind.toUpperCase(),
				glyph: s.glyph,
				x: RX * Math.cos(a),
				y: RY * Math.sin(a),
				w: satW(s.label),
				h: 44
			});
		});
		if (openChain) {
			const i = satIndex(openChain);
			const a = base(i) + rotTarget;
			const sx = RX * Math.cos(a);
			const sy = RY * Math.sin(a);
			const chain = chainFor(openChain);
			chain.forEach((c, k) => {
				out.push({
					id: c.id,
					kind: 'chain',
					label: c.label,
					glyph: c.glyph,
					x: sx + STEP,
					y: sy + (k - (chain.length - 1) / 2) * ROW,
					w: chainW(c.label),
					h: 34
				});
			});
		}
		return out;
	});

	const posOf = $derived(new Map(nodes.map((n) => [n.id, n])));

	type LEdge = { id: string; x1: number; y1: number; x2: number; y2: number; chain: boolean };
	const edges = $derived.by<LEdge[]>(() => {
		const out: LEdge[] = [];
		for (const s of SATELLITES) {
			const p = posOf.get(s.id);
			if (p) out.push({ id: 'm>' + s.id, x1: 0, y1: 0, x2: p.x, y2: p.y, chain: false });
		}
		if (openChain) {
			const chain = chainFor(openChain);
			let prev = openChain;
			for (const c of chain) {
				const a = posOf.get(prev);
				const b = posOf.get(c.id);
				if (a && b) out.push({ id: prev + '>' + c.id, x1: a.x, y1: a.y, x2: b.x, y2: b.y, chain: true });
				prev = c.id;
			}
		}
		return out;
	});

	// ---- pan / zoom ----
	let tx = $state(0);
	let ty = $state(0);
	let k = $state(1);
	let animating = $state(false);

	function fitView(animate = true) {
		if (!nodes.length) return;
		measure();
		let minX = Infinity, maxX = -Infinity, minY = Infinity, maxY = -Infinity;
		for (const n of nodes) {
			minX = Math.min(minX, n.x - n.w / 2);
			maxX = Math.max(maxX, n.x + n.w / 2);
			minY = Math.min(minY, n.y - n.h / 2 - (n.kind === 'sat' ? 16 : 0));
			maxY = Math.max(maxY, n.y + n.h / 2);
		}
		// generous vertical padding keeps a legible margin in the short Top-Bottom
		// canvas band; extra right padding when a chain is unfurled to its right
		const padX = 90 + (openChain ? 120 : 0);
		const padY = 78;
		const nk = clamp(Math.min(hostW / (maxX - minX + padX * 2), hostH / (maxY - minY + padY * 2)), 0.42, 1.3);
		const cx = (minX + maxX) / 2, cy = (minY + maxY) / 2;
		if (animate) {
			animating = true;
			setTimeout(() => (animating = false), 620);
		}
		k = nk;
		tx = hostW / 2 - cx * nk;
		ty = hostH / 2 - cy * nk;
	}

	// re-fit when the diagram's shape or the host size changes; defer a frame so
	// bind:clientWidth/Height have flushed for the new layout.
	$effect(() => {
		void openChain;
		void nodes.length;
		void hostW;
		void hostH;
		const raf = requestAnimationFrame(() => fitView(true));
		return () => cancelAnimationFrame(raf);
	});

	// Pan without stealing node clicks: record the press, but only capture the
	// pointer (which would suppress the <g> click) once the drag passes a
	// movement threshold.
	let down: { x: number; y: number; tx: number; ty: number; id: number } | null = null;
	let panning = false;
	function bgPointerDown(e: PointerEvent) {
		if (e.button !== 0) return;
		down = { x: e.clientX, y: e.clientY, tx, ty, id: e.pointerId };
		panning = false;
	}
	function bgPointerMove(e: PointerEvent) {
		if (!down) return;
		const dx = e.clientX - down.x;
		const dy = e.clientY - down.y;
		if (!panning && Math.hypot(dx, dy) > 3) {
			panning = true;
			animating = false;
			try {
				(e.currentTarget as HTMLElement).setPointerCapture(down.id);
			} catch {
				/* ignore */
			}
		}
		if (panning) {
			tx = down.tx + dx;
			ty = down.ty + dy;
		}
	}
	function bgPointerUp(e: PointerEvent) {
		if (down && panning) {
			try {
				(e.currentTarget as HTMLElement).releasePointerCapture(down.id);
			} catch {
				/* ignore */
			}
		}
		down = null;
		panning = false;
	}

	function onWheel(e: WheelEvent) {
		e.preventDefault();
		animating = false;
		const rect = hostEl?.getBoundingClientRect();
		const px = (rect ? e.clientX - rect.left : hostW / 2) - tx;
		const py = (rect ? e.clientY - rect.top : hostH / 2) - ty;
		const factor = e.deltaY < 0 ? 1.12 : 1 / 1.12;
		const nk = clamp(k * factor, 0.25, 3);
		tx += px - (px / k) * nk;
		ty += py - (py / k) * nk;
		k = nk;
	}
	$effect(() => {
		const el = hostEl;
		if (!el) return;
		el.addEventListener('wheel', onWheel, { passive: false });
		return () => el.removeEventListener('wheel', onWheel);
	});

	function nodeClick(n: LNode) {
		if (n.kind === 'chain') {
			onopenrecord(n.id);
			return;
		}
		onfocus(n.id);
		if (n.kind === 'sat' && CHAINED_SATELLITES.has(n.id)) ontogglechain(n.id);
	}

	// ---- breadcrumb ----
	const crumbs = $derived(openChain ? chainFor(openChain) : []);
</script>

<div
	class="canvas-host"
	bind:this={hostEl}
	style="--bg:{tokens.pageBg}; --panel:{tokens.panelBg}; --border:{tokens.border}; --text:{tokens.textPrimary}; --text-2:{tokens.textSecondary}; --accent:{tokens.accent}; --card:{tokens.cardBg};"
>
	<svg
		class="graph"
		role="presentation"
		onpointerdown={bgPointerDown}
		onpointermove={bgPointerMove}
		onpointerup={bgPointerUp}
		onpointercancel={bgPointerUp}
	>
		<g class="scene" class:animating transform="translate({tx},{ty}) scale({k})">
			{#each edges as e (e.id)}
				<line class="edge" class:chain={e.chain} x1={e.x1} y1={e.y1} x2={e.x2} y2={e.y2} />
			{/each}
			{#each nodes as n (n.id)}
				<g
					class="node {n.kind}"
					class:focused={n.id === focusId}
					transform="translate({n.x},{n.y})"
					role="button"
					tabindex="0"
					onclick={(ev) => {
						ev.stopPropagation();
						nodeClick(n);
					}}
					onkeydown={(ev) => {
						if (ev.key === 'Enter' || ev.key === ' ') {
							ev.preventDefault();
							nodeClick(n);
						}
					}}
					in:scale={n.kind === 'chain' ? { duration: 240, start: 0.85 } : { duration: 0 }}
					out:scale={n.kind === 'chain' ? { duration: 180, start: 0.85 } : { duration: 0 }}
				>
					<rect class="box" x={-n.w / 2} y={-n.h / 2} width={n.w} height={n.h} rx={n.kind === 'chain' ? 6 : 9} />
					<path class="icon" d={GLYPH[n.glyph]} transform="translate({-n.w / 2 + 12},{n.kind === 'core' ? -10 : -7}) scale({n.kind === 'core' ? 20 / 16 : 14 / 16})" />
					{#if n.kind === 'core'}
						<text class="label" x={-n.w / 2 + 40} y={-3}>{n.label}</text>
						<text class="sub" x={-n.w / 2 + 40} y={13}>{n.sub}</text>
					{:else}
						<text class="label" x={-n.w / 2 + (n.kind === 'chain' ? 32 : 34)} y={1}>{n.label}</text>
						{#if n.kind === 'sat'}
							<text class="kind" x={0} y={-n.h / 2 - 7}>{n.sub}</text>
						{/if}
					{/if}
				</g>
			{/each}
		</g>
	</svg>

	{#if crumbs.length}
		<div class="trail">
			<b>Metric</b>
			<span class="sep">·</span>
			<span>{openChain ? SATELLITES[satIndex(openChain)].label : ''}</span>
			{#each crumbs as c (c.id)}
				<button class="crumb" onclick={() => onopenrecord(c.id)}>▸ {c.label}</button>
			{/each}
		</div>
	{/if}

	<div class="controls">
		<button onclick={() => fitView(true)} title="Fit">⤢</button>
		<button onclick={() => { animating = false; k = clamp(k * 1.15, 0.25, 3); }} title="Zoom in">+</button>
		<button onclick={() => { animating = false; k = clamp(k / 1.15, 0.25, 3); }} title="Zoom out">−</button>
	</div>
</div>

<style>
	.canvas-host {
		position: absolute;
		inset: 0;
		overflow: hidden;
		background: var(--bg);
	}
	.graph {
		width: 100%;
		height: 100%;
		display: block;
		cursor: grab;
		touch-action: none;
	}
	.graph:active {
		cursor: grabbing;
	}
	.scene {
		transition: transform 0ms;
	}
	.scene.animating {
		transition: transform 560ms cubic-bezier(0.22, 1, 0.36, 1);
	}

	.edge {
		stroke: var(--border);
		stroke-width: 1.2px;
	}
	.edge.chain {
		stroke: var(--accent);
		stroke-dasharray: 1 4;
		stroke-linecap: round;
		opacity: 0.7;
	}

	.node {
		cursor: pointer;
		transition: transform 520ms cubic-bezier(0.22, 1, 0.36, 1);
	}
	.node .box {
		fill: var(--card);
		stroke: var(--border);
		stroke-width: 1.2px;
		transition: stroke 0.15s;
	}
	.node .icon {
		fill: none;
		stroke: var(--accent);
		stroke-width: 1.5px;
		stroke-linecap: round;
		stroke-linejoin: round;
	}
	.node .label {
		font: 500 12.5px/1 'Fraunces', Georgia, serif;
		fill: var(--text);
		dominant-baseline: middle;
	}
	.node .sub {
		font: 500 8px/1 ui-sans-serif, system-ui, sans-serif;
		letter-spacing: 0.16em;
		fill: var(--text-2);
		dominant-baseline: middle;
	}
	.node .kind {
		font: 500 8px/1 ui-sans-serif, system-ui, sans-serif;
		letter-spacing: 0.16em;
		fill: var(--text-2);
		text-anchor: middle;
	}
	.node.core .box {
		fill: var(--accent);
		stroke: var(--accent);
	}
	.node.core .icon {
		stroke: #fff;
	}
	.node.core .label {
		fill: #fff;
		font-weight: 600;
		font-size: 16px;
	}
	.node.core .sub {
		fill: rgba(255, 255, 255, 0.7);
	}
	.node.chain .box {
		stroke-dasharray: 3 3;
		stroke-width: 1px;
	}
	.node.chain .label {
		font-size: 10.5px;
		fill: var(--text-2);
	}
	.node.sat:hover .box,
	.node.chain:hover .box {
		stroke: var(--accent);
	}
	.node.focused .box {
		stroke: var(--accent);
		stroke-width: 2px;
		stroke-dasharray: none;
	}
	.node:focus {
		outline: none;
	}
	.node:focus-visible .box {
		stroke: var(--accent);
		stroke-width: 2px;
	}

	.trail {
		position: absolute;
		left: 12px;
		top: 10px;
		max-width: calc(100% - 150px);
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 4px;
		font: italic 12px/1.4 'Fraunces', Georgia, serif;
		color: var(--text-2);
	}
	.trail b {
		font-style: normal;
		font-weight: 600;
		color: var(--accent);
	}
	.trail .sep {
		opacity: 0.6;
	}
	.trail .crumb {
		appearance: none;
		background: transparent;
		border: 0;
		padding: 0;
		cursor: pointer;
		font: italic 12px/1.4 'Fraunces', Georgia, serif;
		color: var(--text-2);
	}
	.trail .crumb:hover {
		color: var(--accent);
	}

	.controls {
		position: absolute;
		right: 12px;
		top: 10px;
		display: flex;
		gap: 4px;
	}
	.controls button {
		width: 24px;
		height: 24px;
		border-radius: 6px;
		border: 1px solid var(--border);
		background: var(--panel);
		color: var(--text-2);
		cursor: pointer;
		font: 13px/1 ui-sans-serif, system-ui, sans-serif;
	}
	.controls button:hover {
		border-color: var(--accent);
		color: var(--accent);
	}
</style>
