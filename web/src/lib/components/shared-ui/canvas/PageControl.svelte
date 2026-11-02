<script lang="ts">
 import { m } from '$lib/paraglide/messages.js';
 import GripIcon from '@lucide/svelte/icons/grip';
 import type { ConfigMode } from './WpiProperties.svelte';
 let { mode, onChoose }: { mode: ConfigMode | null; onChoose: (mode: ConfigMode) => void } = $props();
 let x = $state(0);
 let y = $state(0);
 let drag: { id: number; startX: number; startY: number; x: number; y: number } | null = null;
 function start(event: PointerEvent) {
  if (event.button !== 0) return;
  drag = { id: event.pointerId, startX: event.clientX, startY: event.clientY, x, y };
  (event.currentTarget as HTMLElement).setPointerCapture(event.pointerId);
  event.preventDefault();
 }
 function move(event: PointerEvent) {
  if (!drag || drag.id !== event.pointerId) return;
  x = drag.x + event.clientX - drag.startX;
  y = drag.y + event.clientY - drag.startY;
 }
 function end(event: PointerEvent) { if (drag?.id === event.pointerId) drag = null; }
 function keymove(event: KeyboardEvent) {
  const delta = event.shiftKey ? 20 : 5;
  if (event.key === 'ArrowLeft') x -= delta;
  else if (event.key === 'ArrowRight') x += delta;
  else if (event.key === 'ArrowUp') y -= delta;
  else if (event.key === 'ArrowDown') y += delta;
  else return;
  event.preventDefault();
 }
</script>
<div class="frame" style="transform:translate({x}px, {y}px)" role="group" aria-label={m.wpi_page_controls()}>
 <button type="button" class="handle select-none" aria-label={m.wpi_move_page_controls()} title={m.wpi_move_page_controls()} onpointerdown={start} onpointermove={move} onpointerup={end} onpointercancel={end} onkeydown={keymove}><GripIcon size={14} /></button>
 <button type="button" class:active={mode === 'page'} aria-pressed={mode === 'page'} onclick={() => onChoose('page')}>{m.wpi_view_page_config()}</button>
 <button type="button" class:active={mode === 'panes'} aria-pressed={mode === 'panes'} onclick={() => onChoose('panes')}>{m.wpi_panes_title()}</button>
 <button type="button" class:active={mode === 'panel'} aria-pressed={mode === 'panel'} onclick={() => onChoose('panel')}>{m.wpi_panel_title()}</button>
</div>
<style>
 .frame { display:flex; gap:2px; align-items:center; padding:4px; background:var(--cv-raised); border:1px solid var(--cv-border); border-radius:6px; box-shadow:0 3px 12px var(--cv-shadow); pointer-events:auto; }
 button { border:0; border-radius:4px; padding:6px 8px; background:transparent; color:var(--cv-text-2); white-space:nowrap; font-size:11px; cursor:pointer; }
 button:hover, button.active { background:var(--cv-hover); }
 button.active { color:var(--cv-accent-2); }
 button:focus-visible { outline:2px solid var(--cv-accent); outline-offset:1px; }
 .handle { cursor:grab; touch-action:none; padding:6px; display:flex; }
 .handle:active { cursor:grabbing; }
</style>
