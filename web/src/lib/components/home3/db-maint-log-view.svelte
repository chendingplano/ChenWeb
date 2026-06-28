<script lang="ts">
	import { onMount } from 'svelte';
	import RefreshCwIcon  from '@lucide/svelte/icons/refresh-cw';
	import SearchIcon     from '@lucide/svelte/icons/search';
	import ChevronDownIcon from '@lucide/svelte/icons/chevron-down';
	import ChevronUpIcon   from '@lucide/svelte/icons/chevron-up';

	let { darkMode = true }: { darkMode?: boolean } = $props();

	// --- Design tokens ---
	let pageBg        = $derived(darkMode ? '#171B26' : '#F2F4F7');
	let cardBg        = $derived(darkMode ? '#1F2333' : '#FFFFFF');
	let surface2      = $derived(darkMode ? '#252A3A' : '#ECEEF2');
	let borderColor   = $derived(darkMode ? '#2D3348' : '#E4E6EB');
	let accent        = $derived(darkMode ? '#818CF8' : '#6366F1');
	let textPrimary   = $derived(darkMode ? '#E2E8F0' : '#111827');
	let textSecondary = $derived(darkMode ? '#94A3B8' : '#6B7280');
	let textMuted     = $derived(darkMode ? '#64748B' : '#9CA3AF');
	let inputBg       = $derived(darkMode ? '#141824' : '#FFFFFF');

	// --- Types ---
	type LogRow = {
		id: number;
		operation: string;
		result_data: Record<string, unknown>;
		performed_at: string;
	};

	// --- State ---
	let rows      = $state<LogRow[]>([]);
	let total     = $state(0);
	let page      = $state(1);
	let pageSize  = $state(50);
	let loading   = $state(false);
	let error     = $state('');
	let expandedId = $state<number | null>(null);

	// Filters
	let filterOperation = $state('');
	let filterDateFrom  = $state('');
	let filterDateTo    = $state('');

	const operationOptions = [
		{ value: '', label: 'All operations' },
		{ value: 'check-kb-inputs-status', label: 'check-kb-inputs-status' },
		{ value: 'fix-kb-inputs-status',   label: 'fix-kb-inputs-status' }
	];

	// --- Data loading ---
	async function load() {
		loading = true;
		error = '';
		try {
			const params = new URLSearchParams();
			if (filterOperation) params.set('operation', filterOperation);
			if (filterDateFrom)  params.set('date_from', new Date(filterDateFrom).toISOString());
			if (filterDateTo)    params.set('date_to',   new Date(filterDateTo + 'T23:59:59').toISOString());
			params.set('page',      String(page));
			params.set('page_size', String(pageSize));

			const res = await fetch(`/api/v1/admin/db/maintenance-logs?${params}`);
			if (!res.ok) {
				const body = await res.json().catch(() => ({}));
				throw new Error((body as { error?: string }).error ?? `HTTP ${res.status}`);
			}
			const data = await res.json() as { results: LogRow[]; total: number; page: number; page_size: number };
			rows  = data.results ?? [];
			total = data.total ?? 0;
		} catch (e) {
			error = e instanceof Error ? e.message : String(e);
		} finally {
			loading = false;
		}
	}

	function search() {
		page = 1;
		load();
	}

	function formatDate(iso: string): string {
		try {
			return new Date(iso).toLocaleString();
		} catch {
			return iso;
		}
	}

	function formatResult(result: Record<string, unknown>): string {
		return Object.entries(result)
			.map(([k, v]) => `${k}: ${v}`)
			.join('  ·  ');
	}

	const totalPages = $derived(Math.max(1, Math.ceil(total / pageSize)));

	onMount(() => { load(); });
</script>

<div class="p-6" style="background:{pageBg}; min-height:100%;">
	<div class="mb-5">
		<h1 style="font-size:20px; font-weight:600; color:{textPrimary}; margin-bottom:4px;">
			Database Maintenance — Maintenance Log
		</h1>
		<p style="font-size:13px; color:{textSecondary};">
			Audit log of all database maintenance operations (consistency checks and fixes).
		</p>
	</div>

	<!-- Search section -->
	<div
		class="rounded-xl p-5 mb-5"
		style="background:{cardBg}; border:1px solid {borderColor};"
	>
		<div class="flex flex-wrap gap-3 items-end">
			<!-- Operation filter -->
			<div class="flex flex-col gap-1">
				<label style="font-size:12px; font-weight:500; color:{textMuted};">Operation</label>
				<select
					bind:value={filterOperation}
					style="
						background:{inputBg}; border:1px solid {borderColor}; color:{textPrimary};
						border-radius:7px; padding:6px 10px; font-size:13px; min-width:220px;
					"
				>
					{#each operationOptions as opt}
						<option value={opt.value}>{opt.label}</option>
					{/each}
				</select>
			</div>

			<!-- Date from -->
			<div class="flex flex-col gap-1">
				<label style="font-size:12px; font-weight:500; color:{textMuted};">From</label>
				<input
					type="date"
					bind:value={filterDateFrom}
					style="
						background:{inputBg}; border:1px solid {borderColor}; color:{textPrimary};
						border-radius:7px; padding:6px 10px; font-size:13px;
					"
				/>
			</div>

			<!-- Date to -->
			<div class="flex flex-col gap-1">
				<label style="font-size:12px; font-weight:500; color:{textMuted};">To</label>
				<input
					type="date"
					bind:value={filterDateTo}
					style="
						background:{inputBg}; border:1px solid {borderColor}; color:{textPrimary};
						border-radius:7px; padding:6px 10px; font-size:13px;
					"
				/>
			</div>

			<!-- Buttons -->
			<div class="flex gap-2">
				<button
					onclick={search}
					disabled={loading}
					style="
						display:flex; align-items:center; gap:6px;
						padding:7px 16px; border-radius:7px; border:none; cursor:pointer;
						font-size:13px; font-weight:500;
						background:{accent}; color:white;
						opacity:{loading ? 0.6 : 1};
					"
				>
					<SearchIcon style="width:13px; height:13px;" />
					Search
				</button>
				<button
					onclick={load}
					disabled={loading}
					title="Refresh"
					style="
						display:flex; align-items:center; justify-content:center;
						width:34px; height:34px; border-radius:7px; border:none; cursor:pointer;
						background:{surface2}; color:{textSecondary};
						opacity:{loading ? 0.6 : 1};
					"
				>
					<RefreshCwIcon style="width:14px; height:14px;{loading ? ' animation:spin 1s linear infinite;' : ''}" />
				</button>
			</div>
		</div>
	</div>

	<!-- Error -->
	{#if error}
		<div
			class="rounded-xl p-4 mb-4"
			style="background:rgba(248,113,113,0.08); border:1px solid rgba(248,113,113,0.25); color:#F87171; font-size:13px;"
		>
			{error}
		</div>
	{/if}

	<!-- Results -->
	<div class="rounded-xl overflow-hidden" style="background:{cardBg}; border:1px solid {borderColor};">
		<!-- Table header -->
		<div
			class="grid"
			style="
				grid-template-columns: 60px 220px 1fr 180px 36px;
				padding:10px 16px;
				background:{surface2};
				border-bottom:1px solid {borderColor};
				font-size:12px; font-weight:600; color:{textMuted};
			"
		>
			<span>ID</span>
			<span>Operation</span>
			<span>Result</span>
			<span>Performed At</span>
			<span></span>
		</div>

		{#if loading && rows.length === 0}
			<div class="p-8 text-center" style="font-size:13px; color:{textMuted};">Loading…</div>
		{:else if rows.length === 0}
			<div class="p-8 text-center" style="font-size:13px; color:{textMuted};">No records found.</div>
		{:else}
			{#each rows as row (row.id)}
				<!-- Main row -->
				<div
					class="grid items-center"
					style="
						grid-template-columns: 60px 220px 1fr 180px 36px;
						padding:10px 16px;
						border-bottom:1px solid {borderColor};
						font-size:13px;
						cursor:pointer;
					"
					onclick={() => { expandedId = expandedId === row.id ? null : row.id; }}
				>
					<span style="color:{textMuted}; font-size:12px;">{row.id}</span>
					<span
						style="
							color:{accent}; font-family:monospace; font-size:12px;
							white-space:nowrap; overflow:hidden; text-overflow:ellipsis;
						"
					>{row.operation}</span>
					<span style="color:{textSecondary}; font-size:12px; white-space:nowrap; overflow:hidden; text-overflow:ellipsis;">
						{formatResult(row.result_data)}
					</span>
					<span style="color:{textMuted}; font-size:12px;">{formatDate(row.performed_at)}</span>
					<span style="display:flex; justify-content:center; color:{textMuted};">
						{#if expandedId === row.id}
							<ChevronUpIcon style="width:14px; height:14px;" />
						{:else}
							<ChevronDownIcon style="width:14px; height:14px;" />
						{/if}
					</span>
				</div>

				<!-- Expanded detail -->
				{#if expandedId === row.id}
					<div
						style="
							padding:12px 16px 14px 80px;
							border-bottom:1px solid {borderColor};
							background:{surface2};
						"
					>
						<div style="font-size:11px; font-weight:600; color:{textMuted}; margin-bottom:6px; text-transform:uppercase; letter-spacing:0.05em;">
							Result Data
						</div>
						<pre style="
							font-size:12px; color:{textPrimary}; background:{inputBg};
							border:1px solid {borderColor}; border-radius:6px;
							padding:10px 12px; overflow-x:auto; margin:0;
						">{JSON.stringify(row.result_data, null, 2)}</pre>
					</div>
				{/if}
			{/each}
		{/if}
	</div>

	<!-- Pagination -->
	{#if total > 0}
		<div class="flex items-center justify-between mt-4" style="font-size:13px; color:{textSecondary};">
			<span>{total} record{total === 1 ? '' : 's'} total</span>
			<div class="flex items-center gap-2">
				<button
					onclick={() => { page = Math.max(1, page - 1); load(); }}
					disabled={page <= 1 || loading}
					style="
						padding:5px 12px; border-radius:6px; border:1px solid {borderColor};
						background:{surface2}; color:{textPrimary}; cursor:pointer; font-size:13px;
						opacity:{page <= 1 || loading ? 0.4 : 1};
					"
				>Prev</button>
				<span>Page {page} of {totalPages}</span>
				<button
					onclick={() => { page = Math.min(totalPages, page + 1); load(); }}
					disabled={page >= totalPages || loading}
					style="
						padding:5px 12px; border-radius:6px; border:1px solid {borderColor};
						background:{surface2}; color:{textPrimary}; cursor:pointer; font-size:13px;
						opacity:{page >= totalPages || loading ? 0.4 : 1};
					"
				>Next</button>
			</div>
		</div>
	{/if}
</div>

<style>
	@keyframes spin {
		from { transform: rotate(0deg); }
		to   { transform: rotate(360deg); }
	}
</style>
