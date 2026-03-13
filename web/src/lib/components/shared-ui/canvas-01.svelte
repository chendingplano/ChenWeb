<!-- web/src/lib/components/shared-ui/canvas-01.svelte -->
<script lang="ts">
  import { onMount, onDestroy } from 'svelte';
  import {
    SvelteFlow, Controls, MiniMap, Background, BackgroundVariant,
    type Node, type Edge,
  } from '@xyflow/svelte';
  import '@xyflow/svelte/dist/style.css';

  import { flowService }     from '$lib/services/flowService';
  import type { Flow, NodeType, Snapshot } from '$lib/types/flow';

  import FlowToolbar      from './canvas/FlowToolbar.svelte';
  import FlowPicker       from './canvas/FlowPicker.svelte';
  import NodePalette      from './canvas/NodePalette.svelte';
  import PropertiesPanel  from './canvas/PropertiesPanel.svelte';

  // Node type components
  import AiAssistantNode    from './canvas/nodes/AiAssistantNode.svelte';
  import TextNode           from './canvas/nodes/TextNode.svelte';
  import FileNode           from './canvas/nodes/FileNode.svelte';
  import DocumentNode       from './canvas/nodes/DocumentNode.svelte';
  import MediaNode          from './canvas/nodes/MediaNode.svelte';
  import ToolNode           from './canvas/nodes/ToolNode.svelte';
  import McpNode            from './canvas/nodes/McpNode.svelte';
  import HttpRequestNode    from './canvas/nodes/HttpRequestNode.svelte';
  import RuleNode           from './canvas/nodes/RuleNode.svelte';
  import CodingAssistantNode from './canvas/nodes/CodingAssistantNode.svelte';
  import GitNode            from './canvas/nodes/GitNode.svelte';

  const NODE_TYPES = {
    'ai-assistant':     AiAssistantNode,
    'text':             TextNode,
    'file':             FileNode,
    'document':         DocumentNode,
    'media':            MediaNode,
    'tool':             ToolNode,
    'mcp':              McpNode,
    'http-request':     HttpRequestNode,
    'rule':             RuleNode,
    'coding-assistant': CodingAssistantNode,
    'git':              GitNode,
  };

  let {
    darkMode = true,
    onClose,
    onCollapseRail,
    onRestoreRail,
  }: {
    darkMode?:      boolean;
    onClose:        () => void;
    onCollapseRail: () => void;
    onRestoreRail:  () => void;
  } = $props();

  // ── Core state ────────────────────────────────────────────────────────
  let activeFlow        = $state<Flow | null>(null);
  let nodes             = $state<Node[]>([]);
  let edges             = $state<Edge[]>([]);
  let selectedNodeId    = $state<string | null>(null);
  let showPicker        = $state(false);
  let nodeTypes         = $state<NodeType[]>([]);
  let toastMsg          = $state('');
  let toastTimeout: ReturnType<typeof setTimeout> | null = null;

  // Undo / redo
  let undoStack         = $state<Snapshot[]>([]);
  let redoStack         = $state<Snapshot[]>([]);
  let lastSavedSnapshot = $state<Snapshot | null>(null);
  // Threshold warning flags (fire once)
  let warnedNodes       = $state(false);
  let warnedEdges       = $state(false);

  const MAX_UNDO = 50;

  const isDirty = $derived(
    lastSavedSnapshot !== null &&
    JSON.stringify({ nodes, edges }) !== JSON.stringify(lastSavedSnapshot)
  );
  const canUndo = $derived(undoStack.length > 0);
  const canRedo = $derived(redoStack.length > 0);

  const selectedNode = $derived(nodes.find(n => n.id === selectedNodeId) ?? null);
  const selectedNodeType = $derived(
    selectedNode ? (nodeTypes.find(t => t.id === selectedNode.type) ?? null) : null
  );

  // ── Toast ──────────────────────────────────────────────────────────────
  function toast(msg: string) {
    toastMsg = msg;
    if (toastTimeout) clearTimeout(toastTimeout);
    toastTimeout = setTimeout(() => { toastMsg = ''; }, 4000);
  }

  // ── Snapshot helpers ────────────────────────────────────────────────────
  function takeSnapshot() {
    const snap: Snapshot = { nodes: JSON.parse(JSON.stringify(nodes)), edges: JSON.parse(JSON.stringify(edges)) };
    undoStack = [snap, ...undoStack].slice(0, MAX_UNDO);
    redoStack = [];
  }

  function undo() {
    if (!undoStack.length) return;
    const [top, ...rest] = undoStack;
    redoStack = [{ nodes: JSON.parse(JSON.stringify(nodes)), edges: JSON.parse(JSON.stringify(edges)) }, ...redoStack];
    undoStack = rest;
    nodes = top.nodes;
    edges = top.edges;
  }

  function redo() {
    if (!redoStack.length) return;
    const [top, ...rest] = redoStack;
    undoStack = [{ nodes: JSON.parse(JSON.stringify(nodes)), edges: JSON.parse(JSON.stringify(edges)) }, ...undoStack].slice(0, MAX_UNDO);
    redoStack = rest;
    nodes = top.nodes;
    edges = top.edges;
  }

  // ── Load flow ───────────────────────────────────────────────────────────
  async function loadFlow(flow: Flow) {
    activeFlow = flow;
    nodes = (flow.flow_data?.nodes ?? []) as Node[];
    edges = (flow.flow_data?.edges ?? []) as Edge[];
    lastSavedSnapshot = { nodes: JSON.parse(JSON.stringify(nodes)), edges: JSON.parse(JSON.stringify(edges)) };
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

  // ── Save ────────────────────────────────────────────────────────────────
  async function save() {
    if (!activeFlow) return;
    try {
      const res = await flowService.update(activeFlow.flow_id, {
        flow_name:     activeFlow.flow_name,
        flow_data:     { nodes: nodes as any, edges: edges as any },
        thumbnail_svg: generateThumbnail(),
      });
      activeFlow = res.flow;
      lastSavedSnapshot = { nodes: JSON.parse(JSON.stringify(nodes)), edges: JSON.parse(JSON.stringify(edges)) };
      toast('Saved');
    } catch {
      toast('Failed to save — your changes are preserved');
    }
  }

  // ── Thumbnail (simple SVG preview) ─────────────────────────────────────
  function generateThumbnail(): string {
    const W = 200, H = 120;
    const rects = nodes.slice(0, 8).map(n => {
      const x = Math.round((n.position.x / 2000) * W);
      const y = Math.round((n.position.y / 1200) * H);
      return `<rect x="${x}" y="${y}" width="28" height="16" rx="3" fill="#1e2535" stroke="#374151" stroke-width="0.5"/>`;
    }).join('');
    return `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 ${W} ${H}" width="${W}" height="${H}" style="background:#0d1117;">${rects}</svg>`;
  }

  // ── Drag-and-drop from palette ──────────────────────────────────────────
  function onPaletteDragStart(event: DragEvent, nodeType: NodeType) {
    event.dataTransfer?.setData('application/xyflow-node-type', JSON.stringify(nodeType));
  }

  function onCanvasDrop(event: DragEvent) {
    event.preventDefault();
    const raw = event.dataTransfer?.getData('application/xyflow-node-type');
    if (!raw) return;
    const nodeType: NodeType = JSON.parse(raw);
    const bounds = (event.currentTarget as HTMLElement).getBoundingClientRect();
    const position = { x: event.clientX - bounds.left, y: event.clientY - bounds.top };
    const id = `${nodeType.id}-${Date.now()}`;
    takeSnapshot();
    nodes = [...nodes, { id, type: nodeType.id, position, data: { ...nodeType.defaultData, label: nodeType.label } }];
    selectedNodeId = id;
    checkLimits();
  }

  function checkLimits() {
    if (!warnedNodes && nodes.length > 500) { toast('Warning: over 500 nodes — performance may degrade'); warnedNodes = true; }
    if (!warnedEdges && edges.length > 1000) { toast('Warning: over 1000 edges — performance may degrade'); warnedEdges = true; }
  }

  // ── Close with guard ────────────────────────────────────────────────────
  function requestClose() {
    if (isDirty) {
      if (!confirm('You have unsaved changes. Discard and continue?')) return;
    }
    onRestoreRail();
    onClose();
  }

  // ── Node update from properties panel ───────────────────────────────────
  function updateNodeData(nodeId: string, data: Record<string, unknown>) {
    nodes = nodes.map(n => n.id === nodeId ? { ...n, data } : n);
  }

  // ── New empty flow ───────────────────────────────────────────────────────
  async function createNewEmpty() {
    const emptySnap: Snapshot = { nodes: [], edges: [] };
    lastSavedSnapshot = emptySnap;
    nodes = [];
    edges = [];
    undoStack = [];
    redoStack = [];
    try {
      const res = await flowService.create({
        flow_name: 'Untitled Flow',
        flow_data: { nodes: [], edges: [] },
        is_shared: false,
        thumbnail_svg: null,
      });
      activeFlow = res.flow;
      lastSavedSnapshot = { nodes: [], edges: [] };
    } catch {
      toast('Could not create flow on server — working offline');
      activeFlow = {
        flow_id: -1, user_id: 0, flow_name: 'Untitled Flow',
        flow_desc: '', is_default: false, is_shared: false,
        is_template: false, template_category: '',
        flow_data: { nodes: [], edges: [] }, thumbnail_svg: null,
        created_at: new Date().toISOString(), updated_at: new Date().toISOString(),
      };
    }
    showPicker = false;
  }

  // ── Save as template ────────────────────────────────────────────────────
  async function saveCurrentAsTemplate() {
    if (!activeFlow || activeFlow.flow_id < 0) { toast('Save the flow first'); return; }
    try {
      await flowService.saveAsTemplate(activeFlow.flow_id);
      toast('Saved as template');
    } catch { toast('Failed to save as template'); }
  }

  // ── Keyboard shortcuts ───────────────────────────────────────────────────
  function onKeydown(e: KeyboardEvent) {
    const mod = e.ctrlKey || e.metaKey;
    if (mod && e.key === 'z' && !e.shiftKey) { e.preventDefault(); undo(); }
    if (mod && (e.key === 'y' || (e.key === 'z' && e.shiftKey))) { e.preventDefault(); redo(); }
    if (mod && e.key === 's') { e.preventDefault(); save(); }
  }

  // ── Lifecycle ────────────────────────────────────────────────────────────
  onMount(async () => {
    onCollapseRail();
    const res = await flowService.getNodeTypes();
    nodeTypes = res.nodeTypes ?? [];
    await loadDefaultOrPicker();
  });

  onDestroy(() => {
    if (toastTimeout) clearTimeout(toastTimeout);
  });
</script>

<svelte:window onkeydown={onKeydown} />

<div class="fixed inset-0 flex flex-col" style="background:#0d1117; z-index:40; left:56px;">

  <!-- Toolbar -->
  <FlowToolbar
    {activeFlow}
    {isDirty}
    {canUndo}
    {canRedo}
    {darkMode}
    onPickerOpen={() => { if (isDirty) { if (!confirm('You have unsaved changes. Discard and continue?')) return; } showPicker = true; }}
    onRename={(name) => { if (activeFlow) { activeFlow = { ...activeFlow, flow_name: name }; } }}
    onUndo={undo}
    onRedo={redo}
    onFitView={() => {}}
    onSave={save}
    onSaveAsTemplate={saveCurrentAsTemplate}
    onClose={requestClose}
  />

  <!-- 3-panel body -->
  <div class="flex flex-1 overflow-hidden">

    <!-- Node Palette -->
    <NodePalette {nodeTypes} {darkMode} onDragStart={onPaletteDragStart} />

    <!-- Canvas -->
    <div
      class="flex-1 overflow-hidden"
      ondragover={(e) => e.preventDefault()}
      ondrop={onCanvasDrop}
      role="application"
      aria-label="Flow canvas"
    >
      <SvelteFlow
        bind:nodes
        bind:edges
        nodeTypes={NODE_TYPES}
        fitView
        onselectionchange={(params) => {
          selectedNodeId = params.nodes.length === 1 ? params.nodes[0].id : null;
        }}
        onnodedragstop={() => takeSnapshot()}
        onconnect={(connection) => {
          takeSnapshot();
          const sh = connection.sourceHandle ?? 'out';
          const th = connection.targetHandle ?? 'in';
          edges = [...edges, {
            id: `e-${connection.source}-${sh}-${connection.target}-${th}-${Date.now()}`,
            source: connection.source,
            sourceHandle: sh,
            target: connection.target,
            targetHandle: th,
            type: 'smoothstep',
          }];
          checkLimits();
        }}
        ondelete={() => takeSnapshot()}
        style="background:#0d1117;"
      >
        <Background variant={BackgroundVariant.Dots} gap={24} size={1} patternColor="#374151" />
        <Controls />
        <MiniMap style="background:#161b27;" />
      </SvelteFlow>
    </div>

    <!-- Properties Panel -->
    <PropertiesPanel
      node={selectedNode as any}
      nodeType={selectedNodeType}
      {darkMode}
      onUpdate={updateNodeData}
    />
  </div>

  <!-- Toast -->
  {#if toastMsg}
    <div
      class="fixed bottom-6 left-1/2 -translate-x-1/2 px-4 py-2 rounded-lg text-sm shadow-xl"
      style="background:#1e2535; border:1px solid #374151; color:#e2e8f0; z-index:60; pointer-events:none;"
    >
      {toastMsg}
    </div>
  {/if}
</div>

<!-- Flow Picker Modal -->
{#if showPicker}
  <FlowPicker
    {darkMode}
    currentUserId={activeFlow?.user_id ?? 0}
    onOpen={loadFlow}
    onNewEmpty={createNewEmpty}
    onClose={() => { if (!activeFlow) requestClose(); else showPicker = false; }}
  />
{/if}
