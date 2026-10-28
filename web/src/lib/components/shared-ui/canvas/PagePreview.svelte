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

 let { descriptor, nodes, selectedId, onSelect }: { descriptor: JsonObject; nodes: Node[]; selectedId: string | null; onSelect: (id: string) => void } = $props();
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

<div class="preview select-text" data-wpi-page-preview>
 {#if failures.length}<ErrorBox title={m.wpi_editor_element_classes_failed()} message={failures.join(', ')} />{/if}
 {#if ready}
  <div class="h-full p-3" data-page={page.descriptor_id}>
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
</div>

<style>
 .preview { position: absolute; left: 0; top: 0; width: 1000px; height: 650px; background: var(--background); color: var(--foreground); border: 1px solid var(--cv-border); overflow: auto; pointer-events: auto; }
 .element { position: relative; }
 .overlay { position: absolute; inset: 0; z-index: 2; width: 100%; height: 100%; min-height: 20px; background: transparent; border: 1px solid transparent; cursor: pointer; }
 .overlay:hover { background: rgba(99,102,241,.08); border-color: rgba(99,102,241,.5); }
 .overlay.selected, .overlay:focus-visible { background: rgba(99,102,241,.08); border: 2px solid #6366f1; outline: none; }
</style>
