<script lang="ts">
	import { onMount } from 'svelte';
	import RefreshCwIcon from '@lucide/svelte/icons/refresh-cw';
	import CircleAlertIcon from '@lucide/svelte/icons/circle-alert';
	import XIcon from '@lucide/svelte/icons/x';

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
	let overlay       = $derived(darkMode ? '#0D1117E6' : '#00000066');

	// --- Types ---
	type UsageEventRow = {
		id: string;
		account_id: string | null;
		account_name: string | null;
		profile_id: string | null;
		record_id: number | null;
		provider: string;
		model_name: string;
		prompt_name: string;
		call_reason: string;
		call_loc: string;
		request_started_at: string;
		input_tokens: number;
		output_tokens: number;
		total_tokens: number;
		prompt_cache_hit_tokens: number;
		prompt_cache_miss_tokens: number;
		latency_ms: number;
		error_message: string;
		input_body_ref: string;
		output_body_ref: string;
		metadata_json: Record<string, unknown> | null;
	};

	// --- Filter state ---
	let filterModel      = $state('');
	let filterPrompt     = $state('');
	let filterCallReason = $state('');
	let filterCallLoc    = $state('');
	let filterStartedFrom = $state('');
	let filterStartedTo   = $state('');
	let filterInTokMin   = $state('');
	let filterInTokMax   = $state('');
	let filterOutTokMin  = $state('');
	let filterOutTokMax  = $state('');
	let filterMetaKey    = $state('');
	let filterMetaValue  = $state('');

	function applyFilters() {
		page = 1;
		load();
	}

	function clearFilters() {
		filterModel = '';
		filterPrompt = '';
		filterCallReason = '';
		filterCallLoc = '';
		filterStartedFrom = '';
		filterStartedTo = '';
		filterInTokMin = '';
		filterInTokMax = '';
		filterOutTokMin = '';
		filterOutTokMax = '';
		filterMetaKey = '';
		filterMetaValue = '';
		page = 1;
		load();
	}

	// --- State ---
	let rows      = $state<UsageEventRow[]>([]);
	let total     = $state(0);
	let page      = $state(1);
	let pageSize  = $state(50);
	let loading   = $state(false);
	let error     = $state('');

	// Modal state for body viewer
	let modalVisible   = $state(false);
	let modalTitle     = $state('');
	let modalContent   = $state('');
	let modalLoading   = $state(false);
	let modalError     = $state('');

	let totalPages = $derived(Math.ceil(total / pageSize) || 1);

	// --- Data loading ---
	function toRFC3339(localDateTime: string): string {
		return new Date(localDateTime).toISOString();
	}

	async function load() {
		loading = true;
		error = '';
		try {
			const params = new URLSearchParams({ page: String(page), page_size: String(pageSize) });
			if (filterModel) params.set('model', filterModel);
			if (filterPrompt) params.set('prompt', filterPrompt);
			if (filterCallReason) params.set('call_reason', filterCallReason);
			if (filterCallLoc) params.set('call_loc', filterCallLoc);
			if (filterStartedFrom) params.set('started_from', toRFC3339(filterStartedFrom));
			if (filterStartedTo) params.set('started_to', toRFC3339(filterStartedTo));
			if (filterInTokMin) params.set('in_tok_min', filterInTokMin);
			if (filterInTokMax) params.set('in_tok_max', filterInTokMax);
			if (filterOutTokMin) params.set('out_tok_min', filterOutTokMin);
			if (filterOutTokMax) params.set('out_tok_max', filterOutTokMax);
			if (filterMetaKey && filterMetaValue) {
				params.set('meta_key', filterMetaKey);
				params.set('meta_value', filterMetaValue);
			}
			const res = await fetch(`/api/v1/llm/usage-events-admin?${params}`, { credentials: 'same-origin' });
			const data = await res.json();
			if (!res.ok) throw new Error(data.message ?? 'Failed to load');
			rows  = data.events ?? [];
			total = data.total  ?? 0;
		} catch (err) {
			error = err instanceof Error ? err.message : String(err);
			rows  = [];
			total = 0;
		} finally {
			loading = false;
		}
	}

	function prevPage() { if (page > 1) { page--; load(); } }
	function nextPage() { if (page < totalPages) { page++; load(); } }

	onMount(() => {
		load();
		const onSelChange = () => {
			const sel = window.getSelection();
			hasSelection = !!sel && sel.toString().trim().length > 0;
		};
		document.addEventListener('selectionchange', onSelChange);
		return () => document.removeEventListener('selectionchange', onSelChange);
	});

	// --- Body viewer ---
	async function openBody(row: UsageEventRow, type: 'input' | 'output') {
		const ref = type === 'input' ? row.input_body_ref : row.output_body_ref;
		if (!ref) return;
		modalVisible = true;
		modalLoading = true;
		modalError   = '';
		modalContent = '';
		modalTitle   = `${type === 'input' ? 'Input' : 'Output'} Body — ${row.id.slice(0, 12)}…`;
		try {
			const res = await fetch(`/api/v1/llm/usage-events/${row.id}/body?type=${type}`, { credentials: 'same-origin' });
			const text = await res.text();
			if (!res.ok) {
				const msg = (() => { try { return JSON.parse(text).message; } catch { return text; } })();
				throw new Error(msg ?? 'Failed to load body');
			}
			// Pretty-print JSON
			try { modalContent = JSON.stringify(JSON.parse(text), null, 2); }
			catch { modalContent = text; }
		} catch (err) {
			modalError = err instanceof Error ? err.message : String(err);
		} finally {
			modalLoading = false;
		}
	}

	function metadataText(row: UsageEventRow): string {
		try { return JSON.stringify(row.metadata_json ?? {}, null, 2); }
		catch { return '{}'; }
	}

	function openMetadata(row: UsageEventRow) {
		modalVisible = true;
		modalLoading = false;
		modalError   = '';
		modalTitle   = `Metadata — ${row.id.slice(0, 12)}…`;
		modalContent = metadataText(row);
	}

	function openDetails(row: UsageEventRow) {
		modalVisible = true;
		modalLoading = false;
		modalError   = '';
		modalTitle   = `Details — ${row.id.slice(0, 12)}…`;
		modalContent = JSON.stringify(row, null, 2);
	}

	let hasSelection     = $state(false);
	let showingSelection = $state(false);
	let selectionHtml    = $state('');
	let selectionError   = $state('');

	function closeModal() {
		modalVisible = false;
		showingSelection = false;
		selectionHtml = '';
		selectionError = '';
	}

	function showSelected() {
		const sel = window.getSelection();
		if (!sel) return;
		const text = sel.toString().trim();
		if (!text) return;
		selectionHtml = '';
		selectionError = '';
		try {
			const parsed = JSON.parse(text);
			selectionHtml = renderJsonHtml(parsed, 0);
		} catch (err) {
			const looksLikeJson = text.startsWith('{') || text.startsWith('[');
			selectionError = looksLikeJson
				? `Invalid JSON: ${err instanceof Error ? err.message : String(err)}`
				: 'Only JSON content can be rendered in name-value format.';
		}
		showingSelection = true;
	}

	function backToFull() {
		showingSelection = false;
		selectionHtml = '';
		selectionError = '';
	}

	// --- Helpers ---
	function formatTime(iso: string): string {
		if (!iso) return '—';
		return new Date(iso).toLocaleString();
	}

	function formatLatency(ms: number): string {
		if (ms < 1000) return `${ms}ms`;
		return `${(ms / 1000).toFixed(1)}s`;
	}

	function isEmbeddingCall(row: UsageEventRow): boolean {
		return row.prompt_name === 'embedding_no_prompt';
	}

	function escHtml(s: string): string {
		return s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
	}

	function renderJsonHtml(value: unknown, depth: number): string {
		const indent = depth * 16;

		if (value === null) return `<span style="color:${textMuted}; font-family:monospace; font-size:12px;">null</span>`;
		if (typeof value === 'boolean') return `<span style="color:${accent}; font-family:monospace; font-size:12px;">${value}</span>`;
		if (typeof value === 'number') return `<span style="color:${accent}; font-family:monospace; font-size:12px;">${value}</span>`;
		if (typeof value === 'string') {
			return `<span style="color:${textSecondary}; font-family:monospace; font-size:12px; word-break:break-word; white-space:pre-wrap; line-height:1.5;">${escHtml(value)}</span>`;
		}

		if (Array.isArray(value)) {
			if (value.length === 0) return `<span style="color:${textMuted}; font-size:12px;">[]</span>`;
			return value.map((item, i) => {
				const isComplex = item !== null && typeof item === 'object';
				if (isComplex) {
					return `<div style="padding-left:${indent + 16}px; margin-top:4px;">
						<div style="color:${textMuted}; font-size:11px; font-family:monospace; margin-bottom:2px;">[${i}]</div>
						${renderJsonHtml(item, depth + 1)}
					</div>`;
				}
				return `<div style="display:flex; gap:12px; padding-left:${indent + 16}px; padding-top:1px; padding-bottom:1px; align-items:flex-start;">
					<span style="color:${textMuted}; font-size:11px; font-family:monospace; flex-shrink:0;">[${i}]</span>
					${renderJsonHtml(item, depth + 1)}
				</div>`;
			}).join('');
		}

		if (typeof value === 'object' && value !== null) {
			const entries = Object.entries(value as Record<string, unknown>);
			if (entries.length === 0) return `<span style="color:${textMuted}; font-size:12px;">—</span>`;
			return entries.map(([k, v]) => {
				const isComplex = v !== null && typeof v === 'object';
				if (isComplex) {
					return `<div style="padding-left:${indent}px; margin-top:5px;">
						<div style="color:${textMuted}; font-size:12px; font-family:monospace; font-weight:500; margin-bottom:2px;">${escHtml(k)}</div>
						${renderJsonHtml(v, depth + 1)}
					</div>`;
				}
				return `<div style="display:flex; align-items:flex-start; gap:16px; padding-left:${indent}px; padding-top:2px; padding-bottom:2px;">
					<span style="color:${textMuted}; font-family:monospace; font-size:12px; min-width:130px; flex-shrink:0;">${escHtml(k)}</span>
					${renderJsonHtml(v, depth)}
				</div>`;
			}).join('');
		}

		return `<span style="color:${textSecondary}; font-size:12px;">${escHtml(String(value))}</span>`;
	}

	let modalHtml = $derived.by(() => {
		if (!modalContent) return '';
		try {
			const parsed = JSON.parse(modalContent);
			return renderJsonHtml(parsed, 0);
		} catch {
			return `<pre style="color:${textSecondary}; white-space:pre-wrap; word-break:break-word; font-family:monospace; font-size:12px; line-height:1.5;">${escHtml(modalContent)}</pre>`;
		}
	});
</script>

<div class="p-6 space-y-4 h-full flex flex-col overflow-hidden" style="background:{pageBg};">

	<!-- Header card -->
	<div class="rounded-xl p-5 flex-shrink-0" style="background:{cardBg}; border:1px solid {borderColor};">
		<div class="flex flex-wrap items-start justify-between gap-3">
			<div>
				<h2 style="font-size:18px; font-weight:600; color:{textPrimary};">LLM Usage Logs</h2>
				<p style="font-size:13px; color:{textSecondary}; margin-top:2px;">
					All LLM call events from <code style="font-size:12px; color:{accent};">public.llm_usage_event</code>, ordered by start time. Double-click a body ref to view the archived payload.
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
	</div>

	<!-- Filter section -->
	<div class="rounded-xl p-5 flex-shrink-0" style="background:{cardBg}; border:1px solid {borderColor};">
		<div class="grid gap-3" style="grid-template-columns:repeat(auto-fill, minmax(160px, 1fr));">
			<label class="flex flex-col gap-1">
				<span style="font-size:11px; color:{textMuted};">Model</span>
				<input type="text" bind:value={filterModel} placeholder="Contains…"
					class="rounded px-2 py-1.5 text-sm" style="background:{surface2}; color:{textPrimary}; border:1px solid {borderColor};" />
			</label>
			<label class="flex flex-col gap-1">
				<span style="font-size:11px; color:{textMuted};">Prompt</span>
				<input type="text" bind:value={filterPrompt} placeholder="Contains…"
					class="rounded px-2 py-1.5 text-sm" style="background:{surface2}; color:{textPrimary}; border:1px solid {borderColor};" />
			</label>
			<label class="flex flex-col gap-1">
				<span style="font-size:11px; color:{textMuted};">Call Reason</span>
				<input type="text" bind:value={filterCallReason} placeholder="Contains…"
					class="rounded px-2 py-1.5 text-sm" style="background:{surface2}; color:{textPrimary}; border:1px solid {borderColor};" />
			</label>
			<label class="flex flex-col gap-1">
				<span style="font-size:11px; color:{textMuted};">Call LOC</span>
				<input type="text" bind:value={filterCallLoc} placeholder="Contains…"
					class="rounded px-2 py-1.5 text-sm" style="background:{surface2}; color:{textPrimary}; border:1px solid {borderColor};" />
			</label>
			<label class="flex flex-col gap-1">
				<span style="font-size:11px; color:{textMuted};">Started From</span>
				<input type="datetime-local" bind:value={filterStartedFrom}
					class="rounded px-2 py-1.5 text-sm" style="background:{surface2}; color:{textPrimary}; border:1px solid {borderColor};" />
			</label>
			<label class="flex flex-col gap-1">
				<span style="font-size:11px; color:{textMuted};">Started To</span>
				<input type="datetime-local" bind:value={filterStartedTo}
					class="rounded px-2 py-1.5 text-sm" style="background:{surface2}; color:{textPrimary}; border:1px solid {borderColor};" />
			</label>
			<label class="flex flex-col gap-1">
				<span style="font-size:11px; color:{textMuted};">In Tok Min</span>
				<input type="number" bind:value={filterInTokMin}
					class="rounded px-2 py-1.5 text-sm" style="background:{surface2}; color:{textPrimary}; border:1px solid {borderColor};" />
			</label>
			<label class="flex flex-col gap-1">
				<span style="font-size:11px; color:{textMuted};">In Tok Max</span>
				<input type="number" bind:value={filterInTokMax}
					class="rounded px-2 py-1.5 text-sm" style="background:{surface2}; color:{textPrimary}; border:1px solid {borderColor};" />
			</label>
			<label class="flex flex-col gap-1">
				<span style="font-size:11px; color:{textMuted};">Out Tok Min</span>
				<input type="number" bind:value={filterOutTokMin}
					class="rounded px-2 py-1.5 text-sm" style="background:{surface2}; color:{textPrimary}; border:1px solid {borderColor};" />
			</label>
			<label class="flex flex-col gap-1">
				<span style="font-size:11px; color:{textMuted};">Out Tok Max</span>
				<input type="number" bind:value={filterOutTokMax}
					class="rounded px-2 py-1.5 text-sm" style="background:{surface2}; color:{textPrimary}; border:1px solid {borderColor};" />
			</label>
			<label class="flex flex-col gap-1">
				<span style="font-size:11px; color:{textMuted};">Metadata Key</span>
				<input type="text" bind:value={filterMetaKey} placeholder="e.g. capture_source"
					class="rounded px-2 py-1.5 text-sm" style="background:{surface2}; color:{textPrimary}; border:1px solid {borderColor};" />
			</label>
			<label class="flex flex-col gap-1">
				<span style="font-size:11px; color:{textMuted};">Metadata Value</span>
				<input type="text" bind:value={filterMetaValue} placeholder="Contains…"
					class="rounded px-2 py-1.5 text-sm" style="background:{surface2}; color:{textPrimary}; border:1px solid {borderColor};" />
			</label>
		</div>
		<div class="flex items-center gap-2 mt-4">
			<button
				onclick={applyFilters}
				disabled={loading}
				class="rounded-lg px-3 py-2 text-sm cursor-pointer"
				style="background:{accent}; color:white;"
			>Apply Filters</button>
			<button
				onclick={clearFilters}
				disabled={loading}
				class="rounded-lg px-3 py-2 text-sm cursor-pointer"
				style="background:{surface2}; color:{textPrimary}; border:1px solid {borderColor};"
			>Clear</button>
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

	<!-- Table card -->
	<div class="rounded-xl flex flex-col flex-1 min-h-0" style="background:{cardBg}; border:1px solid {borderColor};">
		<!-- Pagination bar -->
		<div class="px-5 py-3 flex items-center justify-between flex-shrink-0"
			style="border-bottom:1px solid {borderColor};">
			<span style="font-size:13px; color:{textMuted};">
				Total: {total} events
				{#if total > 0} &middot; page {page} of {totalPages}{/if}
			</span>
			<div class="flex items-center gap-2">
				<button
					onclick={prevPage}
					disabled={page <= 1 || loading}
					class="rounded px-3 py-1 text-sm cursor-pointer disabled:opacity-40"
					style="background:{surface2}; color:{textPrimary}; border:1px solid {borderColor};"
				>&lsaquo; Prev</button>
				<button
					onclick={nextPage}
					disabled={page >= totalPages || loading}
					class="rounded px-3 py-1 text-sm cursor-pointer disabled:opacity-40"
					style="background:{surface2}; color:{textPrimary}; border:1px solid {borderColor};"
				>Next &rsaquo;</button>
			</div>
		</div>

		{#if loading}
			<div class="px-5 py-8 text-center" style="color:{textMuted}; font-size:14px;">Loading…</div>
		{:else if rows.length === 0}
			<div class="px-5 py-8 text-center" style="color:{textMuted}; font-size:14px;">No events found.</div>
		{:else}
			<div class="flex-1 min-h-0 overflow-auto" style="user-select:text;">
				<table class="w-full text-sm" style="border-collapse:separate; border-spacing:0;">
					<thead>
						<tr style="background:{surface2};">
							<th class="text-left px-4 py-3 sticky top-0 z-10" style="color:{textMuted}; font-weight:500; white-space:nowrap; font-size:12px; background:{surface2}; border-bottom:1px solid {borderColor};">Started At</th>
							<th class="text-left px-4 py-3 sticky top-0 z-10" style="color:{textMuted}; font-weight:500; white-space:nowrap; font-size:12px; background:{surface2}; border-bottom:1px solid {borderColor};">Model</th>
							<th class="text-left px-4 py-3 sticky top-0 z-10" style="color:{textMuted}; font-weight:500; white-space:nowrap; font-size:12px; background:{surface2}; border-bottom:1px solid {borderColor};">Prompt</th>
							<th class="text-left px-4 py-3 sticky top-0 z-10" style="color:{textMuted}; font-weight:500; white-space:nowrap; font-size:12px; background:{surface2}; border-bottom:1px solid {borderColor};">Metadata</th>
							<th class="text-left px-4 py-3 sticky top-0 z-10" style="color:{textMuted}; font-weight:500; white-space:nowrap; font-size:12px; background:{surface2}; border-bottom:1px solid {borderColor};">Call Reason</th>
							<th class="text-left px-4 py-3 sticky top-0 z-10" style="color:{textMuted}; font-weight:500; white-space:nowrap; font-size:12px; background:{surface2}; border-bottom:1px solid {borderColor};">Call LOC</th>
							<th class="text-right px-4 py-3 sticky top-0 z-10" style="color:{textMuted}; font-weight:500; white-space:nowrap; font-size:12px; background:{surface2}; border-bottom:1px solid {borderColor};">In Tok</th>
							<th class="text-right px-4 py-3 sticky top-0 z-10" style="color:{textMuted}; font-weight:500; white-space:nowrap; font-size:12px; background:{surface2}; border-bottom:1px solid {borderColor};">Out Tok</th>
							<th class="text-right px-4 py-3 sticky top-0 z-10" style="color:{textMuted}; font-weight:500; white-space:nowrap; font-size:12px; background:{surface2}; border-bottom:1px solid {borderColor};">Cache Hit</th>
							<th class="text-right px-4 py-3 sticky top-0 z-10" style="color:{textMuted}; font-weight:500; white-space:nowrap; font-size:12px; background:{surface2}; border-bottom:1px solid {borderColor};">Cache Miss</th>
							<th class="text-right px-4 py-3 sticky top-0 z-10" style="color:{textMuted}; font-weight:500; white-space:nowrap; font-size:12px; background:{surface2}; border-bottom:1px solid {borderColor};">Latency</th>
							<th class="text-left px-4 py-3 sticky top-0 z-10" style="color:{textMuted}; font-weight:500; white-space:nowrap; font-size:12px; background:{surface2}; border-bottom:1px solid {borderColor};">Error</th>
							<th class="text-left px-4 py-3 sticky top-0 z-10" style="color:{textMuted}; font-weight:500; white-space:nowrap; font-size:12px; background:{surface2}; border-bottom:1px solid {borderColor};">Input Body</th>
							<th class="text-left px-4 py-3 sticky top-0 z-10" style="color:{textMuted}; font-weight:500; white-space:nowrap; font-size:12px; background:{surface2}; border-bottom:1px solid {borderColor};">Output Body</th>
							<th class="text-left px-4 py-3 sticky top-0 z-10" style="color:{textMuted}; font-weight:500; white-space:nowrap; font-size:12px; background:{surface2}; border-bottom:1px solid {borderColor};">Details</th>
						</tr>
					</thead>
					<tbody>
						{#each rows as row (row.id)}
							<tr class="transition-colors hover:bg-white/5">
								<td class="px-4 py-2.5" style="border-bottom:1px solid {borderColor}; color:{textMuted}; white-space:nowrap; font-size:12px;">{formatTime(row.request_started_at)}</td>
								<td class="px-4 py-2.5" style="border-bottom:1px solid {borderColor}; color:{textSecondary}; white-space:nowrap; font-size:13px;">{row.model_name || '—'}</td>
								<td class="px-4 py-2.5" style="border-bottom:1px solid {borderColor}; color:{textSecondary}; max-width:220px; overflow:hidden; text-overflow:ellipsis; white-space:nowrap; font-size:12px;"
									title={row.prompt_name}>{row.prompt_name || '—'}</td>

								<!-- Metadata -->
								<td class="px-4 py-2.5" style="border-bottom:1px solid {borderColor};">
									<button
										onclick={() => openMetadata(row)}
										class="text-left rounded px-2 py-1 cursor-pointer"
										style="background:{surface2}; color:{accent}; border:1px solid {borderColor}; font-size:11px; font-family:monospace;"
										title={metadataText(row)}
									>{'{ }'}</button>
								</td>

								<td class="px-4 py-2.5" style="border-bottom:1px solid {borderColor}; color:{textSecondary}; white-space:nowrap; font-size:12px;">{row.call_reason || '—'}</td>
								<td class="px-4 py-2.5" style="border-bottom:1px solid {borderColor}; color:{textSecondary}; white-space:nowrap; font-size:12px;">{row.call_loc || '—'}</td>

								<td class="px-4 py-2.5 text-right" style="border-bottom:1px solid {borderColor}; color:{textSecondary}; font-size:12px; font-variant-numeric:tabular-nums;">{row.input_tokens.toLocaleString()}</td>
								<td class="px-4 py-2.5 text-right" style="border-bottom:1px solid {borderColor}; color:{textSecondary}; font-size:12px; font-variant-numeric:tabular-nums;">{row.output_tokens.toLocaleString()}</td>
								<td class="px-4 py-2.5 text-right" style="border-bottom:1px solid {borderColor}; color:{textSecondary}; font-size:12px; font-variant-numeric:tabular-nums;">{row.prompt_cache_hit_tokens.toLocaleString()}</td>
								<td class="px-4 py-2.5 text-right" style="border-bottom:1px solid {borderColor}; color:{textSecondary}; font-size:12px; font-variant-numeric:tabular-nums;">{row.prompt_cache_miss_tokens.toLocaleString()}</td>
								<td class="px-4 py-2.5 text-right" style="border-bottom:1px solid {borderColor}; color:{textMuted}; font-size:12px; white-space:nowrap;">{formatLatency(row.latency_ms)}</td>

								<td class="px-4 py-2.5" style="border-bottom:1px solid {borderColor}; max-width:200px;">
									{#if row.error_message}
										<span style="color:{danger}; font-size:12px; overflow:hidden; text-overflow:ellipsis; white-space:nowrap; display:block;"
											title={row.error_message}>{row.error_message}</span>
									{:else}
										<span style="color:{success}; font-size:12px;">OK</span>
									{/if}
								</td>

								<!-- Input Body -->
								<td class="px-4 py-2.5" style="border-bottom:1px solid {borderColor};">
									{#if row.input_body_ref}
										<button
											onclick={() => openBody(row, 'input')}
											class="rounded px-2 py-1 text-xs cursor-pointer"
											style="background:{surface2}; color:{accent}; border:1px solid {borderColor};"
											title={row.input_body_ref}
										>View</button>
									{:else}
										<span style="color:{textMuted}; font-size:12px;">—</span>
									{/if}
								</td>

								<!-- Output Body -->
								<td class="px-4 py-2.5" style="border-bottom:1px solid {borderColor};">
									{#if row.output_body_ref}
										<button
											onclick={() => openBody(row, 'output')}
											disabled={isEmbeddingCall(row)}
											class="rounded px-2 py-1 text-xs cursor-pointer disabled:cursor-not-allowed disabled:opacity-40"
											style="background:{surface2}; color:{accent}; border:1px solid {borderColor};"
											title={isEmbeddingCall(row) ? 'Not applicable for embedding calls' : row.output_body_ref}
										>View</button>
									{:else}
										<span style="color:{textMuted}; font-size:12px;">—</span>
									{/if}
								</td>

								<!-- Details -->
								<td class="px-4 py-2.5" style="border-bottom:1px solid {borderColor};">
									<button
										onclick={() => openDetails(row)}
										class="rounded px-2 py-1 text-xs cursor-pointer"
										style="background:{surface2}; color:{accent}; border:1px solid {borderColor};"
									>Details</button>
								</td>
							</tr>
						{/each}
					</tbody>
				</table>
			</div>
		{/if}
	</div>
</div>

<!-- Body viewer modal -->
{#if modalVisible}
	<!-- svelte-ignore a11y_click_events_have_key_events a11y_no_static_element_interactions -->
	<div
		class="fixed inset-0 z-50 flex items-center justify-center p-6"
		style="background:{overlay};"
		onclick={(e) => { if (e.target === e.currentTarget) closeModal(); }}
	>
		<div class="rounded-xl flex flex-col" style="background:{cardBg}; border:1px solid {borderColor}; width:min(900px,100%); max-height:80vh;">
			<!-- Modal header -->
			<div class="flex items-center justify-between px-5 py-4" style="border-bottom:1px solid {borderColor};">
				<span style="font-size:14px; font-weight:600; color:{textPrimary}; font-family:monospace;">{modalTitle}</span>
				<div class="flex items-center gap-2">
					{#if showingSelection}
						<button
							onclick={backToFull}
							class="inline-flex items-center rounded px-3 py-1.5 text-xs cursor-pointer"
							style="background:{surface2}; color:{textSecondary}; border:1px solid {borderColor};"
						>← Full</button>
					{:else}
						<button
							onclick={showSelected}
							disabled={!hasSelection}
							class="inline-flex items-center rounded px-3 py-1.5 text-xs cursor-pointer disabled:opacity-40 disabled:cursor-default"
							style="background:{surface2}; color:{accent}; border:1px solid {borderColor};"
						>Show Selected</button>
					{/if}
					<button
						onclick={closeModal}
						class="rounded p-1.5 cursor-pointer"
						style="background:{surface2}; color:{textMuted}; border:1px solid {borderColor};"
						aria-label="Close"
					><XIcon class="w-4 h-4" /></button>
				</div>
			</div>

			<!-- Modal body -->
			<div class="flex-1 overflow-auto p-5">
				{#if modalLoading}
					<div style="color:{textMuted}; font-size:14px; text-align:center; padding:2rem;">Loading…</div>
				{:else if modalError}
					<div class="rounded-lg p-4 flex items-start gap-2"
						style="background:{danger}15; border:1px solid {danger}40; color:{danger};">
						<CircleAlertIcon class="w-4 h-4 mt-0.5 flex-shrink-0" />
						<span style="font-size:13px;">{modalError}</span>
					</div>
				{:else if showingSelection}
					{#if selectionError}
						<div class="rounded-lg p-4 flex items-start gap-2"
							style="background:{danger}15; border:1px solid {danger}40; color:{danger};">
							<CircleAlertIcon class="w-4 h-4 mt-0.5 flex-shrink-0" />
							<span style="font-size:13px;">{selectionError}</span>
						</div>
					{:else}
						<div class="text-xs" style="line-height:1.6; user-select:text;">
							{@html selectionHtml}
						</div>
					{/if}
				{:else}
					<div class="text-xs" style="line-height:1.6; user-select:text;">
						{@html modalHtml}
					</div>
				{/if}
			</div>
		</div>
	</div>
{/if}
