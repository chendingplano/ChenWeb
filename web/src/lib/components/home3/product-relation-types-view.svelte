<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { m } from '$lib/paraglide/messages.js';
	import { getLocale } from '$lib/paraglide/runtime';
	import {
		approveProductRelationType, deleteProductRelationType, filterProductRelationTypes,
		listProductRelationTypes, updateProductRelationType, validateRelationTypeInput,
		DEFAULT_RELATION_WEIGHT, type ProductRelationType, type ProductRelationTypeFilter
	} from './product-relation-types-client.js';

	let { darkMode = true }: { darkMode?: boolean } = $props();
	let rows = $state<ProductRelationType[]>([]);
	let filter = $state<ProductRelationTypeFilter>('proposed');
	let search = $state('');
	let selectedCode = $state<string | null>(null);
	let loading = $state(true);
	let saving = $state(false);
	let error = $state('');
	let notice = $state('');
	let formEn = $state('');
	let formZh = $state('');
	let formWeight = $state<string>(String(DEFAULT_RELATION_WEIGHT));
	let disposed = false;
	let visibleRows = $derived(filterProductRelationTypes(rows, filter, search));
	let selected = $derived(rows.find((row) => row.code === selectedCode) ?? null);
	let proposedCount = $derived(rows.filter((row) => row.status === 'proposed').length);
	let approvedCount = $derived(rows.length - proposedCount);
	let busy = $derived(loading || saving);

	// Load the form from the selected row; a proposed row starts at the default weight.
	$effect(() => {
		const row = selected;
		formEn = row?.description_en ?? '';
		formZh = row?.description_zh ?? '';
		formWeight = String(row && row.status === 'approved' ? row.weight : DEFAULT_RELATION_WEIGHT);
	});

	async function load() {
		loading = true;
		error = '';
		try {
			const result = await listProductRelationTypes();
			if (disposed) return;
			rows = result.rows;
			if (selectedCode === null) selectedCode = filterProductRelationTypes(rows, filter, search)[0]?.code ?? null;
		} catch (err) {
			if (!disposed) error = err instanceof Error ? err.message : String(err);
		} finally {
			if (!disposed) loading = false;
		}
	}

	async function save(row: ProductRelationType) {
		if (busy) return;
		const input = { description_en: formEn, description_zh: formZh, weight: Number(formWeight) };
		const problem = validateRelationTypeInput(input);
		if (problem) {
			error = problem === 'descriptions' ? m.prt_validation_descriptions() : m.prt_validation_weight();
			return;
		}
		saving = true;
		error = '';
		notice = '';
		try {
			const approving = row.status === 'proposed';
			const result = approving
				? await approveProductRelationType(row.code, input)
				: await updateProductRelationType(row.code, input);
			if (disposed) return;
			rows = rows.map((item) => (item.code === row.code ? result.row : item));
			notice = approving ? m.prt_approved_notice({ code: row.code }) : m.prt_saved_notice({ code: row.code });
		} catch (err) {
			if (!disposed) error = err instanceof Error ? err.message : String(err);
		} finally {
			if (!disposed) saving = false;
		}
	}

	async function remove(row: ProductRelationType) {
		if (busy || row.status !== 'proposed' || row.product_count > 0) return;
		if (!confirm(m.prt_delete_confirm({ code: row.code }))) return;
		saving = true;
		error = '';
		notice = '';
		try {
			await deleteProductRelationType(row.code);
			if (disposed) return;
			rows = rows.filter((item) => item.code !== row.code);
			selectedCode = filterProductRelationTypes(rows, filter, search)[0]?.code ?? null;
			notice = m.prt_deleted_notice({ code: row.code });
		} catch (err) {
			if (!disposed) error = err instanceof Error ? err.message : String(err);
		} finally {
			if (!disposed) saving = false;
		}
	}

	function statusLabel(status: ProductRelationType['status']) {
		return status === 'approved' ? m.prt_approved() : m.prt_proposed();
	}

	function dateLabel(value: string | null) {
		if (!value) return m.prt_not_recorded();
		const date = new Date(value);
		return Number.isNaN(date.getTime()) ? value : date.toLocaleString(getLocale());
	}

	function describe(row: ProductRelationType) {
		const text = getLocale() === 'zh-cn' ? row.description_zh : row.description_en;
		return text || row.value;
	}

	onMount(() => { void load(); });
	onDestroy(() => { disposed = true; });
</script>

<div class:dark={darkMode} class="relation-types select-text">
	<header>
		<div>
			<p class="eyebrow">{m.prt_registry()}</p>
			<h1>{m.prt_title()}</h1>
			<p class="intro">{m.prt_intro()}</p>
		</div>
		<button class="secondary" onclick={() => { notice = ''; void load(); }} disabled={busy}>
			{loading ? m.prt_refreshing() : m.prt_refresh()}
		</button>
	</header>

	<div class="toolbar">
		<div class="filters" role="group" aria-label={m.prt_filter_label()}>
			{#each ['proposed', 'approved', 'all'] as value}
				<button class:active={filter === value} aria-pressed={filter === value} onclick={() => { filter = value as ProductRelationTypeFilter; }}>
					{value === 'all' ? m.prt_all() : statusLabel(value as ProductRelationType['status'])}
					<span class="count">{value === 'proposed' ? proposedCount : value === 'approved' ? approvedCount : rows.length}</span>
				</button>
			{/each}
		</div>
		<label class="search">
			<span class="sr-only">{m.prt_search()}</span>
			<input type="search" bind:value={search} placeholder={m.prt_search()} />
		</label>
	</div>

	{#if error}<div class="feedback error" role="alert">{m.prt_error({ error })}</div>{/if}
	{#if notice}<div class="feedback success" role="status">{notice}</div>{/if}

	<div class="workspace">
		<section class="registry-list" aria-label={m.prt_entries()} aria-busy={loading}>
			<div class="list-heading"><span>{m.prt_code()}</span><span>{m.prt_status()}</span></div>
			{#if loading && rows.length === 0}
				<p class="empty" role="status">{m.prt_loading()}</p>
			{:else if !error && visibleRows.length === 0}
				<p class="empty">{rows.length === 0 ? m.prt_no_entries() : m.prt_no_matches()}</p>
			{:else}
				{#each visibleRows as row (row.code)}
					<button class="type-row" class:selected={selectedCode === row.code} aria-pressed={selectedCode === row.code} onclick={() => { selectedCode = row.code; notice = ''; error = ''; }}>
						<span class="type-name">{row.code}<small>{describe(row)}</small></span>
						<span class="badge" class:approved={row.status === 'approved'}>{statusLabel(row.status)}</span>
					</button>
				{/each}
			{/if}
		</section>

		<section class="detail" aria-label={m.prt_details()}>
			{#if selected}
				<div class="detail-heading">
					<div><span class="badge" class:approved={selected.status === 'approved'}>{statusLabel(selected.status)}</span><h2>{selected.code}</h2></div>
				</div>
				<p class="policy">{selected.status === 'proposed' ? m.prt_proposed_help() : m.prt_approved_help()}</p>

				<div class="form">
					<label>{m.prt_description_en()}<textarea rows="3" bind:value={formEn}></textarea></label>
					<label>{m.prt_description_zh()}<textarea rows="3" bind:value={formZh}></textarea></label>
					<label>{m.prt_weight()}
						<input class="weight-input" type="number" min="0" max="1" step="0.05" bind:value={formWeight} />
						<small>{m.prt_weight_help()}</small>
					</label>
					<div class="actions">
						<button class="primary" onclick={() => selected && save(selected)} disabled={busy}>
							{#if selected.status === 'proposed'}
								{saving ? m.prt_approving() : m.prt_approve()}
							{:else}
								{saving ? m.prt_saving() : m.prt_save()}
							{/if}
						</button>
						{#if selected.status === 'proposed'}
							<button class="danger" onclick={() => selected && remove(selected)} disabled={busy || selected.product_count > 0} title={selected.product_count > 0 ? m.prt_delete_in_use() : undefined}>
								{m.prt_delete()}
							</button>
						{/if}
					</div>
				</div>

				<h3>{m.prt_provenance()}</h3>
				<dl>
					<div><dt>{m.prt_stored_value()}</dt><dd><code>{selected.value}</code></dd></div>
					<div><dt>{m.prt_seen_count()}</dt><dd>{selected.seen_count}</dd></div>
					<div><dt>{m.prt_product_count()}</dt><dd>{selected.product_count}</dd></div>
					<div><dt>{m.prt_first_record()}</dt><dd>{selected.first_record_id ?? m.prt_not_recorded()}</dd></div>
					<div><dt>{m.prt_first_product()}</dt><dd>{selected.first_product_ref || m.prt_not_recorded()}</dd></div>
					<div><dt>{m.prt_created()}</dt><dd>{dateLabel(selected.created_at)}</dd></div>
					{#if selected.status === 'approved'}
						<div><dt>{m.prt_approved_at()}</dt><dd>{dateLabel(selected.approved_at)}</dd></div>
						<div><dt>{m.prt_approved_by()}</dt><dd>{selected.approved_by || m.prt_not_recorded()}</dd></div>
					{/if}
				</dl>
			{:else}
				<div class="detail-empty"><h2>{m.prt_select_title()}</h2><p>{m.prt_select_help()}</p></div>
			{/if}
		</section>
	</div>
</div>

<style>
	.relation-types { --surface: #fff; --page: #f2f4f7; --ink: #17202e; --muted: #647084; --line: #dce1e8; --accent: #166b65; --tint: #e8f4f0; --warning: #805b12; --warning-bg: #fff4da; flex: 1; min-height: 0; overflow: auto; container-type: inline-size; padding: 28px; background: var(--page); color: var(--ink); }
	.relation-types.dark { --surface: #1f2333; --page: #171b26; --ink: #e2e8f0; --muted: #9aa9bc; --line: #343b4c; --accent: #80d4c5; --tint: #243c3d; --warning: #e6bf72; --warning-bg: #3e3423; }
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
	textarea { box-sizing: border-box; width: 100%; padding: 9px 12px; border: 1px solid var(--line); border-radius: 6px; color: var(--ink); background: var(--surface); font: inherit; font-size: 12px; resize: vertical; }
	textarea:focus-visible { outline: 2px solid var(--accent); outline-offset: 3px; }
	.form { display: grid; gap: 14px; margin: 18px 0 4px; }
	.form label { display: grid; gap: 6px; font-size: 12px; color: var(--muted); }
	.form small { color: var(--muted); font-size: 11px; line-height: 1.5; }
	.weight-input { max-width: 120px; }
	.actions { display: flex; gap: 10px; flex-wrap: wrap; }
	.danger { border: 1px solid var(--line); border-radius: 6px; padding: 9px 13px; font-size: 12px; color: #dc5252; background: var(--surface); }
	@container (max-width: 520px) { .workspace { grid-template-columns: 1fr; } .registry-list { max-height: 360px; overflow: auto; } }
	@media (max-width: 900px) { .relation-types { padding: 18px; } }
</style>
