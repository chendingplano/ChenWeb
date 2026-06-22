<script lang="ts">
	import type { Snippet } from 'svelte';
	import { onMount, tick } from 'svelte';
	import { SvelteMap } from 'svelte/reactivity';

	type PdfWorker = { destroy: () => void };
	type PdfJsLib = {
		getDocument: (
			src: string | { url: string; withCredentials?: boolean; cMapUrl?: string; cMapPacked?: boolean; worker?: PdfWorker }
		) => { promise: Promise<unknown> };
		GlobalWorkerOptions?: { workerSrc: string };
		PDFWorker: new (params?: { name?: string }) => PdfWorker;
	};

	export type PdfPageViewport = {
		width: number;
		height: number;
	};

	type PdfPageProxy = {
		rotate?: number;
		getViewport: (params: { scale: number; rotation?: number }) => PdfPageViewport;
		render: (params: {
			canvasContext: CanvasRenderingContext2D;
			viewport: PdfPageViewport;
		}) => { promise: Promise<void>; cancel?: () => void };
	};

	type PdfDocumentProxy = {
		numPages: number;
		getPage: (n: number) => Promise<PdfPageProxy>;
		destroy?: () => void | Promise<void>;
	};

	let {
		inputId,
		fileUrl,
		page = $bindable(1),
		zoom = $bindable(0.5),
		numPages = $bindable(0),
		highlightVersion = 0,
		repaintVersion = 0,
		renderHighlights,
		loadingLabel = 'Rendering page…',
		respectPageRotation = true,
		onselect,
		ondragmove,
		pageBarTool,
		sidebarContent,
		sidebarResizer
	}: {
		inputId: number | null;
		fileUrl: string;
		page?: number;
		zoom?: number;
		numPages?: number;
		highlightVersion?: number | string;
		repaintVersion?: number | string;
		renderHighlights?: (pageNo: number, viewport: PdfPageViewport, overlay: HTMLDivElement) => void;
		loadingLabel?: string;
		respectPageRotation?: boolean;
		onselect?: (ranges: Array<{
			pageNumber: number;
			viewportY1: number;
			viewportY2: number;
			viewport: PdfPageViewport;
		}>) => void;
		ondragmove?: (ranges: Array<{
			pageNumber: number;
			viewportY1: number;
			viewportY2: number;
			viewport: PdfPageViewport;
		}>) => void;
		pageBarTool?: Snippet;
		sidebarContent?: Snippet;
		sidebarResizer?: Snippet;
	} = $props();

	const viewerId = `pdfv-${Math.random().toString(36).slice(2)}`;

	let pdfLib: PdfJsLib | null = null;
	let pdfWorker: PdfWorker | null = null;
	let pdfDoc: PdfDocumentProxy | null = null;
	let pdfLoadedInputId = 0;
	let pdfRenderSeq = 0;
	let pdfLoading = $state(false);
	let pdfError = $state('');
	let pdfRenderedPages = $state<number[]>([]);
	let pdfLastRenderWidth = 0;
	let pdfResizeRaf = 0;
	let pdfHighlightPaintSeq = 0;
	let pdfStageEl = $state<HTMLDivElement | null>(null);
	let pdfCanvasHostEl = $state<HTMLDivElement | null>(null);
	let pdfSidebarClusterEl = $state<HTMLDivElement | null>(null);
	let pdfSidebarWidth = $state(0);
	let pdfViewportByPage = new SvelteMap<number, PdfPageViewport>();
	let pdfActiveRenders = new SvelteMap<number, { cancel?: () => void }>();

	// ---------- Drag-select state ----------
	let dragSelecting = $state(false);
	let dragSelStartClientY = 0;
	let dragSelPage = 0;
	let indViewportTop = $state(0);
	let indHeight = $state(0);
	let indViewportLeft = $state(0);
	let indWidth = $state(0);

	// ---------- Drag-select auto-scroll ----------
	const DRAG_SCROLL_ZONE = 60;   // px from edge that triggers auto-scroll
	const DRAG_SCROLL_MAX_SPD = 14; // max px scrolled per animation frame
	let dragScrollRaf = 0;
	let dragScrollSpeed = 0;

	function stopDragScroll() {
		if (dragScrollRaf) {
			cancelAnimationFrame(dragScrollRaf);
			dragScrollRaf = 0;
		}
		dragScrollSpeed = 0;
	}

	function startDragScroll() {
		if (dragScrollRaf) return;
		function step() {
			if (!dragSelecting || !pdfCanvasHostEl || dragScrollSpeed === 0) {
				dragScrollRaf = 0;
				return;
			}
			pdfCanvasHostEl.scrollBy(0, dragScrollSpeed);
			dragScrollRaf = requestAnimationFrame(step);
		}
		dragScrollRaf = requestAnimationFrame(step);
	}

	// ---------- Drag-select handlers ----------
	function onDragPointerDown(e: PointerEvent) {
		if (!onselect || e.button !== 0) return;
		const host = pdfCanvasHostEl;
		if (!host) return;

		const pageEl = (e.target as HTMLElement).closest('[data-page]');
		if (!pageEl) return;
		const pageNo = parseInt(pageEl.getAttribute('data-page') ?? '0', 10);
		if (!pageNo) return;

		e.preventDefault();
		host.setPointerCapture(e.pointerId);

		const hostRect = host.getBoundingClientRect();
		dragSelStartClientY = e.clientY;
		dragSelPage = pageNo;
		indViewportTop = e.clientY;
		indHeight = 0;
		indViewportLeft = hostRect.left + 18;
		indWidth = hostRect.width - 36;
		dragSelecting = true;
	}

	function getPageRanges(clientY1: number, clientY2: number) {
		const ranges: Array<{
			pageNumber: number;
			viewportY1: number;
			viewportY2: number;
			viewport: PdfPageViewport;
		}> = [];
		for (const [pageNo, viewport] of pdfViewportByPage.entries()) {
			const canvasEl = document.getElementById(`${viewerId}-canvas-${pageNo}`) as HTMLCanvasElement | null;
			if (!canvasEl) continue;
			const rect = canvasEl.getBoundingClientRect();
			if (clientY2 < rect.top || clientY1 > rect.bottom) continue;
			const rawY1 = Math.max(clientY1, rect.top) - rect.top;
			const rawY2 = Math.min(clientY2, rect.bottom) - rect.top;
			const viewportY1 = Math.max(0, Math.min(rawY1, viewport.height));
			const viewportY2 = Math.max(0, Math.min(rawY2, viewport.height));
			ranges.push({ pageNumber: pageNo, viewportY1, viewportY2, viewport });
		}
		return ranges.sort((a, b) => a.pageNumber - b.pageNumber);
	}

	function onDragPointerMove(e: PointerEvent) {
		if (!dragSelecting) return;
		indViewportTop = Math.min(dragSelStartClientY, e.clientY);
		indHeight = Math.abs(e.clientY - dragSelStartClientY);

		// Auto-scroll when pointer is near the top or bottom edge of the scroll container
		if (pdfCanvasHostEl) {
			const rect = pdfCanvasHostEl.getBoundingClientRect();
			const distBottom = rect.bottom - e.clientY;
			const distTop = e.clientY - rect.top;
			if (distBottom < DRAG_SCROLL_ZONE && distBottom >= 0) {
				dragScrollSpeed = Math.round(DRAG_SCROLL_MAX_SPD * (1 - distBottom / DRAG_SCROLL_ZONE));
				startDragScroll();
			} else if (distTop < DRAG_SCROLL_ZONE && distTop >= 0) {
				dragScrollSpeed = -Math.round(DRAG_SCROLL_MAX_SPD * (1 - distTop / DRAG_SCROLL_ZONE));
				startDragScroll();
			} else {
				stopDragScroll();
			}
		}

		if (ondragmove) {
			const clientY1 = Math.min(dragSelStartClientY, e.clientY);
			const clientY2 = Math.max(dragSelStartClientY, e.clientY);
			const ranges = getPageRanges(clientY1, clientY2);
			if (ranges.length > 0) {
				ondragmove(ranges);
				for (const r of ranges) paintOverlayForPage(r.pageNumber);
			}
		}
	}

	function onDragPointerUp(e: PointerEvent) {
		if (!dragSelecting) return;
		dragSelecting = false;
		stopDragScroll();
		if (!onselect) return;

		const clientY1 = Math.min(dragSelStartClientY, e.clientY);
		const clientY2 = Math.max(dragSelStartClientY, e.clientY);
		if (clientY2 - clientY1 < 5) return;

		const ranges = getPageRanges(clientY1, clientY2);
		if (ranges.length > 0) onselect(ranges);
		paintHighlights();
	}

	function clampPage(nextPage: number): number {
		const max = Math.max(1, numPages || 1);
		if (!Number.isFinite(nextPage)) return 1;
		return Math.max(1, Math.min(Math.trunc(nextPage), max));
	}

	function goToPage(nextPage: number, behavior: ScrollBehavior = 'smooth') {
		page = clampPage(nextPage);
		scrollToPage(page, behavior);
	}

	function scrollToPage(pageNo: number, behavior: ScrollBehavior = 'smooth') {
		const pageEl = document.getElementById(`${viewerId}-page-${pageNo}`);
		const host = pdfCanvasHostEl;
		if (!pageEl || !host) return;
		const relativeTop = pageEl.getBoundingClientRect().top - host.getBoundingClientRect().top + host.scrollTop;
		host.scrollTo({ top: Math.max(0, relativeTop), behavior });
	}

	function zoomIn() {
		zoom = Math.min(3, Number((zoom + 0.1).toFixed(2)));
	}

	function zoomOut() {
		zoom = Math.max(0.1, Number((zoom - 0.1).toFixed(2)));
	}

	function zoomLabel(): string {
		return `${Math.round(zoom * 100)}%`;
	}

	function openPdfInNewTab() {
		window.open(fileUrl, '_blank', 'noopener');
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
		pdfWorker = new pdfLib.PDFWorker({ name: 'shared-pdf-viewer-worker' });
	}

	async function ensurePdfDoc() {
		if (!inputId || !fileUrl) return;
		if (pdfDoc && pdfLoadedInputId === inputId) return;

		for (const task of pdfActiveRenders.values()) task.cancel?.();
		pdfActiveRenders.clear();
		// Drop the old document without destroy() — calling destroy() terminates the
		// shared PDFWorker and kills any in-flight load for the incoming document.
		pdfDoc = null;
		pdfLoadedInputId = 0;
		pdfError = '';
		pdfViewportByPage.clear();

		await ensurePdfLib();
		if (!pdfLib || !pdfWorker) return;

		const task = pdfLib.getDocument({
			url: fileUrl,
			withCredentials: true,
			cMapUrl: '/pdfjs-cmaps/',
			cMapPacked: true,
			worker: pdfWorker
		});
		pdfDoc = (await task.promise) as PdfDocumentProxy;
		pdfLoadedInputId = inputId;
		numPages = Math.max(1, pdfDoc.numPages || 1);
		pdfRenderedPages = Array.from({ length: numPages }, (_, i) => i + 1);
		page = clampPage(page);
	}

	function paintHighlights() {
		for (const pageNo of pdfRenderedPages) {
			paintOverlayForPage(pageNo);
		}
	}

	function paintOverlayForPage(pageNo: number) {
		const overlay = document.getElementById(`${viewerId}-overlay-${pageNo}`) as HTMLDivElement | null;
		const viewport = pdfViewportByPage.get(pageNo);
		if (!overlay || !viewport) return;
		overlay.innerHTML = '';
		renderHighlights?.(pageNo, viewport, overlay);
	}

	function scrollToFirstHighlight(pageNo: number, behavior: ScrollBehavior = 'auto') {
		const overlay = document.getElementById(`${viewerId}-overlay-${pageNo}`) as HTMLDivElement | null;
		const firstHighlight = overlay?.querySelector('.pdf-highlight') as HTMLElement | null;
		const host = pdfCanvasHostEl;
		if (!firstHighlight || !host) return false;

		const hostRect = host.getBoundingClientRect();
		const highlightRect = firstHighlight.getBoundingClientRect();

		// Position of highlight's top edge relative to host's scroll origin
		const relativeTop = highlightRect.top - hostRect.top + host.scrollTop;
		const relativeBottom = relativeTop + highlightRect.height;

		// Already fully visible within the host's visible area — no scroll needed
		const visTop = host.scrollTop;
		const visBottom = visTop + host.clientHeight;
		if (relativeTop >= visTop && relativeBottom <= visBottom) return true;

		// Center the highlight within the host's visible area
		const targetScrollTop = relativeTop - host.clientHeight / 2 + highlightRect.height / 2;
		host.scrollTo({ top: Math.max(0, targetScrollTop), behavior });
		return true;
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
			const firstRotation = respectPageRotation ? (firstPage.rotate ?? 0) : 0;
			const baseViewport = firstPage.getViewport({ scale: 1, rotation: firstRotation });
			const availableWidth = Math.max(stageWidth - 40, 320);
			const baseScale = availableWidth / baseViewport.width;
			const targetScale = Math.max(0.1, baseScale * zoom);

			for (const pageNo of pdfRenderedPages) {
				if (seq !== pdfRenderSeq) return;
				const canvas = document.getElementById(`${viewerId}-canvas-${pageNo}`) as HTMLCanvasElement | null;
				const overlay = document.getElementById(`${viewerId}-overlay-${pageNo}`) as HTMLDivElement | null;
				if (!canvas || !overlay) continue;

				const pageProxy = await pdfDoc.getPage(pageNo);
				const rotation = respectPageRotation ? (pageProxy.rotate ?? 0) : 0;
				const viewport = pageProxy.getViewport({ scale: targetScale, rotation });
				const ctx = canvas.getContext('2d');
				if (!ctx) continue;

				canvas.width = Math.floor(viewport.width);
				canvas.height = Math.floor(viewport.height);
				canvas.style.width = `${viewport.width}px`;
				canvas.style.height = `${viewport.height}px`;
				overlay.style.width = `${viewport.width}px`;
				overlay.style.height = `${viewport.height}px`;
				pdfViewportByPage.set(pageNo, viewport);

				const prev = pdfActiveRenders.get(pageNo);
				prev?.cancel?.();

				ctx.setTransform(1, 0, 0, 1, 0, 0);
				ctx.clearRect(0, 0, viewport.width, viewport.height);
				const renderTask = pageProxy.render({ canvasContext: ctx, viewport });
				pdfActiveRenders.set(pageNo, renderTask);
				try {
					await renderTask.promise;
				} finally {
					if (pdfActiveRenders.get(pageNo) === renderTask) pdfActiveRenders.delete(pageNo);
				}
			}

			if (seq !== pdfRenderSeq) return;
			await tick();
			if (seq !== pdfRenderSeq) return;
			paintHighlights();
			if (!scrollToFirstHighlight(clampPage(page), 'auto')) {
				scrollToPage(clampPage(page), 'auto');
			}
		} catch (err) {
			if ((err as { name?: string })?.name === 'RenderingCancelledException') return;
			pdfError = err instanceof Error ? err.message : 'Failed to render PDF';
		} finally {
			if (seq === pdfRenderSeq) pdfLoading = false;
		}
	}

	$effect(() => {
		if (!inputId || !fileUrl || !pdfStageEl) return;
		const currentZoom = zoom;
		// NOTE: do NOT read pdfRenderedPages.length here — it would re-fire this
		// effect when the array is updated inside ensurePdfDoc() while inputId
		// is still the previous record, causing the "Worker was destroyed" loop.
		// renderPdfPages() is already called sequentially below, so no dependency needed.
		let cancelled = false;
		(async () => {
			try {
				await ensurePdfDoc();
				if (cancelled || currentZoom !== zoom) return;
				await tick();
				if (cancelled || currentZoom !== zoom) return;
				await renderPdfPages();
			} catch (err) {
				if ((err as { name?: string })?.name !== 'RenderingCancelledException') {
					pdfError = err instanceof Error ? err.message : 'Failed to render PDF';
				}
			}
		})();
		return () => {
			cancelled = true;
		};
	});

	$effect(() => {
		// Always read `page` synchronously so the effect tracks it as a dependency.
		// pdfDoc is a plain let (not $state), so reading it here would not be tracked;
		// guard it inside the async callback instead.
		const currentPage = page;
		void tick().then(() => {
			if (!pdfDoc || !pdfCanvasHostEl) return;
			paintHighlights();
			if (!scrollToFirstHighlight(clampPage(currentPage), 'auto')) {
				scrollToPage(clampPage(currentPage), 'auto');
			}
		});
	});

	$effect(() => {
		const version = highlightVersion;
		const seq = ++pdfHighlightPaintSeq;
		void tick().then(() => {
			if (seq !== pdfHighlightPaintSeq || highlightVersion !== version || pdfViewportByPage.size === 0) return;
			paintHighlights();
			if (!scrollToFirstHighlight(clampPage(page), 'auto')) {
				scrollToPage(clampPage(page), 'auto');
			}
		});
	});

	let pdfRepaintSeq = 0;
	$effect(() => {
		const version = repaintVersion;
		const seq = ++pdfRepaintSeq;
		void tick().then(() => {
			if (seq !== pdfRepaintSeq || repaintVersion !== version || pdfViewportByPage.size === 0) return;
			paintHighlights();
		});
	});

	$effect(() => {
		if (!pdfStageEl) return;
		const ro = new ResizeObserver(() => {
			const width = Math.floor(pdfStageEl?.clientWidth ?? 0);
			if (width <= 0 || width === pdfLastRenderWidth) return;
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

	$effect(() => {
		if (!pdfCanvasHostEl) return;
		const ro = new ResizeObserver(() => {
			const w = Math.floor(pdfCanvasHostEl?.clientWidth ?? 0);
			if (w <= 0 || w === pdfLastRenderWidth) return;
			if (pdfResizeRaf) cancelAnimationFrame(pdfResizeRaf);
			pdfResizeRaf = requestAnimationFrame(() => {
				void renderPdfPages();
			});
		});
		ro.observe(pdfCanvasHostEl);
		return () => ro.disconnect();
	});

	$effect(() => {
		if (!pdfSidebarClusterEl) return;
		const updateSidebarWidth = () => {
			pdfSidebarWidth = Math.ceil(pdfSidebarClusterEl?.getBoundingClientRect().width ?? 0);
		};
		updateSidebarWidth();
		const ro = new ResizeObserver(() => updateSidebarWidth());
		ro.observe(pdfSidebarClusterEl);
		return () => {
			ro.disconnect();
		};
	});

	onMount(() => {
		return () => {
			for (const task of pdfActiveRenders.values()) task.cancel?.();
			pdfActiveRenders.clear();
			if (pdfDoc?.destroy) void pdfDoc.destroy();
			pdfWorker?.destroy();
		};
	});
</script>

<div class="doc-page-bar">
	<div class="page-bar-sidebar-spacer" style={`width:${pdfSidebarWidth}px;`}></div>
	<div class="page-controls-wrap">
		<div class="page-controls">
		<button
			class="page-btn"
			onclick={() => goToPage(page - 1)}
			disabled={page <= 1}
			aria-label="Previous page">‹</button
		>
		<div class="page-bar-label">
			<span class="page-bar-folio">page</span>
			<input
				type="number"
				min="1"
				max={Math.max(1, numPages)}
				class="page-input"
				bind:value={page}
				onchange={() => goToPage(page)}
			/>
			<span class="page-total">/ {Math.max(1, numPages)}</span>
		</div>
		<button
			class="page-btn"
			onclick={() => goToPage(page + 1)}
			disabled={page >= Math.max(1, numPages)}
			aria-label="Next page">›</button
		>
		<button class="page-btn small" onclick={zoomOut} title="Zoom out" aria-label="Zoom out">−</button>
		<span class="zoom-label">{zoomLabel()}</span>
		<button class="page-btn small" onclick={zoomIn} title="Zoom in" aria-label="Zoom in">+</button>
		<button class="page-btn small" type="button" onclick={openPdfInNewTab} title="Open in new tab" aria-label="Open in new tab">↗</button>
		</div>
	</div>
	{@render pageBarTool?.()}
</div>

<div class="pdf-stage" bind:this={pdfStageEl}>
	<div class="pdf-layout">
		<div class="pdf-sidebar-cluster" bind:this={pdfSidebarClusterEl}>
			{@render sidebarContent?.()}
			{@render sidebarResizer?.()}
		</div>
		<div class="pdf-canvas-host" bind:this={pdfCanvasHostEl}
			onpointerdown={onDragPointerDown}
			onpointermove={onDragPointerMove}
			onpointerup={onDragPointerUp}
			onselectstart={(e) => e.preventDefault()}
		>
			<div class="pdf-pages">
				{#each pdfRenderedPages as pageNo (pageNo)}
					<div class="pdf-page" id={`${viewerId}-page-${pageNo}`} data-page={pageNo}>
						<div class="pdf-page-head">
							<span class="pdf-page-label">page</span>
							<span class="pdf-page-num">{String(pageNo).padStart(3, '0')}</span>
						</div>
						<div class="pdf-canvas-shell">
							<canvas class="pdf-canvas" id={`${viewerId}-canvas-${pageNo}`}></canvas>
							<div class="pdf-overlay" id={`${viewerId}-overlay-${pageNo}`}></div>
						</div>
					</div>
				{/each}
			</div>
		</div>
	</div>
	{#if dragSelecting}
		<div
			class="pdf-drag-indicator"
			style="top:{indViewportTop}px;left:{indViewportLeft}px;width:{indWidth}px;height:{indHeight}px;"
		></div>
	{/if}
	{#if pdfLoading}
		<div class="pdf-status"><span class="dot-loop"></span>{loadingLabel}</div>
	{/if}
	{#if pdfError}
		<div class="doc-error" style="padding-top:20px;">
			<div class="doc-error-title">⚠ Cannot render this PDF</div>
			<div class="doc-error-msg">
				{pdfError}<br />
				<button class="doc-error-link" type="button" onclick={openPdfInNewTab}>Open in a new tab</button>
			</div>
		</div>
	{/if}
</div>

<style>
	.doc-page-bar {
		display: flex;
		align-items: center;
		gap: 10px;
		padding: 10px 14px;
		border-bottom: 1px solid var(--ink-line-soft);
		background: var(--panel-bg);
	}
	.page-bar-sidebar-spacer {
		flex: 0 0 auto;
	}
	.page-controls-wrap {
		flex: 1;
		display: flex;
		justify-content: center;
		min-width: 0;
	}
	.page-controls {
		display: flex;
		align-items: center;
		gap: 10px;
	}
	.page-btn {
		height: 34px;
		min-width: 34px;
		border: 1px solid var(--ink-line);
		background: var(--panel-bg-alt);
		color: var(--text-primary);
		border-radius: 8px;
		cursor: pointer;
	}
	.page-btn.small {
		min-width: 30px;
		height: 30px;
	}
	.page-btn:disabled {
		opacity: 0.5;
		cursor: default;
	}
	.page-bar-label {
		display: flex;
		align-items: center;
		gap: 8px;
	}
	.page-bar-folio,
	.page-total,
	.zoom-label {
		font-family: var(--font-mono);
		font-size: 12px;
		color: var(--text-secondary);
	}
	.page-input {
		width: 68px;
		height: 34px;
		padding: 0 8px;
		border: 1px solid var(--ink-line);
		background: var(--panel-bg-alt);
		color: var(--text-primary);
		border-radius: 8px;
	}
	.page-bar-spacer {
		flex: 1;
	}
	.pdf-stage {
		position: relative;
		min-height: 0;
		flex: 1;
		overflow: hidden;
	}
	.pdf-layout {
		display: flex;
		height: 100%;
		min-height: 100%;
		align-items: stretch;
	}
	.pdf-sidebar-cluster {
		display: flex;
		align-items: stretch;
		align-self: stretch;
		flex: 0 0 auto;
		height: 100%;
		min-height: 0;
		max-height: 100%;
	}
	.pdf-canvas-host {
		flex: 1;
		min-width: 0;
		padding: 18px;
		height: 100%;
		overflow: auto;
		touch-action: none;
		user-select: none;
		-webkit-user-select: none;
		-webkit-tap-highlight-color: transparent;
		scrollbar-width: thin;
		scrollbar-color: rgba(148, 163, 184, 0.25) transparent;
	}
	.pdf-canvas-host::-webkit-scrollbar {
		width: 6px;
	}
	.pdf-canvas-host::-webkit-scrollbar-thumb {
		background: rgba(148, 163, 184, 0.25);
		border-radius: 999px;
	}
	.pdf-canvas-host::-webkit-scrollbar-track {
		background: transparent;
	}
	.pdf-pages {
		display: flex;
		flex-direction: column;
		gap: 24px;
		align-items: center;
	}
	.pdf-page {
		position: relative;
	}
	.pdf-page-head {
		display: flex;
		justify-content: center;
		gap: 8px;
		margin-bottom: 10px;
	}
	.pdf-page-label,
	.pdf-page-num {
		font-family: var(--font-mono);
		font-size: 11px;
		color: var(--text-secondary);
		text-transform: uppercase;
		letter-spacing: 0.08em;
	}
	.pdf-canvas-shell {
		position: relative;
	}
	.pdf-canvas {
		display: block;
		background: white;
		box-shadow: 0 18px 40px rgba(0, 0, 0, 0.24);
	}
	.pdf-overlay {
		position: absolute;
		left: 0;
		top: 0;
		pointer-events: none;
	}
	.pdf-drag-indicator {
		position: fixed;
		z-index: 10;
		pointer-events: none;
		background: transparent;
		border-left: 3px solid rgba(99, 102, 241, 0.5);
		border-right: 3px solid rgba(99, 102, 241, 0.5);
	}
	.pdf-status {
		position: sticky;
		left: 0;
		bottom: 14px;
		margin: 0 auto;
		width: fit-content;
		padding: 8px 12px;
		border-radius: 999px;
		background: rgba(15, 18, 24, 0.86);
		border: 1px solid var(--ink-line);
		color: var(--text-secondary);
		font-family: var(--font-mono);
		font-size: 12px;
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
	.doc-error {
		text-align: center;
		padding: 80px 20px;
		color: var(--text-muted);
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
	.doc-error-link {
		color: var(--brass);
		text-decoration: underline;
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
</style>
