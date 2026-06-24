<script lang="ts">
	import { page } from '$app/state';
	import { onMount } from 'svelte';
	import {
		getReport,
		getRequest,
		updateFinding,
		autoFixFinding,
		regenerateReport,
		type FindingItem
	} from '$lib/services/docReviewService';
	import DocStructureView from '$lib/components/home3/doc-structure-view.svelte';
	import EditToolDialog from '$lib/components/home3/edit-tool-dialog.svelte';

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
	// Scrollbar thumb matched to the Document Structure LINES list (--ink-line).
	let scrollThumb = $derived(dark ? '#2A3140' : '#D7CFB8');

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
	let requestId = $state<number | null>(null);
	let skeleton = $state<ReportSkeleton | null>(null);
	let totals = $state({ total: 0, high: 0, medium: 0, low: 0 });
	let activeKey = $state<string | null>(null);

	// DR16: live findings (with ids + review_status) drive the action buttons.
	let findings = $state<FindingItem[]>([]);
	// Per-package and per-reviewer fold state (all collapsed by default).
	let expandedPackages = $state<Record<string, boolean>>({});
	let expandedReviewers = $state<Record<string, boolean>>({});
	function togglePackage(pass: string) {
		expandedPackages[pass] = !expandedPackages[pass];
	}
	function toggleReviewer(key: string) {
		expandedReviewers[key] = !expandedReviewers[key];
	}
	let busyId = $state<number | null>(null);
	let editFindingId = $state<number | null>(null);
	let dirty = $state(false);
	let regenerating = $state(false);
	let toast = $state<{ kind: 'info' | 'warn' | 'error'; text: string } | null>(null);
	let toastTimer: ReturnType<typeof setTimeout> | null = null;

	const passLabels: Record<string, string> = {
		P1: 'Language & Style',
		P2: 'Structure & Organization',
		P3: 'Content Quality',
		P4: 'Consistency',
		P5: 'Technical & Compliance',
		P6: 'Meta & Process'
	};

	function showToast(kind: 'info' | 'warn' | 'error', text: string, ms = 5000) {
		toast = { kind, text };
		if (toastTimer) clearTimeout(toastTimer);
		toastTimer = setTimeout(() => (toast = null), ms);
	}

	type ReviewerGroup = { aspect: string; items: FindingItem[] };
	type PackageGroup = { pass: string; label: string; count: number; reviewers: ReviewerGroup[] };

	// Prettify a reviewer/aspect key for display, e.g. "tone_voice" -> "Tone Voice".
	function reviewerLabel(aspect: string): string {
		return aspect
			.split(/[_\s]+/)
			.filter(Boolean)
			.map((w) => w.charAt(0).toUpperCase() + w.slice(1))
			.join(' ');
	}

	function reviewerKey(pass: string, aspect: string): string {
		return `${pass}::${aspect}`;
	}

	// Visible findings grouped by pass, then by reviewer/aspect (deleted hidden).
	let livePassGroups = $derived.by(() => {
		const order = ['P1', 'P2', 'P3', 'P4', 'P5', 'P6'];
		// pass -> (aspect -> findings), both preserving first-seen order.
		const byPass = new Map<string, Map<string, FindingItem[]>>();
		for (const f of findings) {
			if (f.review_status === 'deleted') continue;
			let byAspect = byPass.get(f.pass);
			if (!byAspect) {
				byAspect = new Map();
				byPass.set(f.pass, byAspect);
			}
			const arr = byAspect.get(f.aspect) ?? [];
			arr.push(f);
			byAspect.set(f.aspect, arr);
		}
		const build = (p: string): PackageGroup | null => {
			const byAspect = byPass.get(p);
			if (!byAspect) return null;
			const reviewers: ReviewerGroup[] = [];
			let count = 0;
			for (const [aspect, items] of byAspect) {
				reviewers.push({ aspect, items });
				count += items.length;
			}
			if (count === 0) return null;
			return { pass: p, label: passLabels[p] ?? p, count, reviewers };
		};
		const out: PackageGroup[] = [];
		for (const p of order) {
			const g = build(p);
			if (g) out.push(g);
		}
		// Append any unexpected passes.
		for (const p of byPass.keys()) {
			if (!order.includes(p)) {
				const g = build(p);
				if (g) out.push(g);
			}
		}
		return out;
	});

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

	function sevColor(sev: string): string {
		if (sev === 'high') return '#dc2626';
		if (sev === 'medium') return '#f59e0b';
		return '#10b981';
	}

	function onFocusFinding(f: FindingItem) {
		activeKey = String(f.id);
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
			requestId = report?.request_id ?? null;
			skeleton = (report?.report_json ?? null) as ReportSkeleton | null;
			const reportFindings = skeleton?.findings ?? [];
			totals = {
				total: report?.total_findings ?? reportFindings.length,
				high: report?.high_count ?? reportFindings.filter((f) => f.severity === 'high').length,
				medium: report?.medium_count ?? reportFindings.filter((f) => f.severity === 'medium').length,
				low: report?.low_count ?? reportFindings.filter((f) => f.severity === 'low').length
			};

			// Load live findings (with ids + review_status) so the action buttons can
			// target specific finding rows.
			findings = [];
			if (requestId != null) {
				try {
					const reqData = await getRequest(requestId);
					findings = reqData.findings ?? [];
				} catch (e) {
					showToast('warn', 'Could not load editable findings: ' + (e instanceof Error ? e.message : String(e)));
				}
			}
		} catch (e) {
			errorMsg = e instanceof Error ? e.message : 'Failed to load report';
		} finally {
			loading = false;
		}
	}

	function setFindingStatus(id: number, status: string) {
		findings = findings.map((f) => (f.id === id ? { ...f, review_status: status } : f));
	}

	async function onAccept(f: FindingItem) {
		busyId = f.id;
		try {
			await updateFinding(f.id, 'accepted');
			setFindingStatus(f.id, 'accepted');
			showToast('info', 'Finding accepted.');
		} catch (e) {
			showToast('error', e instanceof Error ? e.message : 'Accept failed');
		} finally {
			busyId = null;
		}
	}

	async function onDelete(f: FindingItem) {
		busyId = f.id;
		try {
			await updateFinding(f.id, 'deleted');
			setFindingStatus(f.id, 'deleted');
			dirty = true;
			showToast('info', 'Finding deleted from the report.');
		} catch (e) {
			showToast('error', e instanceof Error ? e.message : 'Delete failed');
		} finally {
			busyId = null;
		}
	}

	async function onAutoFix(f: FindingItem) {
		busyId = f.id;
		try {
			const res = await autoFixFinding(f.id);
			if (res.fixable) {
				setFindingStatus(f.id, 'fixed');
				dirty = true;
				const n = res.corrected?.length ?? 0;
				showToast('info', `Auto-fixed ${n} line${n === 1 ? '' : 's'}.`);
			} else {
				showToast('warn', res.message || 'This finding could not be auto-fixed.', 8000);
			}
		} catch (e) {
			showToast('error', e instanceof Error ? e.message : 'Auto-fix failed', 8000);
		} finally {
			busyId = null;
		}
	}

	function onEditSaved(changed: number) {
		const id = editFindingId;
		editFindingId = null;
		if (id != null && changed > 0) {
			setFindingStatus(id, 'fixed');
			dirty = true;
			showToast('info', `Saved ${changed} edited line${changed === 1 ? '' : 's'}.`);
		}
	}

	async function onRegenerate() {
		regenerating = true;
		try {
			await regenerateReport(reportId);
			dirty = false;
			showToast('info', 'Report PDF regenerated.');
			await load();
		} catch (e) {
			showToast('error', e instanceof Error ? e.message : 'Regenerate failed', 8000);
		} finally {
			regenerating = false;
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
	style="--page-bg:{pageBg}; --card-bg:{cardBg}; --border:{borderColor}; --accent:{accent}; --accent-tint:{accentTint}; --text-primary:{textPrimary}; --text-secondary:{textSecondary}; --text-muted:{textMuted}; --scroll-thumb:{scrollThumb};"
>
	<!-- LEFT: report -->
	<section class="left-panel" style="width:{leftPct}%;">
		{#if loading}
			<div class="state">Loading report…</div>
		{:else if errorMsg}
			<div class="state error">{errorMsg}</div>
		{:else if skeleton}
			<div class="title-row">
				<h1 class="report-title">Document Review Report</h1>
				{#if dirty}
					<button class="regen-btn" disabled={regenerating} onclick={onRegenerate}>
						{regenerating ? 'Regenerating…' : 'Regenerate PDF'}
					</button>
				{/if}
			</div>
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

			{#each livePassGroups as group (group.pass)}
				<div class="package">
					<button
						type="button"
						class="package-head"
						aria-expanded={!!expandedPackages[group.pass]}
						onclick={() => togglePackage(group.pass)}
					>
						<span class="chevron" class:open={expandedPackages[group.pass]}>▶</span>
						<span class="package-title">{group.pass} — {group.label}</span>
						<span class="package-count">{group.count}</span>
					</button>

					{#if expandedPackages[group.pass]}
						<div class="package-body">
							{#each group.reviewers as rv (rv.aspect)}
								{@const rkey = reviewerKey(group.pass, rv.aspect)}
								<div class="reviewer">
									<button
										type="button"
										class="reviewer-head"
										aria-expanded={!!expandedReviewers[rkey]}
										onclick={() => toggleReviewer(rkey)}
									>
										<span class="chevron sub" class:open={expandedReviewers[rkey]}>▶</span>
										<span class="reviewer-title">{reviewerLabel(rv.aspect)}</span>
										<span class="reviewer-count">{rv.items.length}</span>
									</button>

									{#if expandedReviewers[rkey]}
										<div class="reviewer-body scroll-list">
											{#each rv.items as f (f.id)}
												<div
													class="finding"
													class:active={activeKey === String(f.id)}
													class:resolved={f.review_status === 'fixed' ||
														f.review_status === 'accepted'}
													style="border-left-color:{sevColor(f.severity)};"
												>
													<button
														type="button"
														class="finding-body"
														onclick={() => onFocusFinding(f)}
														title={f.location ? `Jump to line ${f.location}` : ''}
													>
														<div class="finding-head">
															<strong>{f.title}</strong>
															<span class="sev" style="color:{sevColor(f.severity)};">[{f.severity}]</span>
															<span class="badge">{f.aspect}</span>
															{#if f.review_status === 'fixed'}
																<span class="status-chip fixed">Fixed</span>
															{:else if f.review_status === 'accepted'}
																<span class="status-chip accepted">Accepted</span>
															{/if}
														</div>
														{#if f.description}<p class="finding-desc">{f.description}</p>{/if}
														{#if f.suggestion}<p class="finding-sug"><em>Suggestion:</em> {f.suggestion}</p>{/if}
														{#if f.location}<p class="finding-loc">Location: {f.location}</p>{/if}
													</button>

													<div class="finding-actions">
														<button
															class="act"
															disabled={busyId === f.id}
															onclick={() => onAutoFix(f)}
															title="Fix the offending line(s) automatically with the configured model"
														>
															{busyId === f.id ? '…' : 'LLM Auto Fix'}
														</button>
														<button
															class="act"
															disabled={busyId === f.id}
															onclick={() => (editFindingId = f.id)}
															title="Open the find/replace editor for the offending line(s)"
														>
															Edit Tool
														</button>
														<button
															class="act danger"
															disabled={busyId === f.id}
															onclick={() => onDelete(f)}
															title="Remove this finding from the report"
														>
															Delete
														</button>
														<button
															class="act"
															disabled={busyId === f.id}
															onclick={() => onAccept(f)}
															title="Keep as is — take no action"
														>
															Accept
														</button>
													</div>
												</div>
											{/each}
										</div>
									{/if}
								</div>
							{/each}
						</div>
					{/if}
				</div>
			{/each}

			{#if livePassGroups.length === 0 && findings.length > 0}
				<p class="body-text">All findings have been deleted.</p>
			{/if}
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

{#if toast}
	<div class="toast {toast.kind}">{toast.text}</div>
{/if}

{#if editFindingId != null}
	<EditToolDialog
		findingId={editFindingId}
		suggestion={findings.find((f) => f.id === editFindingId)?.suggestion ?? ''}
		{dark}
		onsaved={onEditSaved}
		oncancel={() => (editFindingId = null)}
	/>
{/if}

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
	.title-row {
		display: flex;
		align-items: flex-start;
		justify-content: space-between;
		gap: 0.75rem;
		border-bottom: 2px solid var(--accent);
		margin-bottom: 0.5rem;
		padding-bottom: 0.5rem;
	}
	.report-title {
		margin: 0;
		font-size: 1.5rem;
	}
	.title-row .report-title {
		border-bottom: none;
		padding-bottom: 0;
	}
	.regen-btn {
		flex: 0 0 auto;
		padding: 0.45rem 0.85rem;
		border: none;
		border-radius: 6px;
		background: var(--accent);
		color: #fff;
		font: inherit;
		font-size: 0.82rem;
		font-weight: 600;
		cursor: pointer;
		white-space: nowrap;
		box-shadow: 0 1px 4px rgba(0, 0, 0, 0.2);
	}
	.regen-btn:disabled {
		opacity: 0.6;
		cursor: not-allowed;
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
	.package {
		margin: 0.6rem 0;
	}
	.package-head {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		width: 100%;
		text-align: left;
		background: var(--card-bg);
		border: 1px solid var(--border);
		border-radius: 8px;
		padding: 0.6rem 0.85rem;
		cursor: pointer;
		color: var(--accent);
		font: inherit;
		font-size: 1.05rem;
		font-weight: 600;
		transition: border-color 0.15s;
	}
	.package-head:hover {
		border-color: var(--accent);
	}
	.chevron {
		flex: 0 0 auto;
		font-size: 0.7rem;
		color: var(--text-secondary);
		transition: transform 0.15s;
	}
	.chevron.open {
		transform: rotate(90deg);
	}
	.package-title {
		flex: 1 1 auto;
		min-width: 0;
	}
	.package-count {
		flex: 0 0 auto;
		font-size: 0.72rem;
		font-weight: 600;
		color: var(--text-secondary);
		background: var(--accent-tint);
		border-radius: 999px;
		padding: 0.1rem 0.55rem;
	}
	.package-body {
		padding: 0.25rem 0 0.25rem 0.75rem;
		margin-top: 0.35rem;
	}
	.reviewer {
		margin: 0.4rem 0;
	}
	.reviewer-head {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		width: 100%;
		text-align: left;
		background: transparent;
		border: 1px solid var(--border);
		border-radius: 7px;
		padding: 0.4rem 0.7rem;
		cursor: pointer;
		color: var(--text-secondary);
		font: inherit;
		font-size: 0.9rem;
		font-weight: 600;
		transition: border-color 0.15s, color 0.15s;
	}
	.reviewer-head:hover {
		border-color: var(--accent);
		color: var(--accent);
	}
	.chevron.sub {
		font-size: 0.62rem;
	}
	.reviewer-title {
		flex: 1 1 auto;
		min-width: 0;
	}
	.reviewer-count {
		flex: 0 0 auto;
		font-size: 0.68rem;
		font-weight: 600;
		color: var(--text-muted);
		background: var(--accent-tint);
		border-radius: 999px;
		padding: 0.08rem 0.5rem;
	}
	.reviewer-body {
		max-height: 45vh;
		overflow-y: auto;
		padding-left: 0.6rem;
		padding-right: 0.4rem;
		border-left: 2px solid var(--border);
		margin: 0.3rem 0 0.3rem 0.45rem;
	}
	/* Scrollbar matched to the Document Structure LINES list. */
	.scroll-list {
		scrollbar-width: thin;
		scrollbar-color: var(--scroll-thumb) transparent;
	}
	.scroll-list::-webkit-scrollbar {
		width: 6px;
	}
	.scroll-list::-webkit-scrollbar-thumb {
		background: var(--scroll-thumb);
		border-radius: 999px;
	}
	.scroll-list::-webkit-scrollbar-track {
		background: transparent;
	}
	.finding {
		background: var(--card-bg);
		border: 1px solid var(--border);
		border-left: 4px solid var(--border);
		border-radius: 8px;
		margin: 0.6rem 0;
		transition: border-color 0.15s, box-shadow 0.15s;
	}
	.reviewer-body .finding:first-child {
		margin-top: 0;
	}
	.finding:hover {
		border-color: var(--accent);
	}
	.finding.active {
		box-shadow: 0 0 0 2px var(--accent);
	}
	.finding.resolved {
		opacity: 0.7;
	}
	.finding-body {
		display: block;
		width: 100%;
		text-align: left;
		background: transparent;
		border: none;
		padding: 0.85rem 1rem 0.5rem;
		cursor: pointer;
		color: inherit;
		font: inherit;
	}
	.finding-actions {
		display: flex;
		flex-wrap: wrap;
		gap: 0.4rem;
		padding: 0 1rem 0.75rem;
	}
	.act {
		padding: 0.32rem 0.7rem;
		border: 1px solid var(--border);
		border-radius: 6px;
		background: var(--card-bg);
		color: var(--text-primary);
		font: inherit;
		font-size: 0.76rem;
		cursor: pointer;
		transition: background 0.15s, border-color 0.15s, opacity 0.15s;
	}
	.act:hover:not(:disabled) {
		border-color: var(--accent);
		color: var(--accent);
	}
	.act.danger:hover:not(:disabled) {
		border-color: #dc2626;
		color: #dc2626;
	}
	.act:disabled {
		opacity: 0.5;
		cursor: not-allowed;
	}
	.status-chip {
		display: inline-block;
		padding: 0.1rem 0.5rem;
		border-radius: 999px;
		font-size: 0.68rem;
		font-weight: 600;
	}
	.status-chip.fixed {
		background: rgba(16, 185, 129, 0.18);
		color: #10b981;
	}
	.status-chip.accepted {
		background: var(--accent-tint);
		color: var(--accent);
	}
	.toast {
		position: fixed;
		bottom: 1.25rem;
		left: 50%;
		transform: translateX(-50%);
		z-index: 1100;
		padding: 0.6rem 1rem;
		border-radius: 8px;
		font-size: 0.85rem;
		color: #fff;
		box-shadow: 0 8px 28px rgba(0, 0, 0, 0.35);
		max-width: min(640px, 92vw);
	}
	.toast.info {
		background: #4f46e5;
	}
	.toast.warn {
		background: #b45309;
	}
	.toast.error {
		background: #b91c1c;
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
