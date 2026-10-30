<script lang="ts">
	import { untrack } from 'svelte';
	import { m } from '$lib/paraglide/messages.js';
	import { actionRef } from '../entries';
	import { listQueryAvailability, previewColumns, requestWithTimeout, RequestTimeoutError } from '../list-request';
	import ErrorBox from '../ErrorBox.svelte';
	import { getRuntime } from '../runtime.svelte';
	import type { Json, JsonObject } from '../types';
	import type { ElementProps } from './index';

	let { instanceId, element }: ElementProps = $props();

	const rt = getRuntime();
	const d = $derived(element.desc);
	const columns = $derived(rt.editorPreview ? previewColumns(d.columns ?? [], d.source) : (d.columns ?? []));
	const grid = $derived(
		columns.map((c) => (c.width === 'elastic' || !c.width ? 'minmax(0, 1fr)' : c.width === 'auto' ? 'auto' : c.width)).join(' ')
	);

	let records = $state<JsonObject[]>([]);
	let status = $state<'unconfigured' | 'unsaved' | 'fields' | 'loading' | 'ready' | 'error' | 'timeout'>('unconfigured');
	let failure = $state('');
	const pageSize = 20;
	let page = $state(1);
	const paginated = $derived(d.pagination !== 'no');
	const pageCount = $derived(Math.max(1, Math.ceil(records.length / pageSize)));
	const currentPage = $derived(Math.min(page, pageCount));
	const visibleRecords = $derived(paginated ? records.slice((currentPage - 1) * pageSize, currentPage * pageSize) : records);

	async function query(signal: AbortSignal) {
		const availability = listQueryAvailability(d.source, element.pageId, rt.editorPreview);
		if (availability !== 'ready') {
			records = [];
			status = availability;
			return;
		}
		status = 'loading';
		try {
			const result = await requestWithTimeout((requestSignal) => rt.queryList(instanceId, actionRef(element, 'source'), d.source?.args, requestSignal, d.source), 20_000, signal);
			if (signal.aborted) return;
			records = result;
			page = 1;
			status = 'ready';
		} catch (err) {
			if (signal.aborted) return;
			if (err instanceof RequestTimeoutError) status = 'timeout';
			else { failure = err instanceof Error ? err.message : String(err); status = 'error'; }
		}
	}

	// Refresh for a changed source and when a page configured to refresh is shown again.
	$effect(() => {
		const inst = rt.pages[instanceId];
		const shown = inst?.shown ?? 0;
		const onShow = inst ? !!rt.bundle.pages[inst.pageId]?.refresh?.on_show : false;
		JSON.stringify(d.source);
		element.pageId;
		if (onShow) shown;
		const controller = new AbortController();
		untrack(() => { void query(controller.signal); });
		return () => controller.abort();
	});

	function display(v: Json | undefined): string {
		return v == null ? '' : typeof v === 'object' ? JSON.stringify(v) : String(v);
	}

	function open(record: JsonObject) {
		if (d.on_row_click) void rt.runTrigger(d.on_row_click, actionRef(element, 'on_row_click'), { instanceId, record });
	}
</script>

<div class="flex h-full min-h-0 w-full select-text flex-col text-sm">
	<div class="min-h-0 flex-1 overflow-auto">
	<div class="text-muted-foreground grid gap-x-3 border-b px-2 py-1.5 font-medium" style="grid-template-columns: {grid}">
		{#each columns as c (c.field)}<span class="truncate">{c.label}</span>{/each}
	</div>
	{#if status === 'unconfigured'}
		<p class="text-muted-foreground px-2 py-2">{m.wpi_list_table_unconfigured()}</p>
	{:else if status === 'fields'}
		<p class="text-muted-foreground px-2 py-2">{m.wpi_list_select_fields()}</p>
	{:else if status === 'unsaved'}
		<p class="text-muted-foreground px-2 py-2">{m.wpi_list_requires_saved_page()}</p>
	{:else if status === 'timeout'}
		<p class="text-muted-foreground px-2 py-2" role="alert">{m.wpi_list_timeout()}</p>
	{:else if status === 'loading' && records.length === 0}
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
