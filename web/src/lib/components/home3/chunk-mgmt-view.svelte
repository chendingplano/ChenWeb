<script lang="ts">
	import { m } from '$lib/paraglide/messages.js';
	import { browser } from '$app/environment';
	import { onMount, tick } from 'svelte';
	import SettingsIcon from '@lucide/svelte/icons/settings';
	import { appAuthStore } from '@chendingplano/shared';
	import {
		getKbInput,
		getKbDocStructure,
		listKbChunks,
		type KbInputRecord,
		type KbTopicChunkRecord,
		type RawLine,
		type DocStructureLine
	} from '$lib/services/kbService';
	import KbInputRecordBrowser from '$lib/components/home3/kb-input-record-browser.svelte';
	import PdfViewWindow from '$lib/components/home3/pdf-view-window.svelte';
	import { KB_INPUT_RECORD_BROWSER_DEFAULT_HIGHLIGHT_COLOR } from './kb-input-record-browser-settings.js';
	import type { PdfPageViewport } from '$lib/components/home3/shared-pdf-viewer.svelte';
	import {
		CHUNK_LIST_MAX_WIDTH,
		CHUNK_LIST_MIN_WIDTH,
		ZOOM_MIN,
		ZOOM_MAX,
		ZOOM_STEP,
		CHUNK_PANEL_DEFAULT_SETTINGS,
		CHUNK_THEME_BACKGROUND,
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
	let contentSlab = $derived(darkMode ? '#131720' : '#FDFBF5');
	let crimson = $derived(darkMode ? '#C8553D' : '#A23E26');
	let errorBg = $derived(darkMode ? 'rgba(200,85,61,0.15)' : 'rgba(162,62,38,0.10)');
	let errorText = $derived(darkMode ? '#F3B7AC' : '#8C2F1A');
	let infoType = $derived(darkMode ? '#E8CE8A' : '#8A6A1F');
	let dialogSectionBg = $derived(darkMode ? '#171C26' : '#F7F2E6');
	let dialogFieldBg = $derived(darkMode ? '#1A202B' : '#FFFDF7');
	let dialogFieldLine = $derived(darkMode ? 'rgba(255,255,255,0.06)' : 'rgba(26,20,16,0.08)');
	let dialogInputBg = $derived(darkMode ? '#2A3140' : '#FFFFFF');
	let dialogInputLine = $derived(darkMode ? 'rgba(255,255,255,0.08)' : 'rgba(26,20,16,0.12)');
	let dialogInputText = $derived(darkMode ? '#F3EEDF' : '#1A1410');
	let btnDisabledBg = $derived(darkMode ? '#4A4F5C' : '#DED6C4');
	let btnDisabledText = $derived(darkMode ? '#AEB4C0' : '#8F8472');
	let btnDisabledLine = $derived(darkMode ? '#636B79' : '#CBC2AC');

	const fontSerif = "'Cormorant Garamond', 'Playfair Display', Georgia, serif";
	const fontMono = "'JetBrains Mono', 'IBM Plex Mono', monospace";
	const fontSans = "'Inter Tight', system-ui, sans-serif";
	type ChunkPanelSettings = typeof CHUNK_PANEL_DEFAULT_SETTINGS;

	let currentInput = $state<KbInputRecord | null>(null);
	let chunks = $state<KbTopicChunkRecord[]>([]);
	let lines = $state<DocStructureLine[]>([]);
	let linesError = $state('');
	let lineListEl = $state<HTMLDivElement | null>(null);
	let linesCollapsed = $state(false);
	let lineScrollPending = false;
	let focusedLine = $state<DocStructureLine | null>(null);
	let loadSequence = 0;
	let selectedSeqNo = $state<number | null>(null);
	let highlightSelectionVersion = $state(0);
	let loading = $state(false);
	let errorMsg = $state('');
	let chunkListResizing = $state(false);
	let chunkListSettingsOpen = $state(false);
	let recordHighlightColor = $state(KB_INPUT_RECORD_BROWSER_DEFAULT_HIGHLIGHT_COLOR);
	let chunkListSettingsHydrated = $state(false);
	let chunkPanelSettings = $state<ChunkPanelSettings>({ ...CHUNK_PANEL_DEFAULT_SETTINGS });

	let docPage = $state(1);
	let pdfZoom = $state(CHUNK_PANEL_DEFAULT_SETTINGS.pdfZoom);
	let iframeRevision = $state(0);
	let pdfNumPages = $state(0);

	let isPdf = $derived((currentInput?.type ?? '').toLowerCase() === 'pdf');
	let fileUrl = $derived.by(() => {
		if (!currentInput) return '';
		const url = `/api/v1/kb/inputs/${currentInput.id}/file`;
		return isPdf ? url : `${url}#page=${docPage}&zoom=page-width&_r=${iframeRevision}`;
	});

	let selectedChunk = $derived.by(() => {
		if (selectedSeqNo == null) return null;
		return chunks.find((c) => c.seqno === selectedSeqNo) ?? null;
	});

	let selectedLinesByPage = $derived.by(() => {
		const map = new Map<number, RawLine[]>();
		for (const line of focusedLine ? [focusedLine] : selectedChunk?.content_lines ?? []) {
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
	let chunkLineKeys = $derived(new Set((selectedChunk?.source_line_spans ?? []).map(line => `${line.page_number}:${line.line_number}`)));

	let highlightCount = $derived.by(() => {
		let n = 0;
		for (const arr of selectedLinesByPage.values()) n += arr.length;
		return n;
	});

	let selectedBBoxes = $derived.by(() => selectedChunk?.bounding_boxes ?? []);
	let chunkListWidth = $derived(chunkPanelSettings.chunkListWidth);
	let chunkCardBackground = $derived(chunkPanelSettings.chunkCardBackground);

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
		const HIGHLIGHT_EXPAND_TOP_PX = 2;
		const HIGHLIGHT_EXPAND_LEFT_PX = 5;
		const HIGHLIGHT_EXPAND_RIGHT_PX = 5;
		const lines = selectedLinesByPage.get(pageNo) ?? [];
		const rects = lines.flatMap((ln) => {
			if (!Array.isArray(ln.coords) || ln.coords.length < 4) return [];
			const vx1 = ln.coords[0] * viewport.width / 1000, vy1 = ln.coords[1] * viewport.height / 1000, vx2 = ln.coords[2] * viewport.width / 1000, vy2 = ln.coords[3] * viewport.height / 1000;
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
		const sequence = ++loadSequence;
		errorMsg = '';
		linesError = '';
		lines = [];
		focusedLine = null;
		loading = true;
		chunks = [];
		selectedSeqNo = null;
		highlightSelectionVersion = 0;
		currentInput = null;
		docPage = 1;
		pdfNumPages = 0;
		try {
			const [chunkRes, inputRes, structureRes] = await Promise.all([
				listKbChunks(id),
				getKbInput(id).catch(() => null),
				getKbDocStructure(id).then(value => ({ value, error: '' })).catch(() => ({ value: null, error: m.chunk_mgmt_lines_load_failed() }))
			]);
			if (sequence !== loadSequence) return;
			lines = structureRes.value?.lines ?? [];
			linesError = structureRes.error;
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
			if (sequence !== loadSequence) return;
			errorMsg = err instanceof Error ? err.message : m.chunk_mgmt_failed_to_retrieve_chunks();
		} finally {
			if (sequence === loadSequence) loading = false;
		}
	}

	async function selectChunk(chunk: KbTopicChunkRecord) {
		selectedSeqNo = chunk.seqno;
		focusedLine = null;
		highlightSelectionVersion += 1;
		const first = (chunk.source_line_spans ?? [])[0];
		if (first && first.page_number > 0) {
			docPage = first.page_number;
		}
		if (!isPdf) {
			iframeRevision += 1;
		}
		if (linesCollapsed) {
			lineScrollPending = true;
		} else {
			await tick();
			lineListEl?.querySelector<HTMLElement>('.line-card.chunk-line')?.scrollIntoView({ block: 'nearest' });
		}
	}

	function selectLine(line: DocStructureLine) {
		focusedLine = line;
		docPage = line.page_number;
		highlightSelectionVersion += 1;
		if (!isPdf) iframeRevision += 1;
	}

	async function toggleLines() {
		linesCollapsed = !linesCollapsed;
		if (!linesCollapsed && lineScrollPending) {
			lineScrollPending = false;
			await tick();
			lineListEl?.querySelector<HTMLElement>('.line-card.chunk-line')?.scrollIntoView({ block: 'nearest' });
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
		return r.title?.trim() || r.name?.trim() || r.file_name?.trim() || m.chunk_mgmt_input({ id: r.id });
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
			description: m.chunk_mgmt_select_a_record_to_inspect(),
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
		};
	});

	$effect(() => {
		if (!browser || !chunkListSettingsHydrated) return;
		writeChunkPanelSettings(localStorage, getChunkSettingsUserId(), { ...chunkPanelSettings, pdfZoom });
	});
</script>

<div
	class="chunk-mgmt select-text"
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
		--crimson:{crimson};
		--error-bg:{errorBg};
		--error-text:{errorText};
		--info-type:{infoType};
		--dialog-section-bg:{dialogSectionBg};
		--dialog-field-bg:{dialogFieldBg};
		--dialog-field-line:{dialogFieldLine};
		--dialog-input-bg:{dialogInputBg};
		--dialog-input-line:{dialogInputLine};
		--dialog-input-text:{dialogInputText};
		--btn-disabled-bg:{btnDisabledBg};
		--btn-disabled-text:{btnDisabledText};
		--btn-disabled-line:{btnDisabledLine};
		--content-slab:{contentSlab};
		--chunk-card-bg:{chunkCardBackground || (darkMode ? '#252f3e' : '#e5dbc6')};
		--record-card-bg:var(--chunk-card-bg);
		--chunk-selected-bg:{recordHighlightColor};
	"
>
	<header class="header">
			<div class="header-left">
				<div class="eyebrow">{m.chunk_mgmt_knowledge_system_vol_iv()}</div>
				<h1 class="display">{m.chunk_mgmt_chunks()}</h1>
				<div class="subtitle">{m.chunk_mgmt_review_fixed_size_chunks_and()}</div>
			</div>

	</header>

	<div class="body" style={`grid-template-columns:auto ${chunkListWidth}px 14px ${linesCollapsed ? '42px' : 'minmax(260px, 340px)'} minmax(300px, 1fr);`}>
		<KbInputRecordBrowser
			{darkMode}
			instanceKey="chunks-record-browser"
			title={m.chunk_mgmt_kb_inputs()}
			subtitle={m.chunk_mgmt_search_filter_and_select_input()}
			emptyTitle={m.chunk_mgmt_no_records_yet()}
			emptySubtitle={m.chunk_mgmt_use_search_or_retrieve_to()}
			selectedRecordId={currentInput?.id ?? null}
			mapRecord={mapBrowserRecord}
			onSelect={(record) => void loadChunksForRecord(record.id)}
			onHighlightColorChange={(color) => (recordHighlightColor = color)}
			onError={(error) => {
				errorMsg = error.message;
			}}
		/>

		<aside class="chunk-sidebar">
			<div class="left-meta">
				<div class="left-meta-copy">
					<div class="left-meta-title">{m.chunk_mgmt_chunks()}</div>
					<div class="left-meta-count">{m.chunk_mgmt_found({ chunksCount: chunks.length })}</div>
				</div>
				<button class="btn btn-ghost" type="button" onclick={openChunkSettings}>
					<SettingsIcon size={15} />
					{m.chunk_mgmt_settings()}
				</button>
			</div>

			<div class="chunk-list">
				{#if errorMsg}
					<div class="error">{errorMsg}</div>
				{:else if !loading && chunks.length === 0}
					<div class="empty">
						<div class="empty-title">{m.chunk_mgmt_no_chunks_loaded()}</div>
						<div class="empty-sub">{m.chunk_mgmt_select_a_record_to_retrieve()}</div>
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
								<div class="chunk-section-label">{m.chunk_mgmt_summary()}</div>
								<div class="chunk-line1">
									<span class="seq">#{chunk.seqno}</span>
									<span class="type">{chunk.topic_type}</span>
								</div>
								<div class="chunk-topic">{chunk.topic}</div>
								<div class="chunk-keywords">{(chunk.keywords ?? []).join(', ') || '—'}</div>
							</div>
							{#if (chunk.source_line_spans ?? []).length > 0}
								<div class="chunk-section-label">{m.chunk_mgmt_source_lines()}</div>
								<div class="info-spans">
									{#each formatSourceLineRanges(chunk.source_line_spans) as spanLabel (spanLabel)}
										<span class="info-span-chip">{spanLabel}</span>
									{/each}
								</div>
							{/if}
						</button>
					{/each}
				{/if}
			</div>
		</aside>
		<button
			type="button"
			class="chunk-list-resizer"
			class:active={chunkListResizing}
			aria-label={m.chunk_mgmt_resize_chunks_list()}
			onpointerdown={startChunkListResize}
			onkeydown={onChunkListResizerKeydown}
		>
			<span class="chunk-list-resizer-grip" aria-hidden="true"></span>
		</button>
		<aside class="lines-sidebar" class:collapsed={linesCollapsed}>
			<div class="left-meta lines-header">
				{#if !linesCollapsed}<div class="left-meta-copy"><div class="left-meta-title">{m.doc_structure_lines()}</div><div class="left-meta-count">{lines.length}</div></div>{/if}
				<button type="button" class="lines-toggle" onclick={toggleLines} aria-expanded={!linesCollapsed} aria-controls="chunk-lines-list" aria-label={linesCollapsed ? m.chunk_mgmt_expand_lines() : m.chunk_mgmt_collapse_lines()} title={linesCollapsed ? m.chunk_mgmt_expand_lines() : m.chunk_mgmt_collapse_lines()}>{linesCollapsed ? '›' : '‹'}</button>
			</div>
			<div id="chunk-lines-list" class="lines-list" bind:this={lineListEl} hidden={linesCollapsed}>
				{#if linesError}<div class="error">{linesError}</div>
				{:else if !loading && lines.length === 0}<div class="empty">{m.doc_structure_no_lines_loaded()}</div>
				{:else}
					{#each lines as line, index (index)}
						<button type="button" class="line-card" class:chunk-line={chunkLineKeys.has(`${line.page_number}:${line.line_number}`)} class:focused-line={focusedLine === line} onclick={() => selectLine(line)}>
							<span class="line-number">P{line.page_number} · L{line.line_number}</span>
							<span class="line-type">{line.corrected_line_type || line.line_type}</span>
							<span class="line-content">{line.content}</span>
						</button>
					{/each}
				{/if}
			</div>
		</aside>

		<section class="right">
			<div class="right-toolbar">
				<div class="title">
					<span class="name">{currentInput?.file_name ?? m.chunk_mgmt_no_document_loaded()}</span>
					{#if currentInput}<span class="type">{currentInput.type}</span>{/if}
				</div>
				<div class="stats">
					<span>{m.chunk_mgmt_marks({ highlightCount })}</span>
					<span>{m.chunk_mgmt_bbox({ selectedBBoxesCount: selectedBBoxes.length, plural: selectedBBoxes.length === 1 ? '' : 'es' })}</span>
				</div>
			</div>

			{#if !currentInput}
				<div class="doc-empty">{m.chunk_mgmt_retrieve_a_record_to_display()}</div>
			{:else}
				{#if isPdf}
					<PdfViewWindow
						inputId={currentInput.id}
						{fileUrl}
						bind:page={docPage}
						bind:zoom={pdfZoom}
						bind:numPages={pdfNumPages}
						highlightVersion={`${selectedSeqNo ?? 0}:${highlightSelectionVersion}`}
						highlightPages={[...selectedLinesByPage.keys()].sort((a, b) => a - b)}
						renderHighlights={renderChunkHighlights}
						showSidebar={false}
					>
					</PdfViewWindow>
				{:else}
					<iframe class="doc-iframe" src={fileUrl} title={m.chunk_mgmt_document_viewer()}></iframe>
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
					<div class="dialog-eyebrow">{m.chunk_mgmt_preferences()}</div>
					<h2 class="dialog-title">{m.chunk_mgmt_chunk_view_settings()}</h2>
					<p class="dialog-subtitle">
						{m.chunk_mgmt_adjust_the_chunk_list_layout()}
					</p>
				</div>
			</div>

			<div class="dialog-scroll">
				<div class="dialog-controls">
					<div class="dialog-section">
						<div class="dialog-section-head">
							<div class="dialog-section-title">{m.chunk_mgmt_appearance()}</div>
							<div class="dialog-section-copy">{m.chunk_mgmt_set_the_saved_defaults_used()}</div>
						</div>
						<div class="settings-grid">
							<label class="field dialog-field settings-field">
								<span class="field-label">{m.chunk_mgmt_chunk_background_color()}</span>
								<div class="settings-color-row">
									<input
										class="settings-color-input"
										type="color"
										value={chunkPanelSettings.chunkCardBackground || (darkMode ? '#252f3e' : '#e5dbc6')}
										oninput={(e) =>
											applyChunkPanelSettings({
												chunkCardBackground: (e.currentTarget as HTMLInputElement).value
											})}
									/>
									<button
										type="button"
										class="settings-color-reset"
										disabled={chunkPanelSettings.chunkCardBackground === CHUNK_THEME_BACKGROUND}
										onclick={() =>
											applyChunkPanelSettings({ chunkCardBackground: CHUNK_THEME_BACKGROUND })}
									>
										{m.chunk_mgmt_follow_theme()}
									</button>
								</div>
							</label>
							<div class="field dialog-field settings-field settings-field-wide">
								<span class="field-label">{m.chunk_mgmt_chunks_list_width_px({ chunkListWidth: chunkPanelSettings.chunkListWidth })}</span>
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
									{m.chunk_mgmt_you_can_also_drag_the()}
								</div>
							</div>
							<label class="field dialog-field settings-field settings-field-wide">
								<span class="field-label">{m.chunk_mgmt_zoom_percent({ value: Math.round(pdfZoom * 100) })}</span>
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
				<div class="dialog-foot-hint">{m.chunk_mgmt_updates_are_saved_automatically()}</div>
				<div class="dialog-foot-buttons">
					<button class="btn btn-primary dialog-select-btn" type="button" onclick={resetChunkSettings}>{m.chunk_mgmt_reset()}</button>
					<button class="btn btn-primary dialog-select-btn" type="button" onclick={closeChunkSettings}>
						{m.chunk_mgmt_close()}
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
		grid-template-rows: auto minmax(0, 1fr);
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
	.eyebrow { font-size: 12px; letter-spacing: 0.08em; text-transform: uppercase; color: var(--text-muted); }
	.display { font-family: var(--font-serif); font-size: 32px; line-height: 1.1; margin: 6px 0 2px; }
	.subtitle { color: var(--text-secondary); font-size: 14px; }
	.body {
		display: grid;
		grid-template-rows: minmax(0, 1fr);
		min-height: 0;
	}
	.chunk-sidebar {
		border-right: 1px solid var(--ink-line);
		background: var(--panel-bg);
		display: grid;
		grid-template-rows: auto minmax(0, 1fr);
		min-height: 0;
	}
	.lines-sidebar { min-width: 0; min-height: 0; display: grid; grid-template-rows: auto minmax(0, 1fr); background: var(--panel-bg); border-right: 1px solid var(--ink-line); }
	.lines-header { gap: 4px; }
	.lines-sidebar.collapsed .lines-header { justify-content: center; padding: 10px 2px; }
	.lines-toggle { flex: none; width: 28px; height: 32px; border: 1px solid var(--ink-line); border-radius: 8px; background: var(--panel-bg-alt); color: var(--text-secondary); cursor: pointer; font-size: 20px; line-height: 1; }
	.lines-toggle:hover { color: var(--text-primary); }
	.lines-list { min-height: 0; overflow-y: auto; padding: 10px; scrollbar-width: thin; scrollbar-color: var(--ink-line) transparent; }
	.lines-list::-webkit-scrollbar { width: 6px; }
	.lines-list::-webkit-scrollbar-thumb { background: var(--ink-line); border-radius: 999px; }
	.lines-list::-webkit-scrollbar-track { background: transparent; }
	.line-card { display: grid; grid-template-columns: auto 1fr; gap: 4px 12px; width: 100%; text-align: left; padding: 10px; margin-bottom: 6px; color: var(--text-primary); background: var(--panel-bg-alt); border: 1px solid transparent; border-radius: 8px; cursor: pointer; }
	.line-card.chunk-line { border-color: var(--brass); }
	.line-card.focused-line { background: var(--chunk-selected-bg); color: white; }
	.line-number, .line-type { font: 11px var(--font-mono); color: var(--text-secondary); }
	.line-type { text-transform: uppercase; color: var(--brass); }
	.focused-line .line-number, .focused-line .line-type { color: white; }
	.line-content { grid-column: 1 / -1; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-size: 13px; }
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
	.chunk-list { min-height: 0; overflow: auto; padding: 10px; display: grid; align-content: start; gap: 10px; scrollbar-width: thin; scrollbar-color: var(--ink-line) transparent; }
	.chunk-list::-webkit-scrollbar { width: 6px; }
	.chunk-list::-webkit-scrollbar-thumb { background: var(--ink-line); border-radius: 999px; }
	.chunk-list::-webkit-scrollbar-track { background: transparent; }
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
	.chunk-card.selected { background: var(--chunk-selected-bg); border-color: var(--chunk-selected-bg); color: #fff; }
	.chunk-card.selected .chunk-section-label, .chunk-card.selected .type, .chunk-card.selected .seq, .chunk-card.selected .chunk-keywords, .chunk-card.selected .info-span-chip { color: #fff; }
	.chunk-summary {
		border-radius: 10px;
		padding: 10px;
	}
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
	.info-spans { display: flex; flex-wrap: wrap; gap: 4px; }
	.info-span-chip {
		font-family: var(--font-mono);
		font-size: 11px;
		padding: 2px 6px;
		border-radius: 4px;
		background: rgba(255, 255, 255, 0.05);
		color: var(--text-secondary);
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
		background: var(--error-bg);
		color: var(--error-text);
	}
	.empty { padding: 18px 10px; text-align: center; color: var(--text-secondary); }
	.empty-title { font-weight: 700; margin-bottom: 4px; }
	.empty-sub { font-size: 12px; color: var(--text-muted); }
	.doc-iframe { width: 100%; height: 100%; border: none; }
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
			var(--dialog-section-bg);
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
	.dialog-field {
		margin: 0;
		padding: 10px 10px 8px;
		border-radius: 16px;
		border: 1px solid var(--dialog-field-line);
		background: var(--dialog-field-bg);
		box-shadow: inset 0 1px 0 rgba(255, 255, 255, 0.02);
	}
	.dialog-field :global(input),
	.dialog-field :global(select) {
		background: var(--dialog-input-bg);
		border-color: var(--dialog-input-line);
		color: var(--dialog-input-text);
	}
	.dialog-field :global(input:focus),
	.dialog-field :global(select:focus) {
		border-color: var(--brass);
		box-shadow:
			0 0 0 1px rgba(212, 162, 76, 0.28),
			0 0 0 4px rgba(212, 162, 76, 0.08);
	}
	.dialog-select-btn {
		min-width: 132px;
		background: var(--brass) !important;
		color: #1d1508 !important;
		border: 1px solid var(--brass) !important;
		opacity: 1 !important;
	}
	.dialog-select-btn:hover:not(:disabled) { background: color-mix(in srgb, var(--brass), #fff 14%) !important; color: #1d1508 !important; }
	.dialog-select-btn:disabled {
		background: var(--btn-disabled-bg) !important;
		color: var(--btn-disabled-text) !important;
		border: 1px solid var(--btn-disabled-line) !important;
		box-shadow: none !important;
		cursor: not-allowed !important;
		opacity: 1 !important;
	}
	.dialog-foot {
		padding: 14px 28px 18px;
		display: flex;
		justify-content: space-between;
		align-items: center;
		flex: 0 0 auto;
		background: var(--dialog-section-bg);
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
	.settings-field { background: var(--dialog-field-bg); }
	.settings-field-wide { grid-column: 1 / -1; }
	.settings-color-reset {
		flex-shrink: 0;
		height: 42px;
		padding: 0 12px;
		border: 1px solid var(--ink-line);
		border-radius: 8px;
		background: var(--panel-bg-alt);
		color: var(--text-secondary);
		font-family: var(--font-mono);
		font-size: 11px;
		letter-spacing: 0.08em;
		text-transform: uppercase;
		cursor: pointer;
	}
	.settings-color-reset:hover:not(:disabled) {
		color: var(--brass);
		border-color: var(--brass);
		background: var(--brass-faint);
	}
	.settings-color-reset:disabled {
		opacity: 0.45;
		cursor: default;
	}
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
		background: var(--brass);
		border: 2px solid rgba(0, 0, 0, 0.3);
		box-shadow: 0 1px 6px rgba(0, 0, 0, 0.35);
		cursor: pointer;
	}
	.settings-width-slider::-moz-range-thumb {
		width: 16px;
		height: 16px;
		border-radius: 50%;
		background: var(--brass);
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
		.body { grid-template-columns: 1fr !important; overflow-y: auto; grid-template-rows: auto auto auto minmax(300px, 50vh); }
		.chunk-sidebar { max-height: 44vh; border-right: none; border-bottom: 1px solid var(--ink-line); }
		.lines-sidebar { height: 35vh; }
		.lines-sidebar.collapsed { height: auto; }
		.chunk-list-resizer { display: none; }
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
		.settings-grid { grid-template-columns: 1fr; }
		.dialog-section-head,
		.dialog-foot { flex-direction: column; align-items: flex-start; }
		.dialog-foot-buttons { width: 100%; }
		.left-meta { align-items: flex-start; flex-direction: column; }
		.settings-color-row { flex-direction: column; align-items: stretch; }
		.dialog-select-btn,
		.dialog-foot-buttons .btn { width: 100%; }
	}
</style>
