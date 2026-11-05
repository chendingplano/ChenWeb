<script lang="ts">
  import { m } from '$lib/paraglide/messages.js';
  import { canvasThemeVars } from './canvas-theme';
  import type { ComponentNode, ComponentTarget } from './wpi-json';
  let { tree, darkMode, onClose, onSelect }: { tree: ComponentNode; darkMode: boolean; onClose: () => void; onSelect: (target: ComponentTarget) => void } = $props();

  // A floating window: not modal, so the canvas stays usable behind it.
  let x = $state(Math.max(16, window.innerWidth - 440));
  let y = $state(140);
  let width = $state(400);
  let height = $state(420);

  // Drag by the header; pointer capture is not needed because the listeners sit on window.
  function startMove(event: PointerEvent) {
    if ((event.target as HTMLElement).closest('button')) return;
    const offsetX = event.clientX - x;
    const offsetY = event.clientY - y;
    const move = (e: PointerEvent) => { x = e.clientX - offsetX; y = e.clientY - offsetY; };
    const up = () => { window.removeEventListener('pointermove', move); window.removeEventListener('pointerup', up); };
    window.addEventListener('pointermove', move);
    window.addEventListener('pointerup', up);
  }

  // Resize from the lower-right corner.
  function startResize(event: PointerEvent) {
    event.preventDefault();
    const startX = event.clientX;
    const startY = event.clientY;
    const startWidth = width;
    const startHeight = height;
    const move = (e: PointerEvent) => {
      width = Math.max(240, startWidth + e.clientX - startX);
      height = Math.max(160, startHeight + e.clientY - startY);
    };
    const up = () => { window.removeEventListener('pointermove', move); window.removeEventListener('pointerup', up); };
    window.addEventListener('pointermove', move);
    window.addEventListener('pointerup', up);
  }
</script>

{#snippet branch(node: ComponentNode, depth: number)}
  {#if node.target}
    <button onclick={() => onSelect(node.target!)} class="node hover:underline" style="margin-left:{depth * 16}px;">{node.label}</button>
  {:else}
    <div class="node" style="margin-left:{depth * 16}px;">{node.label}</div>
  {/if}
  {#each node.children as child (child.id)}
    {@render branch(child, depth + 1)}
  {/each}
{/snippet}

<div class="fixed z-50 rounded-xl overflow-hidden shadow-2xl select-text"
  style="{canvasThemeVars(darkMode)} left:{x}px; top:{y}px; width:{width}px; height:{height}px; max-width:calc(100vw - 16px); background:var(--cv-panel); border:1px solid var(--cv-divider); display:flex; flex-direction:column;"
  role="dialog" aria-labelledby="wpi-component-tree-title">
  <div onpointerdown={startMove} class="cursor-move select-none"
    style="padding:10px 14px; border-bottom:1px solid var(--cv-divider); display:flex; justify-content:space-between; align-items:center; gap:12px;">
    <div id="wpi-component-tree-title" style="font-size:14px; font-weight:600; color:var(--cv-text);">{m.wpi_toolbar_component_tree()}</div>
    <button onclick={onClose} class="select-none" style="color:var(--cv-text-muted); font-size:16px; line-height:1; background:none; border:none; cursor:pointer;" aria-label={m.wpi_view_json_close()}>✕</button>
  </div>
  <div style="flex:1; min-height:0; overflow:auto; padding:10px 14px; font:12px/1.5 ui-monospace, SFMono-Regular, Menlo, monospace;">
    {@render branch(tree, 0)}
  </div>
  <div onpointerdown={startResize} class="select-none"
    style="position:absolute; right:0; bottom:0; width:14px; height:14px; cursor:nwse-resize; background:linear-gradient(135deg, transparent 50%, var(--cv-text-faint) 50%);"></div>
</div>

<style>
  .node {
    display: block;
    width: fit-content;
    margin-top: 4px;
    margin-bottom: 4px;
    padding: 3px 10px;
    border: 1px solid var(--cv-border);
    border-radius: 8px;
    background: var(--cv-surface);
    color: var(--cv-text);
    font: inherit;
    white-space: nowrap;
    text-align: left;
  }
  button.node { cursor: pointer; }
</style>
