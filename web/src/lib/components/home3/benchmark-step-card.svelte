<script lang="ts">
	import PlayIcon from '@lucide/svelte/icons/play';
	import LoaderCircleIcon from '@lucide/svelte/icons/loader-circle';
	import type { BenchmarkStepState } from '$lib/services/docBenchmarkAdminService';

	let {
		step,
		darkMode = true,
		busy = false,
		onRun = (_stepId: string) => {}
	}: {
		step: BenchmarkStepState;
		darkMode?: boolean;
		busy?: boolean;
		onRun?: (stepId: string) => void;
	} = $props();

	let cardBg = $derived(darkMode ? '#1F2333' : '#FFFFFF');
	let border = $derived(darkMode ? '#2D3348' : '#E4E6EB');
	let textPrimary = $derived(darkMode ? '#E2E8F0' : '#111827');
	let textSecondary = $derived(darkMode ? '#94A3B8' : '#6B7280');
	let muted = $derived(darkMode ? '#64748B' : '#9CA3AF');
	let accent = $derived(darkMode ? '#818CF8' : '#6366F1');

	function statusTone(status: string) {
		switch (status) {
			case 'completed':
				return { bg: darkMode ? 'rgba(52,211,153,0.14)' : 'rgba(16,185,129,0.10)', fg: darkMode ? '#34d399' : '#059669' };
			case 'running':
				return { bg: darkMode ? 'rgba(129,140,248,0.16)' : 'rgba(99,102,241,0.12)', fg: accent };
			case 'failed':
			case 'blocked':
				return { bg: darkMode ? 'rgba(248,113,113,0.14)' : 'rgba(220,38,38,0.08)', fg: darkMode ? '#f87171' : '#dc2626' };
			default:
				return { bg: darkMode ? 'rgba(148,163,184,0.12)' : 'rgba(107,114,128,0.08)', fg: textSecondary };
		}
	}

	let tone = $derived(statusTone(step.status));
	let completed = $derived(step.status === 'completed');
</script>

<section
	class="step-card"
	style="background:{cardBg}; border:1px solid {border}; color:{textPrimary};"
>
	<div class="step-head">
		<div class="step-meta">
			<div class="step-title-row">
				<h3>{step.title}</h3>
				<span class="status-pill" style="background:{tone.bg}; color:{tone.fg};">{step.status}</span>
			</div>
			<p class="step-desc" style="color:{textSecondary};">{step.description}</p>
			{#if step.message}
				<p class="step-msg" style="color:{completed ? muted : textSecondary};">{step.message}</p>
			{/if}
		</div>
		<button
			class="step-run"
			style="border:1px solid {border}; color:{busy ? textSecondary : accent};"
			disabled={busy || step.status === 'running'}
			onclick={() => onRun(step.id)}
		>
			{#if busy || step.status === 'running'}
				<LoaderCircleIcon size={15} style="animation:spin 1s linear infinite;" />
				<span>Running</span>
			{:else}
				<PlayIcon size={15} />
				<span>{completed ? 'Run again' : 'Run step'}</span>
			{/if}
		</button>
	</div>

	{#if step.completed_at || step.failed_at}
		<div class="step-time" style="color:{muted};">
			{#if step.completed_at}
				<span>Completed: {step.completed_at}</span>
			{:else if step.failed_at}
				<span>Failed: {step.failed_at}</span>
			{/if}
		</div>
	{/if}

	{#if step.detected && Object.keys(step.detected).length > 0}
		<div class="detected-grid">
			{#each Object.entries(step.detected) as [key, value]}
				<div class="detected-item" style="border-top:1px solid {border};">
					<div class="detected-key" style="color:{muted};">{key}</div>
					<div class="detected-value" style="color:{textPrimary};">
						{typeof value === 'object' ? JSON.stringify(value) : String(value)}
					</div>
				</div>
			{/each}
		</div>
	{/if}
</section>

<style>
	.step-card {
		border-radius: 16px;
		padding: 18px;
		display: flex;
		flex-direction: column;
		gap: 12px;
	}
	.step-head {
		display: flex;
		gap: 16px;
		justify-content: space-between;
		align-items: flex-start;
	}
	.step-meta {
		display: flex;
		flex-direction: column;
		gap: 6px;
		min-width: 0;
	}
	.step-title-row {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 10px;
	}
	h3 {
		margin: 0;
		font-size: 15px;
		font-weight: 700;
	}
	.step-desc, .step-msg, .step-time {
		margin: 0;
		font-size: 13px;
		line-height: 1.5;
	}
	.status-pill {
		font-size: 11px;
		font-weight: 700;
		padding: 3px 8px;
		border-radius: 999px;
		text-transform: uppercase;
		letter-spacing: 0.04em;
	}
	.step-run {
		display: inline-flex;
		align-items: center;
		gap: 7px;
		border-radius: 10px;
		padding: 9px 12px;
		background: transparent;
		cursor: pointer;
		font-size: 12px;
		font-weight: 700;
	}
	.step-run:disabled {
		cursor: not-allowed;
		opacity: 0.75;
	}
	.detected-grid {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(220px, 1fr));
		gap: 0 14px;
	}
	.detected-item {
		padding-top: 10px;
	}
	.detected-key {
		font-size: 11px;
		text-transform: uppercase;
		letter-spacing: 0.05em;
		margin-bottom: 4px;
	}
	.detected-value {
		font-size: 13px;
		word-break: break-word;
	}
	@keyframes spin {
		from { transform: rotate(0deg); }
		to { transform: rotate(360deg); }
	}
</style>
