<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { m } from '$lib/paraglide/messages.js';
	import { getLocale } from '$lib/paraglide/runtime';
	import { listQuestionJobs, type QuestionRun } from '$lib/services/questionGeneratorService';

	let { darkMode = true, onview }: { darkMode?: boolean; onview: (job: QuestionRun) => void } =
		$props();
	let jobs = $state<QuestionRun[]>([]);
	let total = $state(0);
	let page = $state(1);
	const pageSize = 25;
	let loading = $state(false);
	let error = $state(false);
	let alive = true;
	let controller: AbortController | undefined;
	let timer: ReturnType<typeof setTimeout> | undefined;
	const statusLabel = (status: QuestionRun['status']) =>
		status === 'running' ? m.qg_running() : status === 'done' ? m.qg_done() : m.qg_failed();
	const when = (iso: string) => new Date(iso).toLocaleString(getLocale());
	const duration = (job: QuestionRun) =>
		job.finished_at
			? m.qg_job_seconds({
					seconds: (
						(Date.parse(job.finished_at) - Date.parse(job.created_at)) /
						1000
					).toLocaleString(getLocale(), { maximumFractionDigits: 1 })
				})
			: '—';
	const progress = (job: QuestionRun) =>
		m.qg_progress_count({
			count:
				(job.generated_one_hop_count ?? 0) +
				(job.generated_two_hop_count ?? 0) +
				(job.generated_multi_hop_count ?? 0),
			total:
				job.input_record_id > 0
					? job.one_hop_count + job.two_hop_count + job.multi_hop_count
					: job.total_one_hop_count + job.total_two_hop_count + job.total_multi_hop_count
		});
	onMount(() => void load(1));
	export function refresh() {
		void load(1);
	}
	onDestroy(() => {
		alive = false;
		controller?.abort();
		clearTimeout(timer);
	});
	async function load(nextPage: number) {
		controller?.abort();
		clearTimeout(timer);
		const request = new AbortController();
		controller = request;
		loading = true;
		error = false;
		try {
			const result = await listQuestionJobs(nextPage, pageSize, request.signal);
			if (!alive || request.signal.aborted) return;
			jobs = result.runs;
			total = result.total;
			page = result.page;
			if (jobs.some((job) => job.status === 'running'))
				timer = setTimeout(() => void load(page), 2500);
		} catch {
			if (alive && !request.signal.aborted) error = true;
		} finally {
			if (alive && !request.signal.aborted) loading = false;
		}
	}
</script>

<div class="jobs" class:dark={darkMode} aria-busy={loading}>
	<div class="heading">
		<h2>{m.qg_jobs()}</h2>
		<div class="actions">
			<span>{m.qg_jobs_count({ count: total })}</span>
			<button type="button" disabled={loading} onclick={() => void load(page)}
				>{m.qg_jobs_refresh()}</button
			>
		</div>
	</div>
	{#if error}<p class="error" role="alert">{m.qg_load_error()}</p>{/if}
	{#if loading && jobs.length === 0}<p role="status">{m.qg_loading()}</p>
	{:else if jobs.length === 0 && !error}<p>{m.qg_no_jobs()}</p>
	{:else if jobs.length > 0}
		<div class="table-wrap">
			<table>
				<thead
					><tr>
						<th>{m.qg_filter_run()}</th><th>{m.qg_filter_record()}</th><th>{m.qg_jobs_status()}</th>
						<th class="num">{m.qg_jobs_progress()}</th><th>{m.qg_jobs_started()}</th>
						<th class="num">{m.qg_jobs_time()}</th><th>{m.qg_description()}</th><th
							><span class="sr-only">{m.qg_jobs_view()}</span></th
						>
					</tr></thead
				>
				<tbody
					>{#each jobs as job (job.id)}<tr>
							<td>{job.id}</td><td>{job.input_record_id || m.qg_jobs_random_documents()}</td>
							<td
								><span
									class="badge"
									class:running={job.status === 'running'}
									class:failed={job.status === 'failed'}>{statusLabel(job.status)}</span
								></td
							>
							<td class="num">{progress(job)}</td><td class="when">{when(job.created_at)}</td><td
								class="num">{duration(job)}</td
							>
							<td class="description">{job.description}</td><td
								><button
									type="button"
									aria-label={m.qg_jobs_view_run({ id: job.id })}
									onclick={() => onview(job)}>{m.qg_jobs_view()}</button
								></td
							>
						</tr>{/each}</tbody
				>
			</table>
		</div>
	{/if}
	<nav class="pagination" aria-label={m.qg_jobs_pagination()}>
		<button type="button" disabled={loading || page <= 1} onclick={() => void load(page - 1)}
			>{m.qg_previous()}</button
		>
		<span>{m.qg_page_status({ page, pages: Math.max(1, Math.ceil(total / pageSize)) })}</span>
		<button
			type="button"
			disabled={loading || page * pageSize >= total}
			onclick={() => void load(page + 1)}>{m.qg_next()}</button
		>
	</nav>
</div>

<style>
	.heading,
	.actions,
	.pagination {
		display: flex;
		align-items: center;
		gap: 12px;
	}
	.heading {
		justify-content: space-between;
		flex-wrap: wrap;
		margin-bottom: 20px;
	}
	h2 {
		font-size: 18px;
		margin: 0;
	}
	.actions,
	.pagination {
		font-size: 13px;
		color: var(--muted);
	}
	button {
		color: var(--text);
		border: 1px solid var(--line);
		border-radius: 6px;
		padding: 8px 12px;
		font-size: 13px;
		cursor: pointer;
		background: var(--bg);
	}
	button:disabled {
		opacity: 0.5;
		cursor: default;
	}
	button:focus-visible {
		outline: 2px solid var(--accent);
		outline-offset: 2px;
	}
	.table-wrap {
		overflow-x: auto;
	}
	table {
		width: 100%;
		border-collapse: collapse;
		font-size: 13px;
	}
	th,
	td {
		padding: 12px 10px;
		border-bottom: 1px solid var(--line);
		text-align: left;
		vertical-align: top;
	}
	th {
		color: var(--muted);
		font-size: 11px;
		text-transform: uppercase;
		letter-spacing: 0.6px;
		white-space: nowrap;
	}
	.num {
		text-align: right;
		white-space: nowrap;
		font-variant-numeric: tabular-nums;
	}
	.when {
		white-space: nowrap;
	}
	.description {
		max-width: 320px;
		overflow-wrap: anywhere;
	}
	.badge {
		display: inline-block;
		font-size: 11px;
		border: 1px solid var(--line);
		border-radius: 4px;
		padding: 3px 7px;
		color: var(--accent);
		white-space: nowrap;
	}
	.badge.running {
		--running-background: #fef3c7;
		--running-highlight: #fcd34d;
		color: #78350f;
		background: var(--running-background);
		border-color: #d97706;
		font-weight: 700;
		animation: running-flash 1.4s ease-in-out infinite;
	}
	.dark .badge.running {
		--running-background: #78350f;
		--running-highlight: #92400e;
		color: #fef3c7;
		border-color: #f59e0b;
	}
	.badge.failed,
	.error {
		color: #dc2626;
	}
	.dark .badge.failed,
	.dark .error {
		color: #fca5a5;
	}
	@keyframes running-flash {
		50% {
			background: var(--running-highlight);
			box-shadow: 0 0 0 3px #f59e0b40;
		}
	}
	@media (prefers-reduced-motion: reduce) {
		.badge.running {
			animation: none;
		}
	}
	.pagination {
		justify-content: flex-end;
		margin-top: 20px;
	}
	.sr-only {
		position: absolute;
		width: 1px;
		height: 1px;
		overflow: hidden;
		clip: rect(0 0 0 0);
		white-space: nowrap;
	}
</style>
