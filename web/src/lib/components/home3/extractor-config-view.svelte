<script lang="ts">
 import { onMount, tick } from 'svelte';
 import { m } from '$lib/paraglide/messages.js';
 let { darkMode = true, extractor }: { darkMode?: boolean; extractor: 'metrics-simple' | 'references' | 'provisions' } = $props();
 let values = $state<Record<string,string>>({}), inherited = $state<Record<string,string>>({});
 let models = $state<string[]>([]), prompts = $state<Record<string,string[]>>({});
 let loading = $state(true), loaded = $state(false), saving = $state(false), error = $state(false), saved = $state(false);
 let viewport: HTMLDivElement;
 const draftKey = `extractor-config-${extractor}-draft`, scrollKey = `extractor-config-${extractor}-scroll`;
 let title = $derived(extractor === 'metrics-simple' ? m.nav_sysadmin_doc_process_config_metrics_simple() : extractor === 'references' ? m.nav_sysadmin_doc_process_config_references() : m.nav_sysadmin_doc_process_config_provisions());
 let fields = $derived(extractor === 'metrics-simple' ? [
  { key:'EXTRACT_METRICS_SIMPLE_MODEL_NAME', label:m.extractor_config_model(), type:'model' },
  { key:'EXTRACT_METRICS_SIMPLE_PROMPT', label:m.extractor_config_prompt(), type:'prompt' },
  { key:'EXTRACT_METRICS_MAX_TASKS', label:m.extractor_config_tasks(), type:'number' }
 ] : extractor === 'references' ? [
  { key:'EXTRACT_REFERENCES_MODEL_NAME', label:m.extractor_config_model(), type:'model' },
  { key:'EXTRACT_REFERENCES_PROMPT', label:m.extractor_config_prompt(), type:'prompt' }
 ] : [
  { key:'EXTRACT_PROVISIONS_MODEL_NAME', label:m.extractor_config_model(), type:'model' },
  { key:'EXTRACT_PROVISIONS_MODEL_FALLBACK', label:m.extractor_config_fallback(), type:'model' },
  { key:'EXTRACT_PROVISIONS_PROMPT', label:m.extractor_config_prompt(), type:'prompt' },
  { key:'EXTRACT_PROVISIONS_REVIEW_PROMPT', label:m.extractor_config_review_prompt(), type:'prompt' },
  { key:'EXTRACT_PROVISIONS_MAX_TASKS', label:m.extractor_config_tasks(), type:'number' },
  { key:'EXTRACT_PROVISIONS_INPUT', label:m.extractor_config_input(), type:'input' }
 ]);
 function remember() { saved = false; try { sessionStorage.setItem(draftKey, JSON.stringify(values)); } catch { /* storage is optional */ } }
 onMount(() => {
  const scroller = viewport.closest('main'); let mounted = true;
  const rememberScroll = () => { try { if (scroller) sessionStorage.setItem(scrollKey, String(scroller.scrollTop)); } catch { /* storage is optional */ } };
  scroller?.addEventListener('scroll', rememberScroll);
  void (async () => { try {
   const res = await fetch(`/api/v1/kb/extractor-config/${extractor}`, { credentials:'same-origin' }); if (!res.ok) throw new Error();
   const data = await res.json(); values = data.config; models = data.models; prompts = data.prompts; inherited = data.inherited; loaded = true;
   try { const draft = JSON.parse(sessionStorage.getItem(draftKey) ?? 'null'); if (draft && typeof draft === 'object' && !Array.isArray(draft)) values = draft; } catch { /* invalid draft */ }
  } catch { error = true; } finally { loading = false; await tick(); try { if (mounted && scroller) scroller.scrollTop = Number(sessionStorage.getItem(scrollKey) ?? 0); } catch { /* storage is optional */ } } })();
  return () => { mounted = false; scroller?.removeEventListener('scroll', rememberScroll); };
 });
 async function save() { saving = true; error = false; try {
  const res = await fetch(`/api/v1/kb/extractor-config/${extractor}`, { method:'PUT', credentials:'same-origin', headers:{'Content-Type':'application/json'}, body:JSON.stringify(values) }); if (!res.ok) throw new Error();
  try { sessionStorage.removeItem(draftKey); } catch { /* storage is optional */ } saved = true;
 } catch { error = true; } finally { saving = false; } }
</script>
<div bind:this={viewport} class:light={!darkMode} class="page select-text">
 <header><h1>{title}</h1><p>{m.extractor_config_intro()}</p><p>{m.products_config_inherited_note()}</p><p>{m.products_config_restart()}</p></header>
 {#if error}<p class="notice error" role="alert">{m.products_config_error()}</p>{/if}
 {#if saved}<p class="notice" role="status">{m.products_config_saved()}</p>{/if}
 {#if !loading && loaded}<form onsubmit={(event) => { event.preventDefault(); void save(); }}>
 <section><div class="fields">{#each fields as field}<div class="field"><label for={field.key}>{field.label}</label>
 {#if field.type === 'number'}<input id={field.key} type="number" min="1" step="1" value={values[field.key] ?? ''} placeholder={inherited[field.key] || m.products_config_inherit()} oninput={(e) => { values[field.key] = e.currentTarget.value; remember(); }} />
 {:else}
 {@const options = field.type === 'model' ? models : field.type === 'prompt' ? (prompts[field.key] ?? []) : ['chunks','blocks']}
 {@const current = values[field.key] ?? ''}
 <select id={field.key} value={current} onchange={(e) => { values[field.key] = e.currentTarget.value; remember(); }}>
  <option value="">{inherited[field.key] || m.products_config_unset()}</option>
  {#if current && !options.includes(current)}<option value={current}>{current}</option>{/if}
  {#each options as option}<option value={option}>{field.type === 'input' ? (option === 'chunks' ? m.extractor_config_chunks() : m.extractor_config_blocks()) : option}</option>{/each}
 </select>{/if}</div>{/each}</div></section>
 <button disabled={saving}>{saving ? m.pipeline_config_saving() : m.pipeline_config_save()}</button></form>{/if}
</div>
<style>
 .page{--bg:#1f2333;--border:#343b52;--text:#e2e8f0;--muted:#9baac1;color:var(--text);max-width:1050px;padding:32px}.page.light{--bg:#fff;--border:#dce1e8;--text:#172033;--muted:#59677b}h1{font-size:26px;margin:0 0 8px}p{color:var(--muted);line-height:1.5;margin:5px 0}header{margin-bottom:24px}section{background:var(--bg);border:1px solid var(--border);border-radius:12px;padding:22px;margin-bottom:16px}.fields{display:grid;grid-template-columns:repeat(auto-fit,minmax(270px,1fr));gap:18px}.field{display:flex;flex-direction:column;gap:5px}label{font-weight:600}input,select{box-sizing:border-box;width:100%;padding:10px;background:var(--bg);color:var(--text);border:1px solid var(--border);border-radius:8px}button{background:#818cf8;color:#101426;border:0;border-radius:8px;padding:11px 18px;font-weight:700;cursor:pointer}button:disabled{opacity:.5}.notice{padding:12px;margin-bottom:16px;border:1px solid #4a9f85;border-radius:8px}.error{border-color:#d96969}
</style>
