<script lang="ts">
 import { onMount } from 'svelte';
 import { m } from '$lib/paraglide/messages.js';
 let { darkMode = true }: { darkMode: boolean } = $props();
 type Credential = { key: string; name: string; sensitive: boolean };
 const fields: Credential[] = [
  {key:'OPENALEX_API_KEY',name:'OpenAlex',sensitive:true}, {key:'OPENALEX_EMAIL',name:'OpenAlex',sensitive:false},
  {key:'SEMANTIC_SCHOLAR_API_KEY',name:'Semantic Scholar',sensitive:true}, {key:'CORE_API_KEY',name:'CORE',sensitive:true},
  {key:'UNPAYWALL_EMAIL',name:'Unpaywall',sensitive:false}, {key:'DOAJ_API_KEY',name:'DOAJ',sensitive:true},
  {key:'ZENODO_ACCESS_TOKEN',name:'Zenodo',sensitive:true}, {key:'GOOGLE_SCHOLAR_PROXY_URL',name:'Google Scholar',sensitive:true},
  {key:'OPENAIRE_API_KEY',name:'OpenAIRE',sensitive:true}, {key:'CITESEERX_API_KEY',name:'CiteSeerX',sensitive:true},
  {key:'IEEE_API_KEY',name:'IEEE Xplore',sensitive:true}, {key:'WOS_API_KEY',name:'Web of Science',sensitive:true},
  {key:'SCOPUS_API_KEY',name:'Scopus',sensitive:true}, {key:'SCOPUS_INST_TOKEN',name:'Scopus',sensitive:true}
 ];
 let source=$state('fast'); let maxResults=$state(10); let timeout=$state(30); let configured=$state<Record<string,boolean>>({}); let values=$state<Record<string,string>>({}); let clear=$state<Record<string,boolean>>({}); let loaded=$state(false); let loading=$state(true); let saving=$state(false); let error=$state(false); let saved=$state(false);
 function fieldLabel(field:Credential) { if(field.key.endsWith('_EMAIL')) return m.paper_config_email({provider:field.name}); if(field.key.endsWith('_PROXY_URL')) return m.paper_config_proxy({provider:field.name}); if(field.key.endsWith('_INST_TOKEN')) return m.paper_config_institution({provider:field.name}); if(field.key.endsWith('_ACCESS_TOKEN')) return m.paper_config_token({provider:field.name}); return m.paper_config_key({provider:field.name}); }
 async function load() { loading=true; error=false; try { const r=await fetch('/api/v1/scientific-paper-search/config'); if(!r.ok) throw new Error(); const c=await r.json(); source=c.source;maxResults=c.max_results;timeout=c.timeout;configured=c.configured ?? {};loaded=true; } catch {error=true;} finally {loading=false;} }
 async function save() { saving=true;error=false;saved=false;try {const credentials=Object.fromEntries(Object.entries(values).filter(([,v])=>v.trim())); const r=await fetch('/api/v1/scientific-paper-search/config',{method:'PUT',headers:{'Content-Type':'application/json'},body:JSON.stringify({source,max_results:Number(maxResults),timeout:Number(timeout),credentials,clear:Object.keys(clear).filter(k=>clear[k])})});if(!r.ok) throw new Error();values={};clear={};await load();if(!error)saved=true;}catch{error=true;}finally{saving=false;} }
 onMount(load);
</script>
<section class:light={!darkMode} class="settings select-text"><div class="inner">
 <p class="eyebrow">{m.paper_config_eyebrow()}</p><h1>{m.paper_config_title()}</h1><p class="intro">{m.paper_config_intro()}</p>
 {#if loading}<p>{m.paper_config_loading()}</p>{:else if error && !loaded}<p role="alert">{m.paper_config_error()}</p><button type="button" onclick={load}>{m.paper_config_retry()}</button>{:else}
 <form onsubmit={(e)=>{e.preventDefault();save();}}>
  <h2>{m.paper_config_search_settings()}</h2><div class="grid">
   <label>{m.paper_config_source()}<select bind:value={source}><option value="fastest">{m.paper_config_fastest()}</option><option value="fast">{m.paper_config_fast()}</option><option value="all">{m.paper_config_all()}</option></select></label>
   <label>{m.paper_config_limit()}<input type="number" min="1" max="50" required bind:value={maxResults}/></label>
   <label>{m.paper_config_timeout()}<input type="number" min="5" max="60" required bind:value={timeout}/></label>
  </div><p class="help">{m.paper_config_sources_help()}</p>
  <h2>{m.paper_config_credentials()}</h2><p class="help">{m.paper_config_credentials_help()}</p><div class="credential-grid">
   {#each fields as field}<div class="credential"><label for={field.key}>{fieldLabel(field)}</label><span class="status">{configured[field.key] ? m.paper_config_configured() : m.paper_config_inherited()}</span><input id={field.key} type={field.sensitive?'password':'text'} autocomplete="off" bind:value={values[field.key]} disabled={clear[field.key]} placeholder={configured[field.key] ? m.paper_config_replace() : m.paper_config_optional()} />{#if configured[field.key]}<label class="clear"><input type="checkbox" bind:checked={clear[field.key]}/>{m.paper_config_remove()}</label>{/if}</div>{/each}
  </div>
  {#if error}<p class="notice" role="alert">{m.paper_config_error()}</p>{/if}{#if saved}<p class="success" role="status">{m.paper_config_saved()}</p>{/if}
  <button disabled={saving}>{saving ? m.paper_config_saving() : m.paper_config_save()}</button>
 </form>{/if}
</div></section>
<style>
 .settings{--bg:#171b26;--card:#1f2531;--ink:#e7edf2;--muted:#a1afbd;--line:#394657;--accent:#8dd3c7;min-height:100%;background:var(--bg);color:var(--ink);padding:clamp(24px,4vw,56px)}.settings.light{--bg:#f4f5f2;--card:#fff;--ink:#182b31;--muted:#566a70;--line:#cbd6d2;--accent:#126c62}.inner{max-width:1000px;margin:auto}.eyebrow{color:var(--accent);text-transform:uppercase;letter-spacing:.15em;font-size:.72rem;font-weight:700}h1{font-size:clamp(2rem,4vw,3rem);letter-spacing:-.04em;margin:.5rem 0}h2{font-size:1.1rem;margin:2.2rem 0 .9rem}.intro,.help{color:var(--muted);line-height:1.55}.help{font-size:.85rem}.grid,.credential-grid{display:grid;grid-template-columns:repeat(3,minmax(0,1fr));gap:1rem}.credential-grid{grid-template-columns:repeat(2,minmax(0,1fr))}label{display:grid;gap:.5rem;font-size:.85rem;font-weight:600}input,select{width:100%;border:1px solid var(--line);background:var(--card);color:var(--ink);border-radius:6px;padding:.7rem}.credential{border:1px solid var(--line);background:var(--card);padding:1rem;border-radius:6px}.credential input:not([type=checkbox]){margin-top:.6rem}.status{color:var(--muted);font-size:.75rem}.clear{display:flex;align-items:center;color:var(--muted);margin-top:.7rem;font-weight:400}.clear input{width:auto}button{border:0;border-radius:6px;background:var(--accent);color:#102f2c;font-weight:700;padding:.85rem 1.4rem;cursor:pointer;margin-top:1.5rem}button:disabled{opacity:.5}.notice{color:#e6a66b}.success{color:var(--accent)}@media(max-width:700px){.grid,.credential-grid{grid-template-columns:1fr}}
</style>
