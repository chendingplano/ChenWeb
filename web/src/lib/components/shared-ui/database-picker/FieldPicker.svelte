<script lang="ts">
 import { m } from '$lib/paraglide/messages.js';
 import { untrack } from 'svelte';
 import { requestWithTimeout, RequestTimeoutError } from '$lib/wpi/list-request';
 import type { Json, JsonObject } from '$lib/wpi/types';
 import { toggleField, renameField, renameColumn } from './field-mappings';
 let { table, fields, onChange }: { table: string; fields: Json[]; onChange: (fields: Json[]) => void } = $props();
 let open = $state(false), loading = $state(false), error = $state('');
 let columns = $state<string[]>([]);
 let trigger: HTMLButtonElement;
 let popup = $state<HTMLDivElement>();
 let left = $state(0), top = $state(0), requestId = 0;
 let width = $state(410), height = $state(320);
 let drag: { id: number; x: number; y: number; width: number; height: number } | null = null;
 let drafts = $state<Record<string, string>>({});
 const selected = $derived(fields.filter((field): field is JsonObject => !!field && typeof field === 'object' && !Array.isArray(field) && typeof field.column === 'string'));
 const allColumns = $derived([...columns, ...selected.map(field => String(field.column)).filter(column => !columns.some(original => (drafts[original] ?? original) === column))]);
 function resizeTo(w: number, h: number) {
  width = Math.min(Math.max(280, w), Math.max(280, window.innerWidth - left - 4));
  height = Math.min(Math.max(160, h), Math.max(160, window.innerHeight - top - 4));
 }
 function startResize(event: PointerEvent) {
  if (event.button !== 0) return;
  drag = { id: event.pointerId, x: event.clientX, y: event.clientY, width, height };
  (event.currentTarget as HTMLElement).setPointerCapture(event.pointerId);
  event.preventDefault();
 }
 function resize(event: PointerEvent) {
  if (drag?.id === event.pointerId) resizeTo(drag.width + event.clientX - drag.x, drag.height + event.clientY - drag.y);
 }
 function resizeKey(event: KeyboardEvent) {
  const step = event.shiftKey ? 50 : 10;
  if (event.key === 'ArrowRight') resizeTo(width + step, height);
  else if (event.key === 'ArrowLeft') resizeTo(width - step, height);
  else if (event.key === 'ArrowDown') resizeTo(width, height + step);
  else if (event.key === 'ArrowUp') resizeTo(width, height - step);
  else return;
  event.preventDefault();
 }
 function editColumn(original: string, old: string, next: string) {
  const name = next.trim();
  if (!name || (name !== old && selected.some(field => field.column === name))) {
   drafts = { ...drafts, [original]: old };
   return;
  }
  if (selected.some(field => field.column === old)) onChange(renameColumn(fields, old, name));
  drafts = { ...drafts, [original]: name };
 }
 function portal(node: HTMLElement) {
  const styles = getComputedStyle(trigger);
  for (const name of ['--cv-text', '--cv-panel', '--cv-border', '--cv-shadow', '--cv-divider', '--cv-surface', '--cv-surface-hover', '--cv-accent']) node.style.setProperty(name, styles.getPropertyValue(name));
  document.body.appendChild(node);
  return { destroy() { node.remove(); } };
 }
 async function load() {
  const id = ++requestId;
  const requestedTable = table;
  error = ''; loading = true; columns = [];
  try {
   const body = await requestWithTimeout(async signal => {
    const response = await fetch(`/api/v1/wpi/table-columns?table=${encodeURIComponent(requestedTable)}`, { credentials: 'same-origin', signal });
    if (!response.ok) throw new Error();
    return response.json();
   });
   if (id === requestId && table === requestedTable) columns = body.columns ?? [];
  } catch (err) {
   if (id === requestId) error = err instanceof RequestTimeoutError ? m.propertiespanel_fields_timeout() : m.propertiespanel_fields_unavailable();
  } finally { if (id === requestId) loading = false; }
 }
 function toggle() {
  open = !open;
  if (!open) { requestId++; return; }
  const rect = trigger.getBoundingClientRect();
  drafts = {};
  left = Math.max(4, Math.min(rect.left, window.innerWidth - 420));
  top = Math.max(4, Math.min(rect.bottom + 4, window.innerHeight - 340));
  resizeTo(410, 320);
  void load();
 }
 function outside(event: PointerEvent) { if (open && !trigger.contains(event.target as Node) && !popup?.contains(event.target as Node)) { open = false; requestId++; } }
 function keyboard(event: KeyboardEvent) { if (open && event.key === 'Escape') { open = false; requestId++; trigger.focus(); event.preventDefault(); } }
 $effect(() => { table; untrack(() => { if (open) { open = false; requestId++; } }); });
</script>
<svelte:window onpointerdown={outside} onkeydown={keyboard} />
<button type="button" class="picker" bind:this={trigger} disabled={!table} aria-haspopup="dialog" aria-expanded={open} onclick={toggle}>{m.propertiespanel_pick_fields()}</button>
{#if open}
 <div use:portal bind:this={popup} class="popup" role="dialog" aria-label={m.propertiespanel_pick_fields()} style:left="{left}px" style:top="{top}px" style:width="{width}px" style:height="{height}px">
  {#if loading}<div class="message" role="status">{m.wpi_panel_loading()}</div>
  {:else if error}<div class="message" role="alert">{error} <button type="button" onclick={load}>{m.propertiespanel_tables_retry()}</button></div>
  {:else}
   <div class="grid heading"><span>{m.propertiespanel_select_field()}</span><span>{m.propertiespanel_source_column()}</span><span>{m.propertiespanel_source_attribute()}</span></div>
   <div class="rows">
    {#each allColumns as column (column)}
     {@const actual = drafts[column] ?? column}
     {@const field = selected.find(item => item.column === actual)}
     <div class="grid row">
      <input type="checkbox" aria-label={m.propertiespanel_select_column({ column: actual })} checked={!!field} onchange={(e) => onChange(toggleField(fields, actual, e.currentTarget.checked))} />
      <div class="column-cell"><input class="attribute" type="text" aria-label={m.propertiespanel_column_for_field({ column })} value={actual} onchange={(e) => editColumn(column, actual, e.currentTarget.value)} />{#if !columns.includes(actual)}<small>{m.propertiespanel_field_missing()}</small>{/if}</div>
      <input class="attribute" type="text" aria-label={m.propertiespanel_attribute_for_column({ column: actual })} value={String(field?.attribute ?? actual)} disabled={!field} oninput={(e) => onChange(renameField(fields, actual, e.currentTarget.value))} />
     </div>
    {/each}
   </div>
  {/if}
  <button type="button" class="resize-handle" aria-label={m.propertiespanel_resize_fields()} title={m.propertiespanel_resize_fields()} onpointerdown={startResize} onpointermove={resize} onpointerup={(event) => { if (drag?.id === event.pointerId) drag = null; }} onpointercancel={() => drag = null} onkeydown={resizeKey}></button>
 </div>
{/if}
<style>
 .picker { width:100%; text-align:left; padding:5px 8px; border:1px solid var(--cv-border); border-radius:4px; color:var(--cv-text); background:var(--cv-surface); cursor:pointer; }
 .picker:disabled { opacity:.5; cursor:not-allowed; }
 .popup { position:fixed; display:flex; flex-direction:column; box-sizing:border-box; max-width:calc(100vw - 8px); z-index:10000; color:var(--cv-text); background:var(--cv-panel); border:1px solid var(--cv-border); border-radius:5px; box-shadow:0 8px 24px var(--cv-shadow); font-size:12px; }
 .grid { display:grid; grid-template-columns:55px minmax(0,1fr) minmax(0,1fr); gap:8px; align-items:center; padding:6px 10px; }
 .heading { border-bottom:1px solid var(--cv-divider); font-weight:600; }
 .rows { flex:1; min-height:0; overflow:auto; padding-bottom:14px; }
 .column-cell { min-width:0; overflow-wrap:anywhere; }
 .row input[type=checkbox] { justify-self:center; accent-color:var(--cv-accent); }
 .attribute { min-width:0; width:100%; padding:4px; color:inherit; background:var(--cv-surface); border:1px solid var(--cv-border); border-radius:3px; }
 .attribute:disabled { opacity:.55; }
 .message { padding:12px; }
 .resize-handle { position:absolute; right:0; bottom:0; width:18px; height:18px; border:0; cursor:nwse-resize; touch-action:none; background:linear-gradient(135deg, transparent 45%, var(--cv-accent) 46%, var(--cv-accent) 52%, transparent 53%, transparent 64%, var(--cv-accent) 65%, var(--cv-accent) 71%, transparent 72%); }
 .resize-handle:focus-visible { outline:2px solid var(--cv-accent); }
</style>
