<script lang="ts">
	import { onDestroy, onMount } from 'svelte';
	import RefreshCwIcon from '@lucide/svelte/icons/refresh-cw';
	import SaveIcon from '@lucide/svelte/icons/save';
	import PlayIcon from '@lucide/svelte/icons/play';
	import BenchmarkStepCard from '$lib/components/home3/benchmark-step-card.svelte';
	import BenchmarkJobList from '$lib/components/home3/benchmark-job-list.svelte';
	import {
		getBenchmarkSetupState,
		runBenchmarkStep,
		runNextBenchmarkStep,
		saveBenchmarkConfig,
		type BenchmarkConfig,
		type BenchmarkSetupState
	} from '$lib/services/docBenchmarkAdminService';

	let { darkMode = true }: { darkMode?: boolean } = $props();

	let pageBg = $derived(darkMode ? '#171B26' : '#F2F4F7');
	let cardBg = $derived(darkMode ? '#1F2333' : '#FFFFFF');
	let border = $derived(darkMode ? '#2D3348' : '#E4E6EB');
	let textPrimary = $derived(darkMode ? '#E2E8F0' : '#111827');
	let textSecondary = $derived(darkMode ? '#94A3B8' : '#6B7280');
	let muted = $derived(darkMode ? '#64748B' : '#9CA3AF');
	let accent = $derived(darkMode ? '#818CF8' : '#6366F1');

	let setupState = $state<BenchmarkSetupState | null>(null);
	let draft: BenchmarkConfig | null = $state(null);
	let loading = $state(true);
	let saving = $state(false);
	let runningStepId: string | null = $state(null);
	let runNextBusy = $state(false);
	let error = $state('');
	let pollTimer: ReturnType<typeof setInterval> | null = null;

	const configFields: Array<{ key: keyof BenchmarkConfig; label: string; type?: string }> = [
		{ key: 'experiment_path', label: 'Experiment path' },
		{ key: 'dataset_root', label: 'Dataset root' },
		{ key: 'artifact_root', label: 'Artifact root' },
		{ key: 'work_root', label: 'Work root' },
		{ key: 'evidence_root', label: 'Evidence root' },
		{ key: 'store_id', label: 'Store ID', type: 'number' },
		{ key: 'owner', label: 'Owner' },
		{ key: 'tenant_id', label: 'Tenant ID' },
		{ key: 'metrics_model_name', label: 'Metrics model name' },
		{ key: 'report_format', label: 'Report format' },
		{ key: 'report_output_path', label: 'Report output path' },
		{ key: 'metrics_baseline', label: 'Metrics baseline' },
		{ key: 'metrics_candidate', label: 'Metrics candidate' },
		{ key: 'chunk_baseline', label: 'Chunk baseline' },
		{ key: 'chunk_candidate', label: 'Chunk candidate' }
	];

	async function loadState() {
		loading = true;
		error = '';
		try {
			setupState = await getBenchmarkSetupState();
			draft = { ...setupState.config };
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to load benchmark setup state';
		} finally {
			loading = false;
		}
	}

	async function refreshState() {
		try {
			setupState = await getBenchmarkSetupState();
			if (!draft) {
				draft = { ...setupState.config };
			}
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to refresh benchmark setup state';
		}
	}

	async function saveConfig() {
		if (!draft) return;
		saving = true;
		error = '';
		try {
			draft = await saveBenchmarkConfig(draft);
			await refreshState();
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to save benchmark config';
		} finally {
			saving = false;
		}
	}

	async function runStep(stepId: string) {
		runningStepId = stepId;
		error = '';
		try {
			await runBenchmarkStep(stepId);
			await refreshState();
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to run benchmark step';
		} finally {
			runningStepId = null;
		}
	}

	async function runNext() {
		runNextBusy = true;
		error = '';
		try {
			await runNextBenchmarkStep();
			await refreshState();
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to run the next unfinished step';
		} finally {
			runNextBusy = false;
		}
	}

	function updateField(key: keyof BenchmarkConfig, value: string | boolean) {
		if (!draft) return;
		if (key === 'store_id' && typeof value === 'string') {
			draft = { ...draft, [key]: Number(value || 0) };
			return;
		}
		draft = { ...draft, [key]: value } as BenchmarkConfig;
	}

	onMount(async () => {
		await loadState();
		pollTimer = setInterval(async () => {
			if (setupState?.active_jobs?.length) {
				await refreshState();
			}
		}, 4000);
	});

	onDestroy(() => {
		if (pollTimer) clearInterval(pollTimer);
	});
</script>

<section class="bench-page" style="background:{pageBg}; color:{textPrimary};">
	<div class="hero">
		<div>
			<p class="eyebrow" style="color:{accent};">System Admin / Benchmark / Setup</p>
			<h1>Benchmark setup and operations</h1>
			<p class="lede" style="color:{textSecondary};">
				Configure the benchmark once, inspect which steps are already done, and run the remaining setup and benchmark operations from the browser.
			</p>
			{#if setupState?.last_experiment_id}
				<p class="lede" style="color:{muted};">Last experiment: <code>{setupState.last_experiment_id}</code></p>
			{/if}
		</div>
		<div class="hero-actions">
			<button class="hero-btn" style="border:1px solid {border}; color:{accent};" onclick={refreshState}>
				<RefreshCwIcon size={16} />
				<span>Refresh</span>
			</button>
			<button class="hero-btn" style="border:1px solid {border}; color:{accent};" disabled={runNextBusy} onclick={runNext}>
				<PlayIcon size={16} />
				<span>{runNextBusy ? 'Running…' : 'Run next unfinished step'}</span>
			</button>
		</div>
	</div>

	{#if error}
		<div class="alert" style="background:{cardBg}; border:1px solid {border}; color:{textPrimary};">{error}</div>
	{/if}

	{#if loading || !setupState || !draft}
		<div class="panel" style="background:{cardBg}; border:1px solid {border}; color:{textSecondary};">Loading benchmark setup state…</div>
	{:else}
		<div class="layout">
			<div class="main-column">
				<section class="panel" style="background:{cardBg}; border:1px solid {border};">
					<div class="panel-head">
						<div>
							<h2>Benchmark config</h2>
							<p style="color:{textSecondary};">These values are persisted by the server and used by validate, run, report, and compare.</p>
						</div>
						<button class="hero-btn" style="border:1px solid {border}; color:{accent};" disabled={saving} onclick={saveConfig}>
							<SaveIcon size={16} />
							<span>{saving ? 'Saving…' : 'Save config'}</span>
						</button>
					</div>
					<div class="config-grid">
						{#each configFields as field}
							<label class="field">
								<span style="color:{muted};">{field.label}</span>
								<input
									type={field.type ?? 'text'}
									value={String(draft[field.key] ?? '')}
									oninput={(e) => updateField(field.key, (e.currentTarget as HTMLInputElement).value)}
									style="border:1px solid {border}; background:{pageBg}; color:{textPrimary};"
								/>
							</label>
						{/each}
						<label class="field toggle">
							<span style="color:{muted};">Allow dirty working copy</span>
							<input
								type="checkbox"
								checked={draft.allow_dirty}
								onchange={(e) => updateField('allow_dirty', (e.currentTarget as HTMLInputElement).checked)}
							/>
						</label>
					</div>
				</section>

				<section class="section-block">
					<div class="section-head">
						<h2>Setup progress</h2>
						<p style="color:{textSecondary};">Section 8 setup checks and the benchmark operations flow rendered as one browser-driven sequence.</p>
					</div>
					<div class="step-stack">
							{#each setupState.steps as step (step.id)}
							<BenchmarkStepCard
								{step}
								{darkMode}
								busy={runningStepId === step.id}
								onRun={runStep}
							/>
						{/each}
					</div>
				</section>
			</div>

			<div class="side-column">
				<BenchmarkJobList jobs={setupState.recent_jobs} {darkMode} />
			</div>
		</div>
	{/if}
</section>

<style>
	.bench-page {
		padding: 24px;
		display: flex;
		flex-direction: column;
		gap: 18px;
	}
	.hero {
		display: flex;
		gap: 20px;
		justify-content: space-between;
		align-items: flex-start;
	}
	.hero h1, .panel-head h2, .section-head h2 {
		margin: 0;
	}
	.eyebrow, .lede, .panel-head p, .section-head p {
		margin: 0;
	}
	.eyebrow {
		font-size: 12px;
		font-weight: 700;
		letter-spacing: 0.08em;
		text-transform: uppercase;
		margin-bottom: 8px;
	}
	.hero h1 {
		font-size: 30px;
		line-height: 1.1;
		margin-bottom: 10px;
	}
	.lede {
		font-size: 14px;
		line-height: 1.6;
		max-width: 720px;
	}
	.hero-actions {
		display: flex;
		gap: 10px;
		flex-wrap: wrap;
	}
	.hero-btn {
		display: inline-flex;
		align-items: center;
		gap: 8px;
		border-radius: 10px;
		padding: 10px 12px;
		background: transparent;
		cursor: pointer;
		font-weight: 700;
	}
	.hero-btn:disabled {
		cursor: not-allowed;
		opacity: 0.7;
	}
	.alert, .panel {
		border-radius: 16px;
		padding: 18px;
	}
	.layout {
		display: grid;
		grid-template-columns: minmax(0, 1.6fr) minmax(320px, 0.9fr);
		gap: 18px;
		align-items: start;
	}
	.main-column, .side-column {
		display: flex;
		flex-direction: column;
		gap: 18px;
	}
	.panel-head, .section-head {
		display: flex;
		justify-content: space-between;
		gap: 16px;
		align-items: flex-start;
		margin-bottom: 14px;
	}
	.config-grid {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(220px, 1fr));
		gap: 14px;
	}
	.field {
		display: flex;
		flex-direction: column;
		gap: 7px;
		font-size: 13px;
	}
	.field input[type='text'],
	.field input[type='number'] {
		border-radius: 10px;
		padding: 10px 12px;
		outline: none;
	}
	.toggle {
		justify-content: end;
	}
	.step-stack {
		display: flex;
		flex-direction: column;
		gap: 14px;
	}
	@media (max-width: 1080px) {
		.layout {
			grid-template-columns: 1fr;
		}
		.hero {
			flex-direction: column;
		}
	}
</style>
