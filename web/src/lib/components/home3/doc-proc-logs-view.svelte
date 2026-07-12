<script lang="ts">
	import { onMount } from 'svelte';
	import RefreshCwIcon from '@lucide/svelte/icons/refresh-cw';
	import CircleAlertIcon from '@lucide/svelte/icons/circle-alert';
	import SearchIcon from '@lucide/svelte/icons/search';
	import TrashIcon from '@lucide/svelte/icons/trash-2';
	import ArrowDownIcon from '@lucide/svelte/icons/arrow-down';
	import ArrowUpIcon from '@lucide/svelte/icons/arrow-up';
	import ArrowUpDownIcon from '@lucide/svelte/icons/arrow-up-down';
	import EyeIcon from '@lucide/svelte/icons/eye';

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
		run_id?: number;
		entry_type: string;
		proc_progress?: string;
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
	let orderBy      = $state('create_time');
	let orderDir     = $state<'asc' | 'desc'>('desc');

	// Filters
	let filterEntryType = $state('');
	let filterDocProcName = $state('');
	let filterActivityName = $state('');
	let filterRunID = $state('');
	let filterCreateStart = $state('');
	let filterCreateEnd = $state('');

	// Retention
	let retentionDays       = $state(30);
	let retentionLoading    = $state(false);
	let retentionError      = $state('');
	let retentionSuccess    = $state('');

	// Selected row for detail dialog
	let detailDialogOpen = $state(false);
	let detailRow = $state<LogRow | null>(null);
	let entryTypeOptions = $state<Array<{ value: string; label: string }>>([
		{ value: '', label: 'All types' }
	]);
	let processorOptions = $state<Array<{ value: string; label: string }>>([
		{ value: '', label: 'All processors' }
	]);
	let activityOptions = $state<Array<{ value: string; label: string }>>([
		{ value: '', label: 'All activities' }
	]);

	const sortableColumns = [
		{ field: 'entry_type', label: 'Type' },
		{ field: 'doc_proc_name', label: 'Processor' },
		{ field: 'activity_name', label: 'Activity' },
		{ field: 'model_names', label: 'Model(s)' },
		{ field: 'pass', label: 'Pass' },
		{ field: 'ms_used', label: 'Duration' },
		{ field: 'run_id', label: 'Run ID' },
		{ field: 'create_time', label: 'Create Time' },
		{ field: 'errors', label: 'Errors' }
	];

	// --- Data loading ---
	function toRFC3339(localDateTime: string): string {
		return new Date(localDateTime).toISOString();
	}

	function parseTimestamp(value?: string): Date | null {
		if (!value) return null;
		const direct = new Date(value);
		if (!Number.isNaN(direct.getTime())) return direct;

		const normalized = value
			.trim()
			.replace(' ', 'T')
			.replace(/ ([+-]\d{2})(\d{2})$/, '$1:$2');
		const fallback = new Date(normalized);
		if (!Number.isNaN(fallback.getTime())) return fallback;

		return null;
	}

	function buildOptions(values: string[], allLabel: string): Array<{ value: string; label: string }> {
		return [{ value: '', label: allLabel }, ...values.map((value) => ({ value, label: value }))];
	}

	async function loadFilterOptions() {
		try {
			const res = await fetch('/api/v1/kb/doc-proc-logs/filter-options', {
				credentials: 'same-origin'
			});
			const data = await res.json();
			if (!res.ok || !data.status) {
				throw new Error(data.error_msg ?? 'Failed to load filter options');
			}
			entryTypeOptions = buildOptions(data.entry_types ?? [], 'All types');
			processorOptions = buildOptions(data.doc_proc_names ?? [], 'All processors');
			activityOptions = buildOptions(data.activity_names ?? [], 'All activities');
		} catch {
			entryTypeOptions = buildOptions([], 'All types');
			processorOptions = buildOptions([], 'All processors');
			activityOptions = buildOptions([], 'All activities');
		}
	}

	async function load() {
		loading = true;
		error = '';
		try {
			const params = new URLSearchParams({
				page: String(page),
				page_size: String(pageSize),
				order_by: orderBy,
				order_dir: orderDir
			});
			if (filterEntryType)   params.set('entry_type',    filterEntryType);
			if (filterDocProcName) params.set('doc_proc_name', filterDocProcName.trim());
			if (filterActivityName) params.set('activity_name', filterActivityName);
			if (filterRunID.trim()) params.set('run_id', filterRunID.trim());
			if (filterCreateStart) params.set('create_start_time', toRFC3339(filterCreateStart));
			if (filterCreateEnd) params.set('create_end_time', toRFC3339(filterCreateEnd));

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

	function setOrder(field: string) {
		if (orderBy === field) {
			orderDir = orderDir === 'asc' ? 'desc' : 'asc';
		} else {
			orderBy = field;
			orderDir = 'asc';
		}
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

	onMount(() => {
		loadFilterOptions();
		load();
	});

	// --- Helpers ---
	function formatTime(iso: string): string {
		if (!iso) return '—';
		const parsed = parseTimestamp(iso);
		return parsed ? parsed.toLocaleString() : iso;
	}

	function formatDuration(ms?: number): string {
		if (ms == null) return '—';
		if (ms < 1000) return `${ms}ms`;
		return `${(ms / 1000).toFixed(1)}s`;
	}

	function entryTypeBadge(t: string): string {
		return t === 'llm_call' ? accent : warning;
	}

	function entryTypeLabel(t: string): string {
		return t;
	}

	function hasErrors(row: LogRow): boolean {
		return !!(row.errors && row.errors.trim());
	}

	function showDetails(row: LogRow) {
		detailRow = row;
		detailDialogOpen = true;
	}

	function closeDetails() {
		detailDialogOpen = false;
		detailRow = null;
	}

	function prettyJSON(value?: string): string {
		if (!value) return '';
		try {
			return JSON.stringify(JSON.parse(value), null, 2);
		} catch {
			return value;
		}
	}

	function parseObjectEntries(value?: string): Array<{ key: string; value: string }> {
		if (!value) return [];
		try {
			const parsed = JSON.parse(value);
			if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) return [];
			return Object.entries(parsed).map(([key, entryValue]) => ({
				key,
				value: typeof entryValue === 'string' ? entryValue : JSON.stringify(entryValue)
			}));
		} catch {
			return [];
		}
	}

	let totalPages = $derived(Math.ceil(total / pageSize) || 1);
	let detailExtraInfoEntries = $derived(parseObjectEntries(detailRow?.extra_info));
</script>

<div class="p-6 space-y-4 h-full flex flex-col overflow-hidden" style="background:{pageBg};">

	<!-- Header card -->
	<div class="rounded-xl p-5 flex-shrink-0" style="background:{cardBg}; border:1px solid {borderColor};">
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
				<label for="doc-proc-log-entry-type" style="font-size:12px; color:{textMuted}; display:block; margin-bottom:4px;">Entry Type</label>
				<select
					id="doc-proc-log-entry-type"
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
				<label for="doc-proc-log-processor-name" style="font-size:12px; color:{textMuted}; display:block; margin-bottom:4px;">Processor</label>
				<select
					id="doc-proc-log-processor-name"
					bind:value={filterDocProcName}
					class="rounded-lg px-3 py-2 text-sm"
					style="background:{surface2}; border:1px solid {borderColor}; color:{textPrimary}; width:180px;"
				>
					{#each processorOptions as opt}
						<option value={opt.value}>{opt.label}</option>
					{/each}
				</select>
			</div>
			<div>
				<label for="doc-proc-log-activity-name" style="font-size:12px; color:{textMuted}; display:block; margin-bottom:4px;">Activity</label>
				<select
					id="doc-proc-log-activity-name"
					bind:value={filterActivityName}
					class="rounded-lg px-3 py-2 text-sm"
					style="background:{surface2}; border:1px solid {borderColor}; color:{textPrimary}; width:220px;"
				>
					{#each activityOptions as opt}
						<option value={opt.value}>{opt.label}</option>
					{/each}
				</select>
			</div>
			<div>
				<label for="doc-proc-log-run-id" style="font-size:12px; color:{textMuted}; display:block; margin-bottom:4px;">Run ID</label>
				<input
					id="doc-proc-log-run-id"
					type="number"
					min="1"
					bind:value={filterRunID}
					placeholder="e.g. 42"
					class="rounded-lg px-3 py-2 text-sm"
					style="background:{surface2}; border:1px solid {borderColor}; color:{textPrimary}; width:140px;"
				/>
			</div>
			<div>
				<label for="doc-proc-log-create-start" style="font-size:12px; color:{textMuted}; display:block; margin-bottom:4px;">Start Time From</label>
				<input
					id="doc-proc-log-create-start"
					type="datetime-local"
					bind:value={filterCreateStart}
					class="rounded-lg px-3 py-2 text-sm"
					style="background:{surface2}; border:1px solid {borderColor}; color:{textPrimary}; width:210px;"
				/>
			</div>
			<div>
				<label for="doc-proc-log-create-end" style="font-size:12px; color:{textMuted}; display:block; margin-bottom:4px;">Start Time To</label>
				<input
					id="doc-proc-log-create-end"
					type="datetime-local"
					bind:value={filterCreateEnd}
					class="rounded-lg px-3 py-2 text-sm"
					style="background:{surface2}; border:1px solid {borderColor}; color:{textPrimary}; width:210px;"
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
		<div class="rounded-xl p-4 flex items-start gap-2 flex-shrink-0"
			style="background:{danger}20; border:1px solid {danger}70; color:{danger};">
			<CircleAlertIcon class="w-4 h-4 mt-0.5 flex-shrink-0" />
			<span style="font-size:13px;">{error}</span>
		</div>
	{/if}

	<!-- Log table -->
	<div class="rounded-xl overflow-hidden flex flex-col flex-1 min-h-0" style="background:{cardBg}; border:1px solid {borderColor};">
		<!-- Table meta -->
		<div class="px-5 py-3 flex items-center justify-between flex-shrink-0"
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
			<div class="flex-1 min-h-0 overflow-auto">
				<table class="w-full text-sm border-collapse">
					<thead>
						<tr style="border-bottom:1px solid {borderColor}; background:{surface2};">
							{#each sortableColumns as column}
								<th class="text-left px-4 py-3 sticky top-0 z-10" style="white-space:nowrap; background:{surface2};">
									<button
										onclick={() => setOrder(column.field)}
										class="inline-flex items-center gap-1.5 rounded px-1.5 py-1 text-xs cursor-pointer"
										style="
											color:{orderBy === column.field ? textPrimary : textMuted};
											background:{orderBy === column.field ? accent + '18' : 'transparent'};
											border:1px solid {orderBy === column.field ? accent + '55' : 'transparent'};
											font-weight:500;
										"
										title={`Order by ${column.label}`}
										aria-label={`Order by ${column.label}`}
									>
										<span>{column.label}</span>
										{#if orderBy === column.field && orderDir === 'asc'}
											<ArrowUpIcon class="w-3.5 h-3.5" />
										{:else if orderBy === column.field && orderDir === 'desc'}
											<ArrowDownIcon class="w-3.5 h-3.5" />
										{:else}
											<ArrowUpDownIcon class="w-3.5 h-3.5" />
										{/if}
									</button>
								</th>
							{/each}
							<th class="text-left px-4 py-3 sticky top-0 z-10" style="color:{textMuted}; font-weight:500; white-space:nowrap; background:{surface2};">Actions</th>
						</tr>
					</thead>
					<tbody>
						{#each logs as row (row.id)}
							<tr
								class="transition-colors"
								style="
									border-bottom:1px solid {borderColor};
									background:{detailDialogOpen && detailRow?.id === row.id ? accent + '10' : 'transparent'};
								"
							>
								<td class="px-4 py-3">
									<span
										class="inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium"
										style="background:{entryTypeBadge(row.entry_type)}22; color:{entryTypeBadge(row.entry_type)};"
									>
										{entryTypeLabel(row.entry_type)}
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
								<td class="px-4 py-3" style="color:{textSecondary}; white-space:nowrap; font-family:ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;">
									{row.run_id != null ? row.run_id : '—'}
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
								<td class="px-4 py-3">
									<button
										onclick={() => showDetails(row)}
										class="inline-flex items-center gap-1.5 rounded px-2.5 py-1.5 text-xs cursor-pointer"
										style="background:{surface2}; color:{textPrimary}; border:1px solid {borderColor};"
										aria-expanded={detailDialogOpen && detailRow?.id === row.id}
										aria-label={`Show details for log ${row.id}`}
									>
										<EyeIcon class="w-3.5 h-3.5" />
										Details
									</button>
								</td>
							</tr>
						{/each}
					</tbody>
				</table>
			</div>
		{/if}
	</div>

	<!-- Retention configuration card -->
	<div class="rounded-xl p-5 flex-shrink-0" style="background:{cardBg}; border:1px solid {borderColor};">
		<h3 style="font-size:15px; font-weight:600; color:{textPrimary}; margin-bottom:4px;">Retention Policy</h3>
		<p style="font-size:13px; color:{textSecondary}; margin-bottom:16px;">
			Remove log entries older than the specified number of days. This action is permanent.
		</p>

		<div class="flex flex-wrap items-end gap-3">
			<div>
				<label for="doc-proc-log-retention-days" style="font-size:12px; color:{textMuted}; display:block; margin-bottom:4px;">
					Retain logs for (days)
				</label>
				<input
					id="doc-proc-log-retention-days"
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

{#if detailDialogOpen && detailRow}
	<div
		class="doc-proc-view-dialog-overlay"
		onclick={(e) => { if (e.target === e.currentTarget) closeDetails(); }}
		onkeydown={(e) => { if (e.key === 'Escape') closeDetails(); }}
		role="button"
		tabindex="0"
	>
		<div
			class="doc-proc-view-dialog"
			onclick={(e) => e.stopPropagation()}
			onkeydown={(e) => e.stopPropagation()}
			role="dialog"
			aria-modal="true"
			aria-label="Doc processor log details"
			tabindex="0"
		>
			<div class="doc-proc-view-dialog-head">
				<h3 class="doc-proc-view-dialog-title">
					Log Entry #{detailRow.id}
				</h3>
				<button class="doc-proc-view-close-btn" onclick={closeDetails}>Close</button>
			</div>
			<div class="doc-proc-view-dialog-body">
				<div class="doc-proc-view-section">
					<div class="doc-proc-view-section-label">Summary</div>
					<div class="doc-proc-view-rows-box">
						<div class="doc-proc-view-row">
							<span class="doc-proc-view-key">entry_type</span>
							<span class="doc-proc-view-val">{detailRow.entry_type || '—'}</span>
						</div>
						<div class="doc-proc-view-row">
							<span class="doc-proc-view-key">doc_proc_name</span>
							<span class="doc-proc-view-val">{detailRow.doc_proc_name || '—'}</span>
						</div>
						<div class="doc-proc-view-row">
							<span class="doc-proc-view-key">activity_name</span>
							<span class="doc-proc-view-val">{detailRow.activity_name || '—'}</span>
						</div>
						<div class="doc-proc-view-row">
							<span class="doc-proc-view-key">call_reason</span>
							<span class="doc-proc-view-val">{detailRow.call_reason || '—'}</span>
						</div>
						<div class="doc-proc-view-row">
							<span class="doc-proc-view-key">prompt_name</span>
							<span class="doc-proc-view-val">{detailRow.prompt_name || '—'}</span>
						</div>
						<div class="doc-proc-view-row">
							<span class="doc-proc-view-key">model_names</span>
							<span class="doc-proc-view-val">{detailRow.model_names.length ? detailRow.model_names.join(', ') : '—'}</span>
						</div>
						<div class="doc-proc-view-row">
							<span class="doc-proc-view-key">pass</span>
							<span class="doc-proc-view-val">{detailRow.pass != null ? detailRow.pass : '—'}</span>
						</div>
						<div class="doc-proc-view-row">
							<span class="doc-proc-view-key">duration</span>
							<span class="doc-proc-view-val">{formatDuration(detailRow.ms_used)}</span>
						</div>
						<div class="doc-proc-view-row">
							<span class="doc-proc-view-key">create_time</span>
							<span class="doc-proc-view-val">{formatTime(detailRow.create_time)}</span>
						</div>
						<div class="doc-proc-view-row">
							<span class="doc-proc-view-key">run_id</span>
							<span class="doc-proc-view-val doc-proc-view-val-mono">{detailRow.run_id != null ? detailRow.run_id : '—'}</span>
						</div>
						<div class="doc-proc-view-row">
							<span class="doc-proc-view-key">proc_progress</span>
							<span class="doc-proc-view-val">{detailRow.proc_progress || '—'}</span>
						</div>
						<div class="doc-proc-view-row">
							<span class="doc-proc-view-key">llm_call_id</span>
							<span class="doc-proc-view-val doc-proc-view-val-mono">{detailRow.llm_call_id || '—'}</span>
						</div>
					</div>
				</div>

				{#if detailRow.extra_info}
					<div class="doc-proc-view-section">
						<div class="doc-proc-view-section-label">Extra Info</div>
						{#if detailExtraInfoEntries.length > 0}
							<div class="doc-proc-view-rows-box">
								{#each detailExtraInfoEntries as entry (entry.key)}
									<div class="doc-proc-view-row">
										<span class="doc-proc-view-key">{entry.key}</span>
										<span class="doc-proc-view-val">{entry.value}</span>
									</div>
								{/each}
							</div>
						{:else}
							<pre class="doc-proc-view-pre">{prettyJSON(detailRow.extra_info)}</pre>
						{/if}
					</div>
				{/if}

				{#if detailRow.artifact}
					<div class="doc-proc-view-section">
						<div class="doc-proc-view-section-label">Artifact</div>
						<pre class="doc-proc-view-pre">{prettyJSON(detailRow.artifact)}</pre>
					</div>
				{/if}

				<div class="doc-proc-view-section">
					<div class="doc-proc-view-section-label">Errors</div>
					{#if hasErrors(detailRow)}
						<div class="doc-proc-view-error-box">{detailRow.errors}</div>
					{:else}
						<div class="doc-proc-view-empty">No errors recorded for this entry.</div>
					{/if}
				</div>
			</div>
		</div>
	</div>
{/if}

<style>
	.doc-proc-view-dialog-overlay {
		position: fixed;
		inset: 0;
		z-index: 50;
		display: flex;
		align-items: center;
		justify-content: center;
		padding: 0.75rem;
		background: rgba(2, 6, 23, 0.72);
		backdrop-filter: blur(10px);
	}

	.doc-proc-view-dialog {
		display: flex;
		flex-direction: column;
		width: min(1046px, calc(100vw - 48px));
		max-height: calc(100vh - 48px);
		min-width: 480px;
		overflow: hidden;
		border-radius: 20px;
		border: 1px solid rgba(148, 163, 184, 0.16);
		background: #111827;
		color: #f3eedf;
	}

	.doc-proc-view-dialog-head {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 16px;
		padding: 16px 24px;
		border-bottom: 1px solid rgba(148, 163, 184, 0.12);
		flex-shrink: 0;
	}

	.doc-proc-view-dialog-title {
		margin: 0;
		font-size: 15px;
		font-weight: 600;
		color: #f3eedf;
		font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
	}

	.doc-proc-view-close-btn {
		height: 30px;
		padding: 0 12px;
		border-radius: 8px;
		border: 1px solid rgba(148, 163, 184, 0.16);
		background: rgba(255, 255, 255, 0.04);
		color: #9ca3af;
		font-size: 12px;
		cursor: pointer;
	}

	.doc-proc-view-dialog-body {
		flex: 1;
		min-height: 0;
		overflow-y: auto;
		padding: 16px 24px 24px;
		display: flex;
		flex-direction: column;
		gap: 16px;
		user-select: text;
	}

	.doc-proc-view-section-label {
		margin-bottom: 6px;
		font-size: 11px;
		font-weight: 600;
		letter-spacing: 0.1em;
		text-transform: uppercase;
		color: #9ca3af;
		font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
	}

	.doc-proc-view-rows-box {
		border: 1px solid rgba(148, 163, 184, 0.12);
		border-radius: 12px;
		background: rgba(148, 163, 184, 0.06);
		padding: 8px 0;
	}

	.doc-proc-view-row {
		display: grid;
		grid-template-columns: 180px minmax(0, 1fr);
		gap: 12px;
		padding: 5px 14px;
		align-items: start;
	}

	.doc-proc-view-key {
		color: #94a3b8;
		font-size: 12px;
		font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
		word-break: break-word;
	}

	.doc-proc-view-val {
		color: #e5e7eb;
		font-size: 12px;
		line-height: 1.5;
		word-break: break-word;
		user-select: text;
	}

	.doc-proc-view-val-mono {
		font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
	}

	.doc-proc-view-pre {
		margin: 0;
		padding: 12px 14px;
		border-radius: 12px;
		border: 1px solid rgba(148, 163, 184, 0.12);
		background: rgba(148, 163, 184, 0.06);
		color: #cbd5e1;
		font-size: 12px;
		line-height: 1.55;
		font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
		overflow: auto;
		white-space: pre-wrap;
		word-break: break-word;
		max-height: 320px;
		user-select: text;
	}

	.doc-proc-view-error-box,
	.doc-proc-view-empty {
		padding: 12px 14px;
		border-radius: 12px;
		font-size: 12px;
		line-height: 1.55;
	}

	.doc-proc-view-error-box {
		border: 1px solid rgba(248, 113, 113, 0.28);
		background: rgba(248, 113, 113, 0.12);
		color: #fca5a5;
		white-space: pre-wrap;
		word-break: break-word;
		user-select: text;
	}

	.doc-proc-view-empty {
		border: 1px solid rgba(148, 163, 184, 0.12);
		background: rgba(148, 163, 184, 0.06);
		color: #94a3b8;
	}

	@media (max-width: 720px) {
		.doc-proc-view-dialog {
			width: min(100vw - 24px, 1046px);
			min-width: 0;
		}

		.doc-proc-view-dialog-head {
			padding: 14px 16px;
		}

		.doc-proc-view-dialog-body {
			padding: 14px 16px 18px;
		}

		.doc-proc-view-row {
			grid-template-columns: 1fr;
			gap: 4px;
		}
	}
</style>
