<script lang="ts">
 // The page being edited, drawn on the canvas with the interpreter's layout and
 // element components (OpenSpec wpi-pages-own-panes, design D12): its pane tree,
 // and in each leaf pane its element nodes. Each element has a selection
 // overlay; each leaf pane is a drop target (data-editor-pane) and is selected
 // by its tag or a click on its empty area.
 import { onMount, tick, untrack } from 'svelte';
 import CopyIcon from '@lucide/svelte/icons/copy';
 import Trash2Icon from '@lucide/svelte/icons/trash-2';
 import PencilIcon from '@lucide/svelte/icons/pencil';
 import { getNodeActions } from './node-actions';
 import type { Node } from '@xyflow/svelte';
 import { m } from '$lib/paraglide/messages.js';
 import { getElementClass } from '$lib/wpi/api';
 import { AppRuntime, setRuntime } from '$lib/wpi/runtime.svelte';
 import { resolveEntry } from '$lib/wpi/entries';
 import { cssLength, effectiveRoot, flowItemStyle, flowStyle, inheritPaneColors, inheritPaneBorder, paneBorderStyle, isLeaf, pageBoxStyle, panelStyle, paneStyle, rootPaneStyle, placeStyle, splitStyle, surfaceColorStyle } from '$lib/wpi/layout';
 import { flowGeometry } from '$lib/wpi/layouters/flow-geometry';
 import ElementView from '$lib/wpi/elements/Element.svelte';
 import ErrorBox from '$lib/wpi/ErrorBox.svelte';
 import type { Bundle, ClassRow, ElementInstance, JsonObject, PageDescriptor, PaneSpec, SurfaceColors, PaneBorder } from '$lib/wpi/types';
 import { instanceJson, isElementNode, paneNodes } from './wpi-json';
 import { MIN_PANE_SIZE } from './pane-edit';
 import { pageDragSize, type PageSide } from './page-resize';

 let { darkMode, descriptor, nodes, selectedId, selectedPane, size, zoom, onResizeStart, onResize, onSelect, onSelectPane, onMeasure, onPaneResize, onPageResize, onPageCancel, onMeasurePanes }: {
  darkMode: boolean; descriptor: JsonObject; nodes: Node[]; selectedId: string | null; selectedPane: string | null;
  size: { width: number; height: number }; zoom: number;
  onResizeStart: () => void; onResize: (size: { width: number; height: number }) => void;
  onSelect: (id: string) => void; onSelectPane: (pane: string) => void;
  onPaneResize: (a: string, b: string, axis: 'width' | 'height', first: number, second: number, parentSize: number) => void;
  onMeasurePanes: (panes: { id: string; width: number; height: number; parentWidth: number; parentHeight: number }[]) => void;
  onPageResize: (axis: 'width' | 'height', value: number) => void;
  onPageCancel: (original: JsonObject) => void;
  onMeasure: (id: string, size: { width: number; height: number }) => void;
 } = $props();
 const actions = getNodeActions();
 let previewElement: HTMLElement;
 let toolbarPosition = $state<{ left: number; top: number } | null>(null);
 function positionToolbar() {
  const target = previewElement?.querySelector<HTMLElement>('.overlay.selected');
  if (!target) { toolbarPosition = null; return; }
  const root = previewElement.getBoundingClientRect();
  const rect = target.getBoundingClientRect();
  const scale = zoom || 1;
  if (rect.bottom <= root.top || rect.top >= root.bottom || rect.right <= root.left || rect.left >= root.right) { toolbarPosition = null; return; }
  toolbarPosition = { left: Math.max(55, Math.min(size.width - 55, (rect.left + rect.width / 2 - root.left) / scale)), top: Math.max(2, (rect.top - root.top) / scale - 30) };
 }
 $effect(() => {
  selectedId; nodes; descriptor; size; zoom; ready;
  void tick().then(positionToolbar);
 });
 function measurePanes() {
  requestAnimationFrame(() => {
   onMeasurePanes(Array.from(previewElement.querySelectorAll<HTMLElement>('[data-pane-id]')).map((pane) => ({ id: pane.dataset.paneId!, width: pane.getBoundingClientRect().width / (zoom || 1), height: pane.getBoundingClientRect().height / (zoom || 1), parentWidth: (pane.parentElement?.getBoundingClientRect().width ?? 0) / (zoom || 1), parentHeight: (pane.parentElement?.getBoundingClientRect().height ?? 0) / (zoom || 1) })));
  });
 }
 let drag: { id: number; x: number; y: number; width: number; height: number; zoom: number; pageWidth: number; pageHeight: number; original: JsonObject; movedX: boolean; movedY: boolean } | null = null;
 let resizeOutline = $state<{ width: number; height: number } | null>(null);
 function startResize(event: PointerEvent) {
  if (event.button !== 0) return;
  drag = { id: event.pointerId, x: event.clientX, y: event.clientY, ...size, zoom: zoom || 1, pageWidth: size.width, pageHeight: size.height, original: JSON.parse(JSON.stringify(descriptor)), movedX: false, movedY: false };
  resizeOutline = { ...size };
  (event.currentTarget as HTMLElement).setPointerCapture(event.pointerId);
  onResizeStart();
  event.stopPropagation();
  event.preventDefault();
 }
 function resize(event: PointerEvent) {
  if (!drag || drag.id !== event.pointerId) return;
  const dx = event.clientX - drag.x, dy = event.clientY - drag.y;
  resizeOutline = { width: Math.max(240, Math.round(drag.width + dx / drag.zoom)), height: Math.max(160, Math.round(drag.height + dy / drag.zoom)) };
  drag.movedX ||= dx !== 0; drag.movedY ||= dy !== 0;
  event.stopPropagation();
 }
 function endResize(event: PointerEvent, cancel = false) {
  if (!drag || drag.id !== event.pointerId) return;
  if (!cancel && resizeOutline) {
   onResize(resizeOutline);
   if (drag.movedX) onPageResize('width', resizeOutline.width);
   if (drag.movedY) onPageResize('height', resizeOutline.height);
   measurePanes();
  }
  drag = null; resizeOutline = null; event.stopPropagation();
 }
 function resizeKey(event: KeyboardEvent) {
  let dx = 0, dy = 0; const step = event.shiftKey ? 50 : 10;
  if (event.key === 'ArrowRight') dx = step; else if (event.key === 'ArrowLeft') dx = -step;
  else if (event.key === 'ArrowDown') dy = step; else if (event.key === 'ArrowUp') dy = -step; else return;
  onResizeStart();
  onResize({ width: Math.max(240, size.width + dx), height: Math.max(160, size.height + dy) });
  if (dx) onPageResize('width', Math.max(240, size.width + dx));
  if (dy) onPageResize('height', Math.max(160, size.height + dy));
  measurePanes(); event.preventDefault(); event.stopPropagation();
 }
 let paneDrag: { pointer: number; start: number; first: number; second: number; a: string; b: string; axis: 'width' | 'height'; left: HTMLElement; right: HTMLElement; value: number; oldLeft: string; oldRight: string; min: number; max: number } | null = null;
 function startPaneResize(event: PointerEvent, a: string, b: string, axis: 'width' | 'height') {
  if (event.button !== 0) return;
  const left = (event.currentTarget as HTMLElement).parentElement as HTMLElement;
  const right = left.nextElementSibling as HTMLElement | null;
  if (!right) return;
  const first = left.getBoundingClientRect()[axis] / (zoom || 1);
  const second = right.getBoundingClientRect()[axis] / (zoom || 1);
  paneDrag = { pointer: event.pointerId, start: axis === 'width' ? event.clientX : event.clientY, first, second, a, b, axis, left, right, value: first, oldLeft: left.style.flex, oldRight: right.style.flex, min: MIN_PANE_SIZE, max: Math.max(MIN_PANE_SIZE, first + second - MIN_PANE_SIZE) };
  (event.currentTarget as HTMLElement).setPointerCapture(event.pointerId);
  event.preventDefault(); event.stopPropagation();
 }
 function movePaneResize(event: PointerEvent) {
  const d = paneDrag;
  if (!d || d.pointer !== event.pointerId) return;
  const delta = ((d.axis === 'width' ? event.clientX : event.clientY) - d.start) / (zoom || 1);
  d.value = Math.round(Math.max(d.min, Math.min(d.max, d.first + delta)));
  d.left.style.flex = `0 0 ${d.value}px`;
  d.right.style.flex = `0 0 ${Math.round(d.first + d.second - d.value)}px`;
  event.stopPropagation();
 }
 function endPaneResize(event: PointerEvent, cancel = false) {
  const d = paneDrag;
  if (!d || d.pointer !== event.pointerId) return;
  d.left.style.flex = d.oldLeft; d.right.style.flex = d.oldRight;
  paneDrag = null;
  if (!cancel && Math.abs(d.value - d.first) >= 1) onPaneResize(d.a, d.b, d.axis, d.value, Math.round(d.first + d.second - d.value), d.left.parentElement!.getBoundingClientRect()[d.axis] / (zoom || 1));
  event.stopPropagation();
 }
 type Side = PageSide;
 let outerDrag: { pointer: number; side: Side; axis: 'width' | 'height'; start: number; initial: number; zoom: number; original: JsonObject } | null = null;
 function startOuter(event: PointerEvent, side: Side) {
  if (event.button !== 0) return;
  const axis = side === 'left' || side === 'right' ? 'width' : 'height';
  outerDrag = { pointer: event.pointerId, side, axis, start: axis === 'width' ? event.clientX : event.clientY, initial: size[axis], zoom: zoom || 1, original: JSON.parse(JSON.stringify(descriptor)) };
  resizeOutline = { ...size };
  (event.currentTarget as HTMLElement).setPointerCapture(event.pointerId);
  onResizeStart(); event.stopPropagation(); event.preventDefault();
 }
 function moveOuter(event: PointerEvent) {
  const d = outerDrag;
  if (!d || d.pointer !== event.pointerId) return;
  const movement = (d.axis === 'width' ? event.clientX : event.clientY) - d.start;
  const value = Math.max(d.axis === 'width' ? 240 : 160, pageDragSize(d.initial, movement, d.zoom, d.side));
  resizeOutline = { ...size, [d.axis]: value };
  event.stopPropagation();
 }
 function endOuter(event: PointerEvent, cancel = false) {
  const d = outerDrag;
  if (!d || d.pointer !== event.pointerId) return;
  outerDrag = null;
  if (!cancel && resizeOutline && resizeOutline[d.axis] !== d.initial) { onResize(resizeOutline); onPageResize(d.axis, resizeOutline[d.axis]); measurePanes(); }
  resizeOutline = null;
  event.stopPropagation();
 }
 const page = $derived(descriptor as unknown as PageDescriptor);
 const setupPage = untrack(() => descriptor as unknown as PageDescriptor);
 // The runtime gives elements a page instance whose input doc they show; the
 // page drawn here is the one being edited, not this bundle's copy.
 const bundle: Bundle = { app: { descriptor_id: 'editor-preview', schema_version: '0.5', initial_page: setupPage.descriptor_id }, pages: { [setupPage.descriptor_id]: setupPage }, warnings: [] };
 const rt = new AppRuntime(bundle, true);
 setRuntime(rt);
 let classes = $state<Record<string, ClassRow>>({});
 let failures = $state<string[]>([]);
 let ready = $state(false);
 const elements = $derived(nodes.filter(isElementNode));
 const root = $derived(effectiveRoot(page));
 // The element nodes of a leaf pane with their instances resolved; a node
 // without a class copy uses the class row fetched for it.
 function entries(pane: string) {
  return paneNodes(elements, pane).map((node, i) => ({ node, entry: resolveEntry(page, pane, instanceJson(node) as ElementInstance, i, classes) }));
 }
 function panelConfig(pane: string) {
  const el = page.panels?.[pane]?.elem_layouter;
  return el?.config ?? el?.class?.config ?? {};
 }
 // Observe the rendered control, not its zoomed canvas coordinates.
 function measure(host: HTMLElement, id: string | null) {
  let target: HTMLElement | null = null;
  const observer = new ResizeObserver(() => {
   if (target && id) onMeasure(id, { width: target.offsetWidth, height: target.offsetHeight });
  });
  function refresh() {
   const next = id ? host.querySelector<HTMLElement>('[data-slot="button"]') : null;
   if (next === target) return;
   if (target) observer.unobserve(target);
   target = next;
   if (target) observer.observe(target);
  }
  const mutations = new MutationObserver(refresh);
  mutations.observe(host, { childList: true, subtree: true });
  refresh();
  return { update(next: string | null) { id = next; refresh(); }, destroy() { mutations.disconnect(); observer.disconnect(); } };
 }
 const requested = new Set<string>();
 function fetchClasses() {
  for (const node of elements) {
   const id = String(node.data.element ?? '');
   if (!id || node.data.class || classes[id] || requested.has(id)) continue;
   requested.add(id);
   void getElementClass(id).then((row) => {
    classes = { ...classes, [id]: row.class as unknown as ClassRow };
   }).catch(() => { failures = [...failures, id]; });
  }
 }
 $effect(() => {
  if (ready) fetchClasses();
 });
 onMount(() => {
  const observer = new ResizeObserver(positionToolbar);
  observer.observe(previewElement);
  previewElement.addEventListener('scroll', positionToolbar, true);
  rt.start();
  fetchClasses();
  ready = true;
  return () => { observer.disconnect(); previewElement.removeEventListener('scroll', positionToolbar, true); };
 });
</script>

{#snippet paneTree(pane: PaneSpec, axis: 'width' | 'height' | undefined, next: string | undefined = undefined, inheritedColors: SurfaceColors = {}, inheritedBorder: PaneBorder = {})}
 {@const colors = inheritPaneColors(pane, inheritedColors)}
 {@const border = inheritPaneBorder(pane, inheritedBorder)}
 <section class="pane" class:pane-selected={selectedPane === pane.pane_id} data-editor-pane={isLeaf(pane) ? pane.pane_id : undefined} data-pane-id={pane.pane_id} data-pane-axis={axis}
  style={`${axis ? `${paneStyle(pane, axis)}; min-width:${MIN_PANE_SIZE}px; min-height:${MIN_PANE_SIZE}px; max-width:none; max-height:none; ${next ? '' : 'flex-grow:1'}` : rootPaneStyle(pane)}; ${surfaceColorStyle(colors)}; ${paneBorderStyle(border)}`}>
  {#if isLeaf(pane)}
   {@const items = entries(pane.pane_id)}
   {@const config = panelConfig(pane.pane_id)}
   {@const panel = page.panels?.[pane.pane_id]}
   <!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_static_element_interactions -->
   <div style={placeStyle(pane)} onclick={(event) => { event.stopPropagation(); onSelectPane(pane.pane_id); }}>
    <div class="flow flex flex-wrap select-text" style="{flowStyle(config)}; {panelStyle(items.map((it) => ('error' in it.entry ? {} : it.entry.desc)))}; {surfaceColorStyle(panel ?? {}, !!colors.background_color)}" data-flow-layout use:flowGeometry={{ config, count: items.length, sizeToRows: !items.some((it) => !('error' in it.entry) && (it.entry.desc.width === 'elastic' || (typeof it.entry.desc.width === 'string' && it.entry.desc.width.endsWith('%')))) }}>
     {#each items as item (item.node.id)}
      <div class="element min-w-0" class:flow-text={!('error' in item.entry) && item.entry.module === 'text'} class:stretch={!('error' in item.entry) && config.vertical_alignment === 'stretch' && (item.entry.desc.height === undefined || item.entry.desc.height === 'auto' || item.entry.desc.height === 'elastic')} use:measure={'error' in item.entry || item.entry.module !== 'button' ? null : item.node.id} style={'error' in item.entry ? '' : flowItemStyle(item.entry.desc, config)} data-flow-item>
       {#if 'error' in item.entry}
        <ErrorBox title={m.wpi_error_title({ descriptor: page.descriptor_id })} message={item.entry.error} />
       {:else if rt.root}
        <ElementView instanceId={rt.root} element={item.entry} />
       {/if}
       <button class:selected={selectedId === item.node.id} class="overlay nodrag nopan" aria-label={m.wpi_preview_select({ name: String(item.node.data.label ?? item.node.data.element ?? '') })} aria-pressed={selectedId === item.node.id} onpointerdown={(event) => event.stopPropagation()} onclick={(event) => { event.stopPropagation(); onSelect(item.node.id); }}></button>
      </div>
     {/each}
    </div>
   </div>
   <button type="button" class="pane-tag nodrag nopan" aria-label={m.wpi_select_pane({ pane: pane.pane_id })} aria-pressed={selectedPane === pane.pane_id} onpointerdown={(event) => event.stopPropagation()} onclick={(event) => { event.stopPropagation(); onSelectPane(pane.pane_id); }}>{pane.pane_id}</button>
  {:else}
   <div style={splitStyle(pane)}>
    {#each pane.children ?? [] as child, i (child.pane_id)}{@render paneTree(child, pane.split === 'columns' ? 'width' : 'height', pane.children?.[i + 1]?.pane_id, colors, border)}{/each}
   </div>
  {/if}
  {#if axis && next}
   <div role="separator" aria-label={m.wpi_resize_pane()} aria-orientation={axis === 'width' ? 'vertical' : 'horizontal'} class="pane-divider nodrag nopan" class:vertical={axis === 'width'} onpointerdown={(event) => startPaneResize(event, pane.pane_id, next, axis)} onpointermove={movePaneResize} onpointerup={(event) => endPaneResize(event)} onpointercancel={(event) => endPaneResize(event, true)}></div>
  {/if}
 </section>
{/snippet}

<div bind:this={previewElement} class="preview select-text" style="width:{size.width}px; height:{size.height}px; color-scheme:{darkMode ? 'dark' : 'light'}" data-wpi-page-preview>
 {#if failures.length}<ErrorBox title={m.wpi_editor_element_classes_failed()} message={failures.join(', ')} />{/if}
 {#if ready}
  <div style="{placeStyle({ hori_align: page.hori_align, vert_align: page.vert_align, hori_scrollbar: page.hori_scrollbar, vert_scrollbar: page.vert_scrollbar })}; padding: 0 40px 40px">
   <div style="{pageBoxStyle(page)}; position:relative; max-width:min(100%, {cssLength(page.max_width) ?? '100%'}); max-height:min(100%, {cssLength(page.max_height) ?? '100%'}); {(!page.height || page.height === 'auto') ? 'height:100%' : ''}" data-page={page.descriptor_id} data-page-box>
    {@render paneTree(root, undefined)}
   </div>
  </div>
 {/if}
    {#each ['bottom', 'right'] as side}
     <div role="separator" aria-label={m.wpi_resize_preview()} aria-orientation={side === 'left' || side === 'right' ? 'vertical' : 'horizontal'} class="outer-edge nodrag nopan {side}" onpointerdown={(event) => startOuter(event, side as Side)} onpointermove={moveOuter} onpointerup={(event) => endOuter(event)} onpointercancel={(event) => endOuter(event, true)}></div>
    {/each}
 {#if actions && selectedId && toolbarPosition}
  <div class="element-toolbar nodrag nopan" role="group" aria-label={m.wpi_element_actions()} style:left="{toolbarPosition.left}px" style:top="{toolbarPosition.top}px" onpointerdown={(event) => event.stopPropagation()}>
   {#if actions.editClass}<button title={m.basenode_edit_class()} aria-label={m.basenode_edit_class()} onclick={(event) => { event.stopPropagation(); actions.editClass?.(selectedId!); }}><PencilIcon class="w-3 h-3" /></button>{/if}
   <button title={m.basenode_duplicate()} aria-label={m.basenode_duplicate()} onclick={(event) => { event.stopPropagation(); actions.duplicate(selectedId!); }}><CopyIcon class="w-3 h-3" /></button>
   <button title={m.basenode_delete()} aria-label={m.basenode_delete()} onclick={(event) => { event.stopPropagation(); actions.remove(selectedId!); }}><Trash2Icon class="w-3 h-3" /></button>
  </div>
 {/if}
 <button type="button" class="resize-handle nodrag nopan select-none" aria-label={m.wpi_resize_preview()} title={m.wpi_resize_preview()} onpointerdown={startResize} onpointermove={resize} onpointerup={(event) => endResize(event)} onpointercancel={(event) => endResize(event, true)} onkeydown={resizeKey}></button>
</div>
{#if resizeOutline}
 <div class="resize-outline" style:width="{resizeOutline.width}px" style:height="{resizeOutline.height}px" aria-hidden="true"></div>
{/if}

<style>
 .preview { position: absolute; left: 0; top: 0; background: var(--background); color: var(--foreground); border: 1px solid var(--cv-border); overflow: hidden; pointer-events: auto; }
 .resize-outline { position:absolute; left:0; top:0; box-sizing:border-box; border:2px dashed var(--cv-accent); pointer-events:none; z-index:6; }
 .element-toolbar { position:absolute; z-index:10; transform:translateX(-50%); display:flex; gap:4px; background:var(--cv-surface); border:1px solid var(--cv-border); border-radius:6px; padding:3px 6px; }
 .element-toolbar button { background:none; border:0; cursor:pointer; color:var(--cv-text-2); padding:1px 3px; display:flex; align-items:center; }
 .element-toolbar button:last-child { color:#ef4444; }
 .resize-handle { position:absolute; right:0; bottom:0; width:18px; height:18px; z-index:5; border:0; background: linear-gradient(135deg, transparent 45%, var(--cv-accent) 46%, var(--cv-accent) 52%, transparent 53%, transparent 64%, var(--cv-accent) 65%, var(--cv-accent) 71%, transparent 72%); cursor:nwse-resize; touch-action:none; }
 .resize-handle:focus-visible { outline:2px solid var(--cv-accent); }
 .pane { position: relative; outline: 1px dashed rgba(99,102,241,.25); outline-offset: -1px; }
 .outer-edge { position:absolute; z-index:4; touch-action:none; }
 .outer-edge.top, .outer-edge.bottom { left:5px; right:5px; height:8px; cursor:row-resize; }
 .outer-edge.top { top:0; } .outer-edge.bottom { bottom:0; }
 .outer-edge.left, .outer-edge.right { top:5px; bottom:5px; width:8px; cursor:col-resize; }
 .outer-edge.left { left:0; } .outer-edge.right { right:0; }
 .outer-edge:hover { background:rgba(99,102,241,.25); }
 .pane-divider { position:absolute; z-index:4; bottom:-4px; left:0; right:0; height:8px; cursor:row-resize; touch-action:none; }
 .pane-divider.vertical { top:0; bottom:0; left:auto; right:-4px; width:8px; height:auto; cursor:col-resize; }
 .pane-divider:hover { background:rgba(99,102,241,.25); }
 .pane-selected { outline: 2px solid #6366f1 !important; outline-offset: -2px !important; }
 .pane-tag { position: absolute; top: 2px; right: 2px; z-index: 3; border: 0; border-radius: 3px; padding: 0 4px; font-size: 9px; line-height: 14px; background: rgba(99,102,241,.12); color: var(--cv-text-muted); cursor: pointer; }
 .pane-tag[aria-pressed='true'] { background: #6366f1; color: #fff; }
 .flow { position: relative; }
 .element { position: relative; box-sizing: border-box; overflow: auto; }
 .stretch { display: flex; flex-direction: column; }
 .stretch > :global(:not(.overlay)) { flex: 1 1 auto; }
 .overlay { position: absolute; inset: 0; z-index: 2; width: 100%; height: 100%; min-height: 20px; background: transparent; border: 1px solid transparent; cursor: pointer; }
 .overlay:hover { background: rgba(99,102,241,.08); border-color: rgba(99,102,241,.5); }
 .overlay.selected, .overlay:focus-visible { background: rgba(99,102,241,.08); border: 2px solid #6366f1; outline: none; }
</style>
