<script lang="ts">
 import { m } from '$lib/paraglide/messages.js';
 import type { JsonObject } from '$lib/wpi/types';
 import { valueText, withAttribute } from './element-attributes';
 let { page, mode, width, onUpdate }: { page: JsonObject; mode: 'page' | 'layouter'; width: number; onUpdate: (page: JsonObject) => void } = $props();
 const pageFields = ['descriptor_id', 'schema_version', 'page_name', 'description', 'access_controls', 'keywords', 'page_tags', 'page_policy', 'page_style', 'width', 'height', 'min_width', 'min_height', 'max_width', 'max_height', 'hori_align', 'vert_align', 'left_margin', 'right_margin', 'top_margin', 'bottom_margin', 'bkgd_color', 'frgd_color', 'binding_mode', 'input', 'save_action', 'refresh'];
 const layoutFields = ['column_gap', 'row_gap', 'horizontal_alignment', 'vertical_alignment', 'rows_alignment'];
 const config = $derived((page.layouter_config && typeof page.layouter_config === 'object' && !Array.isArray(page.layouter_config) ? page.layouter_config : {}) as JsonObject);
 const fields = $derived(mode === 'page' ? [...new Set([...pageFields, ...Object.keys(page).filter(k => !['elements', 'layouter', 'layouter_config'].includes(k))])] : [...new Set([...layoutFields, ...Object.keys(config)])]);
 let editing = $state<{ key: string; text: string } | null>(null);
 function fieldText(key: string): string { return editing?.key === key ? editing.text : valueText((mode === 'page' ? page : config)[key]); }
 function commit(key: string, text: string) {
  const attrs = withAttribute(mode === 'page' ? page : config, key, text);
  onUpdate(mode === 'page' ? attrs : { ...page, layouter_config: attrs });
  editing = null;
 }
</script>
<aside class="panel select-text" style="width:{width}px">
 <div class="heading">{mode === 'page' ? m.wpi_page_config_title() : m.wpi_layouter_title()}</div>
 <div class="fields">
  {#if mode === 'layouter'}
   <label for="wpi-layouter-id">{m.wpi_layouter_type()}</label>
   <input id="wpi-layouter-id" value={String(page.layouter ?? '')} readonly />
  {/if}
  {#each fields as key (key)}
   <label for={`wpi-page-field-${key}`}>{key}</label>
   <textarea id={`wpi-page-field-${key}`} rows="1" value={fieldText(key)} readonly={mode === 'page' && (key === 'descriptor_id' || key === 'schema_version')}
    oninput={(event) => editing = { key, text: event.currentTarget.value }}
    onchange={(event) => commit(key, event.currentTarget.value)}
    onblur={(event) => { if (editing?.key === key) commit(key, event.currentTarget.value); }}></textarea>
  {/each}
 </div>
</aside>
<style>
 .panel { flex-shrink:0; display:flex; flex-direction:column; height:100%; background:var(--cv-panel); border-left:1px solid var(--cv-divider); color:var(--cv-text); }
 .heading { padding:10px 12px; border-bottom:1px solid var(--cv-divider); font-size:11px; font-weight:600; }
 .fields { overflow:auto; padding:10px 12px; }
 label { display:block; font-size:10px; color:var(--cv-text-muted); margin:0 0 4px; }
 textarea, input { display:block; width:100%; box-sizing:border-box; border:1px solid var(--cv-border); background:var(--cv-surface); color:var(--cv-text); border-radius:4px; padding:5px 8px; margin:0 0 12px; font-size:11px; }
 textarea { resize:vertical; field-sizing:content; }
 input:read-only, textarea:read-only { background:transparent; color:var(--cv-text-2); }
</style>
