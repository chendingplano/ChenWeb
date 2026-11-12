<script lang="ts">
 import { onMount, tick } from 'svelte';
 import { m } from '$lib/paraglide/messages.js';
 import { getKbFrontendConfig, savePipelineDefaults, type KbFrontendConfig } from '$lib/services/kbService';
 let { darkMode = true }: { darkMode?: boolean } = $props();
 type Mode = KbFrontendConfig['default_processing_mode'];
 let available = $state<string[]>([]);
 let mandatory = $state<string[]>([]);
 let selected = $state<string[]>([]);
 let mode = $state<Mode>('auto_offpeak');
 let loading = $state(true);
 let saving = $state(false);
 let error = $state(false);
 let saved = $state(false);
 const key = 'doc-pipeline-config-draft';
 const scrollKey = 'doc-pipeline-config-scroll';
 let viewport: HTMLDivElement;
 function remember() {
  try { sessionStorage.setItem(key, JSON.stringify({ selected, mode })); } catch { /* optional storage */ }
  saved = false;
 }
 onMount(() => {
  void (async () => {
   try {
    const cfg = await getKbFrontendConfig();
    available = cfg.required_processors ?? [];
    mandatory = cfg.mandatory_processors ?? [];
    selected = cfg.default_processors?.length ? cfg.default_processors.filter((id) => available.includes(id)) : available.filter((id) => !mandatory.includes(id));
    mode = cfg.default_processing_mode ?? 'auto_offpeak';
    try {
     const draft = JSON.parse(sessionStorage.getItem(key) ?? 'null');
     if (draft && Array.isArray(draft.selected) && ['auto_offpeak','auto','upload_only','pdf_parsing'].includes(draft.mode)) {
      selected = draft.selected.filter((id: unknown) => typeof id === 'string' && available.includes(id)); mode = draft.mode;
     }
    } catch { /* ignore invalid draft */ }
   } catch { error = true; } finally { loading = false; await tick(); try { viewport.scrollTop = Number(sessionStorage.getItem(scrollKey) ?? 0); } catch { /* optional storage */ } }
  })();
 });
 async function save() {
  saving = true; error = false;
  try { await savePipelineDefaults(selected, mode); sessionStorage.removeItem(key); saved = true; }
  catch { error = true; } finally { saving = false; }
 }
</script>
<div bind:this={viewport} class:light={!darkMode} class="page select-text" onscroll={() => { try { sessionStorage.setItem(scrollKey, String(viewport.scrollTop)); } catch { /* optional storage */ } }}>
 <header><h1>{m.pipeline_config_title()}</h1><p>{m.pipeline_config_intro()}</p></header>
 {#if error}<p class="notice error" role="alert">{m.pipeline_config_error()}</p>{/if}
 {#if saved}<p class="notice" role="status">{m.pipeline_config_saved()}</p>{/if}
 {#if !loading}
 <section><h2>{m.pipeline_config_processors()}</h2><p>{m.pipeline_config_processors_help()}</p>
  <div class="processors">
   {#each mandatory as processor}<div class="processor mandatory"><span>{processor}</span><small>{m.pipeline_config_required()}</small></div>{/each}
   {#each available.filter((id) => !mandatory.includes(id)) as processor}
    <label class="processor"><input type="checkbox" checked={selected.includes(processor)} onchange={(event) => { selected = event.currentTarget.checked ? [...selected, processor] : selected.filter((id) => id !== processor); remember(); }} /><span>{processor}</span></label>
   {/each}
  </div>
  {#if available.length === 0}<p>{m.pipeline_config_empty()}</p>{/if}
 </section>
 <section><h2>{m.pipeline_config_mode()}</h2>
  <select bind:value={mode} onchange={remember} aria-label={m.pipeline_config_mode()}>
   <option value="auto_offpeak">{m.kb_import_auto_off_peak_only()}</option><option value="auto">{m.kb_import_auto()}</option><option value="upload_only">{m.kb_import_upload_files_only()}</option><option value="pdf_parsing">{m.kb_import_pdf_parsing()}</option>
  </select>
 </section>
 <footer><p>{m.pipeline_config_restart()}</p><button onclick={save} disabled={saving || selected.length === 0}>{saving ? m.pipeline_config_saving() : m.pipeline_config_save()}</button></footer>
 {/if}
</div>
<style>
 .page { --bg:#1f2333; --border:#343b52; --text:#e2e8f0; --muted:#9baac1; color:var(--text); max-width:1050px; max-height:calc(100vh - 100px); overflow:auto; padding:32px; }
 .page.light { --bg:#fff; --border:#dce1e8; --text:#172033; --muted:#59677b; }
 h1 { font-size:26px; margin:0 0 8px; } h2 { font-size:17px; margin:0 0 8px; } p { color:var(--muted); margin:0; line-height:1.5; }
 header { margin-bottom:24px; } section { background:var(--bg); border:1px solid var(--border); border-radius:12px; padding:22px; margin-bottom:16px; }
 .processors { display:grid; grid-template-columns:repeat(auto-fit,minmax(230px,1fr)); gap:8px; margin-top:18px; }
 .processor { display:flex; gap:10px; align-items:center; border:1px solid var(--border); border-radius:8px; padding:11px; overflow-wrap:anywhere; } .mandatory { justify-content:space-between; } small { color:var(--muted); white-space:nowrap; }
 input { accent-color:#818cf8; } select { margin-top:10px; padding:10px; background:var(--bg); color:var(--text); border:1px solid var(--border); border-radius:8px; }
 footer { display:flex; align-items:center; justify-content:space-between; gap:16px; } button { background:#818cf8; color:#101426; border:0; border-radius:8px; padding:11px 18px; font-weight:700; cursor:pointer; } button:disabled { opacity:.5; cursor:default; }
 .notice { padding:12px; margin-bottom:16px; border:1px solid #4a9f85; border-radius:8px; } .error { border-color:#d96969; }
</style>
