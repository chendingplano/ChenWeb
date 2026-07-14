<script lang="ts">
	import { tick } from 'svelte';
	import { browser } from '$app/environment';
	import {
		listKbSemanticProjections,
		getKbInput,
		getRawLines,
		type KbInputRecord,
		type KbSemanticProjectionRecord,
		type SemanticCategoryPath,
		type RawLine
	} from '$lib/services/kbService';
	import { buildSemanticProjectionHighlightRect } from './semantic-projection-highlight-rect.js';
	import { normalizeSemanticProjectionLineRefs } from './semantic-projection-line-spans.js';
	import KbInputRecordBrowser from '$lib/components/home3/kb-input-record-browser.svelte';
	import PdfViewWindow from '$lib/components/home3/pdf-view-window.svelte';
	import type { PdfPageViewport } from '$lib/components/home3/shared-pdf-viewer.svelte';
	import ListIcon from '@lucide/svelte/icons/list';
	import FileTextIcon from '@lucide/svelte/icons/file-text';
	import ArrowLeftIcon from '@lucide/svelte/icons/arrow-left';
	import ChevronLeftIcon from '@lucide/svelte/icons/chevron-left';
	import ChevronRightIcon from '@lucide/svelte/icons/chevron-right';
	import ActivityIcon from '@lucide/svelte/icons/activity';
	import TagIcon from '@lucide/svelte/icons/tag';
	import TypeIcon from '@lucide/svelte/icons/type';
	import HashIcon from '@lucide/svelte/icons/hash';
	import ListTreeIcon from '@lucide/svelte/icons/list-tree';
	import LanguagesIcon from '@lucide/svelte/icons/languages';

	let {
		darkMode = true,
		onFocusModeChange
	}: {
		darkMode: boolean;
		onFocusModeChange?: (focused: boolean) => void;
	} = $props();

	// ---------- Aesthetic tokens: "archival reading room" ----------
	let pageBg = $derived(darkMode ? '#0E1116' : '#F5F1E8');
	let docFrameBg = $derived(darkMode ? '#0A0D14' : '#E4DED0');
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
	let recordBrowserFolded = $state(false);
	let keywordFilter = $state('');
	let categoryFilter = $state('');
	let confidenceFilter = $state('');
	let itemNameDropdownValue = $state<number | ''>('');
	let currentInput = $state<KbInputRecord | null>(null);
	let items = $state<KbSemanticProjectionRecord[]>([]);
	let selectedItemId = $state<number | null>(null);
	let highlightSelectionVersion = $state(0);
	let loading = $state(false);
	let errorMsg = $state('');

	let rawLines = $state<RawLine[]>([]);
	let rawLoading = $state(false);
	let rawError = $state('');
	// Document viewer state
	let showLines = $state(false);
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

	let pdfNumPages = $state(0);
	let pdfZoom = $state(0.5);

	// ---------- Focus-mode info/PDF split (info 2/3, PDF 1/3, draggable) ----------
	const FOCUS_PDF_MIN = 280;
	const FOCUS_PDF_MAX = 1460;
	const FOCUS_PDF_DEFAULT = 480;
	const FOCUS_PDF_WIDTH_KEY = 'semantic-projections:focus-pdf-width';
	let focusPdfWidth = $state(FOCUS_PDF_DEFAULT);
	let focusResizing = $state(false);

	// Cap the PDF panel at 75% of the window so the Information panel keeps ≥ ~25%.
	// (FOCUS_PDF_MAX is NOT used as the upper bound — on wide monitors a fixed px cap
	// would leave the info panel stuck near 50%.)
	function clampFocusPdfWidth(value: number) {
		if (!Number.isFinite(value)) return FOCUS_PDF_DEFAULT;
		const maxPdf = browser ? Math.round(window.innerWidth * 0.75) : FOCUS_PDF_MAX;
		return Math.max(FOCUS_PDF_MIN, Math.min(maxPdf, Math.round(value)));
	}

	function persistFocusPdfWidth(next: number) {
		focusPdfWidth = clampFocusPdfWidth(next);
		if (browser) localStorage.setItem(FOCUS_PDF_WIDTH_KEY, String(focusPdfWidth));
	}

	$effect(() => {
		if (!browser) return;
		const saved = Number(localStorage.getItem(FOCUS_PDF_WIDTH_KEY));
		if (Number.isFinite(saved) && saved > 0) {
			focusPdfWidth = clampFocusPdfWidth(saved);
			return;
		}
		focusPdfWidth = clampFocusPdfWidth(Math.round(window.innerWidth / 3));
	});

	function startFocusResize(event: PointerEvent) {
		event.preventDefault();
		const startX = event.clientX;
		const startWidth = focusPdfWidth;
		focusResizing = true;
		document.body.style.cursor = 'col-resize';
		document.body.style.userSelect = 'none';
		const move = (e: PointerEvent) => persistFocusPdfWidth(startWidth - (e.clientX - startX));
		const up = () => {
			focusResizing = false;
			document.body.style.cursor = '';
			document.body.style.userSelect = '';
			window.removeEventListener('pointermove', move);
			window.removeEventListener('pointerup', up);
			window.removeEventListener('pointercancel', up);
		};
		window.addEventListener('pointermove', move);
		window.addEventListener('pointerup', up, { once: true });
		window.addEventListener('pointercancel', up, { once: true });
	}

	function onFocusResizerKeydown(event: KeyboardEvent) {
		if (event.key === 'ArrowLeft') {
			event.preventDefault();
			persistFocusPdfWidth(focusPdfWidth + 24);
		} else if (event.key === 'ArrowRight') {
			event.preventDefault();
			persistFocusPdfWidth(focusPdfWidth - 24);
		}
	}

	type NormalizedSpan = { page_number: number; line_number: number };

	// line_spans uses line-number spans only ("12", "13-15").
	function normalizeItemSpans(m: KbSemanticProjectionRecord | undefined): NormalizedSpan[] {
		return normalizeSemanticProjectionLineRefs(m?.line_spans, rawLines);
	}

	let highlightKeys = $derived.by(() => {
		const s = new Set<string>();
		if (selectedItemId == null) return s;
		const m = items.find((x) => x.id === selectedItemId);
		for (const span of normalizeItemSpans(m)) {
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
		if (selectedItemId == null) return map;
		const item = items.find((x) => x.id === selectedItemId);
		const spans = normalizeItemSpans(item);
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

	// ---------- Semantic-projection helpers ----------
	function pathNodeNames(p: SemanticCategoryPath): string[] {
		return (p?.category_path ?? [])
			.map((n) => (n?.name ?? '').toString().trim())
			.filter((s) => s !== '');
	}

	// Every distinct category node name across both language variants of a record.
	function categoryNamesOf(m: KbSemanticProjectionRecord): string[] {
		const set = new Set<string>();
		for (const p of [...(m.category_paths ?? []), ...(m.category_paths_en ?? [])]) {
			for (const name of pathNodeNames(p)) set.add(name);
		}
		return [...set];
	}

	// The highest path_confidence on a record (used for the card score + filter).
	function maxPathConfidence(m: KbSemanticProjectionRecord): number | undefined {
		const scores = (m.category_paths ?? [])
			.map((p) => p?.path_confidence)
			.filter((v): v is number => typeof v === 'number' && Number.isFinite(v));
		return scores.length ? Math.max(...scores) : undefined;
	}

	// Leaf name of the highest-confidence path — a compact label for the card.
	function topCategoryName(m: KbSemanticProjectionRecord): string {
		let best: SemanticCategoryPath | undefined;
		let bestScore = -Infinity;
		for (const p of m.category_paths ?? []) {
			const s = typeof p?.path_confidence === 'number' ? p.path_confidence : 0;
			if (s >= bestScore) {
				bestScore = s;
				best = p;
			}
		}
		const names = best ? pathNodeNames(best) : [];
		return names.length ? names[names.length - 1] : '';
	}

	// ---------- Keyword/category nav helpers ----------
	let allKeywords = $derived.by(() => {
		const set = new Set<string>();
		for (const m of items) {
			for (const k of m.keywords ?? []) if (typeof k === 'string' && k.trim()) set.add(k);
			for (const k of m.keywords_en ?? []) if (typeof k === 'string' && k.trim()) set.add(k);
			for (const name of categoryNamesOf(m)) set.add(name);
		}
		return [...set].sort((a, b) => a.localeCompare(b));
	});

	let allCategories = $derived.by(() => {
		const set = new Set<string>();
		for (const m of items) {
			for (const name of categoryNamesOf(m)) set.add(name);
		}
		return [...set].sort((a, b) => a.localeCompare(b));
	});

	// ---------- Attribute groups ----------
	type AttrKind = 'text' | 'chips' | 'lines';
	type LineEntry = { head: string; content: string; lineType: string };
	type AttrDef = {
		key: string;
		label: string;
		icon: any;
		kind: AttrKind;
		value: string;
		items: string[];
		entries: LineEntry[];
		count: number;
		hasValue: boolean;
	};
	type GroupNode = {
		key: string;
		label: string;
		icon: any;
		count: number;
		filledCount: number;
		hasValue: boolean;
		attrs: AttrDef[];
	};
	type ItemCanvas = {
		itemLabel: string;
		itemSubLabel: string;
		groups: GroupNode[];
	};

	function buildItemGroupAttrs(m: KbSemanticProjectionRecord): {
		identity: AttrDef[];
		summary: AttrDef[];
		keywords: AttrDef[];
		categories: AttrDef[];
		grounding: AttrDef[];
		provenance: AttrDef[];
	} {
		const fmt = (v: unknown): string => (v == null || v === '' ? '' : String(v));
		const has = (v: unknown): boolean => v != null && v !== '';
		const textAttr = (
			key: string,
			label: string,
			icon: any,
			value: string,
			hasValue: boolean
		): AttrDef => ({
			key,
			label,
			icon,
			kind: 'text',
			value,
			items: [],
			entries: [],
			count: hasValue ? 1 : 0,
			hasValue
		});
		const chipsAttr = (key: string, label: string, icon: any, chipItems: string[]): AttrDef => ({
			key,
			label,
			icon,
			kind: 'chips',
			value: chipItems.join(', '),
			items: chipItems,
			entries: [],
			count: chipItems.length,
			hasValue: chipItems.length > 0
		});
		const linesAttr = (key: string, label: string, icon: any, entries: LineEntry[]): AttrDef => {
			const list = entries.map((e) => (e.content ? `${e.head}: ${e.content}` : e.head));
			return {
				key,
				label,
				icon,
				kind: 'lines',
				value: list.join('\n'),
				items: list,
				entries,
				count: entries.length,
				hasValue: entries.length > 0
			};
		};

		const cleanStrs = (arr: unknown): string[] =>
			(Array.isArray(arr) ? arr : []).filter(
				(v): v is string => typeof v === 'string' && v.trim() !== ''
			);

		// Each ranked category path becomes a line card: the path chain as content,
		// its confidence + keywords summarised in the head.
		const pathEntries = (paths: SemanticCategoryPath[] | undefined): LineEntry[] =>
			(Array.isArray(paths) ? paths : [])
				.map((p) => {
					const chain = pathNodeNames(p).join(' › ');
					if (!chain) return null;
					const conf =
						typeof p?.path_confidence === 'number' ? confidencePct(p.path_confidence) : '';
					const head = conf ? `path · ${conf}` : 'path';
					return { head, content: chain, lineType: '' } as LineEntry;
				})
				.filter((e): e is LineEntry => e !== null);

		const identity: AttrDef[] = [
			textAttr('semantic_proj_id', 'Projection ID', HashIcon, fmt(m.semantic_proj_id), has(m.semantic_proj_id)),
			textAttr('descriptive_name', 'Descriptive Name', TypeIcon, fmt(m.descriptive_name), has(m.descriptive_name)),
			textAttr(
				'descriptive_name_en',
				'Descriptive Name (EN)',
				TypeIcon,
				fmt(m.descriptive_name_en),
				has(m.descriptive_name_en)
			),
			textAttr('language', 'Language', LanguagesIcon, fmt(m.language), has(m.language))
		];

		const summary: AttrDef[] = [
			linesAttr(
				'semantic_projection',
				'Projection',
				FileTextIcon,
				m.semantic_projection?.trim()
					? [{ head: 'Original', content: m.semantic_projection.trim(), lineType: '' }]
					: []
			),
			linesAttr(
				'semantic_projection_en',
				'Projection (EN)',
				FileTextIcon,
				m.semantic_projection_en?.trim()
					? [{ head: 'English', content: m.semantic_projection_en.trim(), lineType: '' }]
					: []
			)
		];

		const keywords: AttrDef[] = [
			chipsAttr('keywords', 'Keywords', TagIcon, cleanStrs(m.keywords)),
			chipsAttr('keywords_en', 'Keywords (EN)', TagIcon, cleanStrs(m.keywords_en))
		];

		const categories: AttrDef[] = [
			linesAttr('category_paths', 'Category Paths', ListTreeIcon, pathEntries(m.category_paths)),
			linesAttr('category_paths_en', 'Category Paths (EN)', ListTreeIcon, pathEntries(m.category_paths_en))
		];

		const groundingEntries: LineEntry[] = normalizeItemSpans(m).flatMap((span) => {
			const rawLine = rawLineByKey.get(`${span.page_number}:${span.line_number}`);
			const content = rawLine?.content ?? '';
			const lineType = rawLine?.line_type ?? '';
			const head = `L${span.line_number} · P${span.page_number}`;
			const segs = content.split('\n').filter((s) => s.trim() !== '');
			if (segs.length <= 1) return [{ head, content, lineType }];
			return segs.map((seg, i) => ({
				head: i === 0 ? head : `${head} · ${i + 1}`,
				content: seg,
				lineType: i === 0 ? lineType : ''
			}));
		});
		const grounding: AttrDef[] = [
			linesAttr('line_spans', 'Lines', FileTextIcon, groundingEntries)
		];

		const provenance: AttrDef[] = [
			textAttr('model_name', 'Model', ActivityIcon, fmt(m.model_name), has(m.model_name)),
			textAttr('prompt_name', 'Prompt', FileTextIcon, fmt(m.prompt_name), has(m.prompt_name)),
			textAttr('event_id', 'Event ID', HashIcon, fmt(m.event_id), has(m.event_id)),
			textAttr('create_time', 'Created', ActivityIcon, formatMaybeDate(m.create_time), has(m.create_time))
		];

		return { identity, summary, keywords, categories, grounding, provenance };
	}

	let itemMap = $derived.by((): ItemCanvas | null => {
		if (selectedItemId == null) return null;
		const m = items.find((x) => x.id === selectedItemId);
		if (!m) return null;
		const attrsByGroup = buildItemGroupAttrs(m);

		type GroupSpec = { key: string; label: string; icon: any; attrs: AttrDef[] };
		const groupSpecs: GroupSpec[] = [
			{ key: 'identity', label: 'Identity', icon: HashIcon, attrs: attrsByGroup.identity },
			{ key: 'summary', label: 'Projection', icon: FileTextIcon, attrs: attrsByGroup.summary },
			{ key: 'keywords', label: 'Keywords', icon: TagIcon, attrs: attrsByGroup.keywords },
			{ key: 'categories', label: 'Categories', icon: ListTreeIcon, attrs: attrsByGroup.categories },
			{ key: 'grounding', label: 'Grounding', icon: ListIcon, attrs: attrsByGroup.grounding },
			{ key: 'provenance', label: 'Provenance', icon: ActivityIcon, attrs: attrsByGroup.provenance }
		];

		const groups: GroupNode[] = groupSpecs.map((gs) => {
			const filledCount = gs.attrs.filter((a) => a.hasValue).length;
			return {
				key: gs.key,
				label: gs.label,
				icon: gs.icon,
				count: gs.attrs.length,
				filledCount,
				hasValue: filledCount > 0,
				attrs: gs.attrs
			};
		});

		return {
			itemLabel: itemNameOf(m),
			itemSubLabel: topCategoryName(m) || m.language || '',
			groups
		};
	});

	let filteredItems = $derived.by(() => {
		let result = items;
		const kw = keywordFilter.trim().toLowerCase();
		if (kw) {
			result = result.filter(
				(m) =>
					(m.descriptive_name ?? '').toLowerCase().includes(kw) ||
					(m.descriptive_name_en ?? '').toLowerCase().includes(kw) ||
					(m.semantic_projection ?? '').toLowerCase().includes(kw) ||
					(m.semantic_projection_en ?? '').toLowerCase().includes(kw) ||
					(m.keywords ?? []).some((a) => a.toLowerCase().includes(kw)) ||
					(m.keywords_en ?? []).some((a) => a.toLowerCase().includes(kw)) ||
					categoryNamesOf(m).some((a) => a.toLowerCase().includes(kw))
			);
		}
		const cat = categoryFilter.trim().toLowerCase();
		if (cat) {
			result = result.filter((m) =>
				categoryNamesOf(m).some((name) => name.toLowerCase().includes(cat))
			);
		}
		const cf = confidenceFilter.trim();
		if (cf) {
			if (cf.startsWith('<')) {
				const th = parseFloat(cf.slice(1).trim());
				if (Number.isFinite(th)) {
					result = result.filter((m) => (maxPathConfidence(m) ?? 0) < th);
				}
			} else {
				const th = parseFloat(cf);
				if (Number.isFinite(th)) {
					result = result.filter((m) => (maxPathConfidence(m) ?? 0) >= th);
				}
			}
		}
		return result;
	});

	let selectedItemInFilteredIndex = $derived(
		filteredItems.findIndex((m) => m.id === selectedItemId)
	);
	let prevItem = $derived(
		selectedItemInFilteredIndex > 0 ? filteredItems[selectedItemInFilteredIndex - 1] : null
	);
	let nextItem = $derived(
		selectedItemInFilteredIndex >= 0 && selectedItemInFilteredIndex < filteredItems.length - 1
			? filteredItems[selectedItemInFilteredIndex + 1]
			: null
	);

	// Keep selection in sync with filter changes.
	$effect(() => {
		const fm = filteredItems;
		if (selectedItemId == null) return;
		if (fm.length === 0) {
			selectedItemId = null;
		} else if (!fm.some((m) => m.id === selectedItemId)) {
			selectedItemId = fm[0].id;
		}
	});

	$effect(() => {
		itemNameDropdownValue = selectedItemId ?? '';
	});

	function renderItemHighlights(
		pageNo: number,
		viewport: PdfPageViewport,
		overlay: HTMLDivElement
	) {
		const lines = selectedLinesByPage.get(pageNo) ?? [];
		const rect = buildSemanticProjectionHighlightRect(lines, viewport);
		if (!rect) return;
		const mark = document.createElement('div');
		mark.className = 'pdf-highlight';
		mark.style.left = `${rect.left}px`;
		mark.style.top = `${rect.top}px`;
		mark.style.width = `${rect.width}px`;
		mark.style.height = `${rect.height}px`;
		mark.title = rect.lineCount === 1 ? '1 line' : `${rect.lineCount} lines`;
		overlay.appendChild(mark);
	}

	function formatMaybeDate(value?: string): string {
		if (!value) return '—';
		return value.replace('T', ' ').slice(0, 19);
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

	function handleUserRecordSelect(record: KbInputRecord) {
		if (currentInput?.id === record.id) return;
		currentInput = record;
		void loadItemsForRecord(record.id);
	}

	function enterFocusMode() {
		if (recordBrowserFolded) return;
		recordBrowserFolded = true;
		onFocusModeChange?.(true);
	}

	function goBack() {
		if (!recordBrowserFolded) return;
		recordBrowserFolded = false;
		selectedItemId = null;
		onFocusModeChange?.(false);
	}

	function goToPrevItem() {
		if (prevItem) void selectItem(prevItem);
	}

	function goToNextItem() {
		if (nextItem) void selectItem(nextItem);
	}

	function handleItemNameDropdown(e: Event) {
		const id = Number((e.target as HTMLSelectElement).value);
		const m = items.find((x) => x.id === id);
		if (m) void selectItem(m);
	}

	async function loadItemsForRecord(id: number) {
		keywordFilter = '';
		categoryFilter = '';
		confidenceFilter = '';
		errorMsg = '';
		loading = true;
		items = [];
		selectedItemId = null;
		highlightSelectionVersion = 0;
		rawLines = [];
		rawError = '';
		rawLoading = false;
		if (currentInput?.id !== id) currentInput = null;
		docPage = 1;
		pdfNumPages = 0;
		try {
			rawLoading = true;
			const [itemRes, inputRes, rawRes] = await Promise.all([
				listKbSemanticProjections(id),
				getKbInput(id).catch(() => null),
				getRawLines(id).catch(() => null)
			]);
			items = itemRes.results ?? [];
			currentInput = inputRes?.record ?? null;
			rawLines = rawRes?.lines ?? [];
			rawError = rawRes ? '' : 'Failed to load raw lines';
		} catch (err) {
			errorMsg = err instanceof Error ? err.message : 'Failed to retrieve semantic projections';
		} finally {
			rawLoading = false;
			loading = false;
		}
	}

	async function selectItem(m: KbSemanticProjectionRecord) {
		selectedItemId = m.id;
		highlightSelectionVersion += 1;
		enterFocusMode();
		const first = normalizeItemSpans(m)[0];
		if (!first) return;
		docPage = first.page_number > 0 ? first.page_number : 1;
		if (showLines) {
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

	function onItemCardClick(event: MouseEvent, m: KbSemanticProjectionRecord) {
		event.preventDefault();
		event.stopPropagation();
		void selectItem(m);
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

	function itemNameOf(m: KbSemanticProjectionRecord): string {
		return (
			m.descriptive_name?.trim() ||
			m.descriptive_name_en?.trim() ||
			m.semantic_proj_id?.trim() ||
			`Projection #${m.id}`
		);
	}
	function confidencePct(c?: number): string {
		if (c == null) return '—';
		return `${Math.round(c * 100)}%`;
	}
	// Number of ranked category paths attached to a projection (card badge).
	function pathCount(m: KbSemanticProjectionRecord): number {
		return Array.isArray(m.category_paths) ? m.category_paths.length : 0;
	}
</script>

<svelte:window
	onkeydown={(e) => {
		if (e.key === 'Escape' && recordBrowserFolded) goBack();
	}}
/>

<div
	class="metric-mgmt"
	style="
		--page-bg:{pageBg};
		--doc-frame-bg:{docFrameBg};
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
			<div class="eyebrow">Knowledge System · Vol. V</div>
			<h1 class="display">Semantic&nbsp;<span class="amp">&amp;</span>&nbsp;Projections</h1>
			<div class="subtitle">
				A reading room for extracted semantic projections — locate, verify, return to source.
			</div>
		</div>
		<div class="header-right">
			<span class="meta-label">RECORD</span><span class="meta-val">{currentInput?.id ?? '—'}</span>
			<span class="meta-label">TYPE</span><span class="meta-val">{currentInput?.type ?? '—'}</span>
			<span class="meta-label">ITEMS</span><span class="meta-val"
				>{items.length.toString().padStart(3, '0')}</span
			>
		</div>
	</header>

	<div class="body" style={recordBrowserFolded ? 'grid-template-columns: minmax(0, 1fr)' : ''}>
		{#if !recordBrowserFolded}
			<KbInputRecordBrowser
				{darkMode}
				instanceKey="semantic-projections-record-browser"
				title="kb.inputs"
				subtitle="Search, filter, and select input records before inspecting extracted semantic projections."
				emptyTitle="No records yet"
				emptySubtitle="Use Search or Retrieve to browse kb.inputs."
				autoSelectFirstRecord={false}
				selectedRecordId={currentInput?.id ?? null}
				mapRecord={mapBrowserRecord}
				onSelect={handleUserRecordSelect}
				onError={(error) => {
					errorMsg = error.message;
				}}
			/>

			<aside class="metric-sidebar">
				<div class="left-meta">
					<div class="left-meta-title">Semantic Projections</div>
					<div class="left-meta-count">{items.length} found</div>
				</div>

				<div class="metrics-list">
					{#if errorMsg}
						<div class="error">{errorMsg}</div>
					{:else if loading}
						<div class="empty">
							<div class="empty-glyph">⌕</div>
							<div class="empty-title">Loading semantic projections</div>
							<div class="empty-sub">Fetching extracted projections for this record…</div>
						</div>
					{:else if items.length === 0}
						<div class="empty">
							<div class="empty-glyph">§</div>
							<div class="empty-title">No semantic projections yet</div>
							<div class="empty-sub">
								Select a record from kb.inputs to populate the projection index.
							</div>
						</div>
					{:else if filteredItems.length === 0 && (keywordFilter || categoryFilter || confidenceFilter)}
						<div class="empty">
							<div class="empty-glyph">§</div>
							<div class="empty-title">No matches</div>
							<div class="empty-sub">No semantic projections match the current filters.</div>
						</div>
					{:else}
						{#each filteredItems as m, idx (m.id)}
							<button
								type="button"
								class="metric-card"
								class:selected={selectedItemId === m.id}
								onclick={(event) => onItemCardClick(event, m)}
							>
								<div class="card-rule" aria-hidden="true"></div>
								<div class="card-body">
									<div class="card-row-top">
										<div class="card-index">№ {String(idx + 1).padStart(3, '0')}</div>
										<div class="card-conf" title="Top path confidence">
											{confidencePct(maxPathConfidence(m))}
										</div>
									</div>
									<div class="card-name">{itemNameOf(m)}</div>
									{#if m.semantic_projection}
										<div class="card-desc">{m.semantic_projection}</div>
									{/if}
									<div class="card-foot">
										<span class="chip">
											<span class="chip-dot"></span>
											{pathCount(m)} path{pathCount(m) === 1 ? '' : 's'}
										</span>
										{#if topCategoryName(m)}<span class="chip chip-mono">{topCategoryName(m)}</span>{/if}
										{#if m.language}<span class="chip chip-quiet">{m.language}</span>{/if}
									</div>
								</div>
							</button>
						{/each}
					{/if}
				</div>
			</aside>
		{/if}

		<!-- ============ RIGHT PANEL ============ -->
		<section class="right" class:focus-split={recordBrowserFolded}>
			{#if recordBrowserFolded}
				<div class="metric-canvas-wrap">
					<div class="canvas-toolbar">
						<button type="button" class="toolbar-back" onclick={goBack} title="Back to record list">
							<ArrowLeftIcon class="toolbar-icon" />
							<span>Back</span>
						</button>
						<div class="toolbar-filters">
							<select
								class="toolbar-select"
								value={itemNameDropdownValue}
								onchange={handleItemNameDropdown}
								title="Jump to projection by name"
							>
								<option value="">— Projection by name —</option>
								{#each items as m (m.id)}
									<option value={m.id}>{itemNameOf(m)}</option>
								{/each}
							</select>
							<div class="toolbar-kw-wrap">
								<input
									class="toolbar-kw-input"
									type="text"
									list="semantic-keywords-datalist-focus"
									placeholder="Filter by keyword…"
									bind:value={keywordFilter}
								/>
								<datalist id="semantic-keywords-datalist-focus">
									{#each allKeywords as kw (kw)}
										<option value={kw}></option>
									{/each}
								</datalist>
								{#if keywordFilter}
									<button
										type="button"
										class="toolbar-kw-clear"
										onclick={() => (keywordFilter = '')}
										title="Clear keyword filter"
										aria-label="Clear keyword filter">×</button
									>
								{/if}
							</div>
							<div class="toolbar-kw-wrap">
								<input
									class="toolbar-kw-input"
									type="text"
									list="semantic-category-options"
									placeholder="Category…"
									title="Filter by category path name"
									bind:value={categoryFilter}
								/>
								<datalist id="semantic-category-options">
									{#each allCategories as cat (cat)}
										<option value={cat}></option>
									{/each}
								</datalist>
								{#if categoryFilter}
									<button
										type="button"
										class="toolbar-kw-clear"
										onclick={() => (categoryFilter = '')}
										title="Clear category filter"
										aria-label="Clear category filter">×</button
									>
								{/if}
							</div>
							<div class="toolbar-kw-wrap">
								<input
									class="toolbar-kw-input"
									type="text"
									list="semantic-confidence-options"
									placeholder="Confidence…"
									title="Filter by top path-confidence. Select or type a value like 0.85, or <0.50 for below-threshold."
									bind:value={confidenceFilter}
								/>
								<datalist id="semantic-confidence-options">
									<option value="0.90"></option>
									<option value="0.80"></option>
									<option value="0.70"></option>
									<option value="0.60"></option>
									<option value="0.50"></option>
									<option value="<0.50"></option>
								</datalist>
								{#if confidenceFilter}
									<button
										type="button"
										class="toolbar-kw-clear"
										onclick={() => (confidenceFilter = '')}
										title="Clear confidence filter"
										aria-label="Clear confidence filter">×</button
									>
								{/if}
							</div>
						</div>
						<div class="toolbar-nav">
							<button
								type="button"
								class="toolbar-nav-btn"
								disabled={!prevItem}
								onclick={goToPrevItem}
								title="Previous item"><ChevronLeftIcon class="toolbar-icon" /></button
							>
							<span class="toolbar-nav-pos">
								{selectedItemInFilteredIndex >= 0
									? `${selectedItemInFilteredIndex + 1} / ${filteredItems.length}`
									: `— / ${filteredItems.length}`}
							</span>
							<button
								type="button"
								class="toolbar-nav-btn"
								disabled={!nextItem}
								onclick={goToNextItem}
								title="Next item"><ChevronRightIcon class="toolbar-icon" /></button
							>
						</div>
					</div>
					<div class="metric-canvas">
						{#if itemMap}
							{@const map = itemMap}
							<div class="attr-view">
								<div class="attr-view-header">
									<div class="attr-view-name">{map.itemLabel}</div>
									{#if map.itemSubLabel}
										<div class="attr-view-subname">{map.itemSubLabel}</div>
									{/if}
								</div>
								{#each map.groups as g (g.key)}
									{@const GIcon = g.icon}
									<section class="attr-group" class:attr-group-empty={!g.hasValue}>
										<div class="attr-group-head">
											<span class="attr-group-ic"><GIcon class="h-4 w-4" /></span>
											<span class="attr-group-label">{g.label}</span>
											<span class="attr-group-count">{g.filledCount}/{g.count}</span>
										</div>
										<div class="attr-group-body">
											{#each g.attrs as a (a.key)}
												<div
													class="gip-row"
													class:gip-row-col={a.kind !== 'text'}
													class:gip-row-empty={!a.hasValue}
												>
													<span class="gip-label">{a.label}</span>
													{#if !a.hasValue}
														<span class="gip-empty">—</span>
													{:else if a.kind === 'text'}
														<span class="gip-val">{a.value}</span>
													{:else if a.kind === 'chips'}
														<div class="gip-chips">
															{#each a.items as item, i (`${a.key}-${i}`)}<span class="gip-chip"
																	>{item}</span
																>{/each}
														</div>
													{:else}
														<div class="gip-line-cards">
															{#each a.entries as entry, i (`${a.key}-${i}`)}
																<div class="gip-line-card">
																	<div class="gip-line-head">
																		<span class="gip-line-loc">{entry.head}</span>
																		{#if entry.lineType}
																			<span class="gip-line-type">{entry.lineType}</span>
																		{/if}
																	</div>
																	{#if entry.content}
																		<div class="gip-line-body">{entry.content}</div>
																	{/if}
																</div>
															{/each}
														</div>
													{/if}
												</div>
											{/each}
										</div>
									</section>
								{/each}
							</div>
						{:else}
							<div class="canvas-empty">
								<div class="canvas-empty-mark">◎</div>
								<div class="canvas-empty-title">Select a projection</div>
								<div class="canvas-empty-sub">
									Click a semantic projection from the list to view its attribute map.
								</div>
							</div>
						{/if}
					</div>
				</div>
				<button
					type="button"
					class="focus-resize-handle"
					class:active={focusResizing}
					aria-label="Resize the source document panel"
					onpointerdown={startFocusResize}
					onkeydown={onFocusResizerKeydown}
				>
					<span class="focus-resize-grip" aria-hidden="true"></span>
				</button>
			{/if}
			<div
				class="doc-frame-wrap"
				style:flex-basis={recordBrowserFolded ? `${focusPdfWidth}px` : null}
			>
				{#if !currentInput}
					<div class="doc-empty">
						<div class="doc-empty-mark">⌬</div>
						<div class="doc-empty-title">Awaiting selection</div>
						<div class="doc-empty-sub">
							Once you retrieve a record, the original document appears here.<br />
							Click any item on the left to jump to its source page.
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
							highlightVersion={`${selectedItemId ?? 0}:${highlightSelectionVersion}`}
							renderHighlights={renderItemHighlights}
							bind:showingLines={showLines}
							enableSelectionDialog={false}
							{darkMode}
						>
							{#snippet toolbar()}
								<button
									type="button"
									class="pvw-tool-btn"
									class:active={showLines}
									onclick={() => (showLines = !showLines)}
									title={showLines ? 'Show PDF Document' : 'Show Lines'}
								>
									{#if showLines}
										<FileTextIcon class="pvw-tb-icon" />
									{:else}
										<ListIcon class="pvw-tb-icon" />
									{/if}
								</button>
							{/snippet}
							{#snippet linesView()}
								<div class="lines-panel">
									{#if rawLoading}
										<div class="doc-status">
											<span class="dot-loop"></span>Reading raw_line file…
										</div>
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
												Once you retrieve a record, the parsed lines appear here.<br />
												Click any item on the left to jump to its source line.
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
							This file is rendered as text by your browser. For exact line highlighting, switch to
							the <button class="inline-tab-btn" onclick={() => (showLines = true)}
								>Source&nbsp;Lines</button
							> view.
						</div>
					{:else if !isPdf}
						<div class="doc-foot-hint">
							Inline preview support varies by file type. For line-level highlights, use the <button
								class="inline-tab-btn"
								onclick={() => (showLines = true)}>Source&nbsp;Lines</button
							> view.
						</div>
					{/if}
				{/if}
			</div>
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

	/* ---------- ITEM SIDEBAR ---------- */
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
	.left-meta-count {
		margin-left: auto;
		font-family: var(--font-mono);
		font-size: 11px;
		letter-spacing: 0.1em;
		color: var(--text-muted);
		text-transform: uppercase;
	}
	.left-meta-title {
		font-family: var(--font-serif);
		font-size: 22px;
		font-weight: 500;
		color: var(--text-primary);
	}

	.metrics-list {
		flex: 1;
		min-height: 0;
		overflow-y: auto;
		padding: 16px 16px 24px;
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
		line-clamp: 2;
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
	/* ---- PDF toolbar buttons (rendered via snippet in PdfViewWindow) ---- */
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
		transition:
			background 120ms ease,
			color 120ms ease;
	}
	.pvw-tool-btn:disabled {
		opacity: 0.38;
		cursor: not-allowed;
	}
	.pvw-tool-btn:hover,
	.pvw-tool-btn:focus-visible,
	.pvw-tool-btn.active {
		background: var(--pvw-hvr, rgba(99, 102, 241, 0.14));
		color: #818cf8;
		outline: none;
	}
	:global(.pvw-tb-icon) {
		width: 15px;
		height: 15px;
		flex-shrink: 0;
		pointer-events: none;
	}

	/* ---- Lines panel (shown via linesView snippet) ---- */
	.lines-panel {
		padding: 32px 36px 80px;
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
	.doc-frame {
		flex: 1;
		width: 100%;
		min-width: 0;
		min-height: 0;
		border: 1px solid var(--ink-line);
		background: var(--doc-frame-bg);
	}
	:global(.pdf-highlight) {
		position: absolute;
		background: rgba(250, 204, 21, 0.22);
		border: 1px solid rgba(234, 179, 8, 0.95);
		box-shadow: inset 0 0 0 1px rgba(254, 240, 138, 0.28);
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

	/* ---- CANVAS (item attribute map) ---- */
	.right.focus-split {
		flex-direction: row;
	}
	.metric-canvas-wrap {
		flex: 1 1 0;
		min-width: 300px;
		min-height: 0;
		display: flex;
		flex-direction: column;
		border-right: 1px solid var(--ink-line);
		overflow: hidden;
	}
	.canvas-toolbar {
		display: flex;
		align-items: center;
		gap: 10px;
		padding: 8px 12px;
		border-bottom: 1px solid var(--ink-line);
		background: var(--panel-bg-alt);
		flex-shrink: 0;
		flex-wrap: wrap;
	}
	.toolbar-back {
		all: unset;
		cursor: pointer;
		display: inline-flex;
		align-items: center;
		gap: 5px;
		padding: 4px 10px 4px 6px;
		border-radius: 6px;
		border: 1px solid var(--ink-line);
		font-family: var(--font-mono);
		font-size: 11px;
		letter-spacing: 0.06em;
		color: var(--text-secondary);
		background: var(--panel-bg);
		transition:
			border-color 120ms,
			color 120ms;
	}
	.toolbar-back:hover {
		border-color: var(--brass);
		color: var(--brass);
	}
	:global(.toolbar-icon) {
		width: 13px;
		height: 13px;
		flex-shrink: 0;
		pointer-events: none;
	}
	.toolbar-filters {
		display: flex;
		align-items: center;
		gap: 8px;
		flex: 1;
		min-width: 0;
		flex-wrap: wrap;
	}
	.toolbar-select {
		background: var(--panel-bg);
		border: 1px solid var(--ink-line);
		border-radius: 6px;
		color: var(--text-primary);
		font-family: var(--font-mono);
		font-size: 11px;
		padding: 4px 8px;
		max-width: 280px;
		min-width: 0;
	}
	.toolbar-select:focus {
		outline: none;
		border-color: var(--brass);
	}
	.toolbar-kw-wrap {
		position: relative;
		display: inline-flex;
		align-items: center;
	}
	.toolbar-kw-input {
		background: var(--panel-bg);
		border: 1px solid var(--ink-line);
		border-radius: 6px;
		color: var(--text-primary);
		font-family: var(--font-mono);
		font-size: 11px;
		padding: 4px 22px 4px 8px;
		width: 160px;
	}
	.toolbar-kw-input:focus {
		outline: none;
		border-color: var(--brass);
	}
	.toolbar-kw-clear {
		all: unset;
		position: absolute;
		right: 6px;
		top: 50%;
		transform: translateY(-50%);
		cursor: pointer;
		color: var(--text-muted);
		font-size: 14px;
		line-height: 1;
		padding: 0 4px;
	}
	.toolbar-kw-clear:hover {
		color: var(--brass);
	}
	.toolbar-nav {
		display: flex;
		align-items: center;
		gap: 4px;
		flex-shrink: 0;
	}
	.toolbar-nav-btn {
		all: unset;
		cursor: pointer;
		display: inline-flex;
		align-items: center;
		justify-content: center;
		width: 26px;
		height: 26px;
		border-radius: 6px;
		border: 1px solid var(--ink-line);
		color: var(--text-secondary);
		background: var(--panel-bg);
		transition:
			border-color 120ms,
			color 120ms,
			opacity 120ms;
	}
	.toolbar-nav-btn:disabled {
		opacity: 0.35;
		cursor: not-allowed;
	}
	.toolbar-nav-btn:not(:disabled):hover {
		border-color: var(--brass);
		color: var(--brass);
	}
	.toolbar-nav-pos {
		font-family: var(--font-mono);
		font-size: 11px;
		color: var(--text-muted);
		padding: 0 6px;
		min-width: 56px;
		text-align: center;
	}
	.metric-canvas {
		flex: 1 1 0;
		min-width: 0;
		min-height: 0;
		overflow-y: auto;
		overflow-x: hidden;
		background:
			radial-gradient(ellipse at 30% 50%, rgba(212, 162, 76, 0.06), transparent 55%),
			radial-gradient(ellipse at 70% 50%, rgba(200, 85, 61, 0.05), transparent 55%), var(--page-bg);
		scrollbar-width: thin;
		scrollbar-color: var(--ink-line) transparent;
	}
	.metric-canvas::-webkit-scrollbar {
		width: 8px;
	}
	.metric-canvas::-webkit-scrollbar-thumb {
		background: var(--ink-line);
	}
	.right.focus-split .doc-frame-wrap {
		flex: 0 0 480px;
		overflow: hidden;
	}
	.focus-resize-handle {
		flex: 0 0 14px;
		position: relative;
		padding: 0;
		border: 0;
		background: transparent;
		cursor: col-resize;
		outline: none;
		align-self: stretch;
		touch-action: none;
	}
	.focus-resize-handle::before {
		content: '';
		position: absolute;
		top: 0;
		bottom: 0;
		left: 6px;
		width: 2px;
		background: var(--ink-line);
		opacity: 0.6;
		transition:
			background 140ms,
			opacity 140ms;
	}
	.focus-resize-handle:hover::before,
	.focus-resize-handle.active::before,
	.focus-resize-handle:focus-visible::before {
		background: var(--brass);
		opacity: 1;
	}
	.focus-resize-grip {
		position: absolute;
		top: 50%;
		left: 50%;
		width: 7px;
		height: 48px;
		transform: translate(-50%, -50%);
		border-radius: 999px;
		background:
			radial-gradient(circle, var(--text-muted) 22%, transparent 24%) center 6px / 5px 10px repeat-y,
			var(--panel-bg);
		border: 1px solid var(--ink-line);
		box-shadow: 0 0 0 2px rgba(0, 0, 0, 0.14);
	}
	.focus-resize-handle.active .focus-resize-grip,
	.focus-resize-handle:hover .focus-resize-grip,
	.focus-resize-handle:focus-visible .focus-resize-grip {
		border-color: var(--brass);
	}

	/* ---- Attribute rows (Information Panel) ---- */
	.gip-row {
		display: flex;
		align-items: baseline;
		gap: 10px;
	}
	.gip-row-col {
		flex-direction: column;
		align-items: stretch;
		gap: 5px;
	}
	.gip-row-empty {
		opacity: 0.55;
	}
	.gip-label {
		font-family: var(--font-mono);
		font-size: 9px;
		letter-spacing: 0.12em;
		text-transform: uppercase;
		color: var(--text-muted);
		flex-shrink: 0;
		min-width: 80px;
	}
	.gip-val {
		font-family: var(--font-sans);
		font-size: 12px;
		color: var(--text-primary);
		line-height: 1.45;
		word-break: break-word;
		flex: 1 1 auto;
		min-width: 0;
	}
	.gip-empty {
		font-family: var(--font-mono);
		font-size: 11px;
		color: var(--text-muted);
	}
	.gip-chips {
		display: flex;
		flex-wrap: wrap;
		gap: 4px;
	}
	.gip-chip {
		display: inline-flex;
		align-items: center;
		padding: 2px 8px;
		border-radius: 999px;
		border: 1px solid var(--ink-line);
		background: var(--panel-bg-alt);
		color: var(--text-primary);
		font-size: 11px;
		line-height: 1.4;
		font-family: var(--font-sans);
	}
	.gip-line-cards {
		display: flex;
		flex-direction: column;
		gap: 8px;
	}
	.gip-line-card {
		display: flex;
		flex-direction: column;
		gap: 4px;
		padding: 8px 10px;
		border: 1px solid var(--ink-line);
		border-radius: 6px;
		background: var(--panel-bg-alt);
		box-shadow: inset 0 0 0 1px rgba(0, 0, 0, 0.08);
	}
	.gip-line-head {
		display: flex;
		align-items: center;
		gap: 6px;
		flex-wrap: wrap;
	}
	.gip-line-loc {
		font-family: var(--font-mono);
		font-size: 10px;
		letter-spacing: 0.08em;
		text-transform: uppercase;
		color: var(--brass);
	}
	.gip-line-type {
		font-family: var(--font-mono);
		font-size: 9px;
		letter-spacing: 0.06em;
		text-transform: uppercase;
		color: var(--text-muted);
		padding: 1px 6px;
		border: 1px solid var(--ink-line);
		border-radius: 999px;
		background: var(--panel-bg);
	}
	.gip-line-body {
		font-family: var(--font-sans);
		font-size: 12px;
		color: var(--text-primary);
		line-height: 1.5;
		word-break: break-word;
	}
	/* ---- Attribute text view ---- */
	.attr-view {
		padding: 16px 20px 40px;
		display: flex;
		flex-direction: column;
	}
	.attr-view-header {
		padding-bottom: 16px;
		border-bottom: 1px solid var(--ink-line);
		margin-bottom: 4px;
	}
	.attr-view-name {
		font-family: var(--font-serif);
		font-size: 22px;
		font-weight: 500;
		line-height: 1.25;
		color: var(--text-primary);
	}
	.attr-view-subname {
		font-family: var(--font-mono);
		font-size: 11px;
		color: var(--text-muted);
		margin-top: 4px;
		letter-spacing: 0.04em;
	}
	.attr-group {
		background: color-mix(in srgb, var(--panel-bg-alt) 95%, white);
		border: 1px solid var(--ink-line-soft);
		border-radius: 10px;
		padding: 12px 14px;
		margin-top: 10px;
	}
	.attr-group-empty {
		opacity: 0.55;
	}
	.attr-group-head {
		display: flex;
		align-items: center;
		gap: 8px;
		margin-bottom: 10px;
	}
	.attr-group-ic {
		display: inline-flex;
		color: var(--brass);
		flex-shrink: 0;
	}
	.attr-group-label {
		font-family: var(--font-serif);
		font-size: 14px;
		font-weight: 600;
		color: var(--brass);
		flex: 1;
		letter-spacing: 0.01em;
	}
	.attr-group-count {
		font-family: var(--font-mono);
		font-size: 10px;
		color: var(--text-muted);
		letter-spacing: 0.06em;
	}
	.attr-group-body {
		display: flex;
		flex-direction: column;
		gap: 8px;
		padding-left: 8px;
	}

	.canvas-empty {
		display: flex;
		flex-direction: column;
		align-items: center;
		justify-content: center;
		min-height: 100%;
		padding: 60px 20px;
		color: var(--text-muted);
		gap: 8px;
	}
	.canvas-empty-mark {
		font-family: var(--font-serif);
		font-size: 48px;
		opacity: 0.3;
		color: var(--brass);
		line-height: 1;
	}
	.canvas-empty-title {
		font-family: var(--font-serif);
		font-size: 18px;
		color: var(--text-secondary);
	}
	.canvas-empty-sub {
		font-size: 12px;
		max-width: 220px;
		text-align: center;
		line-height: 1.5;
	}
</style>
