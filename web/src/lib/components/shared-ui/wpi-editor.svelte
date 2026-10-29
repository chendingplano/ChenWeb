<!-- web/src/lib/components/shared-ui/wpi-editor.svelte (copied from canvas-01.svelte) -->
<script lang="ts">
	import { m } from '$lib/paraglide/messages.js';
	import { onMount, onDestroy } from 'svelte';
	import {
		SvelteFlow,
		ViewportPortal,
		Controls,
		MiniMap,
		Background,
		BackgroundVariant,
		Panel,
		type Node,
		type Edge
	} from '@xyflow/svelte';
	import '@xyflow/svelte/dist/style.css';

	import { flowService } from '$lib/services/flowService';
	import type { Flow, NodeType, Snapshot } from '$lib/types/flow';
	import { listElementClasses, savePage, WpiError, type ElementClassSummary } from '$lib/wpi/api';

	import SettingsIcon from '@lucide/svelte/icons/settings';

	import FlowPicker from './canvas/FlowPicker.svelte';
	import NodePalette from './canvas/NodePalette.svelte';
	import PropertiesPanel from './canvas/PropertiesPanel.svelte';
	import WpiCanvasToolbar, { type WpiNodeKind } from './canvas/WpiCanvasToolbar.svelte';
	import CreateElementDialog from './canvas/CreateElementDialog.svelte';
	import WpiJsonDialog from './canvas/WpiJsonDialog.svelte';
	import PagePreview from './canvas/PagePreview.svelte';
	import PageControl from './canvas/PageControl.svelte';
	import CenterPage from './canvas/CenterPage.svelte';
	import PageConfigPanel from './canvas/PageConfigPanel.svelte';
	import LoadPageDialog from './canvas/LoadPageDialog.svelte';
	import type { JsonObject } from '$lib/wpi/types';
	import { isElementNode, nodeData, nodeJson, pageJson, pageNodes } from './canvas/wpi-json';
	import { duplicateNode, removeNode, setNodeActions } from './canvas/node-actions';
	import { canvasThemeVars } from './canvas/canvas-theme';
	import { pageMinimapBounds } from './canvas/page-minimap';
	import { blankPage, centeredPageViewport, isInsidePage } from './canvas/page-placement';
	import { sameCanvasContent } from './canvas/canvas-dirty';
	import {
		alignNodes,
		distributeNodes,
		selectedCount,
		type AlignMode,
		type DistributeAxis
	} from './canvas/align-nodes';

	// Node type components
	import AiAssistantNode from './canvas/nodes/AiAssistantNode.svelte';
	import TextNode from './canvas/nodes/TextNode.svelte';
	import FileNode from './canvas/nodes/FileNode.svelte';
	import DocumentNode from './canvas/nodes/DocumentNode.svelte';
	import MediaNode from './canvas/nodes/MediaNode.svelte';
	import ToolNode from './canvas/nodes/ToolNode.svelte';
	import McpNode from './canvas/nodes/McpNode.svelte';
	import HttpRequestNode from './canvas/nodes/HttpRequestNode.svelte';
	import RuleNode from './canvas/nodes/RuleNode.svelte';
	import CodingAssistantNode from './canvas/nodes/CodingAssistantNode.svelte';
	import GitNode from './canvas/nodes/GitNode.svelte';
	import WpiNode from './canvas/nodes/WpiNode.svelte';

	const NODE_TYPES = {
		'wpi-page': WpiNode,
		'wpi-element': WpiNode,
		'wpi-action': WpiNode,
		'ai-assistant': AiAssistantNode,
		text: TextNode,
		file: FileNode,
		document: DocumentNode,
		media: MediaNode,
		tool: ToolNode,
		mcp: McpNode,
		'http-request': HttpRequestNode,
		rule: RuleNode,
		'coding-assistant': CodingAssistantNode,
		git: GitNode
	};

	let {
		darkMode = true,
		railOffset = 56,
		active = true,
		onClose,
	}: {
		darkMode?: boolean;
		railOffset?: number;
		active?: boolean;
		onClose: () => void;
	} = $props();

	// ── Canvas config ─────────────────────────────────────────────────────
	type EdgeStyle = 'default' | 'step' | 'smoothstep' | 'straight';
	const EDGE_STYLE_LABELS: Record<EdgeStyle, string> = {
		default: m.canvas_01_curve(),
		step: m.canvas_01_step_broken_lines(),
		smoothstep: m.canvas_01_round_broken_lines(),
		straight: m.canvas_01_straight()
	};
	let edgeStyle = $state<EdgeStyle>('default');
	let showConfig = $state(false);

	// Grid settings
	let gridResolution = $state<number>(20);
	let snapToGrid = $state<boolean>(true);

	// ── Viewport (for drop position calculation) ─────────────────────────
	let viewport = $state({ x: 0, y: 0, zoom: 1 });

	// ── Core state ────────────────────────────────────────────────────────
	let activeFlow = $state<Flow | null>(null);
	let nodes = $state<Node[]>([]);
	let edges = $state<Edge[]>([]);
	let selectedNodeId = $state<string | null>(null);
	let showPicker = $state(false);
	let nodeTypes = $state<NodeType[]>([]);
	// Palette folders for WPI building blocks; no node types belong to them yet.
	const WPI_PALETTE_CATEGORIES = ['Pages', 'Elements'];
	// Node types the tool bar creates. They are not in the server's palette list.
	const WPI_NODE_TYPES = $derived<Record<WpiNodeKind, NodeType>>({
		page: { id: 'wpi-page', label: m.wpi_editor_page(), category: 'Pages', icon: 'file', inputs: ['in'], outputs: ['out'], defaultData: {} },
		element: { id: 'wpi-element', label: m.wpi_editor_element(), category: 'Elements', icon: 'square-mouse-pointer', inputs: [], outputs: [], defaultData: {} },
		action: { id: 'wpi-action', label: m.wpi_editor_action(), category: 'Actions', icon: 'zap', inputs: ['in'], outputs: ['out'], defaultData: {} }
	});
	// Element classes, listed in the palette's ELEMENTS folder. Their palette IDs carry a
	// prefix, so they are unique; dropping one adds a wpi-element node of that class.
	const ELEMENT_CLASS_PREFIX = 'wpi-element:';
	let elementClasses = $state<ElementClassSummary[]>([]);
	const elementClassTypes = $derived<NodeType[]>(
		elementClasses.map((c) => ({
			id: ELEMENT_CLASS_PREFIX + c.descriptor_id,
			label: c.status === 'draft' ? m.wpi_editor_draft_class({ id: c.descriptor_id }) : c.descriptor_id,
			category: 'Elements',
			icon: 'square-mouse-pointer',
			inputs: [],
			outputs: [],
			defaultData: {}
		}))
	);
	let showCreateElement = $state(false);
	// The element class being edited from a node's tool bar.
	let editClassId = $state<string | null>(null);
	// View JSON shows the selected node's descriptor JSON, else the page's.
	let showJson = $state(false);
	let showLoadPage = $state(false);
	let loadedPage = $state<JsonObject | null>(null);
	let configMode = $state<'page' | 'layouter' | null>(null);
	let canvasRevision = $state(0);
	const DEFAULT_PREVIEW_SIZE = { width: 1000, height: 650 };
	let previewSize = $state({ ...DEFAULT_PREVIEW_SIZE });
	function centerPreview() {
		viewport = centeredPageViewport(previewSize, { width: canvasWidth, height: canvasHeight });
	}
	// Size of the canvas area, for placing created nodes in the middle of the view.
	let canvasWidth = $state(0);
	let canvasHeight = $state(0);
	let toastMsg = $state('');
	let toastTimeout: ReturnType<typeof setTimeout> | null = null;

	// ── Properties panel width (dragged by the separator on its left) ────
	const PROPS_MIN_WIDTH = 200;
	const PROPS_MAX_WIDTH = 640;
	let propsWidth = $state(390);
	let body: HTMLElement;
	let resizing: number | null = null;
	let detached = $state(false);
	let panelPosition = $state({ x: 0, y: 0 });
	let panelHeight = $state(500);
	let panelElement: HTMLElement;
	let dragging: { id: number; dx: number; dy: number; startX: number; startY: number } | null = null;
	let floatingResize: { id: number; x: number; y: number; width: number; height: number } | null = null;
	function togglePanel() {
		if (!detached) {
			const b = body.getBoundingClientRect();
			panelPosition = { x: Math.max(b.left, b.right - propsWidth - 16), y: b.top + 16 };
			panelHeight = Math.min(700, b.height - 32);
		}
		detached = !detached;
	}
	function movePanel(x: number, y: number) {
		const b = body.getBoundingClientRect();
		panelPosition = { x: Math.max(b.left, Math.min(x, Math.max(b.left, b.right - propsWidth))), y: Math.max(b.top, Math.min(y, b.bottom - 40)) };
	}
	function startPanelDrag(event: PointerEvent) {
		if (event.button !== 0) return;
		const rect = panelElement.getBoundingClientRect();
		dragging = { id: event.pointerId, dx: event.clientX - rect.left, dy: event.clientY - rect.top, startX: event.clientX, startY: event.clientY };
		(event.currentTarget as HTMLElement).setPointerCapture(event.pointerId);
		event.preventDefault();
	}
	function dragPanel(event: PointerEvent) {
		if (dragging?.id !== event.pointerId) return;
		if (!detached) {
			if (Math.hypot(event.clientX - dragging.startX, event.clientY - dragging.startY) < 5) return;
			panelHeight = Math.min(700, body.getBoundingClientRect().height - 32);
			detached = true;
		}
		movePanel(event.clientX - dragging.dx, event.clientY - dragging.dy);
	}
	function resizeFloating(width: number, height: number) {
		const b = body.getBoundingClientRect();
		propsWidth = Math.round(Math.max(PROPS_MIN_WIDTH, Math.min(PROPS_MAX_WIDTH, b.right - panelPosition.x, width)));
		panelHeight = Math.round(Math.max(100, Math.min(b.bottom - panelPosition.y, height)));
	}
	function startFloatingResize(event: PointerEvent) {
		if (event.button !== 0) return;
		floatingResize = { id: event.pointerId, x: event.clientX, y: event.clientY, width: propsWidth, height: panelHeight };
		(event.currentTarget as HTMLElement).setPointerCapture(event.pointerId);
		event.preventDefault();
	}
	function resizeFloatingDrag(event: PointerEvent) {
		if (floatingResize?.id === event.pointerId) resizeFloating(floatingResize.width + event.clientX - floatingResize.x, floatingResize.height + event.clientY - floatingResize.y);
	}

	const PANEL_STORAGE_KEY = 'wpi-editor-properties-panel';
	let panelRestored = $state(false);

	function restorePanel() {
		try {
			const saved = JSON.parse(localStorage.getItem(PANEL_STORAGE_KEY) ?? 'null');
			if (saved && typeof saved.detached === 'boolean' &&
				[saved.x, saved.y, saved.width, saved.height].every(Number.isFinite)) {
				propsWidth = Math.max(PROPS_MIN_WIDTH, Math.min(PROPS_MAX_WIDTH, saved.width));
				panelHeight = Math.max(100, saved.height);
				detached = saved.detached;
				if (detached) {
					movePanel(saved.x, saved.y);
					resizeFloating(propsWidth, panelHeight);
				}
			}
		} catch {
			// Storage can be unavailable or contain invalid data; use the default layout.
		}
		panelRestored = true;
	}

	$effect(() => {
		if (!panelRestored) return;
		const layout = { detached, x: panelPosition.x, y: panelPosition.y, width: propsWidth, height: panelHeight };
		try { localStorage.setItem(PANEL_STORAGE_KEY, JSON.stringify(layout)); } catch { /* Storage is optional. */ }
	});

	function keepPanelVisible() {
		if (!active || !detached) return;
		movePanel(panelPosition.x, panelPosition.y);
		resizeFloating(propsWidth, panelHeight);
	}

	// Undo / redo
	type PageSnapshot = Snapshot & { page: JsonObject | null; previewSize: { width: number; height: number } };
	let undoStack = $state<PageSnapshot[]>([]);
	let redoStack = $state<PageSnapshot[]>([]);
	let lastSavedSnapshot = $state<PageSnapshot | null>(null);
	let savedName = $state('');
	// Threshold warning flags (fire once)
	let warnedNodes = $state(false);
	let warnedEdges = $state(false);

	const MAX_UNDO = 50;

	const isDirty = $derived(
		lastSavedSnapshot !== null &&
			(activeFlow?.flow_name !== savedName || !sameCanvasContent({ nodes, edges }, lastSavedSnapshot) ||
			 JSON.stringify(loadedPage) !== JSON.stringify(lastSavedSnapshot.page) ||
			 JSON.stringify(previewSize) !== JSON.stringify(lastSavedSnapshot.previewSize))
	);
	type CanvasTab = {
		id: number; flow: Flow | null; savedName: string; snapshot: PageSnapshot; saved: PageSnapshot | null;
		undo: PageSnapshot[]; redo: PageSnapshot[]; viewport: { x: number; y: number; zoom: number };
		selected: string | null; config: 'page' | 'layouter' | null; dirty: boolean;
	};
	let tabs = $state<CanvasTab[]>([]);
	let activeTabId = $state(0);
	let nextTabId = 1;
	function currentSnapshot(): PageSnapshot {
		return JSON.parse(JSON.stringify({ nodes, edges, page: loadedPage, previewSize }));
	}
	function captureTab() {
		if (!activeTabId) return;
		tabs = tabs.map((tab) => tab.id === activeTabId ? { ...tab, flow: activeFlow, savedName, snapshot: currentSnapshot(), saved: lastSavedSnapshot, undo: undoStack, redo: redoStack, viewport: { ...viewport }, selected: selectedNodeId, config: configMode, dirty: isDirty } : tab);
	}
	function switchTab(id: number) {
		if (id === activeTabId) return;
		captureTab();
		const tab = tabs.find((item) => item.id === id);
		if (!tab) return;
		activeTabId = id;
		activeFlow = tab.flow;
		savedName = tab.savedName;
		({ nodes, edges, page: loadedPage, previewSize } = JSON.parse(JSON.stringify(tab.snapshot)));
		lastSavedSnapshot = tab.saved;
		undoStack = tab.undo; redoStack = tab.redo;
		viewport = { ...tab.viewport }; selectedNodeId = tab.selected; configMode = tab.config;
		canvasRevision++;
	}
	function addTab(page?: JsonObject) {
		captureTab();
		const empty: PageSnapshot = { nodes: [], edges: [], page: blankPage(), previewSize: { ...DEFAULT_PREVIEW_SIZE } };
		const id = nextTabId++;
		const flow: Flow = { flow_id: -1, user_id: 0, flow_name: String(page?.descriptor_id || m.wpi_unnamed()), flow_desc: '', is_default: false, is_shared: false, is_template: false, template_category: '', flow_data: { nodes: [], edges: [] }, thumbnail_svg: null, created_at: new Date().toISOString(), updated_at: new Date().toISOString() };
		const tab: CanvasTab = { id, flow, savedName: flow.flow_name, snapshot: empty, saved: empty, undo: [], redo: [], viewport: { x: 0, y: 0, zoom: 1 }, selected: null, config: null, dirty: false };
		tabs = [...tabs, tab];
		switchTab(id);
		if (page) {
			loadPage(page);
			lastSavedSnapshot = currentSnapshot();
			undoStack = [];
			redoStack = [];
		}
	}
	function closeTab(id: number) {
		const tab = tabs.find((item) => item.id === id);
		if (!tab) return;
		if ((id === activeTabId ? isDirty : tab.dirty) && !confirm(m.canvas_01_you_have_unsaved_changes_discard())) return;
		if (id === activeTabId) {
			const index = tabs.findIndex((item) => item.id === id);
			const next = tabs[index + 1] ?? tabs[index - 1];
			if (next) switchTab(next.id);
			else addTab();
		}
		tabs = tabs.filter((item) => item.id !== id);
	}
	function tabTitle(tab: CanvasTab): string {
		const page = tab.id === activeTabId ? loadedPage : tab.snapshot.page;
		return String(page?.descriptor_id || (tab.id === activeTabId ? activeFlow : tab.flow)?.flow_name || m.canvas_01_untitled_flow());
	}
	const pageOverview = $derived(pageMinimapBounds(previewSize, { width: canvasWidth, height: canvasHeight }, viewport));
	function movePageOverview(event: PointerEvent) {
		const svg = event.currentTarget as SVGSVGElement;
		const matrix = svg.getScreenCTM();
		if (!matrix) return;
		const point = new DOMPoint(event.clientX, event.clientY).matrixTransform(matrix.inverse());
		viewport = { ...viewport, x: canvasWidth / 2 - point.x * viewport.zoom, y: canvasHeight / 2 - point.y * viewport.zoom };
	}
	const canUndo = $derived(undoStack.length > 0);
	const canRedo = $derived(redoStack.length > 0);

	const selectedNode = $derived(nodes.find((n) => n.id === selectedNodeId) ?? null);
	const selectedNodeType = $derived(
		selectedNode
			? ([...nodeTypes, ...Object.values(WPI_NODE_TYPES)].find((t) => t.id === selectedNode.type) ??
					null)
			: null
	);
	const selectedTotal = $derived(selectedCount(nodes));
	// The class of the selected element node, whose instance attributes the properties
	// panel edits: null when it is not in the class list, undefined for other nodes.
	const selectedElementClass = $derived(
		selectedNode?.type === 'wpi-element'
			? (elementClasses.find((c) => c.descriptor_id === selectedNode.data.element) ?? null)
			: undefined
	);

	// ── Toast ──────────────────────────────────────────────────────────────
	function toast(msg: string) {
		toastMsg = msg;
		if (toastTimeout) clearTimeout(toastTimeout);
		toastTimeout = setTimeout(() => {
			toastMsg = '';
		}, 4000);
	}

	// ── Snapshot helpers ────────────────────────────────────────────────────
	function takeSnapshot() {
		const snap: PageSnapshot = {
			page: loadedPage ? JSON.parse(JSON.stringify(loadedPage)) : null,
			previewSize: { ...previewSize },
			nodes: JSON.parse(JSON.stringify(nodes)),
			edges: JSON.parse(JSON.stringify(edges))
		};
		undoStack = [snap, ...undoStack].slice(0, MAX_UNDO);
		redoStack = [];
	}

	function undo() {
		if (!undoStack.length) return;
		const [top, ...rest] = undoStack;
		redoStack = [
			{
				nodes: JSON.parse(JSON.stringify(nodes)),
				edges: JSON.parse(JSON.stringify(edges)),
				page: loadedPage ? JSON.parse(JSON.stringify(loadedPage)) : null,
				previewSize: { ...previewSize }
			},
			...redoStack
		];
		undoStack = rest;
		loadedPage = top.page;
		previewSize = { ...top.previewSize };
		nodes = top.nodes;
		edges = top.edges;
	}

	function redo() {
		if (!redoStack.length) return;
		const [top, ...rest] = redoStack;
		undoStack = [
			{
				nodes: JSON.parse(JSON.stringify(nodes)),
				edges: JSON.parse(JSON.stringify(edges)),
				page: loadedPage ? JSON.parse(JSON.stringify(loadedPage)) : null,
				previewSize: { ...previewSize }
			},
			...undoStack
		].slice(0, MAX_UNDO);
		redoStack = rest;
		loadedPage = top.page;
		previewSize = { ...top.previewSize };
		nodes = top.nodes;
		edges = top.edges;
	}

	// ── Load flow ───────────────────────────────────────────────────────────
	async function loadFlow(flow: Flow) {
		activeFlow = flow;
		savedName = flow.flow_name;
		configMode = null;
		loadedPage = (flow.flow_data?.wpi_page as JsonObject | undefined) ?? blankPage();
		previewSize = flow.flow_data?.wpi_preview_size ?? { ...DEFAULT_PREVIEW_SIZE };
		if (loadedPage) centerPreview();
		nodes = (flow.flow_data?.nodes ?? []) as Node[];
		edges = (flow.flow_data?.edges ?? []) as Edge[];
		lastSavedSnapshot = {
			page: loadedPage ? JSON.parse(JSON.stringify(loadedPage)) : null,
			previewSize: { ...previewSize },
			nodes: JSON.parse(JSON.stringify(nodes)),
			edges: JSON.parse(JSON.stringify(edges))
		};
		undoStack = [];
		redoStack = [];
		warnedNodes = false;
		warnedEdges = false;
		showPicker = false;
	}

	async function loadDefaultOrPicker() {
		try {
			const res = await flowService.getDefault();
			await loadFlow(res.flow);
		} catch {
			showPicker = true;
		}
	}

	function loadPage(page: JsonObject) {
		const next = pageNodes(page);
		takeSnapshot();
		nodes = next.map((node) => ({ ...node, hidden: true }));
		edges = [];
		loadedPage = page;
		configMode = null;
		previewSize = { ...DEFAULT_PREVIEW_SIZE };
		centerPreview();
		selectedNodeId = null;
		canvasRevision++;
		showLoadPage = false;
		checkLimits();
	}

	// ── Save ────────────────────────────────────────────────────────────────
	let savingTabs = new Set<number>();
	async function save() {
		if (savingTabs.has(activeTabId)) return;
		if (!loadedPage || typeof loadedPage.descriptor_id !== 'string' || !loadedPage.descriptor_id) {
			toast(m.wpi_save_requires_page());
			return;
		}
		if (edges.length || nodes.some((node) => !isElementNode(node))) {
			toast(m.wpi_save_page_elements_only());
			return;
		}
		const id = activeTabId;
		savingTabs.add(id);
		const snapshot = currentSnapshot();
		const name = activeFlow?.flow_name ?? '';
		const page = { ...snapshot.page, elements: pageJson(snapshot.nodes).elements } as JsonObject;
		try {
			await savePage(String(page.descriptor_id), page);
			if (id === activeTabId) {
				lastSavedSnapshot = snapshot;
				savedName = name;
			} else {
				tabs = tabs.map((tab) => tab.id === id ? { ...tab, savedName: name, saved: snapshot, dirty: tab.flow?.flow_name !== name || !sameCanvasContent(tab.snapshot, snapshot) || JSON.stringify(tab.snapshot.page) !== JSON.stringify(snapshot.page) || JSON.stringify(tab.snapshot.previewSize) !== JSON.stringify(snapshot.previewSize) } : tab);
			}
			toast(m.canvas_01_saved());
		} catch (error) {
			const detail = error instanceof WpiError ? (error.issues.map((issue) => `${issue.path}: ${issue.message}`).join('; ') || error.message) : '';
			toast(detail ? m.wpi_save_failed_detail({ detail }) : m.canvas_01_failed_to_save_your_changes());
		} finally {
			savingTabs.delete(id);
		}
	}

	// ── Drag-and-drop from palette ──────────────────────────────────────────
	function onPaletteDragStart(event: DragEvent, nodeType: NodeType) {
		event.dataTransfer?.setData('application/xyflow-node-type', JSON.stringify(nodeType));
	}

	function snapToGridValue(value: number): number {
		if (!snapToGrid) return value;
		return Math.round(value / gridResolution) * gridResolution;
	}

	function onCanvasDrop(event: DragEvent) {
		event.preventDefault();
		const raw = event.dataTransfer?.getData('application/xyflow-node-type');
		if (!raw) return;
		const nodeType: NodeType = JSON.parse(raw);
		const bounds = (event.currentTarget as HTMLElement).getBoundingClientRect();

		// The node's top-left corner goes where it was dropped: screen → flow coordinates.
		const zoom = viewport.zoom || 1;
		const point = { x: (event.clientX - bounds.left - viewport.x) / zoom, y: (event.clientY - bounds.top - viewport.y) / zoom };
		const position = { x: snapToGridValue(point.x), y: snapToGridValue(point.y) };

		const cls = nodeType.id.startsWith(ELEMENT_CLASS_PREFIX) ? nodeType.id.slice(ELEMENT_CLASS_PREFIX.length) : null;
		const type = cls ? 'wpi-element' : nodeType.id;
		if (type === 'wpi-element' && (!loadedPage || !isInsidePage(point, previewSize))) {
			toast(m.wpi_drop_element_inside_page());
			return;
		}
		const id = `${type}-${Date.now()}`;
		takeSnapshot();
		// The new node becomes the selection, so its resize grips show without a click.
		nodes = [
			...nodes.map((n) => ({ ...n, selected: false })),
			{
				id,
				type,
				hidden: !!loadedPage && type === 'wpi-element',
				position,
				selected: true,
				data: cls ? { label: cls, element: cls } : { ...nodeType.defaultData, label: nodeType.label }
			}
		];
		selectedNodeId = id;
		checkLimits();
	}

	function checkLimits() {
		if (!warnedNodes && nodes.length > 500) {
			toast(m.canvas_01_warning_over_500_nodes_performance());
			warnedNodes = true;
		}
		if (!warnedEdges && edges.length > 1000) {
			toast(m.canvas_01_warning_over_1000_edges_performance());
			warnedEdges = true;
		}
	}

	// ── Tool bar ────────────────────────────────────────────────────────────
	// Create a node in the middle of the visible canvas (BaseNode is 160×60 at minimum),
	// stepping one grid cell down-right while another node already sits there.
	function createWpiNode(kind: WpiNodeKind) {
		const nodeType = WPI_NODE_TYPES[kind];
		const zoom = viewport.zoom || 1;
		const position = {
			x: snapToGridValue((canvasWidth / 2 - viewport.x) / zoom - 80),
			y: snapToGridValue((canvasHeight / 2 - viewport.y) / zoom - 30)
		};
		while (nodes.some((n) => n.position.x === position.x && n.position.y === position.y)) {
			position.x += gridResolution;
			position.y += gridResolution;
		}
		const id = `${nodeType.id}-${Date.now()}`;
		takeSnapshot();
		nodes = [
			...nodes.map((n) => ({ ...n, selected: false })),
			{ id, type: nodeType.id, hidden: !!loadedPage && nodeType.id === 'wpi-element', position, selected: true, data: { ...nodeType.defaultData, label: nodeType.label } }
		];
		selectedNodeId = id;
		checkLimits();
	}

	// ── Node tool bar ───────────────────────────────────────────────────────
	setNodeActions({
		duplicate(id) {
			const src = nodes.find((n) => n.id === id);
			if (!src) return;
			const newId = `${src.type}-${Date.now()}`;
			takeSnapshot();
			nodes = duplicateNode(nodes, id, newId, gridResolution);
			selectedNodeId = newId;
			checkLimits();
		},
		remove(id) {
			takeSnapshot();
			const next = removeNode(nodes, edges, id);
			nodes = next.nodes;
			edges = next.edges;
			if (selectedNodeId === id) selectedNodeId = null;
		},
		editClass(id) {
			const cls = nodes.find((n) => n.id === id)?.data.element;
			if (typeof cls === 'string' && cls !== '') editClassId = cls;
		}
	});

	async function loadElementClasses() {
		try {
			elementClasses = await listElementClasses();
		} catch {
			toast(m.wpi_editor_element_classes_failed());
		}
	}

	function alignSelected(mode: AlignMode) {
		if (selectedTotal < 2) return;
		takeSnapshot();
		nodes = alignNodes(nodes, mode);
	}

	function distributeSelected(axis: DistributeAxis) {
		if (selectedTotal < 3) return;
		takeSnapshot();
		nodes = distributeNodes(nodes, axis);
	}

	// ── Properties panel resize ─────────────────────────────────────────────
	function startResize(event: PointerEvent) {
		if (event.button !== 0) return;
		resizing = event.pointerId;
		(event.currentTarget as HTMLElement).setPointerCapture(event.pointerId);
		event.preventDefault();
	}

	function resize(event: PointerEvent) {
		if (resizing !== event.pointerId) return;
		const next = body.getBoundingClientRect().right - event.clientX;
		propsWidth = Math.round(Math.min(PROPS_MAX_WIDTH, Math.max(PROPS_MIN_WIDTH, next)));
	}

	function endResize(event: PointerEvent) {
		if (resizing === event.pointerId) resizing = null;
	}

	// ── Close with guard ────────────────────────────────────────────────────
	function requestClose() {
		if (isDirty || tabs.some((tab) => tab.id !== activeTabId && tab.dirty)) {
			if (!confirm(m.canvas_01_you_have_unsaved_changes_discard())) return;
		}
		onClose();
	}

	// ── Node update from properties panel ───────────────────────────────────
	function updateNodeData(nodeId: string, data: Record<string, unknown>) {
		nodes = nodes.map((n) => (n.id === nodeId ? { ...n, data } : n));
	}

	// ── New empty flow ───────────────────────────────────────────────────────
	async function createNewEmpty() {
		loadedPage = blankPage();
		configMode = null;
		previewSize = { ...DEFAULT_PREVIEW_SIZE };
		centerPreview();
		canvasRevision++;
		const emptySnap: PageSnapshot = { nodes: [], edges: [], page: loadedPage, previewSize: { ...previewSize } };
		lastSavedSnapshot = emptySnap;
		nodes = [];
		edges = [];
		undoStack = [];
		redoStack = [];
		try {
			const res = await flowService.create({
				flow_name: m.canvas_01_untitled_flow(),
				flow_data: { nodes: [], edges: [] },
				is_shared: false,
				thumbnail_svg: null
			});
			activeFlow = res.flow;
		savedName = res.flow.flow_name;
			lastSavedSnapshot = { nodes: [], edges: [], page: loadedPage, previewSize: { ...previewSize } };
		} catch {
			toast(m.canvas_01_could_not_create_flow_on());
			activeFlow = {
				flow_id: -1,
				user_id: 0,
				flow_name: m.canvas_01_untitled_flow(),
				flow_desc: '',
				is_default: false,
				is_shared: false,
				is_template: false,
				template_category: '',
				flow_data: { nodes: [], edges: [] },
				thumbnail_svg: null,
				created_at: new Date().toISOString(),
				updated_at: new Date().toISOString()
			};
		}
		showPicker = false;
	}

	// ── Edge style ──────────────────────────────────────────────────────────
	function applyEdgeStyle(style: EdgeStyle) {
		edgeStyle = style;
		takeSnapshot();
		edges = edges.map((e) => ({ ...e, type: style }));
		showConfig = false;
	}

	// ── Save as template ────────────────────────────────────────────────────
	async function saveCurrentAsTemplate() {
		if (!activeFlow || activeFlow.flow_id < 0) {
			toast(m.canvas_01_save_the_flow_first());
			return;
		}
		try {
			await flowService.saveAsTemplate(activeFlow.flow_id);
			toast(m.canvas_01_saved_as_template());
		} catch {
			toast(m.canvas_01_failed_to_save_as_template());
		}
	}

	// ── Keyboard shortcuts ───────────────────────────────────────────────────
	function onKeydown(e: KeyboardEvent) {
		if (!active) return;
		// The element class forms have their own fields, and View JSON covers the canvas; undo
		// and save belong to the canvas.
		if (showCreateElement || editClassId !== null || showJson || showLoadPage) return;
		const mod = e.ctrlKey || e.metaKey;
		if (mod && e.key === 'z' && !e.shiftKey) {
			e.preventDefault();
			undo();
		}
		if (mod && (e.key === 'y' || (e.key === 'z' && e.shiftKey))) {
			e.preventDefault();
			redo();
		}
		if (mod && e.key === 's') {
			e.preventDefault();
			save();
		}
	}

	// ── Lifecycle ────────────────────────────────────────────────────────────
	onMount(async () => {
		restorePanel();
		addTab();
		loadElementClasses();
		const res = await flowService.getNodeTypes();
		nodeTypes = res.nodeTypes ?? [];
		await loadDefaultOrPicker();
	});

	onDestroy(() => {
		if (toastTimeout) clearTimeout(toastTimeout);
	});
</script>

<svelte:window onkeydown={onKeydown} onresize={keepPanelVisible} />

<!-- Starts below the sticky site header (routes/semos/components/SiteHeader.svelte: h-[4.25rem] + 1px border, z-50), which would otherwise cover the top of the editor. -->
<div class="fixed bottom-0 right-0 flex flex-col select-text" style="{canvasThemeVars(darkMode)} background:var(--cv-canvas); z-index:40; left:{railOffset}px; top:calc(4.25rem + 1px);">
	<!-- 3-panel body -->
	<div bind:this={body} class="flex flex-1 overflow-hidden">
		<!-- Node Palette -->
		<NodePalette
			nodeTypes={[...nodeTypes, ...elementClassTypes]}
			extraCategories={WPI_PALETTE_CATEGORIES}
			{darkMode}
			onDragStart={onPaletteDragStart}
		/>

		<!-- Canvas column: tool bar + canvas -->
		<div class="flex min-w-0 flex-1 flex-col overflow-hidden">
		<WpiCanvasToolbar
			{activeFlow}
			{isDirty}
			{canUndo}
			{canRedo}
			selectedCount={selectedTotal}
			onPickerOpen={() => {
				if (isDirty) {
					if (!confirm(m.canvas_01_you_have_unsaved_changes_discard())) return;
				}
				showPicker = true;
			}}
			onRename={(name) => {
				if (activeFlow) {
					activeFlow = { ...activeFlow, flow_name: name };
				}
			}}
			onCreate={(kind) => (kind === 'element' ? (showCreateElement = true) : createWpiNode(kind))}
			onLoadPage={() => (showLoadPage = true)}
			onViewJson={() => (showJson = true)}
			onUndo={undo}
			onRedo={redo}
			onAlign={alignSelected}
			onDistribute={distributeSelected}
			onSave={save}
			onSaveAsTemplate={saveCurrentAsTemplate}
		/>
		<div class="canvas-tabs" role="tablist" aria-label={m.wpi_canvas_tabs()}>
			{#each tabs as tab (tab.id)}
				<div class="canvas-tab" class:active={tab.id === activeTabId}>
					<button type="button" role="tab" aria-selected={tab.id === activeTabId} onclick={() => switchTab(tab.id)}>
						{tabTitle(tab)}{#if tab.id === activeTabId ? isDirty : tab.dirty}<span aria-label={m.wpi_tab_unsaved()}> *</span>{/if}
					</button>
					<button type="button" class="close-tab" aria-label={m.wpi_close_canvas_tab({ name: tabTitle(tab) })} title={m.wpi_close_canvas_tab({ name: tabTitle(tab) })} onclick={() => closeTab(tab.id)}>×</button>
				</div>
			{/each}
			<button type="button" class="add-tab" aria-label={m.wpi_add_canvas_tab()} title={m.wpi_add_canvas_tab()} onclick={() => addTab()}>+</button>
		</div>
		<div
			class="min-h-0 flex-1 overflow-hidden"
			bind:clientWidth={canvasWidth}
			bind:clientHeight={canvasHeight}
			ondragover={(e) => e.preventDefault()}
			ondrop={onCanvasDrop}
			role="application"
			aria-label={m.canvas_01_flow_canvas()}
		>
			<!-- Svelte Flow reads fitView once, when it mounts, and on an empty canvas it waits and
			     fits the first node added — moving it from where it was dropped to the middle. So it
			     mounts again for each flow and fits only a flow that already has nodes. -->
			{#key `${activeFlow?.flow_id}-${canvasRevision}`}
			<SvelteFlow
				bind:nodes
				bind:edges
				bind:viewport
				initialViewport={viewport}
				nodeTypes={NODE_TYPES}
				colorMode={darkMode ? 'dark' : 'light'}
				fitView={nodes.length > 0 && !loadedPage}
				fitViewOptions={{ maxZoom: 1 }}
				snapGrid={snapToGrid ? [gridResolution, gridResolution] : undefined}
				onselectionchange={(params: { nodes: Node[] }) => {
					if (!loadedPage || params.nodes.length > 0) {
						selectedNodeId = params.nodes.length === 1 ? params.nodes[0].id : null;
						if (params.nodes.length > 0) configMode = null;
					}
				}}
				onnodedragstop={(e) => {
					takeSnapshot();
					if (snapToGrid) {
						nodes = nodes.map((n) => {
							if (n.id === e.targetNode?.id) {
								return {
									...n,
									position: {
										x: snapToGridValue(n.position.x),
										y: snapToGridValue(n.position.y)
									}
								};
							}
							return n;
						});
					}
				}}
				onconnect={(connection) => {
					const sh = connection.sourceHandle ?? 'out';
					const th = connection.targetHandle ?? 'in';
					// Prevent duplicate edges for the same source/target/handle combination
					const alreadyExists = edges.some(
						(e) =>
							e.source === connection.source &&
							e.sourceHandle === sh &&
							e.target === connection.target &&
							e.targetHandle === th
					);
					if (alreadyExists) return;
					takeSnapshot();
					edges = [
						...edges,
						{
							id: `e-${connection.source}-${sh}-${connection.target}-${th}-${Date.now()}`,
							source: connection.source,
							sourceHandle: sh,
							target: connection.target,
							targetHandle: th,
							type: edgeStyle
						}
					];
					checkLimits();
				}}
				ondelete={() => takeSnapshot()}
				style="background:var(--cv-canvas);"
			>
				{#if loadedPage}
					<CenterPage width={canvasWidth} height={canvasHeight} size={previewSize} />
					<ViewportPortal target="front">
						{#key `${loadedPage.descriptor_id}-${canvasRevision}`}
							<PagePreview descriptor={loadedPage} {nodes} selectedId={selectedNodeId} size={previewSize} zoom={viewport.zoom} onResizeStart={takeSnapshot} onResize={(size) => (previewSize = size)} onSelect={(id) => { configMode = null; selectedNodeId = id; }} />
						{/key}
					</ViewportPortal>
				{/if}
				<Background
					variant={BackgroundVariant.Dots}
					gap={gridResolution}
					size={1}
					bgColor="var(--cv-canvas)"
					patternColor="var(--cv-grid)"
				/>
				<Controls />
				{#if loadedPage}
					<Panel position="bottom-right" class="svelte-flow__minimap" style="background:var(--cv-raised);">
						<svg width="200" height="150" viewBox={`${pageOverview.bounds.x - 40} ${pageOverview.bounds.y - 40} ${pageOverview.bounds.width + 80} ${pageOverview.bounds.height + 80}`} role="application" tabindex="0" aria-label={m.mesh_minimap()}
							onpointerdown={(event) => { event.currentTarget.setPointerCapture(event.pointerId); movePageOverview(event); }}
							onpointermove={(event) => { if (event.buttons === 1) movePageOverview(event); }}
							onkeydown={(event) => { const step = 50; if (event.key === 'ArrowLeft') viewport = { ...viewport, x: viewport.x + step }; else if (event.key === 'ArrowRight') viewport = { ...viewport, x: viewport.x - step }; else if (event.key === 'ArrowUp') viewport = { ...viewport, y: viewport.y + step }; else if (event.key === 'ArrowDown') viewport = { ...viewport, y: viewport.y - step }; else return; event.preventDefault(); }}>
							<rect x="0" y="0" width={previewSize.width} height={previewSize.height} fill="var(--cv-canvas)" stroke="var(--cv-border)" stroke-width="2" vector-effect="non-scaling-stroke" />
							<rect x={pageOverview.viewport.x} y={pageOverview.viewport.y} width={pageOverview.viewport.width} height={pageOverview.viewport.height} fill="none" stroke="#818cf8" stroke-width="2" vector-effect="non-scaling-stroke" />
						</svg>
						<button type="button" class="nodrag nopan" style="display:block; width:100%; padding:4px; color:var(--cv-text); background:var(--cv-surface); border:0; cursor:pointer;" onclick={centerPreview}>{m.wpi_restore_viewport()}</button>
					</Panel>
				{:else}
					<MiniMap style="background:var(--cv-raised);" />
				{/if}

				<!-- Canvas config panel -->
				<Panel
					position="top-right"
					style="display:flex; flex-direction:column; align-items:flex-end; gap:4px; margin:6px;"
				>
					<div style="display:flex; align-items:center; gap:4px;">
						{#if loadedPage}<PageControl mode={configMode} onChoose={(mode) => { configMode = mode; selectedNodeId = null; }} />{/if}
					<button
						onclick={() => {
							showConfig = !showConfig;
						}}
						title={m.canvas_01_canvas_settings()}
						style="background:{showConfig
							? 'var(--cv-accent)'
							: 'var(--cv-surface)'}; border:1px solid var(--cv-border); border-radius:6px; width:28px; height:28px; cursor:pointer; display:flex; align-items:center; justify-content:center; color:{showConfig
							? '#fff'
							: 'var(--cv-text-2)'};"
					>
						<SettingsIcon size={14} />
					</button>
					</div>
					{#if showConfig}
						<div
							style="background:var(--cv-surface); border:1px solid var(--cv-border); border-radius:8px; padding:10px 12px; min-width:170px; box-shadow:0 4px 16px var(--cv-shadow);"
						>
							<p
								style="font-size:10px; font-weight:600; color:var(--cv-text-muted); text-transform:uppercase; letter-spacing:.05em; margin:0 0 8px;"
							>
								{m.canvas_01_edge_style()}
							</p>
							{#each Object.entries(EDGE_STYLE_LABELS) as [k, label]}
								<button
									onclick={() => applyEdgeStyle(k as EdgeStyle)}
									style="display:block; width:100%; text-align:left; padding:5px 8px; border-radius:5px; border:none; cursor:pointer; font-size:12px; margin-bottom:3px;
                    background:{edgeStyle === k ? 'var(--cv-accent)' : 'transparent'};
                    color:{edgeStyle === k ? '#fff' : 'var(--cv-text-2)'};"
								>
									{label}
								</button>
							{/each}

							<hr style="border:none; border-top:1px solid var(--cv-border); margin:10px 0;" />

							<p
								style="font-size:10px; font-weight:600; color:var(--cv-text-muted); text-transform:uppercase; letter-spacing:.05em; margin:0 0 8px;"
							>
								{m.canvas_01_grid_settings()}
							</p>

							<div style="margin-bottom:8px;">
								<label
									style="display:flex; justify-content:space-between; align-items:center; font-size:12px; color:var(--cv-text-2); margin-bottom:4px;"
								>
									<span>{m.canvas_01_resolution()}</span>
									<input
										type="number"
										min="1"
										max="50"
										bind:value={gridResolution}
										style="width:50px; background:var(--cv-field); border:1px solid var(--cv-border); border-radius:4px; padding:2px 6px; color:var(--cv-text); font-size:11px; text-align:right;"
									/>
								</label>
							</div>

							<div>
								<label
									style="display:flex; justify-content:space-between; align-items:center; font-size:12px; color:var(--cv-text-2);"
								>
									<span>{m.canvas_01_snap_to_grid()}</span>
									<input
										type="checkbox"
										bind:checked={snapToGrid}
										style="width:16px; height:16px; accent-color:#6366f1;"
									/>
								</label>
							</div>
						</div>
					{/if}
				</Panel>
			</SvelteFlow>
			{/key}
		</div>
		</div>

		<!-- Drag to resize the properties panel; it straddles the panel's left border. -->
		{#if !detached}
		<div
			role="separator"
			aria-orientation="vertical"
			aria-label={m.wpi_editor_resize_properties()}
			class="relative z-10 -mx-[3px] w-1.5 flex-shrink-0 cursor-col-resize select-none hover:bg-(--cv-accent)/40"
			onpointerdown={startResize}
			onpointermove={resize}
			onpointerup={endResize}
			onpointercancel={endResize}
		></div>
		{/if}

		<!-- Properties Panel -->
		<div bind:this={panelElement} class="properties-container" class:detached style:width="{propsWidth}px" style:height={detached ? `${panelHeight}px` : undefined} style:left={detached ? `${panelPosition.x}px` : undefined} style:top={detached ? `${panelPosition.y}px` : undefined}>
			<div class="properties-controls">
				<button type="button" class="panel-handle" aria-label={m.wpi_editor_move_panel()} title={m.wpi_editor_move_panel()}
					onpointerdown={startPanelDrag} onpointermove={dragPanel} onpointerup={() => dragging = null} onpointercancel={() => dragging = null}
					onkeydown={(event) => { if (!detached) return; const d = event.shiftKey ? 20 : 5; if (event.key === 'ArrowLeft') movePanel(panelPosition.x - d, panelPosition.y); else if (event.key === 'ArrowRight') movePanel(panelPosition.x + d, panelPosition.y); else if (event.key === 'ArrowUp') movePanel(panelPosition.x, panelPosition.y - d); else if (event.key === 'ArrowDown') movePanel(panelPosition.x, panelPosition.y + d); else return; event.preventDefault(); }}>⋮⋮</button>
				<button type="button" class="panel-toggle" onclick={togglePanel} aria-label={detached ? m.wpi_editor_dock_panel() : m.wpi_editor_detach_panel()} title={detached ? m.wpi_editor_dock_panel() : m.wpi_editor_detach_panel()}>{detached ? '↪' : '↗'}</button>
			</div>
			<div class="properties-content">
		{#if loadedPage && configMode}
			<PageConfigPanel page={loadedPage} mode={configMode} width={propsWidth} onUpdate={(page) => { takeSnapshot(); loadedPage = page; }} />
		{:else}
		<PropertiesPanel
			node={selectedNode as any}
			nodeType={selectedNodeType}
			elementClass={selectedElementClass}
			nodeTab
			width={propsWidth}
			{darkMode}
			onUpdate={updateNodeData}
		/>
		{/if}
			</div>
			{#if detached}
				<button type="button" class="floating-resize" aria-label={m.wpi_editor_resize_floating()} title={m.wpi_editor_resize_floating()}
					onpointerdown={startFloatingResize} onpointermove={resizeFloatingDrag} onpointerup={() => floatingResize = null} onpointercancel={() => floatingResize = null}
					onkeydown={(event) => { const d = event.shiftKey ? 20 : 5; if (event.key === 'ArrowLeft') resizeFloating(propsWidth - d, panelHeight); else if (event.key === 'ArrowRight') resizeFloating(propsWidth + d, panelHeight); else if (event.key === 'ArrowUp') resizeFloating(propsWidth, panelHeight - d); else if (event.key === 'ArrowDown') resizeFloating(propsWidth, panelHeight + d); else return; event.preventDefault(); }}>◢</button>
			{/if}
		</div>
	</div>

	<!-- Toast -->
	{#if toastMsg}
		<div
			class="fixed bottom-6 left-1/2 -translate-x-1/2 rounded-lg px-4 py-2 text-sm shadow-xl"
			style="background:var(--cv-surface); border:1px solid var(--cv-border); color:var(--cv-text); z-index:60; pointer-events:none;"
		>
			{toastMsg}
		</div>
	{/if}
</div>

{#if showCreateElement}
	<CreateElementDialog
		{darkMode}
		onSaved={(id) => {
			showCreateElement = false;
			toast(m.wpi_editor_element_class_created({ id }));
			loadElementClasses();
		}}
		onClose={() => (showCreateElement = false)}
	/>
{/if}

{#if editClassId !== null}
	<CreateElementDialog
		{darkMode}
		classId={editClassId}
		onSaved={(id) => {
			editClassId = null;
			toast(m.wpi_editor_element_class_saved({ id }));
			loadElementClasses();
		}}
		onClose={() => (editClassId = null)}
	/>
{/if}

{#if showJson}
	<WpiJsonDialog
		{darkMode}
		title={selectedNode
			? m.wpi_view_json_node_title({ name: String(selectedNode.data.label ?? '') })
			: m.wpi_view_json_page_title()}
		note={!selectedNode
			? (loadedPage ? String(loadedPage.descriptor_id ?? '') : m.wpi_view_json_page_note())
			: isElementNode(selectedNode)
				? ''
				: m.wpi_view_json_node_data_note()}
		json={selectedNode ? nodeJson(selectedNode) : { ...pageJson(nodes), ...loadedPage, elements: pageJson(nodes).elements }}
		onApply={(value) => {
			if (selectedNode) {
				if (isElementNode(selectedNode) && typeof value.element !== 'string') return false;
				takeSnapshot();
				updateNodeData(selectedNode.id, nodeData(selectedNode, value));
			} else {
				let next: Node[];
				try { next = pageNodes(value); } catch { return false; }
				const existing = nodes.filter(isElementNode).sort((a, b) => a.position.y - b.position.y || a.position.x - b.position.x);
				takeSnapshot();
				const used = new Set(nodes.map((node) => node.id));
				const elements = next.map((node, index) => {
					const old = existing[index];
					if (old) return { ...old, hidden: true, data: nodeData(old, nodeJson(node)) };
					let id = node.id;
					while (used.has(id)) id += '-new';
					used.add(id);
					return { ...node, id, hidden: true };
				});
				const removed = new Set(existing.slice(elements.length).map((node) => node.id));
				nodes = [...nodes.filter((node) => !isElementNode(node)), ...elements];
				edges = edges.filter((edge) => !removed.has(edge.source) && !removed.has(edge.target));
				loadedPage = value;
				canvasRevision++;
			}
			return true;
		}}
		onClose={() => (showJson = false)}
	/>
{/if}

<!-- Flow Picker Modal -->
{#if showPicker}
	<FlowPicker
		{darkMode}
		currentUserId={activeFlow?.user_id ?? 0}
		onOpen={loadFlow}
		onNewEmpty={createNewEmpty}
		onClose={() => {
			if (!activeFlow) requestClose();
			else showPicker = false;
		}}
	/>
{/if}

{#if showLoadPage}
	<LoadPageDialog
		{darkMode}
		onClose={() => (showLoadPage = false)}
		onLoad={(page) => addTab(page)}
	/>
{/if}

<style>
	.canvas-tabs { display: flex; align-items: stretch; flex-shrink: 0; overflow-x: auto; background: var(--cv-raised); border-bottom: 1px solid var(--cv-divider); }
	.canvas-tab { display: flex; align-items: center; flex-shrink: 0; border-right: 1px solid var(--cv-divider); border-bottom: 2px solid transparent; }
	.canvas-tabs button { flex-shrink: 0; max-width: 220px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; padding: 7px 14px; border: 0; background: transparent; color: var(--cv-text-muted); font-size: 11px; cursor: pointer; }
	.canvas-tab.active { color: var(--cv-text); background: var(--cv-canvas); border-bottom-color: var(--cv-accent); }
	.canvas-tabs button:hover { background: var(--cv-hover); }
	.canvas-tabs .close-tab { opacity: 0; padding: 2px 6px; margin-right: 4px; font-size: 14px; }
	.canvas-tab:hover .close-tab, .canvas-tab:focus-within .close-tab { opacity: 1; }
	.canvas-tabs .add-tab { font-size: 16px; padding: 3px 12px; }

	.properties-container { flex-shrink: 0; display: flex; flex-direction: column; min-height: 0; background: var(--cv-panel); }
	.properties-container.detached { position: fixed; z-index: 45; border: 1px solid var(--cv-border); box-shadow: 0 8px 24px var(--cv-shadow); }
	.properties-controls { display: flex; align-items: center; justify-content: space-between; height: 30px; flex-shrink: 0; border-bottom: 1px solid var(--cv-divider); }
	.panel-handle, .panel-toggle { border: 0; background: transparent; color: var(--cv-text-2); height: 100%; padding: 0 10px; }
	.panel-handle { flex: 1; text-align: left; cursor: grab; touch-action: none; }
	.panel-toggle { cursor: pointer; }
	.panel-toggle:hover, .panel-handle:hover { color: var(--cv-text); background: var(--cv-raised); }
	.properties-content { flex: 1; min-height: 0; overflow: hidden; }
	.floating-resize { position: absolute; right: 0; bottom: 0; width: 20px; height: 20px; border: 0; background: var(--cv-panel); color: var(--cv-text-2); cursor: nwse-resize; touch-action: none; padding: 0; }
</style>
