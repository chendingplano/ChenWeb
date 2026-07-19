<script lang="ts">
	import { getDocReviewLogsFor, type FindingItem, type DocReviewLogRow } from '$lib/services/docReviewService';
	import { artifactTypeFromKey, getArtifactWiki } from './artifact-key.js';
	import { getLLMUsageEventsByIds, type LLMUsageEventDetail } from './llm-activities-client.js';
	import { buildFindingsSections, buildJsonSections, buildJsonTree, formatCompactContent, matchedUnitFocusTarget, matchedUnitLabel, type JsonDialogSection, type JsonTreeNode } from './doc-review-json-dialog.js';
	import JsonTreeDialog from './json-tree-dialog.svelte';
	import JsonSectionsDialog from './json-sections-dialog.svelte';
	import MatchedUnitsDialog from './matched-units-dialog.svelte';
	import LlmUsageEventDialog from './llm-usage-event-dialog.svelte';

	let {
		finding,
		requestId,
		runId,
		dark = true,
		onFocusMatchedUnit,
		onCloseMatchedUnits
	}: {
		finding: FindingItem | null;
		requestId: number | null;
		runId: number | null;
		dark?: boolean;
		onFocusMatchedUnit?: (recordId: number, lineNumbers: number[]) => void;
		onCloseMatchedUnits?: () => void;
	} = $props();

	let cardBg = $derived(dark ? '#1F2333' : '#FFFFFF');
	let borderColor = $derived(dark ? '#2D3348' : '#E4E6EB');
	let accent = $derived(dark ? '#818CF8' : '#6366F1');
	let accentTint = $derived(dark ? 'rgba(129,140,248,0.15)' : 'rgba(99,102,241,0.10)');
	let textPrimary = $derived(dark ? '#E2E8F0' : '#111827');
	let textSecondary = $derived(dark ? '#94A3B8' : '#6B7280');
	let textMuted = $derived(dark ? '#64748B' : '#9CA3AF');
	// Matched to the Document Review Report findings list scrollbar (--scroll-thumb).
	let scrollThumb = $derived(dark ? '#2A3140' : '#D7CFB8');

	// ── Fold state ───────────────────────────────────────────────────────────
	let expanded = $state<Record<string, boolean>>({ finding: true, artifact: true, log: false, llm: false });
	function toggle(key: string) {
		expanded[key] = !expanded[key];
	}

	// ── Artifact block ───────────────────────────────────────────────────────
	let artifactLoading = $state(false);
	let artifactError = $state('');
	let artifactRecord = $state<Record<string, unknown> | null>(null);

	// ── Doc Review Log block ─────────────────────────────────────────────────
	let logLoading = $state(false);
	let logError = $state('');
	let logRow = $state<DocReviewLogRow | null>(null);

	// ── LLM Calls block ──────────────────────────────────────────────────────
	let llmLoading = $state(false);
	let llmError = $state('');
	let llmEvents = $state<LLMUsageEventDetail[]>([]);

	// The matched_units array from the loaded log row ([] when null/non-array).
	let matchedUnits = $derived.by(() => {
		const value = logRow?.matched_units;
		return Array.isArray(value) ? (value as unknown[]) : [];
	});

	// ── Dialog ───────────────────────────────────────────────────────────────
	let dialog = $state<
		| { kind: 'tree'; title: string; nodes: JsonTreeNode[] }
		| { kind: 'sections'; title: string; sections: JsonDialogSection[] }
		| { kind: 'matched-units'; title: string; units: unknown[]; initialSelected?: number }
		| { kind: 'llm-event'; event: LLMUsageEventDetail }
		| null
	>(null);

	function openMetadataDialog() {
		dialog = { kind: 'tree', title: `Metadata — Finding #${finding?.id}`, nodes: buildJsonTree(finding?.metadata) };
	}
	// Opens the matched-units dialog drilled into unit `index`, and syncs the
	// PDF panel to that unit's source document/lines.
	function openMatchedUnit(index: number) {
		if (!logRow) return;
		const target = matchedUnitFocusTarget(matchedUnits[index]);
		console.info('[finding-details-panel] matched unit clicked', {
			logId: logRow.id,
			index,
			label: matchedUnitLabel(matchedUnits[index], index),
			focusTarget: target
		});
		dialog = { kind: 'matched-units', title: `Matched — Log #${logRow.id}`, units: matchedUnits, initialSelected: index };
		if (target) onFocusMatchedUnit?.(target.recordId, target.lineNumbers);
	}
	function openLogSections(kind: 'findings' | 'detail') {
		if (!logRow) return;
		const value = logRow[kind];
		const title = `${kind === 'findings' ? 'Findings' : 'LLM Call'} — Log #${logRow.id}`;
		const sections = kind === 'findings' ? buildFindingsSections(value) : buildJsonSections(value);
		dialog = { kind: 'sections', title, sections };
	}
	function openLlmEventDialog(event: LLMUsageEventDetail) {
		dialog = { kind: 'llm-event', event };
	}
	function closeDialog() {
		if (dialog?.kind === 'matched-units') onCloseMatchedUnits?.();
		dialog = null;
	}

	function llmEventIdsFromDetail(detail: unknown): string[] {
		if (detail && typeof detail === 'object' && !Array.isArray(detail)) {
			const ids = (detail as Record<string, unknown>).llm_usage_event_ids;
			if (Array.isArray(ids)) return ids.filter((x): x is string => typeof x === 'string');
		}
		return [];
	}

	async function loadArtifactAndLog(artifactId: string, effectiveRunId: number) {
		const artifactType = artifactTypeFromKey(artifactId);
		if (artifactType) {
			artifactLoading = true;
			artifactError = '';
			try {
				artifactRecord = await getArtifactWiki(artifactType, artifactId);
			} catch (e) {
				artifactError = e instanceof Error ? e.message : String(e);
			} finally {
				artifactLoading = false;
			}
		}

		logLoading = true;
		logError = '';
		try {
			logRow = await getDocReviewLogsFor(artifactId, effectiveRunId);
			const mu = logRow?.matched_units;
			console.info('[finding-details-panel] doc review log loaded', {
				artifactId,
				runId: effectiveRunId,
				logId: logRow?.id ?? null,
				unitKey: logRow?.unit_key ?? null,
				matchedUnitsIsArray: Array.isArray(mu),
				matchedUnitsCount: Array.isArray(mu) ? mu.length : null,
				matchedUnitsRaw: mu
			});
		} catch (e) {
			logError = e instanceof Error ? e.message : String(e);
		} finally {
			logLoading = false;
		}

		const ids = logRow ? llmEventIdsFromDetail(logRow.detail) : [];
		if (ids.length > 0) {
			llmLoading = true;
			llmError = '';
			try {
				llmEvents = await getLLMUsageEventsByIds(ids);
			} catch (e) {
				llmError = e instanceof Error ? e.message : String(e);
			} finally {
				llmLoading = false;
			}
		}
	}

	$effect(() => {
		const f = finding;
		const effectiveRunId = runId ?? f?.run_id ?? null;
		artifactRecord = null;
		artifactError = '';
		artifactLoading = false;
		logRow = null;
		logError = '';
		logLoading = false;
		llmEvents = [];
		llmError = '';
		llmLoading = false;
		dialog = null;
		if (f?.artifact_id && effectiveRunId != null) {
			void loadArtifactAndLog(f.artifact_id, effectiveRunId);
		}
	});

	function formatConfidence(c: number): string {
		return `${Math.round(c * 100)}%`;
	}
</script>

<div class="fd-panel" style="--fd-card:{cardBg}; --fd-border:{borderColor}; --fd-accent:{accent}; --fd-accent-tint:{accentTint}; --fd-text:{textPrimary}; --fd-text-2:{textSecondary}; --fd-text-muted:{textMuted}; --fd-scroll-thumb:{scrollThumb};">
	{#if !finding}
		<div class="fd-empty">Select a finding to view its details.</div>
	{:else}
		<!-- Finding block -->
		<div class="fd-block">
			<button type="button" class="fd-head" aria-expanded={expanded.finding} onclick={() => toggle('finding')}>
				<span class="fd-chevron" class:open={expanded.finding}>▶</span>
				<span class="fd-title">Finding</span>
			</button>
			{#if expanded.finding}
				<div class="fd-body">
					<div class="fd-row"><span class="fd-label">artifact_id</span><span class="fd-value">{finding.artifact_id || '—'}</span></div>
					<div class="fd-row"><span class="fd-label">aspect</span><span class="fd-value">{finding.aspect}</span></div>
					<div class="fd-row"><span class="fd-label">severity</span><span class="fd-value">{finding.severity}</span></div>
					<div class="fd-row"><span class="fd-label">finding_type</span><span class="fd-value">{finding.finding_type}</span></div>
					<div class="fd-row"><span class="fd-label">title</span><span class="fd-value">{finding.title}</span></div>
					<div class="fd-row"><span class="fd-label">description</span><span class="fd-value">{finding.description || '—'}</span></div>
					<div class="fd-row"><span class="fd-label">suggestion</span><span class="fd-value">{finding.suggestion || '—'}</span></div>
					<div class="fd-row"><span class="fd-label">confidence</span><span class="fd-value">{formatConfidence(finding.confidence)}</span></div>
					<div class="fd-row">
						<span class="fd-label">metadata</span>
						<button type="button" class="fd-btn" onclick={openMetadataDialog}>View metadata</button>
					</div>
					<div class="fd-row"><span class="fd-label">location</span><span class="fd-value">{finding.location || '—'}</span></div>
					<div class="fd-row"><span class="fd-label">reference_doc</span><span class="fd-value">{formatCompactContent(finding.reference_doc)}</span></div>
				</div>
			{/if}
		</div>

		{#if finding.artifact_id}
			<!-- Artifact block -->
			<div class="fd-block">
				<button type="button" class="fd-head" aria-expanded={expanded.artifact} onclick={() => toggle('artifact')}>
					<span class="fd-chevron" class:open={expanded.artifact}>▶</span>
					<span class="fd-title">Artifact</span>
				</button>
				{#if expanded.artifact}
					<div class="fd-body">
						{#if artifactLoading}
							<div class="fd-note">Loading…</div>
						{:else if artifactError}
							<div class="fd-note fd-error">{artifactError}</div>
						{:else if artifactRecord}
							{#each buildJsonSections(artifactRecord)[0]?.rows ?? [] as row (row.label)}
								<div class="fd-row"><span class="fd-label">{row.label}</span><span class="fd-value">{row.value}</span></div>
							{/each}
						{:else}
							<div class="fd-note">No artifact found for this id.</div>
						{/if}
					</div>
				{/if}
			</div>

			<!-- Doc Review Log block -->
			<div class="fd-block">
				<button type="button" class="fd-head" aria-expanded={expanded.log} onclick={() => toggle('log')}>
					<span class="fd-chevron" class:open={expanded.log}>▶</span>
					<span class="fd-title">Doc Review Log</span>
				</button>
				{#if expanded.log}
					<div class="fd-body">
						{#if logLoading}
							<div class="fd-note">Loading…</div>
						{:else if logError}
							<div class="fd-note fd-error">{logError}</div>
						{:else if logRow}
							<div class="fd-row"><span class="fd-label">unit_key</span><span class="fd-value">{logRow.unit_key}</span></div>
							<div class="fd-row"><span class="fd-label">outcome</span><span class="fd-value">{logRow.outcome || '—'}</span></div>
							<div class="fd-row">
								<span class="fd-label">matched_units</span>
								{#if matchedUnits.length}
									<div class="fd-unit-btns">
										{#each matchedUnits as unit, i (i)}
											<button type="button" class="fd-btn" onclick={() => openMatchedUnit(i)}>{matchedUnitLabel(unit, i)}</button>
										{/each}
									</div>
								{:else}
									<span class="fd-value">—</span>
								{/if}
							</div>
							<div class="fd-row">
								<span class="fd-label">findings</span>
								<button type="button" class="fd-btn" onclick={() => openLogSections('findings')}>View findings</button>
							</div>
							<div class="fd-row">
								<span class="fd-label">detail</span>
								<button type="button" class="fd-btn" onclick={() => openLogSections('detail')}>View detail</button>
							</div>
						{:else}
							<div class="fd-note">No matching doc review log found.</div>
						{/if}
					</div>
				{/if}
			</div>

			<!-- LLM Calls block -->
			<div class="fd-block">
				<button type="button" class="fd-head" aria-expanded={expanded.llm} onclick={() => toggle('llm')}>
					<span class="fd-chevron" class:open={expanded.llm}>▶</span>
					<span class="fd-title">LLM Calls</span>
					{#if llmEvents.length}<span class="fd-count">{llmEvents.length}</span>{/if}
				</button>
				{#if expanded.llm}
					<div class="fd-body">
						{#if llmLoading}
							<div class="fd-note">Loading…</div>
						{:else if llmError}
							<div class="fd-note fd-error">{llmError}</div>
						{:else if llmEvents.length}
							<ul class="fd-llm-list">
								{#each llmEvents as event (event.id)}
									<li>
										<button type="button" class="fd-llm-item" onclick={() => openLlmEventDialog(event)}>
											<span class="fd-llm-id">{event.id}</span>
											<span class="fd-llm-model">{event.model_name}</span>
											<span class="fd-llm-time">{event.request_started_at}</span>
										</button>
									</li>
								{/each}
							</ul>
						{:else}
							<div class="fd-note">No LLM calls recorded for this finding.</div>
						{/if}
					</div>
				{/if}
			</div>
		{/if}
	{/if}
</div>

{#if dialog?.kind === 'tree'}
	<JsonTreeDialog title={dialog.title} nodes={dialog.nodes} {dark} onclose={closeDialog} />
{:else if dialog?.kind === 'sections'}
	<JsonSectionsDialog title={dialog.title} sections={dialog.sections} {dark} onclose={closeDialog} />
{:else if dialog?.kind === 'matched-units'}
	<MatchedUnitsDialog
		title={dialog.title}
		units={dialog.units}
		initialSelected={dialog.initialSelected}
		{dark}
		onclose={closeDialog}
		onFocusUnit={onFocusMatchedUnit}
	/>
{:else if dialog?.kind === 'llm-event'}
	<LlmUsageEventDialog event={dialog.event} {dark} onclose={closeDialog} />
{/if}

<style>
	.fd-panel {
		height: 100%;
		overflow-y: auto;
		overflow-x: hidden;
		padding: 0.75rem;
		color: var(--fd-text);
		font-family: system-ui, -apple-system, 'Segoe UI', Roboto, sans-serif;
		scrollbar-width: thin;
		scrollbar-color: var(--fd-scroll-thumb) transparent;
	}
	.fd-panel::-webkit-scrollbar {
		width: 6px;
	}
	.fd-panel::-webkit-scrollbar-thumb {
		background: var(--fd-scroll-thumb);
		border-radius: 999px;
	}
	.fd-panel::-webkit-scrollbar-track {
		background: transparent;
	}
	.fd-empty {
		padding: 1.5rem 0.5rem;
		color: var(--fd-text-2);
		font-size: 0.85rem;
	}
	.fd-block {
		margin: 0 0 0.6rem;
	}
	.fd-head {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		width: 100%;
		text-align: left;
		background: var(--fd-card);
		border: 1px solid var(--fd-border);
		border-radius: 8px;
		padding: 0.55rem 0.75rem;
		cursor: pointer;
		color: var(--fd-accent);
		font: inherit;
		font-size: 0.92rem;
		font-weight: 600;
	}
	.fd-chevron {
		flex: 0 0 auto;
		font-size: 0.65rem;
		color: var(--fd-text-2);
		transition: transform 0.15s;
	}
	.fd-chevron.open {
		transform: rotate(90deg);
	}
	.fd-title {
		flex: 1 1 auto;
	}
	.fd-count {
		font-size: 0.68rem;
		font-weight: 600;
		color: var(--fd-text-2);
		background: var(--fd-accent-tint);
		border-radius: 999px;
		padding: 0.08rem 0.5rem;
	}
	.fd-body {
		padding: 0.5rem 0.25rem 0.25rem 0.75rem;
		border-left: 2px solid var(--fd-border);
		margin: 0.3rem 0 0 0.45rem;
	}
	.fd-row {
		display: grid;
		grid-template-columns: minmax(90px, 130px) minmax(0, 1fr);
		gap: 0.5rem;
		padding: 0.28rem 0;
		font-size: 0.8rem;
		align-items: start;
	}
	.fd-label {
		color: var(--fd-text-muted);
		font-family: monospace;
		word-break: break-word;
	}
	.fd-value {
		color: var(--fd-text-2);
		word-break: break-word;
		white-space: pre-wrap;
	}
	.fd-unit-btns {
		display: flex;
		flex-wrap: wrap;
		gap: 0.3rem;
		justify-self: start;
	}
	.fd-btn {
		justify-self: start;
		padding: 0.22rem 0.6rem;
		border: 1px solid var(--fd-border);
		border-radius: 6px;
		background: var(--fd-card);
		color: var(--fd-accent);
		font: inherit;
		font-size: 0.74rem;
		cursor: pointer;
	}
	.fd-btn:hover {
		border-color: var(--fd-accent);
	}
	.fd-note {
		font-size: 0.8rem;
		color: var(--fd-text-muted);
		padding: 0.3rem 0;
	}
	.fd-note.fd-error {
		color: #dc2626;
	}
	.fd-llm-list {
		list-style: none;
		margin: 0;
		padding: 0;
	}
	.fd-llm-item {
		display: flex;
		flex-wrap: wrap;
		gap: 0.5rem;
		width: 100%;
		text-align: left;
		background: transparent;
		border: 1px solid var(--fd-border);
		border-radius: 6px;
		padding: 0.4rem 0.6rem;
		margin: 0.25rem 0;
		cursor: pointer;
		color: var(--fd-text-2);
		font: inherit;
		font-size: 0.76rem;
	}
	.fd-llm-item:hover {
		border-color: var(--fd-accent);
	}
	.fd-llm-id {
		font-family: monospace;
		color: var(--fd-accent);
	}
</style>
