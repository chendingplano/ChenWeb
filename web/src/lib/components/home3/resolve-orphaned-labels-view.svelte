<script lang="ts">
	import { onMount } from 'svelte';
	import SearchIcon from '@lucide/svelte/icons/search';
	import AlertTriangleIcon from '@lucide/svelte/icons/alert-triangle';
	import Trash2Icon from '@lucide/svelte/icons/trash-2';
	import LoaderCircleIcon from '@lucide/svelte/icons/loader-circle';
	import {
		listOrphanedLabels,
		resolveOrphanedLabels,
		type OrphanedLabel,
		type OrphanedLabelsFilters
	} from './resolve-orphaned-labels-client.js';

	let { darkMode = true }: { darkMode?: boolean } = $props();

	let pageBg = $derived(darkMode ? '#171B26' : '#F2F4F7');
	let cardBg = $derived(darkMode ? '#1F2333' : '#FFFFFF');
	let borderColor = $derived(darkMode ? '#2D3348' : '#E4E6EB');
	let accent = $derived(darkMode ? '#818CF8' : '#6366F1');
	let accentTint = $derived(darkMode ? 'rgba(129,140,248,0.15)' : 'rgba(99,102,241,0.10)');
	let textPrimary = $derived(darkMode ? '#E2E8F0' : '#111827');
	let textSecondary = $derived(darkMode ? '#94A3B8' : '#6B7280');
	let textMuted = $derived(darkMode ? '#64748B' : '#9CA3AF');
	let inputBg = $derived(darkMode ? '#141824' : '#FFFFFF');
	let warning = $derived(darkMode ? '#FBBF24' : '#B45309');
	let danger = $derived(darkMode ? '#F87171' : '#DC2626');

	let rows = $state<OrphanedLabel[]>([]);
	let total = $state(0);
	let searchQuery = $state('');
	let searchLang = $state('');
	let searchRole = $state('');
	let loading = $state(false);
	let resolving = $state(false);
	let errorMessage = $state('');
	let successMessage = $state('');

	const filters = $derived<OrphanedLabelsFilters>({
		q: searchQuery,
		lang: searchLang,
		label_role: searchRole
	});

	async function loadRows() {
		loading = true;
		errorMessage = '';
		successMessage = '';
		try {
			const response = await listOrphanedLabels(filters);
			rows = response.results;
			total = response.total;
		} catch (error) {
			errorMessage = error instanceof Error ? error.message : String(error);
		} finally {
			loading = false;
		}
	}

	async function resolveRows() {
		if (rows.length === 0 || resolving) return;
		if (!window.confirm(`Remove ${rows.length} listed orphaned label${rows.length === 1 ? '' : 's'}?`)) return;
		resolving = true;
		errorMessage = '';
		successMessage = '';
		try {
			const response = await resolveOrphanedLabels(rows.map((row) => row.id), filters);
			successMessage = `Removed ${response.deleted_count} orphaned label${response.deleted_count === 1 ? '' : 's'}.`;
			await loadRows();
		} catch (error) {
			errorMessage = error instanceof Error ? error.message : String(error);
		} finally {
			resolving = false;
		}
	}

	function formatTime(value?: string) {
		if (!value) return '—';
		const date = new Date(value);
		return Number.isNaN(date.valueOf()) ? value : date.toLocaleString();
	}

	onMount(loadRows);
</script>

<div class="flex h-full flex-col overflow-hidden p-6" style="background:{pageBg};">
	<div class="mb-5 flex-shrink-0">
		<div class="mb-1 flex items-center gap-2">
			<AlertTriangleIcon style="width:18px; height:18px; color:{warning};" />
			<h1 style="font-size:20px; font-weight:600; color:{textPrimary};">
				Database Maintenance — Resolve Orphaned Labels
			</h1>
		</div>
		<p style="max-width:900px; font-size:13px; line-height:1.55; color:{textSecondary};">
			An orphaned label is a row in <code>kb.ontology_term_labels</code> whose
			<code>term_id</code> no longer matches a row in <code>kb.ontology_terms</code>.
			These labels cannot be resolved safely and may block ontology identity updates. Search for
			the rows you want to clean up, then use Resolve to remove the listed orphaned labels.
		</p>
	</div>

	<div class="flex min-h-0 flex-1 flex-col gap-4">
		<div class="flex flex-wrap items-end gap-3 rounded-xl p-4" style="background:{cardBg}; border:1px solid {borderColor};">
			<div class="flex min-w-[220px] flex-1 flex-col gap-1">
				<label for="orphaned-label-query" style="font-size:12px; font-weight:500; color:{textMuted};">
					Search term ID, label, or language
				</label>
				<input id="orphaned-label-query" bind:value={searchQuery} onkeydown={(event) => event.key === 'Enter' && loadRows()} placeholder="e.g. 每户配备" style="background:{inputBg}; border:1px solid {borderColor}; color:{textPrimary}; border-radius:7px; padding:7px 10px; font-size:13px;" />
			</div>
			<div class="flex w-36 flex-col gap-1">
				<label for="orphaned-label-lang" style="font-size:12px; font-weight:500; color:{textMuted};">Language</label>
				<input id="orphaned-label-lang" bind:value={searchLang} placeholder="zh-cn" style="background:{inputBg}; border:1px solid {borderColor}; color:{textPrimary}; border-radius:7px; padding:7px 10px; font-size:13px;" />
			</div>
			<div class="flex w-36 flex-col gap-1">
				<label for="orphaned-label-role" style="font-size:12px; font-weight:500; color:{textMuted};">Label role</label>
				<select id="orphaned-label-role" bind:value={searchRole} style="background:{inputBg}; border:1px solid {borderColor}; color:{textPrimary}; border-radius:7px; padding:7px 10px; font-size:13px;">
					<option value="">All roles</option>
					<option value="prefLabel">prefLabel</option>
					<option value="altLabel">altLabel</option>
					<option value="hiddenLabel">hiddenLabel</option>
				</select>
			</div>
			<button onclick={loadRows} disabled={loading} style="display:flex; align-items:center; gap:6px; border:0; border-radius:7px; padding:8px 14px; background:{accent}; color:white; font-size:13px; font-weight:500; opacity:{loading ? 0.6 : 1};">
				<SearchIcon style="width:14px; height:14px;" />
				{loading ? 'Searching…' : 'Search'}
			</button>
			<button onclick={resolveRows} disabled={rows.length === 0 || resolving || loading} style="display:flex; align-items:center; gap:6px; border:1px solid rgba(248,113,113,0.45); border-radius:7px; padding:8px 14px; background:rgba(248,113,113,0.12); color:{danger}; font-size:13px; font-weight:600; opacity:{rows.length === 0 || resolving || loading ? 0.45 : 1};">
				{#if resolving}<LoaderCircleIcon class="animate-spin" style="width:14px; height:14px;" />{:else}<Trash2Icon style="width:14px; height:14px;" />{/if}
				{resolving ? 'Resolving…' : 'Resolve'}
			</button>
		</div>

		{#if errorMessage}<div class="rounded-xl p-3" style="background:rgba(248,113,113,0.08); border:1px solid rgba(248,113,113,0.25); color:{danger}; font-size:13px;">{errorMessage}</div>{/if}
		{#if successMessage}<div class="rounded-xl p-3" style="background:{accentTint}; border:1px solid {borderColor}; color:{textPrimary}; font-size:13px;">{successMessage}</div>{/if}

		<div class="min-h-0 flex-1 overflow-auto rounded-xl" style="background:{cardBg}; border:1px solid {borderColor};">
			<div class="flex items-center justify-between border-b px-4 py-3" style="border-color:{borderColor};">
				<div style="font-size:13px; font-weight:600; color:{textPrimary};">Orphaned labels</div>
				<div style="font-size:12px; color:{textSecondary};">{total} found · {rows.length} listed</div>
			</div>
			{#if loading && rows.length === 0}
				<div class="p-6 text-center" style="font-size:13px; color:{textMuted};">Loading orphaned labels…</div>
			{:else if rows.length === 0}
				<div class="p-6 text-center" style="font-size:13px; color:{textMuted};">No orphaned labels match the current search.</div>
			{:else}
				<table class="w-full border-collapse text-left" style="font-size:12px; color:{textSecondary};">
					<thead style="position:sticky; top:0; background:{cardBg}; color:{textMuted};">
						<tr>
							<th class="px-4 py-3 font-medium">Term ID</th><th class="px-4 py-3 font-medium">Label</th><th class="px-4 py-3 font-medium">Language</th><th class="px-4 py-3 font-medium">Role</th><th class="px-4 py-3 font-medium">Status</th><th class="px-4 py-3 font-medium">Modified</th>
						</tr>
					</thead>
					<tbody>
						{#each rows as row (row.id)}
							<tr style="border-top:1px solid {borderColor};">
								<td class="max-w-[260px] break-all px-4 py-3" style="color:{textPrimary};">{row.term_id}</td>
								<td class="max-w-[360px] break-words px-4 py-3" style="color:{textPrimary};">{row.label}</td>
								<td class="px-4 py-3">{row.lang}</td><td class="px-4 py-3">{row.label_role}</td><td class="px-4 py-3">{row.status}</td><td class="whitespace-nowrap px-4 py-3">{formatTime(row.modify_time)}</td>
							</tr>
						{/each}
					</tbody>
				</table>
			{/if}
		</div>
	</div>
</div>
