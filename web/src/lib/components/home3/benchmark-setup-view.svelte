<script lang="ts">
	import { m } from '$lib/paraglide/messages.js';
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
	// Which field's help tooltip is currently open. Driven by explicit pointer/focus
	// events on the icon (not CSS :hover) because the CSS :hover flag can get stuck,
	// leaving the tooltip visible while the cursor is nowhere near the icon.
	let openHelp = $state<string | null>(null);

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
			label: m.benchmark_setup_experiment_path(),
			placeholder: 'benchmark/doc-processors/experiments/example-20260717-clean.toml',
			help: {
				purpose: m.benchmark_setup_the_experiment_toml_that_defines(),
				valid: m.benchmark_setup_any_readable_path_on_the(),
				recommended: m.benchmark_setup_use_the_checked_in_clean()
			}
		},
		{
			key: 'dataset_root',
			label: m.benchmark_setup_dataset_root(),
			placeholder: 'benchmark/doc-processors/datasets',
			help: {
				purpose: m.benchmark_setup_the_root_folder_that_contains(),
				valid: m.benchmark_setup_a_readable_directory_containing_dataset(),
				recommended: m.benchmark_setup_keep_the_default_benchmark_doc()
			}
		},
		{
			key: 'artifact_root',
			label: m.benchmark_setup_artifact_root(),
			placeholder: 'Data/kb/artifacts',
			help: {
				purpose: m.benchmark_setup_the_production_artifact_root_used(),
				valid: m.benchmark_setup_a_writable_directory_this_should(),
				recommended: m.benchmark_setup_point_it_at_the_same()
			}
		},
		{
			key: 'work_root',
			label: m.benchmark_setup_work_root(),
			placeholder: '.benchmark/work',
			help: {
				purpose: m.benchmark_setup_disposable_workspace_storage_for_temporary(),
				valid: m.benchmark_setup_a_writable_directory_that_does(),
				recommended: m.benchmark_setup_use_benchmark_work_under_the()
			}
		},
		{
			key: 'evidence_root',
			label: m.benchmark_setup_evidence_root(),
			placeholder: '.benchmark/evidence',
			help: {
				purpose: m.benchmark_setup_immutable_captured_evidence_for_benchmark(),
				valid: m.benchmark_setup_a_writable_directory_that_does_2(),
				recommended: m.benchmark_setup_use_benchmark_evidence_under_the()
			}
		},
		{
			key: 'store_id',
			label: m.benchmark_setup_store_id(),
			type: 'number',
			help: {
				purpose: m.benchmark_setup_the_knowledge_store_id_used(),
				valid: m.benchmark_setup_any_numeric_store_id_that(),
				recommended: m.benchmark_setup_use_1_unless_your_environment()
			}
		},
		{
			key: 'owner',
			label: m.benchmark_setup_owner(),
			placeholder: 'benchmark-admin',
			help: {
				purpose: m.benchmark_setup_a_label_used_for_benchmark(),
				valid: m.benchmark_setup_any_short_identifier_string(),
				recommended: m.benchmark_setup_use_a_stable_machine_or()
			}
		},
		{
			key: 'tenant_id',
			label: m.benchmark_setup_tenant_id(),
			placeholder: 'benchmark',
			help: {
				purpose: m.benchmark_setup_the_tenant_label_used_by(),
				valid: m.benchmark_setup_any_non_empty_tenant_identifier(),
				recommended: m.benchmark_setup_keep_benchmark_unless_you_intentionally()
			}
		},
		{
			key: 'metrics_model_name',
			label: m.benchmark_setup_metrics_model_name(),
			placeholder: 'deepseek-flash-chen',
			help: {
				purpose: m.benchmark_setup_the_model_reference_injected_into(),
				valid: m.benchmark_setup_a_model_name_that_resolves(),
				recommended: m.benchmark_setup_use_a_known_local_model()
			}
		},
		{
			key: 'report_format',
			label: m.benchmark_setup_report_format(),
			type: 'select',
			options: [
				{ value: 'markdown', label: 'Markdown' },
				{ value: 'typst', label: m.benchmark_setup_typst_planned(), disabled: true }
			],
			help: {
				purpose: m.benchmark_setup_the_output_format_used_when(),
				valid: m.benchmark_setup_markdown_is_supported_today_typst(),
				recommended: m.benchmark_setup_use_markdown_for_now_typst()
			}
		},
		{
			key: 'report_output_path',
			label: m.benchmark_setup_report_output_path(),
			placeholder: 'Data/kb/artifacts/doc-benchmark/report-<experiment-id>.md',
			help: {
				purpose: m.benchmark_setup_an_optional_explicit_output_path(),
				valid: 'Any writable file path. If empty, the server generates a default path under the artifact root.',
				recommended: m.benchmark_setup_leave_it_blank_at_first()
			}
		},
		{
			key: 'metrics_baseline',
			label: m.benchmark_setup_metrics_baseline(),
			type: 'select',
			options: [
				{ value: 'metrics-baseline', label: 'metrics-baseline' },
				{ value: 'metrics-alt', label: 'metrics-alt' }
			],
			help: {
				purpose: m.benchmark_setup_the_baseline_variant_used_when(),
				valid: m.benchmark_setup_a_variant_name_that_exists(),
				recommended: m.benchmark_setup_use_metrics_baseline_as_the()
			}
		},
		{
			key: 'metrics_candidate',
			label: m.benchmark_setup_metrics_candidate(),
			type: 'select',
			options: [
				{ value: 'metrics-baseline', label: 'metrics-baseline' },
				{ value: 'metrics-alt', label: 'metrics-alt' }
			],
			help: {
				purpose: m.benchmark_setup_the_candidate_variant_used_when(),
				valid: m.benchmark_setup_a_variant_name_that_exists(),
				recommended: m.benchmark_setup_use_metrics_alt_as_the()
			}
		},
		{
			key: 'chunk_baseline',
			label: m.benchmark_setup_chunk_baseline(),
			type: 'select',
			options: [
				{ value: 'chunk-small', label: 'chunk-small' },
				{ value: 'chunk-large', label: 'chunk-large' }
			],
			help: {
				purpose: m.benchmark_setup_the_baseline_variant_used_when_2(),
				valid: m.benchmark_setup_a_variant_name_that_exists(),
				recommended: m.benchmark_setup_use_chunk_small_as_the()
			}
		},
		{
			key: 'chunk_candidate',
			label: m.benchmark_setup_chunk_candidate(),
			type: 'select',
			options: [
				{ value: 'chunk-small', label: 'chunk-small' },
				{ value: 'chunk-large', label: 'chunk-large' }
			],
			help: {
				purpose: m.benchmark_setup_the_candidate_variant_used_when_2(),
				valid: m.benchmark_setup_a_variant_name_that_exists(),
				recommended: m.benchmark_setup_use_chunk_large_when_testing()
			}
		}
	];

	const allowDirtyHelp = {
		purpose: m.benchmark_setup_controls_whether_the_benchmark_runner(),
		valid: m.benchmark_setup_checked_or_unchecked(),
		recommended: m.benchmark_setup_leave_it_unchecked_for_reproducible()
	};

	async function loadState() {
		loading = true;
		error = '';
		try {
			setupState = await getBenchmarkSetupState();
			draft = { ...setupState.config };
		} catch (e) {
			error = e instanceof Error ? e.message : m.benchmark_setup_failed_to_load_benchmark_setup();
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
			error = e instanceof Error ? e.message : m.benchmark_setup_failed_to_refresh_benchmark_setup();
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
			error = e instanceof Error ? e.message : m.benchmark_setup_failed_to_save_benchmark_config();
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
			error = e instanceof Error ? e.message : m.benchmark_setup_failed_to_run_benchmark_step();
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
			error = e instanceof Error ? e.message : m.benchmark_setup_failed_to_run_the_next();
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
			<p class="eyebrow" style="color:{accent};">{m.benchmark_setup_system_admin_benchmark_setup()}</p>
			<h1>{m.benchmark_setup_benchmark_setup_and_operations()}</h1>
			<p class="lede" style="color:{textSecondary};">
				{m.benchmark_setup_configure_the_benchmark_once_inspect()}
			</p>
			{#if setupState?.last_experiment_id}
				<p class="lede" style="color:{muted};">{m.benchmark_setup_last_experiment()} <code>{setupState.last_experiment_id}</code></p>
			{/if}
		</div>
		<div class="hero-actions">
			<button class="hero-btn" style="border:1px solid {border}; color:{accent};" onclick={refreshState}>
				<RefreshCwIcon size={16} />
				<span>{m.benchmark_setup_refresh()}</span>
			</button>
			<button class="hero-btn" style="border:1px solid {border}; color:{accent};" disabled={runNextBusy} onclick={runNext}>
				<PlayIcon size={16} />
				<span>{runNextBusy ? m.benchmark_setup_running() : m.benchmark_setup_run_next_unfinished_step()}</span>
			</button>
		</div>
	</div>

	{#if error}
		<div class="alert" style="background:{cardBg}; border:1px solid {border}; color:{textPrimary};">{error}</div>
	{/if}

	{#if loading || !setupState || !draft}
		<div class="panel" style="background:{cardBg}; border:1px solid {border}; color:{textSecondary};">{m.benchmark_setup_loading_benchmark_setup_state()}</div>
	{:else}
		<div class="layout">
			<div class="main-column">
				<section class="panel" style="background:{cardBg}; border:1px solid {border};">
					<div class="panel-head">
						<div>
							<h2>{m.benchmark_setup_benchmark_config()}</h2>
							<p style="color:{textSecondary};">{m.benchmark_setup_these_values_are_persisted_by()}</p>
						</div>
						<button class="hero-btn" style="border:1px solid {border}; color:{accent};" disabled={saving} onclick={saveConfig}>
							<SaveIcon size={16} />
							<span>{saving ? m.benchmark_setup_saving() : m.benchmark_setup_save_config()}</span>
						</button>
					</div>
					<div class="config-grid">
						{#each configFields as field}
							<label class="field">
								<span class="field-label" style="color:{muted};">
									<span>{field.label}</span>
									<span class="help-anchor">
										<button
											type="button"
											class="help-tip"
											aria-label={`${field.label} help`}
											onmouseenter={() => (openHelp = String(field.key))}
											onmouseleave={() => { if (openHelp === String(field.key)) openHelp = null; }}
											onfocus={() => (openHelp = String(field.key))}
											onblur={() => { if (openHelp === String(field.key)) openHelp = null; }}
										>
											<CircleHelpIcon size={14} />
										</button>
										<span class="tooltip" class:open={openHelp === String(field.key)} style="background:{pageBg}; border:1px solid {border}; color:{textPrimary};">
											<strong>{field.label}</strong>
											<span><b>{m.benchmark_setup_purpose()}</b> {field.help.purpose}</span>
											<span><b>{m.benchmark_setup_valid_values()}</b> {field.help.valid}</span>
											<span><b>{m.benchmark_setup_recommended()}</b> {field.help.recommended}</span>
										</span>
									</span>
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
								<span>{m.benchmark_setup_allow_dirty_working_copy()}</span>
								<span class="help-anchor">
									<button
										type="button"
										class="help-tip"
										aria-label={m.benchmark_setup_allow_dirty_working_copy_help()}
										onmouseenter={() => (openHelp = 'allow_dirty')}
										onmouseleave={() => { if (openHelp === 'allow_dirty') openHelp = null; }}
										onfocus={() => (openHelp = 'allow_dirty')}
										onblur={() => { if (openHelp === 'allow_dirty') openHelp = null; }}
									>
										<CircleHelpIcon size={14} />
									</button>
									<span class="tooltip" class:open={openHelp === 'allow_dirty'} style="background:{pageBg}; border:1px solid {border}; color:{textPrimary};">
										<strong>{m.benchmark_setup_allow_dirty_working_copy()}</strong>
										<span><b>{m.benchmark_setup_purpose()}</b> {allowDirtyHelp.purpose}</span>
										<span><b>{m.benchmark_setup_valid_values()}</b> {allowDirtyHelp.valid}</span>
										<span><b>{m.benchmark_setup_recommended()}</b> {allowDirtyHelp.recommended}</span>
									</span>
								</span>
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
						<h2>{m.benchmark_setup_setup_progress()}</h2>
						<p style="color:{textSecondary};">{m.benchmark_setup_section_8_setup_checks_and()}</p>
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
		display: inline-flex;
		align-items: center;
		justify-content: center;
		color: inherit;
		cursor: help;
		outline: none;
		padding: 0;
		border: 0;
		background: transparent;
		width: 14px;
		height: 14px;
		line-height: 1;
		flex: 0 0 auto;
	}
	.help-anchor {
		position: relative;
		display: inline-flex;
		width: 14px;
		height: 14px;
		flex: 0 0 auto;
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
		pointer-events: none;
	}
	.tooltip.open {
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
