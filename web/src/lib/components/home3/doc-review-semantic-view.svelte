<script lang="ts">
	import { onMount } from 'svelte';
	import RefreshCwIcon from '@lucide/svelte/icons/refresh-cw';
	import { listAssertions, type Assertion } from './semantic-assertions-client';
	import { listEvidence, type Evidence } from './assertion-evidence-client';
	import {
		stateLabel,
		stateSeverity,
		statusLabel,
		statusSeverity,
		SEVERITY_COLORS
	} from './semantic-diagnostics-labels';
	import JsonTreeViewer from './json-tree-viewer.svelte';

	let {
		darkMode = true,
		inputRecordId = 0,
		embedded = false
	}: {
		darkMode: boolean;
		inputRecordId: number;
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

	let rows = $state<Assertion[]>([]);
	let evidenceByAssertion = $state<Map<number, Evidence[]>>(new Map());
	let total = $state(0);
	let page = $state(1);
	let pageSize = $state(50);
	let loading = $state(false);
	let error = $state('');
	let expandedErrors = $state<Set<number>>(new Set());

	let totalPages = $derived(Math.max(1, Math.ceil(total / pageSize)));

	// This tab is a read-only diagnostic projection: it surfaces every
	// lifecycle state a semantic assertion can carry -- including raw-preserved,
	// unresolved-mapping, and unsupported instances -- and never presents a
	// non-empty finding list as a document processing failure (ADR 2026081801
	// D6/DR11; spec semantic-consumer-compatibility "Findings are not shown as
	// document failures").
	let withProcessingErrors = $derived(rows.filter((r) => r.processing_error_details != null).length);
	let byStatus = $derived.by(() => {
		const counts = new Map<string, number>();
		for (const r of rows) counts.set(r.status, (counts.get(r.status) ?? 0) + 1);
		return [...counts.entries()].sort((a, b) => b[1] - a[1]);
	});

	async function load() {
		if (!inputRecordId) return;
		loading = true;
		error = '';
		try {
			const [assertionsResult, evidenceResult] = await Promise.all([
				listAssertions(
					{
						status: '',
						logical_identity: '',
						subject_ref_kind: '',
						subject_ref_id: '',
						predicate_term_id: '',
						object_ref_id: '',
						subject_object_id: '',
						latest_only: true,
						input_record_id: String(inputRecordId)
					},
					page,
					pageSize,
					'modified',
					'desc'
				),
				listEvidence(
					{
						assertion_id: '',
						input_record_id: String(inputRecordId),
						artifact_type: '',
						artifact_id: '',
						evidence_role: '',
						actor_kind: '',
						include_deleted: false
					},
					1,
					500
				)
			]);
			rows = assertionsResult.results;
			total = assertionsResult.total;
			const grouped = new Map<number, Evidence[]>();
			for (const e of evidenceResult.results) {
				const list = grouped.get(e.assertion_id) ?? [];
				list.push(e);
				grouped.set(e.assertion_id, list);
			}
			evidenceByAssertion = grouped;
		} catch (e) {
			error = e instanceof Error ? e.message : String(e);
			rows = [];
			total = 0;
			evidenceByAssertion = new Map();
		} finally {
			loading = false;
		}
	}

	function toggleErrorDetails(id: number) {
		const next = new Set(expandedErrors);
		if (next.has(id)) next.delete(id);
		else next.add(id);
		expandedErrors = next;
	}

	function claimText(a: Assertion): string {
		const object = a.object_literal !== undefined && a.object_literal !== null
			? typeof a.object_literal === 'object'
				? JSON.stringify(a.object_literal)
				: String(a.object_literal)
			: (a.object_ref_id ?? '');
		return `${a.subject_ref_kind}:${a.subject_ref_id} — ${a.predicate_term_id} — ${object || '(no object)'}`;
	}

	function normalizedValueText(a: Assertion): string {
		if (a.lower_value != null || a.upper_value != null) {
			const lo = a.lower_value != null ? a.lower_value : '…';
			const hi = a.upper_value != null ? a.upper_value : '…';
			const cmp = a.comparator ? `${a.comparator} ` : '';
			return `${cmp}[${lo}, ${hi}]${a.unit_term_id ? ` ${a.unit_term_id}` : ''}`;
		}
		if (a.numeric_value != null) {
			return `${a.numeric_value}${a.unit_term_id ? ` ${a.unit_term_id}` : ''}`;
		}
		return '—';
	}

	$effect(() => {
		if (inputRecordId) {
			page = 1;
			load();
		}
	});
</script>

<div
	class="flex h-full flex-col space-y-4 overflow-hidden"
	style="background:{embedded ? 'transparent' : pageBg}; padding:{embedded ? '0' : '1.5rem'}; user-select:text; -webkit-user-select:text"
>
	<div class="flex-shrink-0 rounded-xl p-5" style="background:{cardBg};border:1px solid {borderColor}">
		<div class="flex flex-wrap items-start justify-between gap-3">
			<div>
				<h2 style="font-size:18px;font-weight:600;color:{textPrimary}">Semantic Diagnostics</h2>
				<p style="font-size:13px;color:{textSecondary};margin-top:2px;max-width:640px">
					Every semantic assertion with active evidence in this document, including raw-preserved,
					unresolved-mapping, ambiguous, and unsupported instances. This is a diagnostic view, not
					a governance judgment — a non-empty list of findings below is expected output, not a
					processing failure.
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
		{#if rows.length > 0 || total > 0}
			<div class="mt-4 flex flex-wrap items-center gap-2" style="font-size:12px;">
				<span style="color:{textMuted}">{total} assertion{total !== 1 ? 's' : ''}</span>
				{#each byStatus as [status, count]}
					{@const colors = SEVERITY_COLORS[statusSeverity(status)]}
					<span
						style="padding:0.15rem 0.55rem;border-radius:999px;background:{colors.bg};color:{colors.fg};font-weight:600"
						>{statusLabel(status)}: {count}</span
					>
				{/each}
				{#if withProcessingErrors > 0}
					<span style="color:{textMuted}"
						>· {withProcessingErrors} with processing error detail</span
					>
				{/if}
			</div>
		{/if}
	</div>

	{#if error}
		<div
			class="flex-shrink-0 rounded-xl p-4"
			style="background:{danger}20;border:1px solid {danger}70;color:{danger};font-size:13px"
		>
			{error}
		</div>
	{/if}

	<div class="flex min-h-0 flex-1 flex-col rounded-xl" style="background:{cardBg};border:1px solid {borderColor}">
		<div class="flex flex-shrink-0 justify-between px-5 py-3" style="border-bottom:1px solid {borderColor}">
			<span style="font-size:13px;color:{textMuted}"
				>{total} assertion{total !== 1 ? 's' : ''}{#if total}
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
				No semantic assertions with active evidence in this document yet.
			</div>
		{:else}
			<div class="min-h-0 flex-1 space-y-3 overflow-auto p-4">
				{#each rows as a (a.id)}
					{@const evidence = evidenceByAssertion.get(a.id) ?? []}
					{@const statusColors = SEVERITY_COLORS[statusSeverity(a.status)]}
					<div class="rounded-lg p-4" style="background:{surface2};border:1px solid {borderColor}">
						<div class="flex flex-wrap items-start justify-between gap-2">
							<div style="min-width:0;flex:1">
								<div style="font-size:13px;color:{textPrimary};font-weight:500;word-break:break-word">
									{claimText(a)}
								</div>
								<div style="font-size:11px;color:{textMuted};margin-top:2px;font-family:monospace">
									#{a.id} · rev {a.revision}
								</div>
							</div>
							<div class="flex flex-wrap items-center gap-1.5">
								<span
									style="padding:0.15rem 0.55rem;border-radius:999px;background:{statusColors.bg};color:{statusColors.fg};font-size:11px;font-weight:600"
									>{statusLabel(a.status)}</span
								>
								{#if a.unsupported_prior_status}
									<span style="font-size:11px;color:{textMuted}"
										>(was {statusLabel(a.unsupported_prior_status)})</span
									>
								{/if}
							</div>
						</div>

						<div class="mt-3 grid gap-3" style="grid-template-columns:repeat(auto-fit,minmax(220px,1fr))">
							<div>
								<div style="font-size:11px;color:{textMuted};text-transform:uppercase;letter-spacing:0.04em">
									Raw value
								</div>
								<div style="font-size:12.5px;color:{textPrimary};margin-top:2px;word-break:break-word">
									{a.raw_text || '—'}
								</div>
							</div>
							<div>
								<div style="font-size:11px;color:{textMuted};text-transform:uppercase;letter-spacing:0.04em">
									Normalized value
								</div>
								<div style="font-size:12.5px;color:{textPrimary};margin-top:2px">
									{normalizedValueText(a)}
								</div>
							</div>
							<div>
								<div style="font-size:11px;color:{textMuted};text-transform:uppercase;letter-spacing:0.04em">
									Class confidence
								</div>
								<div style="font-size:12.5px;color:{textPrimary};margin-top:2px">
									{a.confidence != null ? a.confidence.toFixed(2) : '—'}
								</div>
							</div>
						</div>

						<div class="mt-3 flex flex-wrap gap-1.5">
							{#each [['Class identity', a.class_identity_state_term_id], ['Mapping', a.mapping_resolution_state_term_id], ['Value', a.value_state_term_id], ['Conformance', a.conformance_state_term_id]] as [label, term]}
								{#if term}
									{@const colors = SEVERITY_COLORS[stateSeverity(term)]}
									<span
										style="padding:0.15rem 0.5rem;border-radius:6px;background:{colors.bg};color:{colors.fg};font-size:11px"
										title={term}><strong>{label}:</strong> {stateLabel(term)}</span
									>
								{/if}
							{/each}
						</div>

						{#if a.processing_error_details != null}
							<div class="mt-3">
								<button
									onclick={() => toggleErrorDetails(a.id)}
									class="cursor-pointer rounded px-2 py-1 text-xs"
									style="background:{danger}18;color:{danger};border:1px solid {danger}50"
								>
									{expandedErrors.has(a.id) ? 'Hide' : 'Show'} processing error detail
								</button>
								{#if expandedErrors.has(a.id)}
									<div class="mt-2 rounded p-2" style="background:{cardBg};border:1px solid {borderColor}">
										<JsonTreeViewer value={a.processing_error_details} depth={0} {darkMode} />
									</div>
								{/if}
							</div>
						{/if}

						{#if evidence.length > 0}
							<div class="mt-3">
								<div style="font-size:11px;color:{textMuted};text-transform:uppercase;letter-spacing:0.04em">
									Active evidence ({evidence.length})
								</div>
								<div class="mt-1.5 space-y-1.5">
									{#each evidence as e (e.id)}
										<div
											class="rounded p-2"
											style="background:{cardBg};border:1px solid {borderColor};font-size:12px"
										>
											<div style="color:{textSecondary};display:flex;gap:0.5rem;flex-wrap:wrap;align-items:baseline">
												<span style="font-family:monospace;color:{textMuted}"
													>{e.artifact_type}:{e.artifact_id}</span
												>
												<span
													style="color:{e.evidence_role === 'contradicts' ? danger : textSecondary}"
													>{e.evidence_role}</span
												>
												{#if e.confidence != null}
													<span style="color:{textMuted}">conf {e.confidence.toFixed(2)}</span>
												{/if}
											</div>
											{#if e.evidence_quote}
												<div style="color:{textPrimary};margin-top:2px">"{e.evidence_quote}"</div>
											{/if}
										</div>
									{/each}
								</div>
							</div>
						{:else}
							<div class="mt-3" style="font-size:11.5px;color:{textMuted}">No active evidence.</div>
						{/if}
					</div>
				{/each}
			</div>
		{/if}
	</div>
</div>
