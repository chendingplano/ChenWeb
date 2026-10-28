<script lang="ts">
 import { onMount, untrack } from 'svelte';
 import type { Node } from '@xyflow/svelte';
 import { m } from '$lib/paraglide/messages.js';
 import { getElementClass } from '$lib/wpi/api';
 import { AppRuntime, setRuntime } from '$lib/wpi/runtime.svelte';
 import { resolveEntry } from '$lib/wpi/entries';
 import { flowItemStyle, flowStyle, pageStyle } from '$lib/wpi/layout';
 import ElementView from '$lib/wpi/elements/Element.svelte';
 import ErrorBox from '$lib/wpi/ErrorBox.svelte';
 import type { Bundle, ClassRow, ElementInstance, JsonObject, PageDescriptor } from '$lib/wpi/types';
 import { isElementNode, nodeJson } from './wpi-json';

 let { descriptor, nodes, selectedId, size, zoom, onResizeStart, onResize, onSelect }: { descriptor: JsonObject; nodes: Node[]; selectedId: string | null; size: { width: number; height: number }; zoom: number; onResizeStart: () => void; onResize: (size: { width: number; height: number }) => void; onSelect: (id: string) => void } = $props();
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
 const initial = $derived(descriptor as unknown as PageDescriptor);
 const setupPage = untrack(() => descriptor as unknown as PageDescriptor);
 const bundle: Bundle = { app: { descriptor_id: 'editor-preview', schema_version: '0.2', layout_template: 'single-pane', panes: { preview: { page: setupPage.descriptor_id } } }, pages: { [setupPage.descriptor_id]: setupPage }, elements: {}, actions: {}, warnings: [] };
 const rt = new AppRuntime(bundle);
 setRuntime(rt);
 let classes = $state<Record<string, ClassRow>>({});
 let failures = $state<string[]>([]);
 let ready = $state(false);
 const elements = $derived(nodes.filter(isElementNode));
 const page = $derived({ ...initial, elements: elements.map((node) => nodeJson(node) as ElementInstance) });
 const entries = $derived(elements.map((node, i) => ({ node, entry: resolveEntry({ ...bundle, elements: classes }, page, page.elements[i], i) })));
 let panelId = $state('');
 const requested = new Set<string>();
 $effect(() => {
  if (!ready) return;
  for (const node of elements) {
   const id = String(node.data.element ?? '');
   if (!id || classes[id] || requested.has(id)) continue;
   requested.add(id);
   void getElementClass(id).then((row) => {
    const cls = row.class as unknown as ClassRow;
    classes = { ...classes, [id]: cls };
    bundle.elements = classes;
   }).catch(() => { failures = [...failures, id]; });
  }
 });
 onMount(() => {
  let cancelled = false;
  void (async () => {
   const ids = [...new Set(elements.map(node => String(node.data.element ?? '')).filter(Boolean))];
   const results = await Promise.allSettled(ids.map(id => getElementClass(id)));
   if (cancelled) return;
   const loaded: Record<string, ClassRow> = {};
   const failed: string[] = [];
   results.forEach((result, i) => { if (result.status === 'fulfilled') loaded[ids[i]] = result.value.class as unknown as ClassRow; else failed.push(ids[i]); });
   classes = loaded;
   bundle.elements = loaded;
   failures = failed;
   await rt.openPage({ page: initial.descriptor_id, pane: 'preview' });
   if (cancelled) return;
   panelId = rt.panes.preview.visible ?? '';
   ready = true;
  })();
  return () => { cancelled = true; };
 });
</script>

<div class="preview select-text" style="width:{size.width}px; height:{size.height}px" data-wpi-page-preview>
 {#if failures.length}<ErrorBox title={m.wpi_editor_element_classes_failed()} message={failures.join(', ')} />{/if}
 {#if ready}
  <div class="h-full overflow-auto p-3" data-page={page.descriptor_id}>
   <div style={pageStyle(page)} data-page-box>
    <div class="flex flex-wrap select-text" style={flowStyle(page)} data-flow-layout>
     {#each entries as item, i (item.node.id)}
      <div class="element min-w-0" style={'error' in item.entry ? '' : flowItemStyle(item.entry.desc)} data-flow-item>
       {#if 'error' in item.entry}
        <ErrorBox title={m.wpi_error_title({ descriptor: page.descriptor_id })} message={item.entry.error} />
       {:else}
        <ElementView {panelId} element={item.entry} />
       {/if}
       <button class:selected={selectedId === item.node.id} class="overlay nodrag nopan" aria-label={m.wpi_preview_select({ name: String(item.node.data.label ?? item.node.data.element ?? '') })} aria-pressed={selectedId === item.node.id} onpointerdown={(event) => event.stopPropagation()} onclick={(event) => { event.stopPropagation(); onSelect(item.node.id); }}></button>
      </div>
     {/each}
    </div>
   </div>
  </div>
 {/if}
 <button type="button" class="resize-handle nodrag nopan select-none" aria-label={m.wpi_resize_preview()} title={m.wpi_resize_preview()} onpointerdown={startResize} onpointermove={resize} onpointerup={(event) => { if (drag?.id === event.pointerId) drag = null; event.stopPropagation(); }} onpointercancel={() => drag = null} onkeydown={resizeKey}></button>
</div>

<style>
 .preview { position: absolute; left: 0; top: 0; background: var(--background); color: var(--foreground); border: 1px solid var(--cv-border); overflow: hidden; pointer-events: auto; }
 .resize-handle { position:absolute; right:0; bottom:0; width:18px; height:18px; z-index:5; border:0; background: linear-gradient(135deg, transparent 45%, var(--cv-accent) 46%, var(--cv-accent) 52%, transparent 53%, transparent 64%, var(--cv-accent) 65%, var(--cv-accent) 71%, transparent 72%); cursor:nwse-resize; touch-action:none; }
 .resize-handle:focus-visible { outline:2px solid var(--cv-accent); }
 .element { position: relative; }
 .overlay { position: absolute; inset: 0; z-index: 2; width: 100%; height: 100%; min-height: 20px; background: transparent; border: 1px solid transparent; cursor: pointer; }
 .overlay:hover { background: rgba(99,102,241,.08); border-color: rgba(99,102,241,.5); }
 .overlay.selected, .overlay:focus-visible { background: rgba(99,102,241,.08); border: 2px solid #6366f1; outline: none; }
</style>
