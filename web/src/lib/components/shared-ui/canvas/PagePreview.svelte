<script lang="ts">
 // The page being edited, drawn on the canvas with the interpreter's layout and
 // element components (OpenSpec wpi-pages-own-panes, design D12): its pane tree,
 // and in each leaf pane its element nodes. Each element has a selection
 // overlay; each leaf pane is a drop target (data-editor-pane) and is selected
 // by its tag or a click on its empty area.
 import { onMount, untrack } from 'svelte';
 import type { Node } from '@xyflow/svelte';
 import { m } from '$lib/paraglide/messages.js';
 import { getElementClass } from '$lib/wpi/api';
 import { AppRuntime, setRuntime } from '$lib/wpi/runtime.svelte';
 import { resolveEntry } from '$lib/wpi/entries';
 import { effectiveRoot, flowItemStyle, flowStyle, inheritPaneColors, inheritPaneBorder, paneBorderStyle, isLeaf, pageBoxStyle, panelStyle, paneStyle, placeStyle, splitStyle, surfaceColorStyle } from '$lib/wpi/layout';
 import { flowGeometry } from '$lib/wpi/layouters/flow-geometry';
 import ElementView from '$lib/wpi/elements/Element.svelte';
 import ErrorBox from '$lib/wpi/ErrorBox.svelte';
 import type { Bundle, ClassRow, ElementInstance, JsonObject, PageDescriptor, PaneSpec, SurfaceColors, PaneBorder } from '$lib/wpi/types';
 import { instanceJson, isElementNode, paneNodes } from './wpi-json';
 import { MIN_PANE_SIZE } from './pane-edit';

 let { darkMode, descriptor, nodes, selectedId, selectedPane, size, zoom, onResizeStart, onResize, onSelect, onSelectPane, onMeasure, onPaneResize, onOuterResize, onOuterShift, onMeasurePanes }: {
  darkMode: boolean; descriptor: JsonObject; nodes: Node[]; selectedId: string | null; selectedPane: string | null;
  size: { width: number; height: number }; zoom: number;
  onResizeStart: () => void; onResize: (size: { width: number; height: number }) => void;
  onSelect: (id: string) => void; onSelectPane: (pane: string) => void;
  onPaneResize: (a: string, b: string, axis: 'width' | 'height', first: number, second: number, parentSize: number) => void;
  onMeasurePanes: (panes: { id: string; width: number; height: number; parentWidth: number; parentHeight: number }[]) => void;
  onOuterShift: (x: number, y: number, start: boolean) => void;
  onOuterResize: (pane: string | null, axis: 'width' | 'height', value: number, parentSize: number, pageSize: number) => void;
  onMeasure: (id: string, size: { width: number; height: number }) => void;
 } = $props();
 let previewElement: HTMLElement;
 function measurePanes() {
  requestAnimationFrame(() => {
   onMeasurePanes(Array.from(previewElement.querySelectorAll<HTMLElement>('[data-pane-id]')).map((pane) => ({ id: pane.dataset.paneId!, width: pane.getBoundingClientRect().width / (zoom || 1), height: pane.getBoundingClientRect().height / (zoom || 1), parentWidth: (pane.parentElement?.getBoundingClientRect().width ?? 0) / (zoom || 1), parentHeight: (pane.parentElement?.getBoundingClientRect().height ?? 0) / (zoom || 1) })));
  });
 }
 let drag: { id: number; x: number; y: number; width: number; height: number; zoom: number } | null = null;
 function startResize(event: PointerEvent) {
  if (event.button !== 0) return;
  drag = { id: event.pointerId, x: event.clientX, y: event.clientY, ...size, zoom: zoom || 1 };
  (event.currentTarget as HTMLElement).setPointerCapture(event.pointerId);
  onResizeStart();
  event.stopPropagation();
  event.preventDefault();
 }
 function resize(event: PointerEvent) {
  if (!drag || drag.id !== event.pointerId) return;
  onResize({ width: Math.max(240, Math.round(drag.width + (event.clientX - drag.x) / drag.zoom)), height: Math.max(160, Math.round(drag.height + (event.clientY - drag.y) / drag.zoom)) });
  event.stopPropagation();
 }
 function resizeKey(event: KeyboardEvent) {
  let dx = 0, dy = 0; const step = event.shiftKey ? 50 : 10;
  if (event.key === 'ArrowRight') dx = step; else if (event.key === 'ArrowLeft') dx = -step;
  else if (event.key === 'ArrowDown') dy = step; else if (event.key === 'ArrowUp') dy = -step; else return;
  onResizeStart();
  onResize({ width: Math.max(240, size.width + dx), height: Math.max(160, size.height + dy) });
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
 type Side = 'top' | 'bottom' | 'left' | 'right';
 let outerDrag: { pointer: number; side: Side; axis: 'width' | 'height'; start: number; initial: number; page: number; pane: HTMLElement | null; oldFlex: string; id: string | null; value: number; parentSize: number } | null = null;
 function startOuter(event: PointerEvent, side: Side) {
  if (event.button !== 0) return;
  const axis = side === 'left' || side === 'right' ? 'width' : 'height';
  let pane = (event.currentTarget as HTMLElement).parentElement as HTMLElement | null;
  while (pane && pane.dataset.paneAxis !== axis) pane = pane.parentElement?.closest<HTMLElement>('[data-pane-axis]') ?? null;
  const initial = pane ? pane.getBoundingClientRect()[axis] / (zoom || 1) : size[axis];
  const parentSize = pane?.parentElement ? pane.parentElement.getBoundingClientRect()[axis] / (zoom || 1) : size[axis];
  outerDrag = { pointer: event.pointerId, side, axis, start: axis === 'width' ? event.clientX : event.clientY, initial, page: size[axis], pane, oldFlex: pane?.style.flex ?? '', id: pane?.dataset.paneId ?? null, value: initial, parentSize };
  (event.currentTarget as HTMLElement).setPointerCapture(event.pointerId);
  onResizeStart(); onOuterShift(0, 0, true); event.stopPropagation(); event.preventDefault();
 }
 function moveOuter(event: PointerEvent) {
  const d = outerDrag;
  if (!d || d.pointer !== event.pointerId) return;
  const sign = d.side === 'left' || d.side === 'top' ? -1 : 1;
  const delta = sign * ((d.axis === 'width' ? event.clientX : event.clientY) - d.start) / (zoom || 1);
  const change = Math.max(MIN_PANE_SIZE - d.initial, (d.axis === 'width' ? 240 : 160) - d.page, delta);
  d.value = Math.round(d.initial + change);
  if (d.pane) d.pane.style.flex = `0 0 ${d.value}px`;
  onResize({ ...size, [d.axis]: Math.round(d.page + change) });
  onOuterShift(d.side === 'left' ? -change : 0, d.side === 'top' ? -change : 0, false);
  event.stopPropagation();
 }
 function endOuter(event: PointerEvent, cancel = false) {
  const d = outerDrag;
  if (!d || d.pointer !== event.pointerId) return;
  if (d.pane) d.pane.style.flex = d.oldFlex;
  outerDrag = null;
  if (cancel) { onResize({ ...size, [d.axis]: d.page }); onOuterShift(0, 0, false); }
  else if (Math.abs(d.value - d.initial) >= 1) onOuterResize(d.id, d.axis, d.value, d.parentSize, Math.round(d.page + d.value - d.initial));
  if (!cancel) measurePanes();
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
  rt.start();
  fetchClasses();
  ready = true;
 });
</script>

{#snippet paneTree(pane: PaneSpec, axis: 'width' | 'height' | undefined, next: string | undefined = undefined, sides: Side[] = ['top', 'bottom', 'left', 'right'], inheritedColors: SurfaceColors = {}, inheritedBorder: PaneBorder = {})}
 {@const colors = inheritPaneColors(pane, inheritedColors)}
 {@const border = inheritPaneBorder(pane, inheritedBorder)}
 <section class="pane" class:pane-selected={selectedPane === pane.pane_id} data-editor-pane={isLeaf(pane) ? pane.pane_id : undefined} data-pane-id={pane.pane_id} data-pane-axis={axis}
  style={`${axis ? `${paneStyle(pane, axis)}; min-width:${MIN_PANE_SIZE}px; min-height:${MIN_PANE_SIZE}px; max-width:none; max-height:none; ${next ? '' : 'flex-grow:1'}` : 'flex: 1 1 auto; width: 100%; min-width: 0; min-height: 0'}; ${surfaceColorStyle(colors)}; ${paneBorderStyle(border)}`}>
  {#if isLeaf(pane)}
   {@const items = entries(pane.pane_id)}
   {@const config = panelConfig(pane.pane_id)}
   {@const panel = page.panels?.[pane.pane_id]}
   <!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_static_element_interactions -->
   <div style={placeStyle(pane)} onclick={(event) => { event.stopPropagation(); onSelectPane(pane.pane_id); }}>
    <div class="flow flex flex-wrap select-text" style="{flowStyle(config)}; {panelStyle(items.map((it) => ('error' in it.entry ? {} : it.entry.desc)))}; {surfaceColorStyle(panel ?? {}, !!colors.background_color)}" data-flow-layout use:flowGeometry={{ config, count: items.length }}>
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
    {#each pane.children ?? [] as child, i (child.pane_id)}{@render paneTree(child, pane.split === 'columns' ? 'width' : 'height', pane.children?.[i + 1]?.pane_id, sides.filter((side) => pane.split === 'columns' ? (side !== 'left' || i === 0) && (side !== 'right' || i === (pane.children?.length ?? 0) - 1) : (side !== 'top' || i === 0) && (side !== 'bottom' || i === (pane.children?.length ?? 0) - 1)), colors, border)}{/each}
   </div>
  {/if}
  {#if isLeaf(pane)}
   {#each sides as side}
    <div role="separator" aria-label={m.wpi_resize_pane()} aria-orientation={side === 'left' || side === 'right' ? 'vertical' : 'horizontal'} class="outer-edge nodrag nopan {side}" onpointerdown={(event) => startOuter(event, side)} onpointermove={moveOuter} onpointerup={(event) => endOuter(event)} onpointercancel={(event) => endOuter(event, true)}></div>
   {/each}
  {/if}
  {#if axis && next}
   <div role="separator" aria-label={m.wpi_resize_pane()} aria-orientation={axis === 'width' ? 'vertical' : 'horizontal'} class="pane-divider nodrag nopan" class:vertical={axis === 'width'} onpointerdown={(event) => startPaneResize(event, pane.pane_id, next, axis)} onpointermove={movePaneResize} onpointerup={(event) => endPaneResize(event)} onpointercancel={(event) => endPaneResize(event, true)}></div>
  {/if}
 </section>
{/snippet}

<div bind:this={previewElement} class="preview select-text" style="width:{size.width}px; height:{size.height}px; color-scheme:{darkMode ? 'dark' : 'light'}" data-wpi-page-preview>
 {#if failures.length}<ErrorBox title={m.wpi_editor_element_classes_failed()} message={failures.join(', ')} />{/if}
 {#if ready}
  <div style={placeStyle({ hori_align: page.hori_align, vert_align: page.vert_align, hori_scrollbar: page.hori_scrollbar, vert_scrollbar: page.vert_scrollbar })}>
   <div style="{pageBoxStyle(page)}; width:100%; height:100%; max-width:none; max-height:none" data-page={page.descriptor_id} data-page-box>
    {@render paneTree(root, undefined)}
   </div>
  </div>
 {/if}
 <button type="button" class="resize-handle nodrag nopan select-none" aria-label={m.wpi_resize_preview()} title={m.wpi_resize_preview()} onpointerdown={startResize} onpointermove={resize} onpointerup={(event) => { if (drag?.id === event.pointerId) { drag = null; measurePanes(); } event.stopPropagation(); }} onpointercancel={() => drag = null} onkeydown={resizeKey}></button>
</div>

<style>
 .preview { position: absolute; left: 0; top: 0; background: var(--background); color: var(--foreground); border: 1px solid var(--cv-border); overflow: hidden; pointer-events: auto; }
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
