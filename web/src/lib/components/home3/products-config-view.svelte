<script lang="ts">
 import { onMount, tick } from 'svelte';
 import { m } from '$lib/paraglide/messages.js';
 let { darkMode = true }: { darkMode?: boolean } = $props();
 let values = $state<Record<string, string>>({});
 let loading = $state(true), loaded = $state(false), saving = $state(false), error = $state(false), saved = $state(false);
 let viewport: HTMLDivElement;
 const draftKey = 'products-config-draft', scrollKey = 'products-config-scroll';
 let groups = $derived([
  { title: m.products_config_workflow(), fields: [
   {key:'EXTRACT_PRODUCTS_MERGED_PASS', label:m.products_config_merged(), type:'bool'},
   {key:'EXTRACT_PRODUCT_PASS_1_ONLY', label:m.products_config_pass1(), type:'bool'},
   {key:'EXTRACT_PRODUCTS_REASONING', label:m.products_config_reasoning(), type:'bool'},
   {key:'EXTRACT_PRODUCTS_MAX_TASKS', label:m.products_config_tasks(), type:'number'} ] },
  { title: m.products_config_models(), fields: [
   {key:'EXTRACT_PRODUCT_MENTIONS_MODEL_NAME', label:m.products_config_mention_model(), type:'text'},
   {key:'ENRICH_PRODUCT_RELATIONS_MODEL_NAME', label:m.products_config_relation_model(), type:'text'},
   {key:'EXTRACT_PRODUCT_MODEL_NAME', label:m.products_config_common_model(), type:'text'},
   {key:'EXTRACT_PRODUCT_MODEL_FALLBACK', label:m.products_config_retry_model(), type:'text'} ] },
  { title: m.products_config_prompts(), fields: [
   {key:'EXTRACT_PRODUCT_MENTIONS_PROMPT', label:m.products_config_mention_prompt(), type:'text'},
   {key:'ENRICH_PRODUCT_MENTION_PROMPT', label:m.products_config_relation_prompt(), type:'text'},
   {key:'EXTRACT_PRODUCT_MERGED_PROMPT', label:m.products_config_merged_prompt(), type:'text'} ] },
  { title: m.products_config_translation(), fields: [
   {key:'EXTRACT_PRODUCT_TRANSLATION', label:m.products_config_translation_enabled(), type:'translation'},
   {key:'TRANSLATION_MODEL_NAME', label:m.products_config_translation_model(), type:'text'},
   {key:'TRANSLATE_PRODUCTS_PROMPT', label:m.products_config_translation_prompt(), type:'text'},
   {key:'TRANSLATE_PRODUCTS_BATCH_SIZE', label:m.products_config_batch(), type:'number'},
   {key:'TRANSLATE_PRODUCTS_THINKING', label:m.products_config_thinking(), type:'text'} ] }
 ]);
 function remember() { saved = false; try { sessionStorage.setItem(draftKey, JSON.stringify(values)); } catch { /* optional storage */ } }
 onMount(() => { void (async () => { try {
  const res = await fetch('/api/v1/kb/products-config', {credentials:'same-origin'}); if (!res.ok) throw new Error();
  values = (await res.json()).config; loaded = true;
  try { const draft = JSON.parse(sessionStorage.getItem(draftKey) ?? 'null'); if (draft && typeof draft === 'object' && !Array.isArray(draft)) values = draft; } catch { /* invalid draft */ }
 } catch { error = true; } finally { loading = false; await tick(); try { viewport.scrollTop = Number(sessionStorage.getItem(scrollKey) ?? 0); } catch { /* optional storage */ } } })(); });
 async function save() { saving = true; error = false; try {
  const res = await fetch('/api/v1/kb/products-config', {method:'PUT',credentials:'same-origin',headers:{'Content-Type':'application/json'},body:JSON.stringify(values)}); if (!res.ok) throw new Error();
  try { sessionStorage.removeItem(draftKey); } catch { /* optional storage */ } saved = true;
 } catch { error = true; } finally { saving = false; } }
</script>
<div bind:this={viewport} class:light={!darkMode} class="page select-text" onscroll={() => { try { sessionStorage.setItem(scrollKey, String(viewport.scrollTop)); } catch { /* optional storage */ } }}>
 <header><h1>{m.products_config_title()}</h1><p>{m.products_config_intro()}</p><p>{m.products_config_restart()}</p></header>
 {#if error}<p class="notice error" role="alert">{m.products_config_error()}</p>{/if}
 {#if saved}<p class="notice" role="status">{m.products_config_saved()}</p>{/if}
 {#if !loading && loaded}
 <form onsubmit={(event) => { event.preventDefault(); void save(); }}>
 {#each groups as group}<section><h2>{group.title}</h2><div class="fields">
  {#each group.fields as field}<label><span>{field.label}</span><code>{field.key}</code>
   {#if field.type === 'bool' || field.type === 'translation'}<select value={values[field.key] ?? ''} onchange={(e) => { values[field.key] = e.currentTarget.value; remember(); }}><option value="">{m.products_config_inherit()}</option><option value={field.type === 'bool' ? 'true' : 'on'}>{m.products_config_on()}</option><option value={field.type === 'bool' ? 'false' : 'off'}>{m.products_config_off()}</option></select>
   {:else}<input type={field.type} min={field.type === 'number' ? 1 : undefined} step={field.type === 'number' ? 1 : undefined} value={values[field.key] ?? ''} oninput={(e) => { values[field.key] = e.currentTarget.value; remember(); }} placeholder={m.products_config_inherit()} />{/if}
  </label>{/each}
 </div>{#if group.title === m.products_config_workflow()}<p class="hint">{m.products_config_testing_hint()}</p>{/if}</section>{/each}
 <footer><button disabled={saving}>{saving ? m.pipeline_config_saving() : m.pipeline_config_save()}</button></footer></form>
 {/if}
</div>
<style>
 .page{--bg:#1f2333;--border:#343b52;--text:#e2e8f0;--muted:#9baac1;color:var(--text);max-width:1050px;max-height:calc(100vh - 100px);overflow:auto;padding:32px}.page.light{--bg:#fff;--border:#dce1e8;--text:#172033;--muted:#59677b}h1{font-size:26px;margin:0 0 8px}h2{font-size:17px;margin:0 0 16px}p{color:var(--muted);line-height:1.5;margin:5px 0}header{margin-bottom:24px}section{background:var(--bg);border:1px solid var(--border);border-radius:12px;padding:22px;margin-bottom:16px}.fields{display:grid;grid-template-columns:repeat(auto-fit,minmax(270px,1fr));gap:18px}label{display:flex;flex-direction:column;gap:5px;font-weight:600}code{font-size:11px;color:var(--muted);font-weight:400;overflow-wrap:anywhere}input,select{box-sizing:border-box;width:100%;padding:10px;background:var(--bg);color:var(--text);border:1px solid var(--border);border-radius:8px}.hint{margin-top:16px}button{background:#818cf8;color:#101426;border:0;border-radius:8px;padding:11px 18px;font-weight:700;cursor:pointer}button:disabled{opacity:.5}.notice{padding:12px;margin-bottom:16px;border:1px solid #4a9f85;border-radius:8px}.error{border-color:#d96969}
</style>
