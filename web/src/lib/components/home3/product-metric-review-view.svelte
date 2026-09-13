<script lang="ts">
	import { onMount } from 'svelte';
	import { browser } from '$app/environment';
	import { m } from '$lib/paraglide/messages.js';
	import {
		AlertTriangle,
		ChevronDown,
		ChevronRight,
		CircleDot,
		Download,
		FileText,
		FolderTree,
		Layers,
		Pencil,
		Plus,
		RefreshCw,
		Trash2
	} from '@lucide/svelte';
	import {
		acceptNode,
		addNode,
		deleteNode,
		getProfile,
		getReview,
		getRun,
		getMetricDetail,
		getObjectNames,
		getRunDiff,
		getRunDocuments,
		getRunResults,
		needsReconcileReview,
		rejectNode,
		rerunReview,
		runExportUrl,
		setProfileDrawing,
		updateNode,
		type MetricDetail,
		type Profile,
		type ProfileNode,
		type ResultRow,
		type ReviewRun,
		type RunDiff,
		type ScopedDocument
	} from '$lib/services/productMetricReviewService';
	import {
		composeDrawingPrompt,
		generateProductDrawing,
		ignoreProductDrawing,
		keepProductDrawing,
		pendingProductDrawingContentUrl,
		productDrawingContentUrl,
		type PendingProductDrawing
	} from '$lib/services/productDrawingService';
	import {
		getKbInput,
		getRawLines,
		listKbMetrics,
		type KbInputRecord,
		type KbMetricRecord,
		type RawLine
	} from '$lib/services/kbService';
	import {
		buildLineNumToPage,
		buildMetricGroupAttrs,
		normalizeMetricSpans,
		type AttrDef
	} from './metric-detail-groups';
	import PdfViewWindow from './pdf-view-window.svelte';
	import type { PdfPageViewport } from './shared-pdf-viewer.svelte';

	// `embedded`/`onBack`: rendered inside content-panel.svelte's app shell —
	// hide our own header (the shell already supplies breadcrumb/topbar) and
	// offer a way back to the intake view instead of relying on the browser's
	// Back button, which would leave the shell entirely.
	let {
		darkMode = false,
		embedded = false,
		initialRunId = null,
		onBack
	}: {
		darkMode?: boolean;
		embedded?: boolean;
		initialRunId?: number | null;
		onBack?: () => void;
	} = $props();

	// ── url params ────────────────────────────────────────────────────────────
	function param(name: string): string {
		if (typeof window === 'undefined') return '';
		return new URLSearchParams(window.location.search).get(name) ?? '';
	}
	let runIdInput = $state(initialRunId ? String(initialRunId) : param('run'));
	let runId = $state<number | null>(initialRunId ?? (param('run') ? Number(param('run')) : null));

	// ── loaded state ──────────────────────────────────────────────────────────
	let loading = $state(false);
	let error = $state('');
	let run = $state<ReviewRun | null>(null);
	let profileId = $state<number | null>(null);
	let profileVersion = $state<number | null>(null);
	let currentProfileVersion = $state<number | null>(null);
	let profileName = $state('');
	let profile = $state<Profile | null>(null);
	let nodes = $state<ProfileNode[]>([]);
	let results = $state<ResultRow[]>([]);
	let scopedDocs = $state<ScopedDocument[]>([]);
	let diff = $state<RunDiff | null>(null);

	// ── ui state ──────────────────────────────────────────────────────────────
	let tab = $state<'results' | 'report'>('results');
	let selectedNodeId = $state<number | null>(null);
	let tierFilter = $state<'' | 'direct' | 'part' | 'aspect' | 'document_scope'>('');
	let pathFilter = $state<'' | 'document_first' | 'direct'>('');
	let docFilter = $state<number | null>(null);
	let textFilter = $state('');
	let showDocumentScope = $state(false);
	let collapsed = $state<Record<number, boolean>>({});
	let addingUnder = $state<number | null>(null);
	let newLabel = $state('');
	let newKind = $state<'module' | 'part'>('part');
	let editingNode = $state<number | null>(null);
	let editLabel = $state('');
	let busyNode = $state<number | null>(null);

	// ── resizable layout (results tab) — drag handles between the drawing area
	// and the scope-tree / results / metric-details panes, sizes persisted per
	// browser (spec: product-review-results-layout; pattern mirrors
	// metric-ontology-explorer/panel-shell.svelte's splitter) ─────────────────
	type ReviewLayout = { drawingH: number; scopeW: number; detailW: number };
	const LAYOUT_KEY = 'pmr-review-layout';
	const LAYOUT_DEFAULTS: ReviewLayout = { drawingH: 320, scopeW: 320, detailW: 420 };
	const SCOPE_MIN = 240,
		SCOPE_MAX = 480;
	const DETAIL_MIN = 320,
		DETAIL_MAX = 900;
	const MAIN_MIN = -40;
	const DRAWING_MIN = 140,
		DRAWING_MAX = 940;

	function loadLayout(): ReviewLayout {
		if (!browser) return { ...LAYOUT_DEFAULTS };
		try {
			const raw = localStorage.getItem(LAYOUT_KEY);
			if (raw) {
				const p = JSON.parse(raw) as Partial<ReviewLayout>;
				return {
					drawingH: typeof p.drawingH === 'number' ? p.drawingH : LAYOUT_DEFAULTS.drawingH,
					scopeW: typeof p.scopeW === 'number' ? p.scopeW : LAYOUT_DEFAULTS.scopeW,
					detailW: typeof p.detailW === 'number' ? p.detailW : LAYOUT_DEFAULTS.detailW
				};
			}
		} catch {
			/* private mode / blocked / malformed — fall through to defaults */
		}
		return { ...LAYOUT_DEFAULTS };
	}

	let layout = $state<ReviewLayout>(loadLayout());
	let layoutEl: HTMLDivElement | null = $state(null);

	function persistLayout() {
		if (!browser) return;
		try {
			localStorage.setItem(LAYOUT_KEY, JSON.stringify(layout));
		} catch {
			/* ignore */
		}
	}

	const clamp = (v: number, lo: number, hi: number) => Math.max(lo, Math.min(hi, v));

	type ResizeKey = 'drawingH' | 'scopeW' | 'detailW';
	let drag: { key: ResizeKey; startX: number; startY: number; startVal: number } | null = null;

	function startResize(e: PointerEvent, key: ResizeKey) {
		(e.currentTarget as HTMLElement).setPointerCapture(e.pointerId);
		drag = { key, startX: e.clientX, startY: e.clientY, startVal: layout[key] };
		document.body.style.userSelect = 'none';
	}
	function moveResize(e: PointerEvent) {
		if (!drag || !layoutEl) return;
		const W = layoutEl.clientWidth;
		if (drag.key === 'scopeW') {
			const maxScope = Math.max(SCOPE_MIN, W - layout.detailW - MAIN_MIN - 12);
			layout.scopeW = clamp(drag.startVal + (e.clientX - drag.startX), SCOPE_MIN, Math.min(SCOPE_MAX, maxScope));
		} else if (drag.key === 'detailW') {
			const maxDetail = Math.max(DETAIL_MIN, W - layout.scopeW - MAIN_MIN - 12);
			layout.detailW = clamp(
				drag.startVal - (e.clientX - drag.startX),
				DETAIL_MIN,
				Math.min(DETAIL_MAX, maxDetail)
			);
		} else {
			layout.drawingH = clamp(drag.startVal + (e.clientY - drag.startY), DRAWING_MIN, DRAWING_MAX);
		}
	}
	function endResize(e: PointerEvent) {
		if (!drag) return;
		try {
			(e.currentTarget as HTMLElement).releasePointerCapture(e.pointerId);
		} catch {
			/* ignore */
		}
		drag = null;
		document.body.style.userSelect = '';
		persistLayout();
	}

	// ── product drawing (results tab) — generate/keep/ignore reuses the
	// existing System Admin "Generate 3D Product Drawings" flow; a kept
	// drawing is cached on the profile via drawing_id so it isn't regenerated
	// on every visit (spec: product-review-results-layout) ────────────────────
	let drawingId = $state<number | null>(null);
	$effect(() => {
		drawingId = profile?.drawing_id ?? null;
	});
	let drawingPrompt = $state('');
	let drawingPromptTouched = $state(false);
	let drawingModel = $state<'Qwen' | 'OpenAI'>('Qwen');
	let pendingDrawing = $state<PendingProductDrawing | null>(null);
	let drawingGenerating = $state(false);
	let drawingBusy = $state(false);
	let drawingError = $state('');
	let showDrawingGenerator = $state(false);

	// part-tier scope-tree labels for this product, excluding anything the
	// reviewer has rejected — fed into the backend's exploded-view template
	// so the drawing prompt names this product's actual components instead
	// of a generic/mismatched part list.
	function drawingComponents(): string[] {
		return nodes
			.filter((n) => n.node_kind === 'part' && n.status !== 'rejected')
			.map((n) => n.label)
			.filter(Boolean)
			.slice(0, 15);
	}

	async function refreshDrawingPrompt() {
		if (!profile) return;
		try {
			const { prompt } = await composeDrawingPrompt(profileName || 'product', drawingComponents());
			drawingPrompt = prompt;
		} catch (e) {
			drawingError = e instanceof Error ? e.message : String(e);
		}
	}

	$effect(() => {
		if (profile && !drawingPromptTouched) {
			void refreshDrawingPrompt();
		}
	});

	async function generateDrawing() {
		if (!drawingPrompt.trim()) return;
		drawingGenerating = true;
		drawingError = '';
		try {
			pendingDrawing = await generateProductDrawing({
				name: profileName || 'Product',
				description: profile?.product_description ?? '',
				prompt: drawingPrompt.trim(),
				keywords: (profile?.keywords ?? []).join(', '),
				notes: profile?.notes ?? '',
				model: drawingModel
			});
		} catch (e) {
			drawingError = e instanceof Error ? e.message : String(e);
		} finally {
			drawingGenerating = false;
		}
	}

	async function keepDrawing() {
		if (!pendingDrawing || !profileId) return;
		drawingBusy = true;
		drawingError = '';
		try {
			const kept = await keepProductDrawing(pendingDrawing.token);
			if (kept.id) {
				await setProfileDrawing(profileId, kept.id);
				drawingId = kept.id;
				if (profile) profile.drawing_id = kept.id;
			}
			pendingDrawing = null;
			showDrawingGenerator = false;
		} catch (e) {
			drawingError = e instanceof Error ? e.message : String(e);
		} finally {
			drawingBusy = false;
		}
	}

	async function discardDrawing() {
		if (!pendingDrawing) return;
		drawingBusy = true;
		try {
			await ignoreProductDrawing(pendingDrawing.token);
		} catch {
			/* best-effort — the pending file also expires on its own */
		} finally {
			pendingDrawing = null;
			drawingBusy = false;
		}
	}

	function openRegenerate() {
		pendingDrawing = null;
		drawingError = '';
		drawingPromptTouched = false;
		void refreshDrawingPrompt();
		showDrawingGenerator = true;
	}

	// Full-details dialog — shows the same curated Metric/Metadata/Context/
	// Grounding/Reasoning fields as the Metrics workspace (metric-mgmt-view.svelte)
	// for the selected result's metric, in-place, without leaving this page.
	let showFullDetailsDialog = $state(false);
	let fullDetailsLoading = $state(false);
	let fullDetailsError = $state('');
	let fullDetailsMetric = $state<KbMetricRecord | null>(null);
	let fullDetailsRequestId = 0;

	function openFullDetails(r: ResultRow | null) {
		if (!r) return;
		showFullDetailsDialog = true;
		fullDetailsError = '';
		fullDetailsMetric = null;
		fullDetailsLoading = true;
		const requestId = ++fullDetailsRequestId;
		listKbMetrics(r.input_record_id)
			.then((res) => {
				if (requestId !== fullDetailsRequestId) return; // superseded
				const target = (res.results ?? []).find((metric) => metric.metric_id === r.artifact_id);
				if (!target) {
					fullDetailsError = m.pmr_full_details_not_found();
					return;
				}
				fullDetailsMetric = target;
			})
			.catch((e) => {
				if (requestId !== fullDetailsRequestId) return;
				fullDetailsError = e instanceof Error ? e.message : String(e);
			})
			.finally(() => {
				if (requestId === fullDetailsRequestId) fullDetailsLoading = false;
			});
	}

	// The source document's raw lines are already loaded for the selected
	// result (to jump/highlight the PDF); reuse them for the Grounding section
	// instead of fetching again.
	let fullDetailsRawLines = $derived.by(() =>
		fullDetailsMetric && detailInputId === fullDetailsMetric.input_record_id ? detailRawLines : []
	);
	let fullDetailsGroups = $derived.by(() => {
		const metric = fullDetailsMetric;
		if (!metric) return null;
		const lineNumToPage = buildLineNumToPage(fullDetailsRawLines);
		const lineByKey = new Map<string, RawLine>();
		for (const ln of fullDetailsRawLines) lineByKey.set(`${ln.page_number}:${ln.line_number}`, ln);
		const spans = normalizeMetricSpans(metric, lineNumToPage);
		return buildMetricGroupAttrs(metric, spans, lineByKey);
	});

	function attrRows(attrs: AttrDef[]): FlatRow[] {
		const rows: FlatRow[] = [];
		for (const a of attrs) {
			if (a.kind === 'lines' && a.hasValue) {
				rows.push({ key: a.label, value: null, depth: 0 });
				for (const entry of a.entries) {
					const key = entry.lineType ? `${entry.head} (${entry.lineType})` : entry.head;
					rows.push({ key, value: entry.content || '—', depth: 1 });
				}
			} else {
				rows.push({ key: a.label, value: a.hasValue ? a.value : '—', depth: 0 });
			}
		}
		return rows;
	}

	// ── metric-details panel (results tab, right column) — selecting a result
	// row loads its source document's raw lines once (cached per document, so
	// clicking between rows of the same document doesn't refetch), jumps the
	// PDF to the page containing the matched lines, and highlights them ───────
	let selectedResult = $state<ResultRow | null>(null);
	let detailInputId = $state<number | null>(null);
	let detailInput = $state<KbInputRecord | null>(null);
	let detailRawLines = $state<RawLine[]>([]);
	let detailLoading = $state(false);
	let detailError = $state('');
	let metricDetailId = $state<string | null>(null);
	let metricDetail = $state<MetricDetail | null>(null);
	let detailDocPage = $state(1);
	let detailPdfZoom = $state(0.5);
	let detailPdfNumPages = $state(0);
	let detailHighlightVersion = $state(0);

	let detailFileUrl = $derived(
		detailInput ? `/api/v1/kb/inputs/${detailInput.id}/file#page=${detailDocPage}&zoom=page-width` : ''
	);
	let detailIsPdf = $derived((detailInput?.type ?? '').toLowerCase() === 'pdf');

	// source_line_spans is line-number spans only (e.g. "129", "117:119") —
	// same convention as kb.metrics.source_line_spans (metric-mgmt-view.svelte).
	function normalizeSpanLineNumbers(spans: unknown): number[] {
		if (!Array.isArray(spans)) return [];
		const out: number[] = [];
		for (const item of spans) {
			if (typeof item === 'number' && item > 0) {
				out.push(Math.trunc(item));
			} else if (typeof item === 'string') {
				const s = item.trim();
				const range = s.match(/^(\d+)\s*[:,-]\s*(\d+)$/);
				if (range) {
					const start = parseInt(range[1], 10);
					const end = parseInt(range[2], 10);
					for (let n = start; n <= end && n <= start + 200; n++) out.push(n);
				} else {
					const n = parseInt(s, 10);
					if (n > 0) out.push(n);
				}
			} else if (item && typeof item === 'object') {
				const obj = item as Record<string, unknown>;
				const l = obj.line_number ?? obj.line ?? obj.line_no ?? obj.lineNo;
				const n = typeof l === 'number' ? l : parseInt(String(l ?? ''), 10);
				if (Number.isFinite(n) && n > 0) out.push(n);
			}
		}
		return out;
	}

	function firstSpanPage(spans: unknown, lines: RawLine[]): number | null {
		const lineNums = normalizeSpanLineNumbers(spans);
		if (lineNums.length === 0) return null;
		const byLine = new Map<number, number>();
		for (const ln of lines) if (!byLine.has(ln.line_number)) byLine.set(ln.line_number, ln.page_number);
		for (const n of lineNums) {
			const p = byLine.get(n);
			if (p) return p;
		}
		return null;
	}

	let detailLineNumToPage = $derived.by(() => {
		const map = new Map<number, number>();
		for (const ln of detailRawLines) if (!map.has(ln.line_number)) map.set(ln.line_number, ln.page_number);
		return map;
	});
	let detailRawLineByKey = $derived.by(() => {
		const map = new Map<string, RawLine>();
		for (const ln of detailRawLines) map.set(`${ln.page_number}:${ln.line_number}`, ln);
		return map;
	});
	let detailSelectedLinesByPage = $derived.by(() => {
		const map = new Map<number, RawLine[]>();
		if (!selectedResult) return map;
		for (const lineNo of normalizeSpanLineNumbers(selectedResult.source_line_spans)) {
			const pageNo = detailLineNumToPage.get(lineNo);
			if (!pageNo) continue;
			const ln = detailRawLineByKey.get(`${pageNo}:${lineNo}`);
			if (ln && Array.isArray(ln.coords) && ln.coords.length >= 4) {
				const arr = map.get(pageNo) ?? [];
				arr.push(ln);
				map.set(pageNo, arr);
			}
		}
		return map;
	});

	async function selectResult(r: ResultRow) {
		selectedResult = r;
		detailHighlightVersion += 1;
		detailError = '';
		if (metricDetailId !== r.artifact_id) {
			metricDetailId = r.artifact_id;
			metricDetail = null;
			getMetricDetail(r.artifact_id)
				.then((res) => {
					if (metricDetailId === r.artifact_id) metricDetail = res.metric;
				})
				.catch(() => {
					if (metricDetailId === r.artifact_id) metricDetail = null;
				});
		}
		if (detailInputId !== r.input_record_id) {
			detailInputId = r.input_record_id;
			detailInput = null;
			detailRawLines = [];
			detailLoading = true;
			const [inputRes, rawRes] = await Promise.all([
				getKbInput(r.input_record_id).catch(() => null),
				getRawLines(r.input_record_id).catch(() => null)
			]);
			if (detailInputId !== r.input_record_id) return; // superseded by a newer selection
			detailInput = inputRes?.record ?? null;
			detailRawLines = rawRes?.lines ?? [];
			if (!detailInput) detailError = m.pmr_detail_load_failed();
			detailLoading = false;
		}
		detailDocPage = firstSpanPage(r.source_line_spans, detailRawLines) ?? 1;
	}

	function renderResultHighlights(pageNo: number, viewport: PdfPageViewport, overlay: HTMLDivElement) {
		const HIGHLIGHT_EXPAND_TOP_PX = 2;
		const HIGHLIGHT_EXPAND_RIGHT_PX = 20;
		const lines = detailSelectedLinesByPage.get(pageNo) ?? [];
		const rects = lines.flatMap((ln) => {
			if (!Array.isArray(ln.coords) || ln.coords.length < 4) return [];
			const vx1 = (ln.coords[0] * viewport.width) / 1000,
				vy1 = (ln.coords[1] * viewport.height) / 1000,
				vx2 = (ln.coords[2] * viewport.width) / 1000,
				vy2 = (ln.coords[3] * viewport.height) / 1000;
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
			const isContiguous = nextRect && nextRect.lineNumber === rect.lineNumber + 1;
			const bottom = isContiguous ? nextRect.top : rect.rawBottom;
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

	const staleVersion = $derived(
		profileVersion != null &&
			currentProfileVersion != null &&
			currentProfileVersion !== profileVersion
	);

	async function load() {
		if (!runId) return;
		loading = true;
		error = '';
		try {
			const r = await getRun(runId);
			run = r.run;
			const review = await getReview(r.run.request_id);
			profileId = review.request.profile_id;
			profileVersion = review.request.profile_version;
			const prof = await getProfile(profileId);
			profileName = prof.profile.name;
			currentProfileVersion = prof.profile.version;
			profile = prof.profile;
			nodes = prof.nodes;
			const [res, docs, d] = await Promise.all([
				getRunResults(runId),
				getRunDocuments(runId),
				getRunDiff(runId).catch(() => ({ diff: null as unknown as RunDiff }))
			]);
			results = res.results;
			scopedDocs = docs.documents;
			diff = d.diff;
		} catch (e) {
			error = e instanceof Error ? e.message : String(e);
		} finally {
			loading = false;
		}
	}

	async function reloadNodesAndResults() {
		if (!profileId || !runId) return;
		try {
			const [prof, res] = await Promise.all([getProfile(profileId), getRunResults(runId)]);
			nodes = prof.nodes;
			currentProfileVersion = prof.profile.version;
			profile = prof.profile;
			results = res.results;
		} catch (e) {
			error = e instanceof Error ? e.message : String(e);
		}
	}

	onMount(load);

	function openRun() {
		const n = Number(runIdInput);
		if (!Number.isFinite(n) || n <= 0) return;
		runId = n;
		if (typeof window !== 'undefined') {
			const u = new URL(window.location.href);
			u.searchParams.set('run', String(n));
			window.history.replaceState({}, '', u);
		}
		load();
	}

	async function doRerun() {
		if (!run) return;
		busyNode = -1;
		try {
			const out = await rerunReview(run.request_id);
			runId = out.run.id;
			runIdInput = String(out.run.id);
			if (typeof window !== 'undefined') {
				const u = new URL(window.location.href);
				u.searchParams.set('run', String(out.run.id));
				window.history.replaceState({}, '', u);
			}
			await load();
		} catch (e) {
			error = e instanceof Error ? e.message : String(e);
		} finally {
			busyNode = null;
		}
	}

	// ── scope tree ────────────────────────────────────────────────────────────
	type TreeNode = ProfileNode & { children: TreeNode[] };
	const tree = $derived.by(() => {
		const byId = new Map<number, TreeNode>();
		for (const n of nodes) byId.set(n.id, { ...n, children: [] });
		const roots: TreeNode[] = [];
		for (const n of byId.values()) {
			if (n.parent_node_id != null && byId.has(n.parent_node_id)) {
				byId.get(n.parent_node_id)!.children.push(n);
			} else {
				roots.push(n);
			}
		}
		const kindRank: Record<string, number> = { product: 0, module: 1, part: 2, aspect: 3 };
		const sortRec = (list: TreeNode[]) => {
			list.sort(
				(a, b) => (kindRank[a.node_kind] ?? 9) - (kindRank[b.node_kind] ?? 9) || a.id - b.id
			);
			list.forEach((c) => sortRec(c.children));
		};
		sortRec(roots);
		return roots;
	});

	const coverageByNode = $derived.by(() => {
		const map = new Map<number, { artifacts: number; docs: number }>();
		const rep = run?.report_json as
			| { coverage?: { node_id: number; artifact_count: number; document_count: number }[] }
			| undefined;
		for (const c of rep?.coverage ?? [])
			map.set(c.node_id, { artifacts: c.artifact_count, docs: c.document_count });
		return map;
	});

	const nodeLabel = $derived.by(() => {
		const map = new Map<number, string>();
		for (const n of nodes) map.set(n.id, n.label);
		return map;
	});

	const nodeObjectId = $derived.by(() => {
		const map = new Map<number, string>();
		for (const n of nodes) map.set(n.id, n.object_id);
		return map;
	});

	// object_id is a kb.object_nodes foreign key, not a display value — resolve
	// it to the object's canonical name whenever the node set changes.
	let objectName = $state<Map<string, string>>(new Map());
	$effect(() => {
		const ids = Array.from(new Set(nodes.map((n) => n.object_id).filter((id) => id)));
		if (ids.length === 0) {
			objectName = new Map();
			return;
		}
		getObjectNames(ids)
			.then((res) => {
				const map = new Map<string, string>();
				for (const o of res.objects) map.set(o.object_id, o.name || o.name_en || o.object_id);
				objectName = map;
			})
			.catch(() => {
				objectName = new Map();
			});
	});

	// ── results view ──────────────────────────────────────────────────────────
	const filtered = $derived.by(() => {
		const t = textFilter.trim().toLowerCase();
		return results.filter((r) => {
			if (selectedNodeId != null && r.node_id !== selectedNodeId) return false;
			if (tierFilter && r.tier !== tierFilter) return false;
			if (pathFilter && !r.paths.includes(pathFilter)) return false;
			if (docFilter != null && r.input_record_id !== docFilter) return false;
			if (
				t &&
				!`${r.primary_label} ${r.inclusion_reason} ${r.artifact_id}`.toLowerCase().includes(t)
			)
				return false;
			return true;
		});
	});

	const groups = $derived.by(() => {
		const attributed = new Map<number, ResultRow[]>();
		const scope: ResultRow[] = [];
		for (const r of filtered) {
			if (r.tier === 'document_scope' || r.node_id == null) {
				scope.push(r);
			} else {
				const arr = attributed.get(r.node_id) ?? [];
				arr.push(r);
				attributed.set(r.node_id, arr);
			}
		}
		const ordered = [...attributed.entries()].sort(
			(a, b) => b[1].length - a[1].length || a[0] - b[0]
		);
		return { ordered, scope };
	});

	const gaps = $derived.by(() => {
		const rep = run?.report_json as
			| { gaps?: { node_id: number; label: string; kind: string; grounding: string }[] }
			| undefined;
		return rep?.gaps ?? [];
	});

	function selectNode(id: number | null) {
		selectedNodeId = selectedNodeId === id ? null : id;
	}

	async function setStatus(node: ProfileNode, next: 'accepted' | 'rejected') {
		if (!profileId) return;
		busyNode = node.id;
		try {
			if (next === 'accepted') await acceptNode(profileId, node.id);
			else await rejectNode(profileId, node.id);
			await reloadNodesAndResults();
		} catch (e) {
			error = e instanceof Error ? e.message : String(e);
		} finally {
			busyNode = null;
		}
	}

	async function removeNode(node: ProfileNode) {
		if (!profileId) return;
		busyNode = node.id;
		try {
			await deleteNode(profileId, node.id);
			await reloadNodesAndResults();
		} catch (e) {
			error = e instanceof Error ? e.message : String(e);
		} finally {
			busyNode = null;
		}
	}

	function startRename(node: ProfileNode) {
		editingNode = editingNode === node.id ? null : node.id;
		editLabel = node.label;
	}

	async function submitRename(node: ProfileNode) {
		if (!profileId || !editLabel.trim() || editLabel.trim() === node.label) {
			editingNode = null;
			return;
		}
		busyNode = node.id;
		try {
			await updateNode(profileId, node.id, { label: editLabel.trim() });
			editingNode = null;
			await reloadNodesAndResults();
		} catch (e) {
			error = e instanceof Error ? e.message : String(e);
		} finally {
			busyNode = null;
		}
	}

	async function submitNewNode() {
		if (!profileId || addingUnder == null || !newLabel.trim()) return;
		busyNode = addingUnder;
		try {
			await addNode(profileId, {
				parent_node_id: addingUnder,
				node_kind: newKind,
				label: newLabel.trim()
			});
			newLabel = '';
			addingUnder = null;
			await reloadNodesAndResults();
		} catch (e) {
			error = e instanceof Error ? e.message : String(e);
		} finally {
			busyNode = null;
		}
	}

	// ── document dialog — shows the selected result's kb.inputs record (the
	// same "Record Fields" / "Doc Metadata" layout as the Upload Files view's
	// record viewer) instead of navigating to a page ─────────────────────────
	let showDocumentDialog = $state(false);

	type FlatRow = { key: string; value: string | null; depth: number };

	function tryParseJsonLike(v: unknown): unknown {
		if (typeof v !== 'string') return v;
		const t = v.trim();
		if ((t.startsWith('{') && t.endsWith('}')) || (t.startsWith('[') && t.endsWith(']'))) {
			try {
				return JSON.parse(t);
			} catch {
				/* not JSON */
			}
		}
		return v;
	}

	function flattenForDisplay(obj: unknown, depth = 0): FlatRow[] {
		const rows: FlatRow[] = [];
		if (obj === null || obj === undefined || typeof obj !== 'object') return rows;
		const entries: Array<[string, unknown]> = Array.isArray(obj)
			? (obj as unknown[]).map((v, i) => [`[${i}]`, v] as [string, unknown])
			: Object.entries(obj as Record<string, unknown>);
		for (const [k, v] of entries) {
			const parsed = tryParseJsonLike(v);
			if (parsed !== null && parsed !== undefined && typeof parsed === 'object') {
				const isArr = Array.isArray(parsed);
				const len = isArr ? (parsed as unknown[]).length : Object.keys(parsed as object).length;
				if (len === 0) {
					rows.push({ key: k, value: isArr ? '[]' : '{}', depth });
				} else if (isArr && (parsed as unknown[]).every((item) => item === null || typeof item !== 'object')) {
					rows.push({
						key: k,
						value: (parsed as unknown[]).map((item) => (item === null ? 'null' : String(item))).join(', '),
						depth
					});
				} else {
					rows.push({ key: k, value: null, depth });
					rows.push(...flattenForDisplay(parsed, depth + 1));
				}
			} else {
				rows.push({ key: k, value: v === null || v === undefined ? '—' : String(v).trim() || '—', depth });
			}
		}
		return rows;
	}

	let documentDialogRecordRows = $derived.by((): FlatRow[] => {
		if (!detailInput) return [];
		const filtered: Record<string, unknown> = {};
		for (const [k, v] of Object.entries(detailInput as unknown as Record<string, unknown>)) {
			if (k !== 'status' && k !== 'doc_metadata') filtered[k] = v;
		}
		return flattenForDisplay(filtered);
	});

	let documentDialogDocMeta = $derived.by((): FlatRow[] => {
		if (!detailInput) return [];
		let meta: unknown = detailInput.doc_metadata;
		if (typeof meta === 'string') {
			try {
				meta = JSON.parse(meta);
			} catch {
				return [];
			}
		}
		if (!meta || typeof meta !== 'object' || Array.isArray(meta)) return [];
		return flattenForDisplay(meta);
	});

	function fmtScore(s: number): string {
		return s.toFixed(3);
	}
	function spansText(spans: unknown): string {
		if (Array.isArray(spans))
			return spans.map((s) => (Array.isArray(s) ? s.join('–') : String(s))).join(', ');
		return typeof spans === 'string' ? spans : '';
	}
</script>

<div class="pmr-shell" class:dark={darkMode} class:embedded>
	{#if !embedded}
		<header class="topbar">
			<div class="brand">
				<span class="brand-mark">PMR</span>
				<div>
					<p class="kicker">{m.pmr_kicker()}</p>
					<p class="brand-name">{m.pmr_title()}</p>
				</div>
			</div>
			<div class="crumbs">
				{#if profileName}
					<span>{profileName}</span><span class="slash">/</span>
				{/if}
				{#if run}<strong>{m.pmr_run_n({ n: run.run_number })}</strong>{/if}
				<span class="route-badge">/home3/product-metric-review</span>
			</div>
		</header>
	{/if}

	<div class="content" class:wide={tab === 'results' && !!run}>
		{#if onBack}
			<button class="linky back" onclick={onBack}>← {m.pmr_back()}</button>
		{/if}
		{#if error}
			<div class="note error">{error}</div>
		{/if}

		{#if !runId}
			<div class="empty-hero">
				<Layers size={22} />
				<h1>{m.pmr_open_run_heading()}</h1>
				<p>{m.pmr_open_run_hint()}</p>
				<div class="run-open">
					<input
						type="number"
						min="1"
						placeholder={m.pmr_run_id_placeholder()}
						bind:value={runIdInput}
						onkeydown={(e) => e.key === 'Enter' && openRun()}
					/>
					<button class="primary" onclick={openRun}>{m.pmr_open()}</button>
				</div>
			</div>
		{:else if loading && !run}
			<div class="note">{m.pmr_loading()}</div>
		{:else if run}
			{#if staleVersion}
				<div class="note warn">
					<AlertTriangle size={14} />
					<span
						>{m.pmr_stale_version({
							shown: profileVersion ?? 0,
							current: currentProfileVersion ?? 0
						})}</span
					>
					<button class="linky" onclick={doRerun} disabled={busyNode === -1}>
						<RefreshCw size={12} />{m.pmr_rerun()}
					</button>
				</div>
			{/if}

			<div class="title-row">
				<div>
					<p class="kicker">{m.pmr_review_of()}</p>
					<h1>{profileName || m.pmr_untitled()}</h1>
					<p class="lede">
						{m.pmr_run_summary({
							status: run.status,
							attributed: run.attributed_count,
							scope: run.document_scope_count,
							docs: run.scoped_document_count
						})}
						{#if run.truncated_count > 0}
							· <span class="warn-text">{m.pmr_truncated({ n: run.truncated_count })}</span>
						{/if}
					</p>
				</div>
				<div class="title-actions">
					<a class="ghost" href={runExportUrl(runId)}><Download size={13} />{m.pmr_export()}</a>
					<button class="ghost" onclick={doRerun} disabled={busyNode === -1}>
						<RefreshCw size={13} />{m.pmr_rerun()}
					</button>
				</div>
			</div>

			<div class="tabs">
				<button class:active={tab === 'results'} onclick={() => (tab = 'results')}>
					<Layers size={13} />{m.pmr_tab_results()} <span>{results.length}</span>
				</button>
				<button class:active={tab === 'report'} onclick={() => (tab = 'report')}>
					<FileText size={13} />{m.pmr_tab_report()} <span>{gaps.length}</span>
				</button>
			</div>

			{#if tab === 'results' && profile}
				<div class="drawing-area" style="height:{layout.drawingH}px">
					{#if drawingId != null && !showDrawingGenerator}
						<div class="drawing-kept">
							<img src={productDrawingContentUrl(drawingId)} alt={profileName} />
							<button class="ghost drawing-regenerate" onclick={openRegenerate} disabled={drawingBusy}>
								<RefreshCw size={13} />{m.pmr_drawing_regenerate()}
							</button>
						</div>
					{:else if pendingDrawing}
						<div class="drawing-pending">
							<img src={pendingProductDrawingContentUrl(pendingDrawing.token)} alt={profileName} />
							<div class="drawing-pending-actions">
								<button class="primary" onclick={keepDrawing} disabled={drawingBusy}
									>{m.pmr_drawing_keep()}</button
								>
								<button class="ghost" onclick={discardDrawing} disabled={drawingBusy}
									>{m.pmr_drawing_discard()}</button
								>
							</div>
						</div>
					{:else}
						<div class="drawing-generator">
							<div class="pane-head"><span>{m.pmr_drawing_heading()}</span></div>
							<label class="drawing-prompt-label" for="pmr-drawing-prompt"
								>{m.pmr_drawing_prompt_label()}</label
							>
							<textarea
								id="pmr-drawing-prompt"
								rows="2"
								bind:value={drawingPrompt}
								oninput={() => (drawingPromptTouched = true)}
							></textarea>
							<div class="drawing-generator-actions">
								<label class="drawing-model-label" for="pmr-drawing-model"
									>{m.pmr_drawing_model_label()}</label
								>
								<select id="pmr-drawing-model" class="drawing-model-select" bind:value={drawingModel}>
									<option value="Qwen">Qwen · Aliyun</option>
									<option value="OpenAI">OpenAI · ChatGPT Image 2.5</option>
								</select>
								<button
									class="primary"
									onclick={generateDrawing}
									disabled={drawingGenerating || !drawingPrompt.trim()}
								>
									{drawingGenerating ? m.pmr_drawing_generating() : m.pmr_drawing_generate()}
								</button>
								{#if drawingId != null}
									<button class="linky" onclick={() => (showDrawingGenerator = false)}
										>{m.pmr_cancel()}</button
									>
								{/if}
							</div>
							{#if drawingError}
								<p class="drawing-error">{drawingError}</p>
							{/if}
						</div>
					{/if}
				</div>
				<div
					class="h-splitter"
					role="separator"
					aria-orientation="horizontal"
					onpointerdown={(e) => startResize(e, 'drawingH')}
					onpointermove={moveResize}
					onpointerup={endResize}
					onpointercancel={endResize}
				></div>
			{/if}

			<div
				class="layout"
				class:report-mode={tab === 'report'}
				class:resizable={tab === 'results'}
				bind:this={layoutEl}
			>
				<!-- scope tree -->
				<aside class="scope-pane" style={tab === 'results' ? `width:${layout.scopeW}px` : ''}>
					<div class="pane-head">
						<FolderTree size={13} /><span>{m.pmr_scope_tree()}</span>
						{#if selectedNodeId != null}
							<button class="linky" onclick={() => (selectedNodeId = null)}>{m.pmr_clear()}</button>
						{/if}
					</div>
					<div class="tree">
						{#each tree as root (root.id)}
							{@render treeNode(root, 0)}
						{/each}
					</div>
				</aside>

				{#if tab === 'results'}
					<div
						class="v-splitter"
						role="separator"
						aria-orientation="vertical"
						onpointerdown={(e) => startResize(e, 'scopeW')}
						onpointermove={moveResize}
						onpointerup={endResize}
						onpointercancel={endResize}
					></div>
				{/if}

				<!-- main -->
				<section
					class="main-pane"
					style={tab === 'results'
						? `width:calc(100% - ${layout.scopeW + layout.detailW + 412}px)`
						: ''}
				>
					{#if tab === 'results'}
						<div class="filters">
							<select bind:value={tierFilter}>
								<option value="">{m.pmr_all_tiers()}</option>
								<option value="direct">direct</option>
								<option value="part">part</option>
								<option value="aspect">aspect</option>
								<option value="document_scope">document_scope</option>
							</select>
							<select bind:value={pathFilter}>
								<option value="">{m.pmr_all_paths()}</option>
								<option value="direct">direct</option>
								<option value="document_first">document_first</option>
							</select>
							<select bind:value={docFilter}>
								<option value={null}>{m.pmr_all_documents()}</option>
								{#each scopedDocs as d (d.input_record_id)}
									<option value={d.input_record_id}
										>#{d.input_record_id}{d.doc_kind ? ` · ${d.doc_kind}` : ''}</option
									>
								{/each}
							</select>
							<input type="search" placeholder={m.pmr_filter_text()} bind:value={textFilter} />
						</div>

						{#if filtered.length === 0}
							<div class="empty-state">
								<p>{m.pmr_no_results()}</p>
								{#if gaps.length > 0}
									<button class="linky" onclick={() => (tab = 'report')}
										>{m.pmr_see_gaps({ n: gaps.length })}</button
									>
								{/if}
							</div>
						{:else}
							{#each groups.ordered as [nodeId, rows] (nodeId)}
								<div class="result-group">
									<button
										class="group-head"
										onclick={() => selectNode(nodeId)}
										class:sel={selectedNodeId === nodeId}
									>
										<span class="g-label">{nodeLabel.get(nodeId) ?? `#${nodeId}`}</span>
										<span class="g-count">{rows.length}</span>
									</button>
									{#each rows as r (r.artifact_id)}
										{@render resultRow(r)}
									{/each}
								</div>
							{/each}

							{#if groups.scope.length > 0}
								<div class="result-group scope-group">
									<button
										class="group-head"
										onclick={() => (showDocumentScope = !showDocumentScope)}
									>
										{#if showDocumentScope}<ChevronDown size={13} />{:else}<ChevronRight
												size={13}
											/>{/if}
										<span class="g-label">{m.pmr_document_scope()}</span>
										<span class="g-count">{groups.scope.length}</span>
									</button>
									{#if showDocumentScope}
										{#each groups.scope as r (r.artifact_id)}
											{@render resultRow(r)}
										{/each}
									{/if}
								</div>
							{/if}
						{/if}
					{:else}
						{@render reportView()}
					{/if}
				</section>

				{#if tab === 'results'}
					<div
						class="v-splitter"
						role="separator"
						aria-orientation="vertical"
						onpointerdown={(e) => startResize(e, 'detailW')}
						onpointermove={moveResize}
						onpointerup={endResize}
						onpointercancel={endResize}
					></div>

					<!-- metric details -->
					<aside class="detail-pane" style="width:{layout.detailW}px">
						<div class="detail-attrs">
							<div class="pane-head">
								<FileText size={13} /><span>{m.pmr_detail_heading()}</span>
							</div>
							{#if !selectedResult}
								<div class="detail-empty">
									<p>{m.pmr_detail_empty()}</p>
								</div>
							{:else}
								<p class="kicker">{selectedResult.tier} · {selectedResult.artifact_type}</p>
								<h3 class="detail-title">
									{selectedResult.primary_label || selectedResult.artifact_id}
								</h3>
								<dl class="drawer-fields">
									<dt>{m.pmr_field_artifact()}</dt>
									<dd class="mono">{selectedResult.artifact_id}</dd>
									<dt>{m.pmr_field_metric_name()}</dt>
									<dd>{metricDetail?.metric_name || '—'}</dd>
									<dt>{m.pmr_field_subject()}</dt>
									<dd>{metricDetail?.subject || '—'}</dd>
									<dt>{m.pmr_field_object()}</dt>
									<dd>
										{selectedResult.node_id == null
											? m.pmr_none()
											: objectName.get(nodeObjectId.get(selectedResult.node_id) ?? '') || '—'}
									</dd>
									<dt>{m.pmr_field_document()}</dt>
									<dd>
										<button
											type="button"
											class="field-link"
											onclick={() => (showDocumentDialog = true)}
										>
											#{selectedResult.input_record_id}
										</button>
									</dd>
									<dt>{m.pmr_field_matched_name()}</dt>
									<dd>
										{selectedResult.node_id != null
											? (nodeLabel.get(selectedResult.node_id) ?? `#${selectedResult.node_id}`)
											: m.pmr_none()}
									</dd>
									<dt>{m.pmr_field_paths()}</dt>
									<dd class="mono">{selectedResult.paths.join(' · ') || '—'}</dd>
									<dt>{m.pmr_field_value()}</dt>
									<dd>{metricDetail?.value || '—'}</dd>
									<dt>{m.pmr_field_threshold()}</dt>
									<dd>{metricDetail?.threshold || '—'}</dd>
									<dt>{m.pmr_field_unit()}</dt>
									<dd>{metricDetail?.unit || '—'}</dd>
									<dt>{m.pmr_field_frequency()}</dt>
									<dd>{metricDetail?.frequency || '—'}</dd>
									<dt>{m.pmr_field_class()}</dt>
									<dd>{metricDetail?.class || '—'}</dd>
									<dt>{m.pmr_field_data_type()}</dt>
									<dd>{metricDetail?.data_type || '—'}</dd>
									<dt>{m.pmr_field_range_type()}</dt>
									<dd>{metricDetail?.range_type || '—'}</dd>
									<dt>{m.pmr_field_score()}</dt>
									<dd class="mono">{fmtScore(selectedResult.score)}</dd>
									<dt>{m.pmr_field_line_spans()}</dt>
									<dd class="mono">{spansText(selectedResult.source_line_spans) || '—'}</dd>
									<dt>{m.pmr_field_reason()}</dt>
									<dd>{selectedResult.inclusion_reason}</dd>
								</dl>
								<button
									type="button"
									class="ghost full-details-btn"
									onclick={() => openFullDetails(selectedResult)}
								>
									{m.pmr_full_details_button()}
								</button>
							{/if}
						</div>
						<div class="detail-pdf">
							{#if !selectedResult}
								<!-- nothing to show until a result is selected -->
							{:else if detailLoading}
								<div class="note">{m.pmr_loading()}</div>
							{:else if detailError}
								<div class="note error">{detailError}</div>
							{:else if detailIsPdf && detailInput}
								<PdfViewWindow
									inputId={detailInput.id}
									fileUrl={detailFileUrl}
									bind:page={detailDocPage}
									bind:zoom={detailPdfZoom}
									bind:numPages={detailPdfNumPages}
									highlightVersion={`${selectedResult.artifact_id}:${detailHighlightVersion}`}
									renderHighlights={renderResultHighlights}
									enableSelectionDialog={false}
									showSidebar={false}
									{darkMode}
								/>
							{:else}
								<div class="note">{m.pmr_detail_pdf_unavailable()}</div>
							{/if}
						</div>
					</aside>
				{/if}
			</div>
		{/if}
	</div>

	{#if showDocumentDialog && detailInput}
	<div
		class="doc-dialog-overlay"
		onmousedown={(e) => {
			if (e.target === e.currentTarget) showDocumentDialog = false;
		}}
		onkeydown={(e) => {
			if (e.key === 'Escape') showDocumentDialog = false;
		}}
		role="button"
		tabindex="0"
	>
		<div
			class="doc-dialog"
			onmousedown={(e) => e.stopPropagation()}
			onkeydown={(e) => e.stopPropagation()}
			role="dialog"
			aria-modal="true"
			aria-label={m.pmr_field_document()}
			tabindex="0"
		>
			<div class="doc-dialog-head">
				<h3>Record ID: {detailInput.id}</h3>
				<button type="button" class="ghost" onclick={() => (showDocumentDialog = false)}>
					{m.pmr_close()}
				</button>
			</div>
			<div class="doc-dialog-body">
				<div>
					<div class="doc-dialog-section-title">{m.pmr_record_fields()}</div>
					<div class="doc-dialog-rows">
						{#each documentDialogRecordRows as row}
							<div class="doc-dialog-row" style="padding-left:{row.depth * 16}px">
								<span class="doc-dialog-key">{row.key}</span>
								{#if row.value !== null}
									<span class="doc-dialog-value">{row.value}</span>
								{/if}
							</div>
						{/each}
					</div>
				</div>
				<div>
					<div class="doc-dialog-section-title">
						{m.pmr_doc_metadata({ n: documentDialogDocMeta.filter((r) => r.depth === 0).length })}
					</div>
					{#if documentDialogDocMeta.length === 0}
						<div class="muted">{m.pmr_doc_metadata_empty()}</div>
					{:else}
						<div class="doc-dialog-rows">
							{#each documentDialogDocMeta as row}
								<div class="doc-dialog-row" style="padding-left:{row.depth * 16}px">
									<span class="doc-dialog-key">{row.key}</span>
									{#if row.value !== null}
										<span class="doc-dialog-value">{row.value}</span>
									{/if}
								</div>
							{/each}
						</div>
					{/if}
				</div>
			</div>
		</div>
	</div>
	{/if}

	{#if showFullDetailsDialog}
		<div
			class="doc-dialog-overlay"
			onmousedown={(e) => {
				if (e.target === e.currentTarget) showFullDetailsDialog = false;
			}}
			onkeydown={(e) => {
				if (e.key === 'Escape') showFullDetailsDialog = false;
			}}
			role="button"
			tabindex="0"
		>
			<div
				class="doc-dialog"
				onmousedown={(e) => e.stopPropagation()}
				onkeydown={(e) => e.stopPropagation()}
				role="dialog"
				aria-modal="true"
				aria-label={m.pmr_full_details_button()}
				tabindex="0"
			>
				<div class="doc-dialog-head">
					<h3>{fullDetailsMetric?.metric_name || selectedResult?.artifact_id || ''}</h3>
					<button type="button" class="ghost" onclick={() => (showFullDetailsDialog = false)}>
						{m.pmr_close()}
					</button>
				</div>
				<div class="doc-dialog-body">
					{#if fullDetailsLoading}
						<div class="note">{m.pmr_full_details_loading()}</div>
					{:else if fullDetailsError}
						<div class="note">{fullDetailsError}</div>
					{:else if fullDetailsGroups}
						{@const groups = fullDetailsGroups}
						{#each [{ label: m.pmr_group_metric(), attrs: groups.metric }, { label: m.pmr_group_metadata(), attrs: groups.metadata }, { label: m.pmr_group_context(), attrs: groups.context }, { label: m.pmr_group_grounding(), attrs: groups.grounding }, { label: m.pmr_group_reasoning(), attrs: groups.reasoning }] as group (group.label)}
							{@const filled = group.attrs.filter((a) => a.hasValue).length}
							<div>
								<div class="doc-dialog-section-title">
									{group.label} ({filled}/{group.attrs.length})
								</div>
								<div class="doc-dialog-rows">
									{#each attrRows(group.attrs) as row}
										<div class="doc-dialog-row" style="padding-left:{row.depth * 16}px">
											<span class="doc-dialog-key">{row.key}</span>
											{#if row.value !== null}
												<span class="doc-dialog-value">{row.value}</span>
											{/if}
										</div>
									{/each}
								</div>
							</div>
						{/each}
					{/if}
				</div>
			</div>
		</div>
	{/if}
</div>

{#snippet treeNode(node: TreeNode, depth: number)}
	{@const cov = coverageByNode.get(node.id)}
	<div class="tnode" style="--d:{depth}">
		<div
			class="tnode-row"
			class:sel={selectedNodeId === node.id}
			class:rejected={node.status === 'rejected'}
		>
			{#if node.children.length > 0}
				<button
					class="twist"
					onclick={() => (collapsed = { ...collapsed, [node.id]: !collapsed[node.id] })}
				>
					{#if collapsed[node.id]}<ChevronRight size={12} />{:else}<ChevronDown size={12} />{/if}
				</button>
			{:else}
				<span class="twist-spacer"></span>
			{/if}
			<button class="tnode-label" onclick={() => selectNode(node.id)}>
				<span class="badge k-{node.node_kind}">{node.node_kind}</span>
				<span class="t-name">{node.label}</span>
				{#if needsReconcileReview(node)}
					<span class="flag" title={node.reconcile_status}><AlertTriangle size={11} /></span>
				{/if}
				<span class="ground g-{node.grounding}" title={node.grounding}><CircleDot size={10} /></span
				>
				<span class="origin"
					>{node.origin === 'llm_proposed'
						? 'llm'
						: node.origin === 'graph_expanded'
							? 'graph'
							: 'user'}</span
				>
				{#if cov}<span class="t-count">{cov.artifacts}</span>{/if}
			</button>
			<div class="tnode-actions">
				{#if node.status !== 'accepted'}
					<button
						class="mini"
						title={m.pmr_accept()}
						disabled={busyNode === node.id}
						onclick={() => setStatus(node, 'accepted')}>✓</button
					>
				{/if}
				{#if node.status !== 'rejected'}
					<button
						class="mini"
						title={m.pmr_reject()}
						disabled={busyNode === node.id}
						onclick={() => setStatus(node, 'rejected')}>✕</button
					>
				{/if}
				<button class="mini" title={m.pmr_edit()} onclick={() => startRename(node)}>
					<Pencil size={11} />
				</button>
				<button
					class="mini"
					title={m.pmr_add_child()}
					onclick={() => (addingUnder = addingUnder === node.id ? null : node.id)}
					><Plus size={11} /></button
				>
				{#if node.node_kind !== 'product'}
					<button
						class="mini"
						title={m.pmr_delete()}
						disabled={busyNode === node.id}
						onclick={() => removeNode(node)}><Trash2 size={11} /></button
					>
				{/if}
			</div>
		</div>
		{#if editingNode === node.id}
			<div class="add-row" style="--d:{depth + 1}">
				<input bind:value={editLabel} onkeydown={(e) => e.key === 'Enter' && submitRename(node)} />
				<button class="mini" onclick={() => submitRename(node)}>{m.pmr_save()}</button>
				<button class="mini" onclick={() => (editingNode = null)}>{m.pmr_cancel()}</button>
			</div>
		{/if}
		{#if addingUnder === node.id}
			<div class="add-row" style="--d:{depth + 1}">
				<select bind:value={newKind}>
					<option value="part">part</option>
					<option value="module">module</option>
				</select>
				<input
					placeholder={m.pmr_new_node_label()}
					bind:value={newLabel}
					onkeydown={(e) => e.key === 'Enter' && submitNewNode()}
				/>
				<button class="mini" onclick={submitNewNode}>{m.pmr_add()}</button>
			</div>
		{/if}
		{#if !collapsed[node.id]}
			{#each node.children as child (child.id)}
				{@render treeNode(child, depth + 1)}
			{/each}
		{/if}
	</div>
{/snippet}

{#snippet resultRow(r: ResultRow)}
	<button
		class="rrow"
		class:sel={selectedResult?.artifact_id === r.artifact_id}
		onclick={() => selectResult(r)}
	>
		<span class="r-tier t-{r.tier}">{r.tier}</span>
		<span class="r-label">{r.primary_label || r.artifact_id}</span>
		<span class="r-doc">#{r.input_record_id}</span>
		<span class="r-paths">{r.paths.join('·')}</span>
		<span class="r-score mono">{fmtScore(r.score)}</span>
	</button>
{/snippet}

{#snippet reportView()}
	{@const rep = run?.report_json as
		| {
				coverage?: {
					node_id: number;
					label: string;
					kind: string;
					grounding: string;
					artifact_count: number;
					document_count: number;
				}[];
				per_document_truncated?: Record<string, number>;
		  }
		| undefined}
	<div class="report">
		<div class="metrics-strip">
			<div><strong>{run?.attributed_count ?? 0}</strong><span>{m.pmr_attributed()}</span></div>
			<div>
				<strong>{run?.document_scope_count ?? 0}</strong><span>{m.pmr_document_scope()}</span>
			</div>
			<div>
				<strong>{run?.scoped_document_count ?? 0}</strong><span>{m.pmr_docs_in_scope()}</span>
			</div>
			<div class:warn-tone={(run?.truncated_count ?? 0) > 0}>
				<strong>{run?.truncated_count ?? 0}</strong><span>{m.pmr_truncated_label()}</span>
			</div>
		</div>

		<h3>{m.pmr_coverage()}</h3>
		<div class="tbl-wrap">
			<table>
				<thead>
					<tr
						><th>{m.pmr_node()}</th><th>{m.pmr_kind()}</th><th>{m.pmr_grounding()}</th><th
							class="num">{m.pmr_metrics()}</th
						><th class="num">{m.pmr_documents()}</th></tr
					>
				</thead>
				<tbody>
					{#each rep?.coverage ?? [] as c (c.node_id)}
						<tr class:zero={c.artifact_count === 0}>
							<td>{c.label}</td>
							<td class="mono">{c.kind}</td>
							<td class="mono">{c.grounding}</td>
							<td class="num">{c.artifact_count}</td>
							<td class="num">{c.document_count}</td>
						</tr>
					{/each}
				</tbody>
			</table>
		</div>

		<h3>{m.pmr_gaps_heading({ n: gaps.length })}</h3>
		{#if gaps.length === 0}
			<p class="muted">{m.pmr_no_gaps()}</p>
		{:else}
			<ul class="gaps">
				{#each gaps as g (g.node_id)}
					<li><strong>{g.label}</strong> <span class="mono">{g.kind} · {g.grounding}</span></li>
				{/each}
			</ul>
		{/if}

		{#if rep?.per_document_truncated && Object.keys(rep.per_document_truncated).length > 0}
			<h3>{m.pmr_truncation()}</h3>
			<ul class="gaps">
				{#each Object.entries(rep.per_document_truncated) as [doc, n] (doc)}
					<li><span class="mono">#{doc}</span> — {m.pmr_truncated({ n })}</li>
				{/each}
			</ul>
		{/if}

		{#if diff && (diff.added.length || diff.removed.length || diff.retiered.length || diff.profile_version_changed)}
			<h3>{m.pmr_diff_heading({ prev: diff.previous_run_number })}</h3>
			{#if diff.profile_version_changed}
				<p class="warn-text">
					{m.pmr_diff_version({
						a: diff.previous_profile_version,
						b: diff.current_profile_version
					})}
				</p>
			{/if}
			<div class="diff-cols">
				<div>
					<h4>{m.pmr_added({ n: diff.added.length })}</h4>
					<ul>
						{#each diff.added as a (a.artifact_id)}<li class="mono">
								{a.artifact_id} <em>{a.tier}</em>
							</li>{/each}
					</ul>
				</div>
				<div>
					<h4>{m.pmr_removed({ n: diff.removed.length })}</h4>
					<ul>
						{#each diff.removed as a (a.artifact_id)}<li class="mono">
								{a.artifact_id} <em>{a.tier}</em>
							</li>{/each}
					</ul>
				</div>
				<div>
					<h4>{m.pmr_retiered({ n: diff.retiered.length })}</h4>
					<ul>
						{#each diff.retiered as a (a.artifact_id)}<li class="mono">
								{a.artifact_id} <em>{a.from}→{a.to}</em>
							</li>{/each}
					</ul>
				</div>
			</div>
		{/if}
	</div>
{/snippet}

<style>
	@import url('https://fonts.googleapis.com/css2?family=DM+Mono:wght@400;500&family=Manrope:wght@400;500;600;700;800&display=swap');

	.pmr-shell {
		--bg: oklch(0.965 0.014 78);
		--surface: oklch(0.985 0.009 78);
		--text: oklch(0.23 0.025 63);
		--subtle: oklch(0.55 0.025 72);
		--border: oklch(0.88 0.028 75);
		--bronze: oklch(0.61 0.09 69);
		--green: oklch(0.55 0.1 150);
		--blue: oklch(0.56 0.11 245);
		--violet: oklch(0.58 0.11 300);
		--red: oklch(0.57 0.14 27);
		--shadow: 0 12px 30px oklch(0.3 0.02 70 / 0.06);
		min-height: 100vh;
		background: var(--bg);
		color: var(--text);
		font-family:
			'Manrope',
			-apple-system,
			BlinkMacSystemFont,
			'Segoe UI',
			sans-serif;
		transition:
			background 180ms ease,
			color 180ms ease;
	}
	.pmr-shell.embedded {
		min-height: 0;
	}
	.pmr-shell.dark {
		--bg: #111827;
		--surface: #182334;
		--text: #f7f5f1;
		--subtle: #c4cfdf;
		--border: #304663;
		--bronze: #ff9b54;
		--green: #47b699;
		--blue: #78a5e5;
		--violet: #9c8de2;
		--red: #ff7d6b;
		--shadow: 0 12px 30px rgba(7, 10, 18, 0.32);
	}
	.pmr-shell :global(*) {
		box-sizing: border-box;
	}
	.pmr-shell button,
	.pmr-shell input,
	.pmr-shell select {
		font: inherit;
		color: inherit;
	}
	.pmr-shell button:focus-visible,
	.pmr-shell a:focus-visible,
	.pmr-shell input:focus-visible,
	.pmr-shell select:focus-visible {
		outline: 2px solid var(--bronze);
		outline-offset: 2px;
	}

	.kicker {
		margin: 0;
		text-transform: uppercase;
		letter-spacing: 0.13em;
		font:
			500 10px 'DM Mono',
			monospace;
		color: var(--bronze);
	}
	.mono {
		font-family: 'DM Mono', monospace;
	}
	.muted {
		color: var(--subtle);
		font-size: 12px;
	}
	.warn-text {
		color: var(--bronze);
	}

	.topbar {
		height: 62px;
		padding: 0 clamp(16px, 3vw, 40px);
		border-bottom: 1px solid var(--border);
		display: flex;
		justify-content: space-between;
		align-items: center;
		background: color-mix(in oklch, var(--surface) 82%, transparent);
	}
	.brand {
		display: flex;
		align-items: center;
		gap: 11px;
	}
	.brand-mark {
		width: 30px;
		height: 30px;
		border: 1px solid var(--bronze);
		color: var(--bronze);
		display: grid;
		place-items: center;
		font:
			500 11px 'DM Mono',
			monospace;
		letter-spacing: -0.06em;
	}
	.brand-name {
		margin: 2px 0 0;
		font-size: 13px;
		font-weight: 700;
	}
	.crumbs {
		display: flex;
		align-items: center;
		gap: 9px;
		color: var(--subtle);
		font-size: 12px;
	}
	.crumbs strong {
		color: var(--text);
	}
	.slash {
		color: var(--bronze);
	}
	.route-badge {
		margin-left: 6px;
		padding: 3px 8px;
		border: 1px solid var(--border);
		border-radius: 99px;
		font:
			500 10px 'DM Mono',
			monospace;
	}

	.content {
		max-width: 1720px;
		margin: 0 auto;
		padding: 26px clamp(16px, 2.4vw, 40px) 40px;
	}
	.content.wide {
		max-width: none;
	}

	.note {
		display: flex;
		align-items: center;
		gap: 9px;
		margin: 12px 0;
		padding: 10px 12px;
		border: 1px solid var(--border);
		background: color-mix(in oklch, var(--surface) 60%, var(--bronze) 5%);
		font-size: 12px;
	}
	.note.error {
		color: var(--red);
		border-color: color-mix(in oklch, var(--red) 45%, var(--border));
	}
	.note.warn {
		color: var(--bronze);
		border-color: color-mix(in oklch, var(--bronze) 45%, var(--border));
	}
	.linky {
		border: 0;
		background: transparent;
		color: var(--bronze);
		cursor: pointer;
		font-size: 11px;
		display: inline-flex;
		align-items: center;
		gap: 4px;
		padding: 0 0 0 6px;
	}
	.linky:disabled {
		opacity: 0.5;
		cursor: wait;
	}
	.linky.back {
		padding: 0;
		margin-bottom: 14px;
		font-size: 12px;
	}

	.empty-hero {
		margin: 60px auto;
		max-width: 460px;
		text-align: center;
		color: var(--subtle);
	}
	.empty-hero h1 {
		margin: 14px 0 6px;
		font-size: 22px;
		letter-spacing: -0.03em;
		color: var(--text);
	}
	.empty-hero p {
		margin: 0 0 18px;
		font-size: 13px;
		line-height: 1.6;
	}
	.run-open {
		display: flex;
		gap: 8px;
		justify-content: center;
	}
	.run-open input {
		width: 150px;
		padding: 9px 11px;
		border: 1px solid var(--border);
		background: var(--surface);
	}

	.title-row {
		display: flex;
		justify-content: space-between;
		gap: 24px;
		align-items: end;
		margin: 22px 0 18px;
	}
	.title-row h1 {
		margin: 6px 0 8px;
		font-size: clamp(24px, 3vw, 38px);
		letter-spacing: -0.05em;
		line-height: 1.04;
	}
	.lede {
		margin: 0;
		max-width: 640px;
		color: var(--subtle);
		font-size: 13px;
		line-height: 1.6;
	}
	.title-actions {
		display: flex;
		gap: 8px;
		flex-shrink: 0;
	}
	.ghost {
		display: inline-flex;
		align-items: center;
		gap: 6px;
		padding: 9px 12px;
		border: 1px solid var(--border);
		background: transparent;
		color: var(--text);
		font-size: 12px;
		text-decoration: none;
		cursor: pointer;
	}
	.ghost:hover {
		background: var(--surface);
	}
	.ghost:disabled {
		opacity: 0.5;
		cursor: wait;
	}
	.primary {
		padding: 9px 14px;
		border: 1px solid var(--bronze);
		background: var(--bronze);
		color: var(--bg);
		cursor: pointer;
		font-size: 12px;
	}

	.tabs {
		display: flex;
		gap: 2px;
		border-bottom: 1px solid var(--border);
		margin-bottom: 16px;
	}
	.tabs button {
		display: inline-flex;
		align-items: center;
		gap: 6px;
		padding: 10px 14px 11px;
		border: 0;
		border-bottom: 2px solid transparent;
		background: transparent;
		color: var(--subtle);
		cursor: pointer;
		font-size: 12px;
	}
	.tabs button:hover,
	.tabs button.active {
		color: var(--text);
		border-color: var(--bronze);
	}
	.tabs button span {
		color: var(--bronze);
		font:
			500 10px 'DM Mono',
			monospace;
	}

	.layout {
		display: grid;
		grid-template-columns: minmax(280px, 340px) minmax(320px, 640px) minmax(360px, 1fr);
		gap: 16px;
		align-items: start;
	}
	.layout.report-mode {
		grid-template-columns: minmax(260px, 300px) 1fr;
	}
	/* Results tab: independently resizable panes (drag handles between them)
	   instead of the fixed grid tracks above — spec: product-review-results-layout. */
	.layout.resizable {
		display: flex;
		align-items: flex-start;
		gap: 0;
	}
	.layout.resizable .scope-pane {
		flex: 0 0 auto;
	}
	.layout.resizable .main-pane {
		flex: 0 0 auto;
		min-width: 0;
	}
	.layout.resizable .detail-pane {
		flex: 1 1 auto;
	}
	.v-splitter {
		flex: 0 0 auto;
		align-self: stretch;
		width: 9px;
		margin: 0 -1px;
		cursor: col-resize;
		background: transparent;
		position: relative;
	}
	.v-splitter::after {
		content: '';
		position: absolute;
		top: 0;
		bottom: 0;
		left: 4px;
		width: 1px;
		background: var(--border);
	}
	.v-splitter:hover::after {
		background: var(--bronze);
	}
	.drawing-area {
		border: 1px solid var(--border);
		background: var(--surface);
		box-shadow: var(--shadow);
		margin-bottom: 0;
		overflow: hidden;
	}
	.drawing-kept,
	.drawing-pending {
		position: relative;
		height: 100%;
		display: flex;
		align-items: center;
		justify-content: center;
		background: var(--bg);
	}
	.drawing-kept img,
	.drawing-pending img {
		max-width: 100%;
		max-height: 100%;
		object-fit: contain;
	}
	.drawing-regenerate {
		position: absolute;
		top: 10px;
		right: 10px;
	}
	.drawing-pending-actions {
		position: absolute;
		bottom: 10px;
		right: 10px;
		display: flex;
		gap: 8px;
	}
	.drawing-generator {
		height: 100%;
		padding: 14px;
		display: flex;
		flex-direction: column;
		overflow: auto;
	}
	.drawing-prompt-label {
		font-size: 11px;
		color: var(--subtle);
		margin-bottom: 6px;
	}
	.drawing-generator textarea {
		flex: 1;
		min-height: 48px;
		padding: 9px 11px;
		border: 1px solid var(--border);
		background: var(--bg);
		color: inherit;
		font: inherit;
		font-size: 12px;
		resize: vertical;
	}
	.drawing-generator-actions {
		display: flex;
		align-items: center;
		gap: 4px;
		margin-top: 10px;
	}
	.drawing-model-label {
		font-size: 11px;
		color: var(--subtle);
		margin-right: 2px;
	}
	.drawing-model-select {
		border: 1px solid var(--border);
		background: var(--bg);
		color: inherit;
		font: inherit;
		font-size: 12px;
		padding: 6px 8px;
		margin-right: 8px;
	}
	.drawing-error {
		margin: 8px 0 0;
		font-size: 11px;
		color: var(--red, #c0392b);
	}
	.h-splitter {
		height: 9px;
		margin: -1px 0;
		cursor: row-resize;
		background: transparent;
		position: relative;
	}
	.h-splitter::after {
		content: '';
		position: absolute;
		left: 0;
		right: 0;
		top: 4px;
		height: 1px;
		background: var(--border);
	}
	.h-splitter:hover::after {
		background: var(--bronze);
	}
	.pane-head {
		display: flex;
		align-items: center;
		gap: 7px;
		padding-bottom: 9px;
		border-bottom: 1px solid var(--border);
		margin-bottom: 8px;
		font:
			500 10px 'DM Mono',
			monospace;
		text-transform: uppercase;
		letter-spacing: 0.08em;
		color: var(--bronze);
	}
	.pane-head span {
		margin-right: auto;
	}
	.scope-pane,
	.main-pane {
		border: 1px solid var(--border);
		background: var(--surface);
		box-shadow: var(--shadow);
		padding: 14px;
	}
	.scope-pane {
		position: sticky;
		top: 14px;
		max-height: calc(100vh - 40px);
		overflow: auto;
	}

	/* metric-details pane */
	.detail-pane {
		position: sticky;
		top: 14px;
		display: flex;
		flex-direction: column;
		gap: 16px;
		max-height: calc(100vh - 40px);
	}
	.detail-attrs {
		border: 1px solid var(--border);
		background: var(--surface);
		box-shadow: var(--shadow);
		padding: 14px;
		flex-shrink: 0;
		max-height: 46vh;
		overflow: auto;
	}
	.detail-title {
		margin: 5px 0 0;
		font-size: 15px;
		letter-spacing: -0.02em;
	}
	.detail-empty {
		padding: 18px 0;
		color: var(--subtle);
		font-size: 12px;
		text-align: center;
	}
	.detail-pdf {
		flex: 1 1 auto;
		min-height: 320px;
		border: 1px solid var(--border);
		background: var(--surface);
		box-shadow: var(--shadow);
		overflow: hidden;
		display: flex;
	}
	.detail-pdf .note {
		margin: auto;
	}
	:global(.pdf-highlight) {
		position: absolute;
		background: color-mix(in oklch, var(--bronze) 22%, transparent);
		border: 1px solid var(--bronze);
		pointer-events: none;
	}

	/* tree */
	.tnode-row {
		display: flex;
		align-items: center;
		gap: 2px;
		padding-left: calc(var(--d) * 13px);
		border-radius: 3px;
	}
	.tnode-row.sel {
		background: color-mix(in oklch, var(--bronze) 12%, transparent);
	}
	.tnode-row.rejected {
		opacity: 0.42;
		text-decoration: line-through;
	}
	.twist,
	.mini,
	.icon {
		border: 0;
		background: transparent;
		color: var(--subtle);
		cursor: pointer;
		display: inline-flex;
		align-items: center;
		justify-content: center;
	}
	.twist {
		width: 16px;
		height: 22px;
		flex-shrink: 0;
	}
	.twist-spacer {
		width: 16px;
		flex-shrink: 0;
	}
	.tnode-label {
		flex: 1;
		min-width: 0;
		display: flex;
		align-items: center;
		gap: 5px;
		border: 0;
		background: transparent;
		cursor: pointer;
		padding: 4px 2px;
		text-align: left;
	}
	.t-name {
		font-size: 12px;
		font-weight: 600;
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
	}
	.badge {
		flex-shrink: 0;
		padding: 1px 5px;
		border: 1px solid currentColor;
		border-radius: 2px;
		font:
			500 8.5px 'DM Mono',
			monospace;
		text-transform: uppercase;
	}
	.k-product {
		color: var(--bronze);
	}
	.k-module {
		color: var(--violet);
	}
	.k-part {
		color: var(--blue);
	}
	.k-aspect {
		color: var(--green);
	}
	.origin {
		flex-shrink: 0;
		color: var(--subtle);
		font:
			500 8.5px 'DM Mono',
			monospace;
	}
	.ground {
		flex-shrink: 0;
		display: inline-flex;
	}
	.g-object_node {
		color: var(--green);
	}
	.g-keyword_concept {
		color: var(--blue);
	}
	.g-ontology_term {
		color: var(--violet);
	}
	.g-ungrounded {
		color: var(--subtle);
		opacity: 0.5;
	}
	.flag {
		flex-shrink: 0;
		color: var(--red);
		display: inline-flex;
	}
	.t-count {
		flex-shrink: 0;
		margin-left: auto;
		padding: 0 5px;
		border: 1px solid var(--border);
		border-radius: 99px;
		font:
			500 9px 'DM Mono',
			monospace;
		color: var(--subtle);
	}
	.tnode-actions {
		display: flex;
		gap: 1px;
		flex-shrink: 0;
		opacity: 0;
		transition: opacity 120ms;
	}
	.tnode-row:hover .tnode-actions {
		opacity: 1;
	}
	.mini {
		width: 20px;
		height: 20px;
		font-size: 11px;
	}
	.mini:hover {
		color: var(--text);
	}
	.mini:disabled {
		opacity: 0.4;
	}
	.add-row {
		display: flex;
		gap: 5px;
		margin: 4px 0 6px;
		padding-left: calc(var(--d) * 13px + 18px);
	}
	.add-row input {
		flex: 1;
		min-width: 0;
		padding: 5px 7px;
		border: 1px solid var(--border);
		background: var(--bg);
		font-size: 11px;
	}
	.add-row select {
		padding: 5px;
		border: 1px solid var(--border);
		background: var(--bg);
		font-size: 11px;
	}
	.add-row .mini {
		width: auto;
		padding: 0 8px;
		border: 1px solid var(--border);
		font-size: 11px;
	}

	/* filters + results */
	.filters {
		display: flex;
		gap: 8px;
		margin-bottom: 12px;
		flex-wrap: wrap;
	}
	.filters select,
	.filters input {
		padding: 7px 9px;
		border: 1px solid var(--border);
		background: var(--bg);
		font-size: 11px;
	}
	.filters input {
		flex: 1;
		min-width: 120px;
	}

	.result-group {
		margin-bottom: 10px;
	}
	.group-head {
		width: 100%;
		display: flex;
		align-items: center;
		gap: 6px;
		padding: 7px 8px;
		border: 0;
		border-left: 2px solid var(--bronze);
		background: color-mix(in oklch, var(--bronze) 6%, transparent);
		cursor: pointer;
		font-size: 11px;
		text-align: left;
	}
	.group-head.sel {
		background: color-mix(in oklch, var(--bronze) 14%, transparent);
	}
	.scope-group .group-head {
		border-left-color: var(--subtle);
		background: color-mix(in oklch, var(--subtle) 8%, transparent);
	}
	.g-label {
		font-weight: 700;
	}
	.g-count {
		margin-left: auto;
		font:
			500 10px 'DM Mono',
			monospace;
		color: var(--subtle);
	}
	.rrow {
		width: 100%;
		display: grid;
		grid-template-columns: 78px 1fr 56px minmax(0, 90px) 58px;
		gap: 8px;
		align-items: center;
		padding: 8px;
		border: 0;
		border-bottom: 1px solid var(--border);
		background: transparent;
		cursor: pointer;
		text-align: left;
		font-size: 11px;
	}
	.rrow:hover {
		background: color-mix(in oklch, var(--bronze) 5%, transparent);
	}
	.rrow.sel {
		background: color-mix(in oklch, var(--bronze) 12%, transparent);
	}
	.r-tier {
		padding: 2px 5px;
		border: 1px solid currentColor;
		border-radius: 2px;
		font:
			500 8.5px 'DM Mono',
			monospace;
		text-align: center;
	}
	.t-direct {
		color: var(--bronze);
	}
	.t-part {
		color: var(--blue);
	}
	.t-aspect {
		color: var(--green);
	}
	.t-document_scope {
		color: var(--subtle);
	}
	.r-label {
		font-weight: 600;
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
	}
	.r-doc,
	.r-paths {
		color: var(--subtle);
		font:
			500 10px 'DM Mono',
			monospace;
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
	}
	.r-score {
		text-align: right;
		color: var(--subtle);
	}

	.empty-state {
		padding: 40px 0;
		text-align: center;
		color: var(--subtle);
		font-size: 12px;
	}

	/* report */
	.report h3 {
		margin: 22px 0 10px;
		font-size: 14px;
		letter-spacing: -0.02em;
	}
	.report h4 {
		margin: 0 0 6px;
		font:
			500 10px 'DM Mono',
			monospace;
		text-transform: uppercase;
		letter-spacing: 0.06em;
		color: var(--bronze);
	}
	.metrics-strip {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(120px, 1fr));
		gap: 8px;
	}
	.metrics-strip > div {
		padding: 12px 13px;
		border: 1px solid var(--border);
		border-top: 2px solid var(--bronze);
		background: var(--bg);
	}
	.metrics-strip > div.warn-tone {
		border-top-color: var(--red);
	}
	.metrics-strip strong {
		display: block;
		font:
			700 22px 'DM Mono',
			monospace;
		letter-spacing: -0.06em;
	}
	.metrics-strip span {
		font-size: 10px;
		color: var(--subtle);
	}
	.tbl-wrap {
		overflow-x: auto;
		border: 1px solid var(--border);
	}
	.report table {
		width: 100%;
		border-collapse: collapse;
		font-size: 11px;
		min-width: 460px;
	}
	.report th {
		padding: 9px 12px;
		text-align: left;
		color: var(--subtle);
		font:
			500 9px 'DM Mono',
			monospace;
		text-transform: uppercase;
		letter-spacing: 0.06em;
		border-bottom: 1px solid var(--border);
	}
	.report td {
		padding: 9px 12px;
		border-bottom: 1px solid var(--border);
	}
	.report th.num,
	.report td.num {
		text-align: right;
		font-family: 'DM Mono', monospace;
	}
	.report tr.zero td {
		color: var(--subtle);
	}
	.gaps {
		margin: 0;
		padding: 0;
		list-style: none;
	}
	.gaps li {
		padding: 7px 0;
		border-bottom: 1px solid var(--border);
		font-size: 12px;
	}
	.gaps .mono {
		color: var(--subtle);
		font-size: 10px;
		margin-left: 6px;
	}
	.diff-cols {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(180px, 1fr));
		gap: 14px;
	}
	.diff-cols ul {
		margin: 0;
		padding: 0;
		list-style: none;
	}
	.diff-cols li {
		padding: 4px 0;
		font-size: 10px;
	}
	.diff-cols em {
		color: var(--bronze);
		font-style: normal;
	}

	/* metric-details attrs (formerly the evidence drawer) */
	.drawer-fields {
		display: grid;
		grid-template-columns: 108px 1fr;
		gap: 9px 12px;
		margin: 14px 0 0;
		font-size: 12px;
	}
	.drawer-fields dt {
		color: var(--subtle);
		font:
			500 9px 'DM Mono',
			monospace;
		text-transform: uppercase;
		letter-spacing: 0.05em;
		padding-top: 2px;
	}
	.drawer-fields dd {
		margin: 0;
		word-break: break-word;
	}
	.drawer-fields .field-link {
		color: var(--bronze);
		font-family: 'DM Mono', monospace;
		border: 0;
		background: none;
		padding: 0;
		font-size: inherit;
		cursor: pointer;
	}
	.full-details-btn {
		width: 100%;
		justify-content: center;
		margin-top: 14px;
	}

	/* document record dialog */
	.doc-dialog-overlay {
		position: fixed;
		inset: 0;
		z-index: 50;
		display: flex;
		align-items: center;
		justify-content: center;
		padding: 24px;
		background: rgba(15, 23, 42, 0.62);
	}
	.doc-dialog {
		display: flex;
		flex-direction: column;
		width: min(760px, calc(100vw - 48px));
		max-height: calc(100vh - 48px);
		overflow: hidden;
		border-radius: 12px;
		background: var(--surface);
		border: 1px solid var(--border);
		box-shadow: var(--shadow);
	}
	.doc-dialog-head {
		display: flex;
		align-items: center;
		justify-content: space-between;
		padding: 12px 16px;
		border-bottom: 1px solid var(--border);
		flex-shrink: 0;
	}
	.doc-dialog-head h3 {
		margin: 0;
		font-size: 14px;
		font-weight: 600;
		color: var(--text);
	}
	.doc-dialog-body {
		padding: 16px;
		overflow-y: auto;
		display: flex;
		flex-direction: column;
		gap: 16px;
	}
	.doc-dialog-section-title {
		font-size: 12px;
		font-weight: 600;
		color: var(--subtle);
		margin-bottom: 6px;
	}
	.doc-dialog-rows {
		border: 1px solid var(--border);
		border-radius: 8px;
		padding: 6px 8px;
	}
	.doc-dialog-row {
		display: flex;
		align-items: baseline;
		gap: 8px;
		min-height: 20px;
		padding: 2px 0;
	}
	.doc-dialog-key {
		width: 160px;
		flex-shrink: 0;
		font-size: 12px;
		color: var(--subtle);
		word-break: break-all;
	}
	.doc-dialog-value {
		font-size: 12px;
		color: var(--text);
		word-break: break-word;
		flex: 1;
		min-width: 0;
	}

	@media (max-width: 1000px) {
		.layout,
		.layout.report-mode {
			grid-template-columns: 1fr;
		}
		.layout.resizable {
			flex-direction: column;
		}
		.layout.resizable .scope-pane,
		.layout.resizable .detail-pane {
			width: auto !important;
			flex: 1 1 auto;
		}
		.scope-pane,
		.detail-pane {
			position: static;
			max-height: 340px;
		}
		.detail-attrs {
			max-height: none;
		}
		.v-splitter,
		.h-splitter {
			display: none;
		}
		.drawing-area {
			height: auto !important;
			max-height: 260px;
		}
	}
	@media (prefers-reduced-motion: reduce) {
		.pmr-shell :global(*) {
			animation-duration: 0.01ms !important;
			transition-duration: 0.01ms !important;
		}
	}
</style>
