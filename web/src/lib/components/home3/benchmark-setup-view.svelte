<script lang="ts">
	import { onDestroy, onMount } from 'svelte';
	import CircleHelpIcon from '@lucide/svelte/icons/circle-help';
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

	type FieldOption = {
		value: string;
		label: string;
		disabled?: boolean;
	};

	type ConfigField = {
		key: keyof BenchmarkConfig;
		label: string;
		type?: 'text' | 'number' | 'select';
		placeholder?: string;
		help: {
			purpose: string;
			valid: string;
			recommended: string;
		};
		options?: FieldOption[];
	};

	const configFields: ConfigField[] = [
		{
			key: 'experiment_path',
			label: 'Experiment path',
			placeholder: 'benchmark/doc-processors/experiments/example-20260717-clean.toml',
			help: {
				purpose: 'The experiment TOML that defines processors, variants, repetitions, and compare targets.',
				valid: 'Any readable path on the ChenWeb server. Relative paths are resolved from the ChenWeb repo root.',
				recommended: 'Use the checked-in clean example experiment first, then switch to a dedicated experiment file when you create a new benchmark run.'
			}
		},
		{
			key: 'dataset_root',
			label: 'Dataset root',
			placeholder: 'benchmark/doc-processors/datasets',
			help: {
				purpose: 'The root folder that contains benchmark datasets and versions.',
				valid: 'A readable directory containing dataset folders such as doc-processors-synthetic-core/1.0.0.',
				recommended: 'Keep the default benchmark/doc-processors/datasets unless you intentionally maintain an alternate dataset tree.'
			}
		},
		{
			key: 'artifact_root',
			label: 'Artifact root',
			placeholder: 'Data/kb/artifacts',
			help: {
				purpose: 'The production artifact root used by benchmarked processors during execution.',
				valid: 'A writable directory. This should match the effective ARTIFACT_DIR used by the production runtime.',
				recommended: 'Point it at the same artifact root your document processor already uses, then let benchmark outputs live under a doc-benchmark subdirectory.'
			}
		},
		{
			key: 'work_root',
			label: 'Work root',
			placeholder: '.benchmark/work',
			help: {
				purpose: 'Disposable workspace storage for temporary benchmark execution files.',
				valid: 'A writable directory that does not overlap the evidence root.',
				recommended: 'Use .benchmark/work under the ChenWeb repo unless your environment requires a different scratch location.'
			}
		},
		{
			key: 'evidence_root',
			label: 'Evidence root',
			placeholder: '.benchmark/evidence',
			help: {
				purpose: 'Immutable captured evidence for benchmark attempts, reports, diagnostics, and provenance.',
				valid: 'A writable directory that does not overlap the work root.',
				recommended: 'Use .benchmark/evidence under the ChenWeb repo for local work and keep it separate from disposable workspace files.'
			}
		},
		{
			key: 'store_id',
			label: 'Store ID',
			type: 'number',
			help: {
				purpose: 'The knowledge-store ID used when seeding temporary benchmark input rows.',
				valid: 'Any numeric store ID that exists in the target environment.',
				recommended: 'Use 1 unless your environment has a dedicated benchmark store that should isolate these runs.'
			}
		},
		{
			key: 'owner',
			label: 'Owner',
			placeholder: 'benchmark-admin',
			help: {
				purpose: 'A label used for benchmark leases and job ownership metadata.',
				valid: 'Any short identifier string.',
				recommended: 'Use a stable machine or operator name so job ownership and lease recovery are easy to trace.'
			}
		},
		{
			key: 'tenant_id',
			label: 'Tenant ID',
			placeholder: 'benchmark',
			help: {
				purpose: 'The tenant label used by the benchmark runner when creating temporary benchmark inputs.',
				valid: 'Any non-empty tenant identifier recognized by your runtime conventions.',
				recommended: 'Keep benchmark unless you intentionally isolate benchmark traffic under another tenant label.'
			}
		},
		{
			key: 'metrics_model_name',
			label: 'Metrics model name',
			placeholder: 'deepseek-flash-chen',
			help: {
				purpose: 'The model reference injected into the metrics benchmark variant through DOC_BENCHMARK_METRICS_MODEL_NAME.',
				valid: 'A model name that resolves correctly in the local .models.toml configuration.',
				recommended: 'Use a known local model ref that already works for extract_metrics in your environment.'
			}
		},
		{
			key: 'report_format',
			label: 'Report format',
			type: 'select',
			options: [
				{ value: 'markdown', label: 'Markdown' },
				{ value: 'typst', label: 'Typst (Planned)', disabled: true }
			],
			help: {
				purpose: 'The output format used when generating benchmark reports and compare outputs from stored results.',
				valid: 'Markdown is supported today. Typst is planned but not implemented yet.',
				recommended: 'Use Markdown for now. Typst is shown to document the intended future export option.'
			}
		},
		{
			key: 'report_output_path',
			label: 'Report output path',
			placeholder: 'Data/kb/artifacts/doc-benchmark/report-<experiment-id>.md',
			help: {
				purpose: 'An optional explicit output path for the main benchmark report.',
				valid: 'Any writable file path. If empty, the server generates a default path under the artifact root.',
				recommended: 'Leave it blank at first and let the system write to the default report path under the benchmark artifact directory.'
			}
		},
		{
			key: 'metrics_baseline',
			label: 'Metrics baseline',
			type: 'select',
			options: [
				{ value: 'metrics-baseline', label: 'metrics-baseline' },
				{ value: 'metrics-alt', label: 'metrics-alt' }
			],
			help: {
				purpose: 'The baseline variant used when generating the metrics compare report.',
				valid: 'A variant name that exists in the configured experiment.',
				recommended: 'Use metrics-baseline as the stable reference variant and compare metrics-alt against it.'
			}
		},
		{
			key: 'metrics_candidate',
			label: 'Metrics candidate',
			type: 'select',
			options: [
				{ value: 'metrics-baseline', label: 'metrics-baseline' },
				{ value: 'metrics-alt', label: 'metrics-alt' }
			],
			help: {
				purpose: 'The candidate variant used when generating the metrics compare report.',
				valid: 'A variant name that exists in the configured experiment.',
				recommended: 'Use metrics-alt as the candidate when comparing prompt or model changes against metrics-baseline.'
			}
		},
		{
			key: 'chunk_baseline',
			label: 'Chunk baseline',
			type: 'select',
			options: [
				{ value: 'chunk-small', label: 'chunk-small' },
				{ value: 'chunk-large', label: 'chunk-large' }
			],
			help: {
				purpose: 'The baseline variant used when generating the chunking compare report.',
				valid: 'A variant name that exists in the configured experiment.',
				recommended: 'Use chunk-small as the baseline if that is your current stable chunking configuration.'
			}
		},
		{
			key: 'chunk_candidate',
			label: 'Chunk candidate',
			type: 'select',
			options: [
				{ value: 'chunk-small', label: 'chunk-small' },
				{ value: 'chunk-large', label: 'chunk-large' }
			],
			help: {
				purpose: 'The candidate variant used when generating the chunking compare report.',
				valid: 'A variant name that exists in the configured experiment.',
				recommended: 'Use chunk-large when testing whether larger chunk sizing helps or hurts quality and efficiency.'
			}
		}
	];

	const allowDirtyHelp = {
		purpose: 'Controls whether the benchmark runner may proceed when the current jj working copy is dirty.',
		valid: 'Checked or unchecked.',
		recommended: 'Leave it unchecked for reproducible benchmark runs. Enable it only for exploratory browser testing when a dirty working copy is acceptable.'
	};

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
								<span class="field-label" style="color:{muted};">
									<span>{field.label}</span>
									<button
										type="button"
										class="help-tip"
										aria-label={`${field.label} help`}
									>
										<CircleHelpIcon size={14} />
										<span class="tooltip" style="background:{pageBg}; border:1px solid {border}; color:{textPrimary};">
											<strong>{field.label}</strong>
											<span><b>Purpose:</b> {field.help.purpose}</span>
											<span><b>Valid values:</b> {field.help.valid}</span>
											<span><b>Recommended:</b> {field.help.recommended}</span>
										</span>
									</button>
								</span>
								{#if field.type === 'select' && field.options}
									<select
										value={String(draft[field.key] ?? '')}
										onchange={(e) => updateField(field.key, (e.currentTarget as HTMLSelectElement).value)}
										style="border:1px solid {border}; background:{pageBg}; color:{textPrimary};"
									>
										{#each field.options as option}
											<option value={option.value} disabled={option.disabled}>
												{option.label}
											</option>
										{/each}
									</select>
								{:else}
									<input
										type={field.type ?? 'text'}
										value={String(draft[field.key] ?? '')}
										placeholder={field.placeholder}
										oninput={(e) => updateField(field.key, (e.currentTarget as HTMLInputElement).value)}
										style="border:1px solid {border}; background:{pageBg}; color:{textPrimary};"
									/>
								{/if}
							</label>
						{/each}
						<label class="field toggle">
							<span class="field-label" style="color:{muted};">
								<span>Allow dirty working copy</span>
								<button type="button" class="help-tip" aria-label="Allow dirty working copy help">
									<CircleHelpIcon size={14} />
									<span class="tooltip" style="background:{pageBg}; border:1px solid {border}; color:{textPrimary};">
										<strong>Allow dirty working copy</strong>
										<span><b>Purpose:</b> {allowDirtyHelp.purpose}</span>
										<span><b>Valid values:</b> {allowDirtyHelp.valid}</span>
										<span><b>Recommended:</b> {allowDirtyHelp.recommended}</span>
									</span>
								</button>
							</span>
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
	.field-label {
		display: inline-flex;
		align-items: center;
		gap: 6px;
	}
	.help-tip {
		position: relative;
		display: inline-flex;
		align-items: center;
		color: inherit;
		cursor: help;
		outline: none;
		padding: 0;
		border: 0;
		background: transparent;
	}
	.tooltip {
		position: absolute;
		left: 0;
		top: calc(100% + 10px);
		z-index: 10;
		width: min(320px, 60vw);
		display: none;
		flex-direction: column;
		gap: 6px;
		padding: 10px 12px;
		border-radius: 12px;
		font-size: 12px;
		line-height: 1.5;
		box-shadow: 0 18px 45px rgba(0, 0, 0, 0.28);
	}
	.help-tip:hover .tooltip,
	.help-tip:focus .tooltip,
	.help-tip:focus-within .tooltip {
		display: flex;
	}
	.field input[type='text'],
	.field input[type='number'],
	.field select {
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
