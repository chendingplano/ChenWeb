<script lang="ts">
	import { page } from '$app/state';
	import { onMount } from 'svelte';
	import {
		getReport,
		getRequest,
		listLanguages,
		updateFinding,
		regenerateReport,
		generateCorrectionReport,
		type FindingItem,
		type ReviewPackageInfo
	} from '$lib/services/docReviewService';
	import DocStructureView from '$lib/components/home3/doc-structure-view.svelte';
	import EditToolDialog from '$lib/components/home3/edit-tool-dialog.svelte';
	import LlmAutoFixDialog from '$lib/components/home3/llm-auto-fix-dialog.svelte';

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
	let baseFindings = $state<FindingItem[]>([]);
	let findings = $state<FindingItem[]>([]);
	let packages = $state<ReviewPackageInfo[]>([]);
	let languages = $state<string[]>(['en']);
	let selectedLanguage = $state('en');
	let languageLoading = $state(false);
	let localizedReviewerCache = $state<Record<string, FindingItem[]>>({});
	// Per-package and per-reviewer fold state (all collapsed by default).
	let expandedPackages = $state<Record<string, boolean>>({});
	let expandedReviewers = $state<Record<string, boolean>>({});
	function togglePackage(pass: string) {
		expandedPackages[pass] = !expandedPackages[pass];
	}
	function cloneFindings(items: FindingItem[]): FindingItem[] {
		return items.map((item) => ({ ...item }));
	}
	function mergeLocalizedFindings(localized: FindingItem[]) {
		if (localized.length === 0) return;
		const byId = new Map(localized.map((item) => [item.id, item]));
		findings = findings.map((item) => {
			const translated = byId.get(item.id);
			return translated ? { ...item, ...translated } : item;
		});
	}
	async function ensureReviewerLocalized(pass: string, aspect: string) {
		if (requestId == null || selectedLanguage === 'en') return;
		const cacheKey = `${selectedLanguage}::${pass}::${aspect}`;
		const cached = localizedReviewerCache[cacheKey];
		if (cached) {
			mergeLocalizedFindings(cached);
			return;
		}
		console.info('[doc-review-report] lazily translating reviewer findings', {
			reportId,
			requestId,
			selectedLanguage,
			pass,
			aspect
		});
		const reqData = await getRequest(requestId, { language: selectedLanguage, pass, aspect });
		const localized = reqData.findings ?? [];
		localizedReviewerCache[cacheKey] = localized;
		mergeLocalizedFindings(localized);
	}
	async function toggleReviewer(pass: string, aspect: string) {
		const key = reviewerKey(pass, aspect);
		const next = !expandedReviewers[key];
		expandedReviewers[key] = next;
		if (!next) return;
		try {
			await ensureReviewerLocalized(pass, aspect);
		} catch (e) {
			console.error('[doc-review-report] failed to lazily translate reviewer findings', {
				reportId,
				requestId,
				selectedLanguage,
				pass,
				aspect,
				error: e
			});
			showToast('error', e instanceof Error ? e.message : 'Lazy translation failed', 8000);
		}
	}
	let busyId = $state<number | null>(null);
	let editFindingId = $state<number | null>(null);
	let autoFixFinding = $state<FindingItem | null>(null);
	let dirty = $state(false);
	let showMode = $state<'active' | 'all'>('active');
	let regenerating = $state(false);
	let correcting = $state(false);
	let toast = $state<{ kind: 'info' | 'warn' | 'error'; text: string } | null>(null);
	let toastTimer: ReturnType<typeof setTimeout> | null = null;

	const fallbackPackages: ReviewPackageInfo[] = [
		{ key: 'P1', label: 'Language & Style' },
		{ key: 'P2', label: 'Structure & Organization' },
		{ key: 'P3', label: 'Content Quality' },
		{ key: 'P4', label: 'Consistency' },
		{ key: 'P5', label: 'Technical & Compliance' },
		{ key: 'P6', label: 'Meta & Process' }
	];

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
		const displayPackages = packages.length > 0 ? packages : fallbackPackages;
		const order = displayPackages.map((pkg) => pkg.key);
		const passLabels = new Map(displayPackages.map((pkg) => [pkg.key, pkg.label]));
		// pass -> (aspect -> findings), both preserving first-seen order.
		const byPass = new Map<string, Map<string, FindingItem[]>>();
		for (const f of findings) {
			if (f.review_status === 'deleted') continue;
			if (showMode === 'active' && f.review_status === 'corrected') continue;
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
			return { pass: p, label: passLabels.get(p) ?? p, count, reviewers };
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
			try {
				const configuredLanguages = await listLanguages();
				languages = configuredLanguages.length > 0 ? configuredLanguages : ['en'];
				if (!languages.includes(selectedLanguage)) {
					selectedLanguage = languages[0] ?? 'en';
				}
			} catch {
				languages = ['en'];
				selectedLanguage = 'en';
			}
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
					baseFindings = reqData.findings ?? [];
					findings = cloneFindings(baseFindings);
					packages = reqData.packages ?? [];
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

	async function reloadFindingsForLanguage() {
		languageLoading = true;
		console.info('[doc-review-report] reloading findings for language', {
			reportId,
			requestId,
			selectedLanguage
		});
		try {
			findings = cloneFindings(baseFindings);
			if (selectedLanguage !== 'en') {
				for (const key of Object.keys(expandedReviewers)) {
					if (!expandedReviewers[key]) continue;
					const [pass, aspect] = key.split('::');
					if (pass && aspect) {
						await ensureReviewerLocalized(pass, aspect);
					}
				}
			}
			console.info('[doc-review-report] language state refreshed', {
				reportId,
				requestId,
				selectedLanguage,
				findingCount: findings.length
			});
		} catch (e) {
			console.error('[doc-review-report] failed to reload findings for language', {
				reportId,
				requestId,
				selectedLanguage,
				error: e
			});
			showToast('error', e instanceof Error ? e.message : 'Language change failed', 8000);
		} finally {
			languageLoading = false;
		}
	}

	function setFindingStatus(id: number, status: string) {
		findings = findings.map((f) => (f.id === id ? { ...f, review_status: status } : f));
		baseFindings = baseFindings.map((f) => (f.id === id ? { ...f, review_status: status } : f));
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

	function onAutoFix(f: FindingItem) {
		autoFixFinding = f;
	}

	async function onAutoFixSaved() {
		const f = autoFixFinding;
		autoFixFinding = null;
		if (!f) return;
		try {
			await updateFinding(f.id, 'corrected');
			setFindingStatus(f.id, 'corrected');
			dirty = true;
			showToast('info', 'Auto-fix applied and hidden.');
		} catch (e) {
			showToast('error', e instanceof Error ? e.message : 'Failed to mark as corrected');
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

	async function onEditSavedAndHide(changed: number) {
		const id = editFindingId;
		editFindingId = null;
		if (id == null) return;
		try {
			await updateFinding(id, 'corrected');
			setFindingStatus(id, 'corrected');
			if (changed > 0) dirty = true;
			showToast('info', `Saved and hidden${changed > 0 ? ` (${changed} line${changed === 1 ? '' : 's'} changed)` : ''}.`);
		} catch (e) {
			showToast('error', e instanceof Error ? e.message : 'Failed to mark as corrected');
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

	async function onCorrectionReport() {
		correcting = true;
		try {
			const file = await generateCorrectionReport(reportId);
			showToast('info', file ? `Correction report generated: ${file}` : 'Correction report generated.');
		} catch (e) {
			showToast('error', e instanceof Error ? e.message : 'Correction report failed', 8000);
		} finally {
			correcting = false;
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
				<label class="language-picker">
					<span>Language</span>
					<select bind:value={selectedLanguage} onchange={reloadFindingsForLanguage} disabled={languageLoading}>
						{#each languages as lang (lang)}
							<option value={lang}>{lang}</option>
						{/each}
					</select>
				</label>
			</div>
			<p class="meta">
				Document: {skeleton.meta?.document_title || '—'} (ID: {inputRecordId ?? '—'})<br />
				Generated: {skeleton.meta?.generated_at || '—'}<br />
				Review Request ID: {requestId ?? '—'}
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

			<div class="show-mode-bar">
				<button
					type="button"
					class="show-mode-btn"
					class:active={showMode === 'active'}
					onclick={() => (showMode = 'active')}
				>Show Active</button>
				<button
					type="button"
					class="show-mode-btn"
					class:active={showMode === 'all'}
					onclick={() => (showMode = 'all')}
				>Show All</button>
				<div class="show-mode-sep"></div>
				<button
					type="button"
					class="show-mode-btn action-btn"
					disabled={correcting}
					onclick={onCorrectionReport}
				>{correcting ? 'Generating…' : 'Generate Change Report'}</button>
				<button
					type="button"
					class="show-mode-btn action-btn"
					disabled={regenerating}
					onclick={onRegenerate}
				>{regenerating ? 'Regenerating…' : 'Re-Generate Review Report'}</button>
			</div>

			{#each livePassGroups as group (group.pass)}
				<div class="package">
					<button
						type="button"
						class="package-head"
						aria-expanded={!!expandedPackages[group.pass]}
						onclick={() => togglePackage(group.pass)}
					>
						<span class="chevron" class:open={expandedPackages[group.pass]}>▶</span>
						<span class="package-title">{group.label}</span>
						<span class="package-count">{group.count}</span>
					</button>

					{#if expandedPackages[group.pass]}
						<div class="package-body scroll-list">
							{#each group.reviewers as rv (rv.aspect)}
								{@const rkey = reviewerKey(group.pass, rv.aspect)}
								<div class="reviewer">
									<button
										type="button"
										class="reviewer-head"
										aria-expanded={!!expandedReviewers[rkey]}
										onclick={() => toggleReviewer(group.pass, rv.aspect)}
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
														f.review_status === 'accepted' ||
														f.review_status === 'corrected'}
													style="border-left-color:{sevColor(f.severity)};"
												>
													<button
														type="button"
														class="finding-body"
														onclick={() => onFocusFinding(f)}
														title={f.location ? `Jump to line ${f.location}` : ''}
													>
														<div class="finding-head">
															<span class="finding-id">#{f.id}</span>
															<strong>{f.title}</strong>
															<span class="sev" style="color:{sevColor(f.severity)};">[{f.severity}]</span>
															<span class="badge">{f.aspect}</span>
															{#if f.review_status === 'fixed'}
																<span class="status-chip chip-fixed">Fixed</span>
															{:else if f.review_status === 'accepted'}
																<span class="status-chip accepted">Accepted</span>
															{:else if f.review_status === 'corrected'}
																<span class="status-chip corrected">Corrected</span>
															{/if}
														</div>
														{#if f.description}<p class="finding-desc">{f.description}</p>{/if}
														{#if f.suggestion}<p class="finding-sug"><em>Suggestion:</em> {f.suggestion}</p>{/if}
														<p class="finding-loc">Confidence: {Math.round(f.confidence * 100)}%</p>
														{#if f.location}<p class="finding-loc">Location: {f.location}</p>{/if}
													</button>

													<div class="finding-actions">
														<button
															class="act"
															disabled={busyId === f.id}
															onclick={() => onAutoFix(f)}
															title="Fix the offending line(s) automatically with the configured model"
														>
															LLM Auto Fix
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
		reason={findings.find((f) => f.id === editFindingId)?.description ?? ''}
		suggestion={findings.find((f) => f.id === editFindingId)?.suggestion ?? ''}
		{dark}
		onsaved={onEditSaved}
		onsavedandhide={onEditSavedAndHide}
		oncancel={() => (editFindingId = null)}
	/>
{/if}

{#if autoFixFinding != null}
	<LlmAutoFixDialog
		finding={autoFixFinding}
		{dark}
		onsaved={onAutoFixSaved}
		oncancel={() => (autoFixFinding = null)}
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
		overflow: hidden;
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
	.language-picker {
		display: inline-flex;
		align-items: center;
		gap: 0.45rem;
		color: var(--text-secondary);
		font-size: 0.82rem;
		white-space: nowrap;
	}
	.language-picker select {
		min-width: 4.5rem;
		border: 1px solid var(--border);
		border-radius: 6px;
		background: var(--card-bg);
		color: var(--text-primary);
		padding: 0.28rem 0.5rem;
		font: inherit;
	}
	.language-picker select:disabled {
		opacity: 0.6;
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
		max-height: min(52vh, calc(100vh - 28rem));
		overflow-y: auto;
		padding: 0.25rem 0 0.25rem 0.75rem;
		padding-right: 0.45rem;
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
	.status-chip.chip-fixed {
		background: rgba(16, 185, 129, 0.18);
		color: #10b981;
	}
	.status-chip.accepted {
		background: var(--accent-tint);
		color: var(--accent);
	}
	.status-chip.corrected {
		background: rgba(15, 118, 110, 0.18);
		color: #0f766e;
	}
	.show-mode-bar {
		display: flex;
		align-items: center;
		flex-wrap: wrap;
		gap: 0.4rem;
		margin: 0.75rem 0 0.5rem;
	}
	.show-mode-sep {
		width: 1px;
		height: 1.25rem;
		background: var(--border);
		margin: 0 0.15rem;
		flex: 0 0 auto;
	}
	.show-mode-btn {
		padding: 0.3rem 0.75rem;
		border: 1px solid var(--border);
		border-radius: 6px;
		background: transparent;
		color: var(--text-secondary);
		font: inherit;
		font-size: 0.78rem;
		cursor: pointer;
		transition: background 0.15s, border-color 0.15s, color 0.15s;
		white-space: nowrap;
	}
	.show-mode-btn:hover:not(:disabled) {
		border-color: var(--accent);
		color: var(--accent);
	}
	.show-mode-btn.active {
		background: var(--accent-tint);
		border-color: var(--accent);
		color: var(--accent);
		font-weight: 600;
	}
	.show-mode-btn.action-btn {
		background: var(--accent);
		border-color: var(--accent);
		color: #fff;
		font-weight: 600;
	}
	.show-mode-btn.action-btn:hover:not(:disabled) {
		opacity: 0.88;
		color: #fff;
	}
	.show-mode-btn:disabled {
		opacity: 0.55;
		cursor: not-allowed;
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
	.finding-id {
		flex: 0 0 auto;
		font-size: 0.68rem;
		font-family: monospace;
		color: var(--text-muted);
		background: var(--accent-tint);
		border-radius: 4px;
		padding: 0.1rem 0.4rem;
		letter-spacing: 0.02em;
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
