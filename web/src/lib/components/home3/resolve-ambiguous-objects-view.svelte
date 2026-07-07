<script lang="ts">
	import { onMount } from 'svelte';
	import {
		listAmbiguousObjects,
		getAmbiguousObjectDetail,
		type AmbiguousObjectSummary,
		type ArtifactObjectDetail,
		type ObjectNodeCandidate
	} from './resolve-ambiguous-objects-client.js';

	let { darkMode = true }: { darkMode?: boolean } = $props();

	// --- Design tokens (matches db-consistency-view.svelte / db-maint-log-view.svelte) ---
	let pageBg        = $derived(darkMode ? '#171B26' : '#F2F4F7');
	let cardBg        = $derived(darkMode ? '#1F2333' : '#FFFFFF');
	let borderColor   = $derived(darkMode ? '#2D3348' : '#E4E6EB');
	let accent        = $derived(darkMode ? '#818CF8' : '#6366F1');
	let accentTint    = $derived(darkMode ? 'rgba(129,140,248,0.15)' : 'rgba(99,102,241,0.10)');
	let textPrimary   = $derived(darkMode ? '#E2E8F0' : '#111827');
	let textSecondary = $derived(darkMode ? '#94A3B8' : '#6B7280');
	let textMuted     = $derived(darkMode ? '#64748B' : '#9CA3AF');

	// --- Left panel state ---
	let rows        = $state<AmbiguousObjectSummary[]>([]);
	let listLoading = $state(false);
	let listError   = $state('');

	// --- Right panel state ---
	let selectedId     = $state<number | null>(null);
	let detailLoading  = $state(false);
	let detailError    = $state('');
	let snapshotObject = $state<ArtifactObjectDetail | null>(null);
	let currentObject  = $state<ArtifactObjectDetail | null>(null);
	let snapshotNodes  = $state<ObjectNodeCandidate[]>([]);
	let currentNodes   = $state<ObjectNodeCandidate[]>([]);

	async function loadList() {
		listLoading = true;
		listError = '';
		try {
			const res = await listAmbiguousObjects();
			rows = res.rows;
			if (rows.length > 0 && selectedId === null) {
				await selectRow(rows[0].id);
			}
		} catch (e) {
			listError = e instanceof Error ? e.message : String(e);
		} finally {
			listLoading = false;
		}
	}

	async function selectRow(id: number) {
		selectedId = id;
		detailLoading = true;
		detailError = '';
		try {
			const detail = await getAmbiguousObjectDetail(id);
			if (selectedId !== id) return; // a newer selectRow call superseded this one
			snapshotObject = detail.artifact_object;
			currentObject = { ...detail.artifact_object };
			snapshotNodes = detail.candidates;
			currentNodes = detail.candidates.map((c) => ({ ...c }));
		} catch (e) {
			if (selectedId !== id) return;
			detailError = e instanceof Error ? e.message : String(e);
		} finally {
			if (selectedId === id) detailLoading = false;
		}
	}

	onMount(() => {
		loadList();
	});
</script>

<div class="flex" style="height:100%; min-height:100%; background:{pageBg};">
	<!-- Left panel -->
	<div
		class="flex-shrink-0 overflow-y-auto"
		style="width:320px; border-right:1px solid {borderColor};"
	>
		<div class="p-4" style="border-bottom:1px solid {borderColor};">
			<h1 style="font-size:16px; font-weight:600; color:{textPrimary}; margin-bottom:2px;">
				Resolve Ambiguous Objects
			</h1>
			<p style="font-size:12px; color:{textSecondary};">
				{rows.length} row{rows.length === 1 ? '' : 's'} at reconcile_status = ambiguous
			</p>
		</div>

		{#if listLoading}
			<div class="p-4" style="color:{textMuted}; font-size:13px;">Loading…</div>
		{:else if listError}
			<div class="p-4" style="color:#F87171; font-size:13px;">Error: {listError}</div>
		{:else if rows.length === 0}
			<div class="p-4" style="color:{textMuted}; font-size:13px;">
				No ambiguous objects — the queue is empty.
			</div>
		{:else}
			{#each rows as row (row.id)}
				<button
					type="button"
					onclick={() => selectRow(row.id)}
					class="w-full text-left p-3 cursor-pointer"
					style="
						border-bottom:1px solid {borderColor};
						background:{selectedId === row.id ? accentTint : 'transparent'};
					"
				>
					<div style="font-size:13px; font-weight:500; color:{selectedId === row.id ? accent : textPrimary};">
						{row.object_name}{row.object_name_en ? ` (${row.object_name_en})` : ''}
					</div>
					<div style="font-size:11px; color:{textMuted}; margin-top:2px;">
						{row.artifact_type} · confidence {row.confidence.toFixed(2)}
					</div>
				</button>
			{/each}
		{/if}
	</div>

	<!-- Right panel -->
	<div class="flex-1 overflow-y-auto p-6">
		{#if detailLoading}
			<div style="color:{textMuted}; font-size:13px;">Loading…</div>
		{:else if detailError}
			<div style="color:#F87171; font-size:13px;">Error: {detailError}</div>
		{:else if !currentObject}
			<div style="color:{textMuted}; font-size:13px;">Select a record on the left to resolve it.</div>
		{:else}
			<div style="color:{textPrimary}; font-size:13px;">
				Loaded: {currentObject.object_name} ({currentObject.id})
			</div>
		{/if}
	</div>
</div>
