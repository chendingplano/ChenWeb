<script lang="ts">
	// The compliance part of the Finding Details panel (openspec compliance-review-storage):
	// content, activities and judged candidates load when a finding is selected; a candidate's
	// evidence when it is expanded; search history and results only when asked for.
	import { m } from '$lib/paraglide/messages.js';
	import { getLocale } from '$lib/paraglide/runtime';
	import {
		exportFinding,
		findingExportUrl,
		getComplianceCandidate,
		getFindingDetails,
		listFindingActivities,
		listFindingCandidates,
		listFindingSearches,
		listSearchResults,
		type CandidateFilter,
		type ComplianceActivitySummary,
		type ComplianceCandidateDetail,
		type ComplianceCandidateSummary,
		type ComplianceSearchResult,
		type ComplianceSearchSummary,
		type FindingDetails,
		type FindingItem
	} from '$lib/services/docReviewService';
	import { cacheDetails, cachedDetails, complianceSearchCounts, detailsCacheKey, readableActivityLabel, verdictCounts, withoutActivityLines } from './compliance-finding-panel.js';

	let {
		finding,
		onOpenCandidate,
		onViewAudit
	}: {
		finding: FindingItem;
		onOpenCandidate?: (recordId: number, artifactId: string) => void;
		onViewAudit?: (title: string, record: unknown) => void;
	} = $props();

	let loading = $state(false);
	let error = $state('');
	let details = $state<FindingDetails | null>(null);
	let activities = $state<ComplianceActivitySummary[]>([]);
	let activityCursor = $state<string | undefined>(undefined);
	let candidates = $state<ComplianceCandidateSummary[]>([]);
	let candidateTotal = $state(0);
	let candidateCursor = $state<string | undefined>(undefined);
	let filter = $state<CandidateFilter>('judged');
	let moreBusy = $state(false);
	let expanded = $state<Record<number, ComplianceCandidateDetail | 'loading' | string>>({});
	let searches = $state<ComplianceSearchSummary[] | null>(null);
	let searchesBusy = $state(false);
	let openSearch = $state<number | null>(null);
	let results = $state<ComplianceSearchResult[]>([]);
	let resultCursor = $state<string | undefined>(undefined);
	let auditBusy = $state(false);

	const VERDICTS = ['compliant', 'compatible', 'not_compliant', 'undetermined', 'unqualified'];
	function verdictLabel(v: string): string {
		switch (v) {
			case 'compliant':
				return m.finding_details_panel_verdict_compliant();
			case 'compatible':
				return m.finding_details_panel_verdict_compatible();
			case 'not_compliant':
				return m.finding_details_panel_verdict_not_compliant();
			case 'undetermined':
				return m.finding_details_panel_verdict_undetermined();
			case 'unqualified':
				return m.finding_details_panel_verdict_unqualified();
		}
		return v;
	}
	function verdictHint(v: string): string {
		switch (v) {
			case 'compliant':
				return m.finding_details_panel_verdict_compliant_hint();
			case 'compatible':
				return m.finding_details_panel_verdict_compatible_hint();
			case 'not_compliant':
				return m.finding_details_panel_verdict_not_compliant_hint();
			case 'undetermined':
				return m.finding_details_panel_verdict_undetermined_hint();
			case 'unqualified':
				return m.finding_details_panel_verdict_unqualified_hint();
		}
		return '';
	}
	function statusLabel(s: string): string {
		switch (s) {
			case 'rejected':
				return m.compliance_finding_details_status_rejected();
			case 'relevance_failed':
				return m.compliance_finding_details_status_relevance_failed();
			case 'pending':
				return m.compliance_finding_details_status_pending();
			case 'judgement_failed':
				return m.compliance_finding_details_status_judgement_failed();
		}
		return s;
	}

	let storedCounts = $derived(details?.finding.counts ?? finding.counts);
	let counts = $derived(complianceSearchCounts(storedCounts));
	let verdictSummary = $derived(
		verdictCounts(storedCounts)
			.map((v) => m.compliance_finding_details_verdict_count({ count: v.count, verdict: verdictLabel(v.verdict) }))
			.join(m.compliance_finding_details_list_separator())
	);
	let description = $derived(
		details?.content ? details.content.description : details?.legacy ? withoutActivityLines(details.legacy.description) : ''
	);
	let errorMessage = $derived(details?.content?.error_message || details?.legacy?.error_msg || '');
	let suggestion = $derived(details?.content?.suggestion || details?.legacy?.suggestion || '');
	let group = $derived(activities.length > 1 || (storedCounts?.activities ?? 0) > 1);

	async function load(f: FindingItem, mode: CandidateFilter) {
		const language = getLocale();
		const key = detailsCacheKey(f.id, language, mode, f.review_status);
		error = '';
		expanded = {};
		searches = null;
		openSearch = null;
		let d = cachedDetails(key);
		if (!d) {
			loading = true;
			try {
				d = await getFindingDetails(f.id, { language, filter: mode });
				cacheDetails(key, d);
			} catch (e) {
				error = e instanceof Error ? e.message : String(e);
				loading = false;
				return;
			}
			loading = false;
		}
		if (finding.id !== f.id) return; // another finding was selected meanwhile
		details = d;
		activities = d.activities?.items ?? [];
		activityCursor = d.activities?.next_cursor;
		candidates = d.candidates?.items ?? [];
		candidateTotal = d.candidates?.total ?? 0;
		candidateCursor = d.candidates?.next_cursor;
	}

	$effect(() => {
		const f = finding;
		details = null;
		activities = [];
		candidates = [];
		filter = 'judged';
		void load(f, 'judged');
	});

	function setFilter(next: CandidateFilter) {
		if (next === filter) return;
		filter = next;
		void load(finding, next);
	}

	async function moreActivities() {
		if (!activityCursor) return;
		moreBusy = true;
		try {
			const page = await listFindingActivities(finding.id, activityCursor);
			activities = [...activities, ...page.items];
			activityCursor = page.next_cursor;
		} catch (e) {
			error = e instanceof Error ? e.message : String(e);
		} finally {
			moreBusy = false;
		}
	}

	async function moreCandidates() {
		if (!candidateCursor) return;
		moreBusy = true;
		try {
			const page = await listFindingCandidates(finding.id, filter, candidateCursor);
			candidates = [...candidates, ...page.items];
			candidateCursor = page.next_cursor;
		} catch (e) {
			error = e instanceof Error ? e.message : String(e);
		} finally {
			moreBusy = false;
		}
	}

	async function toggleCandidate(c: ComplianceCandidateSummary) {
		if (expanded[c.id] !== undefined) {
			const { [c.id]: _, ...rest } = expanded;
			expanded = rest;
			return;
		}
		expanded = { ...expanded, [c.id]: 'loading' };
		try {
			const d = await getComplianceCandidate(c.id);
			expanded = { ...expanded, [c.id]: d };
		} catch (e) {
			expanded = { ...expanded, [c.id]: e instanceof Error ? e.message : String(e) };
		}
	}

	async function toggleSearches() {
		if (searches) {
			searches = null;
			openSearch = null;
			return;
		}
		searchesBusy = true;
		try {
			searches = await listFindingSearches(finding.id);
		} catch (e) {
			error = e instanceof Error ? e.message : String(e);
		} finally {
			searchesBusy = false;
		}
	}

	async function showResults(s: ComplianceSearchSummary, more = false) {
		if (!more && openSearch === s.id) {
			openSearch = null;
			return;
		}
		searchesBusy = true;
		try {
			const page = await listSearchResults(s.id, more ? resultCursor : undefined);
			results = more ? [...results, ...page.items] : page.items;
			resultCursor = page.next_cursor;
			openSearch = s.id;
		} catch (e) {
			error = e instanceof Error ? e.message : String(e);
		} finally {
			searchesBusy = false;
		}
	}

	async function viewAudit() {
		auditBusy = true;
		try {
			const record = await exportFinding(finding.id);
			onViewAudit?.(m.compliance_finding_details_audit_title({ id: finding.id }), record);
		} catch (e) {
			error = e instanceof Error ? e.message : String(e);
		} finally {
			auditBusy = false;
		}
	}

	function evidenceText(e: Record<string, unknown>, key: string): string {
		const v = e[key];
		return typeof v === 'string' ? v : '';
	}
	function verification(e: Record<string, unknown>): string {
		const v = e.verification as { valid?: boolean; reason?: string } | undefined;
		if (!v?.reason) return '';
		if (v.valid === false) return m.compliance_finding_details_verification_rejected({ reason: v.reason });
		return v.reason;
	}
</script>

{#if loading && !details}
	<div class="cf-row"><span class="cf-note">{m.finding_details_panel_loading()}</span></div>
{/if}
{#if error}
	<div class="cf-row"><span class="cf-note cf-error">{error}</span></div>
{/if}

{#if activities.length}
	<div class="cf-row">
		<span class="cf-label">{m.finding_details_panel_activity()}</span>
		<div class="cf-value">
			{#each activities as a (a.activity_id)}
				<div>{m.finding_details_panel_activity_value({ label: readableActivityLabel(a.label), kind: a.kind, start: a.line_start ?? 0, end: a.line_end ?? 0 })}</div>
			{/each}
			{#if activityCursor}
				<button type="button" class="cf-btn" disabled={moreBusy} onclick={moreActivities}>{m.compliance_finding_details_more_activities()}</button>
			{/if}
		</div>
	</div>
{/if}

<div class="cf-row">
	<span class="cf-label">{m.finding_details_panel_text()}</span>
	<div class="cf-value">
		{#if counts}
			<div>{m.finding_details_panel_searched({ count: counts.searched })}</div>
			<div>{m.finding_details_panel_selected({ count: counts.selected, plural: counts.selected === 1 ? '' : 's' })}</div>
			<div>
				{m.finding_details_panel_judged_relevance({
					relevant: counts.relevant,
					relevantPlural: counts.relevant === 1 ? '' : 's',
					irrelevant: counts.irrelevant,
					irrelevantPlural: counts.irrelevant === 1 ? '' : 's'
				})}
			</div>
			{#if verdictSummary}<div>{m.compliance_finding_details_verdicts({ list: verdictSummary })}</div>{/if}
		{/if}
		{#if description}<div class="cf-prose">{description}</div>{/if}
		{#if !counts && !description && details}<div>—</div>{/if}
	</div>
</div>
{#if errorMessage}
	<div class="cf-row"><span class="cf-label">{m.compliance_finding_details_error()}</span><span class="cf-value cf-error cf-prose">{errorMessage}</span></div>
{/if}
<div class="cf-row"><span class="cf-label">{m.finding_details_panel_suggestion()}</span><span class="cf-value">{suggestion || '—'}</span></div>
<div class="cf-row"><span class="cf-label">{m.finding_details_panel_confidence()}</span><span class="cf-value">{Math.round(finding.confidence * 100)}%</span></div>
<div class="cf-row">
	<span class="cf-label">{m.compliance_finding_details_audit()}</span>
	<div class="cf-actions">
		<button type="button" class="cf-btn" disabled={auditBusy} onclick={viewAudit}>{m.compliance_finding_details_view_audit()}</button>
		<a class="cf-btn" href={findingExportUrl(finding.id)} download>{m.compliance_finding_details_export()}</a>
	</div>
</div>

{#if details?.content}
	<div class="cf-row">
		<span class="cf-label">{m.finding_details_panel_candidates()}</span>
		<div class="cf-value">
			<div class="cf-actions">
				<button type="button" class="cf-chip" class:on={filter === 'judged'} onclick={() => setFilter('judged')}>{m.compliance_finding_details_filter_judged()}</button>
				<button type="button" class="cf-chip" class:on={filter === 'all'} onclick={() => setFilter('all')}>{m.compliance_finding_details_filter_all()}</button>
				<span class="cf-note">{m.compliance_finding_details_shown({ shown: candidates.length, total: candidateTotal })}</span>
			</div>
			{#each candidates as c (c.id)}
				{@const open = expanded[c.id]}
				<div class="cf-cand">
					<button type="button" class="cf-cand-head" aria-expanded={open !== undefined} onclick={() => toggleCandidate(c)}>
						{#if c.verdict && VERDICTS.includes(c.verdict)}
							<span class="cf-verdict cf-verdict-{c.verdict}" title={verdictHint(c.verdict)}>{verdictLabel(c.verdict)}</span>
						{:else}
							<span class="cf-status">{statusLabel(c.status)}</span>
						{/if}
						<span class="cf-ref">{m.finding_details_panel_record({ related_record_id: c.candidate_record_id ?? '—', related_artifact_id: c.candidate_id })}</span>
					</button>
					{#if group}<div class="cf-sub">{m.compliance_finding_details_for_activity({ label: c.local_label, activity: readableActivityLabel(c.activity_label) })}</div>{/if}
					{#if c.document_title}<div class="cf-sub">{c.document_title}</div>{/if}
					{#if c.summary}<p class="cf-summary">{c.summary}</p>{/if}
					{#if c.note}<p class="cf-sub"><span class="cf-strong">{m.finding_details_panel_verdict_note()}</span> {c.note}</p>{/if}
					{#if !c.verdict && c.relevance_answer}
						<div class="cf-sub">{m.compliance_finding_details_relevance({ answer: c.relevance_answer, score: c.relevance_score ?? '—' })}</div>
					{/if}
					{#if open === 'loading'}
						<div class="cf-note">{m.finding_details_panel_loading()}</div>
					{:else if typeof open === 'string'}
						<div class="cf-note cf-error">{open}</div>
					{:else if open}
						<div class="cf-evidence">
							{#if evidenceText(open.evidence, 'candidate_quote')}
								<div><span class="cf-strong">{m.compliance_finding_details_candidate_quote()}</span> {evidenceText(open.evidence, 'candidate_quote')}</div>
							{/if}
							{#if evidenceText(open.evidence, 'document_quote')}
								<div>
									<span class="cf-strong">{m.compliance_finding_details_document_quote({ start: String(open.evidence.document_line_start ?? '—'), end: String(open.evidence.document_line_end ?? '—') })}</span>
									{evidenceText(open.evidence, 'document_quote')}
								</div>
							{/if}
							{#if evidenceText(open.evidence, 'numeric_result')}
								<div><span class="cf-strong">{m.compliance_finding_details_numeric()}</span> {evidenceText(open.evidence, 'numeric_result')}</div>
							{/if}
							{#if evidenceText(open.evidence, 'draft_verdict') && evidenceText(open.evidence, 'draft_verdict') !== open.verdict}
								<div><span class="cf-strong">{m.compliance_finding_details_draft_verdict()}</span> {verdictLabel(evidenceText(open.evidence, 'draft_verdict'))}</div>
							{/if}
							{#if verification(open.evidence)}
								<div><span class="cf-strong">{m.compliance_finding_details_verification()}</span> {verification(open.evidence)}</div>
							{/if}
							{#if evidenceText(open.evidence, 'candidate_excerpt')}
								<div><span class="cf-strong">{m.compliance_finding_details_excerpt()}</span> {evidenceText(open.evidence, 'candidate_excerpt')}</div>
							{/if}
							{#if open.error_message}<div class="cf-error">{open.error_message}</div>{/if}
							{#if c.candidate_record_id}
								<button type="button" class="cf-btn" onclick={() => onOpenCandidate?.(c.candidate_record_id ?? 0, c.candidate_id)}>
									{m.finding_details_panel_jump_to_this_related_artifact()}
								</button>
							{/if}
						</div>
					{/if}
				</div>
			{/each}
			{#if candidateCursor}
				<button type="button" class="cf-btn" disabled={moreBusy} onclick={moreCandidates}>{m.compliance_finding_details_more_candidates()}</button>
			{/if}
		</div>
	</div>

	<div class="cf-row">
		<span class="cf-label">{m.compliance_finding_details_search_history()}</span>
		<div class="cf-value">
			<button type="button" class="cf-btn" disabled={searchesBusy} onclick={toggleSearches}>
				{searches ? m.compliance_finding_details_hide_searches() : m.compliance_finding_details_show_searches()}
			</button>
			{#if searches}
				{#each searches as s (s.id)}
					<div class="cf-cand">
						<button type="button" class="cf-cand-head" aria-expanded={openSearch === s.id} onclick={() => showResults(s)}>
							<span class="cf-status">{s.purpose}</span>
							<span class="cf-ref">{m.compliance_finding_details_search_line({ label: s.local_label, count: s.result_count, limit: s.result_limit })}</span>
						</button>
						<div class="cf-sub">{s.query || s.reference || s.cited_document || '—'}</div>
						{#if s.potentially_truncated}<div class="cf-sub">{m.compliance_finding_details_truncated()}</div>{/if}
						{#if openSearch === s.id}
							<ol class="cf-results">
								{#each results as r (r.rank)}
									<li>{m.compliance_finding_details_result_line({ type: r.candidate_type, id: r.candidate_id, record: r.candidate_record_id ?? '—', score: r.score?.toFixed(4) ?? '—' })}</li>
								{/each}
							</ol>
							{#if resultCursor}
								<button type="button" class="cf-btn" disabled={searchesBusy} onclick={() => showResults(s, true)}>{m.compliance_finding_details_more_results()}</button>
							{/if}
						{/if}
					</div>
				{/each}
			{/if}
		</div>
	</div>
{/if}

<style>
	.cf-row {
		display: grid;
		grid-template-columns: 7.5rem 1fr;
		gap: 0.5rem;
		padding: 0.3rem 0;
		font-size: 0.8rem;
		border-bottom: 1px dashed var(--fd-border);
	}
	.cf-label {
		color: var(--fd-text-2);
	}
	.cf-value {
		color: var(--fd-text);
		min-width: 0;
		word-break: break-word;
	}
	.cf-prose {
		white-space: pre-wrap;
	}
	.cf-note {
		color: var(--fd-text-muted);
		font-size: 0.78rem;
	}
	.cf-error {
		color: #ef4444;
	}
	.cf-actions {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 0.4rem;
		margin-bottom: 0.3rem;
	}
	.cf-btn,
	.cf-chip {
		background: transparent;
		border: 1px solid var(--fd-border);
		border-radius: 6px;
		color: var(--fd-accent);
		font-size: 0.75rem;
		padding: 0.15rem 0.5rem;
		cursor: pointer;
		text-decoration: none;
	}
	.cf-btn:disabled {
		opacity: 0.6;
		cursor: default;
	}
	.cf-chip.on {
		background: var(--fd-accent-tint);
	}
	.cf-cand {
		border: 1px solid var(--fd-border);
		border-radius: 6px;
		padding: 0.35rem 0.5rem;
		margin: 0.3rem 0;
	}
	.cf-cand-head {
		display: flex;
		align-items: center;
		gap: 0.4rem;
		width: 100%;
		background: transparent;
		border: none;
		padding: 0;
		color: inherit;
		text-align: left;
		cursor: pointer;
		font-size: 0.78rem;
	}
	.cf-ref {
		color: var(--fd-text-2);
	}
	.cf-sub {
		color: var(--fd-text-2);
		font-size: 0.75rem;
		margin: 0.15rem 0 0;
	}
	.cf-summary {
		margin: 0.25rem 0 0;
		white-space: pre-wrap;
	}
	.cf-strong {
		color: var(--fd-text-2);
		font-weight: 600;
	}
	.cf-status,
	.cf-verdict {
		border-radius: 4px;
		padding: 0 0.35rem;
		font-size: 0.72rem;
		border: 1px solid var(--fd-border);
	}
	.cf-verdict-compliant {
		color: #10b981;
		border-color: #10b981;
	}
	.cf-verdict-compatible {
		color: #3b82f6;
		border-color: #3b82f6;
	}
	.cf-verdict-not_compliant {
		color: #ef4444;
		border-color: #ef4444;
	}
	.cf-verdict-undetermined {
		color: #f59e0b;
		border-color: #f59e0b;
	}
	.cf-verdict-unqualified {
		color: var(--fd-text-muted);
	}
	.cf-evidence {
		margin-top: 0.35rem;
		padding-top: 0.35rem;
		border-top: 1px dashed var(--fd-border);
		display: grid;
		gap: 0.25rem;
		white-space: pre-wrap;
	}
	.cf-results {
		margin: 0.3rem 0 0;
		padding-left: 1.4rem;
		color: var(--fd-text-2);
		font-size: 0.75rem;
	}
</style>
