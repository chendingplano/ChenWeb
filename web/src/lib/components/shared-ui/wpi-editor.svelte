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
	import { listElementClasses, type ElementClassSummary } from '$lib/wpi/api';

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
	import { isElementNode, nodeJson, pageJson, pageNodes } from './canvas/wpi-json';
	import { duplicateNode, removeNode, setNodeActions } from './canvas/node-actions';
	import { canvasThemeVars } from './canvas/canvas-theme';
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
		onClose,
	}: {
		darkMode?: boolean;
		railOffset?: number;
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
		const zoom = Math.max(0.1, Math.min(1, (canvasWidth - 80) / previewSize.width, (canvasHeight - 80) / previewSize.height));
		viewport = { x: (canvasWidth - previewSize.width * zoom) / 2, y: (canvasHeight - previewSize.height * zoom) / 2, zoom };
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
		const empty: PageSnapshot = { nodes: [], edges: [], page: null, previewSize: { ...DEFAULT_PREVIEW_SIZE } };
		const id = nextTabId++;
		const flow: Flow = { flow_id: -1, user_id: 0, flow_name: String(page?.descriptor_id || m.canvas_01_untitled_flow()), flow_desc: '', is_default: false, is_shared: false, is_template: false, template_category: '', flow_data: { nodes: [], edges: [] }, thumbnail_svg: null, created_at: new Date().toISOString(), updated_at: new Date().toISOString() };
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
		loadedPage = (flow.flow_data?.wpi_page as JsonObject | undefined) ?? null;
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
		const id = activeTabId;
		savingTabs.add(id);
		const snapshot = currentSnapshot();
		const flow = activeFlow;
		const flowName = flow?.flow_name ?? String(loadedPage?.descriptor_id || m.canvas_01_untitled_flow());
		const flowData = { nodes: snapshot.nodes as any, edges: snapshot.edges as any, wpi_page: snapshot.page, wpi_preview_size: snapshot.previewSize };
		const thumbnail = generateThumbnail();
		try {
			const res = flow && flow.flow_id >= 0
				? await flowService.update(flow.flow_id, { flow_name: flowName, flow_data: flowData, thumbnail_svg: thumbnail })
				: await flowService.create({ flow_name: flowName, flow_data: flowData, is_shared: false, thumbnail_svg: thumbnail });
			if (id === activeTabId) {
				activeFlow = { ...res.flow, flow_name: activeFlow?.flow_name ?? flowName };
				lastSavedSnapshot = snapshot;
				savedName = flowName;
			} else {
				tabs = tabs.map((tab) => tab.id === id ? { ...tab, flow: { ...res.flow, flow_name: tab.flow?.flow_name ?? flowName }, savedName: flowName, saved: snapshot, dirty: tab.flow?.flow_name !== flowName || (!sameCanvasContent(tab.snapshot, snapshot) || JSON.stringify(tab.snapshot.page) !== JSON.stringify(snapshot.page) || JSON.stringify(tab.snapshot.previewSize) !== JSON.stringify(snapshot.previewSize)) } : tab);
			}
			toast(m.canvas_01_saved());
		} catch {
			toast(m.canvas_01_failed_to_save_your_changes());
		} finally {
			savingTabs.delete(id);
		}
	}

	// ── Thumbnail (simple SVG preview) ─────────────────────────────────────
	function generateThumbnail(): string {
		const W = 200,
			H = 120;
		const rects = nodes
			.slice(0, 8)
			.map((n) => {
				const x = Math.round((n.position.x / 2000) * W);
				const y = Math.round((n.position.y / 1200) * H);
				return `<rect x="${x}" y="${y}" width="28" height="16" rx="3" fill="#1e2535" stroke="#374151" stroke-width="0.5"/>`;
			})
			.join('');
		return `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 ${W} ${H}" width="${W}" height="${H}" style="background:#0d1117;">${rects}</svg>`;
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
		const position = {
			x: snapToGridValue((event.clientX - bounds.left - viewport.x) / zoom),
			y: snapToGridValue((event.clientY - bounds.top - viewport.y) / zoom)
		};

		const cls = nodeType.id.startsWith(ELEMENT_CLASS_PREFIX) ? nodeType.id.slice(ELEMENT_CLASS_PREFIX.length) : null;
		const type = cls ? 'wpi-element' : nodeType.id;
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
		loadedPage = null;
		configMode = null;
		previewSize = { ...DEFAULT_PREVIEW_SIZE };
		const emptySnap: PageSnapshot = { nodes: [], edges: [], page: null, previewSize: { ...previewSize } };
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
			lastSavedSnapshot = { nodes: [], edges: [], page: null, previewSize: { ...previewSize } };
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

<svelte:window onkeydown={onKeydown} />

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
				<MiniMap style="background:var(--cv-raised);" />

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

		<!-- Properties Panel -->
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
</style>
