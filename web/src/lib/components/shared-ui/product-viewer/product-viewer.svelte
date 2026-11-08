<script lang="ts">
	import type { Snippet } from 'svelte';
	import PanelRightIcon from '@lucide/svelte/icons/panel-right';
	import { m } from '$lib/paraglide/messages.js';
	import ProductsView from '$lib/components/home3/products-view.svelte';
	import ContextShelf from '$lib/components/home3/context-shelf.svelte';
	import ProductDetails from './product-details.svelte';

	let {
		darkMode = true,
		source = 'kb',
		browserInstanceKey = 'products',
		scopeToActiveStore = false,
		onFocusModeChange
	}: {
		darkMode?: boolean;
		/** 'kb': Knowledge Base products; 'testbed': read-only Gold Products over one benchmark run. */
		source?: 'kb' | 'testbed';
		browserInstanceKey?: string;
		scopeToActiveStore?: boolean;
		onFocusModeChange?: (focused: boolean) => void;
	} = $props();

	let borderColor = $derived(darkMode ? '#2D3348' : '#E4E6EB');
	let accent = $derived(darkMode ? '#818CF8' : '#6366F1');
	let accentTint = $derived(darkMode ? 'rgba(129,140,248,0.15)' : 'rgba(99,102,241,0.10)');

	let product = $state<any | null>(null);
	let shelfOpen = $state(false);
	// The shelf and the focus-mode PDF card share one edge: dragging moves the shelf and the PDF by
	// equal and opposite amounts, so the map in between keeps its width.
	let shelfWidth = $state(480);
	let pdfWidth = $state(480);
	let drag: { x: number; shelf: number; pdf: number } | null = null;

	function startDrag(e: MouseEvent) {
		e.preventDefault();
		drag = { x: e.clientX, shelf: shelfWidth, pdf: pdfWidth };
	}
	function onMove(e: MouseEvent) {
		if (!drag) return;
		const { x, shelf, pdf } = drag;
		const nextShelf = Math.max(220, Math.min(680, shelf + x - e.clientX));
		const nextPdf = Math.max(320, Math.min(1460, pdf - (nextShelf - shelf)));
		pdfWidth = nextPdf;
		shelfWidth = shelf + (pdf - nextPdf);
	}
	function handleDetailsChange(next: any | null) {
		product = next;
		shelfOpen = next != null;
	}
</script>

<svelte:window onmousemove={onMove} onmouseup={() => (drag = null)} />

{#snippet details()}
	<ProductDetails {product} {darkMode} />
{/snippet}

<div class="flex h-full min-w-0">
	<div class="flex min-w-0 flex-1 flex-col overflow-hidden">
		{#if product}
			<div class="flex flex-shrink-0 justify-end p-2" style="border-bottom:1px solid {borderColor};">
				<button
					type="button"
					class="flex cursor-pointer items-center gap-1.5 rounded-lg px-2 py-1 text-xs"
					style="background:{accentTint}; color:{accent};"
					onclick={() => (shelfOpen = !shelfOpen)}
					aria-label={shelfOpen ? m.content_panel_close_context_panel() : m.content_panel_open_context_panel()}
				>
					<PanelRightIcon class="h-3.5 w-3.5" />
					{shelfOpen ? m.content_panel_close_panel() : m.content_panel_open_panel()}
				</button>
			</div>
		{/if}
		<div class="min-h-0 flex-1 overflow-hidden">
			<ProductsView
				{darkMode}
				{source}
				{browserInstanceKey}
				{scopeToActiveStore}
				{onFocusModeChange}
				onProductDetailsChange={handleDetailsChange}
				bind:focusPdfWidth={pdfWidth}
			/>
		</div>
	</div>
	{#if product && shelfOpen}
		<div class="h-full shrink-0 [&>aside]:h-full">
			<ContextShelf
				{darkMode}
				productDetails={details as Snippet}
				activeMenu={null}
				width={shelfWidth}
				open={shelfOpen}
				onDragStart={startDrag}
				onClose={() => (shelfOpen = false)}
			/>
		</div>
	{/if}
</div>
