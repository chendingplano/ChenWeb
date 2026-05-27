<script lang="ts">
	import { onMount } from 'svelte';
	import RefreshCwIcon from '@lucide/svelte/icons/refresh-cw';
	import CircleAlertIcon from '@lucide/svelte/icons/circle-alert';
	import SearchIcon from '@lucide/svelte/icons/search';
	import TrashIcon from '@lucide/svelte/icons/trash-2';

	let { darkMode = true }: { darkMode: boolean } = $props();

	// --- Design tokens ---
	let pageBg        = $derived(darkMode ? '#171B26' : '#F2F4F7');
	let cardBg        = $derived(darkMode ? '#1F2333' : '#FFFFFF');
	let surface2      = $derived(darkMode ? '#252A3A' : '#ECEEF2');
	let borderColor   = $derived(darkMode ? '#2D3348' : '#E4E6EB');
	let accent        = $derived(darkMode ? '#818CF8' : '#6366F1');
	let textPrimary   = $derived(darkMode ? '#E2E8F0' : '#111827');
	let textSecondary = $derived(darkMode ? '#94A3B8' : '#6B7280');
	let textMuted     = $derived(darkMode ? '#64748B' : '#9CA3AF');
	let success       = $derived(darkMode ? '#34D399' : '#059669');
	let danger        = $derived(darkMode ? '#F87171' : '#DC2626');
	let warning       = $derived(darkMode ? '#FBBF24' : '#D97706');

	// --- Types ---
	type LogRow = {
		id: number;
		call_reason: string;
		doc_proc_name: string;
		model_names: string[];
		prompt_name: string;
		entry_type: string;
		pass?: number;
		llm_call_id?: string;
		activity_name?: string;
		artifact?: string;
		errors?: string;
		extra_info?: string;
		ms_used?: number;
		create_time: string;
	};

	// --- State ---
	let logs         = $state<LogRow[]>([]);
	let total        = $state(0);
	let page         = $state(1);
	let pageSize     = $state(50);
	let loading      = $state(false);
	let error        = $state('');

	// Filters
	let filterEntryType   = $state('');
	let filterDocProcName = $state('');

	// Retention
	let retentionDays       = $state(30);
	let retentionLoading    = $state(false);
	let retentionError      = $state('');
	let retentionSuccess    = $state('');

	// Expanded row for artifact/extra_info detail
	let expandedRowId = $state<number | null>(null);

	const entryTypeOptions = [
		{ value: '', label: 'All types' },
		{ value: 'llm_call', label: 'LLM Call' },
		{ value: 'doc_proc_summary', label: 'Summary' }
	];

	// --- Data loading ---
	async function load() {
		loading = true;
		error = '';
		try {
			const params = new URLSearchParams({
				page: String(page),
				page_size: String(pageSize)
			});
			if (filterEntryType)   params.set('entry_type',    filterEntryType);
			if (filterDocProcName) params.set('doc_proc_name', filterDocProcName.trim());

			const res = await fetch(`/api/v1/kb/doc-proc-logs?${params}`, {
				credentials: 'same-origin'
			});
			const data = await res.json();
			if (!res.ok || !data.status) {
				throw new Error(data.error_msg ?? 'Failed to load logs');
			}
			logs  = data.results ?? [];
			total = data.total  ?? 0;
		} catch (err) {
			error = err instanceof Error ? err.message : String(err);
			logs  = [];
			total = 0;
		} finally {
			loading = false;
		}
	}

	function applyFilters() {
		page = 1;
		load();
	}

	function prevPage() {
		if (page > 1) { page--; load(); }
	}

	function nextPage() {
		const totalPages = Math.ceil(total / pageSize);
		if (page < totalPages) { page++; load(); }
	}

	async function applyRetention() {
		retentionLoading = true;
		retentionError   = '';
		retentionSuccess = '';
		try {
			const res = await fetch(`/api/v1/kb/doc-proc-logs/old?days=${retentionDays}`, {
				method: 'DELETE',
				credentials: 'same-origin'
			});
			const data = await res.json();
			if (!res.ok || !data.status) {
				throw new Error(data.error_msg ?? 'Failed to apply retention');
			}
			retentionSuccess = data.message ?? 'Retention applied.';
			load();
		} catch (err) {
			retentionError = err instanceof Error ? err.message : String(err);
		} finally {
			retentionLoading = false;
		}
	}

	onMount(() => { load(); });

	// --- Helpers ---
	function formatTime(iso: string): string {
		if (!iso) return '—';
		return new Date(iso).toLocaleString();
	}

	function formatDuration(ms?: number): string {
		if (ms == null) return '—';
		if (ms < 1000) return `${ms}ms`;
		return `${(ms / 1000).toFixed(1)}s`;
	}

	function entryTypeBadge(t: string): string {
		return t === 'llm_call' ? accent : warning;
	}

	function hasErrors(row: LogRow): boolean {
		return !!(row.errors && row.errors.trim());
	}

	function toggleExpand(id: number) {
		expandedRowId = expandedRowId === id ? null : id;
	}

	let totalPages = $derived(Math.ceil(total / pageSize) || 1);
</script>

<div class="p-6 space-y-4" style="background:{pageBg}; min-height:100%;">

	<!-- Header card -->
	<div class="rounded-xl p-5" style="background:{cardBg}; border:1px solid {borderColor};">
		<div class="flex flex-wrap items-start justify-between gap-3">
			<div>
				<h2 style="font-size:18px; font-weight:600; color:{textPrimary};">Doc Processor Logs</h2>
				<p style="font-size:13px; color:{textSecondary}; margin-top:2px;">
					LLM call traces and processor-level summaries for all doc pipeline runs.
				</p>
			</div>
			<button
				onclick={load}
				disabled={loading}
				class="inline-flex items-center gap-2 rounded-lg px-3 py-2 cursor-pointer"
				style="background:{surface2}; color:{textPrimary}; border:1px solid {borderColor};"
			>
				<RefreshCwIcon class="w-4 h-4 {loading ? 'animate-spin' : ''}" />
				Refresh
			</button>
		</div>

		<!-- Filters row -->
		<div class="mt-4 flex flex-wrap items-end gap-3">
			<div>
				<label style="font-size:12px; color:{textMuted}; display:block; margin-bottom:4px;">Entry Type</label>
				<select
					bind:value={filterEntryType}
					class="rounded-lg px-3 py-2 text-sm"
					style="background:{surface2}; border:1px solid {borderColor}; color:{textPrimary};"
				>
					{#each entryTypeOptions as opt}
						<option value={opt.value}>{opt.label}</option>
					{/each}
				</select>
			</div>
			<div>
				<label style="font-size:12px; color:{textMuted}; display:block; margin-bottom:4px;">Processor Name</label>
				<input
					type="text"
					bind:value={filterDocProcName}
					placeholder="e.g. extract_metrics"
					class="rounded-lg px-3 py-2 text-sm"
					style="background:{surface2}; border:1px solid {borderColor}; color:{textPrimary}; width:180px;"
				/>
			</div>
			<button
				onclick={applyFilters}
				class="inline-flex items-center gap-2 rounded-lg px-4 py-2 text-sm cursor-pointer"
				style="background:{accent}; color:#fff; border:none;"
			>
				<SearchIcon class="w-4 h-4" />
				Search
			</button>
		</div>
	</div>

	<!-- Error banner -->
	{#if error}
		<div class="rounded-xl p-4 flex items-start gap-2"
			style="background:{danger}20; border:1px solid {danger}70; color:{danger};">
			<CircleAlertIcon class="w-4 h-4 mt-0.5 flex-shrink-0" />
			<span style="font-size:13px;">{error}</span>
		</div>
	{/if}

	<!-- Log table -->
	<div class="rounded-xl overflow-hidden" style="background:{cardBg}; border:1px solid {borderColor};">
		<!-- Table meta -->
		<div class="px-5 py-3 flex items-center justify-between"
			style="border-bottom:1px solid {borderColor};">
			<span style="font-size:13px; color:{textMuted};">
				{total} entries
				{#if total > 0} &middot; page {page} of {totalPages}{/if}
			</span>
			<div class="flex items-center gap-2">
				<button
					onclick={prevPage}
					disabled={page <= 1 || loading}
					class="rounded px-3 py-1 text-sm cursor-pointer disabled:opacity-40"
					style="background:{surface2}; color:{textPrimary}; border:1px solid {borderColor};"
				>
					&lsaquo; Prev
				</button>
				<button
					onclick={nextPage}
					disabled={page >= totalPages || loading}
					class="rounded px-3 py-1 text-sm cursor-pointer disabled:opacity-40"
					style="background:{surface2}; color:{textPrimary}; border:1px solid {borderColor};"
				>
					Next &rsaquo;
				</button>
			</div>
		</div>

		{#if loading}
			<div class="px-5 py-8 text-center" style="color:{textMuted}; font-size:14px;">Loading…</div>
		{:else if logs.length === 0}
			<div class="px-5 py-8 text-center" style="color:{textMuted}; font-size:14px;">No log entries found.</div>
		{:else}
			<div class="overflow-x-auto">
				<table class="w-full text-sm border-collapse">
					<thead>
						<tr style="border-bottom:1px solid {borderColor}; background:{surface2};">
							<th class="text-left px-4 py-3" style="color:{textMuted}; font-weight:500; white-space:nowrap;">Type</th>
							<th class="text-left px-4 py-3" style="color:{textMuted}; font-weight:500; white-space:nowrap;">Processor</th>
							<th class="text-left px-4 py-3" style="color:{textMuted}; font-weight:500; white-space:nowrap;">Activity</th>
							<th class="text-left px-4 py-3" style="color:{textMuted}; font-weight:500; white-space:nowrap;">Model(s)</th>
							<th class="text-left px-4 py-3" style="color:{textMuted}; font-weight:500; white-space:nowrap;">Pass</th>
							<th class="text-left px-4 py-3" style="color:{textMuted}; font-weight:500; white-space:nowrap;">Duration</th>
							<th class="text-left px-4 py-3" style="color:{textMuted}; font-weight:500; white-space:nowrap;">Create Time</th>
							<th class="text-left px-4 py-3" style="color:{textMuted}; font-weight:500; white-space:nowrap;">Errors</th>
						</tr>
					</thead>
					<tbody>
						{#each logs as row (row.id)}
							<tr
								onclick={() => toggleExpand(row.id)}
								class="cursor-pointer transition-colors"
								style="
									border-bottom:1px solid {borderColor};
									background:{expandedRowId === row.id ? accent + '10' : 'transparent'};
								"
							>
								<td class="px-4 py-3">
									<span
										class="inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium"
										style="background:{entryTypeBadge(row.entry_type)}22; color:{entryTypeBadge(row.entry_type)};"
									>
										{row.entry_type === 'llm_call' ? 'LLM Call' : 'Summary'}
									</span>
								</td>
								<td class="px-4 py-3" style="color:{textPrimary}; white-space:nowrap;">
									{row.doc_proc_name}
								</td>
								<td class="px-4 py-3" style="color:{textSecondary};">
									{row.activity_name ?? '—'}
								</td>
								<td class="px-4 py-3" style="color:{textSecondary}; max-width:180px; overflow:hidden; text-overflow:ellipsis; white-space:nowrap;"
									title={row.model_names.join(', ')}>
									{row.model_names.length > 0 ? row.model_names.join(', ') : '—'}
								</td>
								<td class="px-4 py-3" style="color:{textMuted};">
									{row.pass != null ? row.pass : '—'}
								</td>
								<td class="px-4 py-3" style="color:{textSecondary}; white-space:nowrap;">
									{formatDuration(row.ms_used)}
								</td>
								<td class="px-4 py-3" style="color:{textMuted}; white-space:nowrap; font-size:12px;">
									{formatTime(row.create_time)}
								</td>
								<td class="px-4 py-3">
									{#if hasErrors(row)}
										<span style="color:{danger}; font-size:12px; max-width:200px; display:inline-block; overflow:hidden; text-overflow:ellipsis; white-space:nowrap;"
											title={row.errors}>
											{row.errors}
										</span>
									{:else}
										<span style="color:{success}; font-size:12px;">OK</span>
									{/if}
								</td>
							</tr>

							{#if expandedRowId === row.id}
								<tr style="background:{surface2}; border-bottom:1px solid {borderColor};">
									<td colspan="8" class="px-6 py-4">
										<div class="grid gap-3" style="grid-template-columns: repeat(auto-fill, minmax(280px,1fr));">
											{#if row.call_reason}
												<div>
													<div style="font-size:11px; color:{textMuted}; font-weight:500; text-transform:uppercase; letter-spacing:.05em; margin-bottom:4px;">Reason</div>
													<div style="font-size:13px; color:{textSecondary};">{row.call_reason}</div>
												</div>
											{/if}
											{#if row.llm_call_id}
												<div>
													<div style="font-size:11px; color:{textMuted}; font-weight:500; text-transform:uppercase; letter-spacing:.05em; margin-bottom:4px;">LLM Call ID</div>
													<div style="font-size:12px; color:{textSecondary}; font-family:monospace;">{row.llm_call_id}</div>
												</div>
											{/if}
											{#if row.prompt_name}
												<div>
													<div style="font-size:11px; color:{textMuted}; font-weight:500; text-transform:uppercase; letter-spacing:.05em; margin-bottom:4px;">Prompt</div>
													<div style="font-size:13px; color:{textSecondary};">{row.prompt_name}</div>
												</div>
											{/if}
											<div>
												<div style="font-size:11px; color:{textMuted}; font-weight:500; text-transform:uppercase; letter-spacing:.05em; margin-bottom:4px;">Create Time</div>
												<div style="font-size:12px; color:{textSecondary};">{formatTime(row.create_time)}</div>
											</div>
										</div>

										{#if row.extra_info}
											<div class="mt-3">
												<div style="font-size:11px; color:{textMuted}; font-weight:500; text-transform:uppercase; letter-spacing:.05em; margin-bottom:6px;">Extra Info</div>
												<pre class="rounded-lg p-3 overflow-x-auto text-xs"
													style="background:{cardBg}; border:1px solid {borderColor}; color:{textSecondary}; max-height:200px;">{JSON.stringify(JSON.parse(row.extra_info), null, 2)}</pre>
											</div>
										{/if}

										{#if row.artifact}
											<div class="mt-3">
												<div style="font-size:11px; color:{textMuted}; font-weight:500; text-transform:uppercase; letter-spacing:.05em; margin-bottom:6px;">Artifact</div>
												<pre class="rounded-lg p-3 overflow-x-auto text-xs"
													style="background:{cardBg}; border:1px solid {borderColor}; color:{textSecondary}; max-height:200px;">{JSON.stringify(JSON.parse(row.artifact), null, 2)}</pre>
											</div>
										{/if}

										{#if hasErrors(row)}
											<div class="mt-3">
												<div style="font-size:11px; color:{textMuted}; font-weight:500; text-transform:uppercase; letter-spacing:.05em; margin-bottom:6px;">Error Detail</div>
												<div class="rounded-lg p-3 text-xs"
													style="background:{danger}10; border:1px solid {danger}40; color:{danger};">
													{row.errors}
												</div>
											</div>
										{/if}
									</td>
								</tr>
							{/if}
						{/each}
					</tbody>
				</table>
			</div>
		{/if}
	</div>

	<!-- Retention configuration card -->
	<div class="rounded-xl p-5" style="background:{cardBg}; border:1px solid {borderColor};">
		<h3 style="font-size:15px; font-weight:600; color:{textPrimary}; margin-bottom:4px;">Retention Policy</h3>
		<p style="font-size:13px; color:{textSecondary}; margin-bottom:16px;">
			Remove log entries older than the specified number of days. This action is permanent.
		</p>

		<div class="flex flex-wrap items-end gap-3">
			<div>
				<label style="font-size:12px; color:{textMuted}; display:block; margin-bottom:4px;">
					Retain logs for (days)
				</label>
				<input
					type="number"
					min="1"
					bind:value={retentionDays}
					class="rounded-lg px-3 py-2 text-sm w-28"
					style="background:{surface2}; border:1px solid {borderColor}; color:{textPrimary};"
				/>
			</div>
			<button
				onclick={applyRetention}
				disabled={retentionLoading || retentionDays < 1}
				class="inline-flex items-center gap-2 rounded-lg px-4 py-2 text-sm cursor-pointer disabled:opacity-40"
				style="background:{danger}; color:#fff; border:none;"
			>
				<TrashIcon class="w-4 h-4" />
				{retentionLoading ? 'Deleting…' : 'Apply Retention'}
			</button>
		</div>

		{#if retentionSuccess}
			<div class="mt-3 rounded-lg px-4 py-2 text-sm"
				style="background:{success}15; border:1px solid {success}50; color:{success};">
				{retentionSuccess}
			</div>
		{/if}
		{#if retentionError}
			<div class="mt-3 rounded-lg px-4 py-2 text-sm flex items-center gap-2"
				style="background:{danger}15; border:1px solid {danger}50; color:{danger};">
				<CircleAlertIcon class="w-4 h-4 flex-shrink-0" />
				{retentionError}
			</div>
		{/if}
	</div>
</div>
