<script lang="ts">
	import type { BenchmarkJob } from '$lib/services/docBenchmarkAdminService';

	let {
		jobs = [],
		darkMode = true
	}: {
		jobs?: BenchmarkJob[];
		darkMode?: boolean;
	} = $props();

	let cardBg = $derived(darkMode ? '#1F2333' : '#FFFFFF');
	let border = $derived(darkMode ? '#2D3348' : '#E4E6EB');
	let textPrimary = $derived(darkMode ? '#E2E8F0' : '#111827');
	let textSecondary = $derived(darkMode ? '#94A3B8' : '#6B7280');
	let muted = $derived(darkMode ? '#64748B' : '#9CA3AF');
</script>

<section class="jobs-card" style="background:{cardBg}; border:1px solid {border}; color:{textPrimary};">
	<div class="jobs-head">
		<h3>Recent Activity</h3>
		<p style="color:{textSecondary};">Latest benchmark jobs, outputs, and failures.</p>
	</div>
	{#if jobs.length === 0}
		<p class="empty" style="color:{muted};">No benchmark jobs yet.</p>
	{:else}
		<div class="job-list">
			{#each jobs as job (job.id)}
				<div class="job-row" style="border-top:1px solid {border};">
					<div class="job-main">
						<div class="job-line">
							<strong>{job.step_id || job.job_type}</strong>
							<span class="job-status">{job.status}</span>
						</div>
						<div class="job-line muted" style="color:{textSecondary};">
							<span>Job #{job.id}</span>
							{#if job.finished_at}
								<span>{job.finished_at}</span>
							{:else if job.started_at}
								<span>Started {job.started_at}</span>
							{:else if job.created_at}
								<span>Queued {job.created_at}</span>
							{/if}
						</div>
						{#if job.message}
							<p class="job-message" style="color:{muted};">{job.message}</p>
						{/if}
						{#if job.result && Object.keys(job.result).length > 0}
							<pre>{JSON.stringify(job.result, null, 2)}</pre>
						{/if}
						{#if job.error_text}
							<pre class="error">{job.error_text}</pre>
						{/if}
					</div>
				</div>
			{/each}
		</div>
	{/if}
</section>

<style>
	.jobs-card {
		border-radius: 16px;
		padding: 18px;
	}
	.jobs-head h3, .jobs-head p {
		margin: 0;
	}
	.jobs-head {
		display: flex;
		flex-direction: column;
		gap: 6px;
		margin-bottom: 10px;
	}
	.empty {
		margin: 0;
		font-size: 13px;
	}
	.job-row {
		padding: 12px 0;
	}
	.job-line {
		display: flex;
		justify-content: space-between;
		gap: 12px;
		font-size: 13px;
	}
	.job-status {
		text-transform: uppercase;
		font-size: 11px;
		letter-spacing: 0.04em;
	}
	.job-message {
		margin: 8px 0 0;
		font-size: 13px;
	}
	pre {
		margin: 10px 0 0;
		padding: 10px;
		border-radius: 10px;
		font-size: 12px;
		white-space: pre-wrap;
		word-break: break-word;
		background: rgba(15, 23, 42, 0.18);
	}
	.error {
		background: rgba(220, 38, 38, 0.08);
	}
</style>
