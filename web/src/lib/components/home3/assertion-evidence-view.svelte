<script lang="ts">
	import { m } from '$lib/paraglide/messages.js';
	import { onMount } from 'svelte';
	import RefreshCwIcon from '@lucide/svelte/icons/refresh-cw';
	import PlusIcon from '@lucide/svelte/icons/plus';
	import {
		listEvidence,
		createEvidence,
		deleteEvidence,
		restoreEvidence,
		type Evidence,
		type EvidenceFilters,
		type EvidenceSortKey
	} from './assertion-evidence-client';
	import { detailRows } from './assertion-evidence-detail';

	let { darkMode = true }: { darkMode: boolean } = $props();
	let bg = $derived(darkMode ? '#171B26' : '#F2F4F7'),
		card = $derived(darkMode ? '#1F2333' : '#FFFFFF'),
		surface = $derived(darkMode ? '#252A3A' : '#ECEEF2'),
		border = $derived(darkMode ? '#2D3348' : '#E4E6EB'),
		accent = $derived(darkMode ? '#818CF8' : '#6366F1'),
		text = $derived(darkMode ? '#E2E8F0' : '#111827'),
		muted = $derived(darkMode ? '#94A3B8' : '#6B7280'),
		danger = $derived(darkMode ? '#F87171' : '#DC2626');

	let rows = $state<Evidence[]>([]),
		total = $state(0),
		page = $state(1),
		pageSize = $state(50),
		loading = $state(false),
		saving = $state(false),
		error = $state(''),
		info = $state(''),
		detailsRecord = $state<Evidence | null>(null),
		showCreate = $state(false),
		reason = $state(''),
		sortBy = $state<EvidenceSortKey | ''>('created'),
		sortDir = $state<'asc' | 'desc'>('desc');
	let filters = $state<EvidenceFilters>({
		assertion_id: '',
		input_record_id: '',
		artifact_type: '',
		artifact_id: '',
		evidence_role: '',
		actor_kind: '',
		include_deleted: false
	});
	let form = $state({
		assertion_id: '',
		input_record_id: '',
		artifact_type: '',
		artifact_id: '',
		artifact_object_id: '',
		evidence_quote: '',
		source_line_spans: '',
		evidence_role: 'supports',
		actor_kind: 'processor',
		confidence: '',
		extraction_run: '',
		model: '',
		prompt_version: ''
	});
	const pageSizeOptions = [25, 50, 100, 200];
	const sortableHeaders: { key: EvidenceSortKey; label: string }[] = [
		{ key: 'assertion', label: m.assertion_evidence_assertion_id() },
		{ key: 'input_record', label: m.assertion_evidence_input_record_id() },
		{ key: 'artifact', label: m.assertion_evidence_artifact_id() },
		{ key: 'role', label: m.assertion_evidence_evidence_role() },
		{ key: 'confidence', label: m.assertion_evidence_confidence() },
		{ key: 'created', label: m.assertion_evidence_created() }
	];

	async function load() {
		loading = true;
		error = '';
		try {
			const result = await listEvidence(filters, page, pageSize, sortBy || undefined, sortDir);
			rows = result.results;
			total = result.total;
		} catch (e) {
			error = e instanceof Error ? e.message : String(e);
		} finally {
			loading = false;
		}
	}

	function apply() {
		page = 1;
		load();
	}

	function changePageSize(value: string) {
		pageSize = Number(value);
		page = 1;
		load();
	}

	function toggleSort(key: EvidenceSortKey) {
		if (sortBy === key) sortDir = sortDir === 'asc' ? 'desc' : 'asc';
		else {
			sortBy = key;
			sortDir = 'asc';
		}
		page = 1;
		load();
	}

	function clear() {
		filters.assertion_id = '';
		filters.input_record_id = '';
		filters.artifact_type = '';
		filters.artifact_id = '';
		filters.evidence_role = '';
		filters.actor_kind = '';
		filters.include_deleted = false;
		apply();
	}

	function time(value: string) {
		const date = new Date(value);
		return Number.isNaN(date.getTime()) ? value : date.toLocaleString();
	}

	function json(value: unknown) {
		try {
			return JSON.stringify(value, null, 2);
		} catch {
			return String(value);
		}
	}

	function tableValue(value: unknown) {
		if (value === null || value === undefined || value === '') return '—';
		return typeof value === 'object' ? json(value) : String(value);
	}

	async function create() {
		saving = true;
		error = '';
		try {
			const sourceLineSpans = form.source_line_spans.trim()
				? JSON.parse(form.source_line_spans)
				: undefined;
			await createEvidence({
				...form,
				assertion_id: Number(form.assertion_id),
				input_record_id: form.input_record_id ? Number(form.input_record_id) : undefined,
				artifact_object_id: form.artifact_object_id.trim() || undefined,
				source_line_spans: sourceLineSpans,
				confidence: form.confidence ? Number(form.confidence) : undefined
			});
			showCreate = false;
			info = m.assertion_evidence_evidence_created();
			await load();
		} catch (e) {
			error = e instanceof Error ? e.message : String(e);
		} finally {
			saving = false;
		}
	}

	function openDetails(row: Evidence) {
		detailsRecord = row;
		reason = '';
	}

	async function remove() {
		if (!detailsRecord) return;
		saving = true;
		try {
			detailsRecord = await deleteEvidence(detailsRecord.id, { reason });
			info = m.assertion_evidence_evidence_soft_deleted();
			await load();
		} catch (e) {
			error = e instanceof Error ? e.message : String(e);
		} finally {
			saving = false;
		}
	}

	async function restore() {
		if (!detailsRecord) return;
		saving = true;
		try {
			detailsRecord = await restoreEvidence(detailsRecord.id);
			info = m.assertion_evidence_evidence_restored();
			await load();
		} catch (e) {
			error = e instanceof Error ? e.message : String(e);
		} finally {
			saving = false;
		}
	}

	onMount(load);
</script>

<div class="h-full space-y-4 overflow-auto p-6" style="background:{bg}">
	<div class="rounded-xl p-5" style="background:{card};border:1px solid {border}">
		<div class="flex flex-wrap items-start justify-between gap-3">
			<div>
				<h2 style="font-size:18px;font-weight:600;color:{text}">
					{m.assertion_evidence_assertion_evidence()}
				</h2>
				<p style="font-size:13px;color:{muted};margin-top:2px">
					{m.assertion_evidence_lifecycle_safe_administration_of()}
					<code style="color:{accent}">kb.assertion_evidence</code>.
				</p>
			</div>
			<div class="flex gap-2">
				<button
					onclick={() => (showCreate = true)}
					class="cursor-pointer rounded-lg px-3 py-2 text-sm"
					style="background:{accent};color:white"
					><PlusIcon class="inline h-4 w-4" /> {m.assertion_evidence_new_evidence()}</button
				><button
					onclick={load}
					disabled={loading}
					class="cursor-pointer rounded-lg px-3 py-2 text-sm"
					style="background:{surface};color:{text};border:1px solid {border}"
					><RefreshCwIcon class="inline h-4 w-4 {loading ? 'animate-spin' : ''}" />
					{m.assertion_evidence_refresh()}</button
				>
			</div>
		</div>
	</div>

	<div class="rounded-xl p-5" style="background:{card};border:1px solid {border}">
		<div class="grid gap-3" style="grid-template-columns:repeat(auto-fill,minmax(160px,1fr))">
			<label class="flex flex-col gap-1" style="color:{muted}">
				<span style="font-size:11px">{m.assertion_evidence_assertion_id()}</span>
				<input
					bind:value={filters.assertion_id}
					class="rounded px-2 py-1.5 text-sm"
					style="background:{surface};color:{text};border:1px solid {border}"
				/>
			</label>
			<label class="flex flex-col gap-1" style="color:{muted}">
				<span style="font-size:11px">{m.assertion_evidence_input_record_id()}</span>
				<input
					bind:value={filters.input_record_id}
					class="rounded px-2 py-1.5 text-sm"
					style="background:{surface};color:{text};border:1px solid {border}"
				/>
			</label>
			<label class="flex flex-col gap-1" style="color:{muted}">
				<span style="font-size:11px">{m.assertion_evidence_artifact_type()}</span>
				<input
					bind:value={filters.artifact_type}
					class="rounded px-2 py-1.5 text-sm"
					style="background:{surface};color:{text};border:1px solid {border}"
				/>
			</label>
			<label class="flex flex-col gap-1" style="color:{muted}">
				<span style="font-size:11px">{m.assertion_evidence_artifact_id()}</span>
				<input
					bind:value={filters.artifact_id}
					class="rounded px-2 py-1.5 text-sm"
					style="background:{surface};color:{text};border:1px solid {border}"
				/>
			</label>
			<label class="flex flex-col gap-1" style="color:{muted}">
				<span style="font-size:11px">{m.assertion_evidence_evidence_role()}</span>
				<select
					bind:value={filters.evidence_role}
					class="rounded px-2 py-1.5 text-sm"
					style="background:{surface};color:{text};border:1px solid {border}"
				>
					<option value="">{m.assertion_evidence_any()}</option><option
						>{m.assertion_evidence_supports()}</option
					><option value="contradicts">{m.assertion_evidence_contradicts()}</option>
				</select>
			</label>
			<label class="flex flex-col gap-1" style="color:{muted}">
				<span style="font-size:11px">{m.assertion_evidence_actor()}</span>
				<select
					bind:value={filters.actor_kind}
					class="rounded px-2 py-1.5 text-sm"
					style="background:{surface};color:{text};border:1px solid {border}"
				>
					<option value="">{m.assertion_evidence_any()}</option><option
						>{m.assertion_evidence_processor()}</option
					><option value="human">{m.assertion_evidence_human()}</option>
				</select>
			</label>
		</div>
		<label class="mt-3 flex items-center gap-2 text-sm" style="color:{muted}">
			<input type="checkbox" bind:checked={filters.include_deleted} onchange={apply} />
			{m.assertion_evidence_include_deleted()}
		</label>
		<div class="mt-3 flex gap-2">
			<button
				onclick={apply}
				class="cursor-pointer rounded-lg px-3 py-2 text-sm"
				style="background:{accent};color:white">{m.assertion_evidence_apply_filters()}</button
			>
			<button
				onclick={clear}
				class="cursor-pointer rounded-lg px-3 py-2 text-sm"
				style="background:{surface};color:{text};border:1px solid {border}"
				>{m.assertion_evidence_clear()}</button
			>
		</div>
	</div>

	{#if error}<div
			class="rounded-xl p-4"
			style="background:{danger}20;border:1px solid {danger}60;color:{danger}"
		>
			{error}
		</div>{/if}
	{#if info}<div
			class="rounded-xl p-4"
			style="background:{accent}20;border:1px solid {accent}60;color:{accent}"
		>
			{info}
		</div>{/if}

	<div class="overflow-hidden rounded-xl" style="background:{card};border:1px solid {border}">
		<div
			class="flex justify-between px-5 py-3"
			style="border-bottom:1px solid {border};color:{muted};font-size:13px"
		>
			{m.assertion_evidence_total({ total })}
			<div class="flex gap-2">
				<button
					onclick={() => {
						if (page > 1) {
							page--;
							load();
						}
					}}
					disabled={page <= 1 || loading}
					class="rounded px-2 py-1 disabled:opacity-40"
					style="background:{surface};color:{text};border:1px solid {border}">‹</button
				>
				<label class="flex items-center gap-2" style="color:{muted}">
					<span>{m.assertion_evidence_page_size()}</span>
					<select
						value={pageSize}
						onchange={(event) => changePageSize((event.currentTarget as HTMLSelectElement).value)}
						disabled={loading}
						class="rounded px-2 py-1"
						style="background:{surface};color:{text};border:1px solid {border}"
					>
						{#each pageSizeOptions as option}<option value={option}>{option}</option>{/each}
					</select>
				</label>
				<span
					>{m.assertion_evidence_page_of({
						page,
						value: Math.max(1, Math.ceil(total / pageSize))
					})}</span
				>
				<button
					onclick={() => {
						if (page < Math.ceil(total / pageSize)) {
							page++;
							load();
						}
					}}
					disabled={page >= Math.ceil(total / pageSize) || loading}
					class="rounded px-2 py-1 disabled:opacity-40"
					style="background:{surface};color:{text};border:1px solid {border}">›</button
				>
			</div>
		</div>
		{#if loading}
			<div class="p-8 text-center" style="color:{muted}">{m.assertion_evidence_loading()}</div>
		{:else if !rows.length}
			<div class="p-8 text-center" style="color:{muted}">
				{m.assertion_evidence_no_evidence_found()}
			</div>
		{:else}
			<div class="overflow-auto">
				<table class="w-full text-sm">
					<thead class="sticky top-0 z-10" style="background:{surface}">
						<tr>
							<th
								class="px-4 py-3 text-left whitespace-nowrap"
								style="color:{muted};font-size:12px;border-bottom:1px solid {border}"
								>{m.assertion_evidence_id()}</th
							>
							{#each sortableHeaders as header}
								<th
									class="px-4 py-3 text-left whitespace-nowrap"
									style="color:{muted};font-size:12px;border-bottom:1px solid {border}"
								>
									<button
										type="button"
										onclick={() => toggleSort(header.key)}
										aria-label={m.assertion_evidence_sort_by({ label: header.label })}
										class="cursor-pointer"
										style="color:{muted};background:none;border:0;padding:0"
									>
										{header.label}{#if sortBy === header.key}<span aria-hidden="true"
												>{sortDir === 'asc' ? '↑' : '↓'}</span
											>{:else}<span aria-hidden="true"> ↕</span>{/if}
									</button>
								</th>
							{/each}
							<th
								class="px-4 py-3 text-left whitespace-nowrap"
								style="color:{muted};font-size:12px;border-bottom:1px solid {border}"
								>{m.assertion_evidence_artifact_type()}</th
							>
							<th
								class="px-4 py-3 text-left whitespace-nowrap"
								style="color:{muted};font-size:12px;border-bottom:1px solid {border}"
								>{m.assertion_evidence_artifact_object_id()}</th
							>
							<th
								class="px-4 py-3 text-left whitespace-nowrap"
								style="color:{muted};font-size:12px;border-bottom:1px solid {border}"
								>{m.assertion_evidence_evidence_quote()}</th
							>
							<th
								class="px-4 py-3 text-left whitespace-nowrap"
								style="color:{muted};font-size:12px;border-bottom:1px solid {border}"
								>{m.assertion_evidence_source_line_spans()}</th
							>
							<th
								class="px-4 py-3 text-left whitespace-nowrap"
								style="color:{muted};font-size:12px;border-bottom:1px solid {border}"
								>{m.assertion_evidence_details()}</th
							>
						</tr>
					</thead>
					<tbody>
						{#each rows as row (row.id)}
							<tr class="hover:bg-white/5">
								<td
									class="px-4 py-3 whitespace-nowrap"
									style="border-bottom:1px solid {border};color:{text}">#{row.id}</td
								>
								<td
									class="px-4 py-3 whitespace-nowrap"
									style="border-bottom:1px solid {border};color:{text}">{row.assertion_id}</td
								>
								<td
									class="px-4 py-3 whitespace-nowrap"
									style="border-bottom:1px solid {border};color:{muted}"
									>{row.input_record_id ?? '—'}</td
								>
								<td
									class="px-4 py-3 whitespace-nowrap"
									style="border-bottom:1px solid {border};color:{muted}">{row.artifact_id}</td
								>
								<td
									class="px-4 py-3 whitespace-nowrap"
									style="border-bottom:1px solid {border};color:{muted}">{row.evidence_role}</td
								>
								<td
									class="px-4 py-3 whitespace-nowrap"
									style="border-bottom:1px solid {border};color:{muted}">{row.confidence ?? '—'}</td
								>
								<td
									class="px-4 py-3 whitespace-nowrap"
									style="border-bottom:1px solid {border};color:{muted}">{time(row.create_time)}</td
								>
								<td
									class="px-4 py-3 whitespace-nowrap"
									style="border-bottom:1px solid {border};color:{muted}">{row.artifact_type}</td
								>
								<td
									class="max-w-48 px-4 py-3 whitespace-nowrap"
									style="border-bottom:1px solid {border};color:{muted};overflow:hidden;text-overflow:ellipsis"
									title={row.artifact_object_id ?? ''}>{row.artifact_object_id ?? '—'}</td
								>
								<td
									class="max-w-64 px-4 py-3"
									style="border-bottom:1px solid {border};color:{muted};overflow:hidden;text-overflow:ellipsis;white-space:nowrap"
									title={row.evidence_quote ?? ''}>{row.evidence_quote ?? '—'}</td
								>
								<td
									class="max-w-48 px-4 py-3"
									style="border-bottom:1px solid {border};color:{muted};font-family:monospace;overflow:hidden;text-overflow:ellipsis;white-space:nowrap"
									title={tableValue(row.source_line_spans)}>{tableValue(row.source_line_spans)}</td
								>
								<td class="px-4 py-3" style="border-bottom:1px solid {border}"
									><button
										type="button"
										onclick={() => openDetails(row)}
										class="cursor-pointer rounded px-2.5 py-1 text-xs"
										style="background:{surface};color:{accent};border:1px solid {border}"
										>{m.assertion_evidence_details()}</button
									></td
								>
							</tr>
						{/each}
					</tbody>
				</table>
			</div>
		{/if}
	</div>
</div>

{#if showCreate}
	<div
		class="fixed inset-0 z-50 flex items-center justify-center p-6"
		style="background:#0008"
		role="presentation"
		onclick={(event) => {
			if (event.target === event.currentTarget) showCreate = false;
		}}
	>
		<div
			class="max-h-[90vh] w-full max-w-4xl space-y-4 overflow-auto rounded-xl p-5"
			style="background:{card};border:1px solid {border}"
			role="dialog"
			aria-modal="true"
			aria-label={m.assertion_evidence_create_assertion_evidence()}
		>
			<h3 style="color:{text};font-weight:600">{m.assertion_evidence_create_evidence()}</h3>
			<div class="grid gap-3 md:grid-cols-2">
				<label style="color:{muted}"
					>{m.assertion_evidence_assertion_id()}<input
						bind:value={form.assertion_id}
						class="w-full rounded px-2 py-1.5 text-sm"
						style="background:{surface};color:{text};border:1px solid {border}"
					/></label
				>
				<label style="color:{muted}"
					>{m.assertion_evidence_input_record_id()}<input
						bind:value={form.input_record_id}
						class="w-full rounded px-2 py-1.5 text-sm"
						style="background:{surface};color:{text};border:1px solid {border}"
					/></label
				>
				<label style="color:{muted}"
					>{m.assertion_evidence_artifact_type()}<input
						bind:value={form.artifact_type}
						class="w-full rounded px-2 py-1.5 text-sm"
						style="background:{surface};color:{text};border:1px solid {border}"
					/></label
				>
				<label style="color:{muted}"
					>{m.assertion_evidence_artifact_id()}<input
						bind:value={form.artifact_id}
						class="w-full rounded px-2 py-1.5 text-sm"
						style="background:{surface};color:{text};border:1px solid {border}"
					/></label
				>
				<label style="color:{muted}"
					>{m.assertion_evidence_artifact_object_id()}<input
						bind:value={form.artifact_object_id}
						class="w-full rounded px-2 py-1.5 text-sm"
						style="background:{surface};color:{text};border:1px solid {border}"
					/></label
				>
				<label style="color:{muted}"
					>{m.assertion_evidence_confidence()}<input
						bind:value={form.confidence}
						type="number"
						min="0"
						max="1"
						step="0.01"
						class="w-full rounded px-2 py-1.5 text-sm"
						style="background:{surface};color:{text};border:1px solid {border}"
					/></label
				>
				<label style="color:{muted}"
					>{m.assertion_evidence_evidence_role()}<select
						bind:value={form.evidence_role}
						class="w-full rounded px-2 py-1.5 text-sm"
						style="background:{surface};color:{text};border:1px solid {border}"
						><option value="supports">{m.assertion_evidence_supports()}</option><option
							>{m.assertion_evidence_contradicts()}</option
						></select
					></label
				>
				<label style="color:{muted}"
					>{m.assertion_evidence_actor()}<select
						bind:value={form.actor_kind}
						class="w-full rounded px-2 py-1.5 text-sm"
						style="background:{surface};color:{text};border:1px solid {border}"
						><option value="processor">{m.assertion_evidence_processor()}</option><option
							>{m.assertion_evidence_human()}</option
						></select
					></label
				>
				<label style="color:{muted}"
					>{m.assertion_evidence_extraction_run()}<input
						bind:value={form.extraction_run}
						class="w-full rounded px-2 py-1.5 text-sm"
						style="background:{surface};color:{text};border:1px solid {border}"
					/></label
				>
				<label style="color:{muted}"
					>{m.assertion_evidence_model()}<input
						bind:value={form.model}
						class="w-full rounded px-2 py-1.5 text-sm"
						style="background:{surface};color:{text};border:1px solid {border}"
					/></label
				>
				<label style="color:{muted}"
					>{m.assertion_evidence_prompt_version()}<input
						bind:value={form.prompt_version}
						class="w-full rounded px-2 py-1.5 text-sm"
						style="background:{surface};color:{text};border:1px solid {border}"
					/></label
				>
			</div>
			<label class="block" style="color:{muted}"
				>{m.assertion_evidence_evidence_quote()}<textarea
					bind:value={form.evidence_quote}
					rows="3"
					class="w-full rounded px-2 py-1.5 text-sm"
					style="background:{surface};color:{text};border:1px solid {border}"
				></textarea></label
			>
			<label class="block" style="color:{muted}"
				>{m.assertion_evidence_source_line_spans()}
				<span class="text-xs">{m.assertion_evidence_json_e_g_12_14()}</span><textarea
					bind:value={form.source_line_spans}
					rows="3"
					class="w-full rounded px-2 py-1.5 font-mono text-sm"
					style="background:{surface};color:{text};border:1px solid {border}"
					placeholder="[&quot;12:14&quot;]"
				></textarea></label
			>
			<div class="flex justify-end gap-2">
				<button
					onclick={() => (showCreate = false)}
					class="cursor-pointer rounded px-3 py-2 text-sm"
					style="background:{surface};color:{text};border:1px solid {border}"
					>{m.assertion_evidence_cancel()}</button
				>
				<button
					onclick={create}
					disabled={saving}
					class="cursor-pointer rounded px-3 py-2 text-sm"
					style="background:{accent};color:white">{m.assertion_evidence_save()}</button
				>
			</div>
		</div>
	</div>
{/if}

{#if detailsRecord}
	<div
		class="fixed inset-0 z-50 flex items-center justify-center p-6"
		style="background:rgba(15,23,42,0.62)"
		role="button"
		tabindex="0"
		aria-label={m.assertion_evidence_close_evidence_details()}
		onclick={(event) => {
			if (event.target === event.currentTarget) detailsRecord = null;
		}}
		onkeydown={(event) => {
			if (event.key === 'Escape') detailsRecord = null;
		}}
	>
		<div
			class="flex max-h-[calc(100vh-48px)] min-h-[200px] w-full max-w-5xl flex-col overflow-hidden rounded-xl"
			style="background:{card};border:1px solid {border}"
			role="dialog"
			aria-modal="true"
			aria-label={m.assertion_evidence_assertion_evidence_details()}
			tabindex="0"
			onclick={(event) => event.stopPropagation()}
			onkeydown={(event) => event.stopPropagation()}
		>
			<div
				class="flex items-center justify-between px-4 py-3"
				style="border-bottom:1px solid {border}"
			>
				<h3 style="font-size:15px;font-weight:600;color:{text}">
					{m.assertion_evidence_evidence_details({ id: detailsRecord.id })}
				</h3>
				<button
					onclick={() => (detailsRecord = null)}
					class="cursor-pointer rounded px-3 py-1.5 text-xs"
					style="background:{surface};color:{muted};border:1px solid {border}"
					>{m.assertion_evidence_close()}</button
				>
			</div>
			<div class="overflow-y-auto p-4">
				<div style="font-size:12px;font-weight:600;color:{muted};margin-bottom:6px">
					{m.assertion_evidence_record_fields()}
				</div>
				<div class="rounded-lg p-2" style="border:1px solid {border};background:{surface}">
					{#each detailRows(detailsRecord) as row}
						<div
							style="display:flex;align-items:baseline;padding-left:{row.depth *
								16}px;min-height:20px;gap:8px;padding-top:2px;padding-bottom:2px"
						>
							<span
								style="font-size:12px;width:180px;flex-shrink:0;word-break:break-all;color:{muted};font-weight:{row.value ===
								null
									? '600'
									: '400'}">{row.key}</span
							>
							{#if row.value !== null}<span
									style="font-size:12px;color:{text};word-break:break-word;white-space:pre-wrap;flex:1;min-width:0"
									>{row.value}</span
								>{/if}
						</div>
					{/each}
				</div>
				{#if detailsRecord.deleted}
					<button
						onclick={restore}
						disabled={saving}
						class="mt-4 cursor-pointer rounded px-3 py-2 text-sm"
						style="background:{accent};color:white">{m.assertion_evidence_restore()}</button
					>
				{:else}
					<div class="mt-4 flex gap-2">
						<input
							placeholder={m.assertion_evidence_deletion_reason()}
							bind:value={reason}
							class="flex-1 rounded px-2 py-2 text-sm"
							style="background:{surface};color:{text};border:1px solid {border}"
						/>
						<button
							onclick={remove}
							disabled={saving}
							class="cursor-pointer rounded px-3 py-2 text-sm"
							style="background:{danger};color:white">{m.assertion_evidence_soft_delete()}</button
						>
					</div>
				{/if}
			</div>
		</div>
	</div>
{/if}
