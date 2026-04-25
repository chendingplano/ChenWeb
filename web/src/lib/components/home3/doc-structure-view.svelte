<script lang="ts">
	import { onMount } from 'svelte';
	import {
		listKbInputs,
		getKbDocStructure,
		getKbInput,
		type DocStructureLine,
		type KbInputRecord
	} from '$lib/services/kbService';
	import SharedPdfViewer from '$lib/components/home3/shared-pdf-viewer.svelte';

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
	const INFO_PANEL_DEFAULT_WIDTH = 270;
	const INFO_PANEL_MIN_WIDTH = 140;
	const INFO_PANEL_MAX_WIDTH = 420;

	let recordIdInput = $state('');
	let currentInput = $state<KbInputRecord | null>(null);
	let correctedFile = $state('');
	let lines = $state<DocStructureLine[]>([]);
	let selectedLineKey = $state<string | null>(null);
	let highlightSelectionVersion = $state(0);
	let selectedHighlightTarget = $state<{
		page: number;
		coords: number[];
		label: string;
		version: number;
	} | null>(null);
	let headingsOnly = $state(false);
	let loading = $state(false);
	let errorMsg = $state('');
	let infoPanelWidth = $state(INFO_PANEL_DEFAULT_WIDTH);
	let infoPanelResizing = $state(false);
	let searchOpen = $state(false);
	let searchRecordId = $state('');
	let searchTitle = $state('');
	let searchDocNo = $state('');
	let searchFileName = $state('');
	let searchDocType = $state('all');
	let searchParserName = $state('');
	let searchOperation = $state('');
	let searchProcStatus = $state('all');
	let searchCreateStart = $state('');
	let searchCreateEnd = $state('');
	let searchModifyStart = $state('');
	let searchModifyEnd = $state('');
	let searchResults = $state<KbInputRecord[]>([]);
	let searchLoading = $state(false);
	let searchError = $state('');
	let searchSelected = $state<number | null>(null);

	let docPage = $state(1);
	let pdfZoom = $state(0.5);
	let pdfNumPages = $state(0);

	const docTypeOptions = [
		'all',
		'pdf',
		'doc',
		'excel',
		'ppt',
		'text',
		'json',
		'xml',
		'markdown',
		'typst'
	];
	const procStatusOptions = ['all', 'success', 'fail'];

	type PdfPageViewport = {
		width: number;
		height: number;
		convertToViewportRectangle: (rect: number[]) => number[];
	};

	let isPdf = $derived((currentInput?.type ?? '').toLowerCase() === 'pdf');
	let fileUrl = $derived.by(() => {
		if (!currentInput) return '';
		return `/api/v1/kb/inputs/${currentInput.id}/file#page=${docPage}&zoom=page-width`;
	});

	function lineKey(line: DocStructureLine): string {
		return `${line.page_number}:${line.line_number}`;
	}

	function effectiveLineType(line: DocStructureLine): string {
		const corrected = line.corrected_line_type?.trim().toLowerCase() ?? '';
		if (corrected !== '' && corrected !== 'unchanged') return corrected;
		return line.line_type?.trim().toLowerCase() ?? '';
	}

	function isHeadingLine(line: DocStructureLine): boolean {
		const t = effectiveLineType(line);
		return /^heading(?:-\d+)?$/i.test(t) || t.includes('heading');
	}

	let filteredLines = $derived.by(() =>
		headingsOnly ? lines.filter((ln) => isHeadingLine(ln)) : lines
	);

	let headingCount = $derived.by(() => lines.filter((ln) => isHeadingLine(ln)).length);

	let selectedLine = $derived.by(() => {
		if (!selectedLineKey) return null;
		return lines.find((ln) => lineKey(ln) === selectedLineKey) ?? null;
	});

	let selectedLinesByPage = $derived.by(() => {
		const map = new Map<number, DocStructureLine[]>();
		if (!selectedLine) return map;
		if (!Array.isArray(selectedLine.coords) || selectedLine.coords.length < 4) return map;
		map.set(selectedLine.page_number, [selectedLine]);
		return map;
	});

	let highlightCount = $derived.by(() => {
		let n = 0;
		for (const arr of selectedLinesByPage.values()) n += arr.length;
		return n;
	});

	function renderStructureHighlights(
		pageNo: number,
		viewport: PdfPageViewport,
		overlay: HTMLDivElement
	) {
		const target = selectedHighlightTarget;
		if (!target || target.page !== pageNo || target.coords.length < 4) return;
		const [vx1, vy1, vx2, vy2] = viewport.convertToViewportRectangle(target.coords.slice(0, 4));
		const left = Math.max(0, Math.min(vx1, vx2) - 5);
		const top = Math.max(0, Math.min(vy1, vy2) - 4);
		const width = Math.abs(vx2 - vx1) + 10;
		const height = Math.abs(vy2 - vy1) + 8;
		if (width < 1 || height < 1) return;
		const mark = document.createElement('div');
		mark.className = 'pdf-highlight';
		mark.style.left = `${left}px`;
		mark.style.top = `${top}px`;
		mark.style.width = `${width}px`;
		mark.style.height = `${height}px`;
		mark.title = target.label;
		overlay.appendChild(mark);
	}

	async function doRetrieve() {
		errorMsg = '';
		const id = Number(recordIdInput.trim());
		if (!Number.isFinite(id) || id <= 0) {
			errorMsg = 'Enter a valid Record ID';
			return;
		}
		loading = true;
		lines = [];
		currentInput = null;
		selectedLineKey = null;
		highlightSelectionVersion = 0;
		selectedHighlightTarget = null;
		docPage = 1;
		pdfZoom = 0.5;
		pdfNumPages = 0;
		correctedFile = '';
		try {
			const [structureRes, inputRes] = await Promise.all([
				getKbDocStructure(id),
				getKbInput(id).catch(() => null)
			]);
			lines = structureRes.lines ?? [];
			correctedFile = structureRes.corrected_file ?? '';
			currentInput = inputRes?.record ?? null;
			const first = (headingsOnly ? lines.filter((ln) => isHeadingLine(ln)) : lines)[0];
			if (first) {
				await selectLine(first);
			}
		} catch (err) {
			errorMsg = err instanceof Error ? err.message : 'Failed to retrieve document structure';
		} finally {
			loading = false;
		}
	}

	async function selectLine(line: DocStructureLine) {
		selectedLineKey = lineKey(line);
		highlightSelectionVersion += 1;
		selectedHighlightTarget =
			Array.isArray(line.coords) && line.coords.length >= 4
				? {
						page: line.page_number,
						coords: line.coords.slice(0, 4),
						label: `page ${line.page_number}, line ${line.line_number}`,
						version: highlightSelectionVersion
					}
				: null;
		if (line.page_number > 0) {
			docPage = line.page_number;
		}
	}

	function clampInfoPanelWidth(width: number): number {
		return Math.min(INFO_PANEL_MAX_WIDTH, Math.max(INFO_PANEL_MIN_WIDTH, Math.round(width)));
	}

	function adjustInfoPanelWidth(delta: number) {
		infoPanelWidth = clampInfoPanelWidth(infoPanelWidth + delta);
	}

	function startInfoPanelResize(event: PointerEvent) {
		event.preventDefault();
		const handle = event.currentTarget as HTMLElement | null;
		const startX = event.clientX;
		const startWidth = infoPanelWidth;
		infoPanelResizing = true;
		document.body.style.cursor = 'col-resize';
		document.body.style.userSelect = 'none';
		handle?.setPointerCapture?.(event.pointerId);

		const handleMove = (moveEvent: PointerEvent) => {
			infoPanelWidth = clampInfoPanelWidth(startWidth + (moveEvent.clientX - startX));
		};
		const handleUp = (upEvent: PointerEvent) => {
			infoPanelResizing = false;
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

	function onInfoPanelResizerKeydown(event: KeyboardEvent) {
		if (event.key === 'ArrowLeft') {
			event.preventDefault();
			adjustInfoPanelWidth(-16);
		} else if (event.key === 'ArrowRight') {
			event.preventDefault();
			adjustInfoPanelWidth(16);
		} else if (event.key === 'Home') {
			event.preventDefault();
			infoPanelWidth = INFO_PANEL_MIN_WIDTH;
		} else if (event.key === 'End') {
			event.preventDefault();
			infoPanelWidth = INFO_PANEL_MAX_WIDTH;
		}
	}

	function openSearch() {
		searchOpen = true;
		searchSelected = null;
		searchResults = [];
		searchError = '';
		searchRecordId = '';
		searchTitle = '';
		searchDocNo = '';
		searchFileName = '';
		searchDocType = 'all';
		searchParserName = '';
		searchOperation = '';
		searchProcStatus = 'all';
		searchCreateStart = '';
		searchCreateEnd = '';
		searchModifyStart = '';
		searchModifyEnd = '';
	}

	function closeSearch() {
		searchOpen = false;
	}

	async function runSearch() {
		searchLoading = true;
		searchError = '';
		try {
			const res = await listKbInputs({
				recordId: searchRecordId,
				docType: searchDocType,
				parseState: 'all',
				title: searchTitle,
				docNo: searchDocNo,
				fileName: searchFileName,
				parserName: searchParserName,
				operation: searchOperation,
				procStatus: searchProcStatus === 'all' ? '' : searchProcStatus,
				startTime: searchCreateStart,
				endTime: searchCreateEnd,
				modifyStartTime: searchModifyStart,
				modifyEndTime: searchModifyEnd,
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

	function recordDisplayName(r: KbInputRecord): string {
		return r.title?.trim() || r.name?.trim() || r.file_name?.trim() || `Input #${r.id}`;
	}

	function recordDisplayDocNo(r: KbInputRecord): string {
		return r.doc_no?.trim() || '—';
	}

	function searchStatusText(record: KbInputRecord): { operation: string; procStatus: string } {
		const items = record.status ?? [];
		const desiredOperation = searchOperation.trim().toLowerCase();
		const matched =
			desiredOperation !== ''
				? [...items]
						.reverse()
						.find((item) => (item?.operation ?? '').trim().toLowerCase() === desiredOperation)
				: [...items].reverse().find((item) => item != null);

		if (!matched) {
			return { operation: '—', procStatus: 'pending' };
		}

		return {
			operation: matched.operation?.trim() || '—',
			procStatus:
				matched.proc_status?.trim() ||
				matched['proc-status']?.trim() ||
				matched.status?.trim() ||
				'pending'
		};
	}

	onMount(() => {
		return () => {
			infoPanelResizing = false;
			document.body.style.cursor = '';
			document.body.style.userSelect = '';
		};
	});
</script>

<div
	class="doc-structure"
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
			<div class="eyebrow">Knowledge System · Vol. IV</div>
			<h1 class="display">Document Structure</h1>
			<div class="subtitle">Inspect corrected structure labels and highlight their source region in the PDF.</div>
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
						<button
							class="btn btn-ghost"
							type="button"
							onclick={openSearch}
							title="Search records from kb.inputs"
						>
							<span class="btn-icon">⌕</span>Search
						</button>
					</div>
				</label>

				<div class="btn-row">
					<button class="btn btn-primary retrieve" onclick={doRetrieve} disabled={loading}>
						{#if loading}
							<span class="spinner"></span>Retrieving…
						{:else}
							Retrieve
						{/if}
					</button>
					<button
						class="btn btn-secondary"
						type="button"
						class:active={headingsOnly}
						onclick={() => {
							headingsOnly = !headingsOnly;
						}}
					>
						Headings Only
					</button>
				</div>

				{#if errorMsg}<div class="error">{errorMsg}</div>{/if}
			</div>

			<div class="left-meta">
				<div class="left-meta-title">Lines</div>
				<div class="left-meta-count">{filteredLines.length} shown / {lines.length} total</div>
			</div>

			<div class="line-list">
				{#if !loading && filteredLines.length === 0}
					<div class="empty">
						<div class="empty-title">No lines loaded</div>
						<div class="empty-sub">
							Enter a Record ID to load a `.corrected` file.
							{#if headingsOnly}No heading lines found in current result.{/if}
						</div>
					</div>
				{:else}
					{#each filteredLines as line (`${line.page_number}-${line.line_number}`)}
						<button
							type="button"
							class="line-card"
							class:selected={selectedLineKey === lineKey(line)}
							onclick={() => selectLine(line)}
						>
							<div class="line-ref-row">
								<span class="line-ref">P{line.page_number}:L{line.line_number}</span>
								<span class="line-type-chip">{line.corrected_line_type}</span>
							</div>
							<div class="line-content">{line.content || '—'}</div>
							<div class="line-sub">
								orig: {line.line_type}
								{#if line.font}
									· {line.font} {line.font_size}
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
					<span>{highlightCount} mark</span>
					<span>{headingCount} headings</span>
				</div>
			</div>

			{#if !currentInput}
				<div class="doc-empty">Retrieve a record to display document and structure highlights.</div>
			{:else}
				{#if isPdf}
					<SharedPdfViewer
						inputId={currentInput.id}
						{fileUrl}
						bind:page={docPage}
						bind:zoom={pdfZoom}
						bind:numPages={pdfNumPages}
						highlightVersion={selectedHighlightTarget
							? `${selectedHighlightTarget.page}:${selectedHighlightTarget.coords.join(',')}:${selectedHighlightTarget.version}`
							: `${selectedLineKey ?? ''}:${highlightSelectionVersion}`}
						renderHighlights={renderStructureHighlights}
					>
						<div slot="sidebar">
							<div class="info-panel-shell" style={`width:${infoPanelWidth}px;`}>
								<aside class="meta-panel">
									<div class="meta-title">Selected Line</div>
									{#if !selectedLine}
										<div class="meta-empty">Select a line from the left panel.</div>
									{:else}
										<div class="meta-row"><span>Page</span><strong>{selectedLine.page_number}</strong></div>
										<div class="meta-row"><span>Line</span><strong>{selectedLine.line_number}</strong></div>
										<div class="meta-row">
											<span>Corrected</span><strong>{selectedLine.corrected_line_type}</strong>
										</div>
										<div class="meta-row"><span>Original</span><strong>{selectedLine.line_type}</strong></div>
										<div class="meta-row mono">
											[{selectedLine.coords.map((n) => Math.trunc(n)).join(', ')}]
										</div>
									{/if}
									{#if correctedFile}
										<div class="meta-title corrected-title">Corrected File</div>
										<div class="meta-file">{correctedFile}</div>
									{/if}
								</aside>
								<button
									type="button"
									class="info-panel-resizer"
									class:active={infoPanelResizing}
									aria-label="Resize information panel"
									onpointerdown={startInfoPanelResize}
									onkeydown={onInfoPanelResizerKeydown}
								>
									<span class="info-panel-resizer-grip" aria-hidden="true"></span>
								</button>
							</div>
						</div>
					</SharedPdfViewer>
				{:else}
					<iframe class="doc-iframe" src={fileUrl} title="Document viewer"></iframe>
				{/if}
			{/if}
		</section>
	</div>
</div>

{#if searchOpen}
	<div class="dialog-overlay" aria-hidden="true">
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
					<p class="dialog-subtitle">
						Search by record metadata, parser pipeline state, and create or modify windows.
					</p>
				</div>
			</div>

			<div class="dialog-scroll">
				<div class="dialog-controls">
					<div class="dialog-section">
						<div class="dialog-section-head">
							<div class="dialog-section-title">Identity</div>
							<div class="dialog-section-copy">Match the record itself and its document metadata.</div>
						</div>
						<div class="dialog-grid dialog-grid-primary">
							<label class="field dialog-field">
								<span class="field-label">Record ID</span>
								<input
									type="text"
									bind:value={searchRecordId}
									placeholder="84"
									onkeydown={(e) => {
										if (e.key === 'Enter') runSearch();
									}}
								/>
							</label>
							<label class="field dialog-field">
								<span class="field-label">Type</span>
								<select bind:value={searchDocType}>
									{#each docTypeOptions as o}
										<option value={o}>{o}</option>
									{/each}
								</select>
							</label>
							<label class="field dialog-field dialog-field-wide">
								<span class="field-label">Title contains</span>
								<input
									type="text"
									bind:value={searchTitle}
									placeholder="Input title, standard title…"
									onkeydown={(e) => {
										if (e.key === 'Enter') runSearch();
									}}
								/>
							</label>
							<label class="field dialog-field">
								<span class="field-label">Doc No contains</span>
								<input
									type="text"
									bind:value={searchDocNo}
									placeholder="GB/T 123…"
									onkeydown={(e) => {
										if (e.key === 'Enter') runSearch();
									}}
								/>
							</label>
							<label class="field dialog-field dialog-field-wide">
								<span class="field-label">File name contains</span>
								<input
									type="text"
									bind:value={searchFileName}
									placeholder="report, spec, drawing…"
									onkeydown={(e) => {
										if (e.key === 'Enter') runSearch();
									}}
								/>
							</label>
						</div>
					</div>

					<div class="dialog-section">
						<div class="dialog-section-head">
							<div class="dialog-section-title">Processing Status</div>
							<div class="dialog-section-copy">
								Filter the pipeline entry by parser, operation, and final `proc_status`.
							</div>
						</div>
						<div class="dialog-grid">
							<label class="field dialog-field">
								<span class="field-label">Parser name</span>
								<input
									type="text"
									bind:value={searchParserName}
									placeholder="mineru, pdf-parser…"
									onkeydown={(e) => {
										if (e.key === 'Enter') runSearch();
									}}
								/>
							</label>
							<label class="field dialog-field">
								<span class="field-label">Operation</span>
								<input
									type="text"
									bind:value={searchOperation}
									placeholder="extract_metadata"
									onkeydown={(e) => {
										if (e.key === 'Enter') runSearch();
									}}
								/>
							</label>
							<label class="field dialog-field">
								<span class="field-label">Proc status</span>
								<select bind:value={searchProcStatus}>
									{#each procStatusOptions as option}
										<option value={option}>{option}</option>
									{/each}
								</select>
							</label>
						</div>
					</div>

					<div class="dialog-section">
						<div class="dialog-section-head">
							<div class="dialog-section-title">Time Windows</div>
							<div class="dialog-section-copy">
								Search by create and modify timestamps using local date-time ranges.
							</div>
						</div>
						<div class="dialog-grid dialog-grid-time">
							<label class="field dialog-field">
								<span class="field-label">Create time from</span>
								<input type="datetime-local" bind:value={searchCreateStart} />
							</label>
							<label class="field dialog-field">
								<span class="field-label">Create time to</span>
								<input type="datetime-local" bind:value={searchCreateEnd} />
							</label>
							<label class="field dialog-field">
								<span class="field-label">Modify time from</span>
								<input type="datetime-local" bind:value={searchModifyStart} />
							</label>
							<label class="field dialog-field">
								<span class="field-label">Modify time to</span>
								<input type="datetime-local" bind:value={searchModifyEnd} />
							</label>
						</div>
					</div>

					<div class="dialog-toolbar">
						<div class="dialog-toolbar-copy">
							<div class="dialog-toolbar-title">Search Scope</div>
							<div class="dialog-toolbar-text">
								Leave fields empty to broaden the search. Results are capped to the newest 50 records.
							</div>
						</div>
						<div class="dialog-toolbar-actions">
							<button
								class="btn btn-ghost"
								onclick={() => {
									searchRecordId = '';
									searchTitle = '';
									searchDocNo = '';
									searchFileName = '';
									searchDocType = 'all';
									searchParserName = '';
									searchOperation = '';
									searchProcStatus = 'all';
									searchCreateStart = '';
									searchCreateEnd = '';
									searchModifyStart = '';
									searchModifyEnd = '';
									searchResults = [];
									searchSelected = null;
									searchError = '';
								}}>Reset</button
							>
							<button class="btn btn-primary dialog-search-btn" onclick={runSearch} disabled={searchLoading}>
								{searchLoading ? 'Searching…' : 'Search'}
							</button>
						</div>
					</div>
				</div>

				{#if searchError}
					<div class="error dialog-error">{searchError}</div>
				{/if}

				<div class="dialog-results">
					{#if searchResults.length === 0 && !searchLoading}
						<div class="dialog-empty">
							<div class="empty-glyph">⌕</div>
							<div class="dialog-empty-title">Run a search to see records.</div>
							<div class="dialog-empty-copy">
								Use any combination of metadata, parser, and time filters to narrow the archive.
							</div>
						</div>
					{:else}
						<table class="result-table">
							<thead>
								<tr>
									<th>ID</th>
									<th>Type</th>
									<th>Title / Doc No</th>
									<th>File name</th>
									<th>Parser</th>
									<th>Status</th>
									<th>Created</th>
									<th>Updated</th>
								</tr>
							</thead>
							<tbody>
								{#each searchResults as r (r.id)}
									{@const statusSummary = searchStatusText(r)}
									<tr
										class:selected={searchSelected === r.id}
										onclick={() => (searchSelected = r.id)}
										ondblclick={() => pickSearchResult(r)}
									>
										<td class="mono">{r.id}</td>
										<td>{r.type}</td>
										<td>
											<div class="result-primary">{recordDisplayName(r)}</div>
											<div class="result-secondary mono">{recordDisplayDocNo(r)}</div>
										</td>
										<td class="ellipsis">{r.file_name ?? '—'}</td>
										<td class="mono muted">{r.parser_name || '—'}</td>
										<td>
											<div class="status-stack">
												<span class="status-chip mono">{statusSummary.operation}</span>
												<span
													class="status-pill"
													class:status-pill-success={statusSummary.procStatus.toLowerCase() === 'success'}
													class:status-pill-fail={statusSummary.procStatus.toLowerCase() === 'fail' ||
														statusSummary.procStatus.toLowerCase() === 'failed'}
												>
													{statusSummary.procStatus}
												</span>
											</div>
										</td>
										<td class="mono muted">{(r.create_time ?? '').slice(0, 19).replace('T', ' ')}</td>
										<td class="mono muted">{(r.modify_time ?? '').slice(0, 19).replace('T', ' ')}</td>
									</tr>
								{/each}
							</tbody>
						</table>
					{/if}
				</div>
			</div>

			<div class="dialog-foot">
				<div class="dialog-foot-hint">Double-click a row, or select &amp; press Select.</div>
				<div class="dialog-foot-buttons">
					<button class="btn btn-ghost" onclick={closeSearch}>Cancel</button>
					<button
						class="btn btn-primary dialog-select-btn"
						onclick={confirmSearchSelection}
						disabled={searchSelected == null}>Select</button
					>
				</div>
			</div>
		</div>
	</div>
{/if}

<style>
	@import url('https://fonts.googleapis.com/css2?family=Cormorant+Garamond:ital,wght@0,400;0,500;0,600;1,400&family=JetBrains+Mono:wght@400;500;600&family=Inter+Tight:wght@400;500;600&display=swap');

	.doc-structure {
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
	.eyebrow {
		font-size: 12px;
		letter-spacing: 0.08em;
		text-transform: uppercase;
		color: var(--text-muted);
	}
	.display {
		font-family: var(--font-serif);
		font-size: 32px;
		line-height: 1.1;
		margin: 6px 0 2px;
	}
	.subtitle {
		color: var(--text-secondary);
		font-size: 14px;
	}
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
	.left-controls {
		padding: 14px;
		border-bottom: 1px solid var(--ink-line-soft);
	}
	.field-label {
		display: block;
		font-size: 12px;
		color: var(--text-muted);
		margin-bottom: 8px;
	}
	.field-row {
		display: flex;
		gap: 8px;
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
	.btn-row {
		display: grid;
		grid-template-columns: 1fr auto;
		gap: 8px;
		margin-top: 10px;
	}
	.btn {
		height: 38px;
		border: none;
		border-radius: 10px;
		font-weight: 600;
		cursor: pointer;
	}
	.btn-primary {
		background: var(--brass);
		color: #1d1508;
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
	.btn-secondary {
		padding: 0 12px;
		background: var(--panel-bg-alt);
		color: var(--text-secondary);
		border: 1px solid var(--ink-line);
	}
	.btn-secondary.active {
		color: var(--brass);
		border-color: var(--brass);
		box-shadow: 0 0 0 1px var(--brass-faint) inset;
	}
	.btn:disabled {
		opacity: 0.6;
		cursor: default;
	}
	.btn-icon {
		font-size: 13px;
		line-height: 1;
	}
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
	.line-list {
		overflow: auto;
		padding: 10px;
		display: grid;
		gap: 10px;
	}
	.line-card {
		border: 1px solid var(--ink-line-soft);
		background: var(--panel-bg-alt);
		border-radius: 12px;
		padding: 10px;
		text-align: left;
		cursor: pointer;
		color: inherit;
	}
	.line-card.selected {
		border-color: var(--brass);
		box-shadow: 0 0 0 1px var(--brass-faint) inset;
	}
	.line-ref-row {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 8px;
		margin-bottom: 6px;
	}
	.line-ref {
		font-family: var(--font-mono);
		font-size: 11px;
		padding: 2px 7px;
		border-radius: 999px;
		background: var(--brass-faint);
		color: var(--brass);
	}
	.line-type-chip {
		font-size: 11px;
		text-transform: uppercase;
		color: var(--text-muted);
		letter-spacing: 0.06em;
	}
	.line-content {
		font-size: 13px;
		line-height: 1.4;
		margin-bottom: 6px;
	}
	.line-sub {
		font-size: 12px;
		color: var(--text-muted);
	}
	.right {
		min-width: 0;
		display: grid;
		grid-template-rows: auto 1fr;
		min-height: 0;
	}
	.right-toolbar {
		display: flex;
		justify-content: space-between;
		align-items: center;
		padding: 12px 14px;
		border-bottom: 1px solid var(--ink-line);
		background: var(--panel-bg);
	}
	.title {
		display: flex;
		align-items: center;
		gap: 10px;
		min-width: 0;
	}
	.name {
		font-weight: 700;
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
	}
	.type {
		color: var(--text-muted);
		text-transform: uppercase;
		font-size: 11px;
		letter-spacing: 0.06em;
	}
	.stats {
		display: flex;
		gap: 14px;
		color: var(--text-secondary);
		font-size: 12px;
	}
	.doc-empty {
		padding: 20px;
		color: var(--text-secondary);
	}
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
	.page-input {
		width: 72px;
		height: 30px;
	}
	.pdf-stage {
		min-height: 0;
		overflow: auto;
		padding: 14px;
	}
	.pdf-layout {
		display: grid;
		grid-template-columns: 300px 1fr;
		gap: 14px;
	}
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
	.info-panel-shell {
		position: sticky;
		top: 6px;
		align-self: start;
		width: 270px;
		min-width: 140px;
		max-width: 420px;
		flex: 0 0 auto;
		padding-right: 16px;
	}
	.info-panel-resizer {
		position: absolute;
		top: 0;
		right: 0;
		bottom: 0;
		display: flex;
		align-items: center;
		justify-content: center;
		width: 16px;
		min-height: 120px;
		padding: 0;
		border: 0;
		background: transparent;
		cursor: col-resize;
		user-select: none;
		touch-action: none;
		outline: none;
		z-index: 4;
	}
	.info-panel-resizer::before {
		content: '';
		width: 1px;
		height: 100%;
		background: var(--ink-line);
		opacity: 0.8;
		transition: background 150ms ease;
	}
	.info-panel-resizer:hover::before,
	.info-panel-resizer.active::before,
	.info-panel-resizer:focus-visible::before {
		background: var(--brass);
	}
	.info-panel-resizer-grip {
		position: absolute;
		width: 8px;
		height: 52px;
		border-radius: 999px;
		background:
			radial-gradient(circle, var(--text-muted) 22%, transparent 24%) center 6px / 6px 12px repeat-y,
			var(--panel-bg);
		border: 1px solid var(--ink-line-soft);
		box-shadow: 0 0 0 2px rgba(0, 0, 0, 0.14);
		transition:
			border-color 150ms ease,
			background-color 150ms ease;
	}
	.info-panel-resizer:hover .info-panel-resizer-grip,
	.info-panel-resizer.active .info-panel-resizer-grip,
	.info-panel-resizer:focus-visible .info-panel-resizer-grip {
		border-color: var(--brass);
		background:
			radial-gradient(circle, var(--brass) 22%, transparent 24%) center 6px / 6px 12px repeat-y,
			var(--panel-bg);
	}
	.meta-title {
		font-weight: 700;
		margin-bottom: 8px;
	}
	.corrected-title {
		margin-top: 12px;
	}
	.meta-row {
		display: flex;
		justify-content: space-between;
		gap: 8px;
		margin-bottom: 8px;
		font-size: 13px;
		color: var(--text-secondary);
	}
	.meta-row strong {
		color: var(--text-primary);
	}
	.meta-row.mono {
		font-family: var(--font-mono);
		font-size: 12px;
		word-break: break-all;
	}
	.meta-file {
		font-family: var(--font-mono);
		font-size: 11px;
		line-height: 1.4;
		color: var(--text-muted);
		word-break: break-all;
	}
	.meta-empty {
		color: var(--text-muted);
		font-size: 12px;
	}
	.pdf-canvas-col {
		display: grid;
		gap: 14px;
	}
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
		background: rgba(212, 162, 76, 0.3);
		outline: 1px solid rgba(212, 162, 76, 0.85);
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
	.empty {
		padding: 18px 10px;
		text-align: center;
		color: var(--text-secondary);
	}
	.empty-title {
		font-weight: 700;
		margin-bottom: 4px;
	}
	.empty-sub {
		font-size: 12px;
		color: var(--text-muted);
	}
	.doc-iframe {
		width: 100%;
		height: 100%;
		border: none;
	}
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
		to {
			transform: rotate(360deg);
		}
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
		color: #5dafA8;
	}
	.status-pill-fail {
		background: rgba(200, 85, 61, 0.18);
		color: #c8553d;
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
	@media (max-width: 1200px) {
		.body {
			grid-template-columns: 1fr;
		}
		.left {
			max-height: 44vh;
			border-right: none;
			border-bottom: 1px solid var(--ink-line);
		}
		.pdf-layout {
			grid-template-columns: 1fr;
		}
		.info-panel-shell,
		.meta-panel {
			position: static;
		}
		.info-panel-resizer {
			display: none;
		}
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
		.dialog-grid-primary,
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
			align-items: flex-start;
		}
		.dialog-toolbar-actions,
		.dialog-foot-buttons {
			width: 100%;
		}
		.dialog-search-btn,
		.dialog-select-btn,
		.dialog-foot-buttons .btn {
			width: 100%;
		}
	}
</style>
