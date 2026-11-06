<script lang="ts">
  import { m } from '$lib/paraglide/messages.js';
  import { canvasThemeVars } from './canvas-theme';
  import type { ComponentNode, ComponentTarget } from './wpi-json';
  let { tree, darkMode, selectedTarget, onClose, onSelect, onDock, docked = false, x = $bindable(), y = $bindable() }: { x: number; y: number; tree: ComponentNode; darkMode: boolean; selectedTarget: ComponentTarget | null; onClose: () => void; onDock: () => void; docked?: boolean; onSelect: (target: ComponentTarget) => void } = $props();

  // A floating window: not modal, so the canvas stays usable behind it.
  let width = $state(400);
  let height = $state(420);
  let selectedId = $state<string | null>(null);

  // Drag by the header; pointer capture is not needed because the listeners sit on window.
  function startMove(event: PointerEvent) {
    if (event.button !== 0) return;
    event.preventDefault();
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
    <button onclick={() => { selectedId = node.id; onSelect(node.target!); }} class="node hover:underline" class:selected={selectedId === node.id && JSON.stringify(node.target) === JSON.stringify(selectedTarget)} aria-pressed={selectedId === node.id && JSON.stringify(node.target) === JSON.stringify(selectedTarget)} style="margin-left:{depth * 16}px;">{node.label}</button>
  {:else}
    <div class="node" style="margin-left:{depth * 16}px;">{node.label}</div>
  {/if}
  {#each node.children as child (child.id)}
    {@render branch(child, depth + 1)}
  {/each}
{/snippet}

{#if docked}
  <div class="docked-tree select-text" style={canvasThemeVars(darkMode)}>
    <div class="tree-heading"><span>{m.wpi_toolbar_component_tree()}</span><button type="button" class="control" onclick={onDock} aria-label={m.wpi_editor_detach_panel()} title={m.wpi_editor_detach_panel()}>↗</button></div>
    <div class="tree-content">{@render branch(tree, 0)}</div>
  </div>
{:else}
  <div class="tree-window select-text" style="{canvasThemeVars(darkMode)} left:{x}px; top:{y}px; width:{width}px; height:{height}px;" role="dialog" aria-labelledby="wpi-component-tree-title">
    <div class="controls">
      <button type="button" class="handle" onpointerdown={startMove} onkeydown={(event) => { const d = event.shiftKey ? 20 : 5; if (event.key === 'ArrowLeft') x = Math.max(0, x - d); else if (event.key === 'ArrowRight') x = Math.min(window.innerWidth - 80, x + d); else if (event.key === 'ArrowUp') y = Math.max(0, y - d); else if (event.key === 'ArrowDown') y = Math.min(window.innerHeight - 40, y + d); else return; event.preventDefault(); }} aria-label={m.wpi_editor_move_panel()} title={m.wpi_editor_move_panel()}>⋮⋮</button>
      <button type="button" class="control" onclick={onDock} aria-label={m.wpi_editor_dock_panel()} title={m.wpi_editor_dock_panel()}>↪</button>
      <button type="button" class="control" onclick={onClose} aria-label={m.wpi_view_json_close()} title={m.wpi_view_json_close()}>✕</button>
    </div>
    <div id="wpi-component-tree-title" class="tree-heading">{m.wpi_toolbar_component_tree()}</div>
    <div class="tree-content">{@render branch(tree, 0)}</div>
    <button type="button" onpointerdown={startResize} onkeydown={(event) => { const d = event.shiftKey ? 20 : 5; if (event.key === 'ArrowLeft') width = Math.max(240, width - d); else if (event.key === 'ArrowRight') width += d; else if (event.key === 'ArrowUp') height = Math.max(160, height - d); else if (event.key === 'ArrowDown') height += d; else return; event.preventDefault(); }} class="resize" aria-label={m.wpi_editor_resize_floating()} title={m.wpi_editor_resize_floating()}></button>
  </div>
{/if}

<style>
  .tree-window { position: fixed; z-index: 50; max-width: calc(100vw - 16px); display:flex; flex-direction:column; background:var(--cv-panel); border:1px solid var(--cv-border); box-shadow:0 8px 24px var(--cv-shadow); }
  .docked-tree { height:100%; min-height:0; display:flex; flex-direction:column; }
  .controls { display:flex; height:30px; flex-shrink:0; border-bottom:1px solid var(--cv-divider); }
  .handle, .control { border:0; background:transparent; color:var(--cv-text-2); padding:0 10px; cursor:pointer; }
  .handle { flex:1; text-align:left; cursor:grab; touch-action:none; }
  .control:hover, .handle:hover { color:var(--cv-text); background:var(--cv-raised); }
  .tree-heading { display:flex; justify-content:space-between; align-items:center; padding:10px; font-size:11px; font-weight:600; text-transform:uppercase; color:var(--cv-accent); border-bottom:1px solid var(--cv-divider); }
  .tree-content { flex:1; min-height:0; overflow:auto; padding:8px 10px; font:12px/1.5 ui-monospace, SFMono-Regular, Menlo, monospace; }
  .resize { position:absolute; right:0; bottom:0; width:18px; height:18px; border:0; background:linear-gradient(135deg, transparent 45%, var(--cv-accent) 46%, var(--cv-accent) 52%, transparent 53%, transparent 64%, var(--cv-accent) 65%, var(--cv-accent) 71%, transparent 72%); cursor:nwse-resize; }

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
  button.node.selected { border-color: var(--cv-accent); background: var(--cv-accent); color: white; }
</style>
