	<script lang="ts">
	import { onMount, untrack, tick } from 'svelte';
	import SettingsIcon from '@lucide/svelte/icons/settings';
	import Trash2Icon from '@lucide/svelte/icons/trash-2';
	import SquarePenIcon from '@lucide/svelte/icons/square-pen';
	import CrosshairIcon from '@lucide/svelte/icons/crosshair';
	import ChevronLeftIcon from '@lucide/svelte/icons/chevron-left';
	import ChevronRightIcon from '@lucide/svelte/icons/chevron-right';
	import SearchIcon from '@lucide/svelte/icons/search';
	import {
		getKbDocStructure,
		getKbInput,
		updateKbDocStructureLine,
		deleteKbDocStructureLine,
		splitKbDocStructureLines,
		renumberKbDocStructureLines,
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
	import {
		buildDocStructureLineViews,
		docStructureLineKey
	} from './doc-structure-line-keys.js';
	import PdfViewWindow from '$lib/components/home3/pdf-view-window.svelte';

	let {
		darkMode = true,
		lockedRecordId = null
	}: { darkMode?: boolean; lockedRecordId?: number | null } = $props();

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
	// Extra source lines highlighted when a report finding spans multiple lines
	// (driven externally via focusSourceLines). Cleared on direct row clicks.
	let findingHighlightLines = $state<DocStructureLine[]>([]);
	// Reference to the Line List scroll container, for scroll-to-center.
	let lineListEl = $state<HTMLDivElement | null>(null);
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
	let lineSearch = $state('');
	// Find & Replace inside the inline content editor (self-contained per edit session).
	let editFind = $state('');
	let editReplace = $state('');
	let browserCollapsed = $state(false);

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

	// Edit Coordinates mode
	let editingCoordsMode = $state(false);
	let editCoordsDraft = $state<number[]>([]);
	let editCoordsSaving = $state(false);
	let editCoordsError = $state('');

	type PdfPageViewport = {
		width: number;
		height: number;
	};

	// MinerU bboxes are stored in a ~1000×1000 pixel image space (top-left origin, y↓).
	// Scale directly to viewport — no Y-flip needed.
	const MINERU_COORD_SIZE = 1000;

	function mineruToViewport(coords: number[], vp: PdfPageViewport): [number, number, number, number] {
		return [
			coords[0] * vp.width / MINERU_COORD_SIZE,
			coords[1] * vp.height / MINERU_COORD_SIZE,
			coords[2] * vp.width / MINERU_COORD_SIZE,
			coords[3] * vp.height / MINERU_COORD_SIZE,
		];
	}

	let isPdf = $derived((currentInput?.type ?? '').toLowerCase() === 'pdf');
	let fileUrl = $derived.by(() => {
		if (!currentInput) return '';
		return `/api/v1/kb/inputs/${currentInput.id}/file#page=${docPage}&zoom=page-width`;
	});

	function lineKey(line: DocStructureLine): string {
		return docStructureLineKey(line);
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

	function escapeHtml(s: string): string {
		return s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');
	}

	// Builds a matcher for the current search query.
	// Supports plain text (case-insensitive substring) and /pattern/flags (regex).
	function buildSearchMatcher(query: string): (text: string) => { matched: boolean; highlights: [number, number][] } {
		const q = query.trim();
		if (!q) return () => ({ matched: true, highlights: [] });
		const reLiteral = q.match(/^\/(.+?)\/([gimsuy]*)$/);
		if (reLiteral) {
			try {
				const flags = [...new Set([...(reLiteral[2] || ''), 'g'])].join('');
				const re = new RegExp(reLiteral[1], flags);
				return (text: string) => {
					re.lastIndex = 0;
					const highlights: [number, number][] = [];
					let m: RegExpExecArray | null;
					while ((m = re.exec(text)) !== null) {
						highlights.push([m.index, m.index + m[0].length]);
						if (m[0].length === 0) re.lastIndex++;
					}
					return { matched: highlights.length > 0, highlights };
				};
			} catch { /* invalid regex — fall through to substring */ }
		}
		const lower = q.toLowerCase();
		return (text: string) => {
			const highlights: [number, number][] = [];
			const textLow = text.toLowerCase();
			let idx = 0;
			while (idx < textLow.length) {
				const pos = textLow.indexOf(lower, idx);
				if (pos < 0) break;
				highlights.push([pos, pos + lower.length]);
				idx = pos + lower.length;
			}
			return { matched: highlights.length > 0, highlights };
		};
	}

	// Wraps every match of `query` within `text` in a <mark>, escaping the rest.
	// `markClass` lets the editor backdrop use a zero-width highlight (no padding)
	// so the overlay stays pixel-aligned with the textarea below it.
	function renderHighlightHtml(text: string, query: string, markClass = 'search-hl'): string {
		const q = (query || '').trim();
		if (!q) return escapeHtml(text);
		const { highlights } = buildSearchMatcher(q)(text);
		if (!highlights.length) return escapeHtml(text);
		let out = '';
		let last = 0;
		for (const [s, e] of highlights) {
			out += escapeHtml(text.slice(last, s));
			out += `<mark class="${markClass}">${escapeHtml(text.slice(s, e))}</mark>`;
			last = e;
		}
		return out + escapeHtml(text.slice(last));
	}

	function highlightContent(text: string): string {
		if (!text) return '—';
		return renderHighlightHtml(text, lineSearch);
	}

	// Backdrop HTML for the inline content editor (highlights the Find matches as
	// you type). A trailing newline needs an extra blank line so the backdrop's
	// height tracks the textarea exactly.
	let editContentHtml = $derived.by(() => {
		const html = renderHighlightHtml(editingContent ?? '', editFind, 'search-hl-overlay');
		return editingContent.endsWith('\n') ? html + '\n' : html;
	});

	// Refs for the content editor overlay (transparent textarea + highlight backdrop).
	let editTextareaEl = $state<HTMLTextAreaElement | null>(null);
	let editBackdropEl = $state<HTMLDivElement | null>(null);
	function syncBackdropScroll() {
		if (!editTextareaEl || !editBackdropEl) return;
		editBackdropEl.scrollTop = editTextareaEl.scrollTop;
		editBackdropEl.scrollLeft = editTextareaEl.scrollLeft;
	}

	let filteredLines = $derived.by(() => {
		const byType = filterDocStructureLines(lines, lineFilter);
		const q = lineSearch.trim();
		if (!q) return byType;
		const matcher = buildSearchMatcher(q);
		return byType.filter((ln) => matcher(ln.content || '').matched);
	});
	let filteredLineViews = $derived(buildDocStructureLineViews(filteredLines));

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

	// The kb.inputs record browser is shown only in the standalone Document
	// Structure page. When it is absent the body grid drops to two columns so the
	// PDF (right column) fills to the right edge instead of leaving the unused
	// `1fr` track empty.
	let showBrowser = $derived(!browserCollapsed && lockedRecordId == null);

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

		if (editingCoordsMode && editCoordsDraft.length >= 4) {
			// console.log('[coord-editor] renderStructureHighlights → entering coord-edit mode for page', pageNo);
			renderCoordEditor(viewport, overlay);
			return;
		}

		const drawMark = (coords: number[], label: string) => {
			if (!Array.isArray(coords) || coords.length < 4) return;
			const [vx1, vy1, vx2, vy2] = mineruToViewport(coords, viewport);
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
			mark.title = label;
			overlay.appendChild(mark);
		};

		if (target && target.page === pageNo) {
			drawMark(target.coords, target.label);
		}

		// Additional lines for multi-line report findings (e.g. location "120-121").
		for (const ln of findingHighlightLines) {
			if (ln.page_number !== pageNo) continue;
			if (lineKey(ln) === selectedLineKey) continue; // already drawn as primary target
			drawMark(ln.coords, `page ${ln.page_number}, line ${ln.line_number}`);
		}
	}

	function renderCoordEditor(viewport: PdfPageViewport, overlay: HTMLDivElement) {
		// console.log('[coord-editor] renderCoordEditor called, draft=', $state.snapshot(editCoordsDraft));

		const [vx1, vy1, vx2, vy2] = mineruToViewport(editCoordsDraft, viewport);

		// Raw coordinate boundaries (no visual padding)
		let vLeft = Math.min(vx1, vx2);
		let vTop = Math.min(vy1, vy2);
		let vRight = Math.max(vx1, vx2);
		let vBottom = Math.max(vy1, vy2);

		// Match the static highlight's visual padding
		const PAD_H = 5;
		const PAD_V = 4;
		const HS = 8; // handle size in px

		const rectEl = document.createElement('div');
		rectEl.style.cssText =
			'position:absolute;box-sizing:border-box;' +
			'background:rgba(212,162,76,0.3);' +
			'outline:1px solid rgba(212,162,76,0.85);' +
			'border-radius:2px;cursor:move;pointer-events:auto;';

		// 8 handles: 4 corners + 4 edge midpoints
		const hTL = makeHandle('nwse-resize');
		const hTC = makeHandle('n-resize');
		const hTR = makeHandle('nesw-resize');
		const hML = makeHandle('w-resize');
		const hMR = makeHandle('e-resize');
		const hBL = makeHandle('nesw-resize');
		const hBC = makeHandle('s-resize');
		const hBR = makeHandle('nwse-resize');

		const btnPanel = document.createElement('div');
		btnPanel.style.cssText = 'position:absolute;display:flex;gap:6px;pointer-events:none;z-index:3;';

		const saveBtn = document.createElement('button');
		saveBtn.type = 'button';
		saveBtn.textContent = 'Save';
		saveBtn.style.cssText =
			'height:26px;padding:0 12px;background:#d4a24c;color:#15110a;border:1px solid #e0b768;' +
			'border-radius:8px;font-size:12px;font-weight:600;cursor:pointer;pointer-events:auto;';

		const cancelBtn = document.createElement('button');
		cancelBtn.type = 'button';
		cancelBtn.textContent = 'Cancel';
		cancelBtn.style.cssText =
			'height:26px;padding:0 12px;background:rgba(255,255,255,0.08);color:#94a3b8;' +
			'border:1px solid rgba(148,163,184,0.2);border-radius:8px;font-size:12px;font-weight:600;' +
			'cursor:pointer;pointer-events:auto;';

		btnPanel.appendChild(saveBtn);
		btnPanel.appendChild(cancelBtn);

		function applyLayout() {
			const l = Math.min(vLeft, vRight) - PAD_H;
			const t = Math.min(vTop, vBottom) - PAD_V;
			const w = Math.abs(vRight - vLeft) + PAD_H * 2;
			const h = Math.abs(vBottom - vTop) + PAD_V * 2;
			const hs2 = HS / 2;

			rectEl.style.left = `${l}px`;
			rectEl.style.top = `${t}px`;
			rectEl.style.width = `${w}px`;
			rectEl.style.height = `${h}px`;

			hTL.style.left = `${l - hs2}px`; hTL.style.top = `${t - hs2}px`;
			hTC.style.left = `${l + w / 2 - hs2}px`; hTC.style.top = `${t - hs2}px`;
			hTR.style.left = `${l + w - hs2}px`; hTR.style.top = `${t - hs2}px`;
			hML.style.left = `${l - hs2}px`; hML.style.top = `${t + h / 2 - hs2}px`;
			hMR.style.left = `${l + w - hs2}px`; hMR.style.top = `${t + h / 2 - hs2}px`;
			hBL.style.left = `${l - hs2}px`; hBL.style.top = `${t + h - hs2}px`;
			hBC.style.left = `${l + w / 2 - hs2}px`; hBC.style.top = `${t + h - hs2}px`;
			hBR.style.left = `${l + w - hs2}px`; hBR.style.top = `${t + h - hs2}px`;

			btnPanel.style.left = `${l}px`;
			btnPanel.style.top = `${t + h + 8}px`;
		}

		function commitCoords() {
			const l = Math.min(vLeft, vRight);
			const t = Math.min(vTop, vBottom);
			const r = Math.max(vLeft, vRight);
			const b = Math.max(vTop, vBottom);
			// Convert viewport pixels back to MinerU coord space (inverse of mineruToViewport)
			editCoordsDraft = [
				Math.round(l * MINERU_COORD_SIZE / viewport.width),
				Math.round(t * MINERU_COORD_SIZE / viewport.height),
				Math.round(r * MINERU_COORD_SIZE / viewport.width),
				Math.round(b * MINERU_COORD_SIZE / viewport.height),
			];
			highlightSelectionVersion += 1;
			// console.log('[coord-editor] commitCoords, new MinerU coords=', editCoordsDraft);
		}

		function attachDrag(
			el: HTMLDivElement,
			applyDelta: (
				sv: { l: number; t: number; r: number; b: number },
				dx: number,
				dy: number
			) => void
		) {
			el.addEventListener('pointerdown', (e: PointerEvent) => {
				e.stopPropagation();
				e.preventDefault();
				el.setPointerCapture(e.pointerId);
				const sx = e.clientX;
				const sy = e.clientY;
				const sv = { l: vLeft, t: vTop, r: vRight, b: vBottom };
				const onMove = (me: PointerEvent) => {
					applyDelta(sv, me.clientX - sx, me.clientY - sy);
					applyLayout();
				};
				const onUp = () => {
					el.removeEventListener('pointermove', onMove);
					commitCoords();
				};
				el.addEventListener('pointermove', onMove);
				el.addEventListener('pointerup', onUp, { once: true });
			});
		}

		// Rect body: move all edges together
		attachDrag(rectEl, (sv, dx, dy) => {
			vLeft = sv.l + dx; vTop = sv.t + dy;
			vRight = sv.r + dx; vBottom = sv.b + dy;
		});
		// Corner handles
		attachDrag(hTL, (sv, dx, dy) => { vLeft = sv.l + dx; vTop = sv.t + dy; });
		attachDrag(hTR, (sv, dx, dy) => { vRight = sv.r + dx; vTop = sv.t + dy; });
		attachDrag(hBL, (sv, dx, dy) => { vLeft = sv.l + dx; vBottom = sv.b + dy; });
		attachDrag(hBR, (sv, dx, dy) => { vRight = sv.r + dx; vBottom = sv.b + dy; });
		// Edge midpoint handles
		attachDrag(hTC, (sv, _dx, dy) => { vTop = sv.t + dy; });
		attachDrag(hBC, (sv, _dx, dy) => { vBottom = sv.b + dy; });
		attachDrag(hML, (sv, dx, _dy) => { vLeft = sv.l + dx; });
		attachDrag(hMR, (sv, dx, _dy) => { vRight = sv.r + dx; });

		let savingInProgress = false;
		saveBtn.addEventListener('click', (e) => {
			e.stopPropagation();
			if (savingInProgress || editCoordsSaving) return;
			savingInProgress = true;
			saveBtn.disabled = true;
			saveBtn.textContent = 'Saving…';
			void saveEditCoords().finally(() => {
				savingInProgress = false;
				if (editCoordsError) {
					saveBtn.disabled = false;
					saveBtn.textContent = 'Save';
					window.alert(editCoordsError);
					editCoordsError = '';
				}
			});
		});

		cancelBtn.addEventListener('click', (e) => {
			e.stopPropagation();
			cancelEditCoords();
		});

		applyLayout();
		overlay.appendChild(rectEl);
		overlay.appendChild(hTL);
		overlay.appendChild(hTC);
		overlay.appendChild(hTR);
		overlay.appendChild(hML);
		overlay.appendChild(hMR);
		overlay.appendChild(hBL);
		overlay.appendChild(hBC);
		overlay.appendChild(hBR);
		overlay.appendChild(btnPanel);
	}

	function makeHandle(cursor: string): HTMLDivElement {
		const h = document.createElement('div');
		h.style.cssText =
			`position:absolute;width:${8}px;height:${8}px;` +
			`background:#fff;border:2px solid rgba(212,162,76,0.9);border-radius:2px;` +
			`box-shadow:0 1px 4px rgba(0,0,0,0.3);cursor:${cursor};pointer-events:auto;` +
			`z-index:2;box-sizing:border-box;`;
		return h;
	}

	function startEditCoords() {
		/*
		console.log('[coord-editor] startEditCoords called', {
			selectedLine: selectedLine ? { page: selectedLine.page_number, line: selectedLine.line_number, coords: selectedLine.coords } : null
		});
		*/
		if (!selectedLine) {
			console.warn('[coord-editor] no selectedLine');
			return;
		}
		if (!Array.isArray(selectedLine.coords) || selectedLine.coords.length < 4) {
			console.warn('[coord-editor] selectedLine.coords invalid:', selectedLine.coords);
			return;
		}
		editCoordsDraft = [...selectedLine.coords.slice(0, 4)];
		editingCoordsMode = true;
		editCoordsError = '';
		highlightSelectionVersion += 1;
		// console.log('[coord-editor] entered coord-edit mode, draft=', $state.snapshot(editCoordsDraft), 'version=', highlightSelectionVersion);
	}

	function cancelEditCoords() {
		editingCoordsMode = false;
		editCoordsDraft = [];
		editCoordsError = '';
		highlightSelectionVersion += 1;
	}

	async function saveEditCoords() {
		if (!selectedLine || !currentInput || editCoordsSaving || editCoordsDraft.length < 4) return;
		editCoordsSaving = true;
		editCoordsError = '';
		try {
			const res = await updateKbDocStructureLine({
				input_record_id: currentInput.id,
				page_number: selectedLine.page_number,
				line_number: selectedLine.line_number,
				coords: editCoordsDraft
			});
			lines = res.lines ?? [];
			editingCoordsMode = false;
			editCoordsDraft = [];
			const updatedLine = lines.find((l) => lineKey(l) === selectedLineKey);
			if (updatedLine) await selectLine(updatedLine);
		} catch (err) {
			editCoordsError = err instanceof Error ? err.message : 'Failed to save coordinates.';
		} finally {
			editCoordsSaving = false;
		}
	}

	async function loadStructureForRecord(id: number) {
		errorMsg = '';
		loading = true;
		lines = [];
		currentInput = null;
		selectedLineKey = null;
		highlightSelectionVersion = 0;
		selectedHighlightTarget = null;
		findingHighlightLines = [];
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

	async function selectLine(line: DocStructureLine, keepFindingHighlight = false) {
		selectedLineKey = lineKey(line);
		highlightSelectionVersion += 1;
		if (!keepFindingHighlight) {
			findingHighlightLines = [];
		}
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

	/**
	 * Externally-driven focus used by the Document Review report's left panel.
	 * Selects the first matching source line (driving the PDF page + highlight),
	 * highlights every matching line (for multi-line findings), and scrolls the
	 * Line List so the primary line is centered.
	 */
	export async function focusSourceLines(lineNumbers: number[]) {
		if (!Array.isArray(lineNumbers) || lineNumbers.length === 0) return;
		const wanted = new Set(lineNumbers);
		const matched = lines.filter((ln) => wanted.has(ln.line_number));
		if (matched.length === 0) return;
		const primary = matched[0];

		// If the primary line is hidden by the active filter, reset to "all".
		if (!filterDocStructureLines(lines, lineFilter).some((ln) => lineKey(ln) === lineKey(primary))) {
			lineFilter = 'all';
		}
		// If the primary line is hidden by the active search, clear it.
		if (lineSearch.trim() && !buildSearchMatcher(lineSearch)(primary.content || '').matched) {
			lineSearch = '';
		}

		await selectLine(primary, true);
		findingHighlightLines = matched;

		await tick();
		const target = lineListEl?.querySelector<HTMLElement>(
			`[data-line-key="${primary.page_number}-${primary.line_number}"]`
		);
		target?.scrollIntoView({ block: 'center', behavior: 'smooth' });
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
		// Seed the editor's Find with the active line search, if any.
		editFind = lineSearch;
		editReplace = '';
	}

	// Number of matches of the editor's Find pattern within the content being edited.
	let editMatchCount = $derived.by(() => {
		if (!editFind.trim() || !editingContent) return 0;
		return buildSearchMatcher(editFind)(editingContent).highlights.length;
	});

	function applyEditReplace() {
		if (!editFind.trim() || !editingContent) return;
		const { highlights } = buildSearchMatcher(editFind)(editingContent);
		if (!highlights.length) return;
		let result = editingContent;
		for (let i = highlights.length - 1; i >= 0; i--) {
			const [s, e] = highlights[i];
			result = result.slice(0, s) + editReplace + result.slice(e);
		}
		editingContent = result;
	}

	function cancelLineEdit() {
		if (editingSaving) return;
		editingLineKey = null;
		editingCorrectedType = '';
		editingContent = '';
		editingOriginalType = '';
		editingOriginalContent = '';
		editingError = '';
		editFind = '';
		editReplace = '';
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

	let renumbering = $state(false);
	let renumberError = $state('');
	let hasNewLines = $derived(lines.some((l) => l.line_number < 0));

	async function renumberLines() {
		if (!currentInput || renumbering) return;
		renumbering = true;
		renumberError = '';
		try {
			const res = await renumberKbDocStructureLines(currentInput.id);
			lines = res.lines ?? [];
		} catch (err) {
			renumberError = err instanceof Error ? err.message : 'Renumber failed.';
		} finally {
			renumbering = false;
		}
	}

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
			badges: [record.type?.trim() || '—']
		};
	}

	onMount(() => {
		docStructureSettings = readDocStructureSettings(
			localStorage,
			getDocStructureSettingsUserId()
		) as DocStructureSettings;
		settingsHydrated = true;

		// When locked to a specific document (e.g. embedded in the Document
		// Review report), hide the record browser and auto-load that record.
		if (lockedRecordId != null && lockedRecordId > 0) {
			browserCollapsed = true;
			void loadStructureForRecord(lockedRecordId);
		}

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
		untrack(() => {
			if (editingCoordsMode) {
				editingCoordsMode = false;
				editCoordsDraft = [];
				editCoordsError = '';
			}
		});
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

	<div class="body" class:no-browser={!showBrowser}>
		{#if showBrowser}
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
		{/if}

		<aside class="structure-sidebar" style={`width:${lineListWidth}px;`}>
			<div class="left-meta">
				{#if lockedRecordId == null}
					<button
						class="browser-collapse-btn"
						type="button"
						onclick={() => (browserCollapsed = !browserCollapsed)}
						title={browserCollapsed ? 'Expand records panel' : 'Collapse records panel'}
						aria-label={browserCollapsed ? 'Expand records panel' : 'Collapse records panel'}
					>
						{#if browserCollapsed}
							<ChevronRightIcon style="width:13px; height:13px;" />
						{:else}
							<ChevronLeftIcon style="width:13px; height:13px;" />
						{/if}
					</button>
				{/if}
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
					{#if hasNewLines}
						<button
							class="btn btn-renumber"
							type="button"
							disabled={renumbering}
							title="Assign sequential line numbers to all 'new' lines"
							onclick={() => void renumberLines()}
						>
							{renumbering ? '…' : 'Renumber'}
						</button>
					{/if}
					{#if renumberError}
						<span class="renumber-error" title={renumberError}>⚠</span>
					{/if}
					<button class="btn btn-ghost settings-btn" type="button" onclick={openLineListSettings}>
						<SettingsIcon style="width:14px; height:14px;" />
						Settings
					</button>
				</div>
			</div>

			<div class="search-bar">
				<div
					class="search-row"
					title="Search tips: plain text → case-insensitive substring · /pattern/ → regex · /pattern/i → case-insensitive regex · /pattern/g → global"
				>
					<SearchIcon style="width:13px;height:13px;flex-shrink:0;color:var(--text-muted);" />
					<input
						class="search-input"
						type="search"
						placeholder="Search lines…"
						bind:value={lineSearch}
						spellcheck="false"
						aria-label="Search line content"
					/>
					{#if lineSearch}
						<span class="search-count">{filteredLines.length} of {filterDocStructureLines(lines, lineFilter).length}</span>
						<button
							class="search-clear"
							type="button"
							onclick={() => (lineSearch = '')}
							aria-label="Clear search"
						>&times;</button>
					{/if}
				</div>
			</div>

			<div class="line-list" bind:this={lineListEl}>
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
					{#each filteredLineViews as { line, lineKey: viewLineKey, uiKey } (uiKey)}
						{#if editingLineKey === viewLineKey}
							<div
								class="line-card line-card-editing"
								class:selected={selectedLineKey === viewLineKey}
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
										<div class="content-edit-wrap">
											<div
												class="content-backdrop line-edit-textarea"
												bind:this={editBackdropEl}
												aria-hidden="true"
											>{@html editContentHtml}</div>
											<textarea
												class="line-edit-input line-edit-textarea content-edit-textarea"
												bind:value={editingContent}
												bind:this={editTextareaEl}
												onscroll={syncBackdropScroll}
												onkeydown={(e) => {
													if (e.key === 'Escape') cancelLineEdit();
												}}
											></textarea>
										</div>
									</div>
									<div class="line-edit-field">
										<span
											class="line-edit-label"
											title="Find supports plain text (case-insensitive) or /regex/flags. Replace fills every match; cleared when you Cancel."
										>Find &amp; Replace in content</span>
										<div class="find-replace-row">
											<input
												class="line-edit-input"
												type="text"
												placeholder="Find — text or /regex/"
												bind:value={editFind}
												spellcheck="false"
												onkeydown={(e) => {
													if (e.key === 'Escape') cancelLineEdit();
												}}
											/>
											<input
												class="line-edit-input"
												type="text"
												placeholder="Replace with"
												bind:value={editReplace}
												spellcheck="false"
												onkeydown={(e) => {
													if (e.key === 'Enter') {
														e.preventDefault();
														applyEditReplace();
													} else if (e.key === 'Escape') cancelLineEdit();
												}}
											/>
											<button
												class="btn btn-ghost line-edit-action-btn find-replace-btn"
												type="button"
												disabled={!editFind.trim() || editMatchCount === 0}
												title={editFind.trim() ? `Replace ${editMatchCount} match(es) of '${editFind}' with '${editReplace}'` : 'Enter a Find pattern to enable replace'}
												onclick={applyEditReplace}
											>Replace{editFind.trim() ? ` (${editMatchCount})` : ''}</button>
										</div>
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
								data-line-key={`${line.page_number}-${line.line_number}`}
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
								<span class="line-number-cell" class:line-new={line.line_number < 0}>
									{line.line_number < 0 ? 'new' : `L${line.line_number}`}
								</span>
								<span class="line-type-cell">{displayLineType(line)}</span>
								<span class="line-content-cell">
									{#if lineSearch.trim()}
										{@html highlightContent(line.content || '')}
									{:else}
										{line.content || '—'}
									{/if}
								</span>
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
						highlightVersion={editingCoordsMode && editCoordsDraft.length >= 4
							? `edit:${editCoordsDraft.join(',')}:${highlightSelectionVersion}`
							: selectedHighlightTarget
							? `${selectedHighlightTarget.page}:${selectedHighlightTarget.coords.join(',')}:${selectedHighlightTarget.version}:f${findingHighlightLines.length}`
							: `${selectedLineKey ?? ''}:${highlightSelectionVersion}:f${findingHighlightLines.length}`}
						renderHighlights={renderStructureHighlights}
						{darkMode}
						sidebarMinWidth={140}
						sidebarMaxWidth={420}
						sidebarDefaultWidth={270}
						sidebarTitle="Selected Line"
						sidebarSettingsKey="doc-structure-pdf-sidebar"
						sidebarWidthSettingLabel="Panel Width"
						showSidebar={lockedRecordId == null}
					>
						{#snippet toolbar()}
							<button
								type="button"
								class="pvw-tool-btn"
								disabled={!selectedLine}
								onclick={openSidebarTypeEdit}
								title="Edit Line Type"
							><SquarePenIcon class="pvw-tb-icon" /></button>
							<div class="pvw-tool-sep"></div>
							<button
								type="button"
								class="pvw-tool-btn"
								class:active={editingCoordsMode}
								disabled={!selectedLine}
								onclick={editingCoordsMode ? cancelEditCoords : startEditCoords}
								title={editingCoordsMode ? 'Cancel coordinate edit' : 'Edit Coordinates'}
							><CrosshairIcon class="pvw-tb-icon" /></button>
							<div class="pvw-tool-sep"></div>
							<button
								type="button"
								class="pvw-tool-btn"
								disabled={!selectedLine || !!deletingLineKey}
								onclick={(e) => { if (selectedLine) requestDeleteConfirm(selectedLine, e); }}
								title="Delete Line"
							><Trash2Icon class="pvw-tb-icon" /></button>
						{/snippet}
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
		grid-template-rows: minmax(0, 1fr);
		min-height: 0;
		overflow: hidden;
	}
	/* No record browser: line list + PDF only, PDF fills the remaining width. */
	.body.no-browser {
		grid-template-columns: auto 1fr;
	}
	.structure-sidebar {
		position: relative;
		border-right: 1px solid var(--ink-line);
		background: var(--panel-bg);
		display: grid;
		grid-template-rows: auto auto 1fr;
		min-height: 0;
		min-width: 280px;
		max-width: 960px;
	}
	.field-label {
		display: block;
		font-size: 12px;
		color: var(--text-muted);
		margin-bottom: 8px;
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
	.btn:disabled {
		opacity: 0.6;
		cursor: default;
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
	.browser-collapse-btn {
		flex: 0 0 auto;
		width: 26px;
		height: 26px;
		padding: 0;
		border: 1px solid var(--ink-line);
		border-radius: 7px;
		background: var(--panel-bg-alt);
		color: var(--text-muted);
		cursor: pointer;
		display: flex;
		align-items: center;
		justify-content: center;
		transition: color 0.12s, border-color 0.12s, background 0.12s;
	}
	.browser-collapse-btn:hover {
		color: var(--brass);
		border-color: var(--brass);
		background: var(--brass-faint);
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
	.btn-renumber {
		flex: 0 0 auto;
		height: 34px;
		padding: 0 10px;
		font-size: 12px;
		font-weight: 600;
		border: 1px solid var(--brass);
		border-radius: 10px;
		background: var(--brass-faint);
		color: var(--brass);
		cursor: pointer;
	}
	.btn-renumber:hover:not(:disabled) {
		background: var(--brass);
		color: #1d1508;
	}
	.btn-renumber:disabled {
		opacity: 0.6;
		cursor: default;
	}
	.renumber-error {
		color: #f87171;
		font-size: 14px;
		cursor: default;
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
	.search-bar {
		display: flex;
		flex-direction: column;
		gap: 5px;
		padding: 7px 10px;
		border-bottom: 1px solid var(--ink-line-soft);
		flex-shrink: 0;
	}
	.search-row {
		display: flex;
		align-items: center;
		gap: 6px;
		background: var(--panel-bg-alt);
		border: 1px solid var(--ink-line);
		border-radius: 8px;
		padding: 0 8px;
		height: 30px;
	}
	.search-row:focus-within {
		border-color: var(--brass);
	}
	.search-input {
		flex: 1;
		min-width: 0;
		border: none;
		background: transparent;
		color: var(--text-primary);
		font-size: 12px;
		padding: 0;
		outline: none;
		font-family: var(--font-sans);
	}
	.search-input::placeholder {
		color: var(--text-muted);
	}
	.search-input::-webkit-search-cancel-button {
		display: none;
	}
	.search-count {
		font-family: var(--font-mono);
		font-size: 10px;
		color: var(--text-muted);
		white-space: nowrap;
		flex-shrink: 0;
	}
	.search-clear {
		background: transparent;
		border: none;
		cursor: pointer;
		color: var(--text-muted);
		font-size: 15px;
		line-height: 1;
		padding: 0 1px;
		flex-shrink: 0;
	}
	.search-clear:hover {
		color: var(--text-primary);
	}
	:global(.search-hl) {
		background: rgba(212, 162, 76, 0.35);
		color: var(--text-primary);
		border-radius: 2px;
		padding: 0 1px;
	}
	/* Editor backdrop highlight: bright, high-contrast, and crucially adds NO
	   horizontal footprint (no padding/border/margin) so the overlaid text stays
	   pixel-aligned with the editable textarea underneath. */
	:global(.search-hl-overlay) {
		background: #f5c542;
		color: #1a1410;
		border-radius: 2px;
		padding: 0;
		margin: 0;
		border: 0;
	}
	.line-list {
		overflow: auto;
		padding: 10px;
		display: flex;
		flex-direction: column;
		gap: var(--line-record-gap);
		scrollbar-width: thin;
		scrollbar-color: var(--ink-line) transparent;
	}
	.line-list::-webkit-scrollbar {
		width: 6px;
	}
	.line-list::-webkit-scrollbar-thumb {
		background: var(--ink-line);
		border-radius: 999px;
	}
	.line-list::-webkit-scrollbar-track {
		background: transparent;
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
	:global(.line-action-icon) {
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
	.line-edit-textarea {
		height: auto;
		min-height: 62px;
		padding: 6px 8px;
		line-height: 1.45;
		font-family: var(--font-sans);
		font-size: 12px;
	}
	/* Highlight overlay for the content editor: a backdrop renders the text with
	   <mark> highlights for the current Find pattern, while a transparent-text
	   textarea on top handles editing. Both share identical metrics so the visible
	   (backdrop) text lines up with the caret. */
	.content-edit-wrap {
		position: relative;
		display: block;
	}
	.content-backdrop {
		position: absolute;
		inset: 0;
		box-sizing: border-box;
		border: 1px solid transparent;
		border-radius: 8px;
		background: var(--panel-bg-alt);
		color: var(--text-primary);
		overflow: hidden;
		pointer-events: none;
		white-space: pre-wrap;
		overflow-wrap: break-word;
		resize: none;
		margin: 0;
	}
	.content-edit-textarea {
		position: relative;
		z-index: 1;
		width: 100%;
		background: transparent;
		color: transparent;
		caret-color: var(--text-primary);
		resize: vertical;
		white-space: pre-wrap;
		overflow-wrap: break-word;
	}
	.content-edit-textarea::selection {
		background: rgba(212, 162, 76, 0.35);
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
	.find-replace-row {
		display: grid;
		grid-template-columns: 1fr 1fr auto;
		gap: 6px;
		align-items: center;
	}
	.find-replace-row .line-edit-input {
		flex: initial;
	}
	.find-replace-btn {
		flex: 0 0 auto;
		white-space: nowrap;
		height: 30px;
		padding: 0 12px;
	}
	.line-number-cell {
		font-family: var(--font-mono);
		font-size: 11px;
		color: var(--text-secondary);
		white-space: nowrap;
	}
	.line-number-cell.line-new {
		color: var(--brass);
		font-style: italic;
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
		min-height: 0;
		display: flex;
		flex-direction: column;
	}
	.pvw-tool-btn {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		width: 32px;
		height: 32px;
		border-radius: 8px;
		border: none;
		background: transparent;
		color: var(--pvw-tc, #94a3b8);
		cursor: pointer;
		transition: background 120ms ease, color 120ms ease;
	}
	.pvw-tool-btn:disabled {
		opacity: 0.38;
		cursor: not-allowed;
	}
	.pvw-tool-btn:hover:not(:disabled),
	.pvw-tool-btn:focus-visible {
		background: var(--pvw-hvr, rgba(99, 102, 241, 0.14));
		color: #818cf8;
		outline: none;
	}
	.pvw-tool-btn.active {
		background: var(--pvw-hvr, rgba(99, 102, 241, 0.14));
		color: #818cf8;
	}
	.pvw-tool-sep {
		width: 1px;
		height: 18px;
		background: var(--pvw-bdc, rgba(148, 163, 184, 0.18));
		flex-shrink: 0;
		margin: 0 2px;
	}
	:global(.pvw-tb-icon) {
		width: 15px;
		height: 15px;
		flex-shrink: 0;
		pointer-events: none;
	}
	.doc-empty {
		padding: 20px;
		color: var(--text-secondary);
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
	.dialog-field {
		margin: 0;
		padding: 10px 10px 8px;
		border-radius: 16px;
		border: 1px solid rgba(255, 255, 255, 0.06);
		background: #1a202b;
		box-shadow: inset 0 1px 0 rgba(255, 255, 255, 0.02);
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
		.line-list-resizer {
			display: none;
		}
	}
	@media (max-width: 1100px) {
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
		.dialog-section-head,
		.dialog-foot {
			flex-direction: column;
			align-items: flex-start;
		}
		.dialog-foot-buttons {
			width: 100%;
		}
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
