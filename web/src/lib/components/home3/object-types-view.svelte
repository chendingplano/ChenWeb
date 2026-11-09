<script lang="ts">
	import { onMount, onDestroy, tick } from 'svelte';
	import { m } from '$lib/paraglide/messages.js';
	import { getLocale } from '$lib/paraglide/runtime';
	import {
		approveObjectType, listObjectTypes, filterObjectTypes, restoreObjectTypesState,
		OBJECT_TYPES_STATE_KEY, type ObjectType, type ObjectTypeFilter
	} from './object-types-client.js';

	let { darkMode = true }: { darkMode?: boolean } = $props();
	let rows = $state<ObjectType[]>([]);
	let filter = $state<ObjectTypeFilter>('proposed');
	let search = $state('');
	let selectedId = $state<number | null>(null);
	let loading = $state(true);
	let approving = $state(false);
	let error = $state('');
	let notice = $state('');
	let scrollTop = 0;
	let listScrollTop = 0;
	let listViewport: HTMLElement;
	let viewport: HTMLDivElement;
	let mounted = false;
	let disposed = false;
	let visibleRows = $derived(filterObjectTypes(rows, filter, search));
	let selected = $derived(rows.find((row) => row.id === selectedId) ?? null);
	let proposedCount = $derived(rows.filter((row) => row.status === 'proposed').length);
	let approvedCount = $derived(rows.length - proposedCount);
	let busy = $derived(loading || approving);

	function saveState() {
		if (!mounted) return;
		try {
			sessionStorage.setItem(OBJECT_TYPES_STATE_KEY, JSON.stringify({ filter, search, selectedId, scrollTop, listScrollTop }));
		} catch { /* The page remains usable when browser storage is unavailable. */ }
	}

	$effect(() => {
		// Persist reactive choices immediately, including while a refresh is running.
		void filter; void search; void selectedId;
		saveState();
	});

	async function load() {
		loading = true;
		error = '';
		try {
			const result = await listObjectTypes();
			if (disposed) return;
			rows = result.rows;
			if (selectedId === null) selectedId = filterObjectTypes(rows, filter, search)[0]?.id ?? null;
			await tick();
			if (viewport) viewport.scrollTop = scrollTop;
			if (listViewport) listViewport.scrollTop = listScrollTop;
		} catch (err) {
			if (!disposed) error = err instanceof Error ? err.message : String(err);
		} finally {
			if (!disposed) loading = false;
		}
	}

	async function approve(row: ObjectType) {
		if (busy || row.status !== 'proposed') return;
		approving = true;
		error = '';
		notice = '';
		try {
			const result = await approveObjectType(row.id);
			if (disposed) return;
			rows = rows.map((item) => item.id === row.id ? result.row : item);
			notice = m.otypes_approved_notice({ name: result.row.name });
		} catch (err) {
			if (!disposed) error = err instanceof Error ? err.message : String(err);
		} finally {
			if (!disposed) approving = false;
		}
	}

	function statusLabel(status: ObjectType['status']) {
		return status === 'approved' ? m.otypes_approved() : m.otypes_proposed();
	}

	function dateLabel(value: string | null) {
		if (!value) return m.otypes_not_recorded();
		const date = new Date(value);
		return Number.isNaN(date.getTime()) ? value : date.toLocaleString(getLocale());
	}

	onMount(() => {
		let saved = restoreObjectTypesState(null);
		try { saved = restoreObjectTypesState(sessionStorage.getItem(OBJECT_TYPES_STATE_KEY)); } catch { /* Storage is optional. */ }
		filter = saved.filter;
		search = saved.search;
		selectedId = saved.selectedId;
		scrollTop = saved.scrollTop;
		listScrollTop = saved.listScrollTop;
		mounted = true;
		void load();
	});

	onDestroy(() => {
		saveState();
		disposed = true;
	});
</script>

<div
	bind:this={viewport}
	class:dark={darkMode}
	class="object-types select-text"
	onscroll={() => { scrollTop = viewport.scrollTop; saveState(); }}
>
	<header>
		<div>
			<p class="eyebrow">{m.otypes_registry()}</p>
			<h1>{m.otypes_title()}</h1>
			<p class="intro">{m.otypes_intro()}</p>
		</div>
		<button class="secondary" onclick={() => { notice = ''; void load(); }} disabled={busy}>
			{loading ? m.otypes_refreshing() : m.otypes_refresh()}
		</button>
	</header>

	<div class="toolbar">
		<div class="filters" role="group" aria-label={m.otypes_filter_label()}>
			{#each ['proposed', 'approved', 'all'] as value}
				<button
					class:active={filter === value}
					aria-pressed={filter === value}
					onclick={() => { filter = value as ObjectTypeFilter; scrollTop = 0; }}
				>
					{value === 'all' ? m.otypes_all() : statusLabel(value as ObjectType['status'])}
					<span class="count">{value === 'proposed' ? proposedCount : value === 'approved' ? approvedCount : rows.length}</span>
				</button>
			{/each}
		</div>
		<label class="search">
			<span class="sr-only">{m.otypes_search()}</span>
			<input type="search" bind:value={search} placeholder={m.otypes_search()} oninput={() => { scrollTop = 0; }} />
		</label>
	</div>

	{#if error}<div class="feedback error" role="alert">{m.otypes_error({ error })}</div>{/if}
	{#if notice}<div class="feedback success" role="status">{notice}</div>{/if}

	<div class="workspace">
		<section bind:this={listViewport} onscroll={() => { listScrollTop = listViewport.scrollTop; saveState(); }} class="registry-list" aria-label={m.otypes_entries()} aria-busy={loading}>
			<div class="list-heading"><span>{m.otypes_type()}</span><span>{m.otypes_status()}</span></div>
			{#if loading && rows.length === 0}
				<p class="empty" role="status">{m.otypes_loading()}</p>
			{:else if !error && visibleRows.length === 0}
				<p class="empty">{rows.length === 0 ? m.otypes_no_entries() : m.otypes_no_matches()}</p>
			{:else}
				{#each visibleRows as row (row.id)}
					<button class="type-row" class:selected={selectedId === row.id} aria-pressed={selectedId === row.id} onclick={() => { selectedId = row.id; notice = ''; }}>
						<span class="type-name">{row.name}<small>{row.raw_value || row.name}</small></span>
						<span class="badge" class:approved={row.status === 'approved'}>{statusLabel(row.status)}</span>
					</button>
				{/each}
			{/if}
		</section>

		<section class="detail" aria-label={m.otypes_details()}>
			{#if selected}
				<div class="detail-heading">
					<div><span class="badge" class:approved={selected.status === 'approved'}>{statusLabel(selected.status)}</span><h2>{selected.name}</h2></div>
					{#if selected.status === 'proposed'}
						<button class="primary" onclick={() => selected && approve(selected)} disabled={busy}>
							{approving ? m.otypes_approving() : m.otypes_approve()}
						</button>
					{/if}
				</div>
				<p class="policy">{selected.status === 'proposed' ? m.otypes_proposed_help() : m.otypes_approved_help()}</p>
				<h3>{m.otypes_provenance()}</h3>
				<dl>
					<div><dt>{m.otypes_raw_value()}</dt><dd><code>{selected.raw_value || m.otypes_not_recorded()}</code></dd></div>
					<div><dt>{m.otypes_record()}</dt><dd>{selected.source_record_id || m.otypes_not_recorded()}</dd></div>
					<div><dt>{m.otypes_source_lines()}</dt><dd>{selected.source_spans?.join(', ') || m.otypes_not_recorded()}</dd></div>
					<div><dt>{m.otypes_processor()}</dt><dd>{selected.source_processor || m.otypes_not_recorded()}</dd></div>
					<div><dt>{m.otypes_run()}</dt><dd>{selected.source_run_id || m.otypes_not_recorded()}</dd></div>
					<div><dt>{m.otypes_model()}</dt><dd>{selected.source_model || m.otypes_not_recorded()}</dd></div>
					<div><dt>{m.otypes_prompt()}</dt><dd>{selected.source_prompt || m.otypes_not_recorded()}</dd></div>
					<div><dt>{m.otypes_created()}</dt><dd>{dateLabel(selected.created_at)}</dd></div>
					{#if selected.status === 'approved'}
						<div><dt>{m.otypes_approved_at()}</dt><dd>{dateLabel(selected.approved_at)}</dd></div>
						<div><dt>{m.otypes_approved_by()}</dt><dd>{selected.approved_by || m.otypes_not_recorded()}</dd></div>
					{/if}
				</dl>
			{:else}
				<div class="detail-empty"><h2>{m.otypes_select_title()}</h2><p>{m.otypes_select_help()}</p></div>
			{/if}
		</section>
	</div>
</div>

<style>
	.object-types { --surface: #fff; --page: #f2f4f7; --ink: #17202e; --muted: #647084; --line: #dce1e8; --accent: #166b65; --tint: #e8f4f0; --warning: #805b12; --warning-bg: #fff4da; flex: 1; min-height: 0; overflow: auto; container-type: inline-size; padding: 28px; background: var(--page); color: var(--ink); }
	.object-types.dark { --surface: #1f2333; --page: #171b26; --ink: #e2e8f0; --muted: #9aa9bc; --line: #343b4c; --accent: #80d4c5; --tint: #243c3d; --warning: #e6bf72; --warning-bg: #3e3423; }
	header { display: flex; justify-content: space-between; align-items: flex-start; gap: 20px; margin-bottom: 25px; }
	.eyebrow { margin: 0 0 7px; color: var(--accent); text-transform: uppercase; letter-spacing: .13em; font-size: 10px; font-weight: 700; }
	h1 { margin: 0; font-size: 25px; font-weight: 650; letter-spacing: -.03em; }
	.intro { max-width: 690px; margin: 9px 0 0; color: var(--muted); font-size: 13px; line-height: 1.6; }
	button, input { font: inherit; }
	button { cursor: pointer; }
	button:disabled { opacity: .55; cursor: wait; }
	button:focus-visible, input:focus-visible { outline: 2px solid var(--accent); outline-offset: 3px; }
	.secondary, .primary { border: 1px solid var(--line); border-radius: 6px; padding: 9px 13px; font-size: 12px; white-space: nowrap; }
	.secondary { color: var(--ink); background: var(--surface); }
	.primary { color: var(--surface); background: var(--accent); border-color: var(--accent); font-weight: 650; }
	.toolbar { display: flex; flex-wrap: wrap; justify-content: space-between; gap: 14px; margin-bottom: 17px; }
	.filters { display: flex; flex-wrap: wrap; gap: 5px; }
	.filters button { border: 1px solid transparent; border-radius: 6px; background: transparent; color: var(--muted); padding: 8px 10px; font-size: 12px; }
	.filters button.active { background: var(--surface); color: var(--ink); border-color: var(--line); }
	.count { display: inline-block; margin-left: 6px; font-variant-numeric: tabular-nums; opacity: .7; }
	.search { min-width: min(290px, 100%); }
	input { box-sizing: border-box; width: 100%; padding: 9px 12px; border: 1px solid var(--line); border-radius: 6px; color: var(--ink); background: var(--surface); font-size: 12px; }
	input::placeholder { color: var(--muted); }
	.workspace { display: grid; grid-template-columns: minmax(200px, .8fr) minmax(260px, 1.25fr); gap: 18px; align-items: start; }
	.registry-list, .detail { background: var(--surface); border: 1px solid var(--line); border-radius: 8px; overflow: hidden; }
	.list-heading { display: flex; justify-content: space-between; padding: 13px 16px; border-bottom: 1px solid var(--line); color: var(--muted); font-size: 10px; text-transform: uppercase; letter-spacing: .06em; }
	.type-row { width: 100%; display: flex; align-items: center; justify-content: space-between; gap: 12px; padding: 13px 16px; border: 0; border-bottom: 1px solid var(--line); text-align: left; color: var(--ink); background: transparent; }
	.type-row:last-child { border-bottom: 0; }
	.type-row:hover { background: var(--page); }
	.type-row.selected { background: var(--tint); box-shadow: inset 3px 0 var(--accent); }
	.type-name { font-size: 13px; font-weight: 600; overflow-wrap: anywhere; }
	.type-name small { display: block; margin-top: 4px; font-size: 11px; font-weight: 400; color: var(--muted); }
	.badge { display: inline-block; flex-shrink: 0; padding: 4px 7px; border-radius: 4px; background: var(--warning-bg); color: var(--warning); font-size: 10px; font-weight: 600; }
	.badge.approved { background: var(--tint); color: var(--accent); }
	.detail { padding: 23px; }
	.detail-heading { display: flex; justify-content: space-between; gap: 18px; align-items: center; }
	h2 { margin: 10px 0 0; font-size: 21px; font-weight: 600; overflow-wrap: anywhere; }
	.policy { padding-bottom: 20px; border-bottom: 1px solid var(--line); font-size: 12px; line-height: 1.6; color: var(--muted); }
	h3 { margin: 22px 0 16px; font-size: 11px; font-weight: 600; text-transform: uppercase; letter-spacing: .08em; color: var(--muted); }
	dl { margin: 0; }
	dl > div { display: grid; grid-template-columns: 115px minmax(0, 1fr); gap: 14px; padding: 10px 0; border-bottom: 1px solid var(--line); font-size: 12px; }
	dl > div:last-child { border-bottom: 0; }
	dt { color: var(--muted); }
	dd { margin: 0; overflow-wrap: anywhere; }
	code { font-size: 11px; }
	.empty, .detail-empty { padding: 35px 20px; color: var(--muted); font-size: 13px; line-height: 1.7; }
	.detail-empty h2 { color: var(--ink); font-size: 18px; }
	.feedback { padding: 11px 14px; margin-bottom: 16px; border: 1px solid var(--line); border-radius: 6px; font-size: 12px; overflow-wrap: anywhere; }
	.error { color: #dc5252; }
	.success { color: var(--accent); background: var(--tint); }
	@container (max-width: 520px) { .workspace { grid-template-columns: 1fr; } .registry-list { max-height: 360px; overflow: auto; } }
	@media (max-width: 900px) { .object-types { padding: 18px; } }
</style>
