<script lang="ts">
	import { onMount } from 'svelte';
	import RefreshCwIcon from '@lucide/svelte/icons/refresh-cw';
	import CircleAlertIcon from '@lucide/svelte/icons/circle-alert';
	import XIcon from '@lucide/svelte/icons/x';
	import {
		buildJsonSections,
		buildMetadataSections,
		formatCompactContent,
		type JsonDialogSection
	} from './doc-review-json-dialog.js';

	type FindingRow = {
		id: number;
		input_record_id: number;
		run_id: number;
		pass: string;
		aspect: string;
		severity: string;
		finding_type: string;
		title: string;
		description: string;
		evidence: string;
		location: string;
		suggestion: string;
		confidence: number;
		review_status: string;
		artifact_id: string;
		metadata: unknown;
		reference_doc: unknown;
	};

	let {
		darkMode = true,
		initialRunId = '',
		embedded = false
	}: {
		darkMode: boolean;
		initialRunId?: string;
		embedded?: boolean;
	} = $props();

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

	let rows = $state<FindingRow[]>([]);
	let total = $state(0);
	let page = $state(1);
	let pageSize = $state(50);
	let loading = $state(false);
	let error = $state('');

	let filterInputRecordId = $state('');
	let filterRunId = $state(initialRunId.trim());
	let filterPass = $state('');
	let filterAspect = $state('');
	let filterSeverity = $state('');
	let filterReviewStatus = $state('');
	let filterFindingType = $state('');
	let filterArtifactId = $state('');
	let filterTitle = $state('');

	let modalOpen = $state(false);
	let modalTitle = $state('');
	let modalSections = $state<JsonDialogSection[]>([]);
	let modalElement = $state<HTMLDivElement | null>(null);
	let closeButton = $state<HTMLButtonElement | null>(null);
	let returnFocusElement: HTMLElement | null = null;

	let totalPages = $derived(Math.max(1, Math.ceil(total / pageSize)));

	function add(params: URLSearchParams, name: string, value: string) {
		if (value.trim()) params.set(name, value.trim());
	}

	async function load() {
		loading = true;
		error = '';
		try {
			const params = new URLSearchParams({ page: String(page), page_size: String(pageSize) });
			add(params, 'input_record_id', filterInputRecordId);
			add(params, 'run_id', filterRunId);
			add(params, 'pass', filterPass);
			add(params, 'aspect', filterAspect);
			add(params, 'severity', filterSeverity);
			add(params, 'review_status', filterReviewStatus);
			add(params, 'finding_type', filterFindingType);
			add(params, 'artifact_id', filterArtifactId);
			add(params, 'title', filterTitle);
			const response = await fetch(`/api/v1/kb/doc-review-findings?${params}`, {
				credentials: 'same-origin'
			});
			const body = await response.text();
			let data: Record<string, unknown> = {};
			try {
				data = JSON.parse(body);
			} catch {
				throw new Error(
					response.ok
						? 'Findings response was not valid JSON'
						: body || 'Failed to load document review findings'
				);
			}
			if (!response.ok || !data.status)
				throw new Error(String(data.error_msg ?? 'Failed to load document review findings'));
			rows = Array.isArray(data.results) ? (data.results as FindingRow[]) : [];
			total = typeof data.total === 'number' ? data.total : 0;
		} catch (err) {
			error = err instanceof Error ? err.message : String(err);
			rows = [];
			total = 0;
		} finally {
			loading = false;
		}
	}

	function applyFilters() {
		page = 1;
		load();
	}

	function clearFilters() {
		filterInputRecordId = '';
		filterRunId = '';
		filterPass = '';
		filterAspect = '';
		filterSeverity = '';
		filterReviewStatus = '';
		filterFindingType = '';
		filterArtifactId = '';
		filterTitle = '';
		page = 1;
		load();
	}

	function compact(value: unknown) {
		const text = formatCompactContent(value);
		return text.length > 140 ? `${text.slice(0, 139)}…` : text;
	}

	function openModal(title: string, sections: JsonDialogSection[] = []) {
		returnFocusElement =
			document.activeElement instanceof HTMLElement ? document.activeElement : null;
		modalTitle = title;
		modalSections = sections;
		modalOpen = true;
		requestAnimationFrame(() => closeButton?.focus());
	}

	function closeModal() {
		const focusTarget = returnFocusElement;
		modalOpen = false;
		requestAnimationFrame(() => focusTarget?.focus());
		returnFocusElement = null;
	}

	function handleModalKeydown(event: KeyboardEvent) {
		if (event.key === 'Escape') {
			event.preventDefault();
			closeModal();
			return;
		}
		if (event.key !== 'Tab' || !modalElement) return;
		const focusable = Array.from(
			modalElement.querySelectorAll<HTMLElement>(
				'button:not([disabled]), [href], input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])'
			)
		);
		if (!focusable.length) {
			event.preventDefault();
			modalElement.focus();
			return;
		}
		const first = focusable[0];
		const last = focusable[focusable.length - 1];
		if (event.shiftKey && document.activeElement === first) {
			event.preventDefault();
			last.focus();
		} else if (!event.shiftKey && document.activeElement === last) {
			event.preventDefault();
			first.focus();
		}
	}

	function openMetadata(row: FindingRow) {
		openModal(`Metadata — Finding #${row.id}`, buildMetadataSections(row.metadata));
	}

	function openReferenceDoc(row: FindingRow) {
		openModal(`Reference Doc — Finding #${row.id}`, buildJsonSections(row.reference_doc));
	}

	onMount(load);
</script>

<div
	class="flex h-full flex-col space-y-4 overflow-hidden"
	style="background:{embedded ? 'transparent' : pageBg}; padding:{embedded ? '0' : '1.5rem'}; user-select:text; -webkit-user-select:text"
>
	<div
		class="flex-shrink-0 rounded-xl p-5"
		style="background:{cardBg};border:1px solid {borderColor}"
	>
		<div class="flex flex-wrap items-start justify-between gap-3">
			<div>
				<h2 style="font-size:18px;font-weight:600;color:{textPrimary}">Doc Review Findings</h2>
				<p style="font-size:13px;color:{textSecondary};margin-top:2px">
					Search persisted findings from <code style="color:{accent}">kb.doc_review_findings</code>.
				</p>
			</div>
			<button
				onclick={load}
				disabled={loading}
				class="inline-flex cursor-pointer items-center gap-2 rounded-lg px-3 py-2"
				style="background:{surface2};color:{textPrimary};border:1px solid {borderColor}"
			>
				<RefreshCwIcon class="h-4 w-4 {loading ? 'animate-spin' : ''}" />Refresh
			</button>
		</div>
	</div>

	<div
		class="flex-shrink-0 rounded-xl p-5"
		style="background:{cardBg};border:1px solid {borderColor}"
	>
		<div class="grid gap-3" style="grid-template-columns:repeat(auto-fill,minmax(150px,1fr))">
			<label class="flex flex-col gap-1"
				><span style="font-size:11px;color:{textMuted}">Input Record ID</span><input
					type="number"
					bind:value={filterInputRecordId}
					class="rounded px-2 py-1.5 text-sm"
					style="background:{surface2};color:{textPrimary};border:1px solid {borderColor}"
				/></label
			>
			<label class="flex flex-col gap-1"
				><span style="font-size:11px;color:{textMuted}">Run ID</span><input
					type="number"
					bind:value={filterRunId}
					class="rounded px-2 py-1.5 text-sm"
					style="background:{surface2};color:{textPrimary};border:1px solid {borderColor}"
				/></label
			>
			<label class="flex flex-col gap-1"
				><span style="font-size:11px;color:{textMuted}">Pass</span><input
					bind:value={filterPass}
					class="rounded px-2 py-1.5 text-sm"
					style="background:{surface2};color:{textPrimary};border:1px solid {borderColor}"
				/></label
			>
			<label class="flex flex-col gap-1"
				><span style="font-size:11px;color:{textMuted}">Aspect</span><input
					bind:value={filterAspect}
					class="rounded px-2 py-1.5 text-sm"
					style="background:{surface2};color:{textPrimary};border:1px solid {borderColor}"
				/></label
			>
			<label class="flex flex-col gap-1"
				><span style="font-size:11px;color:{textMuted}">Severity</span><input
					bind:value={filterSeverity}
					placeholder="high / medium / low"
					class="rounded px-2 py-1.5 text-sm"
					style="background:{surface2};color:{textPrimary};border:1px solid {borderColor}"
				/></label
			>
			<label class="flex flex-col gap-1"
				><span style="font-size:11px;color:{textMuted}">Review Status</span><input
					bind:value={filterReviewStatus}
					placeholder="pending / accepted…"
					class="rounded px-2 py-1.5 text-sm"
					style="background:{surface2};color:{textPrimary};border:1px solid {borderColor}"
				/></label
			>
			<label class="flex flex-col gap-1"
				><span style="font-size:11px;color:{textMuted}">Finding Type</span><input
					bind:value={filterFindingType}
					class="rounded px-2 py-1.5 text-sm"
					style="background:{surface2};color:{textPrimary};border:1px solid {borderColor}"
				/></label
			>
			<label class="flex flex-col gap-1"
				><span style="font-size:11px;color:{textMuted}">Artifact ID</span><input
					bind:value={filterArtifactId}
					class="rounded px-2 py-1.5 text-sm"
					style="background:{surface2};color:{textPrimary};border:1px solid {borderColor}"
				/></label
			>
			<label class="flex flex-col gap-1"
				><span style="font-size:11px;color:{textMuted}">Title</span><input
					bind:value={filterTitle}
					placeholder="ILIKE search…"
					class="rounded px-2 py-1.5 text-sm"
					style="background:{surface2};color:{textPrimary};border:1px solid {borderColor}"
				/></label
			>
		</div>
		<div class="mt-4 flex gap-2">
			<button
				onclick={applyFilters}
				disabled={loading}
				class="cursor-pointer rounded-lg px-3 py-2 text-sm"
				style="background:{accent};color:white">Apply Filters</button
			>
			<button
				onclick={clearFilters}
				disabled={loading}
				class="cursor-pointer rounded-lg px-3 py-2 text-sm"
				style="background:{surface2};color:{textPrimary};border:1px solid {borderColor}"
				>Clear</button
			>
		</div>
	</div>

	{#if error}
		<div
			class="flex flex-shrink-0 gap-2 rounded-xl p-4"
			style="background:{danger}20;border:1px solid {danger}70;color:{danger}"
		>
			<CircleAlertIcon class="h-4 w-4" /><span style="font-size:13px">{error}</span>
		</div>
	{/if}

	<div
		class="flex min-h-0 flex-1 flex-col rounded-xl"
		style="background:{cardBg};border:1px solid {borderColor}"
	>
		<div
			class="flex flex-shrink-0 justify-between px-5 py-3"
			style="border-bottom:1px solid {borderColor}"
		>
			<span style="font-size:13px;color:{textMuted}"
				>Total: {total} findings{#if total}
					· page {page} of {totalPages}{/if}</span
			>
			<div class="flex gap-2">
				<button
					onclick={() => {
						if (page > 1) {
							page--;
							load();
						}
					}}
					disabled={page <= 1 || loading}
					class="rounded px-3 py-1 text-sm disabled:opacity-40"
					style="background:{surface2};color:{textPrimary};border:1px solid {borderColor}"
					>‹ Prev</button
				>
				<button
					onclick={() => {
						if (page < totalPages) {
							page++;
							load();
						}
					}}
					disabled={page >= totalPages || loading}
					class="rounded px-3 py-1 text-sm disabled:opacity-40"
					style="background:{surface2};color:{textPrimary};border:1px solid {borderColor}"
					>Next ›</button
				>
			</div>
		</div>
		{#if loading}
			<div class="px-5 py-8 text-center" style="color:{textMuted}">Loading…</div>
		{:else if !rows.length}
			<div class="px-5 py-8 text-center" style="color:{textMuted}">
				No document review findings found.
			</div>
		{:else}
			<div class="min-h-0 flex-1 overflow-auto">
				<table class="w-full text-sm" style="border-collapse:separate;border-spacing:0">
					<thead>
						<tr style="background:{surface2}">
							{#each ['ID', 'Run ID', 'Input Record ID', 'Pass', 'Aspect', 'Severity', 'Type', 'Title', 'Artifact ID', 'Confidence', 'Review Status', 'Evidence', 'Location', 'Suggestion', 'Metadata', 'Reference Doc'] as heading}
								<th
									class="sticky top-0 z-10 px-4 py-3 text-left"
									style="color:{textMuted};font-weight:500;white-space:nowrap;font-size:12px;background:{surface2};border-bottom:1px solid {borderColor}"
									>{heading}</th
								>
							{/each}
						</tr>
					</thead>
					<tbody>
						{#each rows as row (row.id)}
							<tr class="hover:bg-white/5">
								<td
									class="px-4 py-2.5"
									style="border-bottom:1px solid {borderColor};color:{textSecondary};white-space:nowrap"
									>{row.id}</td
								>
								<td
									class="px-4 py-2.5"
									style="border-bottom:1px solid {borderColor};color:{textSecondary};white-space:nowrap"
									>{row.run_id}</td
								>
								<td
									class="px-4 py-2.5"
									style="border-bottom:1px solid {borderColor};color:{textSecondary};white-space:nowrap"
									>{row.input_record_id}</td
								>
								<td
									class="px-4 py-2.5"
									style="border-bottom:1px solid {borderColor};color:{textSecondary};white-space:nowrap"
									>{row.pass || '—'}</td
								>
								<td
									class="px-4 py-2.5"
									style="border-bottom:1px solid {borderColor};color:{textSecondary};white-space:nowrap"
									>{row.aspect || '—'}</td
								>
								<td
									class="px-4 py-2.5"
									style="border-bottom:1px solid {borderColor};color:{textSecondary};white-space:nowrap"
									>{row.severity || '—'}</td
								>
								<td
									class="px-4 py-2.5"
									style="border-bottom:1px solid {borderColor};color:{textSecondary};white-space:nowrap"
									>{row.finding_type || '—'}</td
								>
								<td
									class="px-4 py-2.5"
									style="border-bottom:1px solid {borderColor};min-width:280px"
								>
									<div style="color:{textPrimary};font-weight:500">{row.title || '—'}</div>
									{#if row.description}
										<div style="color:{textMuted};font-size:12px;margin-top:4px;max-width:420px">
											{compact(row.description)}
										</div>
									{/if}
								</td>
								<td
									class="px-4 py-2.5"
									style="border-bottom:1px solid {borderColor};color:{textSecondary};font-family:monospace;white-space:nowrap"
									>{row.artifact_id || '—'}</td
								>
								<td
									class="px-4 py-2.5 text-right"
									style="border-bottom:1px solid {borderColor};color:{textSecondary};white-space:nowrap"
									>{row.confidence?.toFixed?.(2) ?? row.confidence}</td
								>
								<td
									class="px-4 py-2.5"
									style="border-bottom:1px solid {borderColor};color:{textSecondary};white-space:nowrap"
									>{row.review_status || '—'}</td
								>
								<td
									class="px-4 py-2.5"
									style="border-bottom:1px solid {borderColor};color:{textSecondary};max-width:220px"
									title={row.evidence}>{compact(row.evidence || '—')}</td
								>
								<td
									class="px-4 py-2.5"
									style="border-bottom:1px solid {borderColor};color:{textSecondary};max-width:220px"
									title={row.location}>{compact(row.location || '—')}</td
								>
								<td
									class="px-4 py-2.5"
									style="border-bottom:1px solid {borderColor};color:{textSecondary};max-width:220px"
									title={row.suggestion}>{compact(row.suggestion || '—')}</td
								>
								<td class="px-4 py-2.5" style="border-bottom:1px solid {borderColor}"
									><button
										onclick={() => openMetadata(row)}
										class="cursor-pointer rounded px-2 py-1 text-xs"
										style="background:{surface2};color:{accent};border:1px solid {borderColor}"
										>Metadata</button
									></td
								>
								<td class="px-4 py-2.5" style="border-bottom:1px solid {borderColor}"
									><button
										onclick={() => openReferenceDoc(row)}
										class="cursor-pointer rounded px-2 py-1 text-xs"
										style="background:{surface2};color:{accent};border:1px solid {borderColor}"
										>Reference</button
									></td
								>
							</tr>
						{/each}
					</tbody>
				</table>
			</div>
		{/if}
	</div>
</div>

{#if modalOpen}
	<div
		class="fixed inset-0 z-50 flex items-center justify-center p-6"
		style="background:{overlay}"
		role="presentation"
		onclick={(event) => {
			if (event.target === event.currentTarget) closeModal();
		}}
	>
		<div
			bind:this={modalElement}
			tabindex="-1"
			class="flex flex-col rounded-xl"
			role="dialog"
			aria-modal="true"
			aria-label={modalTitle}
			onkeydown={handleModalKeydown}
			style="background:{cardBg};border:1px solid {borderColor};width:min(900px,100%);max-height:80vh"
		>
			<div class="flex justify-between px-5 py-4" style="border-bottom:1px solid {borderColor}">
				<span style="font-size:14px;font-weight:600;color:{textPrimary};font-family:monospace"
					>{modalTitle}</span
				>
				<button
					bind:this={closeButton}
					onclick={closeModal}
					class="cursor-pointer rounded p-1.5"
					style="background:{surface2};color:{textMuted};border:1px solid {borderColor}"
					aria-label="Close"><XIcon class="h-4 w-4" /></button
				>
			</div>
			<div class="flex-1 overflow-auto p-5">
				{#if !modalSections.length}
					<div class="text-center" style="color:{textMuted};padding:2rem">No data.</div>
				{:else}
					<div class="space-y-4 text-xs" style="line-height:1.6;user-select:text">
						{#each modalSections as section, index (index)}
							<div
								class="rounded-lg p-4"
								style="background:{surface2};border:1px solid {borderColor}"
							>
								<div
									class="grid gap-x-6 gap-y-2"
									style="grid-template-columns:minmax(180px,240px) minmax(0,1fr)"
								>
									{#each section.rows as row, i (i)}
										<div
											style="color:{textMuted};font-family:monospace;word-break:break-word;{row.indent
												? 'padding-left:1rem;'
												: ''}"
										>
											{row.label}
										</div>
										<div style="color:{textSecondary};word-break:break-word;white-space:pre-wrap">
											{row.value}
										</div>
									{/each}
								</div>
							</div>
						{/each}
					</div>
				{/if}
			</div>
		</div>
	</div>
{/if}
