<script lang="ts">
	import { m } from '$lib/paraglide/messages.js';
	import { onMount } from 'svelte';
	import RefreshCwIcon from '@lucide/svelte/icons/refresh-cw';
	import CircleAlertIcon from '@lucide/svelte/icons/circle-alert';

	let { darkMode = true }: { darkMode: boolean } = $props();
	let pageBg = $derived(darkMode ? '#171B26' : '#F2F4F7');
	let cardBg = $derived(darkMode ? '#1F2333' : '#FFFFFF');
	let surface2 = $derived(darkMode ? '#252A3A' : '#ECEEF2');
	let borderColor = $derived(darkMode ? '#2D3348' : '#E4E6EB');
	let accent = $derived(darkMode ? '#818CF8' : '#6366F1');
	let textPrimary = $derived(darkMode ? '#E2E8F0' : '#111827');
	let textSecondary = $derived(darkMode ? '#94A3B8' : '#6B7280');
	let textMuted = $derived(darkMode ? '#64748B' : '#9CA3AF');
	let danger = $derived(darkMode ? '#F87171' : '#DC2626');
	let warning = $derived(darkMode ? '#FBBF24' : '#D97706');
	let success = $derived(darkMode ? '#34D399' : '#059669');

	type AlertRow = {
		id: number; level: string; message: string; req_id: string; loc: string; call_flow: string;
		attrs: string; created_at: string; status: string; owner: string | null;
		resolution: string | null; resolver: string | null; resolve_time: string | null; comment_count: number;
	};
	type Op = 'eq' | 'ne' | 'gt' | 'lt';
	type ColKey = 'created_at' | 'level' | 'message' | 'loc' | 'status' | 'owner';
	type Filter = { col: ColKey; op: Op; value: string };
	type Sort = { col: ColKey; dir: 'asc' | 'desc' };
	const ops: { op: Op; sym: string }[] = [
		{ op: 'eq', sym: '=' }, { op: 'ne', sym: '<>' }, { op: 'gt', sym: '>' }, { op: 'lt', sym: '<' }
	];
	const columns = $derived<{ key: ColKey; label: string }[]>([
		{ key: 'created_at', label: m.alert_logs_time() }, { key: 'level', label: m.alert_logs_level() },
		{ key: 'message', label: m.alert_logs_message() }, { key: 'loc', label: m.alert_logs_location() },
		{ key: 'status', label: m.alert_logs_status() }, { key: 'owner', label: m.alert_logs_owner() }
	]);
	const SAVED_KEY = 'alertLogsView.v1';
	const API = '/api/v1/system-admin/alert-logs';

	let rows = $state<AlertRow[]>([]), total = $state(0), page = $state(1), pageSize = 50;
	let loading = $state(false), error = $state('');
	let sort = $state<Sort | null>(null), filters = $state<Filter[]>([]);
	let menu = $state<{ col: ColKey; x: number; y: number } | null>(null);
	let customOp = $state<Op | null>(null), customValue = $state(''), clipboardText = $state('');
	let selected = $state<{ id: number; col: ColKey } | null>(null);
	let commentsId = $state<number | null>(null), comments = $state<{ id: number; author: string; body: string; created_at: string }[]>([]);
	let commentDraft = $state(''), commentError = $state('');
	let ownerDraft = $state<Record<number, string>>({});
	let expandedId = $state<number | null>(null);
	let resolvingId = $state<number | null>(null), resolutionDraft = $state(''), resolveError = $state('');
	let totalPages = $derived(Math.max(1, Math.ceil(total / pageSize)));

	function formatTime(value: string | null) {
		if (!value) return '—';
		const parsed = new Date(value);
		return Number.isNaN(parsed.getTime()) ? value : parsed.toLocaleString();
	}
	async function call(url: string, init?: RequestInit) {
		const response = await fetch(url, { credentials: 'same-origin', ...init });
		const text = await response.text();
		let data: Record<string, unknown> = {};
		try { data = JSON.parse(text); } catch { /* handled below */ }
		if (!response.ok || !data.status) throw new Error(String(data.error_msg ?? text ?? ''));
		return data;
	}
	async function load() {
		loading = true; error = '';
		try {
			const params = new URLSearchParams({ page: String(page), page_size: String(pageSize) });
			if (sort) params.set('sort', `${sort.col}|${sort.dir}`);
			for (const f of filters) params.append('filter', `${f.col}|${f.op}|${f.value}`);
			const data = await call(`${API}?${params}`);
			rows = Array.isArray(data.results) ? (data.results as AlertRow[]) : [];
			total = typeof data.total === 'number' ? data.total : 0;
			ownerDraft = Object.fromEntries(rows.map((r) => [r.id, r.owner ?? '']));
		} catch (err) {
			error = (err instanceof Error && err.message) || m.alert_logs_failed_to_load();
			rows = []; total = 0;
		} finally { loading = false; }
	}
	function applyFilters() { page = 1; load(); }
	function cellValue(row: AlertRow, col: ColKey): string { return row[col] ?? ''; }
	function selectedValue(col: ColKey): string {
		if (!selected || selected.col !== col) return '';
		const row = rows.find((r) => r.id === selected!.id);
		return row ? cellValue(row, col) : '';
	}
	async function openMenu(event: MouseEvent, col: ColKey) {
		const rect = (event.currentTarget as HTMLElement).getBoundingClientRect();
		customOp = null; customValue = ''; clipboardText = '';
		menu = { col, x: Math.min(rect.left, window.innerWidth - 340), y: rect.bottom + 4 };
		try { clipboardText = (await navigator.clipboard.readText()).trim(); } catch { /* permission denied or unsupported */ }
	}
	function closeMenu() { menu = null; customOp = null; }
	// Choosing the direction that is already active turns the ordering off.
	function orderBy(col: ColKey, dir: 'asc' | 'desc') {
		sort = sort?.col === col && sort.dir === dir ? null : { col, dir };
		closeMenu(); applyFilters();
	}
	function disableOrder() { sort = null; closeMenu(); applyFilters(); }
	function addFilter(col: ColKey, op: Op, value: string) {
		closeMenu();
		filters = [...filters, { col, op, value }];
		applyFilters();
	}
	function removeFilter(index: number) { filters = filters.filter((_, i) => i !== index); applyFilters(); }
	function clearAll() { sort = null; filters = []; closeMenu(); applyFilters(); }
	function saveSettings() {
		try { localStorage.setItem(SAVED_KEY, JSON.stringify({ sort, filters })); } catch { /* storage unavailable */ }
		closeMenu();
	}
	function loadSettings() {
		try {
			const saved = JSON.parse(localStorage.getItem(SAVED_KEY) ?? 'null');
			if (saved) { sort = saved.sort ?? null; filters = Array.isArray(saved.filters) ? saved.filters : []; }
		} catch { /* ignore unreadable saved settings */ }
	}
	function opSym(op: Op) { return ops.find((o) => o.op === op)?.sym ?? '='; }
	function shorten(text: string) { return text.length > 40 ? `${text.slice(0, 39)}…` : text; }
	async function toggleComments(row: AlertRow) {
		if (commentsId === row.id) { commentsId = null; return; }
		commentsId = row.id; commentDraft = ''; commentError = ''; comments = [];
		await loadComments(row.id);
	}
	async function loadComments(id: number) {
		try {
			const data = await call(`${API}/${id}/comments`);
			comments = Array.isArray(data.results) ? (data.results as typeof comments) : [];
		} catch (err) { commentError = (err instanceof Error && err.message) || m.alert_logs_failed_to_load(); }
	}
	async function addComment(row: AlertRow) {
		if (!commentDraft.trim()) return;
		try {
			await call(`${API}/${row.id}/comments`, {
				method: 'POST', headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({ body: commentDraft.trim() })
			});
			commentDraft = ''; commentError = '';
			row.comment_count += 1;
			await loadComments(row.id);
		} catch (err) { commentError = (err instanceof Error && err.message) || m.alert_logs_failed_to_save(); }
	}
	async function assign(row: AlertRow) {
		error = '';
		try {
			const owner = (ownerDraft[row.id] ?? '').trim();
			await call(`${API}/${row.id}/assign`, {
				method: 'PUT', headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({ owner: owner || null })
			});
			row.owner = owner || null;
		} catch (err) { error = (err instanceof Error && err.message) || m.alert_logs_failed_to_save(); }
	}
	function startResolve(row: AlertRow) { resolvingId = row.id; resolutionDraft = ''; resolveError = ''; }
	async function saveResolve(row: AlertRow) {
		if (!resolutionDraft.trim()) { resolveError = m.alert_logs_resolution_required(); return; }
		try {
			await call(`${API}/${row.id}/resolve`, {
				method: 'PUT', headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({ resolution: resolutionDraft.trim() })
			});
			resolvingId = null;
			await load();
		} catch (err) { resolveError = (err instanceof Error && err.message) || m.alert_logs_failed_to_save(); }
	}
	function levelColor(level: string) { return level === 'ERROR' ? danger : warning; }
	onMount(() => { loadSettings(); load(); });
</script>

<div class="p-6 space-y-4 h-full flex flex-col overflow-hidden select-text" style="background:{pageBg}">
	<div class="rounded-xl p-5 flex-shrink-0" style="background:{cardBg};border:1px solid {borderColor}">
		<div class="flex flex-wrap items-start justify-between gap-3">
			<div>
				<h2 style="font-size:18px;font-weight:600;color:{textPrimary}">{m.alert_logs_title()}</h2>
				<p style="font-size:13px;color:{textSecondary};margin-top:2px">{m.alert_logs_subtitle()}</p>
			</div>
			<div class="flex flex-wrap items-center gap-2">
				<button onclick={load} disabled={loading} class="inline-flex items-center gap-2 rounded-lg px-3 py-2 cursor-pointer" style="background:{surface2};color:{textPrimary};border:1px solid {borderColor}">
					<RefreshCwIcon class="w-4 h-4 {loading ? 'animate-spin' : ''}" />{m.alert_logs_refresh()}
				</button>
			</div>
		</div>
	</div>
	{#if error}
		<div class="rounded-xl p-4 flex gap-2 flex-shrink-0" style="background:{danger}20;border:1px solid {danger}70;color:{danger}">
			<CircleAlertIcon class="w-4 h-4" /><span style="font-size:13px">{error}</span>
		</div>
	{/if}
	{#if sort || filters.length}
		<div class="flex flex-wrap items-center gap-2 flex-shrink-0" style="font-size:12px">
			{#if sort}
				<span class="rounded px-2 py-1" style="background:{surface2};color:{textPrimary};border:1px solid {borderColor}">{sort.dir === 'asc' ? '↑' : '↓'} {sort.col}</span>
			{/if}
			{#each filters as f, i (i)}
				<span class="inline-flex items-center gap-1 rounded px-2 py-1" style="background:{surface2};color:{textPrimary};border:1px solid {borderColor}">
					{f.col} {opSym(f.op)} '{f.value}'
					<button onclick={() => removeFilter(i)} class="cursor-pointer" aria-label={m.alert_logs_remove_filter()} style="background:none;border:none;color:{textMuted}">×</button>
				</span>
			{/each}
		</div>
	{/if}
	<div class="rounded-xl flex flex-col flex-1 min-h-0" style="background:{cardBg};border:1px solid {borderColor}">
		<div class="px-5 py-3 flex justify-between flex-shrink-0" style="border-bottom:1px solid {borderColor}">
			<span style="font-size:13px;color:{textMuted}">{m.alert_logs_total({ total })}{#if total} · {m.alert_logs_page_of({ page, totalPages })}{/if}</span>
			<div class="flex gap-2">
				<button onclick={() => { if (page > 1) { page--; load(); } }} disabled={page <= 1 || loading} class="rounded px-3 py-1 text-sm disabled:opacity-40" style="background:{surface2};color:{textPrimary};border:1px solid {borderColor}">{m.alert_logs_prev()}</button>
				<button onclick={() => { if (page < totalPages) { page++; load(); } }} disabled={page >= totalPages || loading} class="rounded px-3 py-1 text-sm disabled:opacity-40" style="background:{surface2};color:{textPrimary};border:1px solid {borderColor}">{m.alert_logs_next()}</button>
			</div>
		</div>
		{#if loading}
			<div class="px-5 py-8 text-center" style="color:{textMuted}">{m.alert_logs_loading()}</div>
		{:else if !rows.length}
			<div class="px-5 py-8 text-center" style="color:{textMuted}">{m.alert_logs_empty()}</div>
		{:else}
			<div class="flex-1 min-h-0 overflow-auto">
				<table class="w-full text-sm" style="border-collapse:separate;border-spacing:0">
					<thead>
						<tr style="background:{surface2}">
							{#each columns as col (col.key)}
								<th class="text-left px-4 py-3 sticky top-0 z-10" style="color:{textMuted};font-weight:500;white-space:nowrap;font-size:12px;background:{surface2};border-bottom:1px solid {borderColor}">
									<span class="inline-flex items-center gap-1">
										{col.label}
										{#if sort?.col === col.key}<button onclick={disableOrder} title={m.alert_logs_disable_order({ column: col.key })} aria-label={m.alert_logs_disable_order({ column: col.key })} class="cursor-pointer" style="background:none;border:none;color:{accent}">{sort.dir === 'asc' ? '↑' : '↓'}</button>{/if}
										{#if filters.some((f) => f.col === col.key)}<span style="color:{accent}">●</span>{/if}
										<button onclick={(e) => openMenu(e, col.key)} aria-label={m.alert_logs_column_menu({ column: col.label })} class="rounded px-1 cursor-pointer" style="background:none;border:none;color:{accent}">▾</button>
									</span>
								</th>
							{/each}
							<th class="sticky top-0 z-10" style="background:{surface2};border-bottom:1px solid {borderColor}"></th>
						</tr>
					</thead>
					<tbody>
						{#each rows as row (row.id)}
							<tr class="hover:bg-white/5">
								<td onclick={() => (selected = { id: row.id, col: 'created_at' })} class="px-4 py-2.5" style="border-bottom:1px solid {borderColor};color:{textMuted};white-space:nowrap;font-size:12px;{selected?.id === row.id && selected.col === 'created_at' ? `outline:1px solid ${accent};outline-offset:-1px` : ''}">{formatTime(row.created_at)}</td>
								<td onclick={() => (selected = { id: row.id, col: 'level' })} class="px-4 py-2.5" style="border-bottom:1px solid {borderColor};color:{levelColor(row.level)};font-weight:600;{selected?.id === row.id && selected.col === 'level' ? `outline:1px solid ${accent};outline-offset:-1px` : ''}">{row.level}</td>
								<td onclick={() => (selected = { id: row.id, col: 'message' })} class="px-4 py-2.5" style="border-bottom:1px solid {borderColor};max-width:420px;{selected?.id === row.id && selected.col === 'message' ? `outline:1px solid ${accent};outline-offset:-1px` : ''}">
									<button onclick={() => (expandedId = expandedId === row.id ? null : row.id)} class="text-left cursor-pointer" title={m.alert_logs_details()} style="color:{textPrimary};background:none;border:none;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;max-width:100%;display:block">{row.message}</button>
								</td>
								<td onclick={() => (selected = { id: row.id, col: 'loc' })} class="px-4 py-2.5" style="border-bottom:1px solid {borderColor};color:{textSecondary};font-family:monospace;font-size:12px;{selected?.id === row.id && selected.col === 'loc' ? `outline:1px solid ${accent};outline-offset:-1px` : ''}">{row.loc || '—'}</td>
								<td onclick={() => (selected = { id: row.id, col: 'status' })} class="px-4 py-2.5" style="border-bottom:1px solid {borderColor};color:{row.status === 'resolved' ? success : textSecondary};{selected?.id === row.id && selected.col === 'status' ? `outline:1px solid ${accent};outline-offset:-1px` : ''}">{row.status === 'resolved' ? m.alert_logs_status_resolved() : m.alert_logs_status_open()}</td>
								<td onclick={() => (selected = { id: row.id, col: 'owner' })} class="px-4 py-2.5" style="border-bottom:1px solid {borderColor};{selected?.id === row.id && selected.col === 'owner' ? `outline:1px solid ${accent};outline-offset:-1px` : ''}">
									<div class="flex gap-1">
										<input bind:value={ownerDraft[row.id]} placeholder={m.alert_logs_unassigned()} aria-label={m.alert_logs_owner_placeholder()} class="rounded px-2 py-1 text-sm" style="width:180px;background:{surface2};color:{textPrimary};border:1px solid {borderColor}" />
										<button onclick={() => assign(row)} disabled={(ownerDraft[row.id] ?? '').trim() === (row.owner ?? '')} class="rounded px-2 py-1 text-xs cursor-pointer disabled:opacity-40" style="background:{surface2};color:{accent};border:1px solid {borderColor}">{m.alert_logs_assign()}</button>
									</div>
								</td>
								<td class="px-4 py-2.5" style="border-bottom:1px solid {borderColor}">
									<div class="flex gap-1">
										<button onclick={() => toggleComments(row)} class="rounded px-2 py-1 text-xs cursor-pointer" style="background:{surface2};color:{accent};border:1px solid {borderColor}">{m.alert_logs_comment({ count: row.comment_count })}</button>
										{#if row.status !== 'resolved'}
											<button onclick={() => startResolve(row)} class="rounded px-2 py-1 text-xs cursor-pointer" style="background:{accent};color:white">{m.alert_logs_resolve()}</button>
										{/if}
									</div>
								</td>
							</tr>
							{#if expandedId === row.id || resolvingId === row.id || commentsId === row.id || row.status === 'resolved'}
								<tr>
									<td colspan="7" class="px-4 py-3" style="border-bottom:1px solid {borderColor};background:{surface2}">
										{#if row.status === 'resolved'}
											<div style="font-size:12px;color:{success}">{m.alert_logs_resolved_by({ resolver: row.resolver ?? '—', time: formatTime(row.resolve_time) })}</div>
											<div style="font-size:13px;color:{textSecondary};white-space:pre-wrap;margin-top:2px">{row.resolution}</div>
										{/if}
										{#if resolvingId === row.id}
											<textarea bind:value={resolutionDraft} rows="3" placeholder={m.alert_logs_resolution_placeholder()} aria-label={m.alert_logs_resolution()} class="w-full rounded px-2 py-1.5 text-sm" style="background:{cardBg};color:{textPrimary};border:1px solid {borderColor}"></textarea>
											{#if resolveError}<div style="font-size:12px;color:{danger};margin-top:4px">{resolveError}</div>{/if}
											<div class="flex gap-2 mt-2">
												<button onclick={() => saveResolve(row)} class="rounded px-3 py-1 text-sm cursor-pointer" style="background:{accent};color:white">{m.alert_logs_save()}</button>
												<button onclick={() => (resolvingId = null)} class="rounded px-3 py-1 text-sm cursor-pointer" style="background:{cardBg};color:{textPrimary};border:1px solid {borderColor}">{m.alert_logs_cancel()}</button>
											</div>
										{/if}
										{#if commentsId === row.id}
											<div class="space-y-2" style="margin-top:8px">
												{#each comments as cm (cm.id)}
													<div style="font-size:13px;color:{textSecondary}"><span style="color:{textMuted};font-size:12px">{cm.author} · {formatTime(cm.created_at)}</span><div style="white-space:pre-wrap">{cm.body}</div></div>
												{:else}
													<div style="font-size:12px;color:{textMuted}">{m.alert_logs_no_comments()}</div>
												{/each}
												<textarea bind:value={commentDraft} rows="2" placeholder={m.alert_logs_comment_placeholder()} aria-label={m.alert_logs_comment_placeholder()} class="w-full rounded px-2 py-1.5 text-sm" style="background:{cardBg};color:{textPrimary};border:1px solid {borderColor}"></textarea>
												{#if commentError}<div style="font-size:12px;color:{danger}">{commentError}</div>{/if}
												<button onclick={() => addComment(row)} disabled={!commentDraft.trim()} class="rounded px-3 py-1 text-sm cursor-pointer disabled:opacity-40" style="background:{accent};color:white">{m.alert_logs_add_comment()}</button>
											</div>
										{/if}
										{#if expandedId === row.id}
											<pre style="font-size:12px;color:{textSecondary};white-space:pre-wrap;word-break:break-word;margin-top:8px">{row.message}
{row.req_id}{row.call_flow}
{row.attrs}</pre>
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
</div>

{#if menu}
	{@const col = menu.col}
	{@const cv = selectedValue(col)}
	<div class="fixed inset-0 z-40" role="presentation" onclick={closeMenu} onkeydown={(e) => e.key === 'Escape' && closeMenu()}></div>
	<div class="fixed z-50 rounded-xl py-2 select-text" role="menu" style="left:{menu.x}px;top:{menu.y}px;width:320px;max-height:80vh;overflow:auto;background:{cardBg};border:1px solid {borderColor};box-shadow:0 8px 28px #0006;font-size:13px;color:{textPrimary}">
		{#each ['asc', 'desc'] as const as dir (dir)}
			<button role="menuitem" onclick={() => orderBy(col, dir)} class="w-full text-left px-4 py-1.5 cursor-pointer hover:bg-white/5" style="background:none;border:none;color:inherit">
				<span style="display:inline-block;width:1.25em">{sort?.col === col && sort.dir === dir ? '✓' : ''}</span>{dir === 'asc' ? '↑' : '↓'} {dir === 'asc' ? m.alert_logs_order_asc({ column: col }) : m.alert_logs_order_desc({ column: col })}
			</button>
		{/each}
		{#if sort?.col === col}
			<button role="menuitem" onclick={disableOrder} class="w-full text-left px-4 py-1.5 cursor-pointer hover:bg-white/5" style="background:none;border:none;color:inherit"><span style="display:inline-block;width:1.25em"></span>{m.alert_logs_disable_order({ column: col })}</button>
		{/if}
		{#if cv}
			<div class="px-4 pt-2 pb-1" style="color:{textMuted};font-size:12px;border-top:1px solid {borderColor};margin-top:6px">{m.alert_logs_cell_value()}</div>
			{#each ops as o (o.op)}
				<button role="menuitem" onclick={() => addFilter(col, o.op, cv)} class="w-full text-left px-4 py-1.5 cursor-pointer hover:bg-white/5" style="background:none;border:none;color:inherit">{col} {o.sym} '{shorten(cv)}'</button>
			{/each}
		{/if}
		<div class="px-4 pt-2 pb-1" style="color:{textMuted};font-size:12px;border-top:1px solid {borderColor};margin-top:6px">{m.alert_logs_custom()}</div>
		{#each ops as o (o.op)}
			{#if customOp === o.op}
				<form class="flex items-center gap-1 px-4 py-1" onsubmit={(e) => { e.preventDefault(); if (customValue.trim()) addFilter(col, o.op, customValue.trim()); }}>
					<span>{col} {o.sym}</span>
					<!-- svelte-ignore a11y_autofocus -->
					<input autofocus bind:value={customValue} aria-label={m.alert_logs_custom_value()} class="flex-1 min-w-0 rounded px-2 py-1" style="background:{surface2};color:{textPrimary};border:1px solid {borderColor}" />
				</form>
			{:else}
				<button role="menuitem" onclick={() => { customOp = o.op; customValue = ''; }} class="w-full text-left px-4 py-1.5 cursor-pointer hover:bg-white/5" style="background:none;border:none;color:inherit">{col} {o.sym} ..</button>
			{/if}
		{/each}
		{#if clipboardText}
			<div class="px-4 pt-2 pb-1" style="color:{textMuted};font-size:12px;border-top:1px solid {borderColor};margin-top:6px">{m.alert_logs_clipboard()}</div>
			{#each ops as o (o.op)}
				<button role="menuitem" onclick={() => addFilter(col, o.op, clipboardText)} class="w-full text-left px-4 py-1.5 cursor-pointer hover:bg-white/5" style="background:none;border:none;color:inherit">{col} {o.sym} '{shorten(clipboardText)}'</button>
			{/each}
		{/if}
		<div style="border-top:1px solid {borderColor};margin-top:6px;padding-top:6px">
			<button role="menuitem" onclick={saveSettings} class="w-full text-left px-4 py-1.5 cursor-pointer hover:bg-white/5" style="background:none;border:none;color:inherit">{m.alert_logs_save_filters()}</button>
			<button role="menuitem" onclick={clearAll} class="w-full text-left px-4 py-1.5 cursor-pointer hover:bg-white/5" style="background:none;border:none;color:inherit">{m.alert_logs_remove_filters()}</button>
		</div>
	</div>
{/if}
