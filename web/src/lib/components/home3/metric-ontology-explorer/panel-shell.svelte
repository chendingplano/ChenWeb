<script lang="ts">
	import { browser } from '$app/environment';
	import type { Snippet } from 'svelte';
	import type { ExplorerTokens } from './theme';

	type Arrangement = 'lr' | 'tb';
	type Layout = { arrangement: Arrangement; leftW: number; canvasH: number; tabsW: number };

	let {
		tokens,
		canvas,
		content,
		source,
		onlayout
	}: {
		tokens: ExplorerTokens;
		canvas: Snippet;
		content: Snippet;
		source: Snippet;
		onlayout?: () => void;
	} = $props();

	const KEY = 'moe-explorer-layout';
	const DEFAULTS: Layout = { arrangement: 'tb', leftW: 500, canvasH: 372, tabsW: 440 };

	function load(): Layout {
		if (!browser) return { ...DEFAULTS };
		try {
			const raw = localStorage.getItem(KEY);
			if (raw) {
				const p = JSON.parse(raw) as Partial<Layout>;
				if (p && (p.arrangement === 'lr' || p.arrangement === 'tb')) return { ...DEFAULTS, ...p };
			}
		} catch {
			/* private mode / blocked / malformed — fall through to defaults */
		}
		return { ...DEFAULTS };
	}

	let layout = $state<Layout>(load());
	let wsEl: HTMLDivElement | null = $state(null);

	function persist() {
		if (!browser) return;
		try {
			localStorage.setItem(KEY, JSON.stringify(layout));
		} catch {
			/* ignore */
		}
	}

	const clamp = (v: number, lo: number, hi: number) => Math.max(lo, Math.min(hi, v));

	type Key = 'leftW' | 'canvasH' | 'tabsW';
	let drag: { key: Key; startX: number; startY: number; startVal: number } | null = null;

	function startResize(e: PointerEvent, key: Key) {
		(e.currentTarget as HTMLElement).setPointerCapture(e.pointerId);
		drag = { key, startX: e.clientX, startY: e.clientY, startVal: layout[key] };
		document.body.style.userSelect = 'none';
	}
	function moveResize(e: PointerEvent) {
		if (!drag || !wsEl) return;
		const W = wsEl.clientWidth,
			H = wsEl.clientHeight;
		if (drag.key === 'leftW') {
			layout.leftW = clamp(drag.startVal + (e.clientX - drag.startX), 320, Math.max(340, W - 360));
		} else if (drag.key === 'tabsW') {
			layout.tabsW = clamp(drag.startVal + (e.clientX - drag.startX), 300, Math.max(320, W - 360));
		} else {
			const colH = layout.arrangement === 'lr' ? (wsEl.querySelector('.col-left')?.clientHeight ?? H) : H;
			layout.canvasH = clamp(drag.startVal + (e.clientY - drag.startY), 200, Math.max(220, colH - 180));
		}
		onlayout?.();
	}
	function endResize(e: PointerEvent) {
		if (!drag) return;
		try {
			(e.currentTarget as HTMLElement).releasePointerCapture(e.pointerId);
		} catch {
			/* ignore */
		}
		drag = null;
		document.body.style.userSelect = '';
		persist();
		onlayout?.();
	}

	function toggleArrangement() {
		layout.arrangement = layout.arrangement === 'lr' ? 'tb' : 'lr';
		persist();
		onlayout?.();
	}
	function resetSizes() {
		layout.leftW = DEFAULTS.leftW;
		layout.canvasH = DEFAULTS.canvasH;
		layout.tabsW = DEFAULTS.tabsW;
		persist();
		onlayout?.();
	}
</script>

<div class="shell" style="--bg:{tokens.pageBg}; --panel:{tokens.panelBg}; --border:{tokens.border}; --border-strong:{tokens.borderStrong}; --text:{tokens.textPrimary}; --text-2:{tokens.textSecondary}; --accent:{tokens.accent}; --hover:{tokens.hoverBg};">
	<div class="toolbar">
		<div class="seg" role="group" aria-label="Panel arrangement">
			<button class:on={layout.arrangement === 'lr'} onclick={() => layout.arrangement !== 'lr' && toggleArrangement()} title="Left / Right">
				<svg viewBox="0 0 16 16" aria-hidden="true"><rect x="1" y="1" width="6" height="14" rx="1" fill="none" stroke="currentColor" /><rect x="9" y="1" width="6" height="6" rx="1" fill="none" stroke="currentColor" /><rect x="9" y="9" width="6" height="6" rx="1" fill="none" stroke="currentColor" /></svg>
				L·R
			</button>
			<button class:on={layout.arrangement === 'tb'} onclick={() => layout.arrangement !== 'tb' && toggleArrangement()} title="Top / Bottom">
				<svg viewBox="0 0 16 16" aria-hidden="true"><rect x="1" y="1" width="14" height="6" rx="1" fill="none" stroke="currentColor" /><rect x="1" y="9" width="6" height="6" rx="1" fill="none" stroke="currentColor" /><rect x="9" y="9" width="6" height="6" rx="1" fill="none" stroke="currentColor" /></svg>
				T·B
			</button>
		</div>
		<button class="link" onclick={resetSizes}>Reset layout</button>
	</div>

	<div class="ws {layout.arrangement}" bind:this={wsEl}>
		{#if layout.arrangement === 'lr'}
			<div class="col-left" style="width:{layout.leftW}px; flex:0 0 {layout.leftW}px;">
				<section class="pane" style="height:{layout.canvasH}px; flex:0 0 {layout.canvasH}px;">{@render canvas()}</section>
				<div class="splitter y" role="separator" aria-orientation="horizontal" onpointerdown={(e) => startResize(e, 'canvasH')} onpointermove={moveResize} onpointerup={endResize} onpointercancel={endResize}></div>
				<section class="pane grow">{@render content()}</section>
			</div>
			<div class="splitter x" role="separator" aria-orientation="vertical" onpointerdown={(e) => startResize(e, 'leftW')} onpointermove={moveResize} onpointerup={endResize} onpointercancel={endResize}></div>
			<section class="pane grow">{@render source()}</section>
		{:else}
			<section class="pane" style="height:{layout.canvasH}px; flex:0 0 {layout.canvasH}px;">{@render canvas()}</section>
			<div class="splitter y" role="separator" aria-orientation="horizontal" onpointerdown={(e) => startResize(e, 'canvasH')} onpointermove={moveResize} onpointerup={endResize} onpointercancel={endResize}></div>
			<div class="row-bottom">
				<section class="pane" style="width:{layout.tabsW}px; flex:0 0 {layout.tabsW}px;">{@render content()}</section>
				<div class="splitter x" role="separator" aria-orientation="vertical" onpointerdown={(e) => startResize(e, 'tabsW')} onpointermove={moveResize} onpointerup={endResize} onpointercancel={endResize}></div>
				<section class="pane grow">{@render source()}</section>
			</div>
		{/if}
	</div>
</div>

<style>
	.shell {
		display: flex;
		flex-direction: column;
		height: 100%;
		min-height: 0;
		background: var(--bg);
		color: var(--text);
	}
	.toolbar {
		display: flex;
		align-items: center;
		gap: 12px;
		padding: 6px 12px;
		border-bottom: 1px solid var(--border);
		flex: 0 0 auto;
	}
	.seg {
		display: inline-flex;
		border: 1px solid var(--border-strong);
		border-radius: 7px;
		overflow: hidden;
	}
	.seg button {
		appearance: none;
		border: 0;
		background: transparent;
		color: var(--text-2);
		font: 600 11px/1 ui-sans-serif, system-ui, sans-serif;
		letter-spacing: 0.04em;
		padding: 6px 10px;
		cursor: pointer;
		display: inline-flex;
		align-items: center;
		gap: 6px;
	}
	.seg button + button {
		border-left: 1px solid var(--border);
	}
	.seg button svg {
		width: 13px;
		height: 13px;
	}
	.seg button.on {
		background: var(--accent);
		color: #fff;
	}
	.seg button:not(.on):hover {
		background: var(--hover);
		color: var(--text);
	}
	.link {
		appearance: none;
		background: transparent;
		border: 0;
		color: var(--text-2);
		font: 600 11px/1 ui-sans-serif, system-ui, sans-serif;
		letter-spacing: 0.04em;
		cursor: pointer;
		padding: 4px 2px;
	}
	.link:hover {
		color: var(--accent);
	}

	.ws {
		flex: 1 1 auto;
		display: flex;
		min-height: 0;
		min-width: 0;
	}
	.ws.tb {
		flex-direction: column;
	}
	.col-left {
		display: flex;
		flex-direction: column;
		min-width: 0;
		min-height: 0;
	}
	.row-bottom {
		flex: 1 1 auto;
		display: flex;
		min-height: 0;
		min-width: 0;
	}
	.pane {
		position: relative;
		min-width: 0;
		min-height: 0;
		overflow: hidden;
		background: var(--panel);
		display: flex;
		flex-direction: column;
	}
	.pane.grow {
		flex: 1 1 auto;
	}

	.splitter {
		flex: 0 0 8px;
		position: relative;
		z-index: 3;
		background: var(--bg);
		touch-action: none;
	}
	.splitter::after {
		content: '';
		position: absolute;
		background: var(--border);
		transition: background 0.12s;
	}
	.splitter.x {
		cursor: col-resize;
	}
	.splitter.x::after {
		top: 0;
		bottom: 0;
		left: 50%;
		width: 1px;
		transform: translateX(-50%);
	}
	.splitter.y {
		cursor: row-resize;
	}
	.splitter.y::after {
		left: 0;
		right: 0;
		top: 50%;
		height: 1px;
		transform: translateY(-50%);
	}
	.splitter:hover::after {
		background: var(--accent);
	}
</style>
