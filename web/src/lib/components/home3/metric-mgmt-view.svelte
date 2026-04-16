<script lang="ts">
	import { onMount, tick } from 'svelte';
	import {
		listKbInputs,
		listKbMetrics,
		getKbInput,
		getRawLines,
		type KbInputRecord,
		type KbMetricRecord,
		type RawLine
	} from '$lib/services/kbService';

	let { darkMode = true }: { darkMode: boolean } = $props();

	// ---------- Aesthetic tokens: "archival reading room" ----------
	let pageBg        = $derived(darkMode ? '#0E1116' : '#F5F1E8');
	let panelBg       = $derived(darkMode ? '#161A22' : '#FBF8F0');
	let panelBgAlt    = $derived(darkMode ? '#1C212C' : '#F0EADB');
	let inkLine       = $derived(darkMode ? '#2A3140' : '#D7CFB8');
	let inkLineSoft   = $derived(darkMode ? '#1F2530' : '#E5DEC8');
	let textPrimary   = $derived(darkMode ? '#EDE7D3' : '#1A1410');
	let textSecondary = $derived(darkMode ? '#B5AE94' : '#5C5345');
	let textMuted     = $derived(darkMode ? '#7C7560' : '#8F8472');
	let brass         = $derived(darkMode ? '#D4A24C' : '#B8801E');
	let brassFaint    = $derived(darkMode ? 'rgba(212,162,76,0.12)' : 'rgba(184,128,30,0.10)');
	let crimson       = $derived(darkMode ? '#C8553D' : '#A23E26');
	let crimsonFaint  = $derived(darkMode ? 'rgba(200,85,61,0.18)' : 'rgba(162,62,38,0.12)');
	let teal          = $derived(darkMode ? '#5DAFA8' : '#2D7B73');

	const fontSerif = "'Cormorant Garamond', 'Playfair Display', Georgia, serif";
	const fontMono  = "'JetBrains Mono', 'IBM Plex Mono', monospace";
	const fontSans  = "'Inter Tight', system-ui, sans-serif";

	// ---------- State ----------
	let recordIdInput = $state('');
	let currentInput = $state<KbInputRecord | null>(null);
	let metrics = $state<KbMetricRecord[]>([]);
	let selectedMetricId = $state<number | null>(null);
	let loading = $state(false);
	let errorMsg = $state('');

	let rawLines = $state<RawLine[]>([]);
	let rawLoading = $state(false);
	let rawError = $state('');

	// Document viewer state
	let viewMode = $state<'document' | 'source'>('document');
	let docPage = $state<number>(1);

	let fileUrl = $derived.by(() => {
		if (!currentInput) return '';
		return `/api/v1/kb/inputs/${currentInput.id}/file#page=${docPage}&zoom=page-width`;
	});

	let isPdf = $derived((currentInput?.type ?? '').toLowerCase() === 'pdf');
	let isText = $derived(['text', 'markdown', 'json', 'xml', 'typst', 'html'].includes((currentInput?.type ?? '').toLowerCase()));

	// PDF.js rendering state
	type PdfJsLib = {
		getDocument: (src: string | { url: string; withCredentials?: boolean }) => { promise: Promise<unknown> };
		GlobalWorkerOptions?: { workerSrc: string };
	};
	type PdfPageViewport = {
		width: number;
		height: number;
		convertToViewportRectangle: (rect: number[]) => number[];
	};
	type PdfPageProxy = {
		rotate?: number;
		getViewport: (params: { scale: number; rotation?: number }) => PdfPageViewport;
		render: (params: { canvasContext: CanvasRenderingContext2D; viewport: PdfPageViewport }) => { promise: Promise<void> };
	};
	type PdfDocumentProxy = {
		numPages: number;
		getPage: (n: number) => Promise<PdfPageProxy>;
		destroy?: () => void | Promise<void>;
	};

	let pdfLib: PdfJsLib | null = null;
	let pdfDoc: PdfDocumentProxy | null = null;
	let pdfLoadedInputId = 0;
	let pdfRenderSeq = 0;
	let pdfLoading = $state(false);
	let pdfError = $state('');
	let pdfNumPages = $state(0);
	let pdfZoom = $state(1);
	let pdfRenderedPages = $state<number[]>([]);
	let pdfLastRenderWidth = 0;
	let pdfResizeRaf = 0;

	let pdfStageEl = $state<HTMLDivElement | null>(null);
	let pdfViewportByPage = new Map<number, PdfPageViewport>();

	// Search dialog
	let searchOpen = $state(false);
	let searchFileName = $state('');
	let searchDocType = $state('all');
	let searchResults = $state<KbInputRecord[]>([]);
	let searchLoading = $state(false);
	let searchError = $state('');
	let searchSelected = $state<number | null>(null);

	const docTypeOptions = ['all', 'pdf', 'doc', 'excel', 'ppt', 'text', 'json', 'xml', 'markdown', 'typst'];

	type NormalizedSpan = { page_number: number; line_number: number };

	function toPositiveInt(v: unknown): number | null {
		const n = typeof v === 'string' ? Number(v.trim()) : Number(v);
		if (!Number.isFinite(n)) return null;
		const i = Math.trunc(n);
		return i > 0 ? i : null;
	}

	function normalizeMetricSpans(m: KbMetricRecord | undefined): NormalizedSpan[] {
		const raw = (m as { source_line_spans?: unknown })?.source_line_spans;
		if (!Array.isArray(raw)) return [];
		const out: NormalizedSpan[] = [];
		for (const item of raw) {
			if (typeof item === 'string') {
				const mm = item.trim().match(/^(\d+)\s*[:,-]\s*(\d+)$/);
				if (!mm) continue;
				const p = toPositiveInt(mm[1]);
				const l = toPositiveInt(mm[2]);
				if (p && l) out.push({ page_number: p, line_number: l });
				continue;
			}
			if (item && typeof item === 'object') {
				const obj = item as Record<string, unknown>;
				const p =
					toPositiveInt(obj.page_number) ??
					toPositiveInt(obj.page) ??
					toPositiveInt(obj.page_no) ??
					toPositiveInt(obj.pageNo);
				const l =
					toPositiveInt(obj.line_number) ??
					toPositiveInt(obj.line) ??
					toPositiveInt(obj.line_no) ??
					toPositiveInt(obj.lineNo);
				if (p && l) out.push({ page_number: p, line_number: l });
			}
		}
		return out;
	}

	let highlightKeys = $derived.by(() => {
		const s = new Set<string>();
		if (selectedMetricId == null) return s;
		const m = metrics.find((x) => x.id === selectedMetricId);
		for (const span of normalizeMetricSpans(m)) {
			s.add(`${span.page_number}:${span.line_number}`);
		}
		return s;
	});

	let rawLineByKey = $derived.by(() => {
		const map = new Map<string, RawLine>();
		for (const ln of rawLines) {
			map.set(`${ln.page_number}:${ln.line_number}`, ln);
		}
		return map;
	});

	let selectedLinesByPage = $derived.by(() => {
		const map = new Map<number, RawLine[]>();
		if (selectedMetricId == null) return map;
		const metric = metrics.find((x) => x.id === selectedMetricId);
		const spans = normalizeMetricSpans(metric);
		if (spans.length === 0) return map;
		for (const span of spans) {
			const ln = rawLineByKey.get(`${span.page_number}:${span.line_number}`);
			if (ln && Array.isArray(ln.coords) && ln.coords.length >= 4) {
				const arr = map.get(span.page_number) ?? [];
				arr.push(ln);
				map.set(span.page_number, arr);
			}
		}
		return map;
	});

	let pagesGrouped = $derived.by(() => {
		const map = new Map<number, RawLine[]>();
		for (const ln of rawLines) {
			const arr = map.get(ln.page_number) ?? [];
			arr.push(ln);
			map.set(ln.page_number, arr);
		}
		return Array.from(map.entries())
			.sort((a, b) => a[0] - b[0])
			.map(([page, lines]) => ({
				page,
				lines: lines.sort((a, b) => a.line_number - b.line_number)
			}));
	});

	async function doRetrieve() {
		errorMsg = '';
		const id = Number(recordIdInput.trim());
		if (!Number.isFinite(id) || id <= 0) {
			errorMsg = 'Enter a valid Record ID';
			return;
		}
		loading = true;
		metrics = [];
		selectedMetricId = null;
		rawLines = [];
		rawError = '';
		currentInput = null;
		docPage = 1;
		pdfError = '';
		if (pdfDoc?.destroy) {
			await pdfDoc.destroy();
		}
		pdfDoc = null;
		pdfNumPages = 0;
		pdfRenderedPages = [];
		pdfLoadedInputId = 0;
		pdfViewportByPage.clear();
		try {
			const [metricRes, inputRes] = await Promise.all([
				listKbMetrics(id),
				getKbInput(id).catch(() => null)
			]);
			metrics = metricRes.results ?? [];
			currentInput = inputRes?.record ?? null;
			loadRawLines(id);
		} catch (err) {
			errorMsg = err instanceof Error ? err.message : 'Failed to retrieve metrics';
		} finally {
			loading = false;
		}
	}

	async function loadRawLines(id: number) {
		rawLoading = true;
		rawError = '';
		try {
			const res = await getRawLines(id);
			rawLines = res.lines ?? [];
		} catch (err) {
			rawError = err instanceof Error ? err.message : 'Failed to load raw lines';
		} finally {
			rawLoading = false;
		}
	}

	async function selectMetric(m: KbMetricRecord) {
		selectedMetricId = m.id;
		const first = normalizeMetricSpans(m)[0];
		if (!first) return;

		// Move display to the selected page without forcing iframe remount/reload.
		docPage = first.page_number > 0 ? first.page_number : 1;
		if (viewMode === 'document' && isPdf) {
			await tick();
			scrollPdfToPage(docPage, 'smooth');
		}

		// If user is on the source-lines tab, scroll the highlighted line into view.
		if (viewMode === 'source') {
			await tick();
			const target = document.getElementById(`raw-${first.page_number}-${first.line_number}`);
			if (target) {
				target.scrollIntoView({ behavior: 'smooth', block: 'center' });
			} else {
				document.getElementById(`page-${first.page_number}`)?.scrollIntoView({ behavior: 'smooth', block: 'start' });
			}
		}
	}

	function setMode(mode: 'document' | 'source') {
		viewMode = mode;
	}

	function openSearch() {
		searchOpen = true;
		searchSelected = null;
		searchResults = [];
		searchError = '';
		searchFileName = '';
	}
	function closeSearch() { searchOpen = false; }

	async function runSearch() {
		searchLoading = true;
		searchError = '';
		try {
			const res = await listKbInputs({
				docType: searchDocType,
				parseState: 'all',
				fileName: searchFileName,
				startTime: '',
				endTime: '',
				page: 1,
				pageSize: 50
			});
			searchResults = res.results ?? [];
		} catch (err) {
			searchError = err instanceof Error ? err.message : 'Search failed';
		} finally {
			searchLoading = false;
		}
	}
	function pickSearchResult(r: KbInputRecord) {
		recordIdInput = String(r.id);
		searchOpen = false;
	}
	function confirmSearchSelection() {
		const r = searchResults.find((x) => x.id === searchSelected);
		if (r) pickSearchResult(r);
	}

	function metricNameOf(m: KbMetricRecord): string {
		return m.metric_name?.trim() || m.metric_subject?.trim() || `Metric #${m.id}`;
	}
	function confidencePct(c?: number): string {
		if (c == null) return '—';
		return `${Math.round(c * 100)}%`;
	}
	function spanCount(m: KbMetricRecord): number {
		return normalizeMetricSpans(m).length;
	}

	function clampDocPage(page: number): number {
		const normalized = toPositiveInt(page) ?? 1;
		const max = Math.max(1, pdfNumPages || 1);
		return Math.max(1, Math.min(normalized, max));
	}
	function goToPage(page: number, behavior: ScrollBehavior = 'smooth') {
		docPage = clampDocPage(page);
		if (isPdf) {
			scrollPdfToPage(docPage, behavior);
		}
	}

	function scrollPdfToPage(pageNo: number, behavior: ScrollBehavior = 'smooth') {
		const pageEl = document.getElementById(`pdf-page-${pageNo}`);
		if (!pageEl) return;
		pageEl.scrollIntoView({ behavior, block: 'start' });
	}

	function zoomIn() {
		pdfZoom = Math.min(3, Number((pdfZoom + 0.1).toFixed(2)));
	}
	function zoomOut() {
		pdfZoom = Math.max(0.4, Number((pdfZoom - 0.1).toFixed(2)));
	}
	function zoomLabel(): string {
		return `${Math.round(pdfZoom * 100)}%`;
	}

	async function ensurePdfLib() {
		if (pdfLib) return;
		const mod = (await import('pdfjs-dist')) as unknown as PdfJsLib;
		pdfLib = mod;
		if (pdfLib.GlobalWorkerOptions) {
			pdfLib.GlobalWorkerOptions.workerSrc = new URL('pdfjs-dist/build/pdf.worker.mjs', import.meta.url).toString();
		}
	}

	async function ensurePdfDoc() {
		if (!currentInput || !isPdf) return;
		if (pdfDoc && pdfLoadedInputId === currentInput.id) return;

		if (pdfDoc?.destroy) {
			await pdfDoc.destroy();
		}
		pdfDoc = null;
		pdfLoadedInputId = 0;
		pdfError = '';

		await ensurePdfLib();
		if (!pdfLib) return;
		const task = pdfLib.getDocument({
			url: `/api/v1/kb/inputs/${currentInput.id}/file`,
			withCredentials: true
		});
		pdfDoc = (await task.promise) as PdfDocumentProxy;
		pdfLoadedInputId = currentInput.id;
		pdfNumPages = Math.max(1, pdfDoc.numPages || 1);
		pdfRenderedPages = Array.from({ length: pdfNumPages }, (_, i) => i + 1);
	}

		function drawPdfHighlights() {
		for (const pageNo of pdfRenderedPages) {
			const overlay = document.getElementById(`pdf-overlay-${pageNo}`) as HTMLDivElement | null;
			const viewport = pdfViewportByPage.get(pageNo);
			if (!overlay || !viewport) continue;
			overlay.innerHTML = '';
			const lines = selectedLinesByPage.get(pageNo) ?? [];
			for (const ln of lines) {
				if (!Array.isArray(ln.coords) || ln.coords.length < 4) continue;
				const [vx1, vy1, vx2, vy2] = viewport.convertToViewportRectangle(ln.coords.slice(0, 4));
				const left = Math.min(vx1, vx2);
				const top = Math.min(vy1, vy2);
				const width = Math.abs(vx2 - vx1);
				const height = Math.abs(vy2 - vy1);
				if (width < 1 || height < 1) continue;
				const mark = document.createElement('div');
				mark.className = 'pdf-highlight';
				mark.style.left = `${left}px`;
				mark.style.top = `${top}px`;
				mark.style.width = `${width}px`;
				mark.style.height = `${height}px`;
				mark.title = `line ${ln.line_number}`;
				overlay.appendChild(mark);
			}
		}
	}

	async function renderPdfPages() {
		if (!pdfDoc || !pdfStageEl || pdfRenderedPages.length === 0) return;
		const stageWidth = Math.floor(pdfStageEl.clientWidth);
		if (stageWidth <= 0) return;
		const seq = ++pdfRenderSeq;
		pdfLastRenderWidth = stageWidth;
		pdfLoading = true;
		pdfError = '';
		try {
			const firstPage = await pdfDoc.getPage(1);
			const firstPageRotation = firstPage.rotate ?? 0;
			const baseViewport = firstPage.getViewport({ scale: 1, rotation: firstPageRotation });
			const availableWidth = Math.max(stageWidth - 40, 320);
			const baseScale = availableWidth / baseViewport.width;
			const targetScale = Math.max(0.1, baseScale * pdfZoom);

			for (const pageNo of pdfRenderedPages) {
				if (seq !== pdfRenderSeq) return;
				const canvas = document.getElementById(`pdf-canvas-${pageNo}`) as HTMLCanvasElement | null;
				const overlay = document.getElementById(`pdf-overlay-${pageNo}`) as HTMLDivElement | null;
				if (!canvas || !overlay) continue;
				const page = await pdfDoc.getPage(pageNo);
				const pageRotation = page.rotate ?? 0;
				const viewport = page.getViewport({ scale: targetScale, rotation: pageRotation });
				const ctx = canvas.getContext('2d');
				if (!ctx) continue;

				canvas.width = Math.floor(viewport.width);
				canvas.height = Math.floor(viewport.height);
				canvas.style.width = `${viewport.width}px`;
				canvas.style.height = `${viewport.height}px`;
				overlay.style.width = `${viewport.width}px`;
				overlay.style.height = `${viewport.height}px`;
				pdfViewportByPage.set(pageNo, viewport);

				ctx.setTransform(1, 0, 0, 1, 0, 0);
				ctx.clearRect(0, 0, viewport.width, viewport.height);
				await page.render({ canvasContext: ctx, viewport }).promise;
			}

			if (seq !== pdfRenderSeq) return;
			drawPdfHighlights();
		} catch (err) {
			pdfError = err instanceof Error ? err.message : 'Failed to render PDF';
		} finally {
			if (seq === pdfRenderSeq) {
				pdfLoading = false;
			}
		}
	}

	$effect(() => {
		const canRenderPdf = viewMode === 'document' && isPdf && !!currentInput && !!pdfStageEl;
		if (!canRenderPdf) return;
		pdfZoom;
		pdfRenderedPages.length;
		let cancelled = false;
		(async () => {
			await ensurePdfDoc();
			if (cancelled) return;
			await tick();
			if (cancelled) return;
			await renderPdfPages();
		})();
		return () => {
			cancelled = true;
		};
	});

	$effect(() => {
		// Jump when target page changes (e.g. clicking a metric card, prev/next, page input).
		if (viewMode !== 'document' || !isPdf || !pdfDoc) return;
		docPage;
		void tick().then(() => scrollPdfToPage(clampDocPage(docPage), 'smooth'));
	});

	$effect(() => {
		if (viewMode !== 'document' || !isPdf || pdfViewportByPage.size === 0) return;
		selectedLinesByPage;
		drawPdfHighlights();
	});

	$effect(() => {
		if (viewMode !== 'document' || !isPdf || !pdfStageEl) return;
		const ro = new ResizeObserver(() => {
			const w = Math.floor(pdfStageEl?.clientWidth ?? 0);
			if (w <= 0 || w === pdfLastRenderWidth) return;
			if (pdfResizeRaf) cancelAnimationFrame(pdfResizeRaf);
			pdfResizeRaf = requestAnimationFrame(() => {
				void renderPdfPages();
			});
		});
		ro.observe(pdfStageEl);
		return () => {
			ro.disconnect();
			if (pdfResizeRaf) cancelAnimationFrame(pdfResizeRaf);
			pdfResizeRaf = 0;
		};
	});

	onMount(() => {
		return () => {
			if (pdfDoc?.destroy) {
				void pdfDoc.destroy();
			}
		};
	});
</script>

<div
	class="metric-mgmt"
	style="
		--page-bg:{pageBg};
		--panel-bg:{panelBg};
		--panel-bg-alt:{panelBgAlt};
		--ink-line:{inkLine};
		--ink-line-soft:{inkLineSoft};
		--text-primary:{textPrimary};
		--text-secondary:{textSecondary};
		--text-muted:{textMuted};
		--brass:{brass};
		--brass-faint:{brassFaint};
		--crimson:{crimson};
		--crimson-faint:{crimsonFaint};
		--teal:{teal};
		--font-serif:{fontSerif};
		--font-mono:{fontMono};
		--font-sans:{fontSans};
	"
>
	<header class="header">
		<div class="header-left">
			<div class="eyebrow">Knowledge Base · Vol. III</div>
			<h1 class="display">Metrics&nbsp;<span class="amp">&amp;</span>&nbsp;Provenance</h1>
			<div class="subtitle">A reading room for extracted metrics — locate, verify, return to source.</div>
		</div>
		<div class="header-right">
			<span class="meta-label">RECORD</span><span class="meta-val">{currentInput?.id ?? '—'}</span>
			<span class="meta-label">TYPE</span><span class="meta-val">{currentInput?.type ?? '—'}</span>
			<span class="meta-label">METRICS</span><span class="meta-val">{metrics.length.toString().padStart(3, '0')}</span>
		</div>
	</header>

	<div class="body">
		<!-- ============ LEFT PANEL ============ -->
		<aside class="left">
			<div class="left-controls">
				<label class="field">
					<span class="field-label">Record&nbsp;ID</span>
					<div class="field-row">
						<input
							type="text"
							inputmode="numeric"
							bind:value={recordIdInput}
							placeholder="e.g. 1042"
							onkeydown={(e) => { if (e.key === 'Enter') doRetrieve(); }}
						/>
						<button class="btn btn-ghost" onclick={openSearch} title="Search records from kb.inputs">
							<span class="btn-icon">⌕</span>Search
						</button>
					</div>
				</label>

				<button class="btn btn-primary retrieve" onclick={doRetrieve} disabled={loading}>
					{#if loading}
						<span class="spinner"></span>Retrieving…
					{:else}
						<span class="btn-icon">→</span>Retrieve
					{/if}
				</button>

				{#if errorMsg}
					<div class="error">{errorMsg}</div>
				{/if}
			</div>

			<div class="left-meta">
				<div class="left-meta-title">Metrics</div>
				<div class="left-meta-count">{metrics.length} found</div>
			</div>

			<div class="metrics-list">
				{#if !loading && metrics.length === 0}
					<div class="empty">
						<div class="empty-glyph">§</div>
						<div class="empty-title">No metrics yet</div>
						<div class="empty-sub">Enter a Record ID and press Retrieve to populate the index.</div>
					</div>
				{:else}
					{#each metrics as m, idx (m.id)}
						<button
							class="metric-card"
							class:selected={selectedMetricId === m.id}
							onclick={() => selectMetric(m)}
						>
							<div class="card-rule" aria-hidden="true"></div>
							<div class="card-body">
								<div class="card-row-top">
									<div class="card-index">№ {String(idx + 1).padStart(3, '0')}</div>
									<div class="card-conf" title="Confidence">{confidencePct(m.confidence)}</div>
								</div>
								<div class="card-name">{metricNameOf(m)}</div>
								{#if m.metric_desc}
									<div class="card-desc">{m.metric_desc}</div>
								{/if}
								<div class="card-foot">
									<span class="chip">
										<span class="chip-dot"></span>
										{spanCount(m)} span{spanCount(m) === 1 ? '' : 's'}
									</span>
									{#if m.metric_unit}<span class="chip chip-mono">{m.metric_unit}</span>{/if}
									{#if m.location_type}<span class="chip chip-quiet">{m.location_type}</span>{/if}
								</div>
							</div>
						</button>
					{/each}
				{/if}
			</div>
		</aside>

		<!-- ============ RIGHT PANEL ============ -->
		<section class="right">
			<div class="right-toolbar">
				<div class="right-title">
					{#if currentInput}
						<span class="title-glyph">¶</span>
						<span class="title-name" title={currentInput.file_name ?? ''}>{currentInput.file_name ?? '(unnamed file)'}</span>
						<span class="title-type">{currentInput.type}</span>
					{:else}
						<span class="title-glyph">¶</span>
						<span class="title-name muted">No document loaded</span>
					{/if}
				</div>

				<div class="tabs" role="tablist" aria-label="Right panel mode">
					<button
						class="tab"
						class:active={viewMode === 'document'}
						onclick={() => setMode('document')}
						role="tab"
						aria-selected={viewMode === 'document'}
					>
						Document
					</button>
					<button
						class="tab"
						class:active={viewMode === 'source'}
						onclick={() => setMode('source')}
						role="tab"
						aria-selected={viewMode === 'source'}
					>
						Source&nbsp;Lines
					</button>
				</div>

				<div class="right-stats">
					{#if currentInput && viewMode === 'document'}
						<span class="stat"><span class="stat-num">{String(docPage).padStart(3, '0')}</span><span class="stat-label">page</span></span>
						<span class="stat"><span class="stat-num">{highlightKeys.size}</span><span class="stat-label">marks</span></span>
					{:else if pagesGrouped.length > 0}
						<span class="stat"><span class="stat-num">{pagesGrouped.length}</span><span class="stat-label">pages</span></span>
						<span class="stat"><span class="stat-num">{rawLines.length}</span><span class="stat-label">lines</span></span>
						<span class="stat"><span class="stat-num">{highlightKeys.size}</span><span class="stat-label">marks</span></span>
					{/if}
				</div>
			</div>

			{#if viewMode === 'document'}
				<div class="doc-frame-wrap">
					{#if !currentInput}
						<div class="doc-empty">
							<div class="doc-empty-mark">⌬</div>
							<div class="doc-empty-title">Awaiting selection</div>
							<div class="doc-empty-sub">
								Once you retrieve a record, the original document appears here.<br />
								Click any metric on the left to jump to its source page.
							</div>
						</div>
					{:else}
						<div class="doc-page-bar">
							<button
								class="page-btn"
								onclick={() => goToPage(docPage - 1)}
								disabled={docPage <= 1}
								aria-label="Previous page"
							>‹</button>
							<div class="page-bar-label">
								<span class="page-bar-folio">page</span>
								<input
									type="number"
									min="1"
									max={Math.max(1, pdfNumPages)}
									class="page-input"
									bind:value={docPage}
									onchange={() => goToPage(docPage)}
								/>
								<span class="page-total">/ {Math.max(1, pdfNumPages)}</span>
							</div>
							<button
								class="page-btn"
								onclick={() => goToPage(docPage + 1)}
								disabled={docPage >= Math.max(1, pdfNumPages)}
								aria-label="Next page"
							>›</button>
							<button class="page-btn small" onclick={zoomOut} title="Zoom out" aria-label="Zoom out">−</button>
							<span class="zoom-label">{zoomLabel()}</span>
							<button class="page-btn small" onclick={zoomIn} title="Zoom in" aria-label="Zoom in">+</button>
							<div class="page-bar-spacer"></div>
							<a class="page-btn small" href={fileUrl} target="_blank" rel="noopener" title="Open in new tab">↗</a>
						</div>

						{#if isPdf}
							<div class="pdf-stage" bind:this={pdfStageEl}>
								<div class="pdf-pages">
									{#each pdfRenderedPages as pageNo (pageNo)}
										<div
											class="pdf-page"
											id={`pdf-page-${pageNo}`}
											data-page={pageNo}
										>
											<div class="pdf-page-head">
												<span class="pdf-page-label">page</span>
												<span class="pdf-page-num">{String(pageNo).padStart(3, '0')}</span>
											</div>
											<div class="pdf-canvas-shell">
												<canvas class="pdf-canvas" id={`pdf-canvas-${pageNo}`}></canvas>
												<div class="pdf-overlay" id={`pdf-overlay-${pageNo}`}></div>
											</div>
										</div>
									{/each}
								</div>
								{#if pdfLoading}
									<div class="pdf-status"><span class="dot-loop"></span>Rendering page…</div>
								{/if}
								{#if pdfError}
									<div class="doc-error" style="padding-top:20px;">
										<div class="doc-error-title">⚠ Cannot render this PDF</div>
										<div class="doc-error-msg">
											{pdfError}<br />
											<a class="doc-error-link" href={fileUrl} target="_blank" rel="noopener">Open in a new tab</a>
											&nbsp;or switch to <button class="inline-tab-btn" onclick={() => setMode('source')}>Source&nbsp;Lines</button>.
										</div>
									</div>
								{/if}
							</div>
						{:else}
							<iframe
								class="doc-frame"
								title={currentInput.file_name ?? `Record ${currentInput.id}`}
								src={fileUrl}
							></iframe>
						{/if}

						{#if isText}
							<div class="doc-foot-hint">
								This file is rendered as text by your browser. For exact line highlighting, switch to the <button class="inline-tab-btn" onclick={() => setMode('source')}>Source&nbsp;Lines</button> tab.
							</div>
						{:else if !isPdf}
							<div class="doc-foot-hint">
								Inline preview support varies by file type. For line-level highlights, use the <button class="inline-tab-btn" onclick={() => setMode('source')}>Source&nbsp;Lines</button> tab.
							</div>
						{/if}
					{/if}
				</div>
			{:else}
			<div class="document">
				{#if rawLoading}
					<div class="doc-status"><span class="dot-loop"></span>Reading raw_line file…</div>
				{:else if rawError}
					<div class="doc-error">
						<div class="doc-error-title">⚠ Cannot render document</div>
						<div class="doc-error-msg">{rawError}</div>
					</div>
				{:else if pagesGrouped.length === 0}
					<div class="doc-empty">
						<div class="doc-empty-mark">⌬</div>
						<div class="doc-empty-title">Awaiting selection</div>
						<div class="doc-empty-sub">
							Once you retrieve a record, the parsed document appears here.<br />
							Click any metric on the left to jump to its source line.
						</div>
					</div>
				{:else}
					{#each pagesGrouped as pg (pg.page)}
						<article id={`page-${pg.page}`} class="page">
							<div class="page-edge" aria-hidden="true"></div>
							<header class="page-head">
								<span class="page-folio">page</span>
								<span class="page-num">{String(pg.page).padStart(3, '0')}</span>
								<span class="page-rule"></span>
								<span class="page-count">{pg.lines.length} lines</span>
							</header>
							<div class="page-body">
								{#each pg.lines as ln (ln.line_number)}
									{@const k = `${ln.page_number}:${ln.line_number}`}
									{@const hl = highlightKeys.has(k)}
									<div
										id={`raw-${ln.page_number}-${ln.line_number}`}
										class="line"
										class:highlight={hl}
										class:line-heading={ln.line_type?.includes('header') || ln.line_type === 'title'}
										class:line-list={ln.line_type === 'list-item'}
										class:line-footer={ln.line_type === 'footer'}
									>
										<span class="line-no">{String(ln.line_number).padStart(4, '0')}</span>
										<span class="line-type">{ln.line_type}</span>
										<span class="line-content">{ln.content}</span>
									</div>
								{/each}
							</div>
						</article>
					{/each}
				{/if}
			</div>
			{/if}
		</section>
	</div>
</div>

<!-- ============ SEARCH DIALOG ============ -->
{#if searchOpen}
	<div
		class="dialog-overlay"
		onclick={closeSearch}
		onkeydown={(e) => { if (e.key === 'Escape') closeSearch(); }}
		role="button"
		tabindex="0"
		aria-label="Close search"
	>
		<div
			class="dialog"
			style="background:{panelBg}; border-color:{inkLine};"
			onclick={(e) => e.stopPropagation()}
			onkeydown={(e) => e.stopPropagation()}
			role="dialog"
			aria-modal="true"
			tabindex="0"
		>
			<div class="dialog-head">
				<div>
					<div class="dialog-eyebrow">kb.inputs</div>
					<h2 class="dialog-title">Find a record</h2>
				</div>
				<button class="dialog-close" onclick={closeSearch} aria-label="Close">×</button>
			</div>

			<div class="dialog-controls">
				<label class="field flex-2">
					<span class="field-label">File name contains</span>
					<input
						type="text"
						bind:value={searchFileName}
						placeholder="report, spec, drawing…"
						onkeydown={(e) => { if (e.key === 'Enter') runSearch(); }}
					/>
				</label>
				<label class="field flex-1">
					<span class="field-label">Type</span>
					<select bind:value={searchDocType}>
						{#each docTypeOptions as o}
							<option value={o}>{o}</option>
						{/each}
					</select>
				</label>
				<button class="btn btn-primary" onclick={runSearch} disabled={searchLoading}>
					{searchLoading ? 'Searching…' : 'Search'}
				</button>
			</div>

			{#if searchError}
				<div class="error" style="margin:0 28px;">{searchError}</div>
			{/if}

			<div class="dialog-results">
				{#if searchResults.length === 0 && !searchLoading}
					<div class="dialog-empty">
						<div class="empty-glyph">⌕</div>
						<div>Run a search to see records.</div>
					</div>
				{:else}
					<table class="result-table">
						<thead>
							<tr>
								<th>ID</th>
								<th>Type</th>
								<th>File name</th>
								<th>Created</th>
							</tr>
						</thead>
						<tbody>
							{#each searchResults as r (r.id)}
								<tr
									class:selected={searchSelected === r.id}
									onclick={() => (searchSelected = r.id)}
									ondblclick={() => pickSearchResult(r)}
								>
									<td class="mono">{r.id}</td>
									<td>{r.type}</td>
									<td class="ellipsis">{r.file_name ?? '—'}</td>
									<td class="mono muted">{(r.create_time ?? '').slice(0, 19).replace('T', ' ')}</td>
								</tr>
							{/each}
						</tbody>
					</table>
				{/if}
			</div>

			<div class="dialog-foot">
				<div class="dialog-foot-hint">Double-click a row, or select &amp; press Select.</div>
				<div class="dialog-foot-buttons">
					<button class="btn btn-ghost" onclick={closeSearch}>Cancel</button>
					<button class="btn btn-primary" onclick={confirmSearchSelection} disabled={searchSelected == null}>Select</button>
				</div>
			</div>
		</div>
	</div>
{/if}

<style>
	@import url('https://fonts.googleapis.com/css2?family=Cormorant+Garamond:ital,wght@0,400;0,500;0,600;1,400&family=JetBrains+Mono:wght@400;500;600&family=Inter+Tight:wght@400;500;600&display=swap');

	.metric-mgmt {
		height: 100%;
		min-height: 100%;
		display: flex;
		flex-direction: column;
		background: var(--page-bg);
		color: var(--text-primary);
		font-family: var(--font-sans);
		overflow: hidden;
	}

	/* ---------- HEADER ---------- */
	.header {
		display: flex;
		justify-content: space-between;
		align-items: flex-end;
		padding: 32px 40px 24px;
		border-bottom: 1px solid var(--ink-line);
		position: relative;
		gap: 24px;
	}
	.header::after {
		content: '';
		position: absolute;
		left: 40px; right: 40px; bottom: -1px;
		height: 1px;
		background: linear-gradient(90deg, var(--brass), transparent 35%, transparent 65%, var(--brass));
		opacity: 0.45;
	}
	.eyebrow {
		font-family: var(--font-mono);
		font-size: 11px;
		letter-spacing: 0.18em;
		text-transform: uppercase;
		color: var(--brass);
		margin-bottom: 6px;
	}
	.display {
		font-family: var(--font-serif);
		font-size: 46px;
		line-height: 1;
		font-weight: 500;
		letter-spacing: -0.01em;
		margin: 0 0 8px;
	}
	.display .amp { font-style: italic; color: var(--brass); font-weight: 400; }
	.subtitle {
		font-family: var(--font-serif);
		font-style: italic;
		font-size: 15px;
		color: var(--text-secondary);
		max-width: 520px;
	}
	.header-right {
		display: grid;
		grid-template-columns: auto auto;
		gap: 6px 18px;
		align-items: baseline;
		padding: 12px 18px;
		border: 1px solid var(--ink-line);
		background: var(--panel-bg-alt);
	}
	.meta-label {
		font-family: var(--font-mono);
		font-size: 10px;
		letter-spacing: 0.16em;
		color: var(--text-muted);
		text-transform: uppercase;
	}
	.meta-val {
		font-family: var(--font-mono);
		font-size: 13px;
		color: var(--text-primary);
		text-align: right;
		min-width: 60px;
	}

	/* ---------- BODY ---------- */
	.body {
		flex: 1;
		height: 100%;
		display: grid;
		grid-template-columns: 380px 1fr;
		min-height: 0;
		min-width: 0;
		overflow: hidden;
	}

	/* ---------- LEFT ---------- */
	.left {
		display: flex;
		flex-direction: column;
		border-right: 1px solid var(--ink-line);
		background: var(--panel-bg);
		min-width: 0;
		overflow: hidden;
	}
	.left-controls {
		padding: 24px 24px 18px;
		display: flex;
		flex-direction: column;
		gap: 14px;
		border-bottom: 1px solid var(--ink-line-soft);
	}
	.field { display: flex; flex-direction: column; gap: 6px; }
	.field-label {
		font-family: var(--font-mono);
		font-size: 10px;
		letter-spacing: 0.14em;
		text-transform: uppercase;
		color: var(--text-muted);
	}
	.field-row { display: flex; gap: 8px; }
	.field-row input { flex: 1; min-width: 0; }
	.field input,
	.field select {
		font-family: var(--font-mono);
		font-size: 14px;
		padding: 10px 12px;
		background: var(--panel-bg-alt);
		color: var(--text-primary);
		border: 1px solid var(--ink-line);
		border-radius: 0;
		outline: none;
		transition: border-color 150ms, background 150ms;
	}
	.field input:focus,
	.field select:focus {
		border-color: var(--brass);
		background: var(--panel-bg);
	}

	.btn {
		font-family: var(--font-sans);
		font-size: 13px;
		font-weight: 600;
		letter-spacing: 0.02em;
		padding: 10px 14px;
		border: 1px solid var(--ink-line);
		border-radius: 0;
		background: transparent;
		color: var(--text-primary);
		cursor: pointer;
		display: inline-flex;
		align-items: center;
		justify-content: center;
		gap: 8px;
		transition: background 150ms, color 150ms, border-color 150ms, transform 100ms;
	}
	.btn:hover { background: var(--brass-faint); border-color: var(--brass); color: var(--brass); }
	.btn:active { transform: translateY(1px); }
	.btn:disabled { opacity: 0.5; cursor: not-allowed; transform: none; }
	.btn-icon { font-family: var(--font-mono); font-size: 14px; line-height: 1; }
	.btn-primary {
		background: var(--brass);
		border-color: var(--brass);
		color: #1A1410;
	}
	.btn-primary:hover {
		background: var(--text-primary);
		color: var(--brass);
		border-color: var(--text-primary);
	}
	.btn-ghost { background: var(--panel-bg-alt); }
	.retrieve { padding: 12px; font-size: 14px; }

	.error {
		font-family: var(--font-mono);
		font-size: 12px;
		color: var(--crimson);
		padding: 8px 10px;
		background: var(--crimson-faint);
		border-left: 3px solid var(--crimson);
	}

	.left-meta {
		padding: 18px 24px 8px;
		display: flex;
		justify-content: space-between;
		align-items: baseline;
	}
	.left-meta-title {
		font-family: var(--font-serif);
		font-size: 22px;
		font-weight: 500;
		color: var(--text-primary);
	}
	.left-meta-count {
		font-family: var(--font-mono);
		font-size: 11px;
		letter-spacing: 0.1em;
		color: var(--text-muted);
		text-transform: uppercase;
	}

	.metrics-list {
		flex: 1;
		overflow-y: auto;
		padding: 0 16px 24px;
		display: flex;
		flex-direction: column;
		gap: 10px;
		scrollbar-width: thin;
		scrollbar-color: var(--ink-line) transparent;
	}
	.metrics-list::-webkit-scrollbar { width: 8px; }
	.metrics-list::-webkit-scrollbar-thumb { background: var(--ink-line); }

	.empty {
		text-align: center;
		padding: 60px 20px;
		color: var(--text-muted);
	}
	.empty-glyph {
		font-family: var(--font-serif);
		font-size: 56px;
		color: var(--brass);
		opacity: 0.4;
		line-height: 1;
		margin-bottom: 12px;
	}
	.empty-title {
		font-family: var(--font-serif);
		font-size: 18px;
		color: var(--text-secondary);
		margin-bottom: 4px;
	}
	.empty-sub {
		font-size: 12px;
		max-width: 240px;
		margin: 0 auto;
		line-height: 1.5;
	}

	.metric-card {
		all: unset;
		cursor: pointer;
		display: flex;
		background: var(--panel-bg-alt);
		border: 1px solid var(--ink-line-soft);
		position: relative;
		transition: border-color 150ms, background 150ms;
	}
	.metric-card:hover { border-color: var(--brass); background: var(--panel-bg); }
	.metric-card.selected { border-color: var(--crimson); background: var(--panel-bg); }
	.card-rule {
		width: 4px;
		background: var(--ink-line);
		flex-shrink: 0;
		transition: background 150ms;
	}
	.metric-card:hover .card-rule { background: var(--brass); }
	.metric-card.selected .card-rule { background: var(--crimson); }
	.card-body {
		padding: 12px 14px 12px 12px;
		flex: 1;
		min-width: 0;
		display: flex;
		flex-direction: column;
		gap: 6px;
	}
	.card-row-top {
		display: flex;
		justify-content: space-between;
		align-items: baseline;
	}
	.card-index {
		font-family: var(--font-mono);
		font-size: 10px;
		letter-spacing: 0.1em;
		color: var(--text-muted);
		text-transform: uppercase;
	}
	.card-conf {
		font-family: var(--font-mono);
		font-size: 11px;
		color: var(--teal);
		font-weight: 600;
	}
	.card-name {
		font-family: var(--font-serif);
		font-size: 17px;
		line-height: 1.2;
		font-weight: 500;
		color: var(--text-primary);
	}
	.metric-card.selected .card-name { color: var(--crimson); }
	.card-desc {
		font-size: 12px;
		line-height: 1.45;
		color: var(--text-secondary);
		display: -webkit-box;
		-webkit-line-clamp: 2;
		-webkit-box-orient: vertical;
		overflow: hidden;
	}
	.card-foot {
		display: flex;
		gap: 6px;
		flex-wrap: wrap;
		margin-top: 2px;
	}
	.chip {
		display: inline-flex;
		align-items: center;
		gap: 5px;
		font-family: var(--font-mono);
		font-size: 10px;
		text-transform: uppercase;
		letter-spacing: 0.06em;
		padding: 3px 8px;
		background: transparent;
		border: 1px solid var(--ink-line);
		color: var(--text-secondary);
	}
	.chip-dot {
		width: 4px; height: 4px;
		background: var(--brass);
		border-radius: 50%;
	}
	.chip-mono { color: var(--brass); border-color: var(--brass); }
	.chip-quiet { color: var(--text-muted); }

	/* ---------- RIGHT ---------- */
	.right {
		display: flex;
		flex-direction: column;
		height: 100%;
		min-width: 0;
		min-height: 0;
		background: var(--page-bg);
		overflow: hidden;
	}
	.right-toolbar {
		display: flex;
		justify-content: space-between;
		align-items: center;
		padding: 16px 28px;
		border-bottom: 1px solid var(--ink-line);
		background: var(--panel-bg);
		gap: 16px;
	}
	.right-title {
		display: flex;
		align-items: baseline;
		gap: 10px;
		min-width: 0;
		flex: 1;
	}
	.title-glyph {
		font-family: var(--font-serif);
		font-size: 22px;
		color: var(--brass);
		line-height: 1;
	}
	.title-name {
		font-family: var(--font-serif);
		font-size: 19px;
		color: var(--text-primary);
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
		flex: 1;
		min-width: 0;
	}
	.title-name.muted { color: var(--text-muted); font-style: italic; }
	.title-type {
		font-family: var(--font-mono);
		font-size: 10px;
		text-transform: uppercase;
		letter-spacing: 0.1em;
		color: var(--text-muted);
		padding: 2px 6px;
		border: 1px solid var(--ink-line);
	}
	.right-stats {
		display: flex;
		gap: 18px;
		flex-shrink: 0;
	}
	.stat {
		display: flex;
		flex-direction: column;
		align-items: flex-end;
		line-height: 1;
	}
	.stat-num {
		font-family: var(--font-serif);
		font-size: 22px;
		color: var(--brass);
		font-weight: 500;
	}
	.stat-label {
		font-family: var(--font-mono);
		font-size: 9px;
		text-transform: uppercase;
		letter-spacing: 0.12em;
		color: var(--text-muted);
		margin-top: 4px;
	}

	.document {
		flex: 1;
		overflow-y: auto;
		padding: 32px 36px 80px;
		scrollbar-width: thin;
		scrollbar-color: var(--ink-line) transparent;
	}
	.document::-webkit-scrollbar { width: 10px; }
	.document::-webkit-scrollbar-thumb { background: var(--ink-line); }

	/* Inline document viewer */
	.doc-frame-wrap {
		flex: 1;
		height: 100%;
		min-height: 0;
		display: flex;
		flex-direction: column;
		padding: 12px 20px 16px;
		gap: 12px;
		overflow: hidden;
	}
	.doc-page-bar {
		display: flex;
		align-items: center;
		gap: 10px;
		padding: 8px 10px;
		background: var(--panel-bg);
		border: 1px solid var(--ink-line);
	}
	.page-btn {
		all: unset;
		cursor: pointer;
		font-family: var(--font-mono);
		font-size: 16px;
		line-height: 1;
		color: var(--text-primary);
		padding: 6px 10px;
		border: 1px solid var(--ink-line);
		background: var(--panel-bg-alt);
		transition: border-color 150ms, color 150ms, background 150ms;
	}
	.page-btn:hover {
		border-color: var(--brass);
		color: var(--brass);
		background: var(--panel-bg);
	}
	.page-btn:disabled {
		opacity: 0.45;
		cursor: not-allowed;
	}
	.page-btn.small {
		font-size: 14px;
		padding: 6px 8px;
	}
	.page-bar-label {
		display: inline-flex;
		align-items: center;
		gap: 8px;
	}
	.page-total {
		font-family: var(--font-mono);
		font-size: 11px;
		color: var(--text-muted);
	}
	.zoom-label {
		min-width: 52px;
		text-align: center;
		font-family: var(--font-mono);
		font-size: 12px;
		color: var(--text-primary);
	}
	.page-bar-folio {
		font-family: var(--font-mono);
		font-size: 10px;
		letter-spacing: 0.14em;
		text-transform: uppercase;
		color: var(--text-muted);
	}
	.page-input {
		width: 74px;
		font-family: var(--font-mono);
		font-size: 13px;
		padding: 6px 8px;
		background: var(--panel-bg-alt);
		color: var(--text-primary);
		border: 1px solid var(--ink-line);
		outline: none;
	}
	.page-input:focus {
		border-color: var(--brass);
	}
	.page-bar-spacer { flex: 1; }
	.doc-frame {
		flex: 1;
		width: 100%;
		min-width: 0;
		min-height: 0;
		border: 1px solid var(--ink-line);
		background: #0a0d14;
	}
	.pdf-stage {
		flex: 1 1 0;
		height: 0;
		min-height: 0;
		overflow: auto;
		padding: 2px 0 12px;
		scrollbar-width: thin;
		scrollbar-color: var(--ink-line) transparent;
	}
	.pdf-stage::-webkit-scrollbar { width: 10px; height: 10px; }
	.pdf-stage::-webkit-scrollbar-thumb { background: var(--ink-line); }
	.pdf-pages {
		display: flex;
		flex-direction: column;
		gap: 18px;
		padding: 4px 0 8px;
	}
	.pdf-page {
		display: flex;
		flex-direction: column;
		gap: 6px;
	}
	.pdf-page-head {
		display: flex;
		align-items: center;
		gap: 8px;
		width: fit-content;
		margin: 0 auto;
		padding: 4px 10px;
		border: 1px solid var(--ink-line);
		background: var(--panel-bg);
	}
	.pdf-page-label {
		font-family: var(--font-mono);
		font-size: 10px;
		letter-spacing: 0.1em;
		text-transform: uppercase;
		color: var(--text-muted);
	}
	.pdf-page-num {
		font-family: var(--font-mono);
		font-size: 11px;
		color: var(--brass);
	}
	.pdf-canvas-shell {
		position: relative;
		width: fit-content;
		margin: 0 auto;
		border: 1px solid var(--ink-line);
		background: #0f1218;
	}
	.pdf-canvas {
		display: block;
	}
	.pdf-overlay {
		position: absolute;
		inset: 0;
		pointer-events: none;
	}
	:global(.pdf-highlight) {
		position: absolute;
		background: rgba(200, 85, 61, 0.18);
		border: 1px solid rgba(200, 85, 61, 0.9);
		box-shadow: inset 0 0 0 1px rgba(255, 210, 179, 0.22);
	}
	.pdf-status {
		font-family: var(--font-mono);
		font-size: 12px;
		color: var(--text-secondary);
		text-align: center;
		padding: 12px 0 6px;
	}
	.doc-foot-hint {
		font-size: 12px;
		line-height: 1.5;
		color: var(--text-secondary);
		padding: 8px 2px 0;
	}
	.inline-tab-btn {
		all: unset;
		cursor: pointer;
		color: var(--brass);
		font-family: var(--font-mono);
		font-size: 11px;
		text-transform: uppercase;
		letter-spacing: 0.06em;
		border-bottom: 1px dotted var(--brass);
	}
	.inline-tab-btn:hover {
		color: var(--text-primary);
		border-bottom-color: var(--text-primary);
	}
	.doc-error-link {
		color: var(--brass);
		text-decoration: underline;
	}

	.doc-status, .doc-error, .doc-empty {
		text-align: center;
		padding: 80px 20px;
		color: var(--text-muted);
	}
	.doc-status { font-family: var(--font-mono); font-size: 13px; }
	.dot-loop {
		display: inline-block;
		width: 8px; height: 8px;
		background: var(--brass);
		border-radius: 50%;
		margin-right: 8px;
		vertical-align: middle;
		animation: pulse 1.2s ease-in-out infinite;
	}
	@keyframes pulse {
		0%, 100% { opacity: 0.3; transform: scale(0.85); }
		50% { opacity: 1; transform: scale(1.1); }
	}
	.doc-error-title {
		font-family: var(--font-serif);
		font-size: 22px;
		color: var(--crimson);
		margin-bottom: 8px;
	}
	.doc-error-msg {
		font-family: var(--font-mono);
		font-size: 12px;
		color: var(--text-secondary);
	}
	.doc-empty-mark {
		font-size: 56px;
		color: var(--brass);
		opacity: 0.35;
		line-height: 1;
		margin-bottom: 16px;
	}
	.doc-empty-title {
		font-family: var(--font-serif);
		font-size: 26px;
		color: var(--text-secondary);
		margin-bottom: 8px;
	}
	.doc-empty-sub {
		font-size: 13px;
		line-height: 1.6;
	}

	/* Page rendering */
	.page {
		max-width: 820px;
		margin: 0 auto 36px;
		background: var(--panel-bg);
		border: 1px solid var(--ink-line);
		position: relative;
	}
	.page-edge {
		position: absolute;
		top: 0; bottom: 0; left: -6px;
		width: 6px;
		background: linear-gradient(180deg, var(--brass), var(--ink-line));
		opacity: 0.55;
	}
	.page-head {
		display: flex;
		align-items: center;
		gap: 10px;
		padding: 14px 28px;
		border-bottom: 1px solid var(--ink-line-soft);
		background: var(--panel-bg-alt);
	}
	.page-folio {
		font-family: var(--font-mono);
		font-size: 10px;
		letter-spacing: 0.16em;
		text-transform: uppercase;
		color: var(--text-muted);
	}
	.page-num {
		font-family: var(--font-serif);
		font-size: 22px;
		color: var(--brass);
		font-weight: 500;
		line-height: 1;
	}
	.page-rule { flex: 1; height: 1px; background: var(--ink-line); }
	.page-count {
		font-family: var(--font-mono);
		font-size: 10px;
		color: var(--text-muted);
		text-transform: uppercase;
		letter-spacing: 0.1em;
	}
	.page-body {
		padding: 20px 28px 28px;
		display: flex;
		flex-direction: column;
		gap: 4px;
	}

	.line {
		display: grid;
		grid-template-columns: 50px 90px 1fr;
		gap: 14px;
		align-items: baseline;
		padding: 6px 8px;
		border-left: 2px solid transparent;
		transition: background 200ms, border-color 200ms;
		font-family: var(--font-serif);
		font-size: 15px;
		line-height: 1.55;
		color: var(--text-primary);
	}
	.line:hover { background: var(--panel-bg-alt); }
	.line-no {
		font-family: var(--font-mono);
		font-size: 10px;
		color: var(--text-muted);
		text-align: right;
		letter-spacing: 0.04em;
	}
	.line-type {
		font-family: var(--font-mono);
		font-size: 9px;
		text-transform: uppercase;
		letter-spacing: 0.1em;
		color: var(--text-muted);
		padding-top: 3px;
	}
	.line-content { min-width: 0; word-break: break-word; }
	.line.line-heading .line-content {
		font-size: 19px;
		font-weight: 600;
		color: var(--text-primary);
	}
	.line.line-list .line-content::before {
		content: '— ';
		color: var(--brass);
	}
	.line.line-footer { opacity: 0.55; font-style: italic; }

	.line.highlight {
		background: var(--crimson-faint);
		border-left-color: var(--crimson);
		box-shadow: inset 0 0 0 1px rgba(200, 85, 61, 0.18);
		animation: hl-flash 700ms ease-out;
	}
	.line.highlight .line-no { color: var(--crimson); font-weight: 600; }
	.line.highlight .line-type { color: var(--crimson); }
	@keyframes hl-flash {
		0%   { background: rgba(200, 85, 61, 0.45); }
		100% { background: var(--crimson-faint); }
	}

	.spinner {
		width: 12px; height: 12px;
		border: 2px solid currentColor;
		border-right-color: transparent;
		border-radius: 50%;
		animation: spin 700ms linear infinite;
	}
	@keyframes spin { to { transform: rotate(360deg); } }

	/* ---------- Dialog ---------- */
	.dialog-overlay {
		position: fixed;
		inset: 0;
		background: rgba(8, 10, 14, 0.72);
		backdrop-filter: blur(3px);
		display: flex;
		align-items: center;
		justify-content: center;
		z-index: 50;
		padding: 24px;
	}
	.dialog {
		width: 100%;
		max-width: 880px;
		max-height: 86vh;
		display: flex;
		flex-direction: column;
		border: 1px solid;
		box-shadow: 0 30px 80px rgba(0, 0, 0, 0.55);
	}
	.dialog-head {
		padding: 24px 28px 16px;
		display: flex;
		justify-content: space-between;
		align-items: flex-start;
		border-bottom: 1px solid var(--ink-line-soft);
	}
	.dialog-eyebrow {
		font-family: var(--font-mono);
		font-size: 10px;
		letter-spacing: 0.16em;
		color: var(--brass);
		text-transform: uppercase;
		margin-bottom: 4px;
	}
	.dialog-title {
		font-family: var(--font-serif);
		font-size: 28px;
		font-weight: 500;
		margin: 0;
		color: var(--text-primary);
	}
	.dialog-close {
		all: unset;
		font-size: 28px;
		line-height: 1;
		color: var(--text-muted);
		cursor: pointer;
		padding: 4px 10px;
		transition: color 150ms;
	}
	.dialog-close:hover { color: var(--crimson); }

	.dialog-controls {
		display: flex;
		gap: 12px;
		align-items: flex-end;
		padding: 18px 28px;
	}
	.flex-1 { flex: 1; }
	.flex-2 { flex: 2; }

	.dialog-results {
		flex: 1;
		overflow-y: auto;
		border-top: 1px solid var(--ink-line-soft);
		border-bottom: 1px solid var(--ink-line-soft);
		min-height: 200px;
	}
	.dialog-empty {
		padding: 60px 20px;
		text-align: center;
		color: var(--text-muted);
	}
	.dialog-empty .empty-glyph { font-size: 48px; }
	.result-table {
		width: 100%;
		border-collapse: collapse;
		font-size: 13px;
	}
	.result-table thead th {
		font-family: var(--font-mono);
		font-size: 10px;
		text-transform: uppercase;
		letter-spacing: 0.1em;
		color: var(--text-muted);
		text-align: left;
		padding: 12px 16px;
		border-bottom: 1px solid var(--ink-line);
		background: var(--panel-bg-alt);
		position: sticky;
		top: 0;
	}
	.result-table tbody tr {
		border-bottom: 1px solid var(--ink-line-soft);
		cursor: pointer;
		transition: background 120ms;
	}
	.result-table tbody tr:hover { background: var(--panel-bg-alt); }
	.result-table tbody tr.selected { background: var(--brass-faint); }
	.result-table tbody tr.selected td { color: var(--brass); }
	.result-table td { padding: 11px 16px; color: var(--text-primary); }
	.result-table .mono { font-family: var(--font-mono); font-size: 12px; }
	.result-table .muted { color: var(--text-muted); }
	.result-table .ellipsis {
		max-width: 360px;
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
	}

	.dialog-foot {
		padding: 16px 28px;
		display: flex;
		justify-content: space-between;
		align-items: center;
	}
	.dialog-foot-hint {
		font-family: var(--font-mono);
		font-size: 11px;
		color: var(--text-muted);
	}
	.dialog-foot-buttons { display: flex; gap: 10px; }
</style>
