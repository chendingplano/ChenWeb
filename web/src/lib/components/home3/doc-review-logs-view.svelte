<script lang="ts">
	import { onMount } from 'svelte';
	import RefreshCwIcon from '@lucide/svelte/icons/refresh-cw';
	import CircleAlertIcon from '@lucide/svelte/icons/circle-alert';
	import XIcon from '@lucide/svelte/icons/x';
	import { buildJsonSections, buildMatchedUnitsSections, formatCompactContent, type JsonDialogSection } from './doc-review-json-dialog.js';
	import { artifactTypeFromKey, getArtifactWiki } from './artifact-key.js';

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
		id: number; input_record_id: number | null; run_id: number | null; pass: string | null;
		aspect: string; unit_type: string; unit_key: string; unit_location: unknown;
		matched_units: unknown; findings: unknown; outcome: string; detail: unknown; create_time: string;
	};
	let rows = $state<LogRow[]>([]), total = $state(0), page = $state(1), pageSize = $state(50);
	let loading = $state(false), error = $state('');
	let filterRecordId = $state(''), filterRunId = $state(''), filterPass = $state('');
	let filterAspect = $state(''), filterUnitType = $state(''), filterOutcome = $state(''), filterUnitKey = $state('');
	let filterStart = $state(''), filterEnd = $state('');
	let modalOpen = $state(false), modalTitle = $state(''), modalSections = $state<JsonDialogSection[]>([]), modalLoading = $state(false), modalError = $state('');
	let modalElement = $state<HTMLDivElement | null>(null);
	let closeButton = $state<HTMLButtonElement | null>(null);
	let returnFocusElement: HTMLElement | null = null;
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
			const body = await response.text();
			let data: Record<string, unknown> = {};
			try { data = JSON.parse(body); }
			catch { throw new Error(response.ok ? 'Log response was not valid JSON' : body || 'Failed to load document review logs'); }
			if (!response.ok || !data.status) throw new Error(String(data.error_msg ?? 'Failed to load document review logs'));
			rows = Array.isArray(data.results) ? data.results as LogRow[] : []; total = typeof data.total === 'number' ? data.total : 0;
		} catch (err) { error = err instanceof Error ? err.message : String(err); rows = []; total = 0; }
		finally { loading = false; }
	}
	function applyFilters() { page = 1; load(); }
	function clearFilters() { filterRecordId = ''; filterRunId = ''; filterPass = ''; filterAspect = ''; filterUnitType = ''; filterOutcome = ''; filterUnitKey = ''; filterStart = ''; filterEnd = ''; page = 1; load(); }
	function formatTime(value: string) { const parsed = new Date(value); return Number.isNaN(parsed.getTime()) ? value || '—' : parsed.toLocaleString(); }
	function findingsCount(value: unknown) { return Array.isArray(value) ? value.length : 0; }
	function jsonText(value: unknown) { try { return value == null ? 'null' : JSON.stringify(value); } catch { return String(value); } }
	function compactJson(value: unknown) { const text = formatCompactContent(value); return text.length > 120 ? `${text.slice(0, 119)}…` : text; }
	function openModal(title: string, sections: JsonDialogSection[] = [], loading = false) {
		returnFocusElement = document.activeElement instanceof HTMLElement ? document.activeElement : null;
		modalTitle = title; modalSections = sections; modalError = ''; modalLoading = loading; modalOpen = true;
		requestAnimationFrame(() => closeButton?.focus());
	}
	function closeModal() {
		const focusTarget = returnFocusElement;
		modalOpen = false;
		requestAnimationFrame(() => focusTarget?.focus());
		returnFocusElement = null;
	}
	function handleModalKeydown(event: KeyboardEvent) {
		if (event.key === 'Escape') { event.preventDefault(); closeModal(); return; }
		if (event.key !== 'Tab' || !modalElement) return;
		const focusable = Array.from(modalElement.querySelectorAll<HTMLElement>('button:not([disabled]), [href], input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])'));
		if (!focusable.length) { event.preventDefault(); modalElement.focus(); return; }
		const first = focusable[0], last = focusable[focusable.length - 1];
		if (event.shiftKey && document.activeElement === first) { event.preventDefault(); last.focus(); }
		else if (!event.shiftKey && document.activeElement === last) { event.preventDefault(); first.focus(); }
	}
	function openMatched(row: LogRow) { openModal(`Matched — Log #${row.id}`, buildMatchedUnitsSections(row.matched_units)); }
	function openFindings(row: LogRow) { openModal(`Findings — Log #${row.id}`, buildJsonSections(row.findings)); }
	function openLlmCall(row: LogRow) { openModal(`LLM Call — Log #${row.id}`, buildJsonSections(row.detail)); }
	const artifactType = artifactTypeFromKey;
	async function openArtifact(key: string) {
		const type = artifactType(key); if (!type) return;
		openModal(`Artifact — ${key}`, [], true);
		try { modalSections = buildJsonSections(await getArtifactWiki(type, key)); }
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
			<label class="flex flex-col gap-1"><span style="font-size:11px;color:{textMuted}">Pass</span><input bind:value={filterPass} placeholder="Exact match…" class="rounded px-2 py-1.5 text-sm" style="background:{surface2};color:{textPrimary};border:1px solid {borderColor}" /></label>
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
		{#if loading}<div class="px-5 py-8 text-center" style="color:{textMuted}">Loading…</div>{:else if !rows.length}<div class="px-5 py-8 text-center" style="color:{textMuted}">No document review logs found.</div>{:else}<div class="flex-1 min-h-0 overflow-auto"><table class="w-full text-sm" style="border-collapse:separate;border-spacing:0"><thead><tr style="background:{surface2}">{#each ['Input Record ID','Run ID','Unit Key','Aspect','Unit Type','Unit Location','Outcome','Pass','# Findings','Matched','Findings','LLM Call','Create Time'] as heading}<th class="text-left px-4 py-3 sticky top-0 z-10" style="color:{textMuted};font-weight:500;white-space:nowrap;font-size:12px;background:{surface2};border-bottom:1px solid {borderColor}">{heading}</th>{/each}</tr></thead><tbody>{#each rows as row (row.id)}<tr class="hover:bg-white/5"><td class="px-4 py-2.5" style="border-bottom:1px solid {borderColor};color:{textSecondary}">{row.input_record_id ?? '—'}</td><td class="px-4 py-2.5" style="border-bottom:1px solid {borderColor};color:{textSecondary}">{row.run_id ?? '—'}</td><td class="px-4 py-2.5" style="border-bottom:1px solid {borderColor};white-space:nowrap">{#if artifactType(row.unit_key)}<button onclick={() => openArtifact(row.unit_key)} class="cursor-pointer" style="color:{accent};background:none;border:none;font-family:monospace">{row.unit_key}</button>{:else}<span style="color:{textSecondary};font-family:monospace">{row.unit_key || '—'}</span>{/if}</td><td class="px-4 py-2.5" style="border-bottom:1px solid {borderColor};color:{textSecondary}">{row.aspect || '—'}</td><td class="px-4 py-2.5" style="border-bottom:1px solid {borderColor};color:{textSecondary}">{row.unit_type || '—'}</td><td class="px-4 py-2.5" style="border-bottom:1px solid {borderColor};color:{textSecondary};max-width:260px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap" title={jsonText(row.unit_location)}>{compactJson(row.unit_location)}</td><td class="px-4 py-2.5" style="border-bottom:1px solid {borderColor};color:{textSecondary}">{row.outcome || '—'}</td><td class="px-4 py-2.5" style="border-bottom:1px solid {borderColor};color:{textSecondary}">{row.pass || '—'}</td><td class="px-4 py-2.5 text-right" style="border-bottom:1px solid {borderColor};color:{textSecondary}">{findingsCount(row.findings)}</td><td class="px-4 py-2.5" style="border-bottom:1px solid {borderColor}"><button onclick={() => openMatched(row)} class="rounded px-2 py-1 text-xs cursor-pointer" style="background:{surface2};color:{accent};border:1px solid {borderColor}">Matched</button></td><td class="px-4 py-2.5" style="border-bottom:1px solid {borderColor}"><button onclick={() => openFindings(row)} class="rounded px-2 py-1 text-xs cursor-pointer" style="background:{surface2};color:{accent};border:1px solid {borderColor}">Findings</button></td><td class="px-4 py-2.5" style="border-bottom:1px solid {borderColor}"><button onclick={() => openLlmCall(row)} class="rounded px-2 py-1 text-xs cursor-pointer" style="background:{surface2};color:{accent};border:1px solid {borderColor}">LLM Call</button></td><td class="px-4 py-2.5" style="border-bottom:1px solid {borderColor};color:{textMuted};white-space:nowrap;font-size:12px">{formatTime(row.create_time)}</td></tr>{/each}</tbody></table></div>{/if}
	</div>
</div>
{#if modalOpen}<div class="fixed inset-0 z-50 flex items-center justify-center p-6" style="background:{overlay}" role="presentation" onclick={(event) => { if (event.target === event.currentTarget) closeModal(); }}><div bind:this={modalElement} tabindex="-1" class="rounded-xl flex flex-col" role="dialog" aria-modal="true" aria-label={modalTitle} onkeydown={handleModalKeydown} style="background:{cardBg};border:1px solid {borderColor};width:min(900px,100%);max-height:80vh"><div class="flex justify-between px-5 py-4" style="border-bottom:1px solid {borderColor}"><span style="font-size:14px;font-weight:600;color:{textPrimary};font-family:monospace">{modalTitle}</span><button bind:this={closeButton} onclick={closeModal} class="rounded p-1.5 cursor-pointer" style="background:{surface2};color:{textMuted};border:1px solid {borderColor}" aria-label="Close"><XIcon class="w-4 h-4" /></button></div><div class="flex-1 overflow-auto p-5">{#if modalLoading}<div class="text-center" style="color:{textMuted};padding:2rem">Loading…</div>{:else if modalError}<div class="rounded-lg p-4 flex gap-2" style="background:{danger}15;border:1px solid {danger}40;color:{danger}"><CircleAlertIcon class="w-4 h-4" /><span>{modalError}</span></div>{:else if !modalSections.length}<div class="text-center" style="color:{textMuted};padding:2rem">No data.</div>{:else}<div class="space-y-4 text-xs" style="line-height:1.6;user-select:text">{#each modalSections as section, index (index)}<div class="rounded-lg p-4" style="background:{surface2};border:1px solid {borderColor}"><div class="grid gap-x-6 gap-y-2" style="grid-template-columns:minmax(180px,240px) minmax(0,1fr)">{#each section.rows as row (row.label)}<div style="color:{textMuted};font-family:monospace;word-break:break-word">{row.label}</div><div style="color:{textSecondary};word-break:break-word;white-space:pre-wrap">{row.value}</div>{/each}</div></div>{/each}</div>{/if}</div></div></div>{/if}
