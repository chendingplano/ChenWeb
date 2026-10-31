<script lang="ts" module>
 // Page: the page's attributes. Panes: its page layouter's tree and the selected
 // pane. Panel: the element layouter of the selected leaf pane's panel
 // (OpenSpec wpi-pages-own-panes, design D12).
 export type ConfigMode = 'page' | 'panes' | 'panel';
</script>

<script lang="ts">
 import { m } from '$lib/paraglide/messages.js';
 import { findPane, isLeaf } from '$lib/wpi/layout';
 import type { JsonObject, PaneSpec } from '$lib/wpi/types';
 import { valueText, withAttribute } from './element-attributes';
 import { LEAF_PANE_ATTRS, PANE_ATTRS, renameProblem, rootOf, setPaneAttr, setPanelConfig } from './pane-edit';
 import { flowFields, flowOptions, flowValue, setFlowValue, validFlowValue, type FlowField } from './flow-config';

 let { page, mode, width, selectedPane, pageLayouters, onUpdate, onLayouterChange, onSelectPane, onSplit, onAdd, onDelete, onRename, onMerge }: {
  page: JsonObject; pageLayouters: JsonObject[]; mode: ConfigMode; width: number; selectedPane: string | null;
  onUpdate: (page: JsonObject) => void; onLayouterChange: (cls: JsonObject) => void; onSelectPane: (pane: string) => void;
  onSplit: (pane: string, split: 'rows' | 'columns') => void; onAdd: (pane: string) => void;
  onDelete: (pane: string) => void; onMerge: (pane: string) => void; onRename: (pane: string, to: string) => void;
 } = $props();

 const pageGroups = [
  { title: () => m.wpi_page_group_basic(), fields: ['descriptor_id', 'schema_version', 'page_name', 'description', 'keywords', 'page_tags'] },
  { title: () => m.wpi_page_group_geometry(), fields: ['width', 'height', 'min_width', 'min_height', 'max_width', 'max_height', 'hori_align', 'vert_align', 'hori_scrollbar', 'vert_scrollbar'] },
  { title: () => m.wpi_page_group_colors(), fields: ['bkgd_color', 'frgd_color'] },
  { title: () => m.wpi_page_group_actions(), fields: ['save_action', 'refresh'] },
  { title: () => m.wpi_page_group_misc(), fields: ['access_controls', 'page_policy', 'page_style', 'binding_mode', 'input'] },
 ];
 const pageFields = pageGroups.flatMap((group) => group.fields);
 // Fields are labelled with their attribute names, as in the descriptor.
 const PANE_ID = 'pane_id';

 const root = $derived(rootOf(page));
 const pane = $derived(selectedPane ? findPane(root, selectedPane) : undefined);
 const panel = $derived(pane && isLeaf(pane) ? ((page.panels as JsonObject | undefined)?.[pane.pane_id] as JsonObject | undefined) : undefined);
 const layouter = $derived((panel?.elem_layouter ?? { layouter: 'flow-layouter' }) as JsonObject);
 const copyConfig = $derived((((layouter.class as JsonObject | undefined)?.config) ?? {}) as JsonObject);
 const ownConfig = $derived(layouter.config as JsonObject | undefined);
 const paneFields = $derived(pane ? (isLeaf(pane) ? [...PANE_ATTRS, ...LEAF_PANE_ATTRS] : PANE_ATTRS) : []);
 const extraPageFields = $derived(Object.keys(page).filter((key) => !['page_layouter', 'panels', ...pageFields].includes(key)));
 const fields = $derived(mode === 'panes' ? paneFields : flowFields);

 let editing = $state<{ key: string; text: string } | null>(null);
 let renaming = $state<string | null>(null);
 const renameError = $derived(pane && renaming !== null ? renameProblem(page, pane.pane_id, renaming) : null);

 function source(): JsonObject {
  if (mode === 'page') return page;
  if (mode === 'panes') return (pane ?? {}) as unknown as JsonObject;
  return ownConfig ?? copyConfig;
 }
 function fieldText(key: string): string { return editing?.key === key ? editing.text : valueText(source()[key]); }
 function commit(key: string, text: string) {
  editing = null;
  const next = withAttribute(source(), key, text);
  if (mode === 'page') onUpdate(next);
  else if (mode === 'panes' && pane) onUpdate(setPaneAttr(page, pane.pane_id, key, next[key]));
  else if (mode === 'panel' && pane) onUpdate(setPanelConfig(page, pane.pane_id, next));
 }
 function commitRename() {
  if (pane && renaming !== null && !renameError && renaming !== pane.pane_id) onRename(pane.pane_id, renaming);
  renaming = null;
 }
 function flowLabel(key: FlowField): string {
  const labels = {
   column_gap: m.wpi_flow_column_gap, row_gap: m.wpi_flow_row_gap,
   horizontal_alignment: m.wpi_flow_horizontal_alignment, vertical_alignment: m.wpi_flow_vertical_alignment,
   rows_alignment: m.wpi_flow_rows_alignment, min_row_height: m.wpi_flow_min_row_height,
   max_row_height: m.wpi_flow_max_row_height, min_elem_width: m.wpi_flow_min_elem_width,
   max_elem_width: m.wpi_flow_max_elem_width, max_num_elems: m.wpi_flow_max_num_elems,
   row_separator: m.wpi_flow_row_separator, elem_separator: m.wpi_flow_elem_separator
  };
  return labels[key]();
 }
 function optionLabel(value: string): string {
  const labels: Record<string, () => string> = {
   start: m.wpi_flow_start, center: m.wpi_flow_center, end: m.wpi_flow_end,
   top: m.wpi_flow_top, bottom: m.wpi_flow_bottom, stretch: m.wpi_flow_stretch,
   'space-between': m.wpi_flow_space_between, none: m.wpi_flow_none,
   thin_solid_line: m.wpi_flow_thin_solid, norm_solid_line: m.wpi_flow_normal_solid,
   bold_solid_line: m.wpi_flow_bold_solid, thin_dash_line: m.wpi_flow_thin_dashed,
   norm_dash_line: m.wpi_flow_normal_dashed, bold_dash_line: m.wpi_flow_bold_dashed
  };
  return labels[value]?.() ?? value;
 }
 function flowText(key: FlowField): string {
  return editing?.key === key ? editing.text : String(flowValue(key, ownConfig, copyConfig));
 }
 function commitFlow(key: FlowField, text: string) {
  if (!pane || !validFlowValue(key, text)) { editing = { key, text }; return; }
  editing = null;
  onUpdate(setPanelConfig(page, pane.pane_id, setFlowValue(ownConfig ?? copyConfig, key, text)));
 }
 // The tree as a list, each pane with its depth.
 function flat(p: PaneSpec, depth = 0): { pane: PaneSpec; depth: number }[] {
  return [{ pane: p, depth }, ...(p.children ?? []).flatMap((c) => flat(c, depth + 1))];
 }
</script>

{#snippet pageLayouterSelector()}
   <label for="wpi-page-layouter-id">{m.wpi_page_layouter_type()}</label>
   <select id="wpi-page-layouter-id" value={String((page.page_layouter as JsonObject | undefined)?.layouter ?? '')} onchange={(event) => {
    const cls = pageLayouters.find((item) => item.descriptor_id === event.currentTarget.value);
    if (cls) onLayouterChange(cls);
   }}>
    {#if !pageLayouters.some((item) => item.descriptor_id === (page.page_layouter as JsonObject | undefined)?.layouter)}
     <option value={String((page.page_layouter as JsonObject | undefined)?.layouter ?? '')}>{String((page.page_layouter as JsonObject | undefined)?.layouter ?? '')}</option>
    {/if}
    {#each pageLayouters as cls (String(cls.descriptor_id))}<option value={String(cls.descriptor_id)}>{String(cls.descriptor_id)}</option>{/each}
   </select>
{/snippet}

{#snippet flowField(key: FlowField)}
 <label for={`wpi-flow-${key}`}>{flowLabel(key)}</label>
 {#if flowOptions[key]}
  <select id={`wpi-flow-${key}`} value={flowText(key)} onchange={(event) => commitFlow(key, event.currentTarget.value)}>
   {#if !flowOptions[key]?.includes(flowText(key))}<option value={flowText(key)}>{flowText(key)}</option>{/if}
   {#each flowOptions[key] ?? [] as option}<option value={option}>{optionLabel(option)}</option>{/each}
  </select>
 {:else}
  <input id={`wpi-flow-${key}`} type="text" inputmode={key === 'max_num_elems' ? 'numeric' : 'text'} value={flowText(key)}
   aria-invalid={editing?.key === key && !validFlowValue(key, editing.text)}
   aria-describedby={editing?.key === key && !validFlowValue(key, editing.text) ? `wpi-flow-${key}-error` : key === 'max_num_elems' ? 'wpi-flow-count-hint' : undefined}
   oninput={(event) => editing = { key, text: event.currentTarget.value }}
   onchange={(event) => commitFlow(key, event.currentTarget.value)}
   onblur={(event) => { if (editing?.key === key) commitFlow(key, event.currentTarget.value); }} />
  {#if editing?.key === key && !validFlowValue(key, editing.text)}
   <p class="error" id={`wpi-flow-${key}-error`} role="alert">{key === 'max_num_elems' ? m.wpi_flow_invalid_count() : m.wpi_flow_invalid_length()}</p>
  {:else if key === 'max_num_elems'}
   <p class="field-hint" id="wpi-flow-count-hint">{m.wpi_flow_unlimited_hint()}</p>
  {/if}
 {/if}
{/snippet}

{#snippet field(key: string)}
 <label for={`wpi-page-field-${key}`}>{key}</label>
 <textarea id={`wpi-page-field-${key}`} rows="1" value={fieldText(key)} readonly={mode === 'page' && (key === 'descriptor_id' || key === 'schema_version')}
  placeholder={mode === 'panel' && !ownConfig ? valueText(copyConfig[key]) : undefined}
  oninput={(event) => editing = { key, text: event.currentTarget.value }}
  onchange={(event) => commit(key, event.currentTarget.value)}
  onblur={(event) => { if (editing?.key === key) commit(key, event.currentTarget.value); }}></textarea>
{/snippet}

<aside class="panel select-text" style="width:{width}px">
 <div class="heading">{mode === 'page' ? m.wpi_page_config_title() : mode === 'panes' ? m.wpi_panes_title() : m.wpi_panel_title()}</div>
 <div class="fields">
  {#if mode === 'panes'}
   {@render pageLayouterSelector()}
  {/if}
  {#if mode === 'panes'}
   <div class="label">{m.wpi_pane_tree()}</div>
   <ul class="tree" aria-label={m.wpi_pane_tree()}>
    {#each flat(root) as item (item.pane.pane_id)}
     <li><button type="button" class:active={item.pane.pane_id === selectedPane} aria-pressed={item.pane.pane_id === selectedPane} style="padding-left:{6 + item.depth * 12}px" onclick={() => onSelectPane(item.pane.pane_id)}>{item.depth === 0 ? m.wpi_pane_root() : item.pane.pane_id}{#if item.depth !== 0 && item.pane.split}<span class="split"> · {item.pane.split}</span>{/if}</button></li>
    {/each}
   </ul>
   {#if pane}
    <div class="actions">
     {#if isLeaf(pane)}
      <button type="button" onclick={() => onSplit(pane.pane_id, 'rows')}>{m.wpi_pane_split_rows()}</button>
      <button type="button" onclick={() => onSplit(pane.pane_id, 'columns')}>{m.wpi_pane_split_columns()}</button>
     {:else}
      <button type="button" onclick={() => onAdd(pane.pane_id)}>{m.wpi_pane_add()}</button>
      <button type="button" onclick={() => onMerge(pane.pane_id)}>{m.wpi_pane_merge()}</button>
     {/if}
     {#if pane.pane_id !== root.pane_id}<button type="button" class="danger" onclick={() => onDelete(pane.pane_id)}>{m.wpi_pane_delete()}</button>{/if}
    </div>
    <label for="wpi-pane-id">{PANE_ID}</label>
    <input id="wpi-pane-id" value={renaming ?? pane.pane_id} aria-invalid={!!renameError}
     oninput={(event) => renaming = event.currentTarget.value}
     onchange={commitRename} onblur={commitRename} />
    {#if renameError}<p class="error" role="alert">{renameError === 'taken' ? m.wpi_pane_id_taken({ pane: renaming ?? '' }) : m.wpi_pane_id_invalid()}</p>{/if}
   {:else}
    <p class="hint">{m.wpi_pane_select_hint()}</p>
   {/if}
  {:else if mode === 'panel'}
   {#if !pane || !isLeaf(pane)}
    <p class="hint">{m.wpi_panel_select_leaf()}</p>
   {:else}
    <label for="wpi-layouter-id">{m.wpi_layouter_type()}</label>
    <input id="wpi-layouter-id" value={String(layouter.layouter ?? '')} readonly />
   {/if}
  {/if}
  {#if mode === 'page'}
   {#each pageGroups as group, index}
    <details class="group" open={index === 0}>
     <summary>{group.title()}</summary>
     {#if index === 1}{@render pageLayouterSelector()}{/if}
     {#each index === 4 ? [...group.fields, ...extraPageFields] : group.fields as key (key)}
      {@render field(key)}
     {/each}
    </details>
   {/each}
  {:else if pane && mode === 'panel' && isLeaf(pane)}
   {#each flowFields as key (key)}{@render flowField(key)}{/each}
  {:else if pane && mode === 'panes'}
   {#each fields as key (key)}{@render field(key)}{/each}
  {/if}
 </div>
</aside>
<style>
 .panel { flex-shrink:0; display:flex; flex-direction:column; height:100%; background:var(--cv-panel); border-left:1px solid var(--cv-divider); color:var(--cv-text); user-select:text; }
 .heading { padding:10px 12px; border-bottom:1px solid var(--cv-divider); font-size:11px; font-weight:600; }
 .fields { overflow:auto; padding:10px 12px; }
 .group + .group { margin-top:6px; }
 .group summary { padding:10px 8px; margin-bottom:8px; border:1px solid var(--cv-border); border-radius:4px; background:var(--cv-group-header); font-size:11px; font-weight:600; color:#101827; cursor:pointer; }
 .group summary:hover { background:color-mix(in srgb, var(--cv-group-header) 85%, white); }
 label, .label { display:block; font-size:10px; color:var(--cv-text-muted); margin:0 0 4px; }
 textarea, input, select { display:block; width:100%; box-sizing:border-box; border:1px solid var(--cv-border); background:var(--cv-surface); color:var(--cv-text); border-radius:4px; padding:5px 8px; margin:0 0 12px; font-size:11px; }
 textarea { resize:vertical; field-sizing:content; }
 input:read-only, textarea:read-only { background:transparent; color:var(--cv-text-2); }
 .tree { list-style:none; margin:0 0 10px; padding:0; border:1px solid var(--cv-border); border-radius:4px; }
 .tree button { display:block; width:100%; text-align:left; border:0; background:transparent; color:var(--cv-text-2); font-size:11px; padding:4px 6px; cursor:pointer; }
 .tree button:hover, .tree button.active { background:var(--cv-hover); color:var(--cv-text); }
 .split { color:var(--cv-text-muted); }
 .actions { display:flex; flex-wrap:wrap; gap:4px; margin:0 0 12px; }
 .actions button { border:1px solid var(--cv-border); border-radius:4px; background:var(--cv-surface); color:var(--cv-text-2); font-size:10px; padding:4px 8px; cursor:pointer; }
 .actions button:hover { background:var(--cv-hover); }
 .actions .danger { color:#dc2626; }
 .hint { font-size:11px; color:var(--cv-text-muted); margin:0 0 12px; }
 .error { font-size:10px; color:#dc2626; margin:-8px 0 12px; }
 .field-hint { font-size:10px; color:var(--cv-text-muted); margin:-8px 0 12px; }
</style>
