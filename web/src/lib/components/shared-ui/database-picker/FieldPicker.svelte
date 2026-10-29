<script lang="ts">
 import { m } from '$lib/paraglide/messages.js';
 import { untrack } from 'svelte';
 import { requestWithTimeout, RequestTimeoutError } from '$lib/wpi/list-request';
 import type { Json, JsonObject } from '$lib/wpi/types';
 import { toggleField, renameField } from './field-mappings';
 let { table, fields, onChange }: { table: string; fields: Json[]; onChange: (fields: Json[]) => void } = $props();
 let open = $state(false), loading = $state(false), error = $state('');
 let columns = $state<string[]>([]);
 let trigger: HTMLButtonElement;
 let popup = $state<HTMLDivElement>();
 let left = $state(0), top = $state(0), requestId = 0;
 const selected = $derived(fields.filter((field): field is JsonObject => !!field && typeof field === 'object' && !Array.isArray(field) && typeof field.column === 'string'));
 const allColumns = $derived([...columns, ...selected.map(field => String(field.column)).filter(column => !columns.includes(column))]);
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
  left = Math.max(4, Math.min(rect.left, window.innerWidth - 420));
  top = Math.max(4, Math.min(rect.bottom + 4, window.innerHeight - 240));
  void load();
 }
 function outside(event: PointerEvent) { if (open && !trigger.contains(event.target as Node) && !popup?.contains(event.target as Node)) { open = false; requestId++; } }
 function keyboard(event: KeyboardEvent) { if (open && event.key === 'Escape') { open = false; requestId++; trigger.focus(); event.preventDefault(); } }
 $effect(() => { table; untrack(() => { if (open) { open = false; requestId++; } }); });
</script>
<svelte:window onpointerdown={outside} onkeydown={keyboard} />
<button type="button" class="picker" bind:this={trigger} disabled={!table} aria-haspopup="dialog" aria-expanded={open} onclick={toggle}>{m.propertiespanel_pick_fields()}</button>
{#if open}
 <div use:portal bind:this={popup} class="popup" role="dialog" aria-label={m.propertiespanel_pick_fields()} style:left="{left}px" style:top="{top}px">
  {#if loading}<div class="message" role="status">{m.wpi_panel_loading()}</div>
  {:else if error}<div class="message" role="alert">{error} <button type="button" onclick={load}>{m.propertiespanel_tables_retry()}</button></div>
  {:else}
   <div class="grid heading"><span>{m.propertiespanel_select_field()}</span><span>{m.propertiespanel_source_column()}</span><span>{m.propertiespanel_source_attribute()}</span></div>
   <div class="rows">
    {#each allColumns as column (column)}
     {@const field = selected.find(item => item.column === column)}
     <div class="grid row">
      <input type="checkbox" aria-label={m.propertiespanel_select_column({ column })} checked={!!field} onchange={(e) => onChange(toggleField(fields, column, e.currentTarget.checked))} />
      <span title={column}>{column}{#if !columns.includes(column)} <small>({m.propertiespanel_field_missing()})</small>{/if}</span>
      <input class="attribute" type="text" aria-label={m.propertiespanel_attribute_for_column({ column })} value={String(field?.attribute ?? column)} disabled={!field} oninput={(e) => onChange(renameField(fields, column, e.currentTarget.value))} />
     </div>
    {/each}
   </div>
  {/if}
 </div>
{/if}
<style>
 .picker { width:100%; text-align:left; padding:5px 8px; border:1px solid var(--cv-border); border-radius:4px; color:var(--cv-text); background:var(--cv-surface); cursor:pointer; }
 .picker:disabled { opacity:.5; cursor:not-allowed; }
 .popup { position:fixed; z-index:10000; width:min(410px, calc(100vw - 8px)); color:var(--cv-text); background:var(--cv-panel); border:1px solid var(--cv-border); border-radius:5px; box-shadow:0 8px 24px var(--cv-shadow); font-size:12px; }
 .grid { display:grid; grid-template-columns:55px minmax(0,1fr) minmax(0,1fr); gap:8px; align-items:center; padding:6px 10px; }
 .heading { border-bottom:1px solid var(--cv-divider); font-weight:600; }
 .rows { max-height:min(400px, 60vh); overflow:auto; }
 .row span { overflow-wrap:anywhere; }
 .row input[type=checkbox] { justify-self:center; accent-color:var(--cv-accent); }
 .attribute { min-width:0; width:100%; padding:4px; color:inherit; background:var(--cv-surface); border:1px solid var(--cv-border); border-radius:3px; }
 .attribute:disabled { opacity:.55; }
 .message { padding:12px; }
</style>
