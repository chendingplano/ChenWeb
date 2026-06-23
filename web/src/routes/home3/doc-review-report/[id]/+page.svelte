<script lang="ts">
	import { page } from '$app/state';
	import { onMount } from 'svelte';
	import { getReport } from '$lib/services/docReviewService';
	import DocStructureView from '$lib/components/home3/doc-structure-view.svelte';

	let dark = $derived(page.url.searchParams.get('dark') !== '0');
	let reportId = $derived(Number(page.params.id));

	// Design tokens (mirror doc-review-results-view.svelte for visual parity).
	let pageBg = $derived(dark ? '#15181F' : '#F5F1E8');
	let cardBg = $derived(dark ? '#1F2333' : '#FFFFFF');
	let borderColor = $derived(dark ? '#2D3348' : '#E4E6EB');
	let accent = $derived(dark ? '#818CF8' : '#6366F1');
	let accentTint = $derived(dark ? 'rgba(129,140,248,0.15)' : 'rgba(99,102,241,0.10)');
	let textPrimary = $derived(dark ? '#E2E8F0' : '#111827');
	let textSecondary = $derived(dark ? '#94A3B8' : '#6B7280');
	let textMuted = $derived(dark ? '#64748B' : '#9CA3AF');

	type ReportFinding = {
		pass: string;
		aspect: string;
		severity: string;
		finding_type?: string;
		title: string;
		description?: string;
		evidence?: string;
		location?: string;
		suggestion?: string;
	};
	type PassGroup = { label: string; findings: ReportFinding[] };
	type ReportSkeleton = {
		meta?: Record<string, any>;
		executive_summary?: { text?: string; top_findings?: string[]; overall_assessment?: string };
		findings_by_pass?: Record<string, PassGroup>;
		pass_order?: string[];
		findings?: ReportFinding[];
	};

	let loading = $state(true);
	let errorMsg = $state('');
	let inputRecordId = $state<number | null>(null);
	let skeleton = $state<ReportSkeleton | null>(null);
	let totals = $state({ total: 0, high: 0, medium: 0, low: 0 });
	let activeKey = $state<string | null>(null);

	// Reference to the embedded Document Structure panel for programmatic focus.
	let structureView = $state<DocStructureView | null>(null);

	/**
	 * Mirror of server-side parseLocationRange (report.go:382).
	 *   "162"    -> [162]
	 *   "53-56"  -> [53, 54, 55, 56]
	 *   "53, 87" -> [53, 87]
	 */
	function parseLocationRange(location: string): number[] {
		const loc = (location || '').trim();
		if (!loc) return [];
		const firstInt = (s: string): number => {
			const m = s.match(/\d+/);
			return m ? parseInt(m[0], 10) : 0;
		};
		const dash = loc.indexOf('-');
		if (dash > 0) {
			const after = loc.slice(dash + 1).trim();
			if (after && after[0] >= '0' && after[0] <= '9') {
				const start = firstInt(loc.slice(0, dash));
				const end = firstInt(after);
				if (start > 0 && end >= start && end - start < 200) {
					return Array.from({ length: end - start + 1 }, (_, i) => start + i);
				}
			}
		}
		const out: number[] = [];
		const seen = new Set<number>();
		for (const part of loc.split(/[,;]/)) {
			const n = firstInt(part);
			if (n > 0 && !seen.has(n)) {
				seen.add(n);
				out.push(n);
			}
		}
		return out;
	}

	let passOrder = $derived.by(() => {
		if (!skeleton) return [];
		const fbp = skeleton.findings_by_pass ?? {};
		const ordered = (skeleton.pass_order ?? []).filter((p) => fbp[p]);
		// Append any passes present in the map but missing from pass_order.
		for (const p of Object.keys(fbp)) if (!ordered.includes(p)) ordered.push(p);
		return ordered;
	});

	function sevColor(sev: string): string {
		if (sev === 'high') return '#dc2626';
		if (sev === 'medium') return '#f59e0b';
		return '#10b981';
	}

	function findingKey(pass: string, idx: number): string {
		return `${pass}:${idx}`;
	}

	function onFindingClick(pass: string, idx: number, f: ReportFinding) {
		activeKey = findingKey(pass, idx);
		const lineNumbers = parseLocationRange(f.location ?? '');
		if (lineNumbers.length > 0) {
			void structureView?.focusSourceLines(lineNumbers);
		}
	}

	async function load() {
		loading = true;
		errorMsg = '';
		try {
			const report = await getReport(reportId);
			inputRecordId = report?.input_record_id ?? report?.report_json?.meta?.document_record_id ?? null;
			skeleton = (report?.report_json ?? null) as ReportSkeleton | null;
			const findings = skeleton?.findings ?? [];
			totals = {
				total: report?.total_findings ?? findings.length,
				high: report?.high_count ?? findings.filter((f) => f.severity === 'high').length,
				medium: report?.medium_count ?? findings.filter((f) => f.severity === 'medium').length,
				low: report?.low_count ?? findings.filter((f) => f.severity === 'low').length
			};
		} catch (e) {
			errorMsg = e instanceof Error ? e.message : 'Failed to load report';
		} finally {
			loading = false;
		}
	}

	onMount(() => {
		void load();
		// Initial left panel: 45% minus 200px so the PDF gets more room by default.
		if (containerEl) {
			const w = containerEl.getBoundingClientRect().width;
			const px = w * 0.45 - 200;
			leftPct = Math.max(10, Math.min(70, (px / w) * 100));
		}
	});

	// --- Resizable splitter -------------------------------------------------
	let leftPct = $state(45);
	let dragging = $state(false);
	let containerEl = $state<HTMLDivElement | null>(null);

	function startDrag(e: PointerEvent) {
		dragging = true;
		(e.target as HTMLElement).setPointerCapture?.(e.pointerId);
		document.body.style.cursor = 'col-resize';
		document.body.style.userSelect = 'none';
	}
	function onDrag(e: PointerEvent) {
		if (!dragging || !containerEl) return;
		const rect = containerEl.getBoundingClientRect();
		const pct = ((e.clientX - rect.left) / rect.width) * 100;
		// Lower bound is the old 25% floor reduced by 200px, so the left panel can
		// be dragged 200px narrower (giving the PDF panel more room).
		const minPct = Math.max(0, ((0.25 * rect.width - 200) / rect.width) * 100);
		leftPct = Math.min(70, Math.max(minPct, pct));
	}
	function endDrag(e: PointerEvent) {
		if (!dragging) return;
		dragging = false;
		(e.target as HTMLElement).releasePointerCapture?.(e.pointerId);
		document.body.style.cursor = '';
		document.body.style.userSelect = '';
	}
</script>

<svelte:head>
	<title>{skeleton?.meta?.document_title || 'Document Review'} — Report</title>
</svelte:head>

<div
	class="report-page"
	bind:this={containerEl}
	style="--page-bg:{pageBg}; --card-bg:{cardBg}; --border:{borderColor}; --accent:{accent}; --accent-tint:{accentTint}; --text-primary:{textPrimary}; --text-secondary:{textSecondary}; --text-muted:{textMuted};"
>
	<!-- LEFT: report -->
	<section class="left-panel" style="width:{leftPct}%;">
		{#if loading}
			<div class="state">Loading report…</div>
		{:else if errorMsg}
			<div class="state error">{errorMsg}</div>
		{:else if skeleton}
			<h1 class="report-title">Document Review Report</h1>
			<p class="meta">
				Document: {skeleton.meta?.document_title || '—'} (ID: {inputRecordId ?? '—'})<br />
				Generated: {skeleton.meta?.generated_at || '—'}<br />
				Review Run: {skeleton.meta?.review_run_id || '—'}
			</p>

			<div class="summary-cards">
				<div class="summary-card"><div class="count">{totals.total}</div><div class="label">Total</div></div>
				<div class="summary-card"><div class="count" style="color:#dc2626;">{totals.high}</div><div class="label">High</div></div>
				<div class="summary-card"><div class="count" style="color:#f59e0b;">{totals.medium}</div><div class="label">Medium</div></div>
				<div class="summary-card"><div class="count" style="color:#10b981;">{totals.low}</div><div class="label">Low</div></div>
			</div>

			{#if skeleton.executive_summary}
				<h2>Executive Summary</h2>
				<p class="body-text">{skeleton.executive_summary.text}</p>
				{#if skeleton.executive_summary.overall_assessment}
					<p class="body-text">
						Assessment:
						<span class="assessment">{skeleton.executive_summary.overall_assessment}</span>
					</p>
				{/if}
				{#if skeleton.executive_summary.top_findings?.length}
					<h3>Top Findings</h3>
					<ul class="top-findings">
						{#each skeleton.executive_summary.top_findings as t}<li>{t}</li>{/each}
					</ul>
				{/if}
			{/if}

			{#each passOrder as pass}
				{@const group = skeleton.findings_by_pass?.[pass]}
				{#if group}
					<h2>{pass} — {group.label}</h2>
					{#each group.findings as f, idx}
						<button
							type="button"
							class="finding"
							class:active={activeKey === findingKey(pass, idx)}
							style="border-left-color:{sevColor(f.severity)};"
							onclick={() => onFindingClick(pass, idx, f)}
							title={f.location ? `Jump to line ${f.location}` : ''}
						>
							<div class="finding-head">
								<strong>{f.title}</strong>
								<span class="sev" style="color:{sevColor(f.severity)};">[{f.severity}]</span>
								<span class="badge">{f.aspect}</span>
							</div>
							{#if f.description}<p class="finding-desc">{f.description}</p>{/if}
							{#if f.suggestion}<p class="finding-sug"><em>Suggestion:</em> {f.suggestion}</p>{/if}
							{#if f.location}<p class="finding-loc">Location: {f.location}</p>{/if}
						</button>
					{/each}
				{/if}
			{/each}
		{/if}
	</section>

	<!-- Splitter -->
	<div
		class="splitter"
		role="separator"
		aria-orientation="vertical"
		tabindex="-1"
		onpointerdown={startDrag}
		onpointermove={onDrag}
		onpointerup={endDrag}
		onpointercancel={endDrag}
	></div>

	<!-- RIGHT: Document Structure (line list + PDF) -->
	<section class="right-panel">
		{#if inputRecordId != null}
			<DocStructureView bind:this={structureView} darkMode={dark} lockedRecordId={inputRecordId} />
		{:else if !loading}
			<div class="state">No source document linked to this report.</div>
		{/if}
	</section>
</div>

<style>
	.report-page {
		display: flex;
		height: 100vh;
		width: 100%;
		overflow: hidden;
		background: var(--page-bg);
		color: var(--text-primary);
		font-family: system-ui, -apple-system, 'Segoe UI', Roboto, sans-serif;
	}
	.left-panel {
		height: 100%;
		overflow-y: auto;
		padding: 1.5rem 1.75rem;
		box-sizing: border-box;
		flex: 0 0 auto;
		min-width: 0;
	}
	.right-panel {
		flex: 1 1 0;
		height: 100%;
		min-width: 0;
		overflow: hidden;
	}
	.splitter {
		flex: 0 0 6px;
		cursor: col-resize;
		background: var(--border);
		transition: background 0.15s;
	}
	.splitter:hover {
		background: var(--accent);
	}
	.state {
		padding: 2rem;
		color: var(--text-secondary);
	}
	.state.error {
		color: #dc2626;
	}
	.report-title {
		margin: 0 0 0.5rem;
		font-size: 1.5rem;
		border-bottom: 2px solid var(--accent);
		padding-bottom: 0.5rem;
	}
	.meta {
		color: var(--text-muted);
		font-size: 0.85rem;
		line-height: 1.5;
	}
	h2 {
		margin-top: 1.75rem;
		font-size: 1.1rem;
		color: var(--accent);
	}
	h3 {
		font-size: 0.95rem;
		color: var(--text-secondary);
	}
	.body-text {
		font-size: 0.9rem;
		line-height: 1.55;
		color: var(--text-primary);
	}
	.assessment {
		display: inline-block;
		padding: 0.15rem 0.6rem;
		border-radius: 4px;
		font-weight: 600;
		font-size: 0.8rem;
		background: var(--accent-tint);
		color: var(--accent);
	}
	.top-findings {
		font-size: 0.9rem;
		color: var(--text-primary);
	}
	.summary-cards {
		display: flex;
		gap: 0.75rem;
		margin: 1rem 0;
	}
	.summary-card {
		flex: 1;
		text-align: center;
		background: var(--card-bg);
		border: 1px solid var(--border);
		border-radius: 8px;
		padding: 0.75rem 0.5rem;
	}
	.summary-card .count {
		font-size: 1.5rem;
		font-weight: 700;
	}
	.summary-card .label {
		font-size: 0.7rem;
		color: var(--text-muted);
		text-transform: uppercase;
		letter-spacing: 0.04em;
	}
	.finding {
		display: block;
		width: 100%;
		text-align: left;
		background: var(--card-bg);
		border: 1px solid var(--border);
		border-left: 4px solid var(--border);
		border-radius: 8px;
		padding: 0.85rem 1rem;
		margin: 0.6rem 0;
		cursor: pointer;
		color: inherit;
		font: inherit;
		transition: border-color 0.15s, box-shadow 0.15s, transform 0.05s;
	}
	.finding:hover {
		border-color: var(--accent);
	}
	.finding.active {
		box-shadow: 0 0 0 2px var(--accent);
	}
	.finding:active {
		transform: translateY(1px);
	}
	.finding-head {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		flex-wrap: wrap;
	}
	.finding-head strong {
		font-size: 0.92rem;
	}
	.sev {
		font-weight: 600;
		font-size: 0.8rem;
	}
	.badge {
		display: inline-block;
		padding: 0.1rem 0.5rem;
		border-radius: 999px;
		font-size: 0.7rem;
		background: var(--accent-tint);
		color: var(--text-secondary);
	}
	.finding-desc {
		margin: 0.5rem 0 0;
		font-size: 0.86rem;
		line-height: 1.5;
		color: var(--text-secondary);
	}
	.finding-sug {
		margin: 0.4rem 0 0;
		font-size: 0.84rem;
		color: var(--text-primary);
	}
	.finding-loc {
		margin: 0.4rem 0 0;
		font-size: 0.78rem;
		color: var(--text-muted);
	}
</style>
