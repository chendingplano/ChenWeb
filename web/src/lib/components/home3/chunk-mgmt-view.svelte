<script lang="ts">
	import { onMount, tick } from 'svelte';
	import {
		getKbInput,
		listKbTopicChunks,
		type KbInputRecord,
		type KbTopicChunkRecord,
		type RawLine
	} from '$lib/services/kbService';

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

	let recordIdInput = $state('');
	let currentInput = $state<KbInputRecord | null>(null);
	let chunks = $state<KbTopicChunkRecord[]>([]);
	let selectedSeqNo = $state<number | null>(null);
	let loading = $state(false);
	let errorMsg = $state('');

	let docPage = $state(1);
	let pdfZoom = $state(1);
	let pdfLoading = $state(false);
	let pdfError = $state('');
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
		return `/api/v1/kb/inputs/${currentInput.id}/file#page=${docPage}&zoom=page-width`;
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

	async function doRetrieve() {
		errorMsg = '';
		const id = Number(recordIdInput.trim());
		if (!Number.isFinite(id) || id <= 0) {
			errorMsg = 'Enter a valid Record ID';
			return;
		}
		loading = true;
		chunks = [];
		selectedSeqNo = null;
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
				listKbTopicChunks(id),
				getKbInput(id).catch(() => null)
			]);
			chunks = chunkRes.results ?? [];
			currentInput = inputRes?.record ?? null;
			if (chunks.length > 0) {
				await selectChunk(chunks[0]);
			}
		} catch (err) {
			errorMsg = err instanceof Error ? err.message : 'Failed to retrieve topic chunks';
		} finally {
			loading = false;
		}
	}

	async function selectChunk(chunk: KbTopicChunkRecord) {
		selectedSeqNo = chunk.seqno;
		const first = (chunk.source_line_spans ?? [])[0];
		if (first && first.page_number > 0) {
			docPage = first.page_number;
		}
		if (isPdf) {
			await tick();
			await renderSinglePdfPage(docPage);
			drawPdfHighlights();
			scrollPdfToPage(docPage, 'auto');
		}
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
				const top = Math.max(0, Math.min(vy1, vy2) - 10);
				const width = Math.abs(vx2 - vx1) + 20;
				const height = Math.abs(vy2 - vy1) + 10;
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
		return () => {
			if (pdfDoc?.destroy) void pdfDoc.destroy();
		};
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
	"
>
	<header class="header">
		<div class="header-left">
			<div class="eyebrow">Knowledge Base · Vol. IV</div>
			<h1 class="display">Topic Chunks</h1>
			<div class="subtitle">Review semantic chunk topics and map each chunk back to source PDF regions.</div>
		</div>
	</header>

	<div class="body">
		<aside class="left">
			<div class="left-controls">
				<label class="field">
					<span class="field-label">Record ID</span>
					<div class="field-row">
						<input
							type="text"
							inputmode="numeric"
							bind:value={recordIdInput}
							placeholder="e.g. 1042"
							onkeydown={(e) => {
								if (e.key === 'Enter') doRetrieve();
							}}
						/>
					</div>
				</label>

				<button class="btn btn-primary retrieve" onclick={doRetrieve} disabled={loading}>
					{#if loading}
						<span class="spinner"></span>Retrieving…
					{:else}
						Retrieve
					{/if}
				</button>

				{#if errorMsg}<div class="error">{errorMsg}</div>{/if}
			</div>

			<div class="left-meta">
				<div class="left-meta-title">Chunks</div>
				<div class="left-meta-count">{chunks.length} found</div>
			</div>

			<div class="chunk-list">
				{#if !loading && chunks.length === 0}
					<div class="empty">
						<div class="empty-title">No chunks loaded</div>
						<div class="empty-sub">Enter a Record ID and retrieve `topics.txt` chunks.</div>
					</div>
				{:else}
					{#each chunks as chunk (chunk.seqno)}
						<button
							type="button"
							class="chunk-card"
							class:selected={selectedSeqNo === chunk.seqno}
							onclick={() => selectChunk(chunk)}
						>
							<div class="chunk-line1">
								<span class="seq">#{chunk.seqno}</span>
								<span class="type">{chunk.topic_type}</span>
							</div>
							<div class="chunk-topic">{chunk.topic}</div>
							<div class="chunk-keywords">{(chunk.keywords ?? []).join(', ') || '—'}</div>
							<div class="chunk-content">
								{#if (chunk.content_lines ?? []).length === 0}
									<div class="chunk-content-line muted">No source lines found.</div>
								{:else}
									{#each chunk.content_lines as ln (`${chunk.seqno}-${ln.page_number}-${ln.line_number}`)}
										<div class="chunk-content-line">
											<span class="ln-ref">P{ln.page_number}:L{ln.line_number}</span>
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
					<div class="doc-page-bar">
						<button class="page-btn" onclick={() => goToPage(docPage - 1)} disabled={docPage <= 1}>‹</button>
						<input
							type="number"
							min="1"
							max={Math.max(1, pdfNumPages)}
							class="page-input"
							bind:value={docPage}
							onchange={() => goToPage(docPage)}
						/>
						<span>/ {Math.max(1, pdfNumPages)}</span>
						<button
							class="page-btn"
							onclick={() => goToPage(docPage + 1)}
							disabled={docPage >= Math.max(1, pdfNumPages)}>›</button
						>
						<button class="page-btn" onclick={zoomOut}>−</button>
						<span>{zoomLabel()}</span>
						<button class="page-btn" onclick={zoomIn}>+</button>
						<a class="page-btn" href={fileUrl} target="_blank" rel="noopener">↗</a>
					</div>

					<div class="pdf-stage" bind:this={pdfStageEl}>
						<div class="pdf-layout">
							<aside class="meta-panel">
								<div class="meta-title">Chunk Bounding Boxes</div>
								{#if selectedBBoxes.length === 0}
									<div class="meta-empty">No coordinates available.</div>
								{:else}
									{#each selectedBBoxes as box (`${box.page_number}-${box.coords.join(',')}`)}
										<div class="bbox-row">
											<div class="bbox-page">Page {box.page_number}</div>
											<div class="bbox-coord">[{box.coords.map((n) => Math.trunc(n)).join(', ')}]</div>
										</div>
									{/each}
								{/if}
							</aside>

							<div class="pdf-canvas-col" bind:this={pdfCanvasHostEl}>
								{#if pdfLoading}<div class="pdf-status">Rendering PDF…</div>{/if}
								{#if pdfError}<div class="error">{pdfError}</div>{/if}
								{#each pdfRenderedPages as pageNo (pageNo)}
									<div class="pdf-page" id={`pdf-page-${pageNo}`}>
										<canvas id={`pdf-canvas-${pageNo}`}></canvas>
										<div class="pdf-overlay" id={`pdf-overlay-${pageNo}`}></div>
									</div>
								{/each}
							</div>
						</div>
					</div>
				{:else}
					<iframe class="doc-iframe" src={fileUrl} title="Document viewer"></iframe>
				{/if}
			{/if}
		</section>
	</div>
</div>

<style>
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
		padding: 18px 22px;
		border-bottom: 1px solid var(--ink-line);
		background: linear-gradient(180deg, var(--panel-bg), var(--panel-bg-alt));
	}
	.eyebrow { font-size: 12px; letter-spacing: 0.08em; text-transform: uppercase; color: var(--text-muted); }
	.display { font-family: var(--font-serif); font-size: 32px; line-height: 1.1; margin: 6px 0 2px; }
	.subtitle { color: var(--text-secondary); font-size: 14px; }
	.body {
		display: grid;
		grid-template-columns: 420px 1fr;
		min-height: 0;
	}
	.left {
		border-right: 1px solid var(--ink-line);
		background: var(--panel-bg);
		display: grid;
		grid-template-rows: auto auto 1fr;
		min-height: 0;
	}
	.left-controls { padding: 14px; border-bottom: 1px solid var(--ink-line-soft); }
	.field-label { display: block; font-size: 12px; color: var(--text-muted); margin-bottom: 8px; }
	.field-row { display: flex; gap: 8px; }
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
	.btn-primary { width: 100%; margin-top: 10px; background: var(--brass); color: #1d1508; }
	.btn:disabled { opacity: 0.6; cursor: default; }
	.left-meta {
		display: flex;
		justify-content: space-between;
		padding: 10px 14px;
		border-bottom: 1px solid var(--ink-line-soft);
		color: var(--text-secondary);
		font-size: 12px;
		text-transform: uppercase;
		letter-spacing: 0.08em;
	}
	.chunk-list { overflow: auto; padding: 10px; display: grid; gap: 10px; }
	.chunk-card {
		border: 1px solid var(--ink-line-soft);
		background: var(--panel-bg-alt);
		border-radius: 12px;
		padding: 10px;
		text-align: left;
		cursor: pointer;
		color: inherit;
	}
	.chunk-card.selected { border-color: var(--brass); box-shadow: 0 0 0 1px var(--brass-faint) inset; }
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
	.chunk-content { display: grid; gap: 3px; }
	.chunk-content-line { font-size: 12px; color: var(--text-secondary); display: grid; grid-template-columns: auto 1fr; gap: 6px; }
	.ln-ref { color: var(--text-muted); font-family: var(--font-mono); white-space: nowrap; }
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
	.doc-page-bar {
		display: flex;
		gap: 8px;
		align-items: center;
		padding: 10px 14px;
		border-bottom: 1px solid var(--ink-line-soft);
		background: var(--panel-bg-alt);
	}
	.page-btn {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		min-width: 30px;
		height: 30px;
		border: 1px solid var(--ink-line);
		border-radius: 8px;
		background: var(--panel-bg);
		text-decoration: none;
		color: var(--text-primary);
	}
	.page-input { width: 72px; height: 30px; }
	.pdf-stage { min-height: 0; overflow: auto; padding: 14px; }
	.pdf-layout { display: grid; grid-template-columns: 240px 1fr; gap: 14px; }
	.meta-panel {
		background: var(--panel-bg);
		border: 1px solid var(--ink-line-soft);
		border-radius: 12px;
		padding: 10px;
		height: fit-content;
	}
	.meta-title { font-weight: 700; margin-bottom: 8px; }
	.bbox-row { margin-bottom: 8px; border-bottom: 1px dashed var(--ink-line-soft); padding-bottom: 8px; }
	.bbox-page { font-size: 12px; color: var(--text-secondary); }
	.bbox-coord { font-family: var(--font-mono); font-size: 12px; }
	.pdf-canvas-col { display: grid; gap: 14px; }
	.pdf-page {
		position: relative;
		background: #fff;
		width: fit-content;
		margin: 0 auto;
		box-shadow: 0 10px 30px rgba(0, 0, 0, 0.2);
	}
	.pdf-overlay {
		position: absolute;
		inset: 0;
		pointer-events: none;
	}
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
	@media (max-width: 1200px) {
		.body { grid-template-columns: 1fr; }
		.left { max-height: 44vh; border-right: none; border-bottom: 1px solid var(--ink-line); }
		.pdf-layout { grid-template-columns: 1fr; }
	}
</style>
