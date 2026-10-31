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
 import { effectiveRoot, flowItemStyle, flowStyle, isLeaf, pageBoxStyle, panelStyle, paneStyle, placeStyle, splitStyle } from '$lib/wpi/layout';
 import ElementView from '$lib/wpi/elements/Element.svelte';
 import ErrorBox from '$lib/wpi/ErrorBox.svelte';
 import type { Bundle, ClassRow, ElementInstance, JsonObject, PageDescriptor, PaneSpec } from '$lib/wpi/types';
 import { instanceJson, isElementNode, paneNodes } from './wpi-json';

 let { descriptor, nodes, selectedId, selectedPane, size, zoom, onResizeStart, onResize, onSelect, onSelectPane, onMeasure, onPaneResize }: {
  descriptor: JsonObject; nodes: Node[]; selectedId: string | null; selectedPane: string | null;
  size: { width: number; height: number }; zoom: number;
  onResizeStart: () => void; onResize: (size: { width: number; height: number }) => void;
  onSelect: (id: string) => void; onSelectPane: (pane: string) => void;
  onPaneResize: (a: string, b: string, axis: 'width' | 'height', first: number, second: number) => void;
  onMeasure: (id: string, size: { width: number; height: number }) => void;
 } = $props();
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
  event.preventDefault(); event.stopPropagation();
 }
 let paneDrag: { pointer: number; start: number; first: number; second: number; a: string; b: string; axis: 'width' | 'height'; left: HTMLElement; right: HTMLElement; value: number; oldLeft: string; oldRight: string; min: number; max: number } | null = null;
 function startPaneResize(event: PointerEvent, a: string, b: string, axis: 'width' | 'height') {
  if (event.button !== 0) return;
  const left = (event.currentTarget as HTMLElement).parentElement as HTMLElement;
  const right = left.nextElementSibling as HTMLElement | null;
  if (!right) return;
  const first = left.getBoundingClientRect()[axis] / (zoom || 1);
  const second = right.getBoundingClientRect()[axis] / (zoom || 1);
  const l = getComputedStyle(left), r = getComputedStyle(right);
  const minOf = (style: CSSStyleDeclaration) => parseFloat(axis === 'width' ? style.minWidth : style.minHeight) || 0;
  const maxOf = (style: CSSStyleDeclaration) => parseFloat(axis === 'width' ? style.maxWidth : style.maxHeight) || Infinity;
  paneDrag = { pointer: event.pointerId, start: axis === 'width' ? event.clientX : event.clientY, first, second, a, b, axis, left, right, value: first, oldLeft: left.style.flex, oldRight: right.style.flex, min: Math.max(minOf(l), first + second - maxOf(r)), max: Math.min(maxOf(l), first + second - minOf(r)) };
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
  if (!cancel && Math.abs(d.value - d.first) >= 1) onPaneResize(d.a, d.b, d.axis, d.value, Math.round(d.first + d.second - d.value));
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

{#snippet paneTree(pane: PaneSpec, axis: 'width' | 'height' | undefined, next: string | undefined = undefined)}
 <section class="pane" class:pane-selected={selectedPane === pane.pane_id} data-editor-pane={isLeaf(pane) ? pane.pane_id : undefined}
  style={axis ? paneStyle(pane, axis) : 'flex: 1 1 auto; width: 100%; min-width: 0; min-height: 0'}>
  {#if isLeaf(pane)}
   {@const items = entries(pane.pane_id)}
   <!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_static_element_interactions -->
   <div style={placeStyle(pane)} onclick={(event) => { event.stopPropagation(); onSelectPane(pane.pane_id); }}>
    <div class="flex flex-wrap select-text" style="{flowStyle(panelConfig(pane.pane_id))}; {panelStyle(items.map((it) => ('error' in it.entry ? {} : it.entry.desc)))}" data-flow-layout>
     {#each items as item (item.node.id)}
      <div class="element min-w-0" use:measure={'error' in item.entry || item.entry.module !== 'button' ? null : item.node.id} style={'error' in item.entry ? '' : flowItemStyle(item.entry.desc)} data-flow-item>
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
   <div style={splitStyle(pane)} class={pane.split === 'columns' ? 'divide-x' : 'divide-y'}>
    {#each pane.children ?? [] as child, i (child.pane_id)}{@render paneTree(child, pane.split === 'columns' ? 'width' : 'height', pane.children?.[i + 1]?.pane_id)}{/each}
   </div>
  {/if}
  {#if axis && next}
   <div role="separator" aria-label={m.wpi_resize_pane()} aria-orientation={axis === 'width' ? 'vertical' : 'horizontal'} class="pane-divider nodrag nopan" class:vertical={axis === 'width'} onpointerdown={(event) => startPaneResize(event, pane.pane_id, next, axis)} onpointermove={movePaneResize} onpointerup={(event) => endPaneResize(event)} onpointercancel={(event) => endPaneResize(event, true)}></div>
  {/if}
 </section>
{/snippet}

<div class="preview select-text" style="width:{size.width}px; height:{size.height}px" data-wpi-page-preview>
 {#if failures.length}<ErrorBox title={m.wpi_editor_element_classes_failed()} message={failures.join(', ')} />{/if}
 {#if ready}
  <div style={placeStyle({ hori_align: page.hori_align, vert_align: page.vert_align, hori_scrollbar: page.hori_scrollbar, vert_scrollbar: page.vert_scrollbar })}>
   <div style={pageBoxStyle(page)} data-page={page.descriptor_id} data-page-box>
    {@render paneTree(root, undefined)}
   </div>
  </div>
 {/if}
 <button type="button" class="resize-handle nodrag nopan select-none" aria-label={m.wpi_resize_preview()} title={m.wpi_resize_preview()} onpointerdown={startResize} onpointermove={resize} onpointerup={(event) => { if (drag?.id === event.pointerId) drag = null; event.stopPropagation(); }} onpointercancel={() => drag = null} onkeydown={resizeKey}></button>
</div>

<style>
 .preview { position: absolute; left: 0; top: 0; background: var(--background); color: var(--foreground); border: 1px solid var(--cv-border); overflow: hidden; pointer-events: auto; }
 .resize-handle { position:absolute; right:0; bottom:0; width:18px; height:18px; z-index:5; border:0; background: linear-gradient(135deg, transparent 45%, var(--cv-accent) 46%, var(--cv-accent) 52%, transparent 53%, transparent 64%, var(--cv-accent) 65%, var(--cv-accent) 71%, transparent 72%); cursor:nwse-resize; touch-action:none; }
 .resize-handle:focus-visible { outline:2px solid var(--cv-accent); }
 .pane { position: relative; outline: 1px dashed rgba(99,102,241,.25); outline-offset: -1px; }
 .pane-divider { position:absolute; z-index:4; bottom:-4px; left:0; right:0; height:8px; cursor:row-resize; touch-action:none; }
 .pane-divider.vertical { top:0; bottom:0; left:auto; right:-4px; width:8px; height:auto; cursor:col-resize; }
 .pane-divider:hover { background:rgba(99,102,241,.25); }
 .pane-selected { outline: 2px solid #6366f1; outline-offset: -2px; }
 .pane-tag { position: absolute; top: 2px; right: 2px; z-index: 3; border: 0; border-radius: 3px; padding: 0 4px; font-size: 9px; line-height: 14px; background: rgba(99,102,241,.12); color: var(--cv-text-muted); cursor: pointer; }
 .pane-tag[aria-pressed='true'] { background: #6366f1; color: #fff; }
 .element { position: relative; }
 .overlay { position: absolute; inset: 0; z-index: 2; width: 100%; height: 100%; min-height: 20px; background: transparent; border: 1px solid transparent; cursor: pointer; }
 .overlay:hover { background: rgba(99,102,241,.08); border-color: rgba(99,102,241,.5); }
 .overlay.selected, .overlay:focus-visible { background: rgba(99,102,241,.08); border: 2px solid #6366f1; outline: none; }
</style>
