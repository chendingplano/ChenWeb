<script lang="ts">
 import { m } from '$lib/paraglide/messages.js';
 let { darkMode = true }: { darkMode: boolean } = $props();
 type Paper = { title?: string; authors?: string; published_date?: string; source?: string; doi?: string; url?: string; abstract?: string };
 type Result = { papers?: Paper[]; errors?: Record<string, unknown> };
 let query = $state(''); let result = $state<Result | null>(null); let busy = $state(false); let failed = $state(false);
 let errorSources = $derived(Object.keys(result?.errors ?? {}));
 function safeUrl(url?: string) { try { const u = new URL(url ?? ''); return u.protocol === 'https:' || u.protocol === 'http:' ? u.href : ''; } catch { return ''; } }
 async function search() {
  if (!query.trim() || busy) return;
  busy = true; failed = false; result = null;
  try { const response = await fetch('/api/v1/scientific-paper-search', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ query: query.trim() }) }); if (!response.ok) throw new Error(); result = await response.json(); }
  catch { failed = true; } finally { busy = false; }
 }
</script>
<section class:light={!darkMode} class="paper-search select-text"><div class="inner">
 <p class="eyebrow">{m.paper_search_eyebrow()}</p><h1>{m.paper_search_title()}</h1><p class="intro">{m.paper_search_intro()}</p>
 <form onsubmit={(event) => { event.preventDefault(); search(); }}><label for="paper-query">{m.paper_search_query()}</label><div class="search-row"><input id="paper-query" bind:value={query} maxlength="1000" placeholder={m.paper_search_placeholder()} /><button disabled={busy || !query.trim()}>{busy ? m.paper_search_searching() : m.paper_search_submit()}</button></div></form>
 {#if failed}<p class="notice" role="alert">{m.paper_search_error()}</p>{/if}
 {#if result}<div class="results-heading"><h2>{m.paper_search_results()}</h2><span>{m.paper_search_count({ count: String(result.papers?.length ?? 0) })}</span></div>
 {#if errorSources.length}<p class="notice" role="status">{m.paper_search_partial({ sources: errorSources.join(', ') })}</p>{/if}
 {#if !result.papers?.length}<p class="empty">{m.paper_search_empty()}</p>{/if}
 {#each result.papers ?? [] as paper}<article><div class="meta"><span>{paper.source}</span>{#if paper.published_date}<span>{paper.published_date.slice(0,10)}</span>{/if}</div><h3>{#if safeUrl(paper.url)}<a href={safeUrl(paper.url)} target="_blank" rel="noopener noreferrer">{paper.title}</a>{:else}{paper.title}{/if}</h3>{#if paper.authors}<p class="authors">{paper.authors}</p>{/if}{#if paper.doi}<p class="doi">{m.paper_search_doi({ doi: paper.doi })}</p>{/if}{#if paper.abstract}<p class="abstract">{paper.abstract.replace(/<[^>]*>/g, '')}</p>{/if}</article>{/each}{/if}
</div></section>
<style>
 .paper-search { --bg:#171b26; --card:#1f2531; --ink:#e7edf2; --muted:#a1afbd; --line:#394657; --accent:#8dd3c7; min-height:100%; background:var(--bg); color:var(--ink); padding:clamp(24px,4vw,56px); } .paper-search.light { --bg:#f4f5f2; --card:#fff; --ink:#182b31; --muted:#566a70; --line:#cbd6d2; --accent:#126c62; } .inner { max-width:1000px; margin:auto; } .eyebrow { color:var(--accent); text-transform:uppercase; letter-spacing:.15em; font-size:.72rem; font-weight:700; } h1 { font-size:clamp(2rem,4vw,3rem); letter-spacing:-.04em; margin:.5rem 0; } .intro { color:var(--muted); max-width:680px; line-height:1.6; } form { margin:2rem 0; } label { display:block; font-weight:600; font-size:.85rem; margin-bottom:.6rem; } .search-row { display:flex; gap:.6rem; } input { flex:1; min-width:0; border:1px solid var(--line); background:var(--card); color:var(--ink); border-radius:6px; padding:.85rem; } button { border:0; border-radius:6px; background:var(--accent); color:#102f2c; font-weight:700; padding:.85rem 1.4rem; cursor:pointer; } button:disabled { opacity:.5; cursor:default; } .results-heading { display:flex; align-items:baseline; justify-content:space-between; border-bottom:1px solid var(--line); } .results-heading h2 { font-size:1.1rem; } .results-heading span,.meta,.authors,.empty { color:var(--muted); font-size:.85rem; } article { padding:1.4rem 0; border-bottom:1px solid var(--line); } .meta { display:flex; gap:1rem; text-transform:uppercase; letter-spacing:.07em; font-size:.7rem; } h3 { font-size:1.1rem; margin:.5rem 0; } a { color:var(--ink); text-decoration-color:var(--accent); text-underline-offset:4px; } .authors,.doi { margin:.4rem 0; } .doi { color:var(--accent); font-size:.8rem; } .abstract { line-height:1.55; color:var(--muted); font-size:.9rem; } .notice { border-left:3px solid #d6a85e; padding:.7rem 1rem; background:var(--card); } @media(max-width:600px) { .search-row { flex-direction:column; } }
</style>
