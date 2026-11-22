<script lang="ts">
 import { onMount } from 'svelte';
 import { m } from '$lib/paraglide/messages.js';
 let { darkMode = true }: { darkMode: boolean } = $props();
 type Row = Record<string, string | number | null> & { id: number; created_at: string };
 let rows = $state<Row[]>([]), page = $state(1), more = $state(false), loading = $state(false), error = $state('');
 let selected = $state<number | null>(null);
 let labels = $derived<Record<string, string>>({ id: m.failed_calls_id(), created_at: m.failed_calls_created_at(), operation: m.failed_calls_operation(), model: m.failed_calls_model(), outcome: m.failed_calls_outcome(), record_id: m.failed_calls_record_id(), run_id: m.failed_calls_run_id(), primary_error: m.failed_calls_primary_error(), prompt_name: m.failed_calls_prompt_name(), primary_model: m.failed_calls_primary_model(), fallback_model: m.failed_calls_fallback_model(), raw_response: m.failed_calls_raw_response(), chunk_index: m.failed_calls_chunk_index(), attempt: m.failed_calls_attempt(), review_run_id: m.failed_calls_review_run_id(), aspect: m.failed_calls_aspect() });
 async function load() {
  loading = true; error = '';
  try {
   const response = await fetch(`/api/v1/kb/failed-llm-calls?page=${page}`, { credentials: 'same-origin' });
   const data = await response.json();
   if (!response.ok || !data.status) throw new Error(m.failed_calls_load_error());
   rows = data.results; more = data.more;
  } catch { error = m.failed_calls_load_error(); }
  finally { loading = false; }
 }
 function move(delta: number) { page += delta; selected = null; void load(); }
 onMount(load);
</script>

<div class="select-text h-full flex flex-col gap-4 p-6" style:background={darkMode ? '#171B26' : '#F2F4F7'} style:color={darkMode ? '#E2E8F0' : '#111827'}>
 <div class="flex flex-wrap items-center justify-between gap-3">
  <div><h2 class="text-lg font-semibold">{m.nav_sysadmin_failed_llm_calls()}</h2><p class="text-sm opacity-70">{m.failed_calls_description()} <code>kb.failed_llm_calls</code></p></div>
  <button class="rounded border px-3 py-2" onclick={load} disabled={loading}>{m.failed_calls_refresh()}</button>
 </div>
 {#if error}<p role="alert">{error}</p>{/if}
 <div class="overflow-auto flex-1 rounded border">
  <table class="w-full text-left text-sm">
   <thead><tr>{#each ['id','created_at','operation','model','outcome','record_id','run_id','primary_error'] as key}<th class="p-2 whitespace-nowrap">{labels[key] ?? key}</th>{/each}<th class="p-2">{m.failed_calls_details()}</th></tr></thead>
   <tbody>
    {#each rows as row (row.id)}
     <tr class="border-t align-top">{#each ['id','created_at','operation','model','outcome','record_id','run_id','primary_error'] as key}<td class="p-2" style="max-width: 25rem; overflow-wrap:anywhere">{row[key] ?? '—'}</td>{/each}<td class="p-2"><button class="underline" onclick={() => selected = selected === row.id ? null : row.id}>{m.failed_calls_details()}</button></td></tr>
     {#if selected === row.id}<tr><td colspan="9" class="p-3"><dl class="grid gap-2">{#each Object.entries(row) as [key, value]}<div><dt class="font-semibold">{labels[key] ?? key}</dt><dd class="whitespace-pre-wrap break-all">{value ?? '—'}</dd></div>{/each}</dl></td></tr>{/if}
    {/each}
   </tbody>
  </table>
  {#if !loading && !error && rows.length === 0}<p class="p-4">{m.failed_calls_empty()}</p>{/if}
 </div>
 <div class="flex items-center gap-3"><button class="rounded border px-3 py-1" disabled={page === 1 || loading} onclick={() => move(-1)}>{m.failed_calls_previous()}</button><span>{m.failed_calls_page({ page: String(page) })}</span><button class="rounded border px-3 py-1" disabled={!more || loading} onclick={() => move(1)}>{m.failed_calls_next()}</button></div>
</div>
