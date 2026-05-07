<script lang="ts">
	import { onMount, tick } from 'svelte';
	import {
		listKbMetrics,
		getKbInput,
		updateKbMetric,
		updateKbInput,
		getRawLines,
		type KbInputRecord,
		type KbMetricRecord,
		type RawLine
	} from '$lib/services/kbService';
	import EditableMetadataSection from '$lib/components/home3/editable-metadata-section.svelte';
	import KbInputRecordBrowser from '$lib/components/home3/kb-input-record-browser.svelte';
	import PdfViewWindow from '$lib/components/home3/pdf-view-window.svelte';
	import {
		buildKbMetricMetadataRows,
		buildKbMetricUpdatePayloadForMetadataEdit
	} from './kb-metric-metadata.js';
	import {
		buildKbInputDocMetadataRows,
		buildKbInputRecordMetadataRows,
		buildKbInputUpdatePayloadForMetadataEdit
	} from './kb-input-metadata.js';

	let { darkMode = true }: { darkMode: boolean } = $props();

	// ---------- Aesthetic tokens: "archival reading room" ----------
	let pageBg = $derived(darkMode ? '#0E1116' : '#F5F1E8');
	let panelBg = $derived(darkMode ? '#161A22' : '#FBF8F0');
	let panelBgAlt = $derived(darkMode ? '#1C212C' : '#F0EADB');
	let inkLine = $derived(darkMode ? '#2A3140' : '#D7CFB8');
	let inkLineSoft = $derived(darkMode ? '#1F2530' : '#E5DEC8');
	let textPrimary = $derived(darkMode ? '#EDE7D3' : '#1A1410');
	let textSecondary = $derived(darkMode ? '#B5AE94' : '#5C5345');
	let textMuted = $derived(darkMode ? '#7C7560' : '#8F8472');
	let brass = $derived(darkMode ? '#D4A24C' : '#B8801E');
	let brassFaint = $derived(darkMode ? 'rgba(212,162,76,0.12)' : 'rgba(184,128,30,0.10)');
	let crimson = $derived(darkMode ? '#C8553D' : '#A23E26');
	let crimsonFaint = $derived(darkMode ? 'rgba(200,85,61,0.18)' : 'rgba(162,62,38,0.12)');
	let teal = $derived(darkMode ? '#5DAFA8' : '#2D7B73');

	const fontSerif = "'Cormorant Garamond', 'Playfair Display', Georgia, serif";
	const fontMono = "'JetBrains Mono', 'IBM Plex Mono', monospace";
	const fontSans = "'Inter Tight', system-ui, sans-serif";
	// ---------- State ----------
	let currentInput = $state<KbInputRecord | null>(null);
	let metrics = $state<KbMetricRecord[]>([]);
	let selectedMetricId = $state<number | null>(null);
	let highlightSelectionVersion = $state(0);
	let loading = $state(false);
	let errorMsg = $state('');
	let lastSelectedMetricDebug = $state('none');

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
	let isText = $derived(
		['text', 'markdown', 'json', 'xml', 'typst', 'html'].includes(
			(currentInput?.type ?? '').toLowerCase()
		)
	);

	// PDF.js rendering state
	type PdfJsLib = {
		getDocument: (
			src: string | { url: string; withCredentials?: boolean }
		) => { promise: Promise<unknown> };
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
		render: (params: {
			canvasContext: CanvasRenderingContext2D;
			viewport: PdfPageViewport;
		}) => { promise: Promise<void> };
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
	let pdfSettledRenderInputId = 0;
	let pdfLoading = $state(false);
	let pdfError = $state('');
	let pdfNumPages = $state(0);
	let pdfZoom = $state(0.5);
	let pdfRenderedPages = $state<number[]>([]);
	let pdfLastRenderWidth = 0;
	let pdfResizeRaf = 0;

	let pdfStageEl = $state<HTMLDivElement | null>(null);
	let pdfCanvasHostEl = $state<HTMLDivElement | null>(null);
	let pdfViewportByPage = new Map<number, PdfPageViewport>();

	type NormalizedSpan = { page_number: number; line_number: number };

	function toPositiveInt(v: unknown): number | null {
		const n = typeof v === 'string' ? Number(v.trim()) : Number(v);
		if (!Number.isFinite(n)) return null;
		const i = Math.trunc(n);
		return i > 0 ? i : null;
	}

	// Maps line_number → page_number, built from the loaded raw lines.
	let lineNumToPage = $derived.by(() => {
		const map = new Map<number, number>();
		for (const ln of rawLines) {
			if (!map.has(ln.line_number)) map.set(ln.line_number, ln.page_number);
		}
		return map;
	});

	// source_line_spans uses line-number spans only ("90", "98:99").
	// Page numbers are resolved via lineNumToPage.
	function normalizeMetricSpans(m: KbMetricRecord | undefined): NormalizedSpan[] {
		const raw = (m as { source_line_spans?: unknown })?.source_line_spans;
		if (!Array.isArray(raw)) return [];
		const lineNums: number[] = [];
		for (const item of raw) {
			if (typeof item === 'string') {
				const s = item.trim();
				const mm = s.match(/^(\d+)\s*[:,-]\s*(\d+)$/);
				if (mm) {
					const start = parseInt(mm[1], 10);
					const end = parseInt(mm[2], 10);
					for (let n = start; n <= end && n <= start + 200; n++) lineNums.push(n);
				} else {
					const n = parseInt(s, 10);
					if (n > 0) lineNums.push(n);
				}
			} else if (typeof item === 'number' && item > 0) {
				lineNums.push(Math.trunc(item));
			} else if (item && typeof item === 'object') {
				const obj = item as Record<string, unknown>;
				const l = toPositiveInt(obj.line_number ?? obj.line ?? obj.line_no ?? obj.lineNo);
				if (l) lineNums.push(l);
			}
		}
		const out: NormalizedSpan[] = [];
		for (const lineNo of lineNums) {
			const pageNo = lineNumToPage.get(lineNo);
			if (pageNo) out.push({ page_number: pageNo, line_number: lineNo });
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

	type MetadataLine = {
		span_key: string;
		line_number: number;
		line_type: string;
		content: string;
		coords_text: string;
		found: boolean;
	};

	let selectedMetric = $derived.by(() => {
		if (selectedMetricId == null) return null;
		return metrics.find((x) => x.id === selectedMetricId) ?? null;
	});

	function formatCoords(coords: unknown): string {
		if (!Array.isArray(coords) || coords.length < 4) return '';
		const values = coords
			.slice(0, 4)
			.map((n) => (typeof n === 'number' && Number.isFinite(n) ? String(Math.trunc(n)) : ''))
			.filter((v) => v.length > 0);
		return values.length === 4 ? `[${values.join(', ')}]` : '';
	}

	function metricSourceRecordId(metric: KbMetricRecord | null): number | null {
		if (!metric) return null;
		const sourceRecordId = toPositiveInt(
			(metric as KbMetricRecord & { source_record_id?: unknown }).source_record_id
		);
		if (sourceRecordId) return sourceRecordId;
		return toPositiveInt(metric.input_record_id);
	}

	let currentPageLines = $derived(rawLines.filter((ln) => ln.page_number === docPage));

	let selectedLineGroups = $derived.by(() => {
		const grouped = new Map<number, MetadataLine[]>();
		const metric = selectedMetric;
		if (!metric) return [] as Array<{ page: number; lines: MetadataLine[] }>;
		for (const span of normalizeMetricSpans(metric)) {
			const key = `${span.page_number}:${span.line_number}`;
			const ln = rawLineByKey.get(key);
			const arr = grouped.get(span.page_number) ?? [];
			arr.push({
				span_key: key,
				line_number: span.line_number,
				line_type: ln?.line_type ?? '',
				content: ln?.content ?? '',
				coords_text: formatCoords(ln?.coords),
				found: !!ln
			});
			grouped.set(span.page_number, arr);
		}
		return Array.from(grouped.entries())
			.sort((a, b) => a[0] - b[0])
			.map(([page, lines]) => ({
				page,
				lines: lines.sort((a, b) => a.line_number - b.line_number)
			}));
	});

	function renderMetricHighlights(
		pageNo: number,
		viewport: PdfPageViewport,
		overlay: HTMLDivElement
	) {
		const HIGHLIGHT_EXPAND_TOP_PX = 10;
		const HIGHLIGHT_EXPAND_RIGHT_PX = 20;
		const lines = selectedLinesByPage.get(pageNo) ?? [];
		const rects = lines.flatMap((ln) => {
			if (!Array.isArray(ln.coords) || ln.coords.length < 4) return [];
			const [vx1, vy1, vx2, vy2] = viewport.convertToViewportRectangle(ln.coords.slice(0, 4));
			return [
				{
					lineNumber: ln.line_number,
					left: Math.min(vx1, vx2),
					top: Math.max(0, Math.min(vy1, vy2) - HIGHLIGHT_EXPAND_TOP_PX),
					rawBottom: Math.max(vy1, vy2),
					width: Math.abs(vx2 - vx1) + HIGHLIGHT_EXPAND_RIGHT_PX
				}
			];
		});
		for (let i = 0; i < rects.length; i += 1) {
			const rect = rects[i];
			const nextRect = rects[i + 1];
			const bottom = nextRect ? nextRect.top : rect.rawBottom;
			const height = Math.max(0, bottom - rect.top);
			if (rect.width < 1 || height < 1) continue;
			const mark = document.createElement('div');
			mark.className = 'pdf-highlight';
			mark.style.left = `${rect.left}px`;
			mark.style.top = `${rect.top}px`;
			mark.style.width = `${rect.width}px`;
			mark.style.height = `${height}px`;
			mark.title = `line ${rect.lineNumber}`;
			overlay.appendChild(mark);
		}
	}

	function formatMaybeDate(value?: string): string {
		if (!value) return '—';
		return value.replace('T', ' ').slice(0, 19);
	}

	type MetadataEditorKind = 'text' | 'textarea' | 'datetime' | 'array' | 'json';
	type MetadataRow = {
		label: string;
		key: string;
		value: string;
		rawValue: unknown;
		editable: boolean;
		editor?: MetadataEditorKind;
		editKey?: string;
		wide?: boolean;
		pathLike?: boolean;
	};

	let metricFieldRows = $derived.by(() => {
		return buildKbMetricMetadataRows(selectedMetric);
	});

	let filteredPageLines = $derived.by(() => {
		const CONTEXT = 3;
		if (selectedMetricId == null) return currentPageLines;
		const highlightedOnPage: number[] = [];
		for (const k of highlightKeys) {
			const [pgStr, lnStr] = k.split(':');
			if (Number(pgStr) === docPage) highlightedOnPage.push(Number(lnStr));
		}
		if (highlightedOnPage.length === 0) return [] as typeof currentPageLines;
		const allLineNums = currentPageLines.map((l) => l.line_number);
		const include = new Set<number>();
		for (const hl of highlightedOnPage) {
			const idx = allLineNums.indexOf(hl);
			if (idx < 0) continue;
			for (
				let i = Math.max(0, idx - CONTEXT);
				i <= Math.min(allLineNums.length - 1, idx + CONTEXT);
				i++
			) {
				include.add(allLineNums[i]);
			}
		}
		return currentPageLines.filter((l) => include.has(l.line_number));
	});

	let inputRecordMetaRows = $derived.by(() => buildKbInputRecordMetadataRows(currentInput));
	let inputDocMetadataRows = $derived.by(() => buildKbInputDocMetadataRows(currentInput));

	async function saveInputMetadataRow(row: MetadataRow, draft: string, editor: MetadataEditorKind) {
		if (!currentInput) return;
		const payload = buildKbInputUpdatePayloadForMetadataEdit(currentInput, row, draft, editor);
		const updated = await updateKbInput(currentInput.id, payload);
		currentInput = updated.record;
	}

	async function saveMetricMetadataRow(row: MetadataRow, draft: string, editor: MetadataEditorKind) {
		if (!selectedMetric) return;
		const payload = buildKbMetricUpdatePayloadForMetadataEdit(selectedMetric, row, draft, editor);
		const updated = await updateKbMetric(selectedMetric.id, payload);
		metrics = metrics.map((metric) => (metric.id === updated.record.id ? updated.record : metric));
	}

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

	async function loadMetricsForRecord(id: number) {
		errorMsg = '';
		loading = true;
		metrics = [];
		selectedMetricId = null;
		highlightSelectionVersion = 0;
		rawLines = [];
		rawError = '';
		rawLoading = false;
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
			rawLoading = true;
			const [metricRes, inputRes, rawRes] = await Promise.all([
				listKbMetrics(id),
				getKbInput(id).catch(() => null),
				getRawLines(id).catch(() => null)
			]);
			metrics = metricRes.results ?? [];
			currentInput = inputRes?.record ?? null;
			rawLines = rawRes?.lines ?? [];
			rawError = rawRes ? '' : 'Failed to load raw lines';
		} catch (err) {
			errorMsg = err instanceof Error ? err.message : 'Failed to retrieve metrics';
		} finally {
			rawLoading = false;
			loading = false;
		}
	}

	async function selectMetric(m: KbMetricRecord) {
		lastSelectedMetricDebug = `${m.id} @ ${new Date().toLocaleTimeString()}`;
		selectedMetricId = m.id;
		highlightSelectionVersion += 1;
		const first = normalizeMetricSpans(m)[0];
		if (!first) return;

		// Move display to the selected page without forcing iframe remount/reload.
		console.log('metric selected, id:' + m.id + ", page_num:" + first.page_number);
		docPage = first.page_number > 0 ? first.page_number : 1;
		if (viewMode === 'document' && isPdf) {
			await tick();
			await renderSinglePdfPage(docPage);
			drawPdfHighlights();
			scrollPdfToPage(docPage, 'auto');
		}

		// If user is on the source-lines tab, scroll the highlighted line into view.
		if (viewMode === 'source') {
			await tick();
			const target = document.getElementById(`raw-${first.page_number}-${first.line_number}`);
			if (target) {
				target.scrollIntoView({ behavior: 'smooth', block: 'center' });
			} else {
				document
					.getElementById(`page-${first.page_number}`)
					?.scrollIntoView({ behavior: 'smooth', block: 'start' });
			}
		}
	}

	function onMetricCardClick(event: MouseEvent, m: KbMetricRecord) {
		// Defensive: avoid implicit form-submit or parent click handlers swallowing this action.
		event.preventDefault();
		event.stopPropagation();
		void selectMetric(m);
	}

	function setMode(mode: 'document' | 'source') {
		viewMode = mode;
	}

	function recordDisplayName(r: KbInputRecord): string {
		return r.title?.trim() || r.name?.trim() || r.file_name?.trim() || `Input #${r.id}`;
	}

	function recordDisplayDocNo(r: KbInputRecord): string {
		return r.doc_no?.trim() || '—';
	}

	function mapBrowserRecord(record: KbInputRecord) {
		return {
			id: record.id,
			title: recordDisplayName(record),
			subtitle: record.file_name?.trim() || record.name?.trim() || '—',
			meta: [recordDisplayDocNo(record), record.parser_name?.trim() || '—'],
			status: record.type?.trim() || '—',
			description: formatMaybeDate(record.create_time),
			badges: [record.type?.trim() || '—']
		};
	}

	function metricNameOf(m: KbMetricRecord): string {
		return m.metric_name?.trim() || m.metric_subject?.trim() || `Metric #${m.id}`;
	}
	function confidencePct(c?: number): string {
		if (c == null) return '—';
		return `${Math.round(c * 100)}%`;
	}
	function spanCount(m: KbMetricRecord): number {
		const raw = (m as { source_line_spans?: unknown })?.source_line_spans;
		if (!Array.isArray(raw)) return 0;
		let count = 0;
		for (const item of raw) {
			if (typeof item === 'string') {
				const mm = item.trim().match(/^(\d+)\s*[:,-]\s*(\d+)$/);
				count += mm ? Math.max(0, parseInt(mm[2], 10) - parseInt(mm[1], 10) + 1) : 1;
			} else {
				count += 1;
			}
		}
		return count;
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
		if (!pdfStageEl) return;
		const pageEl = document.getElementById(`pdf-page-${pageNo}`);
		if (!pageEl) return;
		const pageRect = pageEl.getBoundingClientRect();
		const stageRect = pdfStageEl.getBoundingClientRect();
		const targetTop = pdfStageEl.scrollTop + (pageRect.top - stageRect.top);
		pdfStageEl.scrollTo({ top: Math.max(0, targetTop), behavior });
	}

	function zoomIn() {
		pdfZoom = Math.min(3, Number((pdfZoom + 0.1).toFixed(2)));
	}
	function zoomOut() {
		pdfZoom = Math.max(0.1, Number((pdfZoom - 0.1).toFixed(2)));
	}
	function zoomLabel(): string {
		return `${Math.round(pdfZoom * 100)}%`;
	}

	async function ensurePdfLib() {
		if (pdfLib) return;
		const mod = (await import('pdfjs-dist')) as unknown as PdfJsLib;
		pdfLib = mod;
		if (pdfLib.GlobalWorkerOptions) {
			pdfLib.GlobalWorkerOptions.workerSrc = new URL(
				'pdfjs-dist/build/pdf.worker.mjs',
				import.meta.url
			).toString();
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
		const HIGHLIGHT_EXPAND_TOP_PX = 10;
		const HIGHLIGHT_EXPAND_RIGHT_PX = 20;
		for (const pageNo of pdfRenderedPages) {
			const overlay = document.getElementById(`pdf-overlay-${pageNo}`) as HTMLDivElement | null;
			const viewport = pdfViewportByPage.get(pageNo);
			if (!overlay || !viewport) continue;
			overlay.innerHTML = '';
			const lines = selectedLinesByPage.get(pageNo) ?? [];
			const rects = lines.flatMap((ln) => {
				if (!Array.isArray(ln.coords) || ln.coords.length < 4) return [];
				const [vx1, vy1, vx2, vy2] = viewport.convertToViewportRectangle(ln.coords.slice(0, 4));
				return [
					{
						lineNumber: ln.line_number,
						left: Math.min(vx1, vx2),
						top: Math.max(0, Math.min(vy1, vy2) - HIGHLIGHT_EXPAND_TOP_PX),
						rawBottom: Math.max(vy1, vy2),
						width: Math.abs(vx2 - vx1) + HIGHLIGHT_EXPAND_RIGHT_PX
					}
				];
			});
			for (let i = 0; i < rects.length; i += 1) {
				const rect = rects[i];
				const nextRect = rects[i + 1];
				const bottom = nextRect ? nextRect.top : rect.rawBottom;
				const height = Math.max(0, bottom - rect.top);
				if (rect.width < 1 || height < 1) continue;
				const mark = document.createElement('div');
				mark.className = 'pdf-highlight';
				mark.style.left = `${rect.left}px`;
				mark.style.top = `${rect.top}px`;
				mark.style.width = `${rect.width}px`;
				mark.style.height = `${height}px`;
				mark.title = `line ${rect.lineNumber}`;
				overlay.appendChild(mark);
			}
		}
	}

	async function renderPdfPages() {
		if (!pdfDoc || !pdfStageEl || pdfRenderedPages.length === 0) return;
		const stageWidth = Math.floor((pdfCanvasHostEl ?? pdfStageEl).clientWidth);
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

	async function renderSinglePdfPage(pageNo: number) {
		if (!pdfDoc || !pdfStageEl) return;
		const stageWidth = Math.floor((pdfCanvasHostEl ?? pdfStageEl).clientWidth);
		if (stageWidth <= 0) return;
		if (!Number.isFinite(pageNo) || pageNo <= 0) return;

		const firstPage = await pdfDoc.getPage(1);
		const firstPageRotation = firstPage.rotate ?? 0;
		const baseViewport = firstPage.getViewport({ scale: 1, rotation: firstPageRotation });
		const availableWidth = Math.max(stageWidth - 40, 320);
		const baseScale = availableWidth / baseViewport.width;
		const targetScale = Math.max(0.1, baseScale * pdfZoom);

		const canvas = document.getElementById(`pdf-canvas-${pageNo}`) as HTMLCanvasElement | null;
		const overlay = document.getElementById(`pdf-overlay-${pageNo}`) as HTMLDivElement | null;
		if (!canvas || !overlay) return;

		const page = await pdfDoc.getPage(pageNo);
		const pageRotation = page.rotate ?? 0;
		const viewport = page.getViewport({ scale: targetScale, rotation: pageRotation });
		const ctx = canvas.getContext('2d');
		if (!ctx) return;

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
			if (cancelled || !currentInput) return;

			// Initial PDF layout can settle one frame later; do a second pass
			// (equivalent to what users were doing via zoom +/- to correct page orientation).
			if (pdfSettledRenderInputId !== currentInput.id) {
				await new Promise<void>((resolve) => requestAnimationFrame(() => resolve()));
				if (cancelled) return;
				await renderPdfPages();
				pdfSettledRenderInputId = currentInput.id;
			}
		})();
		return () => {
			cancelled = true;
		};
	});

	$effect(() => {
		// Jump when target page changes (e.g. clicking a metric card, prev/next, page input).
		if (viewMode !== 'document' || !isPdf || !pdfDoc) return;
		docPage;
		void tick().then(() => scrollPdfToPage(clampDocPage(docPage), 'auto'));
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
			<div class="eyebrow">Knowledge System · Vol. III</div>
			<h1 class="display">Metrics&nbsp;<span class="amp">&amp;</span>&nbsp;Provenance</h1>
			<div class="subtitle">
				A reading room for extracted metrics — locate, verify, return to source.
			</div>
		</div>
		<div class="header-right">
			<span class="meta-label">RECORD</span><span class="meta-val">{currentInput?.id ?? '—'}</span>
			<span class="meta-label">TYPE</span><span class="meta-val">{currentInput?.type ?? '—'}</span>
			<span class="meta-label">METRICS</span><span class="meta-val"
				>{metrics.length.toString().padStart(3, '0')}</span
			>
		</div>
	</header>

	<div class="body">
		<KbInputRecordBrowser
			{darkMode}
			instanceKey="metrics-record-browser"
			title="kb.inputs"
			subtitle="Search, filter, and select input records before inspecting extracted metrics."
			emptyTitle="No records yet"
			emptySubtitle="Use Search or Retrieve to browse kb.inputs."
			selectedRecordId={currentInput?.id ?? null}
			mapRecord={mapBrowserRecord}
			onSelect={(record) => void loadMetricsForRecord(record.id)}
			onError={(error) => {
				errorMsg = error.message;
			}}
		/>

		<aside class="metric-sidebar">
			<div class="left-meta">
				<div class="left-meta-title">Metrics</div>
				<div class="left-meta-count">{metrics.length} found</div>
			</div>
			<div class="debug-badge" aria-live="polite">
				Debug: last selected metric = {lastSelectedMetricDebug}
			</div>

			<div class="metrics-list">
				{#if errorMsg}
					<div class="error">{errorMsg}</div>
				{:else if !loading && metrics.length === 0}
					<div class="empty">
						<div class="empty-glyph">§</div>
						<div class="empty-title">No metrics yet</div>
						<div class="empty-sub">Select a record from kb.inputs to populate the metrics index.</div>
					</div>
				{:else}
					{#each metrics as m, idx (m.id)}
						<button
							type="button"
							class="metric-card"
							class:selected={selectedMetricId === m.id}
							onclick={(event) => onMetricCardClick(event, m)}
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
						<span class="title-name" title={currentInput.file_name ?? ''}
							>{currentInput.file_name ?? '(unnamed file)'}</span
						>
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
						<span class="stat"
							><span class="stat-num">{String(docPage).padStart(3, '0')}</span><span
								class="stat-label">page</span
							></span
						>
						<span class="stat"
							><span class="stat-num">{highlightKeys.size}</span><span class="stat-label"
								>marks</span
							></span
						>
					{:else if pagesGrouped.length > 0}
						<span class="stat"
							><span class="stat-num">{pagesGrouped.length}</span><span class="stat-label"
								>pages</span
							></span
						>
						<span class="stat"
							><span class="stat-num">{rawLines.length}</span><span class="stat-label">lines</span
							></span
						>
						<span class="stat"
							><span class="stat-num">{highlightKeys.size}</span><span class="stat-label"
								>marks</span
							></span
						>
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
						{#if isPdf}
							<PdfViewWindow
								inputId={currentInput.id}
								{fileUrl}
								bind:page={docPage}
								bind:zoom={pdfZoom}
								bind:numPages={pdfNumPages}
								highlightVersion={`${selectedMetricId ?? 0}:${highlightSelectionVersion}`}
								renderHighlights={renderMetricHighlights}
								sidebarMinWidth={140}
								sidebarMaxWidth={620}
								sidebarDefaultWidth={270}
								sidebarTitle="Metadata"
								sidebarSettingsKey="metrics-pdf-sidebar"
								sidebarWidthSettingLabel="Metadata Panel Width"
							>
								{#snippet sidebar()}
									<EditableMetadataSection
										title="kb.metrics Record"
										rows={metricFieldRows}
										emptyText="Select a metric to inspect its fields."
										canEdit={true}
										onSave={saveMetricMetadataRow}
									/>

									<EditableMetadataSection
										title="kb.inputs Record"
										rows={inputRecordMetaRows}
										emptyText="No record loaded."
										canEdit={true}
										onSave={saveInputMetadataRow}
									/>

									<EditableMetadataSection
										title="kb.inputs Doc Metadata"
										rows={inputDocMetadataRows}
										emptyText="No doc_metadata available."
										canEdit={true}
										onSave={saveInputMetadataRow}
									/>

									<div class="metadata-section">
										<div class="metadata-section-title">Selected Lines (By Page)</div>
										{#if selectedLineGroups.length === 0}
											<div class="metadata-empty">No selected lines.</div>
										{:else}
											{#each selectedLineGroups as group (group.page)}
												<div class="metadata-page-group">
													<div class="metadata-page-title">Page {group.page}</div>
													<div class="metadata-lines">
														{#each group.lines as line (`${group.page}-${line.line_number}`)}
															<div class="metadata-line-row">
																<div class="metadata-line-head">
																	<span class="metadata-line-no">{line.span_key}</span>
																	{#if line.line_type}
																		<span class="metadata-line-type">{line.line_type}</span>
																	{/if}
																</div>
																<div class="metadata-line-content">
																	{line.content || (line.found ? '—' : '(line text unavailable)')}
																	{#if line.coords_text}
																		&nbsp;{line.coords_text}
																	{/if}
																</div>
															</div>
														{/each}
													</div>
												</div>
											{/each}
										{/if}
									</div>

									<div class="metadata-section">
										<div class="metadata-section-title">Lines — Page {docPage}</div>
										{#if rawLoading}
											<div class="metadata-empty">Loading…</div>
										{:else if filteredPageLines.length === 0 && selectedMetricId != null}
											<div class="metadata-empty">No selected lines on page {docPage}.</div>
										{:else if filteredPageLines.length === 0}
											<div class="metadata-empty">No lines for this page.</div>
										{:else}
											<div class="metadata-lines">
												{#each filteredPageLines as line (line.line_number)}
													<div
														class="metadata-line-row"
														class:hl={highlightKeys.has(`${docPage}:${line.line_number}`)}
													>
														<div class="metadata-line-head">
															<span class="metadata-line-no"
																>{String(line.line_number).padStart(4, '0')}</span
															>
															{#if line.line_type}
																<span class="metadata-line-type">{line.line_type}</span>
															{/if}
														</div>
														<div class="metadata-line-content">{line.content}</div>
													</div>
												{/each}
											</div>
										{/if}
									</div>
								{/snippet}
							</PdfViewWindow>
						{:else}
							<iframe
								class="doc-frame"
								title={currentInput.file_name ?? `Record ${currentInput.id}`}
								src={fileUrl}
							></iframe>
						{/if}

						{#if isText}
							<div class="doc-foot-hint">
								This file is rendered as text by your browser. For exact line highlighting, switch
								to the <button class="inline-tab-btn" onclick={() => setMode('source')}
									>Source&nbsp;Lines</button
								> tab.
							</div>
						{:else if !isPdf}
							<div class="doc-foot-hint">
								Inline preview support varies by file type. For line-level highlights, use the <button
									class="inline-tab-btn"
									onclick={() => setMode('source')}>Source&nbsp;Lines</button
								> tab.
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
											class:line-heading={ln.line_type?.includes('header') ||
												ln.line_type === 'title'}
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
		left: 40px;
		right: 40px;
		bottom: -1px;
		height: 1px;
		background: linear-gradient(
			90deg,
			var(--brass),
			transparent 35%,
			transparent 65%,
			var(--brass)
		);
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
	.display .amp {
		font-style: italic;
		color: var(--brass);
		font-weight: 400;
	}
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
		grid-template-columns: auto 360px minmax(0, 1fr);
		min-height: 0;
		min-width: 0;
		overflow: hidden;
	}

	@media (max-width: 1480px) {
		.body {
			grid-template-columns: auto 320px minmax(0, 1fr);
		}
	}

	/* ---------- METRIC SIDEBAR ---------- */
	.metric-sidebar {
		display: flex;
		flex-direction: column;
		border-right: 1px solid var(--ink-line);
		background: var(--panel-bg);
		min-width: 0;
		min-height: 0;
		overflow: hidden;
	}

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
	.debug-badge {
		margin: 0 24px 10px;
		padding: 6px 8px;
		font-family: var(--font-mono);
		font-size: 10px;
		color: var(--teal);
		background: rgba(93, 175, 168, 0.08);
		border: 1px dashed rgba(93, 175, 168, 0.4);
		word-break: break-word;
	}

	.metrics-list {
		flex: 1;
		min-height: 0;
		overflow-y: auto;
		padding: 0 16px 24px;
		display: flex;
		flex-direction: column;
		gap: 10px;
		scrollbar-width: thin;
		scrollbar-color: var(--ink-line) transparent;
	}
	.metrics-list::-webkit-scrollbar {
		width: 8px;
	}
	.metrics-list::-webkit-scrollbar-thumb {
		background: var(--ink-line);
	}

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
		transition:
			border-color 150ms,
			background 150ms;
	}
	.metric-card:hover {
		border-color: var(--brass);
		background: var(--panel-bg);
	}
	.metric-card.selected {
		border-color: var(--crimson);
		background: var(--panel-bg);
	}
	.card-rule {
		width: 4px;
		background: var(--ink-line);
		flex-shrink: 0;
		transition: background 150ms;
	}
	.metric-card:hover .card-rule {
		background: var(--brass);
	}
	.metric-card.selected .card-rule {
		background: var(--crimson);
	}
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
	.metric-card.selected .card-name {
		color: var(--crimson);
	}
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
		width: 4px;
		height: 4px;
		background: var(--brass);
		border-radius: 50%;
	}
	.chip-mono {
		color: var(--brass);
		border-color: var(--brass);
	}
	.chip-quiet {
		color: var(--text-muted);
	}

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
	.title-name.muted {
		color: var(--text-muted);
		font-style: italic;
	}
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
	.document::-webkit-scrollbar {
		width: 10px;
	}
	.document::-webkit-scrollbar-thumb {
		background: var(--ink-line);
	}

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
		transition:
			border-color 150ms,
			color 150ms,
			background 150ms;
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
	.page-bar-spacer {
		flex: 1;
	}
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
	.pdf-stage::-webkit-scrollbar {
		width: 10px;
		height: 10px;
	}
	.pdf-stage::-webkit-scrollbar-thumb {
		background: var(--ink-line);
	}
	.pdf-layout {
		display: grid;
		grid-template-columns: minmax(280px, 340px) 1fr;
		gap: 18px;
		align-items: start;
		padding: 4px 0 8px;
	}
	.metadata-panel {
		position: sticky;
		top: 0;
		align-self: start;
		width: 100%;
		background: var(--panel-bg);
		border: 1px solid var(--ink-line);
		padding: 12px;
		display: flex;
		flex-direction: column;
		gap: 12px;
		max-height: calc(100vh - 320px);
		overflow: auto;
		scrollbar-width: thin;
		scrollbar-color: var(--ink-line) transparent;
	}
	.metadata-title {
		font-family: var(--font-serif);
		font-size: 24px;
		color: var(--brass);
		line-height: 1;
	}
	.metadata-section {
		border: 1px solid var(--ink-line-soft);
		background: var(--panel-bg-alt);
		padding: 10px;
		display: flex;
		flex-direction: column;
		gap: 8px;
	}
	.metadata-section-title {
		font-family: var(--font-mono);
		font-size: 10px;
		text-transform: uppercase;
		letter-spacing: 0.1em;
		color: var(--text-muted);
	}
	.metadata-empty {
		font-size: 12px;
		color: var(--text-muted);
	}
	.metadata-page-group {
		display: flex;
		flex-direction: column;
		gap: 6px;
		padding-top: 4px;
		border-top: 1px dashed var(--ink-line-soft);
	}
	.metadata-page-title {
		font-family: var(--font-mono);
		font-size: 11px;
		color: var(--brass);
	}
	.metadata-lines {
		display: flex;
		flex-direction: column;
		gap: 6px;
	}
	.metadata-line-row {
		padding: 6px;
		background: var(--panel-bg);
		border: 1px solid var(--ink-line-soft);
	}
	.metadata-line-row.hl {
		background: color-mix(in srgb, var(--brass) 12%, var(--panel-bg));
		border-color: var(--brass);
	}
	.metadata-line-head {
		display: flex;
		align-items: center;
		gap: 6px;
		margin-bottom: 4px;
	}
	.metadata-line-no,
	.metadata-line-type {
		font-family: var(--font-mono);
		font-size: 10px;
		color: var(--text-muted);
	}
	.metadata-line-content {
		font-size: 12px;
		color: var(--text-secondary);
		line-height: 1.4;
	}
	.metadata-fields {
		display: flex;
		flex-direction: column;
		gap: 4px;
	}
	.metadata-row {
		display: grid;
		grid-template-columns: 130px 1fr;
		gap: 8px;
		align-items: start;
	}
	.metadata-key {
		font-family: var(--font-mono);
		font-size: 10px;
		text-transform: uppercase;
		letter-spacing: 0.08em;
		color: var(--text-muted);
	}
	.metadata-val {
		font-size: 12px;
		color: var(--text-primary);
		word-break: break-word;
	}
	.pdf-canvas-host {
		min-width: 0;
	}
	.pdf-pages {
		display: flex;
		flex-direction: column;
		gap: 18px;
		padding: 0 0 8px;
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

	.doc-status,
	.doc-error,
	.doc-empty {
		text-align: center;
		padding: 80px 20px;
		color: var(--text-muted);
	}
	.doc-status {
		font-family: var(--font-mono);
		font-size: 13px;
	}
	.dot-loop {
		display: inline-block;
		width: 8px;
		height: 8px;
		background: var(--brass);
		border-radius: 50%;
		margin-right: 8px;
		vertical-align: middle;
		animation: pulse 1.2s ease-in-out infinite;
	}
	@keyframes pulse {
		0%,
		100% {
			opacity: 0.3;
			transform: scale(0.85);
		}
		50% {
			opacity: 1;
			transform: scale(1.1);
		}
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
		top: 0;
		bottom: 0;
		left: -6px;
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
	.page-rule {
		flex: 1;
		height: 1px;
		background: var(--ink-line);
	}
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
		transition:
			background 200ms,
			border-color 200ms;
		font-family: var(--font-serif);
		font-size: 15px;
		line-height: 1.55;
		color: var(--text-primary);
	}
	.line:hover {
		background: var(--panel-bg-alt);
	}
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
	.line-content {
		min-width: 0;
		word-break: break-word;
	}
	.line.line-heading .line-content {
		font-size: 19px;
		font-weight: 600;
		color: var(--text-primary);
	}
	.line.line-list .line-content::before {
		content: '— ';
		color: var(--brass);
	}
	.line.line-footer {
		opacity: 0.55;
		font-style: italic;
	}

	.line.highlight {
		background: var(--crimson-faint);
		border-left-color: var(--crimson);
		box-shadow: inset 0 0 0 1px rgba(200, 85, 61, 0.18);
		animation: hl-flash 700ms ease-out;
	}
	.line.highlight .line-no {
		color: var(--crimson);
		font-weight: 600;
	}
	.line.highlight .line-type {
		color: var(--crimson);
	}
	@keyframes hl-flash {
		0% {
			background: rgba(200, 85, 61, 0.45);
		}
		100% {
			background: var(--crimson-faint);
		}
	}

	.spinner {
		width: 12px;
		height: 12px;
		border: 2px solid currentColor;
		border-right-color: transparent;
		border-radius: 50%;
		animation: spin 700ms linear infinite;
	}
	@keyframes spin {
		to {
			transform: rotate(360deg);
		}
	}

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
		max-width: 1210px;
		max-height: min(90vh, 980px);
		display: flex;
		flex-direction: column;
		border: 1px solid;
		border-radius: 24px;
		overflow: auto;
		background:
			linear-gradient(180deg, rgba(255, 255, 255, 0.03), transparent 22%),
			var(--panel-bg);
		box-shadow:
			0 30px 80px rgba(0, 0, 0, 0.55),
			0 0 0 1px rgba(212, 162, 76, 0.08);
		resize: both;
		min-width: 920px;
		min-height: 720px;
	}
	.dialog-scroll {
		flex: 1 1 auto;
		min-height: 0;
		overflow: auto;
		display: flex;
		flex-direction: column;
	}
	.dialog-head {
		padding: 24px 28px 16px;
		display: flex;
		justify-content: space-between;
		align-items: flex-start;
		border-bottom: 1px solid var(--ink-line-soft);
		background: linear-gradient(180deg, rgba(212, 162, 76, 0.09), rgba(212, 162, 76, 0));
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
	.dialog-subtitle {
		margin: 8px 0 0;
		max-width: 640px;
		font-size: 13px;
		line-height: 1.45;
		color: var(--text-secondary);
	}

	.dialog-controls {
		display: flex;
		flex-direction: column;
		gap: 12px;
		padding: 16px 28px 12px;
		background: linear-gradient(180deg, rgba(255, 255, 255, 0.025), rgba(0, 0, 0, 0.03));
		flex: 0 0 auto;
	}
	.dialog-section {
		border: 1px solid rgba(212, 162, 76, 0.16);
		border-radius: 20px;
		padding: 14px;
		background:
			linear-gradient(180deg, rgba(255, 255, 255, 0.04), rgba(255, 255, 255, 0.01)),
			#171c26;
	}
	.dialog-section-head {
		display: flex;
		justify-content: space-between;
		gap: 16px;
		margin-bottom: 10px;
		align-items: baseline;
	}
	.dialog-section-title {
		font-family: var(--font-mono);
		font-size: 11px;
		letter-spacing: 0.16em;
		text-transform: uppercase;
		color: var(--brass);
	}
	.dialog-section-copy {
		font-size: 12px;
		color: var(--text-secondary);
	}
	.dialog-grid {
		display: grid;
		grid-template-columns: repeat(3, minmax(0, 1fr));
		gap: 10px;
	}
	.dialog-grid-time {
		grid-template-columns: repeat(4, minmax(0, 1fr));
	}
	.dialog-grid-primary {
		grid-template-columns: 0.8fr 0.8fr 1.2fr 1fr 1.4fr;
	}
	.dialog-field {
		margin: 0;
		padding: 10px 10px 8px;
		border-radius: 16px;
		border: 1px solid rgba(255, 255, 255, 0.06);
		background: #1a202b;
		box-shadow: inset 0 1px 0 rgba(255, 255, 255, 0.02);
	}
	.dialog-field-wide {
		grid-column: span 2;
	}
	.dialog-field :global(input),
	.dialog-field :global(select) {
		background: #2a3140;
		border-color: rgba(255, 255, 255, 0.08);
		color: #f3eedf;
	}
	.dialog-field :global(input:focus),
	.dialog-field :global(select:focus) {
		border-color: #d4a24c;
		box-shadow:
			0 0 0 1px rgba(212, 162, 76, 0.28),
			0 0 0 4px rgba(212, 162, 76, 0.08);
	}
	.dialog-toolbar {
		display: flex;
		justify-content: space-between;
		align-items: center;
		gap: 12px;
		padding: 0 2px;
	}
	.dialog-toolbar-title {
		font-family: var(--font-mono);
		font-size: 11px;
		letter-spacing: 0.14em;
		text-transform: uppercase;
		color: var(--text-secondary);
		margin-bottom: 4px;
	}
	.dialog-toolbar-text {
		font-size: 13px;
		color: var(--text-secondary);
	}
	.dialog-toolbar-actions {
		display: flex;
		gap: 10px;
		align-items: center;
	}
	.dialog-search-btn {
		min-width: 140px;
		background: #d4a24c !important;
		color: #15110a !important;
		border: 1px solid #e0b768 !important;
		box-shadow: 0 8px 20px rgba(212, 162, 76, 0.22);
		opacity: 1 !important;
	}
	.dialog-search-btn:hover:not(:disabled) {
		background: #e0b768 !important;
		color: #15110a !important;
	}
	.dialog-search-btn:disabled {
		background: rgba(212, 162, 76, 0.35) !important;
		color: rgba(21, 17, 10, 0.7) !important;
	}
	.dialog-select-btn {
		min-width: 132px;
		background: #d4a24c !important;
		color: #15110a !important;
		border: 1px solid #e0b768 !important;
		opacity: 1 !important;
	}
	.dialog-select-btn:hover:not(:disabled) {
		background: #e0b768 !important;
		color: #15110a !important;
	}
	.dialog-select-btn:disabled {
		background: #4a4f5c !important;
		color: #aeb4c0 !important;
		border: 1px solid #636b79 !important;
		box-shadow: none !important;
		cursor: not-allowed !important;
		opacity: 1 !important;
	}
	.dialog-error {
		margin: 0 32px 16px;
	}

	.dialog-results {
		flex: 1 1 320px;
		border-top: 1px solid var(--ink-line-soft);
		border-bottom: 1px solid var(--ink-line-soft);
		min-height: 360px;
		background: #121720;
		box-shadow: inset 0 1px 0 rgba(255, 255, 255, 0.02);
	}
	.dialog-empty {
		padding: 72px 20px;
		text-align: center;
		color: var(--text-muted);
	}
	.dialog-empty .empty-glyph {
		font-size: 48px;
		color: var(--brass);
	}
	.dialog-empty-title {
		font-size: 18px;
		color: var(--text-primary);
		margin-top: 10px;
	}
	.dialog-empty-copy {
		max-width: 420px;
		margin: 10px auto 0;
		line-height: 1.6;
	}
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
		background: #181d27;
		position: sticky;
		top: 0;
		z-index: 1;
	}
	.result-table tbody tr {
		border-bottom: 1px solid var(--ink-line-soft);
		cursor: pointer;
		transition: background 120ms;
	}
	.result-table tbody tr:hover {
		background: #1d2330;
	}
	.result-table tbody tr.selected {
		background: rgba(212, 162, 76, 0.16);
	}
	.result-table tbody tr.selected td {
		color: #f4ddb0;
	}
	.result-table td {
		padding: 11px 16px;
		color: var(--text-primary);
		vertical-align: top;
	}
	.result-table .mono {
		font-family: var(--font-mono);
		font-size: 12px;
	}
	.result-table .muted {
		color: var(--text-muted);
	}
	.result-table .ellipsis {
		max-width: 360px;
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
	}
	.result-primary {
		font-weight: 600;
		color: var(--text-primary);
	}
	.result-secondary {
		margin-top: 4px;
		color: var(--text-muted);
	}
	.status-stack {
		display: flex;
		flex-direction: column;
		align-items: flex-start;
		gap: 6px;
	}
	.status-chip {
		padding: 2px 8px;
		border-radius: 999px;
		border: 1px solid rgba(255, 255, 255, 0.08);
		background: rgba(255, 255, 255, 0.04);
		color: #d7cfbb;
	}
	.status-pill {
		padding: 4px 10px;
		border-radius: 999px;
		font-size: 11px;
		font-weight: 700;
		letter-spacing: 0.08em;
		text-transform: uppercase;
		background: rgba(124, 117, 96, 0.16);
		color: var(--text-secondary);
	}
	.status-pill-success {
		background: rgba(93, 175, 168, 0.16);
		color: var(--teal);
	}
	.status-pill-fail {
		background: rgba(200, 85, 61, 0.18);
		color: var(--crimson);
	}

	.dialog-foot {
		padding: 14px 28px 18px;
		display: flex;
		justify-content: space-between;
		align-items: center;
		flex: 0 0 auto;
		background: #171c26;
	}
	.dialog-foot-hint {
		font-family: var(--font-mono);
		font-size: 11px;
		color: var(--text-muted);
	}
	.dialog-foot-buttons {
		display: flex;
		gap: 10px;
	}
	.dialog-scroll::-webkit-scrollbar,
	.dialog::-webkit-scrollbar {
		width: 12px;
		height: 12px;
	}
	.dialog-scroll::-webkit-scrollbar-thumb,
	.dialog::-webkit-scrollbar-thumb {
		background: rgba(212, 162, 76, 0.34);
		border-radius: 999px;
		border: 2px solid transparent;
		background-clip: padding-box;
	}
	.dialog-scroll::-webkit-scrollbar-track,
	.dialog::-webkit-scrollbar-track {
		background: rgba(255, 255, 255, 0.03);
	}

	@media (max-width: 1100px) {
		.dialog-grid,
		.dialog-grid-primary {
			grid-template-columns: repeat(2, minmax(0, 1fr));
		}
		.dialog-grid-time {
			grid-template-columns: repeat(2, minmax(0, 1fr));
		}
		.dialog-field-wide {
			grid-column: span 2;
		}
	}

	@media (max-width: 1440px) {
		.pdf-layout {
			grid-template-columns: 1fr;
		}
		.metadata-panel {
			position: static;
			max-height: none;
		}
	}
	@media (max-width: 820px) {
		.dialog-overlay {
			padding: 12px;
		}
		.dialog {
			max-height: 94vh;
			border-radius: 18px;
			min-width: 0;
			min-height: 0;
			resize: none;
		}
		.dialog-head,
		.dialog-controls,
		.dialog-foot {
			padding-left: 18px;
			padding-right: 18px;
		}
		.dialog-grid,
		.dialog-grid-primary {
			grid-template-columns: 1fr;
		}
		.dialog-grid-time {
			grid-template-columns: 1fr;
		}
		.dialog-field-wide {
			grid-column: auto;
		}
		.dialog-section-head,
		.dialog-toolbar,
		.dialog-foot {
			flex-direction: column;
			align-items: stretch;
		}
		.dialog-foot-buttons {
			justify-content: stretch;
		}
		.dialog-foot-buttons :global(button) {
			flex: 1;
		}
	}
</style>
