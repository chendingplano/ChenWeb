	<script lang="ts">
	import { onMount } from 'svelte';
	import SettingsIcon from '@lucide/svelte/icons/settings';
	import Trash2Icon from '@lucide/svelte/icons/trash-2';
	import {
		getKbDocStructure,
		getKbInput,
		updateKbDocStructureLine,
		deleteKbDocStructureLine,
		splitKbDocStructureLines,
		type DocStructureLine,
		type KbInputRecord
	} from '$lib/services/kbService';
	import KbInputRecordBrowser from '$lib/components/home3/kb-input-record-browser.svelte';
	import {
		DOC_STRUCTURE_DEFAULT_SETTINGS,
		DOC_STRUCTURE_RECORD_DEFAULT_BACKGROUND,
		DOC_STRUCTURE_RECORD_GAP_MAX,
		DOC_STRUCTURE_RECORD_GAP_MIN,
		DOC_STRUCTURE_RECORD_MAX_HEIGHT,
		DOC_STRUCTURE_RECORD_MIN_HEIGHT,
		DOC_STRUCTURE_LINE_LIST_MAX_WIDTH,
		DOC_STRUCTURE_LINE_LIST_MIN_WIDTH,
		clampDocStructureLineListWidth,
		readDocStructureSettings,
		writeDocStructureSettings,
		clampDocStructureRecordGap,
		clampDocStructureRecordHeight
	} from './doc-structure-settings.js';
	import {
		DOC_STRUCTURE_FILTER_OPTIONS,
		effectiveDocStructureLineType,
		filterDocStructureLines,
		getDocStructureFilterLabel
	} from './doc-structure-filters.js';
	import PdfViewWindow from '$lib/components/home3/pdf-view-window.svelte';

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
	type DocStructureSettings = {
		lineListWidth: number;
		recordBackground: string;
		recordHeight: number;
		recordGap: number;
	};

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
	type DocStructureFilterValue =
		| 'all'
		| 'headings'
		| 'paragraphs'
		| 'lists'
		| 'tables'
		| 'formulas';

	let lineFilter = $state<DocStructureFilterValue>('all');
	let loading = $state(false);
	let errorMsg = $state('');
	let docStructureSettings = $state<DocStructureSettings>({ ...DOC_STRUCTURE_DEFAULT_SETTINGS });
	let lineListWidth = $derived(docStructureSettings.lineListWidth);
	let recordBackground = $derived(docStructureSettings.recordBackground);
	let recordHeight = $derived(docStructureSettings.recordHeight);
	let recordGap = $derived(docStructureSettings.recordGap);
	let lineListResizing = $state(false);
	let lineListSettingsOpen = $state(false);
	let settingsHydrated = $state(false);

	let editingLineKey = $state<string | null>(null);
	let editingCorrectedType = $state('');
	let editingContent = $state('');
	let editingOriginalType = $state('');
	let editingOriginalContent = $state('');
	let editingSaving = $state(false);
	let editingError = $state('');
	let editingHasChanges = $derived(
		editingCorrectedType !== editingOriginalType || editingContent !== editingOriginalContent
	);

	// Sidebar: inline Line Type edit
	let sidebarTypeEditing = $state(false);
	let sidebarTypeSaving = $state(false);
	let sidebarTypeError = $state('');

	// Sidebar: Content edit dialog
	let sidebarContentDialogOpen = $state(false);
	let sidebarContentDraftText = $state('');
	let sidebarContentSaving = $state(false);
	let sidebarContentError = $state('');

	let sidebarContentParagraphCount = $derived(
		sidebarContentDraftText.split('\n').filter((s) => s.trim().length > 0).length
	);

	let docPage = $state(1);
	let pdfZoom = $state(0.5);
	let pdfNumPages = $state(0);

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

	function isHeadingLine(line: DocStructureLine): boolean {
		const t = effectiveDocStructureLineType(line);
		return /^heading(?:-\d+)?$/i.test(t) || t.includes('heading');
	}

	function displayLineType(line: DocStructureLine): string {
		const t = effectiveDocStructureLineType(line);
		return t === '' ? 'unknown' : t;
	}

	function getDocStructureSettingsUserId(): string | null {
		return null;
	}

	function applyDocStructureSettings(next: Partial<DocStructureSettings>) {
		docStructureSettings = {
			...docStructureSettings,
			...next,
			lineListWidth: clampDocStructureLineListWidth(
				next.lineListWidth ?? docStructureSettings.lineListWidth
			),
			recordHeight: clampDocStructureRecordHeight(
				next.recordHeight ?? docStructureSettings.recordHeight
			),
			recordGap: clampDocStructureRecordGap(next.recordGap ?? docStructureSettings.recordGap)
		};
	}

	function openLineListSettings() {
		lineListSettingsOpen = true;
	}

	function closeLineListSettings() {
		lineListSettingsOpen = false;
	}

	function resetLineListSettings() {
		docStructureSettings = { ...DOC_STRUCTURE_DEFAULT_SETTINGS };
	}

	let filteredLines = $derived.by(() => filterDocStructureLines(lines, lineFilter));

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

	async function loadStructureForRecord(id: number) {
		errorMsg = '';
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
			const first = filterDocStructureLines(lines, lineFilter)[0];
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

	function adjustLineListWidth(delta: number) {
		applyDocStructureSettings({
			lineListWidth: clampDocStructureLineListWidth(lineListWidth + delta)
		});
	}

	function startLineListResize(event: PointerEvent) {
		event.preventDefault();
		const handle = event.currentTarget as HTMLElement | null;
		const startX = event.clientX;
		const startWidth = lineListWidth;
		lineListResizing = true;
		document.body.style.cursor = 'col-resize';
		document.body.style.userSelect = 'none';
		handle?.setPointerCapture?.(event.pointerId);

		const handleMove = (moveEvent: PointerEvent) => {
			applyDocStructureSettings({
				lineListWidth: clampDocStructureLineListWidth(startWidth + (moveEvent.clientX - startX))
			});
		};
		const handleUp = (upEvent: PointerEvent) => {
			lineListResizing = false;
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

	function onLineListResizerKeydown(event: KeyboardEvent) {
		if (event.key === 'ArrowLeft') {
			event.preventDefault();
			adjustLineListWidth(-16);
		} else if (event.key === 'ArrowRight') {
			event.preventDefault();
			adjustLineListWidth(16);
		} else if (event.key === 'Home') {
			event.preventDefault();
			applyDocStructureSettings({ lineListWidth: DOC_STRUCTURE_LINE_LIST_MIN_WIDTH });
		} else if (event.key === 'End') {
			event.preventDefault();
			applyDocStructureSettings({ lineListWidth: DOC_STRUCTURE_LINE_LIST_MAX_WIDTH });
		}
	}

	const LINE_TYPE_OPTIONS = [
		'paragraph', 'heading-1', 'heading-2', 'heading-3', 'heading-4', 'heading-5',
		'toc', 'image', 'table', 'table-caption', 'figure', 'figure-caption',
		'header', 'footer', 'list', 'list-item', 'code', 'doc-title', 'unknown'
	];

	function startLineEdit(line: DocStructureLine) {
		editingLineKey = lineKey(line);
		editingCorrectedType = line.line_type || '';
		editingContent = line.content || '';
		editingOriginalType = line.line_type || '';
		editingOriginalContent = line.content || '';
		editingError = '';
	}

	function cancelLineEdit() {
		if (editingSaving) return;
		editingLineKey = null;
		editingCorrectedType = '';
		editingContent = '';
		editingOriginalType = '';
		editingOriginalContent = '';
		editingError = '';
	}

	async function saveLineEdit() {
		if (!editingLineKey || editingSaving) return;
		const recordId = Number(currentInput?.id ?? 0);
		if (!recordId || recordId <= 0) {
			editingError = 'No record loaded.';
			return;
		}
		const line = lines.find((ln) => lineKey(ln) === editingLineKey);
		if (!line) {
			editingError = 'Line not found.';
			return;
		}
		const correctedType = editingCorrectedType.trim();
		if (!correctedType) {
			editingError = 'Line type cannot be empty.';
			return;
		}
		editingSaving = true;
		editingError = '';
		try {
			const res = await updateKbDocStructureLine({
				input_record_id: recordId,
				page_number: line.page_number,
				line_number: line.line_number,
				corrected_line_type: correctedType,
				content: editingContent
			});
			lines = res.lines ?? [];
			editingLineKey = null;
		} catch (err) {
			editingError = err instanceof Error ? err.message : 'Failed to save.';
		} finally {
			editingSaving = false;
		}
	}

	let deletingLineKey = $state<string | null>(null);
	let deleteConfirmLine = $state<DocStructureLine | null>(null);

	function requestDeleteConfirm(line: DocStructureLine, e: MouseEvent) {
		e.stopPropagation();
		deleteConfirmLine = line;
	}

	function cancelDeleteConfirm() {
		deleteConfirmLine = null;
	}

	async function confirmDeleteLine() {
		const line = deleteConfirmLine;
		if (!line) return;
		const key = lineKey(line);
		deleteConfirmLine = null;
		if (deletingLineKey === key) return;
		const recordId = Number(currentInput?.id ?? 0);
		if (!recordId || recordId <= 0) return;
		deletingLineKey = key;
		try {
			const res = await deleteKbDocStructureLine({
				input_record_id: recordId,
				page_number: line.page_number,
				line_number: line.line_number
			});
			lines = res.lines ?? [];
			if (selectedLineKey === key) selectedLineKey = null;
		} catch (err) {
			console.error('Failed to delete line', err);
		} finally {
			deletingLineKey = null;
		}
	}

	function openSidebarTypeEdit() {
		sidebarTypeEditing = true;
		sidebarTypeError = '';
	}

	function closeSidebarTypeEdit() {
		sidebarTypeEditing = false;
		sidebarTypeError = '';
	}

	async function saveSidebarLineType(newType: string) {
		if (!selectedLine || !currentInput) return;
		sidebarTypeSaving = true;
		sidebarTypeError = '';
		try {
			const res = await updateKbDocStructureLine({
				input_record_id: currentInput.id,
				page_number: selectedLine.page_number,
				line_number: selectedLine.line_number,
				corrected_line_type: newType
			});
			lines = res.lines ?? [];
			sidebarTypeEditing = false;
		} catch (err) {
			sidebarTypeError = err instanceof Error ? err.message : 'Failed to save.';
		} finally {
			sidebarTypeSaving = false;
		}
	}

	function openSidebarContentEdit() {
		if (!selectedLine) return;
		sidebarContentDraftText = selectedLine.content || '';
		sidebarContentDialogOpen = true;
		sidebarContentError = '';
	}

	function closeSidebarContentEdit() {
		if (sidebarContentSaving) return;
		sidebarContentDialogOpen = false;
		sidebarContentDraftText = '';
		sidebarContentError = '';
	}

	async function saveSidebarContent() {
		if (!selectedLine || !currentInput || sidebarContentSaving) return;
		const paragraphs = sidebarContentDraftText
			.split('\n')
			.map((s) => s.trim())
			.filter((s) => s.length > 0);
		if (paragraphs.length === 0) {
			sidebarContentError = 'Content cannot be empty.';
			return;
		}
		sidebarContentSaving = true;
		sidebarContentError = '';
		try {
			const res =
				paragraphs.length === 1
					? await updateKbDocStructureLine({
							input_record_id: currentInput.id,
							page_number: selectedLine.page_number,
							line_number: selectedLine.line_number,
							content: paragraphs[0]
						})
					: await splitKbDocStructureLines({
							input_record_id: currentInput.id,
							page_number: selectedLine.page_number,
							line_number: selectedLine.line_number,
							contents: paragraphs,
							line_type: selectedLine.line_type
						});
			lines = res.lines ?? [];
			sidebarContentDialogOpen = false;
		} catch (err) {
			sidebarContentError = err instanceof Error ? err.message : 'Failed to save.';
		} finally {
			sidebarContentSaving = false;
		}
	}

	function recordDisplayName(r: KbInputRecord): string {
		return r.title?.trim() || r.name?.trim() || r.file_name?.trim() || `Input #${r.id}`;
	}

	function recordDisplayDocNo(r: KbInputRecord): string {
		return r.doc_no?.trim() || '—';
	}

	function handleLineFilterChange(event: Event) {
		lineFilter = (event.currentTarget as HTMLSelectElement).value as DocStructureFilterValue;
	}

	function mapBrowserRecord(record: KbInputRecord) {
		return {
			id: record.id,
			title: recordDisplayName(record),
			subtitle: record.file_name?.trim() || record.name?.trim() || '—',
			meta: [recordDisplayDocNo(record), record.parser_name?.trim() || '—'],
			status: record.type?.trim() || '—',
			description: currentInput?.id === record.id && correctedFile ? correctedFile : 'Select a record to inspect structure lines.',
			badges: [record.type?.trim() || '—']
		};
	}

	onMount(() => {
		docStructureSettings = readDocStructureSettings(
			localStorage,
			getDocStructureSettingsUserId()
		) as DocStructureSettings;
		settingsHydrated = true;

		return () => {
			lineListResizing = false;
			document.body.style.cursor = '';
			document.body.style.userSelect = '';
		};
	});

	$effect(() => {
		if (typeof window === 'undefined' || !settingsHydrated) return;
		writeDocStructureSettings(localStorage, getDocStructureSettingsUserId(), docStructureSettings);
	});

	$effect(() => {
		// Cancel sidebar editing when the selected line changes
		void selectedLineKey;
		sidebarTypeEditing = false;
		sidebarTypeError = '';
	});

	$effect(() => {
		const visibleSelected =
			selectedLineKey != null &&
			filteredLines.some((line) => lineKey(line) === selectedLineKey);
		if (visibleSelected) return;
		const nextLine = filteredLines[0] ?? null;
		if (!nextLine) {
			selectedLineKey = null;
			selectedHighlightTarget = null;
			return;
		}
		void selectLine(nextLine);
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
		--line-record-bg:{recordBackground || DOC_STRUCTURE_RECORD_DEFAULT_BACKGROUND};
		--line-record-h:{recordHeight}px;
		--line-record-gap:{recordGap}px;
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
		<KbInputRecordBrowser
			{darkMode}
			instanceKey="doc-structure-record-browser"
			title="kb.inputs"
			subtitle="Search, filter, and select input records before inspecting corrected structure lines."
			emptyTitle="No records yet"
			emptySubtitle="Use Search or Retrieve to browse kb.inputs."
			selectedRecordId={currentInput?.id ?? null}
			mapRecord={mapBrowserRecord}
			onSelect={(record) => void loadStructureForRecord(record.id)}
			onError={(error) => {
				errorMsg = error.message;
			}}
		/>

		<aside class="structure-sidebar" style={`width:${lineListWidth}px;`}>
			<div class="left-meta">
				<div class="left-meta-copy">
					<div class="left-meta-title">Lines</div>
					<div class="left-meta-count">
						{filteredLines.length} shown / {lines.length} total · {getDocStructureFilterLabel(lineFilter)}
					</div>
				</div>
				<div class="left-meta-actions">
					<label class="filter-field">
						<span class="sr-only">Filter lines by type</span>
						<select class="filter-select" value={lineFilter} onchange={handleLineFilterChange}>
							{#each DOC_STRUCTURE_FILTER_OPTIONS as option (option.value)}
								<option value={option.value}>{option.label}</option>
							{/each}
						</select>
					</label>
					<button class="btn btn-ghost settings-btn" type="button" onclick={openLineListSettings}>
						<SettingsIcon style="width:14px; height:14px;" />
						Settings
					</button>
				</div>
			</div>

			<div class="line-list">
				{#if errorMsg}
					<div class="error">{errorMsg}</div>
				{:else if !loading && filteredLines.length === 0}
					<div class="empty">
						<div class="empty-title">No lines loaded</div>
						<div class="empty-sub">
							Select a record to load a `.txt` file.
							{#if lineFilter !== 'all'}
								No {getDocStructureFilterLabel(lineFilter).toLowerCase()} found in current result.
							{/if}
						</div>
					</div>
				{:else}
					<div class="line-list-head" aria-hidden="true">
						<span>Line</span>
						<span>Line Type</span>
						<span>Content</span>
					</div>
					{#each filteredLines as line (`${line.page_number}-${line.line_number}`)}
						{#if editingLineKey === lineKey(line)}
							<div
								class="line-card line-card-editing"
								class:selected={selectedLineKey === lineKey(line)}
							>
								<form
									class="line-edit-form"
									onsubmit={(e) => {
										e.preventDefault();
										saveLineEdit();
									}}
								>
									<div class="line-edit-field">
										<span class="line-edit-label">Line Type</span>
										<div class="line-type-edit-row">
											<input
												class="line-edit-input"
												type="text"
												bind:value={editingCorrectedType}
												onkeydown={(e) => {
													if (e.key === 'Escape') cancelLineEdit();
												}}
											/>
											<select
												class="line-edit-select"
												value={editingCorrectedType}
												onchange={(e) => {
													editingCorrectedType = (e.currentTarget as HTMLSelectElement).value;
												}}
											>
												<option value="" disabled>pick type</option>
												{#each LINE_TYPE_OPTIONS as opt}
													<option value={opt}>{opt}</option>
												{/each}
											</select>
										</div>
									</div>
									<div class="line-edit-field">
										<span class="line-edit-label">Content</span>
										<input
											class="line-edit-input"
											type="text"
											bind:value={editingContent}
											onkeydown={(e) => {
												if (e.key === 'Escape') cancelLineEdit();
											}}
										/>
									</div>
									{#if editingError}<div class="line-edit-error">{editingError}</div>{/if}
									<div class="line-edit-actions">
										<button
											class="btn btn-primary line-edit-action-btn"
											type="submit"
											disabled={editingSaving || !editingHasChanges}
										>
											{editingSaving ? 'Saving…' : 'Save'}
										</button>
										<button
											class="btn btn-ghost line-edit-action-btn"
											type="button"
											onclick={cancelLineEdit}
											disabled={editingSaving}
										>Cancel</button>
									</div>
								</form>
							</div>
						{:else}
							<div
								class="line-card"
								class:selected={selectedLineKey === lineKey(line)}
								role="button"
								tabindex="0"
								onclick={() => selectLine(line)}
								onkeydown={(e) => {
									if (e.key === 'Enter' || e.key === ' ') {
										e.preventDefault();
										selectLine(line);
									}
								}}
								title={`Page ${line.page_number}, line ${line.line_number}`}
							>
								<span class="line-number-cell">L{line.line_number}</span>
								<span class="line-type-cell">{displayLineType(line)}</span>
								<span class="line-content-cell">{line.content || '—'}</span>
								<div class="line-actions">
									<button
										type="button"
										class="line-edit-btn"
										title="Edit line"
										aria-label={`Edit line ${line.line_number}`}
										onclick={(e) => {
											e.stopPropagation();
											startLineEdit(line);
										}}
									>✎</button>
									<button
										type="button"
										class="line-delete-btn"
										title="Delete line"
										aria-label={`Delete line ${line.line_number}`}
										disabled={deletingLineKey === lineKey(line)}
										onclick={(e) => requestDeleteConfirm(line, e)}
									><Trash2Icon class="line-action-icon" /></button>
								</div>
							</div>
						{/if}
					{/each}
				{/if}
			</div>
			<button
				type="button"
				class="line-list-resizer"
				class:active={lineListResizing}
				aria-label="Resize lines list"
				onpointerdown={startLineListResize}
				onkeydown={onLineListResizerKeydown}
			>
				<span class="line-list-resizer-grip" aria-hidden="true"></span>
			</button>
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
					<PdfViewWindow
						inputId={currentInput.id}
						{fileUrl}
						bind:page={docPage}
						bind:zoom={pdfZoom}
						bind:numPages={pdfNumPages}
						highlightVersion={selectedHighlightTarget
							? `${selectedHighlightTarget.page}:${selectedHighlightTarget.coords.join(',')}:${selectedHighlightTarget.version}`
							: `${selectedLineKey ?? ''}:${highlightSelectionVersion}`}
						renderHighlights={renderStructureHighlights}
						sidebarMinWidth={140}
						sidebarMaxWidth={420}
						sidebarDefaultWidth={270}
						sidebarTitle="Selected Line"
						sidebarSettingsKey="doc-structure-pdf-sidebar"
						sidebarWidthSettingLabel="Panel Width"
					>
						{#snippet sidebar()}
							{#if !selectedLine}
								<div class="meta-empty">Select a line from the left panel.</div>
							{:else}
								<div class="meta-row"><span>Page</span><strong>{selectedLine.page_number}</strong></div>
								<div class="meta-row"><span>Line</span><strong>{selectedLine.line_number}</strong></div>

								<!-- Line Type: hover shows edit icon, click opens inline dropdown -->
								<div class="meta-row sidebar-editable-row">
									<span>Line Type</span>
									{#if sidebarTypeEditing}
										<div
											class="sidebar-type-edit-wrap"
											onfocusout={(e) => {
												if (
													!sidebarTypeSaving &&
													!(e.currentTarget as HTMLElement).contains(e.relatedTarget as Node)
												) {
													closeSidebarTypeEdit();
												}
											}}
										>
											<select
												class="sidebar-type-select"
												value={selectedLine.line_type}
												disabled={sidebarTypeSaving}
												onchange={(e) => {
													const val = (e.currentTarget as HTMLSelectElement).value;
													if (val) void saveSidebarLineType(val);
												}}
											>
												{#each LINE_TYPE_OPTIONS as opt (opt)}
													<option value={opt}>{opt}</option>
												{/each}
											</select>
											{#if !sidebarTypeSaving}
												<button
													class="sidebar-type-cancel-btn"
													type="button"
													tabindex="0"
													onclick={closeSidebarTypeEdit}
												>✕</button>
											{/if}
											{#if sidebarTypeError}
												<div class="sidebar-edit-error">{sidebarTypeError}</div>
											{/if}
										</div>
									{:else}
										<div class="sidebar-val-group">
											<strong>{selectedLine.line_type || '—'}</strong>
											<button
												class="sidebar-edit-icon-btn"
												type="button"
												title="Edit line type"
												onclick={openSidebarTypeEdit}
											>✎</button>
										</div>
									{/if}
								</div>

								<!-- Content: hover shows edit icon, click opens dialog -->
								<div class="meta-row sidebar-editable-row sidebar-content-row">
									<div class="sidebar-content-label-row">
										<span>Content</span>
										<button
											class="sidebar-edit-icon-btn"
											type="button"
											title="Edit content"
											onclick={openSidebarContentEdit}
										>✎</button>
									</div>
									<strong class="sidebar-content-full">{selectedLine.content || '—'}</strong>
								</div>

								<div class="meta-row mono">
									<span>Coordinate</span>[{selectedLine.coords.map((n) => Math.trunc(n)).join(', ')}]
								</div>
							{/if}
							{#if correctedFile}
								<div class="meta-title corrected-title">File</div>
								<div class="meta-file">{correctedFile.replace('/Users/cding/Apps/SemOS/', '')}</div>
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

{#if sidebarContentDialogOpen}
	<div
		class="dialog-overlay"
		aria-hidden="true"
		onclick={(e) => { if (e.target === e.currentTarget) closeSidebarContentEdit(); }}
		onkeydown={(e) => { if (e.key === 'Escape') closeSidebarContentEdit(); }}
	>
		<div
			class="dialog content-edit-dialog"
			style="background:{panelBg}; border-color:{inkLine};"
			onclick={(e) => e.stopPropagation()}
			onkeydown={(e) => { if (e.key === 'Escape') closeSidebarContentEdit(); e.stopPropagation(); }}
			role="dialog"
			aria-modal="true"
			aria-label="Edit content"
			tabindex="0"
		>
			<div class="dialog-head">
				<div>
					<div class="dialog-eyebrow">Edit</div>
					<h2 class="dialog-title">Edit Content</h2>
					<p class="dialog-subtitle">
						Each non-empty line becomes a separate document line. Use newlines to split into multiple paragraphs — all will share the same coordinates and line type.
					</p>
				</div>
			</div>

			<div class="dialog-scroll">
				<div class="content-edit-body">
					<textarea
						class="content-edit-textarea"
						bind:value={sidebarContentDraftText}
						disabled={sidebarContentSaving}
						rows="10"
						placeholder="Enter content…"
						spellcheck="false"
					></textarea>
					{#if sidebarContentError}
						<div class="line-edit-error content-edit-error">{sidebarContentError}</div>
					{/if}
				</div>
			</div>

			<div class="dialog-foot">
				<div class="dialog-foot-hint">
					{sidebarContentParagraphCount} line{sidebarContentParagraphCount === 1 ? '' : 's'} after save
				</div>
				<div class="dialog-foot-buttons">
					<button
						class="btn btn-ghost"
						type="button"
						disabled={sidebarContentSaving}
						onclick={closeSidebarContentEdit}
					>Cancel</button>
					<button
						class="btn btn-primary dialog-select-btn"
						type="button"
						disabled={sidebarContentSaving || sidebarContentParagraphCount === 0}
						onclick={() => void saveSidebarContent()}
					>{sidebarContentSaving ? 'Saving…' : 'Save'}</button>
				</div>
			</div>
		</div>
	</div>
{/if}

{#if lineListSettingsOpen}
	<div
		class="dialog-overlay"
		aria-hidden="true"
		onclick={closeLineListSettings}
		onkeydown={(e) => {
			if (e.key === 'Escape') closeLineListSettings();
		}}
	>
		<div
			class="dialog settings-dialog"
			style="background:{panelBg}; border-color:{inkLine};"
			onclick={(e) => e.stopPropagation()}
			onkeydown={(e) => {
				if (e.key === 'Escape') closeLineListSettings();
				e.stopPropagation();
			}}
			role="dialog"
			aria-modal="true"
			tabindex="0"
		>
			<div class="dialog-head">
				<div>
					<div class="dialog-eyebrow">Preferences</div>
					<h2 class="dialog-title">Lines View Settings</h2>
					<p class="dialog-subtitle">
						Adjust the list card color and spacing. These preferences are saved automatically.
					</p>
				</div>
			</div>

			<div class="dialog-scroll">
				<div class="dialog-controls">
					<div class="dialog-section">
						<div class="dialog-section-head">
							<div class="dialog-section-title">Appearance</div>
							<div class="dialog-section-copy">Tune the record rows in the lines list.</div>
						</div>
						<div class="settings-grid">
							<label class="field dialog-field settings-field">
								<span class="field-label">Record background color</span>
								<div class="settings-color-row">
									<input
										class="settings-color-input"
										type="color"
										value={recordBackground}
										oninput={(e) =>
											applyDocStructureSettings({
												recordBackground: (e.currentTarget as HTMLInputElement).value
											})}
									/>
								</div>
							</label>
							<label class="field dialog-field settings-field settings-field-wide">
								<span class="field-label">Record height ({recordHeight}px)</span>
								<div class="settings-width-row">
									<span class="settings-width-bound">{DOC_STRUCTURE_RECORD_MIN_HEIGHT}</span>
									<input
										type="range"
										min={DOC_STRUCTURE_RECORD_MIN_HEIGHT}
										max={DOC_STRUCTURE_RECORD_MAX_HEIGHT}
										step="2"
										value={recordHeight}
										oninput={(e) =>
											applyDocStructureSettings({
												recordHeight: Number((e.currentTarget as HTMLInputElement).value)
											})}
										class="settings-width-slider"
									/>
									<span class="settings-width-bound">{DOC_STRUCTURE_RECORD_MAX_HEIGHT}</span>
								</div>
							</label>
							<label class="field dialog-field settings-field settings-field-wide">
								<span class="field-label">Gap between records ({recordGap}px)</span>
								<div class="settings-width-row">
									<span class="settings-width-bound">{DOC_STRUCTURE_RECORD_GAP_MIN}</span>
									<input
										type="range"
										min={DOC_STRUCTURE_RECORD_GAP_MIN}
										max={DOC_STRUCTURE_RECORD_GAP_MAX}
										step="1"
										value={recordGap}
										oninput={(e) =>
											applyDocStructureSettings({
												recordGap: Number((e.currentTarget as HTMLInputElement).value)
											})}
										class="settings-width-slider"
									/>
									<span class="settings-width-bound">{DOC_STRUCTURE_RECORD_GAP_MAX}</span>
								</div>
							</label>
							<label class="field dialog-field settings-field settings-field-wide">
								<span class="field-label">List width ({lineListWidth}px)</span>
								<div class="settings-width-row">
									<span class="settings-width-bound">{DOC_STRUCTURE_LINE_LIST_MIN_WIDTH}</span>
									<input
										type="range"
										min={DOC_STRUCTURE_LINE_LIST_MIN_WIDTH}
										max={DOC_STRUCTURE_LINE_LIST_MAX_WIDTH}
										step="4"
										value={lineListWidth}
										oninput={(e) =>
											applyDocStructureSettings({
												lineListWidth: Number((e.currentTarget as HTMLInputElement).value)
											})}
										class="settings-width-slider"
									/>
									<span class="settings-width-bound">{DOC_STRUCTURE_LINE_LIST_MAX_WIDTH}</span>
								</div>
								<div class="dialog-section-copy" style="margin-top:6px;">
									You can also drag the divider beside the list to resize it.
								</div>
							</label>
						</div>
					</div>
				</div>
			</div>

			<div class="dialog-foot">
				<div class="dialog-foot-hint">Updates are saved automatically.</div>
				<div class="dialog-foot-buttons">
					<button class="btn btn-primary dialog-select-btn" type="button" onclick={resetLineListSettings}>
						Reset
					</button>
					<button class="btn btn-primary dialog-select-btn" type="button" onclick={closeLineListSettings}>
						Close
					</button>
				</div>
			</div>
		</div>
	</div>
{/if}

{#if deleteConfirmLine}
	<div
		class="dialog-overlay"
		aria-hidden="true"
		onclick={cancelDeleteConfirm}
		onkeydown={(e) => { if (e.key === 'Escape') cancelDeleteConfirm(); }}
	>
		<div
			class="delete-dialog"
			style="background:{panelBg}; border-color:{inkLine};"
			onclick={(e) => e.stopPropagation()}
			onkeydown={(e) => { if (e.key === 'Escape') cancelDeleteConfirm(); e.stopPropagation(); }}
			role="dialog"
			aria-modal="true"
			tabindex="0"
		>
			<div class="delete-dialog-icon">⚠</div>
			<div class="delete-dialog-title">Delete Line?</div>
			<div class="delete-dialog-body">
				<span class="delete-dialog-type">{deleteConfirmLine.line_type}</span>
				<span class="delete-dialog-content">{deleteConfirmLine.content || '(no content)'}</span>
			</div>
			<p class="delete-dialog-sub">
				Page {deleteConfirmLine.page_number}, line {deleteConfirmLine.line_number}.
				This will remove it from the <code>.txt</code> file and record it in <code>.manual</code>.
			</p>
			<div class="delete-dialog-actions">
				<button class="btn btn-ghost" type="button" onclick={cancelDeleteConfirm}>Cancel</button>
				<button class="btn delete-confirm-btn" type="button" onclick={confirmDeleteLine}>Delete</button>
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
		grid-template-columns: auto auto 1fr;
		min-height: 0;
	}
	.structure-sidebar {
		position: relative;
		border-right: 1px solid var(--ink-line);
		background: var(--panel-bg);
		display: grid;
		grid-template-rows: auto auto 1fr;
		min-height: 0;
		min-width: 280px;
		max-width: 760px;
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
		align-items: center;
		gap: 12px;
		padding: 10px 14px;
		border-bottom: 1px solid var(--ink-line-soft);
		color: var(--text-secondary);
		font-size: 12px;
		text-transform: uppercase;
		letter-spacing: 0.08em;
	}
	.left-meta-copy {
		min-width: 0;
	}
	.left-meta-actions {
		display: flex;
		align-items: center;
		gap: 8px;
		flex: 0 0 auto;
	}
	.filter-field {
		display: flex;
		align-items: center;
	}
	.filter-select {
		height: 34px;
		padding: 0 34px 0 10px;
		border: 1px solid var(--ink-line);
		border-radius: 10px;
		background:
			linear-gradient(180deg, rgba(255,255,255,0.04), rgba(255,255,255,0.01)),
			var(--panel-bg-alt);
		color: var(--text-primary);
		font-size: 12px;
		font-weight: 600;
		letter-spacing: 0.02em;
		cursor: pointer;
		appearance: none;
		background-image:
			linear-gradient(45deg, transparent 50%, var(--text-secondary) 50%),
			linear-gradient(135deg, var(--text-secondary) 50%, transparent 50%);
		background-position:
			calc(100% - 16px) 14px,
			calc(100% - 11px) 14px;
		background-size: 5px 5px, 5px 5px;
		background-repeat: no-repeat;
	}
	.filter-select:focus {
		outline: none;
		border-color: var(--brass);
		box-shadow:
			0 0 0 1px var(--brass-faint),
			0 0 0 4px rgba(212, 162, 76, 0.08);
	}
	.settings-btn {
		flex: 0 0 auto;
		height: 34px;
		padding: 0 10px;
		font-size: 12px;
	}
	.sr-only {
		position: absolute;
		width: 1px;
		height: 1px;
		padding: 0;
		margin: -1px;
		overflow: hidden;
		clip: rect(0, 0, 0, 0);
		white-space: nowrap;
		border: 0;
	}
	.line-list {
		overflow: auto;
		padding: 10px;
		display: flex;
		flex-direction: column;
		gap: var(--line-record-gap);
	}
	.line-list-head {
		display: grid;
		grid-template-columns: 56px minmax(110px, 132px) minmax(0, 1fr);
		gap: 12px;
		padding: 0 10px 6px;
		font-size: 10px;
		letter-spacing: 0.12em;
		text-transform: uppercase;
		color: var(--text-muted);
	}
	.line-card {
		display: grid;
		grid-template-columns: 56px minmax(110px, 132px) minmax(0, 1fr) auto;
		align-items: center;
		gap: 8px;
		border: 1px solid var(--ink-line-soft);
		background: var(--line-record-bg);
		border-radius: 12px;
		min-height: var(--line-record-h);
		padding: 0 6px 0 10px;
		text-align: left;
		cursor: pointer;
		color: inherit;
	}
	.line-actions {
		display: flex;
		gap: 2px;
		align-items: center;
		flex-shrink: 0;
	}
	.line-card.selected {
		border-color: var(--brass);
		box-shadow: 0 0 0 1px var(--brass-faint) inset;
	}
	.line-card-editing {
		display: block;
		min-height: auto;
		padding: 10px;
		cursor: default;
	}
	.line-edit-btn,
	.line-delete-btn {
		width: 22px;
		height: 22px;
		padding: 0;
		border: 1px solid transparent;
		background: transparent;
		color: var(--text-muted);
		border-radius: 6px;
		font-size: 13px;
		cursor: pointer;
		display: flex;
		align-items: center;
		justify-content: center;
		opacity: 0;
		transition: opacity 0.12s, color 0.12s;
		flex-shrink: 0;
	}
	.line-card:hover .line-edit-btn,
	.line-card:hover .line-delete-btn {
		opacity: 1;
	}
	.line-action-icon {
		width: 13px;
		height: 13px;
	}
	.line-edit-btn:hover {
		color: var(--brass);
		border-color: var(--ink-line);
		background: var(--panel-bg-alt);
	}
	.line-delete-btn:hover {
		color: #c8553d;
		border-color: rgba(200, 85, 61, 0.4);
		background: rgba(200, 85, 61, 0.1);
	}
	.line-delete-btn:disabled {
		opacity: 0.4;
		cursor: default;
	}
	.delete-dialog {
		width: 100%;
		max-width: 420px;
		border: 1px solid;
		border-radius: 20px;
		padding: 28px 28px 22px;
		display: flex;
		flex-direction: column;
		gap: 10px;
		box-shadow: 0 24px 60px rgba(0,0,0,0.55);
	}
	.delete-dialog-icon {
		font-size: 28px;
		color: #c8553d;
		line-height: 1;
	}
	.delete-dialog-title {
		font-family: var(--font-serif);
		font-size: 22px;
		font-weight: 600;
		color: var(--text-primary);
	}
	.delete-dialog-body {
		display: flex;
		align-items: baseline;
		gap: 10px;
		flex-wrap: wrap;
	}
	.delete-dialog-type {
		font-family: var(--font-mono);
		font-size: 11px;
		text-transform: uppercase;
		letter-spacing: 0.08em;
		color: var(--brass);
		flex-shrink: 0;
	}
	.delete-dialog-content {
		font-size: 13px;
		color: var(--text-secondary);
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
		max-width: 280px;
	}
	.delete-dialog-sub {
		font-size: 12px;
		color: var(--text-muted);
		line-height: 1.5;
		margin: 0;
	}
	.delete-dialog-sub code {
		font-family: var(--font-mono);
		color: var(--text-secondary);
	}
	.delete-dialog-actions {
		display: flex;
		justify-content: flex-end;
		gap: 10px;
		margin-top: 6px;
	}
	.delete-confirm-btn {
		background: #c8553d !important;
		color: #fff !important;
		border: 1px solid rgba(200,85,61,0.6) !important;
		padding: 0 20px;
	}
	.delete-confirm-btn:hover {
		background: #d9654b !important;
	}
	.line-edit-form {
		display: flex;
		flex-direction: column;
		gap: 7px;
	}
	.line-edit-field {
		display: flex;
		flex-direction: column;
		gap: 3px;
	}
	.line-edit-label {
		font-size: 10px;
		text-transform: uppercase;
		letter-spacing: 0.1em;
		color: var(--text-muted);
	}
	.line-type-edit-row {
		display: grid;
		grid-template-columns: 1fr 1fr;
		gap: 6px;
	}
	.line-edit-input {
		height: 30px;
		padding: 0 8px;
		border: 1px solid var(--ink-line);
		background: var(--panel-bg-alt);
		color: var(--text-primary);
		border-radius: 8px;
		font-size: 12px;
		width: 100%;
		box-sizing: border-box;
	}
	.line-edit-input:focus {
		outline: none;
		border-color: var(--brass);
	}
	.line-edit-select {
		height: 30px;
		padding: 0 6px;
		border: 1px solid var(--ink-line);
		background: var(--panel-bg-alt);
		color: var(--text-primary);
		border-radius: 8px;
		font-size: 12px;
		width: 100%;
		box-sizing: border-box;
		cursor: pointer;
	}
	.line-edit-select:focus {
		outline: none;
		border-color: var(--brass);
	}
	.line-edit-error {
		font-size: 11px;
		color: #c8553d;
	}
	.line-edit-actions {
		display: flex;
		gap: 6px;
		margin-top: 2px;
	}
	.line-edit-action-btn {
		height: 28px;
		padding: 0 10px;
		font-size: 12px;
		flex: 1;
	}
	.line-number-cell {
		font-family: var(--font-mono);
		font-size: 11px;
		color: var(--text-secondary);
		white-space: nowrap;
	}
	.line-type-cell {
		font-family: var(--font-mono);
		font-size: 11px;
		text-transform: uppercase;
		letter-spacing: 0.06em;
		color: var(--brass);
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
	}
	.line-content-cell {
		font-size: 13px;
		line-height: 1.3;
		color: var(--text-primary);
		min-width: 0;
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
	}
	.settings-grid {
		display: grid;
		grid-template-columns: repeat(2, minmax(0, 1fr));
		gap: 10px;
	}
	.settings-field {
		display: flex;
		flex-direction: column;
		gap: 10px;
	}
	.settings-field-wide {
		grid-column: span 2;
	}
	.settings-color-row {
		display: flex;
		align-items: center;
	}
	.settings-color-input {
		width: 100%;
		height: 42px;
		padding: 4px;
		cursor: pointer;
	}
	.settings-width-row {
		display: grid;
		grid-template-columns: auto minmax(0, 1fr) auto;
		align-items: center;
		gap: 10px;
	}
	.settings-width-bound {
		font-family: var(--font-mono);
		font-size: 11px;
		color: var(--text-muted);
	}
	.settings-width-slider {
		width: 100%;
		height: 20px;
		padding: 0;
	}
	.line-list-resizer {
		position: absolute;
		top: 0;
		right: -8px;
		bottom: 0;
		display: flex;
		align-items: center;
		justify-content: center;
		width: 16px;
		padding: 0;
		border: 0;
		background: transparent;
		cursor: col-resize;
		user-select: none;
		touch-action: none;
		outline: none;
		z-index: 3;
	}
	.line-list-resizer::before {
		content: '';
		width: 1px;
		height: 100%;
		background: var(--ink-line);
		opacity: 0.9;
		transition: background 150ms ease;
	}
	.line-list-resizer:hover::before,
	.line-list-resizer.active::before,
	.line-list-resizer:focus-visible::before {
		background: var(--brass);
	}
	.line-list-resizer-grip {
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
	.line-list-resizer:hover .line-list-resizer-grip,
	.line-list-resizer.active .line-list-resizer-grip,
	.line-list-resizer:focus-visible .line-list-resizer-grip {
		border-color: var(--brass);
		background:
			radial-gradient(circle, var(--brass) 22%, transparent 24%) center 6px / 6px 12px repeat-y,
			var(--panel-bg);
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
	.settings-dialog {
		max-width: 760px;
		min-width: 640px;
		min-height: 0;
		resize: none;
	}
	@media (max-width: 1200px) {
		.body {
			grid-template-columns: 1fr;
		}
		.structure-sidebar {
			max-height: 44vh;
			border-right: none;
			border-bottom: 1px solid var(--ink-line);
		}
		.pdf-layout {
			grid-template-columns: 1fr;
		}
		.meta-panel {
			position: static;
		}
		.line-list-resizer {
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
		.settings-grid {
			grid-template-columns: 1fr;
		}
		.settings-field-wide {
			grid-column: auto;
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

	/* ── Sidebar editable rows ─────────────────────────────────────── */
	.sidebar-editable-row {
		position: relative;
	}
	.sidebar-val-group {
		display: flex;
		align-items: center;
		gap: 6px;
		min-width: 0;
		flex: 1 1 0;
		justify-content: flex-end;
	}
	.sidebar-content-row {
		flex-direction: column;
		align-items: stretch;
		gap: 4px;
	}
	.sidebar-content-label-row {
		display: flex;
		justify-content: space-between;
		align-items: center;
	}
	.sidebar-content-full {
		font-size: 13px;
		color: var(--text-primary);
		word-break: break-word;
		white-space: pre-wrap;
		line-height: 1.45;
	}
	.sidebar-edit-icon-btn {
		flex-shrink: 0;
		width: 20px;
		height: 20px;
		padding: 0;
		border: 1px solid transparent;
		background: transparent;
		color: var(--text-muted);
		border-radius: 5px;
		font-size: 12px;
		cursor: pointer;
		display: flex;
		align-items: center;
		justify-content: center;
		opacity: 0;
		transition: opacity 0.12s, color 0.12s;
	}
	.sidebar-editable-row:hover .sidebar-edit-icon-btn {
		opacity: 1;
	}
	.sidebar-edit-icon-btn:hover {
		color: var(--brass);
		border-color: var(--ink-line);
		background: var(--panel-bg-alt);
	}

	/* ── Sidebar inline type editor ───────────────────────────────── */
	.sidebar-type-edit-wrap {
		display: flex;
		align-items: center;
		gap: 5px;
		flex: 1 1 0;
		min-width: 0;
		justify-content: flex-end;
		flex-wrap: wrap;
	}
	.sidebar-type-select {
		height: 26px;
		padding: 0 6px;
		border: 1px solid var(--brass);
		border-radius: 7px;
		background: var(--panel-bg-alt);
		color: var(--text-primary);
		font-size: 11px;
		font-family: var(--font-mono);
		cursor: pointer;
		max-width: 130px;
	}
	.sidebar-type-select:focus {
		outline: none;
		box-shadow: 0 0 0 2px rgba(212, 162, 76, 0.22);
	}
	.sidebar-type-cancel-btn {
		width: 20px;
		height: 20px;
		padding: 0;
		border: 1px solid var(--ink-line);
		background: var(--panel-bg-alt);
		color: var(--text-muted);
		border-radius: 5px;
		font-size: 10px;
		cursor: pointer;
		display: flex;
		align-items: center;
		justify-content: center;
		flex-shrink: 0;
	}
	.sidebar-type-cancel-btn:hover {
		color: #c8553d;
		border-color: rgba(200, 85, 61, 0.4);
	}
	.sidebar-edit-error {
		font-size: 10px;
		color: #c8553d;
		width: 100%;
	}

	/* ── Content edit dialog ──────────────────────────────────────── */
	.content-edit-dialog {
		max-width: 640px;
		min-width: 380px;
		min-height: 0;
		resize: none;
	}
	.content-edit-body {
		padding: 16px 24px 12px;
		display: flex;
		flex-direction: column;
		gap: 8px;
	}
	.content-edit-textarea {
		width: 100%;
		box-sizing: border-box;
		padding: 10px 12px;
		border: 1px solid var(--ink-line);
		border-radius: 10px;
		background: var(--panel-bg-alt);
		color: var(--text-primary);
		font-size: 13px;
		font-family: var(--font-sans);
		line-height: 1.55;
		resize: vertical;
		min-height: 160px;
	}
	.content-edit-textarea:focus {
		outline: none;
		border-color: var(--brass);
		box-shadow: 0 0 0 2px rgba(212, 162, 76, 0.14);
	}
	.content-edit-textarea:disabled {
		opacity: 0.6;
		cursor: default;
	}
	.content-edit-error {
		margin-top: 2px;
	}
</style>
