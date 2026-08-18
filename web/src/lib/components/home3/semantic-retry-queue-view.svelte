<script lang="ts">
	import { onMount } from 'svelte';
	import RefreshCwIcon from '@lucide/svelte/icons/refresh-cw';
	import { listRetryQueue, type RetryQueueJob, type RetryQueueFilters } from './semantic-retry-queue-client';
	let { darkMode = true }: { darkMode: boolean } = $props();
	let bg=$derived(darkMode?'#171B26':'#F2F4F7'), card=$derived(darkMode?'#1F2333':'#FFFFFF'), surface=$derived(darkMode?'#252A3A':'#ECEEF2'), border=$derived(darkMode?'#2D3348':'#E4E6EB'), accent=$derived(darkMode?'#818CF8':'#6366F1'), text=$derived(darkMode?'#E2E8F0':'#111827'), muted=$derived(darkMode?'#94A3B8':'#6B7280'), danger=$derived(darkMode?'#F87171':'#DC2626'), warn=$derived(darkMode?'#FBBF24':'#D97706');
	let rows=$state<RetryQueueJob[]>([]), total=$state(0), page=$state(1), pageSize=$state(50), loading=$state(false), error=$state('');
	let filters=$state<RetryQueueFilters>({state:'',outcome_id:''});
	const states=['pending','claimed','done','stale','failed'];
	let totalPages=$derived(Math.max(1,Math.ceil(total/pageSize)));
	function stateColor(s:string){return s==='done'?'#22c55e':s==='pending'?accent:s==='claimed'?warn:s==='stale'||s==='failed'?danger:muted}
	async function load(){loading=true;error='';try{const r=await listRetryQueue(filters,page,pageSize);rows=r.results;total=r.total;}catch(e){error=e instanceof Error?e.message:String(e)}finally{loading=false}}
	function apply(){page=1;load()}
	function time(v:string){const d=new Date(v);return Number.isNaN(d.getTime())?v:d.toLocaleString()}
	onMount(load);
</script>

<div class="h-full space-y-4 overflow-auto p-6" style="background:{bg}">
	<div class="rounded-xl p-5" style="background:{card};border:1px solid {border}">
		<div class="flex flex-wrap items-start justify-between gap-3">
			<div>
				<h2 style="font-size:18px;font-weight:600;color:{text}">Semantic Retry Queue</h2>
				<p style="font-size:13px;color:{muted};margin-top:2px">
					Read-only view of <code style="color:{accent}">kb.semantic_retry_queue</code> (ADR 2026081801
					DR10). Jobs are keyed on the dependency they are waiting for, not on re-running a
					processor.
				</p>
			</div>
			<button onclick={load} disabled={loading} class="cursor-pointer rounded-lg px-3 py-2 text-sm" style="background:{surface};color:{text};border:1px solid {border}">
				<RefreshCwIcon class="inline h-4 w-4" /> Refresh
			</button>
		</div>
	</div>

	<div class="rounded-xl p-5" style="background:{card};border:1px solid {border}">
		<div class="grid gap-3" style="grid-template-columns:repeat(auto-fill,minmax(160px,1fr))">
			<label style="color:{muted}">State
				<select bind:value={filters.state} class="w-full rounded px-2 py-1.5 text-sm" style="background:{surface};color:{text};border:1px solid {border}">
					<option value="">Any</option>
					{#each states as s}<option>{s}</option>{/each}
				</select>
			</label>
			<label style="color:{muted}">Outcome ID
				<input bind:value={filters.outcome_id} class="w-full rounded px-2 py-1.5 text-sm" style="background:{surface};color:{text};border:1px solid {border}" />
			</label>
		</div>
		<button onclick={apply} class="mt-3 cursor-pointer rounded-lg px-3 py-2 text-sm" style="background:{accent};color:white">Apply Filters</button>
	</div>

	{#if error}<div class="rounded-xl p-4" style="background:{danger}20;color:{danger}">{error}</div>{/if}

	<div class="overflow-hidden rounded-xl" style="background:{card};border:1px solid {border}">
		<div class="flex justify-between px-5 py-3 text-sm" style="border-bottom:1px solid {border};color:{muted}">
			<span>Total: {total}{#if total} &middot; page {page} of {totalPages}{/if}</span>
			<div class="flex items-center gap-2">
				<button onclick={()=>{if(page>1){page--;load()}}} disabled={page<=1||loading}>&lsaquo;</button>
				<button onclick={()=>{if(page<totalPages){page++;load()}}} disabled={page>=totalPages||loading}>&rsaquo;</button>
			</div>
		</div>
		{#if loading}
			<div class="p-8 text-center" style="color:{muted}">Loading&hellip;</div>
		{:else if !rows.length}
			<div class="p-8 text-center" style="color:{muted}">No retry queue jobs found.</div>
		{:else}
			<div class="overflow-auto">
				<table class="w-full text-left text-sm">
					<thead style="color:{muted}">
						<tr>
							<th class="px-4 py-3">ID</th>
							<th class="px-4 py-3">State</th>
							<th class="px-4 py-3">Outcome</th>
							<th class="px-4 py-3">Artifact</th>
							<th class="px-4 py-3">Stage</th>
							<th class="px-4 py-3">Target Fingerprint</th>
							<th class="px-4 py-3">Attempts</th>
							<th class="px-4 py-3">Last Error</th>
							<th class="px-4 py-3">Modified</th>
						</tr>
					</thead>
					<tbody>
						{#each rows as row}
							<tr style="border-top:1px solid {border};color:{text}">
								<td class="px-4 py-3">{row.id}</td>
								<td class="px-4 py-3">
									<span style="padding:0.1rem 0.5rem;border-radius:999px;background:{stateColor(row.state)}20;color:{stateColor(row.state)};font-size:12px;font-weight:600">{row.state}</span>
								</td>
								<td class="px-4 py-3">#{row.outcome_id}{#if row.outcome_input_record_id} &middot; doc {row.outcome_input_record_id}{/if}</td>
								<td class="max-w-40 truncate px-4 py-3" title="{row.outcome_artifact_type}:{row.outcome_artifact_id}">{row.outcome_artifact_type}:{row.outcome_artifact_id}</td>
								<td class="px-4 py-3">{row.outcome_stage_term_id || '—'}</td>
								<td class="max-w-48 truncate px-4 py-3" style="font-family:monospace;font-size:12px" title={row.target_dependency_fingerprint}>{row.target_dependency_fingerprint}</td>
								<td class="px-4 py-3">{row.attempts}</td>
								<td class="max-w-52 truncate px-4 py-3" title={row.last_error}>{row.last_error || '—'}</td>
								<td class="px-4 py-3" style="white-space:nowrap">{time(row.modify_time)}</td>
							</tr>
						{/each}
					</tbody>
				</table>
			</div>
		{/if}
	</div>
</div>
