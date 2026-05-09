<script lang="ts">
	import { browser } from '$app/environment';
	import { onMount, tick } from 'svelte';
	import SettingsIcon from '@lucide/svelte/icons/settings';
	import { appAuthStore } from '@chendingplano/shared';
	import {
		getKbInput,
		listKbChunks,
		type KbInputRecord,
		type KbTopicChunkRecord,
		type RawLine
	} from '$lib/services/kbService';
	import KbInputRecordBrowser from '$lib/components/home3/kb-input-record-browser.svelte';
	import PdfViewWindow from '$lib/components/home3/pdf-view-window.svelte';
	import {
		CHUNK_LIST_MAX_WIDTH,
		CHUNK_LIST_MIN_WIDTH,
		ZOOM_MIN,
		ZOOM_MAX,
		ZOOM_STEP,
		CHUNK_PANEL_DEFAULT_SETTINGS,
		clampChunkListWidth,
		readChunkPanelSettings,
		writeChunkPanelSettings
	} from './chunk-mgmt-settings.js';

	let { darkMode = true }: { darkMode: boolean } = $props();

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

	const fontSerif = "'Cormorant Garamond', 'Playfair Display', Georgia, serif";
	const fontMono = "'JetBrains Mono', 'IBM Plex Mono', monospace";
	const fontSans = "'Inter Tight', system-ui, sans-serif";
	type ChunkPanelSettings = typeof CHUNK_PANEL_DEFAULT_SETTINGS;

	let currentInput = $state<KbInputRecord | null>(null);
	let chunks = $state<KbTopicChunkRecord[]>([]);
	let selectedSeqNo = $state<number | null>(null);
	let highlightSelectionVersion = $state(0);
	let loading = $state(false);
	let errorMsg = $state('');
	let chunkListResizing = $state(false);
	let chunkListSettingsOpen = $state(false);
	let chunkListSettingsHydrated = $state(false);
	let chunkPanelSettings = $state<ChunkPanelSettings>({ ...CHUNK_PANEL_DEFAULT_SETTINGS });

	let docPage = $state(1);
	let pdfZoom = $state(CHUNK_PANEL_DEFAULT_SETTINGS.pdfZoom);
	let pdfLoading = $state(false);
	let pdfError = $state('');
	let iframeRevision = $state(0);
	let pdfNumPages = $state(0);
	let pdfRenderedPages = $state<number[]>([]);
	let pdfStageEl = $state<HTMLDivElement | null>(null);
	let pdfCanvasHostEl = $state<HTMLDivElement | null>(null);

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
	let pdfLastRenderWidth = 0;
	let pdfResizeRaf = 0;
	let pdfViewportByPage = new Map<number, PdfPageViewport>();

	let isPdf = $derived((currentInput?.type ?? '').toLowerCase() === 'pdf');
	let fileUrl = $derived.by(() => {
		if (!currentInput) return '';
		const rev = isPdf ? '' : `&_r=${iframeRevision}`;
		return `/api/v1/kb/inputs/${currentInput.id}/file#page=${docPage}&zoom=page-width${rev}`;
	});

	let selectedChunk = $derived.by(() => {
		if (selectedSeqNo == null) return null;
		return chunks.find((c) => c.seqno === selectedSeqNo) ?? null;
	});

	let selectedLinesByPage = $derived.by(() => {
		const map = new Map<number, RawLine[]>();
		if (!selectedChunk) return map;
		for (const line of selectedChunk.content_lines ?? []) {
			if (!Array.isArray(line.coords) || line.coords.length < 4) continue;
			const arr = map.get(line.page_number) ?? [];
			arr.push(line);
			map.set(line.page_number, arr);
		}
		for (const arr of map.values()) {
			arr.sort((a, b) => a.line_number - b.line_number);
		}
		return map;
	});

	let highlightCount = $derived.by(() => {
		let n = 0;
		for (const arr of selectedLinesByPage.values()) n += arr.length;
		return n;
	});

	let selectedBBoxes = $derived.by(() => selectedChunk?.bounding_boxes ?? []);
	let chunkListWidth = $derived(chunkPanelSettings.chunkListWidth);
	let chunkCardBackground = $derived(chunkPanelSettings.chunkCardBackground);
	let summaryBackground = $derived(chunkPanelSettings.summaryBackground);
	let contentBackground = $derived(chunkPanelSettings.contentBackground);

	function getChunkSettingsUserId(): string | null {
		return appAuthStore.getUser()?.id ?? null;
	}

	function applyChunkPanelSettings(next: Partial<ChunkPanelSettings>) {
		const updated = {
			...chunkPanelSettings,
			...next,
			chunkListWidth: clampChunkListWidth(next.chunkListWidth ?? chunkPanelSettings.chunkListWidth)
		};
		chunkPanelSettings = updated;
		if (typeof next.pdfZoom === 'number') pdfZoom = next.pdfZoom;
	}

	function openChunkSettings() {
		chunkListSettingsOpen = true;
	}

	function closeChunkSettings() {
		chunkListSettingsOpen = false;
	}

	function resetChunkSettings() {
		chunkPanelSettings = { ...CHUNK_PANEL_DEFAULT_SETTINGS };
	}

	function renderChunkHighlights(
		pageNo: number,
		viewport: PdfPageViewport,
		overlay: HTMLDivElement
	) {
		const HIGHLIGHT_EXPAND_TOP_PX = 10;
		const HIGHLIGHT_EXPAND_LEFT_PX = 5;
		const HIGHLIGHT_EXPAND_RIGHT_PX = 5;
		const lines = selectedLinesByPage.get(pageNo) ?? [];
		const rects = lines.flatMap((ln) => {
			if (!Array.isArray(ln.coords) || ln.coords.length < 4) return [];
			const [vx1, vy1, vx2, vy2] = viewport.convertToViewportRectangle(ln.coords.slice(0, 4));
			return [
				{
					lineNumber: ln.line_number,
					left: Math.max(0, Math.min(vx1, vx2) - HIGHLIGHT_EXPAND_LEFT_PX),
					right: Math.max(vx1, vx2) + HIGHLIGHT_EXPAND_RIGHT_PX,
					top: Math.max(0, Math.min(vy1, vy2) - HIGHLIGHT_EXPAND_TOP_PX),
					bottom: Math.max(vy1, vy2)
				}
			];
		});

		const mergedRects: Array<{
			firstLineNumber: number;
			lastLineNumber: number;
			left: number;
			right: number;
			top: number;
			bottom: number;
		}> = [];
		for (const rect of rects) {
			const prev = mergedRects[mergedRects.length - 1];
			const isAdjacent = !!prev && rect.lineNumber === prev.lastLineNumber + 1;
			if (isAdjacent && prev) {
				prev.lastLineNumber = rect.lineNumber;
				prev.left = Math.min(prev.left, rect.left);
				prev.right = Math.max(prev.right, rect.right);
				prev.bottom = Math.max(prev.bottom, rect.bottom);
				continue;
			}
			mergedRects.push({
				firstLineNumber: rect.lineNumber,
				lastLineNumber: rect.lineNumber,
				left: rect.left,
				right: rect.right,
				top: rect.top,
				bottom: rect.bottom
			});
		}

		for (const rect of mergedRects) {
			const width = Math.max(0, rect.right - rect.left);
			const height = Math.max(0, rect.bottom - rect.top);
			if (width < 1 || height < 1) continue;
			const mark = document.createElement('div');
			mark.className = 'pdf-highlight';
			mark.style.left = `${rect.left}px`;
			mark.style.top = `${rect.top}px`;
			mark.style.width = `${width}px`;
			mark.style.height = `${height}px`;
			mark.title =
				rect.firstLineNumber === rect.lastLineNumber
					? `line ${rect.firstLineNumber}`
					: `lines ${rect.firstLineNumber}-${rect.lastLineNumber}`;
			overlay.appendChild(mark);
		}
	}

	async function loadChunksForRecord(id: number) {
		errorMsg = '';
		loading = true;
		chunks = [];
		selectedSeqNo = null;
		highlightSelectionVersion = 0;
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
			const [chunkRes, inputRes] = await Promise.all([
				listKbChunks(id),
				getKbInput(id).catch(() => null)
			]);
			console.log('[chunk-mgmt] listKbChunks response', {
				recordId: id,
				total: chunkRes.total,
				chunks: (chunkRes.results ?? []).map((chunk) => ({
					seqno: chunk.seqno,
					line_tokens: chunk.line_tokens,
					source_line_spans: chunk.source_line_spans,
					content_line_count: chunk.content_lines?.length ?? 0
				}))
			});
			chunks = chunkRes.results ?? [];
			currentInput = inputRes?.record ?? null;
			if (chunks.length > 0) {
				await selectChunk(chunks[0]);
			}
		} catch (err) {
			errorMsg = err instanceof Error ? err.message : 'Failed to retrieve chunks';
		} finally {
			loading = false;
		}
	}

	async function selectChunk(chunk: KbTopicChunkRecord) {
		selectedSeqNo = chunk.seqno;
		highlightSelectionVersion += 1;
		const first = (chunk.source_line_spans ?? [])[0];
		if (first && first.page_number > 0) {
			docPage = first.page_number;
		}
		if (!isPdf) {
			iframeRevision += 1;
		}
	}

	function startChunkListResize(event: PointerEvent) {
		event.preventDefault();
		const handle = event.currentTarget as HTMLElement | null;
		const startX = event.clientX;
		const startWidth = chunkListWidth;
		chunkListResizing = true;
		document.body.style.cursor = 'col-resize';
		document.body.style.userSelect = 'none';
		handle?.setPointerCapture?.(event.pointerId);

		const handleMove = (moveEvent: PointerEvent) => {
			applyChunkPanelSettings({
				chunkListWidth: clampChunkListWidth(startWidth + (moveEvent.clientX - startX))
			});
		};
		const handleUp = (upEvent: PointerEvent) => {
			chunkListResizing = false;
			document.body.style.cursor = '';
			document.body.style.userSelect = '';
			handle?.releasePointerCapture?.(upEvent.pointerId);
			window.removeEventListener('pointermove', handleMove);
			window.removeEventListener('pointerup', handleUp);
			window.removeEventListener('pointercancel', handleUp);
		};

		window.addEventListener('pointermove', handleMove);
		window.addEventListener('pointerup', handleUp, { once: true });
		window.addEventListener('pointercancel', handleUp, { once: true });
	}

	function onChunkListResizerKeydown(event: KeyboardEvent) {
		if (event.key === 'ArrowLeft') {
			event.preventDefault();
			applyChunkPanelSettings({ chunkListWidth: clampChunkListWidth(chunkListWidth - 16) });
		} else if (event.key === 'ArrowRight') {
			event.preventDefault();
			applyChunkPanelSettings({ chunkListWidth: clampChunkListWidth(chunkListWidth + 16) });
		} else if (event.key === 'Home') {
			event.preventDefault();
			applyChunkPanelSettings({ chunkListWidth: CHUNK_LIST_MIN_WIDTH });
		} else if (event.key === 'End') {
			event.preventDefault();
			applyChunkPanelSettings({ chunkListWidth: CHUNK_LIST_MAX_WIDTH });
		}
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
			description: 'Select a record to inspect semantic chunks.',
			badges: [record.type?.trim() || '—']
		};
	}

	function formatSourceLineRanges(
		spans: Array<{ page_number: number; line_number: number }> | null | undefined
	): string[] {
		if (!spans || spans.length === 0) return [];

		const normalized = [...spans]
			.filter(
				(span) =>
					Number.isFinite(span?.page_number) &&
					Number.isFinite(span?.line_number) &&
					span.page_number > 0 &&
					span.line_number > 0
			)
			.sort((a, b) => {
				if (a.page_number !== b.page_number) return a.page_number - b.page_number;
				return a.line_number - b.line_number;
			});

		if (normalized.length === 0) return [];

		const out: string[] = [];
		let start = normalized[0];
		let end = normalized[0];

		for (let i = 1; i < normalized.length; i += 1) {
			const current = normalized[i];
			const isAdjacent =
				current.page_number === end.page_number && current.line_number === end.line_number + 1;
			const isDuplicate =
				current.page_number === end.page_number && current.line_number === end.line_number;
			if (isDuplicate) continue;
			if (isAdjacent) {
				end = current;
				continue;
			}
			out.push(
				start.line_number === end.line_number
					? `P${start.page_number}:${start.line_number}`
					: `P${start.page_number}:${start.line_number}-${end.line_number}`
			);
			start = current;
			end = current;
		}

		out.push(
			start.line_number === end.line_number
				? `P${start.page_number}:${start.line_number}`
				: `P${start.page_number}:${start.line_number}-${end.line_number}`
		);
		return out;
	}

	function clampDocPage(page: number): number {
		const n = Number.isFinite(page) ? Math.trunc(page) : 1;
		const max = Math.max(1, pdfNumPages || 1);
		return Math.max(1, Math.min(n || 1, max));
	}
	function goToPage(page: number, behavior: ScrollBehavior = 'smooth') {
		docPage = clampDocPage(page);
		if (isPdf) scrollPdfToPage(docPage, behavior);
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
		if (pdfDoc?.destroy) await pdfDoc.destroy();
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
		const HIGHLIGHT_EXPAND_LEFT_PX = 5;
		const HIGHLIGHT_EXPAND_RIGHT_PX = 5;
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
						left: Math.max(0, Math.min(vx1, vx2) - HIGHLIGHT_EXPAND_LEFT_PX),
						right: Math.max(vx1, vx2) + HIGHLIGHT_EXPAND_RIGHT_PX,
						top: Math.max(0, Math.min(vy1, vy2) - HIGHLIGHT_EXPAND_TOP_PX),
						bottom: Math.max(vy1, vy2)
					}
				];
			});

			const mergedRects: Array<{
				firstLineNumber: number;
				lastLineNumber: number;
				left: number;
				right: number;
				top: number;
				bottom: number;
			}> = [];
			for (const rect of rects) {
				const prev = mergedRects[mergedRects.length - 1];
				const isAdjacent = !!prev && rect.lineNumber === prev.lastLineNumber + 1;
				if (isAdjacent && prev) {
					prev.lastLineNumber = rect.lineNumber;
					prev.left = Math.min(prev.left, rect.left);
					prev.right = Math.max(prev.right, rect.right);
					prev.bottom = Math.max(prev.bottom, rect.bottom);
					continue;
				}
				mergedRects.push({
					firstLineNumber: rect.lineNumber,
					lastLineNumber: rect.lineNumber,
					left: rect.left,
					right: rect.right,
					top: rect.top,
					bottom: rect.bottom
				});
			}

			for (const rect of mergedRects) {
				const width = Math.max(0, rect.right - rect.left);
				const height = Math.max(0, rect.bottom - rect.top);
				if (width < 1 || height < 1) continue;
				const mark = document.createElement('div');
				mark.className = 'pdf-highlight';
				mark.style.left = `${rect.left}px`;
				mark.style.top = `${rect.top}px`;
				mark.style.width = `${width}px`;
				mark.style.height = `${height}px`;
				mark.title =
					rect.firstLineNumber === rect.lastLineNumber
						? `line ${rect.firstLineNumber}`
						: `lines ${rect.firstLineNumber}-${rect.lastLineNumber}`;
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
			if (seq === pdfRenderSeq) pdfLoading = false;
		}
	}

	async function renderSinglePdfPage(pageNo: number) {
		if (!pdfDoc || !pdfStageEl || !Number.isFinite(pageNo) || pageNo <= 0) return;
		const stageWidth = Math.floor((pdfCanvasHostEl ?? pdfStageEl).clientWidth);
		if (stageWidth <= 0) return;

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
		const canRenderPdf = isPdf && !!currentInput && !!pdfStageEl;
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
		if (!isPdf || !pdfDoc) return;
		docPage;
		void tick().then(() => scrollPdfToPage(clampDocPage(docPage), 'auto'));
	});

	$effect(() => {
		if (!isPdf || pdfViewportByPage.size === 0) return;
		selectedLinesByPage;
		drawPdfHighlights();
	});

	$effect(() => {
		if (!isPdf || !pdfStageEl) return;
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
		if (browser) {
			chunkPanelSettings = readChunkPanelSettings(localStorage, getChunkSettingsUserId());
			pdfZoom = chunkPanelSettings.pdfZoom;
			chunkListSettingsHydrated = true;
		}
		return () => {
			chunkListResizing = false;
			document.body.style.cursor = '';
			document.body.style.userSelect = '';
			if (pdfDoc?.destroy) void pdfDoc.destroy();
		};
	});

	$effect(() => {
		if (!browser || !chunkListSettingsHydrated) return;
		writeChunkPanelSettings(localStorage, getChunkSettingsUserId(), { ...chunkPanelSettings, pdfZoom });
	});
</script>

<div
	class="chunk-mgmt"
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
		--font-serif:{fontSerif};
		--font-mono:{fontMono};
		--font-sans:{fontSans};
		--chunk-card-bg:{chunkCardBackground};
		--chunk-summary-bg:{summaryBackground};
		--chunk-content-bg:{contentBackground};
	"
>
	<header class="header">
			<div class="header-left">
				<div class="eyebrow">Knowledge System· Vol. IV</div>
				<h1 class="display">Chunks</h1>
				<div class="subtitle">Review fixed-size chunks and map each chunk back to source PDF regions.</div>
			</div>
		<div class="header-actions">
			<button class="btn btn-ghost header-settings-btn" type="button" onclick={openChunkSettings}>
				<SettingsIcon size={15} />
				Settings
			</button>
		</div>
	</header>

	<div class="body" style={`grid-template-columns:auto ${chunkListWidth}px 14px 1fr;`}>
		<KbInputRecordBrowser
			{darkMode}
			instanceKey="chunks-record-browser"
			title="kb.inputs"
			subtitle="Search, filter, and select input records before inspecting chunks."
			emptyTitle="No records yet"
			emptySubtitle="Use Search or Retrieve to browse kb.inputs."
			selectedRecordId={currentInput?.id ?? null}
			mapRecord={mapBrowserRecord}
			onSelect={(record) => void loadChunksForRecord(record.id)}
			onError={(error) => {
				errorMsg = error.message;
			}}
		/>

		<aside class="chunk-sidebar">
			<div class="left-meta">
				<div class="left-meta-copy">
					<div class="left-meta-title">Chunks</div>
					<div class="left-meta-count">{chunks.length} found</div>
				</div>
			</div>

			<div class="chunk-list">
				{#if errorMsg}
					<div class="error">{errorMsg}</div>
				{:else if !loading && chunks.length === 0}
					<div class="empty">
						<div class="empty-title">No chunks loaded</div>
						<div class="empty-sub">Select a record to retrieve `.chunks` entries.</div>
					</div>
				{:else}
					{#each chunks as chunk (chunk.seqno)}
						<button
							type="button"
							class="chunk-card"
							class:selected={selectedSeqNo === chunk.seqno}
							onclick={() => selectChunk(chunk)}
						>
							<div class="chunk-summary">
								<div class="chunk-section-label">Summary</div>
								<div class="chunk-line1">
									<span class="seq">#{chunk.seqno}</span>
									<span class="type">{chunk.topic_type}</span>
								</div>
								<div class="chunk-topic">{chunk.topic}</div>
								<div class="chunk-keywords">{(chunk.keywords ?? []).join(', ') || '—'}</div>
							</div>
							<div class="chunk-content">
								<div class="chunk-section-label">Content</div>
								{#if (chunk.content_lines ?? []).length === 0}
									<div class="chunk-content-line muted">No source lines found.</div>
								{:else}
									{#each chunk.content_lines as ln (`${chunk.seqno}-${ln.page_number}-${ln.line_number}`)}
										<div class="chunk-content-line">
											<span>{ln.content}</span>
										</div>
									{/each}
								{/if}
							</div>
						</button>
					{/each}
				{/if}
			</div>
		</aside>
		<button
			type="button"
			class="chunk-list-resizer"
			class:active={chunkListResizing}
			aria-label="Resize chunks list"
			onpointerdown={startChunkListResize}
			onkeydown={onChunkListResizerKeydown}
		>
			<span class="chunk-list-resizer-grip" aria-hidden="true"></span>
		</button>

		<section class="right">
			<div class="right-toolbar">
				<div class="title">
					<span class="name">{currentInput?.file_name ?? 'No document loaded'}</span>
					{#if currentInput}<span class="type">{currentInput.type}</span>{/if}
				</div>
				<div class="stats">
					<span>{highlightCount} marks</span>
					<span>{selectedBBoxes.length} bbox{selectedBBoxes.length === 1 ? '' : 'es'}</span>
				</div>
			</div>

			{#if !currentInput}
				<div class="doc-empty">Retrieve a record to display document and chunk highlights.</div>
			{:else}
				{#if isPdf}
					<PdfViewWindow
						inputId={currentInput.id}
						{fileUrl}
						bind:page={docPage}
						bind:zoom={pdfZoom}
						bind:numPages={pdfNumPages}
						highlightVersion={`${selectedSeqNo ?? 0}:${highlightSelectionVersion}`}
						renderHighlights={renderChunkHighlights}
						sidebarMinWidth={140}
						sidebarMaxWidth={420}
						sidebarDefaultWidth={270}
						sidebarTitle="Chunk Details"
						sidebarSettingsKey="chunk-mgmt-pdf-sidebar"
						sidebarWidthSettingLabel="Panel Width"
					>
						{#snippet sidebar()}
							{#if selectedChunk}
								<div class="info-block">
									<div class="info-section">
										<div class="info-section-title">Chunk</div>
										<div class="info-row">
											<span class="info-label">#{selectedChunk.seqno}</span>
											<span class="info-type">{selectedChunk.topic_type}</span>
										</div>
										<div class="info-topic">{selectedChunk.topic}</div>
									</div>

									{#if (selectedChunk.keywords ?? []).length > 0}
										<div class="info-section">
											<div class="info-section-title">Keywords</div>
											<div class="info-keywords">
												{selectedChunk.keywords.join(', ')}
											</div>
										</div>
									{/if}

									{#if (selectedChunk.source_line_spans ?? []).length > 0}
										<div class="info-section">
											<div class="info-section-title">Source Lines</div>
											<div class="info-spans">
												{#each formatSourceLineRanges(selectedChunk.source_line_spans) as spanLabel (spanLabel)}
													<span class="info-span-chip">{spanLabel}</span>
												{/each}
											</div>
										</div>
									{/if}

									{#if selectedBBoxes.length > 0}
										<div class="info-section">
											<div class="info-section-title">Bounding Boxes ({selectedBBoxes.length})</div>
											<div class="info-bbox-list">
												{#each selectedBBoxes as box (`${box.page_number}-${box.coords.join(',')}`)}
													<div class="info-bbox-row">
														<span class="info-bbox-page">Page {box.page_number}</span>
														<span class="info-bbox-coord">[{box.coords.map((n) => Math.trunc(n)).join(', ')}]</span>
													</div>
												{/each}
											</div>
										</div>
									{/if}

									{#if (selectedChunk.content_lines ?? []).length > 0}
										<div class="info-section info-section-content">
											<div class="info-section-title">Content Lines ({selectedChunk.content_lines.length})</div>
											<div class="info-content-list">
												{#each selectedChunk.content_lines as ln (`${ln.page_number}-${ln.line_number}`)}
													<div class="info-content-item">
														<div class="info-content-meta">
															<span class="info-content-line">L{ln.line_number}</span>
															<span class="info-content-type">{ln.line_type}</span>
														</div>
														<div class="info-content-preview">{ln.content}</div>
													</div>
												{/each}
											</div>
										</div>
									{/if}
								</div>
							{:else}
								<div class="meta-empty">Select a chunk to view details.</div>
							{/if}
						{/snippet}
					</PdfViewWindow>
				{:else}
					<iframe class="doc-iframe" src={fileUrl} title="Document viewer"></iframe>
				{/if}
			{/if}
		</section>
	</div>
</div>

{#if chunkListSettingsOpen}
	<div
		class="dialog-overlay"
		aria-hidden="true"
		onclick={closeChunkSettings}
		onkeydown={(e) => {
			if (e.key === 'Escape') closeChunkSettings();
		}}
	>
		<div
			class="dialog settings-dialog"
			style="background:{panelBg}; border-color:{inkLine};"
			onclick={(e) => e.stopPropagation()}
			onkeydown={(e) => e.stopPropagation()}
			role="dialog"
			aria-modal="true"
			tabindex="0"
		>
			<div class="dialog-head">
				<div>
					<div class="dialog-eyebrow">Preferences</div>
					<h2 class="dialog-title">Chunk View Settings</h2>
					<p class="dialog-subtitle">
						Adjust the chunk list layout and colors. These preferences are saved for the current user.
					</p>
				</div>
			</div>

			<div class="dialog-scroll">
				<div class="dialog-controls">
					<div class="dialog-section">
						<div class="dialog-section-head">
							<div class="dialog-section-title">Appearance</div>
							<div class="dialog-section-copy">Set the saved defaults used by this chunk viewer.</div>
						</div>
						<div class="settings-grid">
							<label class="field dialog-field settings-field">
								<span class="field-label">Chunk background color</span>
								<div class="settings-color-row">
									<input
										class="settings-color-input"
										type="color"
										value={chunkPanelSettings.chunkCardBackground}
										oninput={(e) =>
											applyChunkPanelSettings({
												chunkCardBackground: (e.currentTarget as HTMLInputElement).value
											})}
									/>
								</div>
							</label>
							<label class="field dialog-field settings-field">
								<span class="field-label">Summary block background color</span>
								<div class="settings-color-row">
									<input
										class="settings-color-input"
										type="color"
										value={chunkPanelSettings.summaryBackground}
										oninput={(e) =>
											applyChunkPanelSettings({
												summaryBackground: (e.currentTarget as HTMLInputElement).value
											})}
									/>
								</div>
							</label>
							<label class="field dialog-field settings-field">
								<span class="field-label">Content block background color</span>
								<div class="settings-color-row">
									<input
										class="settings-color-input"
										type="color"
										value={chunkPanelSettings.contentBackground}
										oninput={(e) =>
											applyChunkPanelSettings({
												contentBackground: (e.currentTarget as HTMLInputElement).value
											})}
									/>
								</div>
							</label>
							<div class="field dialog-field settings-field settings-field-wide">
								<span class="field-label">CHUNKS list width ({chunkPanelSettings.chunkListWidth}px)</span>
								<div class="settings-width-row">
									<span class="settings-width-bound">{CHUNK_LIST_MIN_WIDTH}</span>
									<input
										type="range"
										min={CHUNK_LIST_MIN_WIDTH}
										max={CHUNK_LIST_MAX_WIDTH}
										step="4"
										value={chunkPanelSettings.chunkListWidth}
										oninput={(e) =>
											applyChunkPanelSettings({
												chunkListWidth: Number((e.currentTarget as HTMLInputElement).value)
											})}
										class="settings-width-slider"
									/>
									<span class="settings-width-bound">{CHUNK_LIST_MAX_WIDTH}</span>
								</div>
								<div class="dialog-section-copy" style="margin-top:6px;">
									You can also drag the divider beside the list to resize.
								</div>
							</div>
							<label class="field dialog-field settings-field settings-field-wide">
								<span class="field-label">Zoom percent ({Math.round(pdfZoom * 100)}%)</span>
								<div class="settings-width-row">
									<span class="settings-width-bound">{Math.round(ZOOM_MIN * 100)}%</span>
									<input
										type="range"
										min={ZOOM_MIN}
										max={ZOOM_MAX}
										step={ZOOM_STEP}
										value={pdfZoom}
										oninput={(e) => {
											const v = Number((e.currentTarget as HTMLInputElement).value);
											pdfZoom = v;
											applyChunkPanelSettings({ pdfZoom: v });
										}}
										class="settings-width-slider"
									/>
									<span class="settings-width-bound">{Math.round(ZOOM_MAX * 100)}%</span>
								</div>
							</label>
						</div>
					</div>
				</div>
			</div>

			<div class="dialog-foot">
				<div class="dialog-foot-hint">Updates are saved automatically.</div>
				<div class="dialog-foot-buttons">
					<button class="btn btn-primary dialog-select-btn" type="button" onclick={resetChunkSettings}>Reset</button>
					<button class="btn btn-primary dialog-select-btn" type="button" onclick={closeChunkSettings}>
						Close
					</button>
				</div>
			</div>
		</div>
	</div>
{/if}

<style>
	@import url('https://fonts.googleapis.com/css2?family=Cormorant+Garamond:ital,wght@0,400;0,500;0,600;1,400&family=JetBrains+Mono:wght@400;500;600&family=Inter+Tight:wght@400;500;600&display=swap');

	.chunk-mgmt {
		height: 100%;
		display: grid;
		grid-template-rows: auto 1fr;
		background: var(--page-bg);
		color: var(--text-primary);
		font-family: var(--font-sans);
	}
	.header {
		display: flex;
		justify-content: space-between;
		align-items: flex-start;
		padding: 18px 22px;
		border-bottom: 1px solid var(--ink-line);
		background: linear-gradient(180deg, var(--panel-bg), var(--panel-bg-alt));
	}
	.header-actions { display: flex; align-items: center; gap: 10px; }
	.header-settings-btn { white-space: nowrap; }
	.eyebrow { font-size: 12px; letter-spacing: 0.08em; text-transform: uppercase; color: var(--text-muted); }
	.display { font-family: var(--font-serif); font-size: 32px; line-height: 1.1; margin: 6px 0 2px; }
	.subtitle { color: var(--text-secondary); font-size: 14px; }
	.body {
		display: grid;
		min-height: 0;
	}
	.chunk-sidebar {
		border-right: 1px solid var(--ink-line);
		background: var(--panel-bg);
		display: grid;
		grid-template-rows: auto auto 1fr;
		min-height: 0;
	}
	input {
		width: 100%;
		height: 38px;
		padding: 0 10px;
		border: 1px solid var(--ink-line);
		background: var(--panel-bg-alt);
		color: var(--text-primary);
		border-radius: 10px;
	}
	.btn {
		height: 38px;
		border: none;
		border-radius: 10px;
		font-weight: 600;
		cursor: pointer;
	}
	.btn-ghost {
		padding: 0 12px;
		background: var(--panel-bg-alt);
		color: var(--text-primary);
		border: 1px solid var(--ink-line);
		display: inline-flex;
		align-items: center;
		gap: 6px;
	}
	.btn:disabled { opacity: 0.6; cursor: default; }
	.left-meta {
		display: flex;
		justify-content: space-between;
		align-items: center;
		gap: 12px;
		padding: 10px 14px;
		border-bottom: 1px solid var(--ink-line-soft);
	}
	.left-meta-copy { color: var(--text-secondary); font-size: 12px; text-transform: uppercase; letter-spacing: 0.08em; }
	.mono { font-family: var(--font-mono); }
	.chunk-list-resizer {
		display: flex;
		align-items: center;
		justify-content: center;
		width: 14px;
		padding: 0;
		border: 0;
		background: var(--page-bg);
		cursor: col-resize;
		user-select: none;
		touch-action: none;
		outline: none;
	}
	.chunk-list-resizer::before {
		content: '';
		width: 1px;
		height: 100%;
		background: var(--ink-line);
		opacity: 0.9;
		transition: background 150ms ease;
	}
	.chunk-list-resizer:hover::before,
	.chunk-list-resizer.active::before,
	.chunk-list-resizer:focus-visible::before {
		background: var(--brass);
	}
	.chunk-list-resizer-grip {
		position: absolute;
		width: 8px;
		height: 42px;
		border-radius: 999px;
		background:
			radial-gradient(circle, var(--text-muted) 22%, transparent 24%) center 6px / 6px 12px repeat-y,
			var(--panel-bg);
		border: 1px solid var(--ink-line-soft);
		box-shadow: 0 0 0 2px rgba(0, 0, 0, 0.14);
	}
	.chunk-list-resizer:hover .chunk-list-resizer-grip,
	.chunk-list-resizer.active .chunk-list-resizer-grip,
	.chunk-list-resizer:focus-visible .chunk-list-resizer-grip {
		border-color: var(--brass);
		background:
			radial-gradient(circle, var(--brass) 22%, transparent 24%) center 6px / 6px 12px repeat-y,
			var(--panel-bg);
	}
	.chunk-list { overflow: auto; padding: 10px; display: grid; gap: 10px; }
	.chunk-card {
		border: 1px solid var(--ink-line-soft);
		background: var(--chunk-card-bg);
		border-radius: 12px;
		padding: 10px;
		text-align: left;
		cursor: pointer;
		color: inherit;
		display: grid;
		gap: 8px;
	}
	.chunk-card.selected { border-color: var(--brass); box-shadow: 0 0 0 1px var(--brass-faint) inset; }
	.chunk-summary,
	.chunk-content {
		border-radius: 10px;
		padding: 10px;
	}
	.chunk-summary { background: var(--chunk-summary-bg); }
	.chunk-content { background: var(--chunk-content-bg); display: grid; gap: 6px; }
	.chunk-section-label {
		font-family: var(--font-mono);
		font-size: 10px;
		letter-spacing: 0.16em;
		text-transform: uppercase;
		color: var(--text-muted);
	}
	.chunk-line1 { display: flex; gap: 8px; align-items: center; margin-bottom: 4px; }
	.seq {
		font-family: var(--font-mono);
		font-size: 11px;
		padding: 2px 7px;
		border-radius: 999px;
		background: var(--brass-faint);
		color: var(--brass);
	}
	.type { font-size: 11px; text-transform: uppercase; color: var(--text-muted); letter-spacing: 0.06em; }
	.chunk-topic { font-weight: 700; margin-bottom: 4px; }
	.chunk-keywords { font-size: 12px; color: var(--text-secondary); margin-bottom: 8px; }
	.chunk-content-line { font-size: 12px; color: var(--text-secondary); line-height: 1.45; }
	.muted { color: var(--text-muted); }
	.right { min-width: 0; display: grid; grid-template-rows: auto 1fr; min-height: 0; }
	.right-toolbar {
		display: flex;
		justify-content: space-between;
		align-items: center;
		padding: 12px 14px;
		border-bottom: 1px solid var(--ink-line);
		background: var(--panel-bg);
	}
	.title { display: flex; align-items: center; gap: 10px; min-width: 0; }
	.name { font-weight: 700; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
	.stats { display: flex; gap: 14px; color: var(--text-secondary); font-size: 12px; }
	.doc-empty { padding: 20px; color: var(--text-secondary); }
	.meta-panel {
		position: sticky;
		top: 0;
		width: 100%;
		background: var(--panel-bg);
		border: 1px solid var(--ink-line-soft);
		border-radius: 12px;
		padding: 10px;
		height: fit-content;
	}
	.meta-title { font-weight: 700; margin-bottom: 8px; }
	.info-block { display: flex; flex-direction: column; gap: 2px; min-height: 100%; flex: 1 1 auto; }
	.info-section {
		padding: 8px 0;
		border-bottom: 1px solid var(--ink-line-soft);
	}
	.info-section-content {
		display: flex;
		flex: 1 1 auto;
		flex-direction: column;
		min-height: 0;
	}
	.info-section:last-child { border-bottom: none; }
	.info-section-title {
		font-family: var(--font-mono);
		font-size: 10px;
		text-transform: uppercase;
		letter-spacing: 0.1em;
		color: var(--text-muted);
		margin-bottom: 5px;
	}
	.info-row { display: flex; align-items: center; gap: 8px; margin-bottom: 3px; }
	.info-label { font-family: var(--font-mono); font-size: 13px; font-weight: 700; color: var(--brass); }
	.info-type {
		font-family: var(--font-mono);
		font-size: 10px;
		padding: 1px 6px;
		border-radius: 999px;
		background: rgba(212, 162, 76, 0.14);
		color: #e8ce8a;
	}
	.info-topic { font-size: 12px; line-height: 1.5; color: var(--text-primary); }
	.info-keywords { font-size: 12px; color: var(--text-secondary); line-height: 1.5; }
	.info-spans { display: flex; flex-wrap: wrap; gap: 4px; }
	.info-span-chip {
		font-family: var(--font-mono);
		font-size: 11px;
		padding: 2px 6px;
		border-radius: 4px;
		background: rgba(255, 255, 255, 0.05);
		color: var(--text-secondary);
	}
	.info-content-list {
		display: flex;
		flex: 1 1 auto;
		flex-direction: column;
		gap: 4px;
		min-height: 0;
		max-height: none;
		padding-bottom: 24px;
		overflow-y: auto;
	}
	.info-content-item {
		padding: 4px 6px;
		border-radius: 4px;
		background: rgba(255, 255, 255, 0.03);
	}
	.info-content-meta { display: flex; align-items: center; gap: 6px; margin-bottom: 2px; }
	.info-content-line { font-family: var(--font-mono); font-size: 11px; font-weight: 600; color: var(--brass); }
	.info-content-type {
		font-family: var(--font-mono);
		font-size: 10px;
		padding: 0 4px;
		border-radius: 3px;
		background: rgba(255, 255, 255, 0.06);
		color: var(--text-muted);
	}
	.info-content-preview { font-size: 11px; line-height: 1.4; color: var(--text-primary); overflow: hidden; text-overflow: ellipsis; display: -webkit-box; -webkit-line-clamp: 2; -webkit-box-orient: vertical; }
	.info-bbox-list { display: flex; flex-direction: column; gap: 4px; }
	.info-bbox-row { display: flex; gap: 8px; align-items: baseline; padding: 2px 0; }
	.info-bbox-page { font-size: 11px; color: var(--text-secondary); white-space: nowrap; }
	.info-bbox-coord { font-family: var(--font-mono); font-size: 11px; color: var(--text-muted); overflow: hidden; text-overflow: ellipsis; }
	:global(.pdf-highlight) {
		position: absolute;
		background: rgba(212, 162, 76, 0.28);
		outline: 1px solid rgba(212, 162, 76, 0.75);
		border-radius: 2px;
	}
	.error {
		margin-top: 10px;
		padding: 8px 10px;
		border-radius: 8px;
		font-size: 12px;
		background: rgba(200, 85, 61, 0.15);
		color: #f3b7ac;
	}
	.empty { padding: 18px 10px; text-align: center; color: var(--text-secondary); }
	.empty-title { font-weight: 700; margin-bottom: 4px; }
	.empty-sub { font-size: 12px; color: var(--text-muted); }
	.doc-iframe { width: 100%; height: 100%; border: none; }
	.spinner {
		display: inline-block;
		width: 14px;
		height: 14px;
		border: 2px solid rgba(0, 0, 0, 0.2);
		border-top-color: rgba(0, 0, 0, 0.7);
		border-radius: 999px;
		animation: spin 0.8s linear infinite;
		vertical-align: -2px;
		margin-right: 6px;
	}
	@keyframes spin {
		to { transform: rotate(360deg); }
	}
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
	.dialog-section-copy { font-size: 12px; color: var(--text-secondary); }
	.dialog-grid { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 10px; }
	.dialog-grid-time { grid-template-columns: repeat(4, minmax(0, 1fr)); }
	.dialog-grid-primary { grid-template-columns: 0.8fr 0.8fr 1.2fr 1fr 1.4fr; }
	.dialog-field {
		margin: 0;
		padding: 10px 10px 8px;
		border-radius: 16px;
		border: 1px solid rgba(255, 255, 255, 0.06);
		background: #1a202b;
		box-shadow: inset 0 1px 0 rgba(255, 255, 255, 0.02);
	}
	.dialog-field-wide { grid-column: span 2; }
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
	.dialog-toolbar-text { font-size: 13px; color: var(--text-secondary); }
	.dialog-toolbar-actions { display: flex; gap: 10px; align-items: center; }
	.dialog-search-btn {
		min-width: 140px;
		background: #d4a24c !important;
		color: #15110a !important;
		border: 1px solid #e0b768 !important;
		box-shadow: 0 8px 20px rgba(212, 162, 76, 0.22);
		opacity: 1 !important;
	}
	.dialog-search-btn:hover:not(:disabled) { background: #e0b768 !important; color: #15110a !important; }
	.dialog-search-btn:disabled { background: rgba(212, 162, 76, 0.35) !important; color: rgba(21, 17, 10, 0.7) !important; }
	.dialog-select-btn {
		min-width: 132px;
		background: #d4a24c !important;
		color: #15110a !important;
		border: 1px solid #e0b768 !important;
		opacity: 1 !important;
	}
	.dialog-select-btn:hover:not(:disabled) { background: #e0b768 !important; color: #15110a !important; }
	.dialog-select-btn:disabled {
		background: #4a4f5c !important;
		color: #aeb4c0 !important;
		border: 1px solid #636b79 !important;
		box-shadow: none !important;
		cursor: not-allowed !important;
		opacity: 1 !important;
	}
	.dialog-error { margin: 0 32px 16px; }
	.dialog-results {
		flex: 1 1 320px;
		border-top: 1px solid var(--ink-line-soft);
		border-bottom: 1px solid var(--ink-line-soft);
		min-height: 360px;
		background: #121720;
		box-shadow: inset 0 1px 0 rgba(255, 255, 255, 0.02);
	}
	.dialog-empty { padding: 72px 20px; text-align: center; color: var(--text-muted); }
	.dialog-empty .empty-glyph { font-size: 48px; color: var(--brass); }
	.dialog-empty-title { font-size: 18px; color: var(--text-primary); margin-top: 10px; }
	.dialog-empty-copy { max-width: 420px; margin: 10px auto 0; line-height: 1.6; }
	.result-table { width: 100%; border-collapse: collapse; font-size: 13px; }
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
	.result-table tbody tr { border-bottom: 1px solid var(--ink-line-soft); cursor: pointer; transition: background 120ms; }
	.result-table tbody tr:hover { background: #1d2330; }
	.result-table tbody tr.selected { background: rgba(212, 162, 76, 0.16); }
	.result-table tbody tr.selected td { color: #f4ddb0; }
	.result-table td { padding: 11px 16px; color: var(--text-primary); vertical-align: top; }
	.result-table .mono { font-family: var(--font-mono); font-size: 12px; }
	.result-table .muted { color: var(--text-muted); }
	.result-table .ellipsis { max-width: 360px; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
	.result-primary { font-weight: 600; color: var(--text-primary); }
	.result-secondary { margin-top: 4px; color: var(--text-muted); }
	.status-stack { display: flex; flex-direction: column; align-items: flex-start; gap: 6px; }
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
	.status-pill-success { background: rgba(93, 175, 168, 0.16); color: #5dafa8; }
	.status-pill-fail { background: rgba(200, 85, 61, 0.18); color: #c8553d; }
	.dialog-foot {
		padding: 14px 28px 18px;
		display: flex;
		justify-content: space-between;
		align-items: center;
		flex: 0 0 auto;
		background: #171c26;
	}
	.dialog-foot-hint { font-family: var(--font-mono); font-size: 11px; color: var(--text-muted); }
	.dialog-foot-buttons { display: flex; gap: 10px; }
	.dialog-scroll::-webkit-scrollbar,
	.dialog::-webkit-scrollbar { width: 12px; height: 12px; }
	.dialog-scroll::-webkit-scrollbar-thumb,
	.dialog::-webkit-scrollbar-thumb {
		background: rgba(212, 162, 76, 0.34);
		border-radius: 999px;
		border: 2px solid transparent;
		background-clip: padding-box;
	}
	.dialog-scroll::-webkit-scrollbar-track,
	.dialog::-webkit-scrollbar-track { background: rgba(255, 255, 255, 0.03); }
	.settings-dialog {
		max-width: 760px;
		min-width: 640px;
		min-height: 0;
		resize: none;
	}
	.settings-grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 12px; }
	.settings-field { background: #1a202b; }
	.settings-field-wide { grid-column: 1 / -1; }
	.settings-color-row { display: flex; align-items: center; gap: 12px; }
	.settings-color-input {
		width: 52px;
		min-width: 52px;
		padding: 0;
		border-radius: 10px;
		overflow: hidden;
		cursor: pointer;
	}
	.settings-width-row {
		display: flex;
		align-items: center;
		gap: 10px;
	}
	.settings-width-slider {
		flex: 1;
		min-width: 0;
		height: 6px;
		appearance: none;
		background: rgba(212, 162, 76, 0.18);
		border-radius: 999px;
		outline: none;
		cursor: pointer;
	}
	.settings-width-slider::-webkit-slider-thumb {
		appearance: none;
		width: 16px;
		height: 16px;
		border-radius: 50%;
		background: #e8ce8a;
		border: 2px solid rgba(0, 0, 0, 0.3);
		box-shadow: 0 1px 6px rgba(0, 0, 0, 0.35);
		cursor: pointer;
	}
	.settings-width-slider::-moz-range-thumb {
		width: 16px;
		height: 16px;
		border-radius: 50%;
		background: #e8ce8a;
		border: 2px solid rgba(0, 0, 0, 0.3);
		box-shadow: 0 1px 6px rgba(0, 0, 0, 0.35);
		cursor: pointer;
	}
	.settings-width-bound {
		font-family: var(--font-mono);
		font-size: 11px;
		color: var(--text-muted);
		min-width: 28px;
		text-align: center;
	}
	@media (max-width: 1200px) {
		.body { grid-template-columns: 1fr; }
		.chunk-sidebar { max-height: 44vh; border-right: none; border-bottom: 1px solid var(--ink-line); }
		.chunk-list-resizer { display: none; }
		.meta-panel { position: static; }
	}
	@media (max-width: 1100px) {
		.dialog-grid,
		.dialog-grid-primary { grid-template-columns: repeat(2, minmax(0, 1fr)); }
		.dialog-grid-time { grid-template-columns: repeat(2, minmax(0, 1fr)); }
		.dialog-field-wide { grid-column: span 2; }
	}
	@media (max-width: 820px) {
		.dialog-overlay { padding: 12px; }
		.dialog {
			max-height: 94vh;
			border-radius: 18px;
			min-width: 0;
			min-height: 0;
			resize: none;
		}
		.settings-dialog { min-width: 0; }
		.dialog-head,
		.dialog-controls,
		.dialog-foot { padding-left: 18px; padding-right: 18px; }
		.dialog-grid,
		.dialog-grid-primary,
		.dialog-grid-time,
		.settings-grid { grid-template-columns: 1fr; }
		.dialog-field-wide { grid-column: auto; }
		.dialog-section-head,
		.dialog-toolbar,
		.dialog-foot { flex-direction: column; align-items: flex-start; }
		.dialog-toolbar-actions,
		.dialog-foot-buttons { width: 100%; }
		.left-meta { align-items: flex-start; flex-direction: column; }
		.settings-color-row { flex-direction: column; align-items: stretch; }
		.dialog-search-btn,
		.dialog-select-btn,
		.dialog-foot-buttons .btn { width: 100%; }
	}
</style>
