<script lang="ts">
	import XIcon from '@lucide/svelte/icons/x';
	import ChevronLeftIcon from '@lucide/svelte/icons/chevron-left';
	import { buildMatchedUnitRows, matchedUnitFocusTarget, matchedUnitLabel } from './doc-review-json-dialog.js';

	let {
		title,
		units,
		initialSelected,
		dark = true,
		onclose,
		onFocusUnit
	}: {
		title: string;
		units: unknown[];
		initialSelected?: number;
		dark?: boolean;
		onclose: () => void;
		onFocusUnit?: (recordId: number, lineNumbers: number[]) => void;
	} = $props();

	let cardBg = $derived(dark ? '#1F2333' : '#FFFFFF');
	let surface2 = $derived(dark ? '#252A3A' : '#ECEEF2');
	let borderColor = $derived(dark ? '#2D3348' : '#E4E6EB');
	let accent = $derived(dark ? '#818CF8' : '#6366F1');
	let textPrimary = $derived(dark ? '#E2E8F0' : '#111827');
	let textSecondary = $derived(dark ? '#94A3B8' : '#6B7280');
	let textMuted = $derived(dark ? '#64748B' : '#9CA3AF');
	let scrollThumb = $derived(dark ? '#2A3140' : '#D7CFB8');

	let selected = $state<number | null>(initialSelected ?? null);
	let selectedRows = $derived(selected != null ? buildMatchedUnitRows(units[selected]) : []);
	let dialogTitle = $derived(selected != null ? `Matched — ${matchedUnitLabel(units[selected], selected)}` : title);

	function selectUnit(i: number) {
		selected = i;
		const target = matchedUnitFocusTarget(units[i]);
		console.info('[matched-units-dialog] unit selected', {
			index: i,
			label: matchedUnitLabel(units[i], i),
			focusTarget: target
		});
		if (target) onFocusUnit?.(target.recordId, target.lineNumbers);
	}

	let dialogEl = $state<HTMLDivElement | null>(null);
	let closeButton = $state<HTMLButtonElement | null>(null);
	let returnFocusElement: HTMLElement | null = null;

	// Non-modal floating window: only Escape is handled (back to list, then
	// close); no focus trap, so the PDF panel behind stays interactive.
	function handleKeydown(event: KeyboardEvent) {
		if (event.key === 'Escape') {
			event.preventDefault();
			if (selected != null) selected = null;
			else onclose();
		}
	}

	// --- Dragging (via the title bar) --------------------------------------
	// Position/size are written imperatively on the element (not through the
	// reactive style attribute) so they don't fight the browser-managed inline
	// width/height that CSS `resize: both` sets while the user resizes.
	let dragging = false;
	let dragOffset = { x: 0, y: 0 };

	function startDrag(e: PointerEvent) {
		if (!dialogEl) return;
		// Clicks on header buttons (back/close) are not drag starts.
		if ((e.target as HTMLElement).closest('button')) return;
		dragging = true;
		const r = dialogEl.getBoundingClientRect();
		dragOffset = { x: e.clientX - r.left, y: e.clientY - r.top };
		// Switch from the initial centered position to explicit px coords.
		dialogEl.style.left = `${r.left}px`;
		dialogEl.style.top = `${r.top}px`;
		dialogEl.style.transform = 'none';
		(e.currentTarget as HTMLElement).setPointerCapture?.(e.pointerId);
	}
	function onDrag(e: PointerEvent) {
		if (!dragging || !dialogEl) return;
		dialogEl.style.left = `${e.clientX - dragOffset.x}px`;
		dialogEl.style.top = `${e.clientY - dragOffset.y}px`;
	}
	function endDrag(e: PointerEvent) {
		if (!dragging) return;
		dragging = false;
		(e.currentTarget as HTMLElement).releasePointerCapture?.(e.pointerId);
	}

	$effect(() => {
		returnFocusElement = document.activeElement instanceof HTMLElement ? document.activeElement : null;
		requestAnimationFrame(() => closeButton?.focus());
		return () => {
			returnFocusElement?.focus();
		};
	});
</script>

<div
	bind:this={dialogEl}
	tabindex="-1"
	class="mu-dialog rounded-xl flex flex-col"
	role="dialog"
	aria-label={dialogTitle}
	onkeydown={handleKeydown}
	style="background:{cardBg};border:1px solid {borderColor};left:50%;top:8vh;transform:translateX(-50%)"
>
	<!-- svelte-ignore a11y_no_static_element_interactions -->
	<div
		class="mu-titlebar flex justify-between items-center px-5 py-4"
		style="border-bottom:1px solid {borderColor}"
		onpointerdown={startDrag}
		onpointermove={onDrag}
		onpointerup={endDrag}
		onpointercancel={endDrag}
	>
		<div class="flex items-center gap-2 min-w-0">
			{#if selected != null}
				<button
					type="button"
					onclick={() => (selected = null)}
					class="rounded p-1 cursor-pointer"
					style="background:{surface2};color:{textMuted};border:1px solid {borderColor}"
					aria-label="Back to list"
				>
					<ChevronLeftIcon class="w-3.5 h-3.5" />
				</button>
			{/if}
			<span class="truncate" style="font-size:14px;font-weight:600;color:{textPrimary};font-family:monospace">{dialogTitle}</span>
		</div>
		<button
			bind:this={closeButton}
			onclick={onclose}
			class="rounded p-1.5 cursor-pointer"
			style="background:{surface2};color:{textMuted};border:1px solid {borderColor}"
			aria-label="Close"
		>
			<XIcon class="w-4 h-4" />
		</button>
	</div>
	<div class="flex-1 overflow-auto p-5 modal-scroll" style="--modal-scroll-thumb:{scrollThumb};">
		{#if !units.length}
			<div class="text-center" style="color:{textMuted};padding:2rem">No data.</div>
		{:else if selected == null}
			<ul class="mu-list">
				{#each units as unit, i (i)}
					<li>
						<button
							type="button"
							class="mu-item"
							style="border:1px solid {borderColor};color:{textSecondary}"
							onclick={() => selectUnit(i)}
						>
							<span style="color:{accent};font-family:monospace">{matchedUnitLabel(unit, i)}</span>
						</button>
					</li>
				{/each}
			</ul>
		{:else}
			<div class="rounded-lg p-4 text-xs" style="background:{surface2};border:1px solid {borderColor};line-height:1.6;user-select:text">
				<div class="grid gap-x-3 gap-y-2" style="grid-template-columns:minmax(90px,max-content) minmax(0,1fr)">
					{#each selectedRows as row, i (i)}
						<div style="color:{textMuted};font-family:monospace;word-break:break-word">{row.label}</div>
						{#if row.sourceContext}
							<div class="sc-table" style="border:1px solid {borderColor};">
								<div class="sc-head" style="color:{textMuted};border-bottom:1px solid {borderColor};">
									<span>Line</span>
									<span>Content</span>
								</div>
								{#each row.sourceContext as span, j (j)}
									<div class="sc-row" style="color:{textSecondary};{j > 0 ? `border-top:1px solid ${borderColor};` : ''}">
										<span style="color:{textMuted};font-family:monospace">{span.lineNumber}</span>
										<span style="white-space:pre-wrap;word-break:break-word">{span.content}</span>
									</div>
								{/each}
							</div>
						{:else}
							<div style="color:{textSecondary};word-break:break-word;white-space:pre-wrap">{row.value}</div>
						{/if}
					{/each}
				</div>
			</div>
		{/if}
	</div>
</div>

<style>
	/* Floating, non-modal window: no backdrop, draggable by its title bar,
	   resizable from the bottom-right corner (native CSS resize). */
	.mu-dialog {
		position: fixed;
		z-index: 50;
		width: min(900px, 90vw);
		max-height: 84vh;
		min-width: 360px;
		min-height: 200px;
		resize: both;
		overflow: hidden;
		box-shadow: 0 12px 40px rgba(0, 0, 0, 0.45);
	}
	.mu-titlebar {
		cursor: move;
		touch-action: none;
		user-select: none;
		flex: 0 0 auto;
	}
	.modal-scroll {
		scrollbar-width: thin;
		scrollbar-color: var(--modal-scroll-thumb) transparent;
	}
	.modal-scroll::-webkit-scrollbar {
		width: 6px;
	}
	.modal-scroll::-webkit-scrollbar-thumb {
		background: var(--modal-scroll-thumb);
		border-radius: 999px;
	}
	.modal-scroll::-webkit-scrollbar-track {
		background: transparent;
	}
	.mu-list {
		list-style: none;
		margin: 0;
		padding: 0;
	}
	.mu-item {
		display: block;
		width: 100%;
		text-align: left;
		background: transparent;
		border-radius: 6px;
		padding: 0.5rem 0.75rem;
		margin: 0.3rem 0;
		cursor: pointer;
		font-size: 0.8rem;
	}
	.sc-table {
		border-radius: 6px;
		overflow: hidden;
	}
	.sc-head,
	.sc-row {
		display: grid;
		grid-template-columns: minmax(40px, max-content) minmax(0, 1fr);
		gap: 0.6rem;
		padding: 0.35rem 0.55rem;
	}
	.sc-head {
		font-size: 0.7rem;
		text-transform: uppercase;
		letter-spacing: 0.04em;
	}
</style>
