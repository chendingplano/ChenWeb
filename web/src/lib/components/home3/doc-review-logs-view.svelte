<script lang="ts">
	import { onMount } from 'svelte';
	import RefreshCwIcon from '@lucide/svelte/icons/refresh-cw';
	import CircleAlertIcon from '@lucide/svelte/icons/circle-alert';
	import XIcon from '@lucide/svelte/icons/x';

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
	let overlay = $derived(darkMode ? '#0D1117E6' : '#00000066');

	type LogRow = {
		id: number; input_record_id: number | null; run_id: number | null; pass: boolean | null;
		aspect: string; unit_type: string; unit_key: string; unit_location: unknown;
		matched_units: unknown; findings: unknown; outcome: string; detail: unknown; create_time: string;
	};
	let rows = $state<LogRow[]>([]), total = $state(0), page = $state(1), pageSize = $state(50);
	let loading = $state(false), error = $state('');
	let filterRecordId = $state(''), filterRunId = $state(''), filterPass = $state('');
	let filterAspect = $state(''), filterUnitType = $state(''), filterOutcome = $state(''), filterUnitKey = $state('');
	let filterStart = $state(''), filterEnd = $state('');
	let modalOpen = $state(false), modalTitle = $state(''), modalValue = $state<unknown>(null), modalLoading = $state(false), modalError = $state('');
	let totalPages = $derived(Math.max(1, Math.ceil(total / pageSize)));

	function toRFC3339(value: string) { return new Date(value).toISOString(); }
	function add(params: URLSearchParams, name: string, value: string) { if (value.trim()) params.set(name, value.trim()); }
	async function load() {
		loading = true; error = '';
		try {
			const params = new URLSearchParams({ page: String(page), page_size: String(pageSize) });
			add(params, 'input_record_id', filterRecordId); add(params, 'run_id', filterRunId); add(params, 'pass', filterPass);
			add(params, 'aspect', filterAspect); add(params, 'unit_type', filterUnitType); add(params, 'outcome', filterOutcome); add(params, 'unit_key', filterUnitKey);
			if (filterStart) params.set('create_start_time', toRFC3339(filterStart));
			if (filterEnd) params.set('create_end_time', toRFC3339(filterEnd));
			const response = await fetch(`/api/v1/kb/doc-review-logs?${params}`, { credentials: 'same-origin' });
			const data = await response.json();
			if (!response.ok || !data.status) throw new Error(data.error_msg ?? 'Failed to load document review logs');
			rows = data.results ?? []; total = data.total ?? 0;
		} catch (err) { error = err instanceof Error ? err.message : String(err); rows = []; total = 0; }
		finally { loading = false; }
	}
	function applyFilters() { page = 1; load(); }
	function clearFilters() { filterRecordId = ''; filterRunId = ''; filterPass = ''; filterAspect = ''; filterUnitType = ''; filterOutcome = ''; filterUnitKey = ''; filterStart = ''; filterEnd = ''; page = 1; load(); }
	function formatTime(value: string) { const parsed = new Date(value); return Number.isNaN(parsed.getTime()) ? value || '—' : parsed.toLocaleString(); }
	function findingsCount(value: unknown) { return Array.isArray(value) ? value.length : 0; }
	function jsonText(value: unknown) { try { return value == null ? 'null' : JSON.stringify(value); } catch { return String(value); } }
	function compactJson(value: unknown) { const text = jsonText(value); return text.length > 120 ? `${text.slice(0, 119)}…` : text; }
	function escapeHtml(value: string) { return value.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/\"/g, '&quot;'); }
	function renderValue(value: unknown, depth = 0): string {
		const pad = depth * 16;
		if (value === null || value === undefined) return `<span style="color:${textMuted}">null</span>`;
		if (typeof value !== 'object') return `<span style="color:${textSecondary};word-break:break-word">${escapeHtml(String(value))}</span>`;
		const entries = Array.isArray(value) ? value.map((v, i) => [String(i), v] as [string, unknown]) : Object.entries(value as Record<string, unknown>);
		if (!entries.length) return `<span style="color:${textMuted}">—</span>`;
		return entries.map(([key, child]) => `<div style="padding-left:${pad}px;margin:4px 0"><div style="display:flex;gap:12px;align-items:flex-start"><span style="color:${textMuted};font-family:monospace;min-width:120px;flex-shrink:0">${escapeHtml(Array.isArray(value) ? `[${key}]` : key)}</span><div style="min-width:0;flex:1">${child !== null && typeof child === 'object' ? renderValue(child, depth + 1) : renderValue(child)}</div></div></div>`).join('');
	}
	function openDetails(row: LogRow) { modalTitle = `Doc Review Log #${row.id}`; modalValue = { unit_location: row.unit_location, matched_units: row.matched_units, findings: row.findings, detail: row.detail }; modalError = ''; modalLoading = false; modalOpen = true; }
	const artifactPattern = /^[1-9][0-9]*_(mtc|prv|inv)_[1-9][0-9]*$/;
	function artifactType(key: string): string | null {
		const match = key.match(artifactPattern);
		if (!match) return null;
		return { mtc: 'metric', prv: 'provision', inv: 'inventory_item' }[match[1] as 'mtc' | 'prv' | 'inv'];
	}
	async function openArtifact(key: string) {
		const type = artifactType(key); if (!type) return;
		modalTitle = `Artifact — ${key}`; modalValue = null; modalError = ''; modalLoading = true; modalOpen = true;
		try { const response = await fetch(`/api/v1/kb/artifacts/wiki?artifact_type=${encodeURIComponent(type)}&artifact_id=${encodeURIComponent(key)}&include_article=0`, { credentials: 'same-origin' }); const data = await response.json(); if (!response.ok || !data.record) throw new Error(data.error_msg ?? data.message ?? 'Failed to load artifact'); modalValue = data.record; }
		catch (err) { modalError = err instanceof Error ? err.message : String(err); }
		finally { modalLoading = false; }
	}
	onMount(load);
</script>

<div class="p-6 space-y-4 h-full flex flex-col overflow-hidden" style="background:{pageBg}">
	<div class="rounded-xl p-5 flex-shrink-0" style="background:{cardBg};border:1px solid {borderColor}">
		<div class="flex flex-wrap items-start justify-between gap-3"><div><h2 style="font-size:18px;font-weight:600;color:{textPrimary}">Doc Review Logs</h2><p style="font-size:13px;color:{textSecondary};margin-top:2px">Document-review execution records from <code style="color:{accent}">kb.doc_review_logs</code>.</p></div><button onclick={load} disabled={loading} class="inline-flex items-center gap-2 rounded-lg px-3 py-2 cursor-pointer" style="background:{surface2};color:{textPrimary};border:1px solid {borderColor}"><RefreshCwIcon class="w-4 h-4 {loading ? 'animate-spin' : ''}" />Refresh</button></div>
	</div>
	<div class="rounded-xl p-5 flex-shrink-0" style="background:{cardBg};border:1px solid {borderColor}">
		<div class="grid gap-3" style="grid-template-columns:repeat(auto-fill,minmax(150px,1fr))">
			<label class="flex flex-col gap-1"><span style="font-size:11px;color:{textMuted}">Input Record ID</span><input type="number" bind:value={filterRecordId} class="rounded px-2 py-1.5 text-sm" style="background:{surface2};color:{textPrimary};border:1px solid {borderColor}" /></label>
			<label class="flex flex-col gap-1"><span style="font-size:11px;color:{textMuted}">Run ID</span><input type="number" bind:value={filterRunId} class="rounded px-2 py-1.5 text-sm" style="background:{surface2};color:{textPrimary};border:1px solid {borderColor}" /></label>
			<label class="flex flex-col gap-1"><span style="font-size:11px;color:{textMuted}">Pass</span><select bind:value={filterPass} class="rounded px-2 py-1.5 text-sm" style="background:{surface2};color:{textPrimary};border:1px solid {borderColor}"><option value="">All</option><option value="true">Pass</option><option value="false">Fail</option></select></label>
			<label class="flex flex-col gap-1"><span style="font-size:11px;color:{textMuted}">Aspect</span><input bind:value={filterAspect} class="rounded px-2 py-1.5 text-sm" style="background:{surface2};color:{textPrimary};border:1px solid {borderColor}" /></label>
			<label class="flex flex-col gap-1"><span style="font-size:11px;color:{textMuted}">Unit Type</span><input bind:value={filterUnitType} class="rounded px-2 py-1.5 text-sm" style="background:{surface2};color:{textPrimary};border:1px solid {borderColor}" /></label>
			<label class="flex flex-col gap-1"><span style="font-size:11px;color:{textMuted}">Outcome</span><input bind:value={filterOutcome} class="rounded px-2 py-1.5 text-sm" style="background:{surface2};color:{textPrimary};border:1px solid {borderColor}" /></label>
			<label class="flex flex-col gap-1"><span style="font-size:11px;color:{textMuted}">Unit Key</span><input bind:value={filterUnitKey} class="rounded px-2 py-1.5 text-sm" style="background:{surface2};color:{textPrimary};border:1px solid {borderColor}" /></label>
			<label class="flex flex-col gap-1"><span style="font-size:11px;color:{textMuted}">Created From</span><input type="datetime-local" bind:value={filterStart} class="rounded px-2 py-1.5 text-sm" style="background:{surface2};color:{textPrimary};border:1px solid {borderColor}" /></label>
			<label class="flex flex-col gap-1"><span style="font-size:11px;color:{textMuted}">Created To</span><input type="datetime-local" bind:value={filterEnd} class="rounded px-2 py-1.5 text-sm" style="background:{surface2};color:{textPrimary};border:1px solid {borderColor}" /></label>
		</div><div class="flex gap-2 mt-4"><button onclick={applyFilters} disabled={loading} class="rounded-lg px-3 py-2 text-sm cursor-pointer" style="background:{accent};color:white">Apply Filters</button><button onclick={clearFilters} disabled={loading} class="rounded-lg px-3 py-2 text-sm cursor-pointer" style="background:{surface2};color:{textPrimary};border:1px solid {borderColor}">Clear</button></div>
	</div>
	{#if error}<div class="rounded-xl p-4 flex gap-2 flex-shrink-0" style="background:{danger}20;border:1px solid {danger}70;color:{danger}"><CircleAlertIcon class="w-4 h-4" /><span style="font-size:13px">{error}</span></div>{/if}
	<div class="rounded-xl flex flex-col flex-1 min-h-0" style="background:{cardBg};border:1px solid {borderColor}">
		<div class="px-5 py-3 flex justify-between flex-shrink-0" style="border-bottom:1px solid {borderColor}"><span style="font-size:13px;color:{textMuted}">Total: {total} logs{#if total} · page {page} of {totalPages}{/if}</span><div class="flex gap-2"><button onclick={() => { if (page > 1) { page--; load(); } }} disabled={page <= 1 || loading} class="rounded px-3 py-1 text-sm disabled:opacity-40" style="background:{surface2};color:{textPrimary};border:1px solid {borderColor}">‹ Prev</button><button onclick={() => { if (page < totalPages) { page++; load(); } }} disabled={page >= totalPages || loading} class="rounded px-3 py-1 text-sm disabled:opacity-40" style="background:{surface2};color:{textPrimary};border:1px solid {borderColor}">Next ›</button></div></div>
		{#if loading}<div class="px-5 py-8 text-center" style="color:{textMuted}">Loading…</div>{:else if !rows.length}<div class="px-5 py-8 text-center" style="color:{textMuted}">No document review logs found.</div>{:else}<div class="flex-1 min-h-0 overflow-auto"><table class="w-full text-sm" style="border-collapse:separate;border-spacing:0"><thead><tr style="background:{surface2}">{#each ['Input Record ID','Run ID','Unit Key','Aspect','Unit Type','Unit Location','Outcome','Pass','# Findings','Detail','Create Time'] as heading}<th class="text-left px-4 py-3 sticky top-0 z-10" style="color:{textMuted};font-weight:500;white-space:nowrap;font-size:12px;background:{surface2};border-bottom:1px solid {borderColor}">{heading}</th>{/each}</tr></thead><tbody>{#each rows as row (row.id)}<tr class="hover:bg-white/5"><td class="px-4 py-2.5" style="border-bottom:1px solid {borderColor};color:{textSecondary}">{row.input_record_id ?? '—'}</td><td class="px-4 py-2.5" style="border-bottom:1px solid {borderColor};color:{textSecondary}">{row.run_id ?? '—'}</td><td class="px-4 py-2.5" style="border-bottom:1px solid {borderColor};white-space:nowrap">{#if artifactType(row.unit_key)}<button onclick={() => openArtifact(row.unit_key)} class="cursor-pointer" style="color:{accent};background:none;border:none;font-family:monospace">{row.unit_key}</button>{:else}<span style="color:{textSecondary};font-family:monospace">{row.unit_key || '—'}</span>{/if}</td><td class="px-4 py-2.5" style="border-bottom:1px solid {borderColor};color:{textSecondary}">{row.aspect || '—'}</td><td class="px-4 py-2.5" style="border-bottom:1px solid {borderColor};color:{textSecondary}">{row.unit_type || '—'}</td><td class="px-4 py-2.5" style="border-bottom:1px solid {borderColor};color:{textSecondary};max-width:260px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap" title={jsonText(row.unit_location)}>{compactJson(row.unit_location)}</td><td class="px-4 py-2.5" style="border-bottom:1px solid {borderColor};color:{textSecondary}">{row.outcome || '—'}</td><td class="px-4 py-2.5" style="border-bottom:1px solid {borderColor};color:{textSecondary}">{row.pass == null ? '—' : row.pass ? 'Yes' : 'No'}</td><td class="px-4 py-2.5 text-right" style="border-bottom:1px solid {borderColor};color:{textSecondary}">{findingsCount(row.findings)}</td><td class="px-4 py-2.5" style="border-bottom:1px solid {borderColor}"><button onclick={() => openDetails(row)} class="rounded px-2 py-1 text-xs cursor-pointer" style="background:{surface2};color:{accent};border:1px solid {borderColor}">Detail</button></td><td class="px-4 py-2.5" style="border-bottom:1px solid {borderColor};color:{textMuted};white-space:nowrap;font-size:12px">{formatTime(row.create_time)}</td></tr>{/each}</tbody></table></div>{/if}
	</div>
</div>
{#if modalOpen}<div class="fixed inset-0 z-50 flex items-center justify-center p-6" style="background:{overlay}" role="presentation" onclick={(event) => { if (event.target === event.currentTarget) modalOpen = false; }}><div class="rounded-xl flex flex-col" role="dialog" aria-modal="true" aria-label={modalTitle} style="background:{cardBg};border:1px solid {borderColor};width:min(900px,100%);max-height:80vh"><div class="flex justify-between px-5 py-4" style="border-bottom:1px solid {borderColor}"><span style="font-size:14px;font-weight:600;color:{textPrimary};font-family:monospace">{modalTitle}</span><button onclick={() => modalOpen = false} class="rounded p-1.5 cursor-pointer" style="background:{surface2};color:{textMuted};border:1px solid {borderColor}" aria-label="Close"><XIcon class="w-4 h-4" /></button></div><div class="flex-1 overflow-auto p-5">{#if modalLoading}<div class="text-center" style="color:{textMuted};padding:2rem">Loading…</div>{:else if modalError}<div class="rounded-lg p-4 flex gap-2" style="background:{danger}15;border:1px solid {danger}40;color:{danger}"><CircleAlertIcon class="w-4 h-4" /><span>{modalError}</span></div>{:else}<div class="text-xs" style="line-height:1.6;user-select:text">{@html renderValue(modalValue)}</div>{/if}</div></div></div>{/if}
