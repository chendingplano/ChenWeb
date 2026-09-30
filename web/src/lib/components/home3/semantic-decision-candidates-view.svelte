<script lang="ts">
	import { m } from '$lib/paraglide/messages.js';
	import { onMount } from 'svelte';
	import RefreshCwIcon from '@lucide/svelte/icons/refresh-cw';
	import PlusIcon from '@lucide/svelte/icons/plus';
	import {
		listCandidates,
		createCandidate,
		transitionCandidate,
		resolveCandidate,
		deferCandidate,
		retryCandidate,
		type CandidateSortDir,
		type CandidateSortKey,
		type DecisionCandidate
	} from './semantic-decision-candidates-client';

	let { darkMode = true }: { darkMode: boolean } = $props();
	let bg = $derived(darkMode ? '#171B26' : '#F2F4F7'),
		card = $derived(darkMode ? '#1F2333' : '#FFFFFF'),
		surface = $derived(darkMode ? '#252A3A' : '#ECEEF2'),
		border = $derived(darkMode ? '#2D3348' : '#E4E6EB'),
		accent = $derived(darkMode ? '#818CF8' : '#6366F1'),
		text = $derived(darkMode ? '#E2E8F0' : '#111827'),
		muted = $derived(darkMode ? '#94A3B8' : '#6B7280'),
		danger = $derived(darkMode ? '#F87171' : '#DC2626');
	let rows = $state<DecisionCandidate[]>([]),
		total = $state(0),
		page = $state(1),
		loading = $state(false),
		error = $state(''),
		info = $state('');
	let filters = $state({
		status: '',
		candidate_kind: '',
		method: '',
		logical_identity: '',
		source_artifact_type: '',
		source_artifact_id: '',
		input_record_id: ''
	});
	let selected = $state<DecisionCandidate | null>(null),
		showCreate = $state(false),
		saving = $state(false);
	let detailsRecord = $state<DecisionCandidate | null>(null);
	let newIdentity = $state(''),
		newKind = $state('referent'),
		newMethod = $state('semantic_candidate'),
		newPayload = $state('{}'),
		newSourceType = $state(''),
		newSourceID = $state(''),
		newConfidence = $state('');
	let actionReason = $state(''),
		dependency = $state(''),
		outcome = $state('');
	let sortBy = $state<CandidateSortKey | ''>(''),
		sortDir = $state<CandidateSortDir>('asc');
	let pageSize = $state(50);
	const pageSizeOptions = [25, 50, 100, 200];
	const sortableHeaders: { key: CandidateSortKey; label: string }[] = [
		{ key: 'identity', label: m.semantic_decision_candidates_identity() },
		{ key: 'kind', label: m.semantic_decision_candidates_kind() },
		{ key: 'method', label: m.semantic_decision_candidates_method() },
		{ key: 'source', label: m.semantic_decision_candidates_source() },
		{ key: 'confidence', label: m.semantic_decision_candidates_confidence() },
		{ key: 'status', label: m.semantic_decision_candidates_status() },
		{ key: 'resolution', label: m.semantic_decision_candidates_resolution() },
		{ key: 'modified', label: m.semantic_decision_candidates_modified() }
	];
	const kinds = ['referent', 'term_association', 'assertion', 'occurrence', 'profile_selection'];
	const methods = [
		'explicit_structured',
		'deterministic_source_span',
		'released_mapping',
		'lexical_candidate',
		'semantic_candidate',
		'structural_candidate',
		'human'
	];
	const statuses = ['candidate', 'in_review', 'accepted', 'rejected', 'deferred', 'superseded'];
	async function load() {
		loading = true;
		error = '';
		try {
			const result = await listCandidates(
				filters,
				page,
				pageSize,
				sortBy || undefined,
				sortBy ? sortDir : undefined
			);
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
	function toggleSort(key: CandidateSortKey) {
		if (sortBy === key) sortDir = sortDir === 'asc' ? 'desc' : 'asc';
		else {
			sortBy = key;
			sortDir = 'asc';
		}
		page = 1;
		load();
	}
	function clear() {
		for (const key of Object.keys(filters) as (keyof typeof filters)[]) filters[key] = '';
		apply();
	}
	function json(value: unknown) {
		try {
			return JSON.stringify(value, null, 2);
		} catch {
			return String(value);
		}
	}
	function time(value: string) {
		const date = new Date(value);
		return Number.isNaN(date.getTime()) ? value : date.toLocaleString();
	}
	type DetailRow = { key: string; value: string | null; depth: number };
	function detailRows(
		value: unknown,
		key = 'record',
		depth = 0,
		out: DetailRow[] = []
	): DetailRow[] {
		if (value === null || value === undefined) {
			out.push({ key, value: 'null', depth });
			return out;
		}
		if (typeof value !== 'object') {
			out.push({ key, value: String(value), depth });
			return out;
		}
		out.push({ key, value: null, depth });
		if (Array.isArray(value))
			value.forEach((item, index) => detailRows(item, `[${index}]`, depth + 1, out));
		else
			Object.entries(value as Record<string, unknown>).forEach(([childKey, childValue]) =>
				detailRows(childValue, childKey, depth + 1, out)
			);
		return out;
	}
	function can(row: DecisionCandidate, to: string) {
		return (
			(row.status === 'candidate' && ['in_review', 'rejected', 'deferred'].includes(to)) ||
			(row.status === 'in_review' && ['accepted', 'rejected', 'deferred'].includes(to))
		);
	}
	async function transition(to: string) {
		if (!selected) return;
		saving = true;
		try {
			selected = await transitionCandidate(selected.id, { to, reason: actionReason });
			info = m.semantic_decision_candidates_candidate_moved_to({ id: selected.id, to });
			await load();
		} catch (e) {
			error = e instanceof Error ? e.message : String(e);
		} finally {
			saving = false;
		}
	}
	async function saveResolution() {
		if (!selected || !outcome) return;
		saving = true;
		try {
			selected = await resolveCandidate(selected.id, { outcome, reason: actionReason });
			info = m.semantic_decision_candidates_resolution_updated();
			await load();
		} catch (e) {
			error = e instanceof Error ? e.message : String(e);
		} finally {
			saving = false;
		}
	}
	async function defer() {
		if (!selected || !dependency.trim()) return;
		saving = true;
		try {
			selected = await deferCandidate(selected.id, {
				dependency_fingerprint: dependency,
				reason: actionReason
			});
			info = m.semantic_decision_candidates_candidate_deferred();
			await load();
		} catch (e) {
			error = e instanceof Error ? e.message : String(e);
		} finally {
			saving = false;
		}
	}
	async function retry() {
		if (!selected || !dependency.trim()) return;
		saving = true;
		try {
			selected = await retryCandidate(selected.id, { dependency_fingerprint: dependency });
			info = m.semantic_decision_candidates_deferred_candidate_retried();
			await load();
		} catch (e) {
			error = e instanceof Error ? e.message : String(e);
		} finally {
			saving = false;
		}
	}
	async function create() {
		saving = true;
		error = '';
		try {
			const payload = JSON.parse(newPayload);
			const confidence = newConfidence.trim() ? Number(newConfidence) : undefined;
			await createCandidate({
				logical_identity_key: newIdentity,
				candidate_kind: newKind as any,
				method: newMethod as any,
				proposed_payload: payload,
				source_artifact_type: newSourceType,
				source_artifact_id: newSourceID,
				confidence
			});
			showCreate = false;
			info = m.semantic_decision_candidates_candidate_created_or_reused();
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
					{m.semantic_decision_candidates_semantic_decision_candidates()}
				</h2>
				<p style="font-size:13px;color:{muted};margin-top:2px">
					{m.semantic_decision_candidates_lifecycle_safe_administration_of()}
					<code style="color:{accent}">kb.semantic_decision_candidates</code>.
				</p>
			</div>
			<div class="flex gap-2">
				<button
					onclick={() => {
						showCreate = true;
					}}
					class="cursor-pointer rounded-lg px-3 py-2 text-sm"
					style="background:{accent};color:white"
					><PlusIcon class="inline h-4 w-4" />
					{m.semantic_decision_candidates_new_candidate()}</button
				><button
					onclick={load}
					disabled={loading}
					class="cursor-pointer rounded-lg px-3 py-2 text-sm"
					style="background:{surface};color:{text};border:1px solid {border}"
					><RefreshCwIcon class="inline h-4 w-4 {loading ? 'animate-spin' : ''}" />
					{m.semantic_decision_candidates_refresh()}</button
				>
			</div>
		</div>
	</div>
	<div class="rounded-xl p-5" style="background:{card};border:1px solid {border}">
		<div class="grid gap-3" style="grid-template-columns:repeat(auto-fill,minmax(160px,1fr))">
			{#each [['status', m.semantic_decision_candidates_status()], ['candidate_kind', m.semantic_decision_candidates_kind()], ['method', m.semantic_decision_candidates_method()], ['logical_identity', m.semantic_decision_candidates_logical_identity()], ['source_artifact_type', m.semantic_decision_candidates_source_type()], ['source_artifact_id', m.semantic_decision_candidates_source_id()], ['input_record_id', m.semantic_decision_candidates_input_record_id()]] as item}<label
					class="flex flex-col gap-1"
					><span style="font-size:11px;color:{muted}">{item[1]}</span
					>{#if ['status', 'candidate_kind', 'method'].includes(item[0])}<select
							bind:value={filters[item[0] as keyof typeof filters]}
							class="rounded px-2 py-1.5 text-sm"
							style="background:{surface};color:{text};border:1px solid {border}"
							><option value="">{m.semantic_decision_candidates_any()}</option
							>{#each item[0] === 'status' ? statuses : item[0] === 'candidate_kind' ? kinds : methods as option}<option
									value={option}>{option}</option
								>{/each}</select
						>{:else}<input
							bind:value={filters[item[0] as keyof typeof filters]}
							class="rounded px-2 py-1.5 text-sm"
							style="background:{surface};color:{text};border:1px solid {border}"
						/>{/if}</label
				>{/each}
		</div>
		<div class="mt-4 flex gap-2">
			<button
				onclick={apply}
				class="cursor-pointer rounded-lg px-3 py-2 text-sm"
				style="background:{accent};color:white"
				>{m.semantic_decision_candidates_apply_filters()}</button
			><button
				onclick={clear}
				class="cursor-pointer rounded-lg px-3 py-2 text-sm"
				style="background:{surface};color:{text};border:1px solid {border}"
				>{m.semantic_decision_candidates_clear()}</button
			>
		</div>
	</div>
	{#if error}<div
			class="rounded-xl p-4"
			style="background:{danger}20;border:1px solid {danger}60;color:{danger}"
		>
			{error}
		</div>{/if}{#if info}<div
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
			{m.semantic_decision_candidates_total({ total })}
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
				><label class="flex items-center gap-2" style="color:{muted}">
					<span>{m.semantic_decision_candidates_page_size()}</span>
					<select
						value={pageSize}
						onchange={(event) => changePageSize((event.currentTarget as HTMLSelectElement).value)}
						disabled={loading}
						class="rounded px-2 py-1"
						style="background:{surface};color:{text};border:1px solid {border}"
					>
						{#each pageSizeOptions as option}<option value={option}>{option}</option>{/each}
					</select>
				</label><span
					>{m.semantic_decision_candidates_page_of({
						page,
						totalPages: Math.max(1, Math.ceil(total / pageSize))
					})}</span
				><button
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
		{#if loading}<div class="p-8 text-center" style="color:{muted}">
				{m.semantic_decision_candidates_loading()}
			</div>{:else if !rows.length}<div class="p-8 text-center" style="color:{muted}">
				{m.semantic_decision_candidates_no_candidates_found()}
			</div>{:else}<div class="overflow-auto">
				<table class="w-full text-sm">
					<thead class="sticky top-0 z-10" style="background:{surface}"
						><tr style="background:{surface}">
							{#each [m.semantic_decision_candidates_id_revision(), m.semantic_decision_candidates_input_record_id()] as h}<th
									class="px-4 py-3 text-left whitespace-nowrap"
									style="color:{muted};font-size:12px;border-bottom:1px solid {border}">{h}</th
								>{/each}
							{#each sortableHeaders as header}
								<th
									class="px-4 py-3 text-left whitespace-nowrap"
									style="color:{muted};font-size:12px;border-bottom:1px solid {border}"
								>
									<button
										type="button"
										onclick={() => toggleSort(header.key)}
										aria-label={m.semantic_decision_candidates_sort_by({ label: header.label })}
										class="cursor-pointer"
										style="color:{muted};background:none;border:0;padding:0"
									>
										{header.label}{#if sortBy === header.key}<span aria-hidden="true">
												{sortDir === 'asc' ? '↑' : '↓'}</span
											>{:else}<span aria-hidden="true"> ↕</span>{/if}
									</button>
								</th>
							{/each}
							<th
								class="px-4 py-3 text-left whitespace-nowrap"
								style="color:{muted};font-size:12px;border-bottom:1px solid {border}"
								>{m.semantic_decision_candidates_details()}</th
							>
						</tr></thead
					><tbody
						>{#each rows as row (row.id)}<tr class="hover:bg-white/5"
								><td
									class="px-4 py-3 whitespace-nowrap"
									style="border-bottom:1px solid {border};color:{text}"
									><button
										onclick={() => {
											selected = row;
											actionReason = '';
											outcome = row.resolution_outcome ?? '';
											dependency = row.dependency_fingerprint ?? '';
										}}
										style="color:{accent};background:none;border:0;cursor:pointer"
										>#{row.id} / r{row.revision}</button
									></td
								><td
									class="px-4 py-3 whitespace-nowrap"
									style="border-bottom:1px solid {border};color:{muted}"
									>{row.input_record_id ?? '—'}</td
								><td
									class="px-4 py-3"
									style="border-bottom:1px solid {border};color:{muted};font-family:monospace;max-width:260px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap"
									>{row.logical_identity_key}</td
								><td class="px-4 py-3" style="border-bottom:1px solid {border};color:{muted}"
									>{row.candidate_kind}</td
								><td class="px-4 py-3" style="border-bottom:1px solid {border};color:{muted}"
									>{row.method}</td
								><td class="px-4 py-3" style="border-bottom:1px solid {border};color:{muted}"
									>{row.source_artifact_type ?? '—'} {row.source_artifact_id ?? ''}</td
								><td class="px-4 py-3" style="border-bottom:1px solid {border};color:{muted}"
									>{row.confidence == null ? '—' : row.confidence.toFixed(3)}</td
								><td class="px-4 py-3" style="border-bottom:1px solid {border};color:{text}"
									>{row.status}</td
								><td class="px-4 py-3" style="border-bottom:1px solid {border};color:{muted}"
									>{row.resolution_outcome ?? '—'}</td
								><td
									class="px-4 py-3 whitespace-nowrap"
									style="border-bottom:1px solid {border};color:{muted}">{time(row.modify_time)}</td
								><td class="px-4 py-3" style="border-bottom:1px solid {border}"
									><button
										onclick={() => {
											detailsRecord = row;
										}}
										class="rounded px-2.5 py-1 text-xs"
										style="background:{surface};color:{accent};border:1px solid {border}"
										>{m.semantic_decision_candidates_details()}</button
									></td
								></tr
							>{/each}</tbody
					>
				</table>
			</div>{/if}
	</div>
</div>

{#if showCreate || selected}<div
		class="fixed inset-0 z-50 flex items-center justify-center p-6"
		style="background:#0008"
		role="presentation"
		onclick={(e) => {
			if (e.target === e.currentTarget) {
				showCreate = false;
				selected = null;
			}
		}}
	>
		<div
			class="max-h-[90vh] w-full max-w-4xl space-y-4 overflow-auto rounded-xl p-5"
			style="background:{card};border:1px solid {border}"
		>
			{#if showCreate}<h3 style="color:{text};font-weight:600">
					{m.semantic_decision_candidates_new_semantic_decision_candidate()}
				</h3>
				<div class="grid gap-3 md:grid-cols-2">
					<label style="color:{muted}"
						>{m.semantic_decision_candidates_logical_identity_2()}<input
							bind:value={newIdentity}
							class="w-full rounded px-2 py-1.5"
							style="background:{surface};color:{text};border:1px solid {border}"
						/></label
					><label style="color:{muted}"
						>{m.semantic_decision_candidates_kind()}<select
							bind:value={newKind}
							class="w-full rounded px-2 py-1.5"
							style="background:{surface};color:{text};border:1px solid {border}"
							>{#each kinds as option}<option>{option}</option>{/each}</select
						></label
					><label style="color:{muted}"
						>{m.semantic_decision_candidates_method()}<select
							bind:value={newMethod}
							class="w-full rounded px-2 py-1.5"
							style="background:{surface};color:{text};border:1px solid {border}"
							>{#each methods as option}<option>{option}</option>{/each}</select
						></label
					><label style="color:{muted}"
						>{m.semantic_decision_candidates_confidence()}<input
							bind:value={newConfidence}
							type="number"
							min="0"
							max="1"
							step="0.01"
							class="w-full rounded px-2 py-1.5"
							style="background:{surface};color:{text};border:1px solid {border}"
						/></label
					><label style="color:{muted}"
						>{m.semantic_decision_candidates_source_type_2()}<input
							bind:value={newSourceType}
							class="w-full rounded px-2 py-1.5"
							style="background:{surface};color:{text};border:1px solid {border}"
						/></label
					><label style="color:{muted}"
						>{m.semantic_decision_candidates_source_id()}<input
							bind:value={newSourceID}
							class="w-full rounded px-2 py-1.5"
							style="background:{surface};color:{text};border:1px solid {border}"
						/></label
					>
				</div>
				<label style="color:{muted}"
					>{m.semantic_decision_candidates_proposed_payload()}<textarea
						bind:value={newPayload}
						rows="10"
						class="w-full rounded px-2 py-1.5 font-mono text-xs"
						style="background:{surface};color:{text};border:1px solid {border}"
					></textarea></label
				>
				<div class="flex justify-end gap-2">
					<button
						onclick={() => {
							showCreate = false;
						}}
						class="rounded px-3 py-2"
						style="background:{surface};color:{text};border:1px solid {border}"
						>{m.semantic_decision_candidates_cancel()}</button
					><button
						onclick={create}
						disabled={saving}
						class="rounded px-3 py-2"
						style="background:{accent};color:white"
						>{m.semantic_decision_candidates_create()}</button
					>
				</div>{:else if selected}<div class="flex justify-between">
					<h3 style="color:{text};font-weight:600">
						{m.semantic_decision_candidates_candidate_revision({
							id: selected.id,
							revision: selected.revision
						})}
					</h3>
					<button
						onclick={() => {
							selected = null;
						}}
						class="rounded px-3 py-1"
						style="background:{surface};color:{text};border:1px solid {border}"
						>{m.semantic_decision_candidates_close()}</button
					>
				</div>
				<pre
					class="max-h-64 overflow-auto rounded p-3 text-xs"
					style="background:{surface};color:{muted}">{json(selected)}</pre>
				<label style="color:{muted}"
					>{m.semantic_decision_candidates_reason()}<textarea
						bind:value={actionReason}
						rows="2"
						class="w-full rounded px-2 py-1.5"
						style="background:{surface};color:{text};border:1px solid {border}"
					></textarea></label
				>
				<div class="flex flex-wrap gap-2">
					{#each ['in_review', 'accepted', 'rejected', 'deferred'] as next}{#if can(selected, next)}<button
								onclick={() => transition(next)}
								disabled={saving}
								class="rounded px-3 py-2 text-sm"
								style="background:{accent};color:white"
								>{m.semantic_decision_candidates_move_to({ next })}</button
							>{/if}{/each}
				</div>
				<div class="grid gap-3 md:grid-cols-3">
					<label style="color:{muted}"
						>{m.semantic_decision_candidates_resolution()}<select
							bind:value={outcome}
							class="w-full rounded px-2 py-1.5"
							style="background:{surface};color:{text};border:1px solid {border}"
							><option value="">—</option
							>{#each ['matched', 'new_target_candidate', 'rejected', 'deferred'] as option}<option
									>{option}</option
								>{/each}</select
						></label
					><button
						onclick={saveResolution}
						disabled={saving || !outcome}
						class="self-end rounded px-3 py-2"
						style="background:{surface};color:{text};border:1px solid {border}"
						>{m.semantic_decision_candidates_save_resolution()}</button
					><label style="color:{muted}"
						>{m.semantic_decision_candidates_dependency_fingerprint()}<input
							bind:value={dependency}
							class="w-full rounded px-2 py-1.5"
							style="background:{surface};color:{text};border:1px solid {border}"
						/></label
					>
				</div>
				{#if selected.status === 'deferred'}<button
						onclick={retry}
						disabled={saving || !dependency.trim()}
						class="rounded px-3 py-2"
						style="background:{surface};color:{text};border:1px solid {border}"
						>{m.semantic_decision_candidates_retry_deferred()}</button
					>{/if}{/if}
		</div>
	</div>{/if}

{#if detailsRecord}<div
		class="fixed inset-0 z-50 flex items-center justify-center p-6"
		style="background:rgba(15,23,42,0.62)"
		role="button"
		tabindex="0"
		aria-label={m.semantic_decision_candidates_close_candidate_details()}
		onclick={(e) => {
			if (e.target === e.currentTarget) detailsRecord = null;
		}}
		onkeydown={(e) => {
			if (e.key === 'Escape') detailsRecord = null;
		}}
	>
		<div
			class="flex flex-col rounded-xl"
			style="background:{card};border:1px solid {border};width:min(1046px,calc(100vw - 48px));max-height:calc(100vh - 48px);overflow:hidden;resize:both;min-width:480px;min-height:200px"
			role="dialog"
			aria-modal="true"
			aria-label={m.semantic_decision_candidates_semantic_decision_candidate_details()}
			tabindex="0"
			onclick={(e) => e.stopPropagation()}
			onkeydown={(e) => e.stopPropagation()}
		>
			<div
				class="flex items-center justify-between px-4 py-3"
				style="border-bottom:1px solid {border}"
			>
				<h3 style="font-size:15px;font-weight:600;color:{text}">
					{m.semantic_decision_candidates_candidate_details({ id: detailsRecord.id })}
				</h3>
				<button
					onclick={() => {
						detailsRecord = null;
					}}
					class="rounded px-3 py-1.5 text-xs"
					style="background:{surface};color:{muted};border:1px solid {border}"
					>{m.semantic_decision_candidates_close()}</button
				>
			</div>
			<div class="overflow-y-auto p-4">
				<div style="font-size:12px;font-weight:600;color:{muted};margin-bottom:6px">
					{m.semantic_decision_candidates_record_fields()}
				</div>
				<div class="rounded-lg p-2" style="border:1px solid {border};background:{surface}">
					{#each detailRows(detailsRecord) as row}<div
							style="display:flex;align-items:baseline;padding-left:{row.depth *
								16}px;min-height:20px;gap:8px;padding-top:2px;padding-bottom:2px"
						>
							<span
								style="font-size:12px;width:180px;flex-shrink:0;word-break:break-all;color:{muted};font-weight:{row.value ===
								null
									? '600'
									: '400'}">{row.key}</span
							>{#if row.value !== null}<span
									style="font-size:12px;color:{text};word-break:break-word;white-space:pre-wrap;flex:1;min-width:0"
									>{row.value}</span
								>{/if}
						</div>{/each}
				</div>
			</div>
		</div>
	</div>{/if}
