<script lang="ts">
	import { untrack } from 'svelte';
	import { m } from '$lib/paraglide/messages.js';
	import { actionRef } from '../entries';
	import ErrorBox from '../ErrorBox.svelte';
	import { getRuntime } from '../runtime.svelte';
	import type { Json, JsonObject } from '../types';
	import type { ElementProps } from './index';

	let { panelId, element }: ElementProps = $props();

	const rt = getRuntime();
	const d = $derived(element.desc);
	const columns = $derived(d.columns ?? []);
	const grid = $derived(
		columns.map((c) => (c.width === 'elastic' || !c.width ? 'minmax(0, 1fr)' : c.width === 'auto' ? 'auto' : c.width)).join(' ')
	);

	let records = $state<JsonObject[]>([]);
	let status = $state<'loading' | 'ready' | 'error'>('loading');
	let failure = $state('');
	const pageSize = 20;
	let page = $state(1);
	const paginated = $derived(d.pagination !== 'no');
	const pageCount = $derived(Math.max(1, Math.ceil(records.length / pageSize)));
	const currentPage = $derived(Math.min(page, pageCount));
	const visibleRecords = $derived(paginated ? records.slice((currentPage - 1) * pageSize, currentPage * pageSize) : records);

	async function query() {
		if (!d.source) return;
		status = 'loading';
		try {
			records = await rt.queryList(panelId, actionRef(element, 'source'), d.source.args);
			page = 1;
			status = 'ready';
		} catch (err) {
			failure = err instanceof Error ? err.message : String(err);
			status = 'error';
		}
	}

	// Query when first drawn, and again each time the panel is shown if its
	// page refreshes on show.
	let lastShown: number | null = null;
	$effect(() => {
		const panel = rt.panels[panelId];
		const shown = panel?.shown ?? 0;
		const onShow = panel ? !!rt.bundle.pages[panel.pageId].refresh?.on_show : false;
		const first = lastShown === null;
		const reshown = !first && shown !== lastShown && onShow;
		lastShown = shown;
		if (first || reshown) untrack(query);
	});

	function display(v: Json | undefined): string {
		return v == null ? '' : typeof v === 'object' ? JSON.stringify(v) : String(v);
	}

	function open(record: JsonObject) {
		if (d.on_row_click) void rt.runTrigger(d.on_row_click, actionRef(element, 'on_row_click'), { panelId, record });
	}
</script>

<div class="flex h-full min-h-0 w-full select-text flex-col text-sm">
	<div class="min-h-0 flex-1 overflow-auto">
	<div class="text-muted-foreground grid gap-x-3 border-b px-2 py-1.5 font-medium" style="grid-template-columns: {grid}">
		{#each columns as c (c.field)}<span class="truncate">{c.label}</span>{/each}
	</div>
	{#if status === 'loading' && records.length === 0}
		<p class="text-muted-foreground px-2 py-2">{m.wpi_panel_loading()}</p>
	{:else if status === 'error'}
		<div class="py-2"><ErrorBox title={m.wpi_list_failed({ message: failure })} /></div>
	{:else if records.length === 0}
		<p class="text-muted-foreground px-2 py-2">{m.wpi_list_empty()}</p>
	{:else}
		{#each visibleRecords as record, i (i)}
			{#if d.on_row_click}
				<button type="button" class="hover:bg-muted grid w-full gap-x-3 border-b px-2 py-1.5 text-left" style="grid-template-columns: {grid}" onclick={() => open(record)}>
					{#each columns as c (c.field)}<span class="truncate">{display(record[c.field])}</span>{/each}
				</button>
			{:else}
				<div class="grid gap-x-3 border-b px-2 py-1.5" style="grid-template-columns: {grid}">
					{#each columns as c (c.field)}<span class="truncate">{display(record[c.field])}</span>{/each}
				</div>
			{/if}
		{/each}
	{/if}
	</div>
	{#if paginated && status === 'ready' && records.length > 0}
		<nav class="flex shrink-0 flex-wrap items-center justify-end gap-3 border-t px-2 py-2" aria-label={m.wpi_list_pagination()}>
			<button type="button" class="rounded border px-2 py-1 disabled:opacity-50" disabled={currentPage === 1} onclick={() => page = currentPage - 1}>{m.wpi_list_previous()}</button>
			<span aria-live="polite">{m.wpi_list_page({ page: currentPage, pages: pageCount, count: records.length })}</span>
			<button type="button" class="rounded border px-2 py-1 disabled:opacity-50" disabled={currentPage === pageCount} onclick={() => page = currentPage + 1}>{m.wpi_list_next()}</button>
		</nav>
	{/if}
</div>
