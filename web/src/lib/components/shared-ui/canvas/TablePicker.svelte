<script lang="ts">
 import { m } from '$lib/paraglide/messages.js';
 import { tick } from 'svelte';
 import { requestWithTimeout, RequestTimeoutError } from '$lib/wpi/list-request';
 import { catalogSchemas, catalogTables, type CatalogTable } from './table-catalog';
 let { value, onSelect }: { value: string; onSelect: (value: string) => void } = $props();
 let open = $state(false);
 let databases = $state<string[]>([]);
 let activeDatabase = $state('');
 let activeSchema = $state('');
 let tables = $state<Record<string, CatalogTable[]>>({});
 let error = $state('');
 let loading = $state(false);
 let trigger: HTMLButtonElement;
 let menu = $state<HTMLDivElement>();
 let left = $state(0), top = $state(0);
 let requestId = 0;
 function portal(node: HTMLElement) {
  const styles = getComputedStyle(trigger);
  for (const name of ['--cv-text', '--cv-panel', '--cv-border', '--cv-shadow', '--cv-divider', '--cv-surface-hover']) node.style.setProperty(name, styles.getPropertyValue(name));
  document.body.appendChild(node);
  return { destroy() { node.remove(); } };
 }
 async function loadDatabases() {
  error = ''; loading = true;
  try {
   const body = await requestWithTimeout(async signal => { const res = await fetch('/api/v1/wpi/table-catalog', { credentials: 'same-origin', signal }); if (!res.ok) throw new Error(); return res.json(); });
   databases = body.databases ?? [];
  } catch (e) { error = e instanceof RequestTimeoutError ? m.propertiespanel_tables_timeout() : m.propertiespanel_tables_unavailable(); }
  finally { loading = false; }
 }
 async function chooseDatabase(database: string) {
  activeDatabase = database; activeSchema = ''; error = '';
  const id = ++requestId;
  if (tables[database]) { loading = false; return; }
  loading = true;
  try {
   const body = await requestWithTimeout(async signal => { const res = await fetch(`/api/v1/wpi/table-catalog?database=${encodeURIComponent(database)}`, { credentials: 'same-origin', signal }); if (!res.ok) throw new Error(); return res.json(); });
   tables = { ...tables, [database]: body.tables ?? [] };
  } catch (e) { if (id === requestId) error = e instanceof RequestTimeoutError ? m.propertiespanel_tables_timeout() : m.propertiespanel_tables_unavailable(); }
  finally { if (id === requestId) loading = false; }
 }
 function toggle() {
  open = !open;
  if (!open) return;
  const box = trigger.getBoundingClientRect();
  left = Math.max(4, Math.min(box.left, window.innerWidth - 680));
  top = Math.max(4, Math.min(box.bottom + 4, window.innerHeight - 220));
  activeDatabase = ''; activeSchema = ''; void loadDatabases();
 }
 function outside(event: PointerEvent) { if (open && !trigger.contains(event.target as Node) && !menu?.contains(event.target as Node)) open = false; }
 function keydown(event: KeyboardEvent) {
  if (!open) return;
  if (event.key === 'Escape') { open = false; trigger.focus(); event.preventDefault(); return; }
  if (event.target === trigger && event.key === 'ArrowDown') { menu?.querySelector<HTMLButtonElement>('button')?.focus(); event.preventDefault(); return; }
  if (!menu?.contains(event.target as Node)) return;
  const current = event.target as HTMLElement;
  const column = current.closest('[data-column]');
  if (!column) return;
  if (event.key === 'ArrowDown' || event.key === 'ArrowUp') {
   const buttons = [...column.querySelectorAll<HTMLButtonElement>('button:not(:disabled)')];
   const index = buttons.indexOf(current as HTMLButtonElement);
   buttons[(index + (event.key === 'ArrowDown' ? 1 : -1) + buttons.length) % buttons.length]?.focus();
   event.preventDefault();
  } else if (event.key === 'ArrowRight') {
   const nextColumn = async () => {
    if (column.getAttribute('aria-label') === m.propertiespanel_source_database()) await chooseDatabase(current.dataset.database ?? '');
    else current.click();
    await tick();
    (column.nextElementSibling as HTMLElement | null)?.querySelector<HTMLButtonElement>('button')?.focus();
   };
   void nextColumn();
   event.preventDefault();
  } else if (event.key === 'ArrowLeft') {
   (column.previousElementSibling as HTMLElement | null)?.querySelector<HTMLButtonElement>('[aria-expanded="true"]')?.focus();
   event.preventDefault();
  }
 }
</script>
<svelte:window onpointerdown={outside} onkeydown={keydown} />
<button bind:this={trigger} type="button" class="picker" aria-label={m.propertiespanel_source_table()} aria-haspopup="menu" aria-expanded={open} onclick={toggle}>
 <span>{value || m.propertiespanel_select_table()}</span><span aria-hidden="true">⌄</span>
</button>
{#if open}
 <div use:portal bind:this={menu} class="catalog" style:left="{left}px" style:top="{top}px">
  <div class="column" role="menu" aria-label={m.propertiespanel_source_database()} data-column>
   {#each databases as db}
    <button role="menuitem" type="button" data-database={db} aria-haspopup="menu" aria-expanded={activeDatabase === db} onpointerenter={() => { if (activeDatabase !== db) void chooseDatabase(db); }} onfocus={() => { if (activeDatabase !== db) void chooseDatabase(db); }} onclick={() => { if (activeDatabase !== db) void chooseDatabase(db); }}><span>{db}</span><span aria-hidden="true">›</span></button>
   {/each}
   {#if !loading && !error && databases.length === 0}<div class="message">{m.wpi_no_databases()}</div>{/if}
   {#if loading && !activeDatabase}<div class="message">{m.wpi_panel_loading()}</div>{/if}
   {#if error && !activeDatabase}<button type="button" onclick={loadDatabases}>{error} {m.propertiespanel_tables_retry()}</button>{/if}
  </div>
  {#if activeDatabase}
   <div class="column" role="menu" aria-label={m.propertiespanel_source_schema()} data-column>
    {#if loading}<div class="message">{m.wpi_panel_loading()}</div>{/if}
    {#if error}<button type="button" onclick={() => chooseDatabase(activeDatabase)}>{error} {m.propertiespanel_tables_retry()}</button>{/if}
    {#each catalogSchemas(tables[activeDatabase] ?? []) as schema}
     <button role="menuitem" type="button" aria-haspopup="menu" aria-expanded={activeSchema === schema} onpointerenter={() => activeSchema = schema} onfocus={() => activeSchema = schema} onclick={() => activeSchema = schema}><span>{schema}</span><span aria-hidden="true">›</span></button>
    {/each}
   </div>
  {/if}
  {#if activeDatabase && activeSchema}
   <div class="column" role="menu" aria-label={m.propertiespanel_source_table()} data-column>
    {#each catalogTables(tables[activeDatabase] ?? [], activeSchema) as table}
     <button role="menuitem" type="button" onclick={() => { onSelect(`${activeDatabase}.${activeSchema}.${table}`); open = false; trigger.focus(); }}>{table}</button>
    {/each}
   </div>
  {/if}
 </div>
{/if}
<style>
 .picker { display:flex; justify-content:space-between; align-items:center; width:100%; min-width:0; padding:5px 8px; border:1px solid var(--cv-border); border-radius:4px; color:var(--cv-text); background:var(--cv-surface); text-align:left; cursor:pointer; }
 .picker span:first-child { overflow:hidden; text-overflow:ellipsis; white-space:nowrap; }
 .catalog { position:fixed; z-index:10000; display:flex; max-width:calc(100vw - 8px); color:var(--cv-text); background:var(--cv-panel); border:1px solid var(--cv-border); border-radius:5px; box-shadow:0 8px 24px var(--cv-shadow); }
 .column { width:220px; max-width:calc((100vw - 10px) / 3); max-height:min(420px, 65vh); overflow-y:auto; padding:4px; border-right:1px solid var(--cv-divider); }
 .column:last-child { border:0; }
 .column button { display:flex; justify-content:space-between; gap:8px; width:100%; border:0; border-radius:3px; padding:7px 8px; background:transparent; color:inherit; text-align:left; overflow-wrap:anywhere; cursor:pointer; }
 .column button:hover, .column button:focus-visible, .column button[aria-expanded="true"] { background:var(--cv-surface-hover); outline:0; }
 .message { padding:7px 8px; }
</style>
