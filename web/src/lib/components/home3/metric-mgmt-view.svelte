<script lang="ts">
	import { tick } from 'svelte';
	import { browser } from '$app/environment';
	import {
		listKbMetrics,
		getKbInput,
		getRawLines,
		extractKbMetrics,
		saveExtractedKbMetrics,
		createKbProvision,
		updateRawLine,
		type KbInputRecord,
		type KbMetricRecord,
		type ExtractedKbMetric,
		type RawLine,
		type SourceLineSpan
	} from '$lib/services/kbService';
	import { searchKbMetrics, type KbMetricSearchResult } from '$lib/services/kbMetricSearch';
	import KbInputRecordBrowser from '$lib/components/home3/kb-input-record-browser.svelte';
	import PdfViewWindow from '$lib/components/home3/pdf-view-window.svelte';
	import type { PdfPageViewport } from '$lib/components/home3/shared-pdf-viewer.svelte';
	import SquarePenIcon from '@lucide/svelte/icons/square-pen';
	import Trash2Icon from '@lucide/svelte/icons/trash-2';
	import ListPlusIcon from '@lucide/svelte/icons/list-plus';
	import ListIcon from '@lucide/svelte/icons/list';
	import FileTextIcon from '@lucide/svelte/icons/file-text';
	import ArrowLeftIcon from '@lucide/svelte/icons/arrow-left';
	import ChevronLeftIcon from '@lucide/svelte/icons/chevron-left';
	import ChevronRightIcon from '@lucide/svelte/icons/chevron-right';
	import ActivityIcon from '@lucide/svelte/icons/activity';
	import TrendingUpIcon from '@lucide/svelte/icons/trending-up';
	import TagIcon from '@lucide/svelte/icons/tag';
	import MapPinIcon from '@lucide/svelte/icons/map-pin';
	import TypeIcon from '@lucide/svelte/icons/type';
	import BookOpenIcon from '@lucide/svelte/icons/book-open';
	import HashIcon from '@lucide/svelte/icons/hash';
	import FileIcon from '@lucide/svelte/icons/file';
	import CalendarIcon from '@lucide/svelte/icons/calendar';
	import XIcon from '@lucide/svelte/icons/x';
	import {
		KB_METRIC_SEARCH_DEFAULTS,
		buildKbMetricSearchParams,
		createEmptyKbMetricSearchFilters,
		hasKbMetricSearchFilters
	} from './kb-metric-search-state.js';
	import {
		metricSearchResultChips,
		metricSearchResultSecondaryText
	} from './kb-metric-search-result.js';

	let {
		darkMode = true,
		onFocusModeChange
	}: {
		darkMode: boolean;
		onFocusModeChange?: (focused: boolean) => void;
	} = $props();

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
	let recordBrowserFolded = $state(false);
	let keywordFilter = $state('');
	let confidenceFilter = $state('');
	let searchQuery = $state('');
	let searchFilters = $state(createEmptyKbMetricSearchFilters());
	let searchResults = $state<KbMetricSearchResult[]>([]);
	let searchLoading = $state(false);
	let searchError = $state('');
	let searchHasRun = $state(false);
	let searchTotal = $state(0);
	let metricNameDropdownValue = $state<number | ''>('');
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
	let showLines = $state(false);
	let editLineMode = $state(false);
	let deleteLineMode = $state(false);
	let addLineOpen = $state(false);
	let addMetricOpen = $state(false);
	let addMetricEditKey = $state<string | null>(null);
	let addMetricEditContent = $state('');
	let addMetricBusyAction = $state<'line' | 'extract' | 'save' | 'provision' | null>(null);
	let extractedMetricsPreview = $state<ExtractedKbMetric[]>([]);
	let pdfSelectedLines = $state<number[]>([]);
	let pdfDragPreviewLines = $state<number[]>([]);
	// Dedicated buffer for the Add Metric dialog, set during drag-select and
	// cleared only when the dialog closes. Not affected by the click-away handler
	// that clears pdfSelectedLines on outside clicks.
	let addMetricBufferLines = $state<number[]>([]);
	let editingLineKey = $state<string | null>(null);
	let editingLineContent = $state('');
	let newLineContent = $state('');
	let newLineType = $state('text');
	let docPage = $state<number>(1);
	let addMetricBusy = $derived(addMetricBusyAction !== null);

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
	let canvasW = $state(0);
	let canvasH = $state(0);
	let hoveredCanvasAttr = $state<string | null>(null);
	let activeGroupKey = $state<string | null>(null);

	// ---------- Focus-mode chart/PDF split (chart 2/3, PDF 1/3, draggable) ----------
	const FOCUS_PDF_MIN = 280;
	const FOCUS_PDF_MAX = 1460;
	const FOCUS_PDF_DEFAULT = 480;
	const FOCUS_PDF_WIDTH_KEY = 'metrics:focus-pdf-width';
	let focusPdfWidth = $state(FOCUS_PDF_DEFAULT);
	let focusResizing = $state(false);

	function clampFocusPdfWidth(value: number) {
		if (!Number.isFinite(value)) return FOCUS_PDF_DEFAULT;
		return Math.max(FOCUS_PDF_MIN, Math.min(FOCUS_PDF_MAX, Math.round(value)));
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
		// First-load default: PDF at ~1/3 of viewport so the chart gets ~2/3.
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
	// ---------- Group info panel width (draggable) ----------
	const GIP_WIDTH_KEY = 'metrics:info-panel-width';
	const GIP_WIDTH_MIN = 220;
	const GIP_WIDTH_MAX = 620;
	let gipWidth = $state<number | null>(null);
	let gipResizing = $state(false);

	$effect(() => {
		if (!browser) return;
		const saved = Number(localStorage.getItem(GIP_WIDTH_KEY));
		if (Number.isFinite(saved) && saved >= GIP_WIDTH_MIN && saved <= GIP_WIDTH_MAX) {
			gipWidth = saved;
		}
	});

	function startGipResize(event: PointerEvent) {
		event.preventDefault();
		const startX = event.clientX;
		const startWidth = gipWidth ?? 340;
		gipResizing = true;
		document.body.style.cursor = 'col-resize';
		document.body.style.userSelect = 'none';
		const move = (e: PointerEvent) => {
			const next = Math.max(
				GIP_WIDTH_MIN,
				Math.min(GIP_WIDTH_MAX, Math.round(startWidth + (e.clientX - startX)))
			);
			gipWidth = next;
			if (browser) localStorage.setItem(GIP_WIDTH_KEY, String(next));
		};
		const up = () => {
			gipResizing = false;
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

	function onGipResizerKeydown(event: KeyboardEvent) {
		if (event.key === 'ArrowLeft') {
			event.preventDefault();
			const next = Math.max(GIP_WIDTH_MIN, Math.min(GIP_WIDTH_MAX, (gipWidth ?? 340) - 16));
			gipWidth = next;
			if (browser) localStorage.setItem(GIP_WIDTH_KEY, String(next));
		} else if (event.key === 'ArrowRight') {
			event.preventDefault();
			const next = Math.max(GIP_WIDTH_MIN, Math.min(GIP_WIDTH_MAX, (gipWidth ?? 340) + 16));
			gipWidth = next;
			if (browser) localStorage.setItem(GIP_WIDTH_KEY, String(next));
		}
	}
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

	let selectedMetric = $derived.by(() => {
		if (selectedMetricId == null) return null;
		return metrics.find((x) => x.id === selectedMetricId) ?? null;
	});

	// ---------- Keyword filter + metric name nav ----------
	let allKeywords = $derived.by(() => {
		const set = new Set<string>();
		for (const m of metrics) {
			for (const kw of m.metric_keywords ?? []) set.add(kw);
			for (const kw of m.metric_keywords_en ?? []) set.add(kw);
		}
		return [...set].sort((a, b) => a.localeCompare(b));
	});

	type AttrKind = 'text' | 'chips' | 'lines';
	type LineEntry = { head: string; content: string; lineType: string };
	type AttrDef = {
		key: string;
		label: string;
		icon: any;
		kind: AttrKind;
		value: string; // formatted value for `text` kind, joined for chips/lines summary
		items: string[]; // for `chips` and `lines` kinds (joined head + content for lines)
		entries: LineEntry[]; // structured per-line entries for `lines` kind
		count: number; // 1 for scalars with a value, items.length for lists, 0 if empty
		hasValue: boolean;
	};
	type SatelliteNode = AttrDef & {
		x: number;
		y: number;
		wire: { x1: number; y1: number; x2: number; y2: number };
	};
	type GroupNode = {
		key: string;
		label: string;
		icon: any;
		count: number; // total number of attributes in the group
		filledCount: number; // number of attributes with a value
		hasValue: boolean;
		x: number;
		y: number;
		wire: { x1: number; y1: number; x2: number; y2: number };
		satellites: SatelliteNode[];
		attrs: AttrDef[];
	};
	type MetricsCanvas = {
		W: number;
		H: number;
		Rc: number;
		Rgn: number;
		Rsn: number;
		cx: number;
		cy: number;
		metricLabel: string;
		metricSubLabel: string;
		groups: GroupNode[];
	};

	function buildMetricGroupAttrs(
		m: KbMetricRecord,
		spans: NormalizedSpan[],
		lineByKey: Map<string, RawLine>
	): {
		metadata: AttrDef[];
		context: AttrDef[];
		metric: AttrDef[];
		reasoning: AttrDef[];
		grounding: AttrDef[];
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
		const chipsAttr = (
			key: string,
			label: string,
			icon: any,
			items: string[],
			value: string
		): AttrDef => ({
			key,
			label,
			icon,
			kind: 'chips',
			value,
			items,
			entries: [],
			count: items.length,
			hasValue: items.length > 0
		});
		const linesAttr = (key: string, label: string, icon: any, entries: LineEntry[]): AttrDef => {
			const items = entries.map((e) => (e.content ? `${e.head}: ${e.content}` : e.head));
			return {
				key,
				label,
				icon,
				kind: 'lines',
				value: items.join('\n'),
				items,
				entries,
				count: entries.length,
				hasValue: entries.length > 0
			};
		};

		const kwItems = (m.metric_keywords ?? []).filter(
			(v) => typeof v === 'string' && v.trim() !== ''
		);
		const tags = (m.reasoning_tags ?? []).filter((v) => typeof v === 'string' && v.trim() !== '');

		const metadata: AttrDef[] = [
			textAttr('metric_id', 'ID', HashIcon, String(m.id), true),
			textAttr('name', 'Name', TypeIcon, fmt(m.metric_name), has(m.metric_name)),
			textAttr(
				'confidence',
				'Confidence',
				ActivityIcon,
				confidencePct(m.confidence),
				m.confidence != null
			),
			textAttr('desc', 'Desc', FileTextIcon, fmt(m.metric_desc), has(m.metric_desc)),
			textAttr(
				'formula',
				'Formula',
				HashIcon,
				fmt(m.formula_or_definition),
				has(m.formula_or_definition)
			),
			textAttr(
				'explicit',
				'Explicit',
				CalendarIcon,
				m.is_explicit_metric == null ? '' : m.is_explicit_metric ? 'true' : 'false',
				m.is_explicit_metric != null
			)
		];

		const context: AttrDef[] = [
			textAttr(
				'table_section',
				'Section',
				ListIcon,
				fmt(m.table_name_or_section),
				has(m.table_name_or_section)
			),
			textAttr('context', 'Context', BookOpenIcon, fmt(m.metric_context), has(m.metric_context)),
			chipsAttr('keywords', 'Keywords', TagIcon, kwItems, kwItems.join(', '))
		];

		const metric: AttrDef[] = [
			textAttr('subject', 'Subject', TypeIcon, fmt(m.metric_subject), has(m.metric_subject)),
			textAttr(
				'frequency',
				'Frequency',
				CalendarIcon,
				fmt(m.measurement_frequency),
				has(m.measurement_frequency)
			),
			textAttr('value', 'Value', TrendingUpIcon, fmt(m.metric_value), has(m.metric_value)),
			textAttr(
				'threshold',
				'Threshold',
				TrendingUpIcon,
				fmt(m.threshold_or_target),
				has(m.threshold_or_target)
			),
			textAttr('unit', 'Unit', HashIcon, fmt(m.metric_unit), has(m.metric_unit)),
			textAttr('value_class', 'Class', TagIcon, fmt(m.value_class), has(m.value_class)),
			textAttr(
				'value_data_type',
				'Data Type',
				ListIcon,
				fmt(m.value_data_type),
				has(m.value_data_type)
			),
			textAttr(
				'value_range_type',
				'Range Type',
				TrendingUpIcon,
				fmt(m.value_range_type),
				has(m.value_range_type)
			),
			textAttr('location_type', 'Location', MapPinIcon, fmt(m.location_type), has(m.location_type))
		];

		const reasoning: AttrDef[] = [
			chipsAttr('reasoning_tags', 'Tags', TagIcon, tags, tags.join(', '))
		];

		const groundingEntries: LineEntry[] = spans.flatMap((span) => {
			const rawLine = lineByKey.get(`${span.page_number}:${span.line_number}`);
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
			linesAttr('source_line_spans', 'Lines', FileTextIcon, groundingEntries)
		];

		return { metadata, context, metric, reasoning, grounding };
	}

	let metricsMap = $derived.by((): MetricsCanvas | null => {
		const W = canvasW,
			H = canvasH;
		if (W < 100 || H < 100) return null;
		const m = selectedMetric;
		if (!m) return null;
		const base = Math.min(W, H);
		const cx = W / 2,
			cy = H / 2;

		// Center disc, group node, and attribute-satellite radii (modeled after
		// the Scene Blocks chart in `kb-extraction-view.svelte`).
		const Rc = Math.max(54, Math.min(94, base * 0.115));
		const Rgn = Math.max(42, Math.min(58, base * 0.072));
		const Rsn = 21;

		// Distance from the center to each group node, and from each group node
		// to its attribute satellites — clamped so satellite labels stay inside
		// the canvas.
		const reachY = cy - 12 - Rsn - 34;
		const reachX = cx - 12 - 66;
		const reach = Math.max(120, Math.min(reachY, reachX));
		let Rs = Math.max(90, Math.min(200, base * 0.22));
		let Rg = Math.max(Rc + Rgn + 26, Math.min(330, base * 0.34, reach - Rs));
		if (reach - Rs < Rc + Rgn + 26) {
			Rs = Math.max(56, reach - (Rc + Rgn + 28));
			Rg = Rc + Rgn + 28;
		}

		const spans = normalizeMetricSpans(m);
		const attrsByGroup = buildMetricGroupAttrs(m, spans, rawLineByKey);

		type GroupSpec = {
			key: string;
			label: string;
			icon: any;
			angleDeg: number;
			attrs: AttrDef[];
		};

		// 5 groups evenly spaced around the metric center (every 72°), starting
		// at the top so the layout reads like the Scene Blocks chart.
		const groupSpecs: GroupSpec[] = [
			{
				key: 'g_metadata',
				label: 'Metadata',
				icon: BookOpenIcon,
				angleDeg: -90,
				attrs: attrsByGroup.metadata
			},
			{
				key: 'g_context',
				label: 'Context',
				icon: TagIcon,
				angleDeg: -18,
				attrs: attrsByGroup.context
			},
			{
				key: 'g_metric',
				label: 'Metric',
				icon: TrendingUpIcon,
				angleDeg: 54,
				attrs: attrsByGroup.metric
			},
			{
				key: 'g_grounding',
				label: 'Grounding',
				icon: MapPinIcon,
				angleDeg: 126,
				attrs: attrsByGroup.grounding
			},
			{
				key: 'g_reasoning',
				label: 'Reasoning',
				icon: ActivityIcon,
				angleDeg: 198,
				attrs: attrsByGroup.reasoning
			}
		];

		const groups: GroupNode[] = groupSpecs.map((spec) => {
			const ang = (spec.angleDeg * Math.PI) / 180;
			const ux = Math.cos(ang),
				uy = Math.sin(ang);
			const gx = cx + ux * Rg;
			const gy = cy + uy * Rg;
			const baseAng = Math.atan2(uy, ux);
			const k = spec.attrs.length;
			const span = k > 1 ? Math.min(Math.PI * 0.62, (k - 1) * 0.44) : 0;
			const satellites: SatelliteNode[] = spec.attrs.map((def, i) => {
				const sAng = baseAng + (k > 1 ? (i - (k - 1) / 2) * (span / (k - 1)) : 0);
				const sx = gx + Math.cos(sAng) * Rs;
				const sy = gy + Math.sin(sAng) * Rs;
				return {
					...def,
					x: sx,
					y: sy,
					wire: shortenLine(gx, gy, sx, sy, Rgn + 3, Rsn + 3)
				};
			});
			const filled = spec.attrs.filter((a) => a.hasValue).length;
			return {
				key: spec.key,
				label: spec.label,
				icon: spec.icon,
				count: spec.attrs.length,
				filledCount: filled,
				hasValue: filled > 0,
				x: gx,
				y: gy,
				wire: shortenLine(cx, cy, gx, gy, Rc + 5, Rgn + 3),
				satellites,
				attrs: spec.attrs
			};
		});

		return {
			W,
			H,
			Rc,
			Rgn,
			Rsn,
			cx,
			cy,
			metricLabel: m.metric_name?.trim() || m.metric_subject?.trim() || `Metric #${m.id}`,
			metricSubLabel: m.metric_name_en?.trim() || m.metric_subject_en?.trim() || '',
			groups
		};
	});

	let hoveredSatelliteInfo = $derived.by(() => {
		if (!hoveredCanvasAttr || !metricsMap) return null;
		for (const g of metricsMap.groups) {
			const s = g.satellites.find((sn) => sn.key === hoveredCanvasAttr);
			if (s) return { group: g, sat: s };
		}
		return null;
	});

	const ALL_METRIC_KEY = '__all_metric__';
	let activeGroup = $derived.by(() => {
		if (!activeGroupKey || activeGroupKey === ALL_METRIC_KEY || !metricsMap) return null;
		return metricsMap.groups.find((g) => g.key === activeGroupKey) ?? null;
	});
	let showAllMetricAttrs = $derived(activeGroupKey === ALL_METRIC_KEY && metricsMap != null);
	let totalAttrCount = $derived.by(() => {
		if (!metricsMap) return { filled: 0, total: 0 };
		let filled = 0,
			total = 0;
		for (const g of metricsMap.groups) {
			filled += g.filledCount;
			total += g.count;
		}
		return { filled, total };
	});

	let filteredMetrics = $derived.by(() => {
		let result = metrics;
		const kw = keywordFilter.trim().toLowerCase();
		if (kw) {
			result = result.filter(
				(m) =>
					(m.metric_keywords ?? []).some((k) => k.toLowerCase().includes(kw)) ||
					(m.metric_keywords_en ?? []).some((k) => k.toLowerCase().includes(kw)) ||
					(m.metric_name ?? '').toLowerCase().includes(kw) ||
					(m.metric_subject ?? '').toLowerCase().includes(kw)
			);
		}
		const cf = confidenceFilter.trim();
		if (cf) {
			if (cf.startsWith('<')) {
				const th = parseFloat(cf.slice(1).trim());
				if (Number.isFinite(th)) {
					result = result.filter((m) => (m.confidence ?? 0) < th);
				}
			} else {
				const th = parseFloat(cf);
				if (Number.isFinite(th)) {
					result = result.filter((m) => (m.confidence ?? 0) >= th);
				}
			}
		}
		return result;
	});

	let metricSearchActive = $derived(
		searchQuery.trim().length > 0 || hasKbMetricSearchFilters(searchFilters)
	);

	let selectedMetricInFilteredIndex = $derived(
		filteredMetrics.findIndex((m) => m.id === selectedMetricId)
	);
	let prevMetric = $derived(
		selectedMetricInFilteredIndex > 0 ? filteredMetrics[selectedMetricInFilteredIndex - 1] : null
	);
	let nextMetric = $derived(
		selectedMetricInFilteredIndex >= 0 && selectedMetricInFilteredIndex < filteredMetrics.length - 1
			? filteredMetrics[selectedMetricInFilteredIndex + 1]
			: null
	);

	// Keep selection in sync with filter changes — if the current metric falls
	// out of the filtered list, auto-select the first entry; if the list is
	// empty, clear the selection so the canvas shows the empty state.
	$effect(() => {
		const fm = filteredMetrics;
		if (selectedMetricId == null) return;
		if (fm.length === 0) {
			selectedMetricId = null;
		} else if (!fm.some((m) => m.id === selectedMetricId)) {
			selectedMetricId = fm[0].id;
		}
	});

	// Keep metric name dropdown in sync with selected metric.
	$effect(() => {
		metricNameDropdownValue = selectedMetricId ?? '';
	});

	function metricSourceRecordId(metric: KbMetricRecord | null): number | null {
		if (!metric) return null;
		const sourceRecordId = toPositiveInt(
			(metric as KbMetricRecord & { source_record_id?: unknown }).source_record_id
		);
		if (sourceRecordId) return sourceRecordId;
		return toPositiveInt(metric.input_record_id);
	}

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
		// Draw drag preview lines during active drag
		if (pdfDragPreviewLines.length > 0) {
			for (const ln of rawLines) {
				if (ln.page_number !== pageNo) continue;
				if (!pdfDragPreviewLines.includes(ln.line_number)) continue;
				if (!Array.isArray(ln.coords) || ln.coords.length < 4) continue;
				const [vx1, vy1, vx2, vy2] = viewport.convertToViewportRectangle(ln.coords.slice(0, 4));
				const left = Math.min(vx1, vx2);
				const top = Math.max(0, Math.min(vy1, vy2) - HIGHLIGHT_EXPAND_TOP_PX);
				const bottom = Math.max(vy1, vy2);
				const width = Math.abs(vx2 - vx1) + HIGHLIGHT_EXPAND_RIGHT_PX;
				const height = Math.max(0, bottom - top);
				if (width < 1 || height < 1) continue;
				const mark = document.createElement('div');
				mark.className = 'pdf-highlight-preview';
				mark.style.left = `${left}px`;
				mark.style.top = `${top}px`;
				mark.style.width = `${width}px`;
				mark.style.height = `${height}px`;
				mark.title = `line ${ln.line_number}`;
				overlay.appendChild(mark);
			}
		}
	}

	function handleDragSelect(
		ranges: Array<{
			pageNumber: number;
			viewportY1: number;
			viewportY2: number;
			viewport: PdfPageViewport;
		}>
	) {
		pdfDragPreviewLines = [];
		const selected: number[] = [];
		for (const { pageNumber, viewportY1, viewportY2, viewport } of ranges) {
			const pageLines = rawLines.filter((l) => l.page_number === pageNumber);
			for (const line of pageLines) {
				if (!Array.isArray(line.coords) || line.coords.length < 4) continue;
				const [, vy1, , vy2] = viewport.convertToViewportRectangle(line.coords.slice(0, 4));
				const lineTop = Math.min(vy1, vy2);
				const lineBottom = Math.max(vy1, vy2);
				if (Math.max(lineTop, viewportY1) <= Math.min(lineBottom, viewportY2)) {
					selected.push(line.line_number);
				}
			}
		}
		pdfSelectedLines = selected;
		addMetricBufferLines = selected;
		if (selected.length > 0) {
			resetAddMetricPreview();
			addMetricOpen = true;
		}
	}

	function handleDragMove(
		ranges: Array<{
			pageNumber: number;
			viewportY1: number;
			viewportY2: number;
			viewport: PdfPageViewport;
		}>
	) {
		const selected: number[] = [];
		for (const { pageNumber, viewportY1, viewportY2, viewport } of ranges) {
			const pageLines = rawLines.filter((l) => l.page_number === pageNumber);
			for (const line of pageLines) {
				if (!Array.isArray(line.coords) || line.coords.length < 4) continue;
				const [, vy1, , vy2] = viewport.convertToViewportRectangle(line.coords.slice(0, 4));
				const lineTop = Math.min(vy1, vy2);
				const lineBottom = Math.max(vy1, vy2);
				if (Math.max(lineTop, viewportY1) <= Math.min(lineBottom, viewportY2)) {
					selected.push(line.line_number);
				}
			}
		}
		pdfDragPreviewLines = selected;
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
		void loadMetricsForRecord(record.id);
	}

	function enterFocusMode() {
		if (recordBrowserFolded) return;
		recordBrowserFolded = true;
		onFocusModeChange?.(true);
	}

	function goBack() {
		if (!recordBrowserFolded) return;
		recordBrowserFolded = false;
		selectedMetricId = null;
		onFocusModeChange?.(false);
	}

	function goToPrevMetric() {
		if (prevMetric) void selectMetric(prevMetric);
	}

	function goToNextMetric() {
		if (nextMetric) void selectMetric(nextMetric);
	}

	function handleMetricNameDropdown(e: Event) {
		const id = Number((e.target as HTMLSelectElement).value);
		const m = metrics.find((x) => x.id === id);
		if (m) void selectMetric(m);
	}

	function shortenLine(x1: number, y1: number, x2: number, y2: number, d1: number, d2: number) {
		const dx = x2 - x1,
			dy = y2 - y1;
		const len = Math.sqrt(dx * dx + dy * dy);
		if (len < 1) return { x1, y1, x2, y2 };
		const ux = dx / len,
			uy = dy / len;
		return { x1: x1 + ux * d1, y1: y1 + uy * d1, x2: x2 - ux * d2, y2: y2 - uy * d2 };
	}

	async function loadMetricsForRecord(id: number) {
		keywordFilter = '';
		errorMsg = '';
		loading = true;
		metrics = [];
		selectedMetricId = null;
		highlightSelectionVersion = 0;
		rawLines = [];
		rawError = '';
		rawLoading = false;
		// Only clear currentInput if we're loading a different record.
		// Clearing it for the same record opens a race window where the
		// "expand menu → record-browser remount → auto-emit" path sees
		// currentInput=null and re-folds both panels (flash).
		if (currentInput?.id !== id) currentInput = null;
		addMetricOpen = false;
		addMetricBusyAction = null;
		resetAddMetricPreview();
		docPage = 1;
		pdfNumPages = 0;
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

	async function runMetricSearch() {
		searchError = '';
		searchHasRun = true;
		const params = buildKbMetricSearchParams({
			query: searchQuery,
			page: KB_METRIC_SEARCH_DEFAULTS.page,
			pageSize: KB_METRIC_SEARCH_DEFAULTS.pageSize,
			filters: searchFilters
		});
		if (!params.q) {
			searchResults = [];
			searchTotal = 0;
			searchError = 'Enter a query before searching metrics.';
			return;
		}
		searchLoading = true;
		try {
			const response = await searchKbMetrics(params);
			searchResults = response.results ?? [];
			searchTotal = response.total ?? 0;
		} catch (err) {
			searchResults = [];
			searchTotal = 0;
			searchError = err instanceof Error ? err.message : 'Failed to search metrics';
		} finally {
			searchLoading = false;
		}
	}

	function clearMetricSearch() {
		searchQuery = '';
		searchFilters = createEmptyKbMetricSearchFilters();
		searchResults = [];
		searchLoading = false;
		searchError = '';
		searchHasRun = false;
		searchTotal = 0;
	}

	async function handleMetricSearchResultClick(result: KbMetricSearchResult) {
		if (currentInput?.id !== result.input_record_id) {
			await loadMetricsForRecord(result.input_record_id);
		}
		const target = metrics.find((metric) => metric.id === result.id);
		if (!target) {
			errorMsg = `Metric ${result.id} was not found in input ${result.input_record_id}.`;
			return;
		}
		await selectMetric(target);
	}

	async function selectMetric(m: KbMetricRecord) {
		lastSelectedMetricDebug = `${m.id} @ ${new Date().toLocaleTimeString()}`;
		selectedMetricId = m.id;
		highlightSelectionVersion += 1;
		enterFocusMode();
		const first = normalizeMetricSpans(m)[0];
		if (!first) return;

		// Move display to the selected page without forcing iframe remount/reload.
		console.log('metric selected, id:' + m.id + ', page_num:' + first.page_number);
		docPage = first.page_number > 0 ? first.page_number : 1;

		// If user is on the lines panel, scroll the highlighted line into view.
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

	function onMetricCardClick(event: MouseEvent, m: KbMetricRecord) {
		// Defensive: avoid implicit form-submit or parent click handlers swallowing this action.
		event.preventDefault();
		event.stopPropagation();
		void selectMetric(m);
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
	function previewMetricNameOf(m: ExtractedKbMetric, index: number): string {
		return m.metric_name?.trim() || m.metric_subject?.trim() || `Metric ${index + 1}`;
	}
	function confidencePct(c?: number): string {
		if (c == null) return '—';
		return `${Math.round(c * 100)}%`;
	}
	function resetAddMetricPreview() {
		extractedMetricsPreview = [];
	}

	function closeAddMetricDialog() {
		addMetricOpen = false;
		addMetricEditKey = null;
		addMetricEditContent = '';
		addMetricBufferLines = [];
		addMetricBusyAction = null;
		resetAddMetricPreview();
	}

	function removeExtractedMetricPreview(index: number) {
		extractedMetricsPreview = extractedMetricsPreview.filter((_, idx) => idx !== index);
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

	// ---------- Add Metric dialog ----------
	// Reads from addMetricBufferLines (set during drag-select) instead of
	// pdfSelectedLines so the buffer survives click-away clearing.
	let addMetricDialogLines = $derived.by(() => {
		const seen = new Set<string>();
		const result: Array<RawLine & { key: string }> = [];
		for (const lineNo of addMetricBufferLines) {
			for (const ln of rawLines) {
				if (ln.line_number !== lineNo) continue;
				const key = `${ln.page_number}:${ln.line_number}`;
				if (seen.has(key)) continue;
				seen.add(key);
				result.push({ ...ln, key });
			}
		}
		return result.sort((a, b) => a.line_number - b.line_number);
	});

	function startEditDialogLine(key: string, content: string) {
		addMetricEditKey = key;
		addMetricEditContent = content;
	}

	function cancelEditDialogLine() {
		addMetricEditKey = null;
		addMetricEditContent = '';
	}

	async function saveEditDialogLine(pageNo: number, lineNo: number) {
		if (!currentInput || !addMetricEditKey) return;
		const confirmed = window.confirm('Save changes to the original file?');
		if (!confirmed) return;
		addMetricBusyAction = 'line';
		try {
			await updateRawLine({
				input_record_id: currentInput.id,
				page_number: pageNo,
				line_number: lineNo,
				content: addMetricEditContent
			});
			rawLines = rawLines.map((l) =>
				l.page_number === pageNo && l.line_number === lineNo
					? { ...l, content: addMetricEditContent }
					: l
			);
			addMetricEditKey = null;
			addMetricEditContent = '';
			addMetricBufferLines = [];
			resetAddMetricPreview();
		} catch (err) {
			alert(err instanceof Error ? err.message : 'Failed to save line');
		} finally {
			addMetricBusyAction = null;
		}
	}

	function deleteDialogLine(key: string) {
		const removedLines: number[] = [];
		for (const ln of addMetricDialogLines) {
			if (ln.key === key) removedLines.push(ln.line_number);
		}
		if (removedLines.length === 0) return;
		addMetricBufferLines = addMetricBufferLines.filter((ln) => !removedLines.includes(ln));
		resetAddMetricPreview();
	}

	let canAddPrevious = $derived.by(() => {
		if (addMetricBufferLines.length === 0 || rawLines.length === 0) return false;
		const minLine = Math.min(...addMetricBufferLines);
		return rawLines.some((ln) => ln.line_number < minLine);
	});
	let canAddNext = $derived.by(() => {
		if (addMetricBufferLines.length === 0 || rawLines.length === 0) return false;
		const maxLine = Math.max(...addMetricBufferLines);
		return rawLines.some((ln) => ln.line_number > maxLine);
	});

	function addPreviousLine() {
		if (addMetricBufferLines.length === 0) return;
		const minLine = Math.min(...addMetricBufferLines);
		const prev = rawLines
			.filter((ln) => ln.line_number < minLine && !addMetricBufferLines.includes(ln.line_number))
			.sort((a, b) => b.line_number - a.line_number)[0];
		if (prev) {
			addMetricBufferLines = [prev.line_number, ...addMetricBufferLines];
			resetAddMetricPreview();
		}
	}

	function addNextLine() {
		if (addMetricBufferLines.length === 0) return;
		const maxLine = Math.max(...addMetricBufferLines);
		const next = rawLines
			.filter((ln) => ln.line_number > maxLine && !addMetricBufferLines.includes(ln.line_number))
			.sort((a, b) => a.line_number - b.line_number)[0];
		if (next) {
			addMetricBufferLines = [...addMetricBufferLines, next.line_number];
			resetAddMetricPreview();
		}
	}

	async function extractMetric() {
		if (!currentInput || addMetricDialogLines.length === 0) return;
		addMetricBusyAction = 'extract';
		try {
			const nums = [...new Set(addMetricDialogLines.map((l) => l.line_number))].sort(
				(a, b) => a - b
			);
			const lineSpecs: string[] = [];
			let i = 0;
			while (i < nums.length) {
				let j = i;
				while (j + 1 < nums.length && nums[j + 1] === nums[j] + 1) j++;
				lineSpecs.push(i === j ? `${nums[i]}` : `${nums[i]}-${nums[j]}`);
				i = j + 1;
			}
			const result = await extractKbMetrics({
				record_id: currentInput.id,
				lines: lineSpecs
			});
			extractedMetricsPreview = result.metrics ?? [];
		} catch (err) {
			alert(err instanceof Error ? err.message : 'Failed to extract metrics');
		} finally {
			addMetricBusyAction = null;
		}
	}

	async function saveExtractedMetricsPreview() {
		if (!currentInput || extractedMetricsPreview.length === 0) return;
		addMetricBusyAction = 'save';
		try {
			await saveExtractedKbMetrics({
				record_id: currentInput.id,
				metrics: extractedMetricsPreview
			});
			const refreshed = await listKbMetrics(currentInput.id);
			metrics = refreshed.results ?? [];
			closeAddMetricDialog();
		} catch (err) {
			alert(err instanceof Error ? err.message : 'Failed to save metrics');
		} finally {
			addMetricBusyAction = null;
		}
	}

	async function extractProvision() {
		if (!currentInput || addMetricDialogLines.length === 0) return;
		addMetricBusyAction = 'provision';
		try {
			const spans: SourceLineSpan[] = addMetricDialogLines.map((l) => ({
				page_number: l.page_number,
				line_number: l.line_number
			}));
			await createKbProvision({
				input_record_id: currentInput.id,
				provision_name: `Provision from ${currentInput.id}`,
				source_line_spans: spans
			});
			closeAddMetricDialog();
		} catch (err) {
			alert(err instanceof Error ? err.message : 'Failed to create provision');
		} finally {
			addMetricBusyAction = null;
		}
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

	<div class="body" style={recordBrowserFolded ? 'grid-template-columns: minmax(0, 1fr)' : ''}>
		{#if !recordBrowserFolded}
			<KbInputRecordBrowser
				{darkMode}
				instanceKey="metrics-record-browser"
				title="kb.inputs"
				subtitle="Search, filter, and select input records before inspecting extracted metrics."
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
					<div class="left-meta-title">Metrics</div>
					<div class="left-meta-count">
						{#if metricSearchActive && searchHasRun}
							{searchTotal} global hit{searchTotal === 1 ? '' : 's'}
						{:else}
							{metrics.length} found
						{/if}
					</div>
				</div>
				<div class="debug-badge" aria-live="polite">
					Debug: last selected metric = {lastSelectedMetricDebug}
				</div>

				<div class="search-panel">
					<div class="search-panel-head">
						<div class="search-panel-title">Global Metric Search</div>
						<div class="search-panel-sub">
							Search the whole `kb.metrics` corpus with agent-friendly filters.
						</div>
					</div>
					<div class="search-grid">
						<input
							class="search-input search-query"
							type="text"
							placeholder="Search metrics, thresholds, units, keywords…"
							bind:value={searchQuery}
							onkeydown={(event) => {
								if (event.key === 'Enter') void runMetricSearch();
							}}
						/>
						<input
							class="search-input"
							type="text"
							placeholder="Record ID"
							bind:value={searchFilters.inputRecordId}
						/>
						<select class="search-input" bind:value={searchFilters.isExplicitMetric}>
							<option value="">Explicit metric?</option>
							<option value="true">Explicit only</option>
							<option value="false">Implicit only</option>
						</select>
						<input
							class="search-input"
							type="text"
							placeholder="Value class"
							bind:value={searchFilters.valueClass}
						/>
						<input
							class="search-input"
							type="text"
							placeholder="Value type"
							bind:value={searchFilters.valueDataType}
						/>
						<input
							class="search-input"
							type="text"
							placeholder="Metric unit"
							bind:value={searchFilters.metricUnit}
						/>
					</div>
					<div class="search-actions">
						<button
							type="button"
							class="search-btn primary"
							disabled={searchLoading}
							onclick={runMetricSearch}
						>
							{searchLoading ? 'Searching…' : 'Search'}
						</button>
						<button type="button" class="search-btn" onclick={clearMetricSearch}>Clear</button>
						<button
							type="button"
							class="search-btn"
							disabled={!currentInput}
							onclick={() => {
								searchFilters = {
									...searchFilters,
									inputRecordId: currentInput ? String(currentInput.id) : ''
								};
							}}>Use Current</button
						>
					</div>
					{#if searchError}
						<div class="search-status">{searchError}</div>
					{:else if metricSearchActive && searchHasRun}
						<div class="search-status">
							{searchTotal} result{searchTotal === 1 ? '' : 's'} for "{searchQuery.trim()}"
						</div>
					{/if}
				</div>

				<div class="metrics-list">
					{#if errorMsg}
						<div class="error">{errorMsg}</div>
					{:else if searchLoading}
						<div class="empty">
							<div class="empty-glyph">⌕</div>
							<div class="empty-title">Searching metrics</div>
							<div class="empty-sub">Ranking results across the full metrics corpus…</div>
						</div>
					{:else if metricSearchActive && searchHasRun && searchResults.length === 0}
						<div class="empty">
							<div class="empty-glyph">⌕</div>
							<div class="empty-title">No global matches</div>
							<div class="empty-sub">
								Try broader keywords or relax one of the semantic filters.
							</div>
						</div>
					{:else if !loading && metrics.length === 0}
						<div class="empty">
							<div class="empty-glyph">§</div>
							<div class="empty-title">No metrics yet</div>
							<div class="empty-sub">
								Select a record from kb.inputs to populate the metrics index.
							</div>
						</div>
					{:else if !loading && filteredMetrics.length === 0 && (keywordFilter || confidenceFilter)}
						<div class="empty">
							<div class="empty-glyph">§</div>
							<div class="empty-title">No matches</div>
							<div class="empty-sub">
								{#if keywordFilter && confidenceFilter}
									No metrics match keyword "{keywordFilter}" and confidence {confidenceFilter}.
								{:else if keywordFilter}
									No metrics match the keyword "{keywordFilter}".
								{:else}
									No metrics match confidence {confidenceFilter}.
								{/if}
							</div>
						</div>
					{:else if metricSearchActive && searchHasRun}
						{#each searchResults as result, idx (result.id)}
							<button
								type="button"
								class="metric-card"
								class:selected={selectedMetricId === result.id}
								onclick={() => handleMetricSearchResultClick(result)}
							>
								<div class="card-rule" aria-hidden="true"></div>
								<div class="card-body">
									<div class="card-row-top">
										<div class="card-index">⌕ {String(idx + 1).padStart(3, '0')}</div>
										<div class="card-conf" title="Search score">{result.score.toFixed(3)}</div>
									</div>
									<div class="card-name">{result.primary_label}</div>
									<div class="card-desc">{metricSearchResultSecondaryText(result)}</div>
									<div class="card-foot">
										<span class="chip">
											<span class="chip-dot"></span>
											record {result.input_record_id}
										</span>
										{#each metricSearchResultChips(result) as chip (`${result.id}-${chip}`)}
											<span class="chip chip-quiet">{chip}</span>
										{/each}
									</div>
								</div>
							</button>
						{/each}
					{:else}
						{#each filteredMetrics as m, idx (m.id)}
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
								value={metricNameDropdownValue}
								onchange={handleMetricNameDropdown}
								title="Jump to metric by name"
							>
								<option value="">— Metric by name —</option>
								{#each metrics as m (m.id)}
									<option value={m.id}>{metricNameOf(m)}</option>
								{/each}
							</select>
							<div class="toolbar-kw-wrap">
								<input
									class="toolbar-kw-input"
									type="text"
									list="metric-keywords-datalist-focus"
									placeholder="Filter by keyword…"
									bind:value={keywordFilter}
								/>
								<datalist id="metric-keywords-datalist-focus">
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
									list="confidence-options"
									placeholder="Confidence…"
									title="Filter by confidence threshold. Select or type a value like 0.85, or <0.50 for below-threshold."
									bind:value={confidenceFilter}
								/>
								<datalist id="confidence-options">
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
								disabled={!prevMetric}
								onclick={goToPrevMetric}
								title="Previous metric"><ChevronLeftIcon class="toolbar-icon" /></button
							>
							<span class="toolbar-nav-pos">
								{selectedMetricInFilteredIndex >= 0
									? `${selectedMetricInFilteredIndex + 1} / ${filteredMetrics.length}`
									: `— / ${filteredMetrics.length}`}
							</span>
							<button
								type="button"
								class="toolbar-nav-btn"
								disabled={!nextMetric}
								onclick={goToNextMetric}
								title="Next metric"><ChevronRightIcon class="toolbar-icon" /></button
							>
						</div>
					</div>
					<div class="metric-canvas" bind:clientWidth={canvasW} bind:clientHeight={canvasH}>
						{#if metricsMap}
							{@const map = metricsMap}
							<div class="attr-view">
								<div class="attr-view-header">
									<div class="attr-view-name">{map.metricLabel}</div>
									{#if map.metricSubLabel}
										<div class="attr-view-subname">{map.metricSubLabel}</div>
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
								<div class="canvas-empty-title">Select a metric</div>
								<div class="canvas-empty-sub">
									Click a metric from the list to view its attribute map.
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
							bind:showingLines={showLines}
							{darkMode}
							onselect={handleDragSelect}
							ondragmove={handleDragMove}
							bind:selectedLines={pdfSelectedLines}
						>
							{#snippet toolbar()}
								<button
									type="button"
									class="pvw-tool-btn"
									class:active={editLineMode}
									disabled={pdfSelectedLines.length === 0}
									onclick={() => {
										editLineMode = !editLineMode;
										if (editLineMode) deleteLineMode = false;
										editingLineKey = null;
									}}
									title="Edit Lines"><SquarePenIcon class="pvw-tb-icon" /></button
								>
								<button
									type="button"
									class="pvw-tool-btn"
									class:active={deleteLineMode}
									disabled={pdfSelectedLines.length === 0}
									onclick={() => {
										deleteLineMode = !deleteLineMode;
										if (deleteLineMode) editLineMode = false;
										editingLineKey = null;
									}}
									title="Delete Lines"><Trash2Icon class="pvw-tb-icon" /></button
								>
								<button
									type="button"
									class="pvw-tool-btn"
									class:active={addLineOpen}
									onclick={() => (addLineOpen = !addLineOpen)}
									title="Add Line"><ListPlusIcon class="pvw-tb-icon" /></button
								>
								<div class="pvw-tool-sep"></div>
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
												Click any metric on the left to jump to its source line.
											</div>
										</div>
									{:else}
										{#if addLineOpen}
											<div class="add-line-form">
												<span class="add-line-title">Insert Line</span>
												<select class="add-line-type-select" bind:value={newLineType}>
													<option value="text">text</option>
													<option value="title">title</option>
													<option value="header">header</option>
													<option value="list-item">list-item</option>
													<option value="footer">footer</option>
												</select>
												<input
													class="add-line-input"
													type="text"
													placeholder="Line content…"
													bind:value={newLineContent}
												/>
												<button
													type="button"
													class="add-line-save"
													onclick={() => {
														/* TODO: POST /kb/raw-lines when API is available */
														newLineContent = '';
														addLineOpen = false;
													}}>Add</button
												>
												<button
													type="button"
													class="add-line-cancel"
													onclick={() => {
														addLineOpen = false;
														newLineContent = '';
													}}>Cancel</button
												>
											</div>
										{/if}
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
															class:line-edit-active={editingLineKey === k}
														>
															<span class="line-no">{String(ln.line_number).padStart(4, '0')}</span>
															<span class="line-type">{ln.line_type}</span>
															{#if editLineMode && editingLineKey === k}
																<input
																	class="line-edit-input"
																	type="text"
																	bind:value={editingLineContent}
																	onkeydown={(e) => {
																		if (e.key === 'Enter') {
																			/* TODO: PATCH /kb/raw-lines when API is available */
																			rawLines = rawLines.map((l) =>
																				l.line_number === ln.line_number &&
																				l.page_number === ln.page_number
																					? { ...l, content: editingLineContent }
																					: l
																			);
																			editingLineKey = null;
																		} else if (e.key === 'Escape') {
																			editingLineKey = null;
																		}
																	}}
																/>
																<button
																	type="button"
																	class="line-edit-save"
																	onclick={() => {
																		/* TODO: PATCH /kb/raw-lines when API is available */
																		rawLines = rawLines.map((l) =>
																			l.line_number === ln.line_number &&
																			l.page_number === ln.page_number
																				? { ...l, content: editingLineContent }
																				: l
																		);
																		editingLineKey = null;
																	}}>✓</button
																>
																<button
																	type="button"
																	class="line-edit-cancel"
																	onclick={() => (editingLineKey = null)}>✕</button
																>
															{:else if editLineMode}
																<button
																	type="button"
																	class="line-content line-content-editable"
																	onclick={() => {
																		editingLineKey = k;
																		editingLineContent = ln.content;
																	}}>{ln.content}</button
																>
															{:else}
																<span class="line-content">{ln.content}</span>
															{/if}
															{#if deleteLineMode}
																<button
																	type="button"
																	class="line-delete-btn"
																	title="Delete line {ln.line_number}"
																	onclick={() => {
																		/* TODO: DELETE /kb/raw-lines when API is available */
																		rawLines = rawLines.filter(
																			(l) =>
																				!(
																					l.line_number === ln.line_number &&
																					l.page_number === ln.page_number
																				)
																		);
																	}}>×</button
																>
															{/if}
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

{#if addMetricOpen}
	<div
		class="dialog-overlay"
		aria-hidden="true"
		onclick={closeAddMetricDialog}
		onkeydown={(e) => {
			if (e.key === 'Escape') closeAddMetricDialog();
		}}
	>
		<div
			class="dialog am-dialog"
			role="dialog"
			aria-modal="true"
			aria-label="Add metric"
			tabindex="0"
			onclick={(e) => e.stopPropagation()}
			onkeydown={(e) => e.stopPropagation()}
		>
			<div class="dialog-head">
				<div>
					<div class="dialog-eyebrow">KB.Metrics</div>
					<h2 class="dialog-title">Add Metric</h2>
					<p class="dialog-subtitle">
						Review selected lines, extract candidate metrics, remove any you do not want, then save
						the remaining metrics to the database.
					</p>
				</div>
			</div>

			<div class="dialog-controls am-body">
				{#if addMetricDialogLines.length === 0}
					<div class="dialog-section am-empty-section">
						<div class="empty-glyph">§</div>
						<div class="empty-title">No lines selected</div>
						<div class="empty-sub">
							Drag to select lines on the PDF, then open this dialog again.
						</div>
					</div>
				{:else}
					<div class="dialog-section">
						<div class="dialog-section-head">
							<span class="dialog-section-title">Selected Lines</span>
							<span class="dialog-section-copy"
								>{addMetricDialogLines.length} line{addMetricDialogLines.length === 1 ? '' : 's'} selected</span
							>
							<button
								type="button"
								class="am-btn am-btn-head-add"
								disabled={!canAddPrevious}
								onclick={addPreviousLine}>+ Add</button
							>
						</div>
						<div class="am-table-wrap">
							<table class="am-table">
								<thead>
									<tr>
										<th class="am-col-line">Line #</th>
										<th class="am-col-page">Page</th>
										<th class="am-col-type">Type</th>
										<th class="am-col-content">Content</th>
										<th class="am-col-actions"></th>
									</tr>
								</thead>
								<tbody>
									{#each addMetricDialogLines as line (line.key)}
										<tr class="am-row">
											<td class="am-mono">{line.line_number}</td>
											<td class="am-mono">{line.page_number}</td>
											<td><span class="am-type-badge">{line.line_type}</span></td>
											<td class="am-content-cell">
												{#if addMetricEditKey === line.key}
													<input
														class="am-edit-input"
														type="text"
														bind:value={addMetricEditContent}
														onkeydown={(e) => {
															if (e.key === 'Escape') cancelEditDialogLine();
															if (e.key === 'Enter')
																saveEditDialogLine(line.page_number, line.line_number);
														}}
													/>
												{:else}
													<span class="am-content-text">{line.content}</span>
												{/if}
											</td>
											<td class="am-action-cell">
												{#if addMetricEditKey === line.key}
													<button
														type="button"
														class="am-btn am-btn-save"
														disabled={addMetricBusy}
														onclick={() => saveEditDialogLine(line.page_number, line.line_number)}
														>Save</button
													>
													<button
														type="button"
														class="am-btn am-btn-cancel-row"
														onclick={cancelEditDialogLine}>Cancel</button
													>
												{:else}
													<button
														type="button"
														class="am-btn am-btn-edit"
														onclick={() => startEditDialogLine(line.key, line.content)}>Edit</button
													>
													<button
														type="button"
														class="am-btn am-btn-delete"
														onclick={() => deleteDialogLine(line.key)}>Remove</button
													>
												{/if}
											</td>
										</tr>
									{/each}
								</tbody>
							</table>
							<div class="am-table-foot">
								<button
									type="button"
									class="am-btn am-btn-foot-add"
									disabled={!canAddNext}
									onclick={addNextLine}>+ Add</button
								>
							</div>
						</div>
					</div>

					<div class="dialog-section">
						<div class="dialog-section-head">
							<span class="dialog-section-title">Extracted Metrics</span>
							<span class="dialog-section-copy"
								>{extractedMetricsPreview.length} metric{extractedMetricsPreview.length === 1
									? ''
									: 's'} ready</span
							>
						</div>
						{#if addMetricBusyAction === 'extract'}
							<div class="am-status-row" aria-live="polite">
								<span class="am-spinner" aria-hidden="true"></span>
								<span>Extracting metrics from the selected lines…</span>
							</div>
						{:else if extractedMetricsPreview.length === 0}
							<div class="metadata-empty">
								Press <strong>Extract Metric</strong> to preview the metrics returned by the backend.
							</div>
						{:else}
							<div class="am-preview-list">
								{#each extractedMetricsPreview as metric, idx (`${previewMetricNameOf(metric, idx)}-${idx}`)}
									<div class="am-preview-card">
										<div class="am-preview-head">
											<div>
												<div class="am-preview-name">{previewMetricNameOf(metric, idx)}</div>
												<div class="am-preview-meta">
													<span>{confidencePct(metric.confidence)}</span>
													{#if metric.location_type}<span>{metric.location_type}</span>{/if}
													{#if metric.metric_unit}<span>{metric.metric_unit}</span>{/if}
												</div>
											</div>
											<button
												type="button"
												class="am-btn am-btn-delete"
												disabled={addMetricBusy}
												onclick={() => removeExtractedMetricPreview(idx)}>Remove</button
											>
										</div>
										{#if metric.metric_desc}
											<div class="am-preview-desc">{metric.metric_desc}</div>
										{/if}
										<div class="am-preview-fields">
											{#if metric.metric_value}<span class="chip chip-mono"
													>{metric.metric_value}</span
												>{/if}
											{#if metric.value_data_type}<span class="chip chip-quiet"
													>{metric.value_data_type}</span
												>{/if}
											{#if metric.table_name_or_section}<span class="chip"
													>{metric.table_name_or_section}</span
												>{/if}
										</div>
									</div>
								{/each}
							</div>
						{/if}
					</div>
				{/if}
			</div>

			<div class="dialog-foot">
				<div class="dialog-foot-hint">
					<button
						type="button"
						class="am-btn-foot am-btn-help"
						onclick={() => {
							alert(
								'Select PDF lines to extract a metric or provision.\n\n' +
									'• Drag on the PDF to select lines\n' +
									'• Edit: modify line content (saves to the original file)\n' +
									'• Remove: remove a line from this selection\n' +
									'• Extract Provision: create a new provision from the remaining lines\n' +
									'• Extract Metric: preview metrics returned from the backend\n' +
									'• Save: persist the remaining preview metrics to kb.metrics'
							);
						}}>Help</button
					>
				</div>
				<div class="dialog-foot-buttons">
					<button
						type="button"
						class="am-btn-foot am-btn-foot-cancel"
						onclick={closeAddMetricDialog}>Close</button
					>
					<button
						type="button"
						class="am-btn-foot am-btn-foot-extract dialog-search-btn"
						disabled={addMetricDialogLines.length === 0 || addMetricBusy}
						onclick={extractProvision}
						>{addMetricBusyAction === 'provision' ? 'Saving…' : 'Extract Provision'}</button
					>
					<button
						type="button"
						class="am-btn-foot am-btn-foot-extract dialog-search-btn"
						disabled={addMetricDialogLines.length === 0 || addMetricBusy}
						onclick={extractMetric}
					>
						{#if addMetricBusyAction === 'extract'}
							<span class="am-btn-inline"
								><span class="am-spinner" aria-hidden="true"></span>Extracting…</span
							>
						{:else}
							Extract Metric
						{/if}
					</button>
					<button
						type="button"
						class="am-btn-foot am-btn-foot-save dialog-search-btn"
						disabled={extractedMetricsPreview.length === 0 || addMetricBusy}
						onclick={saveExtractedMetricsPreview}
					>
						{#if addMetricBusyAction === 'save'}
							<span class="am-btn-inline"
								><span class="am-spinner" aria-hidden="true"></span>Saving…</span
							>
						{:else}
							Save
						{/if}
					</button>
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
	.left-meta-count {
		margin-left: auto;
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

	.search-panel {
		display: flex;
		flex-direction: column;
		gap: 10px;
		margin: 0 16px 14px;
		padding: 14px;
		border: 1px solid var(--ink-line);
		border-radius: 18px;
		background: linear-gradient(
			180deg,
			color-mix(in srgb, var(--panel-bg-alt) 88%, transparent),
			var(--panel-bg)
		);
	}

	.search-panel-head {
		display: flex;
		flex-direction: column;
		gap: 4px;
	}

	.search-panel-title {
		font: 600 0.95rem/1.2 var(--font-sans);
		color: var(--text-primary);
	}

	.search-panel-sub {
		font: 500 0.78rem/1.35 var(--font-sans);
		color: var(--text-muted);
	}

	.search-grid {
		display: grid;
		grid-template-columns: repeat(2, minmax(0, 1fr));
		gap: 8px;
	}

	.search-query {
		grid-column: 1 / -1;
	}

	.search-input {
		width: 100%;
		border-radius: 12px;
		border: 1px solid var(--ink-line);
		background: color-mix(in srgb, var(--panel-bg) 88%, transparent);
		color: var(--text-primary);
		padding: 10px 12px;
		font: 500 0.82rem/1.2 var(--font-sans);
	}

	.search-actions {
		display: flex;
		flex-wrap: wrap;
		gap: 8px;
	}

	.search-btn {
		border-radius: 999px;
		border: 1px solid var(--ink-line);
		background: transparent;
		color: var(--text-secondary);
		padding: 8px 12px;
		font: 600 0.76rem/1 var(--font-sans);
		cursor: pointer;
	}

	.search-btn.primary {
		border-color: color-mix(in srgb, var(--brass) 60%, var(--ink-line));
		background: var(--brassFaint);
		color: var(--text-primary);
	}

	.search-btn:disabled {
		opacity: 0.55;
		cursor: default;
	}

	.search-status {
		font: 500 0.76rem/1.35 var(--font-sans);
		color: var(--text-muted);
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
	/* ---- PDF toolbar buttons (rendered via snippet in PdfViewWindow's pvw-toolbar-pill) ---- */
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

	/* ---- Lines panel (shown via linesView snippet) ---- */
	.lines-panel {
		padding: 32px 36px 80px;
	}

	.line-edit-active .line-content {
		display: none;
	}
	.line-edit-input {
		flex: 1;
		min-width: 0;
		height: 22px;
		padding: 0 6px;
		border: 1px solid var(--brass);
		background: var(--panel-bg-alt);
		color: var(--text-primary);
		font-family: var(--font-mono, monospace);
		font-size: 11px;
		outline: none;
	}
	.line-edit-save,
	.line-edit-cancel {
		all: unset;
		cursor: pointer;
		padding: 0 6px;
		font-family: var(--font-mono, monospace);
		font-size: 12px;
		color: var(--text-secondary);
	}
	.line-edit-save:hover {
		color: var(--brass);
	}
	.line-edit-cancel:hover {
		color: var(--crimson, #c8553d);
	}

	.line-content-editable {
		all: unset;
		cursor: text;
		border-bottom: 1px dashed var(--ink-line);
		font-family: inherit;
		font-size: inherit;
		color: inherit;
	}
	.line-content-editable:hover {
		border-bottom-color: var(--brass);
		color: var(--brass);
	}

	.line-delete-btn {
		all: unset;
		cursor: pointer;
		margin-left: auto;
		padding: 0 6px;
		font-size: 14px;
		color: var(--text-muted);
		line-height: 1;
	}
	.line-delete-btn:hover {
		color: var(--crimson, #c8553d);
	}

	.add-line-form {
		display: flex;
		align-items: center;
		gap: 8px;
		padding: 10px 14px;
		background: var(--panel-bg-alt);
		border: 1px solid var(--ink-line);
		margin-bottom: 16px;
	}
	.add-line-title {
		font-family: var(--font-mono, monospace);
		font-size: 10px;
		text-transform: uppercase;
		letter-spacing: 0.08em;
		color: var(--text-muted);
		flex-shrink: 0;
	}
	.add-line-type-select {
		height: 26px;
		padding: 0 6px;
		border: 1px solid var(--ink-line);
		background: var(--panel-bg);
		color: var(--text-primary);
		font-family: var(--font-mono, monospace);
		font-size: 10px;
		text-transform: uppercase;
		cursor: pointer;
	}
	.add-line-input {
		flex: 1;
		min-width: 0;
		height: 26px;
		padding: 0 8px;
		border: 1px solid var(--ink-line);
		background: var(--panel-bg);
		color: var(--text-primary);
		font-family: var(--font-mono, monospace);
		font-size: 11px;
	}
	.add-line-input:focus {
		outline: none;
		border-color: var(--brass);
	}
	.add-line-save,
	.add-line-cancel {
		height: 26px;
		padding: 0 10px;
		border: 1px solid var(--ink-line);
		background: var(--panel-bg-alt);
		color: var(--text-secondary);
		font-family: var(--font-mono, monospace);
		font-size: 10px;
		text-transform: uppercase;
		letter-spacing: 0.07em;
		cursor: pointer;
	}
	.add-line-save:hover {
		border-color: var(--brass);
		color: var(--brass);
	}
	.add-line-cancel:hover {
		border-color: var(--ink-line);
		color: var(--text-primary);
	}

	/* ---- Add Metric dialog ---- */
	.am-dialog {
		min-width: 860px;
		min-height: 540px;
		max-width: 1100px;
		max-height: min(88vh, 900px);
	}
	.am-body {
		flex: 1 1 auto;
		overflow-y: auto;
		min-height: 0;
	}
	.am-empty-section {
		display: flex;
		flex-direction: column;
		align-items: center;
		padding: 48px 24px;
		gap: 8px;
		text-align: center;
	}
	.am-table-wrap {
		overflow-x: auto;
		border-radius: 12px;
		background: #13192280;
		border: 1px solid rgba(255, 255, 255, 0.05);
	}
	.am-table {
		width: 100%;
		border-collapse: collapse;
		font-size: 13px;
	}
	.am-table thead th {
		font-family: var(--font-mono);
		font-size: 10px;
		text-transform: uppercase;
		letter-spacing: 0.12em;
		color: var(--text-muted);
		text-align: left;
		padding: 10px 14px;
		border-bottom: 1px solid rgba(255, 255, 255, 0.07);
		background: #181d27;
		white-space: nowrap;
	}
	.am-table tbody tr {
		border-bottom: 1px solid rgba(255, 255, 255, 0.04);
		transition: background 100ms;
	}
	.am-table tbody tr:last-child {
		border-bottom: none;
	}
	.am-table tbody tr:hover {
		background: rgba(255, 255, 255, 0.03);
	}
	.am-table td {
		padding: 10px 14px;
		vertical-align: middle;
		color: var(--text-primary);
	}
	.am-col-line,
	.am-col-page {
		width: 64px;
	}
	.am-col-type {
		width: 130px;
	}
	.am-col-actions {
		width: 180px;
	}
	.am-mono {
		font-family: var(--font-mono);
		font-size: 12px;
		color: var(--text-muted);
	}
	.am-type-badge {
		display: inline-block;
		padding: 2px 8px;
		border-radius: 4px;
		font-family: var(--font-mono);
		font-size: 10px;
		letter-spacing: 0.06em;
		text-transform: lowercase;
		background: rgba(93, 175, 168, 0.12);
		color: var(--teal);
		border: 1px solid rgba(93, 175, 168, 0.2);
		white-space: nowrap;
	}
	.am-content-cell {
		max-width: 480px;
	}
	.am-content-text {
		display: block;
		line-height: 1.45;
		white-space: pre-wrap;
		word-break: break-word;
	}
	.am-action-cell {
		display: flex;
		gap: 6px;
		align-items: center;
		white-space: nowrap;
	}
	.am-edit-input {
		width: 100%;
		min-width: 200px;
		padding: 6px 10px;
		background: #1a202b;
		border: 1px solid rgba(212, 162, 76, 0.35);
		border-radius: 8px;
		color: var(--text-primary);
		font-family: inherit;
		font-size: 13px;
		outline: none;
	}
	.am-edit-input:focus {
		border-color: var(--brass);
		box-shadow: 0 0 0 2px rgba(212, 162, 76, 0.15);
	}
	.am-btn {
		height: 28px;
		padding: 0 10px;
		border-radius: 6px;
		font-family: var(--font-mono);
		font-size: 11px;
		letter-spacing: 0.06em;
		cursor: pointer;
		transition:
			background 120ms,
			border-color 120ms,
			color 120ms;
		white-space: nowrap;
	}
	.am-btn-edit {
		background: rgba(255, 255, 255, 0.06);
		border: 1px solid rgba(255, 255, 255, 0.14);
		color: var(--text-secondary);
	}
	.am-btn-edit:hover {
		background: rgba(255, 255, 255, 0.11);
		border-color: rgba(255, 255, 255, 0.24);
		color: var(--text-primary);
	}
	.am-btn-delete {
		background: rgba(200, 85, 61, 0.12);
		border: 1px solid rgba(200, 85, 61, 0.3);
		color: var(--crimson);
	}
	.am-btn-delete:hover {
		background: rgba(200, 85, 61, 0.22);
		border-color: rgba(200, 85, 61, 0.52);
	}
	.am-btn-save {
		background: rgba(212, 162, 76, 0.15);
		border: 1px solid rgba(212, 162, 76, 0.38);
		color: var(--brass);
	}
	.am-btn-save:hover:not(:disabled) {
		background: rgba(212, 162, 76, 0.26);
		border-color: var(--brass);
	}
	.am-btn-save:disabled {
		opacity: 0.5;
		cursor: not-allowed;
	}
	.am-btn-cancel-row {
		background: rgba(255, 255, 255, 0.04);
		border: 1px solid rgba(255, 255, 255, 0.1);
		color: var(--text-muted);
	}
	.am-btn-cancel-row:hover {
		background: rgba(255, 255, 255, 0.08);
		color: var(--text-secondary);
	}
	.am-btn-foot {
		height: 40px;
		padding: 0 18px;
		border-radius: 10px;
		font-family: var(--font-mono);
		font-size: 12px;
		letter-spacing: 0.08em;
		cursor: pointer;
		transition:
			background 120ms,
			border-color 120ms,
			color 120ms;
	}
	.am-btn-help {
		background: transparent;
		border: 1px solid rgba(255, 255, 255, 0.1);
		color: var(--text-muted);
	}
	.am-btn-help:hover {
		background: rgba(255, 255, 255, 0.05);
		color: var(--text-secondary);
	}
	.am-btn-foot-cancel {
		background: rgba(255, 255, 255, 0.05);
		border: 1px solid rgba(255, 255, 255, 0.14);
		color: var(--text-secondary);
	}
	.am-btn-foot-cancel:hover {
		background: rgba(255, 255, 255, 0.09);
		border-color: rgba(255, 255, 255, 0.22);
		color: var(--text-primary);
	}
	.am-btn-head-add {
		flex-shrink: 0;
		height: 24px;
		padding: 0 10px;
		border-radius: 6px;
		font-family: var(--font-mono);
		font-size: 10px;
		letter-spacing: 0.08em;
		text-transform: uppercase;
		cursor: pointer;
		background: rgba(93, 175, 168, 0.1);
		border: 1px solid rgba(93, 175, 168, 0.28);
		color: var(--teal);
		transition:
			background 120ms,
			border-color 120ms;
		margin-left: auto;
	}
	.am-btn-head-add:hover:not(:disabled) {
		background: rgba(93, 175, 168, 0.2);
		border-color: rgba(93, 175, 168, 0.5);
	}
	.am-btn-head-add:disabled {
		opacity: 0.4;
		cursor: not-allowed;
	}
	.am-table-foot {
		display: flex;
		justify-content: flex-end;
		padding: 8px 10px 6px;
		border-top: 1px solid rgba(255, 255, 255, 0.04);
		background: rgba(0, 0, 0, 0.08);
		border-radius: 0 0 12px 12px;
	}
	.am-btn-foot-add {
		height: 26px;
		padding: 0 12px;
		border-radius: 6px;
		font-family: var(--font-mono);
		font-size: 10px;
		letter-spacing: 0.08em;
		text-transform: uppercase;
		cursor: pointer;
		background: rgba(93, 175, 168, 0.1);
		border: 1px solid rgba(93, 175, 168, 0.28);
		color: var(--teal);
		transition:
			background 120ms,
			border-color 120ms;
	}
	.am-btn-foot-add:hover:not(:disabled) {
		background: rgba(93, 175, 168, 0.2);
		border-color: rgba(93, 175, 168, 0.5);
	}
	.am-btn-foot-add:disabled {
		opacity: 0.4;
		cursor: not-allowed;
	}
	.am-status-row {
		display: flex;
		align-items: center;
		gap: 10px;
		padding: 14px 16px;
		border-radius: 12px;
		background: rgba(212, 162, 76, 0.08);
		border: 1px solid rgba(212, 162, 76, 0.18);
		color: var(--text-secondary);
		font-size: 13px;
	}
	.am-spinner {
		width: 14px;
		height: 14px;
		border-radius: 999px;
		border: 2px solid rgba(212, 162, 76, 0.28);
		border-top-color: var(--brass);
		animation: am-spin 0.8s linear infinite;
		flex: 0 0 auto;
	}
	@keyframes am-spin {
		to {
			transform: rotate(360deg);
		}
	}
	.am-preview-list {
		display: flex;
		flex-direction: column;
		gap: 10px;
	}
	.am-preview-card {
		padding: 14px;
		border-radius: 14px;
		border: 1px solid rgba(212, 162, 76, 0.14);
		background: rgba(11, 16, 24, 0.46);
		display: flex;
		flex-direction: column;
		gap: 10px;
	}
	.am-preview-head {
		display: flex;
		justify-content: space-between;
		align-items: flex-start;
		gap: 12px;
	}
	.am-preview-name {
		font-family: var(--font-serif);
		font-size: 21px;
		line-height: 1.1;
		color: var(--text-primary);
	}
	.am-preview-meta {
		display: flex;
		flex-wrap: wrap;
		gap: 10px;
		margin-top: 6px;
		font-family: var(--font-mono);
		font-size: 11px;
		letter-spacing: 0.06em;
		text-transform: uppercase;
		color: var(--text-muted);
	}
	.am-preview-desc {
		font-size: 13px;
		line-height: 1.5;
		color: var(--text-secondary);
	}
	.am-preview-fields {
		display: flex;
		flex-wrap: wrap;
		gap: 8px;
	}
	.am-btn-inline {
		display: inline-flex;
		align-items: center;
		gap: 8px;
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
		background: #0a0d14;
	}
	:global(.pdf-highlight) {
		position: absolute;
		background: rgba(200, 85, 61, 0.18);
		border: 1px solid rgba(200, 85, 61, 0.9);
		box-shadow: inset 0 0 0 1px rgba(255, 210, 179, 0.22);
	}
	:global(.pdf-highlight-preview) {
		position: absolute;
		background: rgba(22, 163, 74, 0.16);
		border-left: 4px solid rgba(22, 163, 74, 0.85);
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

	/* ---------- Dialog ---------- */
	.dialog-overlay {
		position: fixed;
		inset: 0;
		display: flex;
		align-items: center;
		justify-content: center;
		z-index: 50;
		padding: 24px;
		background: rgba(2, 6, 23, 0.68);
		backdrop-filter: blur(10px);
	}
	.dialog {
		width: 100%;
		max-width: 1210px;
		max-height: min(90vh, 980px);
		display: flex;
		flex-direction: column;
		border-radius: 24px;
		overflow: auto;
		background: #111827;
		color: #f3eedf;
		border: 1px solid rgba(148, 163, 184, 0.16);
		box-shadow:
			0 30px 80px rgba(0, 0, 0, 0.55),
			0 0 0 1px rgba(212, 162, 76, 0.08);
		resize: both;
		min-width: 920px;
		min-height: 720px;
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
			linear-gradient(180deg, rgba(255, 255, 255, 0.04), rgba(255, 255, 255, 0.01)), #171c26;
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
	.am-btn-foot-save {
		background: rgba(93, 175, 168, 0.9) !important;
		border: 1px solid rgba(93, 175, 168, 0.95) !important;
		color: #081311 !important;
		box-shadow: 0 8px 20px rgba(93, 175, 168, 0.18);
	}
	.am-btn-foot-save:hover:not(:disabled) {
		background: #74c4bd !important;
		color: #081311 !important;
	}
	.am-btn-foot-save:disabled {
		background: rgba(93, 175, 168, 0.28) !important;
		color: rgba(8, 19, 17, 0.65) !important;
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
	.dialog::-webkit-scrollbar {
		width: 12px;
		height: 12px;
	}
	.dialog::-webkit-scrollbar-thumb {
		background: rgba(212, 162, 76, 0.34);
		border-radius: 999px;
		border: 2px solid transparent;
		background-clip: padding-box;
	}
	.dialog::-webkit-scrollbar-track {
		background: rgba(255, 255, 255, 0.03);
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
			align-items: stretch;
		}
		.dialog-foot-buttons {
			justify-content: stretch;
		}
		.dialog-foot-buttons :global(button) {
			flex: 1;
		}
	}

	/* ---- CANVAS (metric attribute map) ---- */
	.right.focus-split {
		flex-direction: row;
	}
	.metric-canvas-wrap {
		flex: 1 1 0;
		min-width: 0;
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
		width: 180px;
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
	.canvas-wires {
		position: absolute;
		inset: 0;
		pointer-events: none;
		overflow: visible;
	}
	.wire {
		stroke: var(--ink-line);
		stroke-width: 1.5;
		fill: none;
		opacity: 0.7;
		transition:
			stroke 160ms,
			opacity 160ms;
	}
	.wire.spoke {
		stroke: var(--brass);
		stroke-width: 1;
		opacity: 0.35;
		stroke-dasharray: 4 4;
	}
	.wire.active {
		stroke: var(--brass);
		opacity: 1;
	}
	.wire.is-empty {
		opacity: 0.2;
	}
	.canvas-node {
		position: absolute;
		transform: translate(-0%, -0%);
		border-radius: 50%;
		display: flex;
		flex-direction: column;
		align-items: center;
		justify-content: center;
		text-align: center;
		overflow: hidden;
	}
	.main-node {
		background: var(--panel-bg);
		border: 2px solid var(--ink-line);
		padding: 8px;
		cursor: default;
		z-index: 2;
	}
	.metric-node {
		all: unset;
		position: absolute;
		box-sizing: border-box;
		border-radius: 50%;
		display: flex;
		flex-direction: column;
		align-items: center;
		justify-content: center;
		text-align: center;
		overflow: hidden;
		background: var(--panel-bg);
		border: 2px solid var(--crimson);
		padding: 8px;
		cursor: pointer;
		z-index: 2;
		box-shadow: 0 0 24px rgba(200, 85, 61, 0.14);
		transition:
			box-shadow 140ms,
			border-color 140ms;
	}
	.metric-node:hover,
	.metric-node.active {
		border-color: var(--brass);
		box-shadow: 0 0 28px rgba(212, 162, 76, 0.3);
	}
	.group-node-circle {
		all: unset;
		cursor: pointer;
		position: absolute;
		border-radius: 50%;
		display: flex;
		flex-direction: column;
		align-items: center;
		justify-content: center;
		text-align: center;
		background: var(--panel-bg-alt);
		border: 2px solid var(--ink-line);
		color: var(--text-secondary);
		padding: 6px;
		transition:
			background 140ms,
			border-color 140ms,
			color 140ms,
			box-shadow 140ms;
		z-index: 2;
	}
	.group-node-circle:hover,
	.group-node-circle.active {
		background: var(--brass-faint);
		border-color: var(--brass);
		color: var(--brass);
		box-shadow: 0 0 18px rgba(212, 162, 76, 0.18);
	}
	.group-node-circle.is-empty {
		opacity: 0.45;
	}
	:global(.group-ic) {
		width: 18px;
		height: 18px;
		flex-shrink: 0;
		pointer-events: none;
		margin-bottom: 2px;
	}
	.group-node-label {
		font-family: var(--font-mono);
		font-size: 9px;
		letter-spacing: 0.08em;
		text-transform: uppercase;
		color: inherit;
		line-height: 1.1;
		max-width: 100%;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
		pointer-events: none;
	}
	.main-node-label {
		font-family: var(--font-serif);
		font-size: 12px;
		font-weight: 600;
		line-height: 1.2;
		color: var(--text-primary);
		max-width: 100%;
		overflow: hidden;
		display: -webkit-box;
		-webkit-line-clamp: 3;
		line-clamp: 3;
		-webkit-box-orient: vertical;
	}
	.main-node-sublabel {
		font-family: var(--font-mono);
		font-size: 9px;
		color: var(--text-muted);
		margin-top: 4px;
		max-width: 100%;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.main-node-cap {
		position: absolute;
		transform: translateX(-50%);
		font-family: var(--font-mono);
		font-size: 9px;
		letter-spacing: 0.14em;
		text-transform: uppercase;
		color: var(--text-muted);
		white-space: nowrap;
		pointer-events: none;
	}
	.sat-node {
		all: unset;
		cursor: pointer;
		position: absolute;
		border-radius: 50%;
		display: flex;
		align-items: center;
		justify-content: center;
		background: var(--panel-bg-alt);
		border: 1.5px solid var(--ink-line);
		color: var(--text-secondary);
		transition:
			background 140ms,
			border-color 140ms,
			color 140ms;
		z-index: 3;
	}
	.sat-node:hover,
	.sat-node.active {
		background: var(--brass-faint);
		border-color: var(--brass);
		color: var(--brass);
	}
	.sat-node.is-empty {
		opacity: 0.35;
	}
	:global(.sat-icon) {
		width: 13px;
		height: 13px;
		flex-shrink: 0;
		pointer-events: none;
	}
	.sat-badge {
		position: absolute;
		top: -4px;
		right: -4px;
		background: var(--brass);
		color: var(--page-bg);
		border-radius: 50%;
		width: 14px;
		height: 14px;
		display: flex;
		align-items: center;
		justify-content: center;
		font-family: var(--font-mono);
		font-size: 8px;
		font-weight: 700;
		line-height: 1;
		pointer-events: none;
	}
	.sat-label {
		position: absolute;
		transform: translateX(-50%);
		font-family: var(--font-mono);
		font-size: 9px;
		letter-spacing: 0.08em;
		text-transform: uppercase;
		color: var(--text-muted);
		white-space: nowrap;
		pointer-events: none;
		transition: color 140ms;
	}
	.sat-label.active {
		color: var(--brass);
	}
	.sat-label.is-empty {
		opacity: 0.35;
	}
	.canvas-legend {
		position: absolute;
		bottom: 16px;
		right: 16px;
		background: var(--panel-bg-alt);
		border: 1px solid var(--ink-line);
		padding: 10px 14px;
		font-family: var(--font-mono);
		font-size: 10px;
		color: var(--text-muted);
		pointer-events: none;
	}
	.legend-h {
		letter-spacing: 0.12em;
		text-transform: uppercase;
		color: var(--text-secondary);
		margin-bottom: 6px;
		font-size: 9px;
	}
	.legend-row {
		display: flex;
		align-items: center;
		gap: 6px;
		margin-bottom: 4px;
	}
	.lg {
		width: 10px;
		height: 10px;
		border-radius: 50%;
		flex-shrink: 0;
		border: 1.5px solid;
	}
	.lg-main {
		border-color: var(--crimson);
		background: var(--panel-bg);
	}
	.lg-group {
		border-color: var(--brass);
		background: var(--panel-bg-alt);
	}
	.lg-attr {
		border-color: var(--ink-line);
		background: var(--panel-bg-alt);
	}

	/* ---- Group info panel (shown when a group node is clicked) ---- */
	.group-info-panel {
		position: absolute;
		top: 14px;
		left: 14px;
		width: clamp(260px, 30%, 360px);
		max-height: calc(100% - 28px);
		display: flex;
		flex-direction: column;
		background: var(--panel-bg);
		border: 1px solid var(--ink-line);
		border-radius: 6px;
		box-shadow: 0 10px 28px rgba(0, 0, 0, 0.35);
		z-index: 12;
		overflow: hidden;
	}
	.gip-head {
		display: flex;
		align-items: center;
		gap: 8px;
		padding: 10px 12px;
		background: var(--panel-bg-alt);
		border-bottom: 1px solid var(--ink-line);
	}
	.gip-ic {
		display: inline-flex;
		color: var(--brass);
		flex-shrink: 0;
	}
	.gip-title {
		font-family: var(--font-serif);
		font-size: 14px;
		color: var(--text-primary);
		font-weight: 600;
		flex: 1 1 auto;
		min-width: 0;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.gip-count {
		font-family: var(--font-mono);
		font-size: 10px;
		color: var(--text-muted);
		letter-spacing: 0.06em;
	}
	.gip-close {
		all: unset;
		display: inline-flex;
		align-items: center;
		justify-content: center;
		width: 22px;
		height: 22px;
		border-radius: 4px;
		color: var(--text-muted);
		cursor: pointer;
		transition:
			color 140ms,
			background 140ms;
	}
	.gip-close:hover {
		color: var(--text-primary);
		background: var(--ink-line-soft, rgba(148, 163, 184, 0.16));
	}
	.gip-body {
		flex: 1 1 auto;
		overflow: auto;
		padding: 10px 12px;
		display: flex;
		flex-direction: column;
		gap: 10px;
	}
	.gip-section-head {
		display: flex;
		align-items: baseline;
		gap: 8px;
		margin-top: 6px;
		padding-bottom: 4px;
		border-bottom: 1px dashed var(--ink-line);
	}
	.gip-section-head:first-child {
		margin-top: 0;
	}
	.gip-section-label {
		font-family: var(--font-serif);
		font-size: 13px;
		color: var(--brass);
		font-weight: 600;
		letter-spacing: 0.02em;
	}
	.gip-section-count {
		font-family: var(--font-mono);
		font-size: 10px;
		color: var(--text-muted);
		letter-spacing: 0.06em;
		margin-left: auto;
	}
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
	.gip-resize-handle {
		all: unset;
		position: absolute;
		right: 0;
		top: 0;
		bottom: 0;
		width: 10px;
		cursor: col-resize;
		z-index: 15;
		touch-action: none;
		display: flex;
		align-items: center;
		justify-content: center;
		transition: background 140ms;
	}
	.gip-resize-handle:hover,
	.gip-resize-handle.active {
		background: var(--brass-faint);
	}
	.gip-resize-grip {
		width: 2px;
		height: 32px;
		border-radius: 1px;
		background: var(--ink-line);
		opacity: 0;
		transition: opacity 140ms;
		pointer-events: none;
	}
	.gip-resize-handle:hover .gip-resize-grip,
	.gip-resize-handle.active .gip-resize-grip {
		opacity: 1;
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
