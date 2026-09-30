<script lang="ts">
	import { m as i18n } from '$lib/paraglide/messages.js';
	import { onMount } from 'svelte';
	import SettingsIcon from '@lucide/svelte/icons/settings';
	import VariableIcon from '@lucide/svelte/icons/variable';
	import StarIcon from '@lucide/svelte/icons/star';
	import Trash2Icon from '@lucide/svelte/icons/trash-2';
	import RefreshCwIcon from '@lucide/svelte/icons/refresh-cw';
	import StopCircleIcon from '@lucide/svelte/icons/stop-circle';
	import ModelsModal from '$lib/components/prompt-optimizer/models-modal.svelte';
	import VariablesModal from '$lib/components/prompt-optimizer/variables-modal.svelte';
	import {
		listModels,
		listTemplates,
		listHistory,
		deleteHistory,
		listFavorites,
		createFavorite,
		deleteFavorite,
		optimizeStream,
		type Model,
		type Template,
		type History,
		type Favorite,
		type OptimizeKind,
		type TemplateKind,
		type OptimizeRequest
	} from '$lib/components/prompt-optimizer/api';

	let { darkMode = true }: { darkMode: boolean } = $props();

	let pageBg        = $derived(darkMode ? '#171B26' : '#F2F4F7');
	let cardBg        = $derived(darkMode ? '#1F2333' : '#FFFFFF');
	let surface2      = $derived(darkMode ? '#252A3A' : '#ECEEF2');
	let borderColor   = $derived(darkMode ? '#2D3348' : '#E4E6EB');
	let accent        = $derived(darkMode ? '#818CF8' : '#6366F1');
	let accentTint    = $derived(darkMode ? 'rgba(129,140,248,0.15)' : 'rgba(99,102,241,0.10)');
	let textPrimary   = $derived(darkMode ? '#E2E8F0' : '#111827');
	let textSecondary = $derived(darkMode ? '#94A3B8' : '#6B7280');
	let textMuted     = $derived(darkMode ? '#64748B' : '#9CA3AF');
	let danger        = $derived(darkMode ? '#F87171' : '#DC2626');
	let dangerTint    = $derived(darkMode ? 'rgba(248,113,113,0.12)' : 'rgba(220,38,38,0.08)');
	let success       = '#10B981';

	type Tab = 'optimize' | 'history' | 'templates' | 'favorites';
	let activeTab = $state<Tab>('optimize');

	let models        = $state<Model[]>([]);
	let templates     = $state<Template[]>([]);
	let history       = $state<History[]>([]);
	let favorites     = $state<Favorite[]>([]);
	let favSet        = $derived(new Set(favorites.filter((f) => f.ref_kind === 'history').map((f) => f.ref_id)));

	let topError      = $state('');
	let loadingLists  = $state(false);

	let optKind       = $state<OptimizeKind>('optimize');
	let optModelId    = $state<string>('');
	let optTemplateId = $state<string>('');
	let origPrompt    = $state('');
	let feedback      = $state('');
	let testInput     = $state('');
	let varsOverlay   = $state('{}');
	let running       = $state(false);
	let output        = $state('');
	let lastHistoryId = $state('');
	let runError      = $state('');
	let abortCtrl: AbortController | null = null;

	let templatesForKind = $derived(
		templates.filter((t) => t.kind === (optKind as TemplateKind))
	);

	let showModels    = $state(false);
	let showVariables = $state(false);

	onMount(async () => {
		await refreshAll();
	});

	async function refreshAll() {
		loadingLists = true;
		topError = '';
		try {
			const [ms, ts, hs, fs] = await Promise.all([
				listModels(),
				listTemplates(),
				listHistory({ page: 1, page_size: 50 }),
				listFavorites()
			]);
			models = ms;
			templates = ts;
			history = hs.history;
			favorites = fs;

			if (!optModelId && models.length > 0) {
				const first = models.find((m) => m.enabled && m.has_api_key) ?? models[0];
				optModelId = first.id;
			}
		} catch (e) {
			topError = e instanceof Error ? e.message : String(e);
		} finally {
			loadingLists = false;
		}
	}

	async function runOptimize() {
		runError = '';
		output = '';
		lastHistoryId = '';

		if (!optModelId) { runError = i18n.prompt_optimizer_choose_a_model_first(); return; }
		if (optKind === 'optimize' || optKind === 'analyze') {
			if (!origPrompt.trim()) { runError = i18n.prompt_optimizer_original_prompt_is_required(); return; }
		}
		if (optKind === 'iterate') {
			if (!origPrompt.trim()) { runError = i18n.prompt_optimizer_base_prompt_is_required(); return; }
			if (!feedback.trim()) { runError = i18n.prompt_optimizer_feedback_is_required_for_iterate(); return; }
		}
		if (optKind === 'test') {
			if (!origPrompt.trim()) { runError = i18n.prompt_optimizer_prompt_under_test_is_required(); return; }
			if (!testInput.trim()) { runError = i18n.prompt_optimizer_test_input_is_required(); return; }
		}

		let parsedVars: Record<string, unknown> = {};
		const t = varsOverlay.trim();
		if (t) {
			try {
				const p = JSON.parse(t);
				if (p && typeof p === 'object' && !Array.isArray(p)) {
					parsedVars = p as Record<string, unknown>;
				} else {
					runError = i18n.prompt_optimizer_variables_overlay_must_be_a();
					return;
				}
			} catch {
				runError = i18n.prompt_optimizer_variables_overlay_is_not_valid();
				return;
			}
		}

		const req: OptimizeRequest = {
			kind: optKind,
			model_id: optModelId,
			template_id: optTemplateId || undefined,
			original_prompt: origPrompt.trim() || undefined,
			feedback: optKind === 'iterate' ? feedback : undefined,
			test_input: optKind === 'test' ? testInput : undefined,
			variables: Object.keys(parsedVars).length ? parsedVars : undefined,
			stream: true
		};

		running = true;
		abortCtrl = new AbortController();
		try {
			const res = await optimizeStream(req, {
				onDelta: (_, full) => { output = full; },
				onError: (msg) => { runError = msg; },
				signal: abortCtrl.signal
			});
			lastHistoryId = res.id;
			if (res.output) output = res.output;
			await refreshHistory();
		} catch (e) {
			if ((e as Error).name !== 'AbortError') {
				runError = e instanceof Error ? e.message : String(e);
			}
		} finally {
			running = false;
			abortCtrl = null;
		}
	}

	function cancelRun() {
		if (abortCtrl) abortCtrl.abort();
	}

	async function refreshHistory() {
		try {
			const hs = await listHistory({ page: 1, page_size: 50 });
			history = hs.history;
		} catch (e) {
			topError = e instanceof Error ? e.message : String(e);
		}
	}

	async function removeHistory(h: History) {
		if (!confirm(i18n.prompt_optimizer_delete_this_history_entry())) return;
		try {
			await deleteHistory(h.id);
			await refreshHistory();
			await refreshFavorites();
		} catch (e) {
			topError = e instanceof Error ? e.message : String(e);
		}
	}

	async function refreshFavorites() {
		try {
			favorites = await listFavorites();
		} catch (e) {
			topError = e instanceof Error ? e.message : String(e);
		}
	}

	async function toggleFavorite(h: History) {
		const existing = favorites.find((f) => f.ref_kind === 'history' && f.ref_id === h.id);
		try {
			if (existing) {
				await deleteFavorite(existing.id);
			} else {
				await createFavorite({ ref_kind: 'history', ref_id: h.id });
			}
			await refreshFavorites();
		} catch (e) {
			topError = e instanceof Error ? e.message : String(e);
		}
	}

	function loadFromHistory(h: History) {
		optKind = h.kind;
		if (h.model_id) optModelId = h.model_id;
		if (h.template_id) optTemplateId = h.template_id;
		origPrompt = h.original_prompt ?? '';
		testInput = h.test_input ?? '';
		feedback = '';
		output = h.optimized_prompt ?? h.test_output ?? '';
		lastHistoryId = h.id;
		activeTab = 'optimize';
	}

	function kindLabel(k: OptimizeKind): string {
		return ({
			optimize: i18n.prompt_optimizer_optimize(),
			iterate:  i18n.prompt_optimizer_iterate(),
			test:     i18n.prompt_optimizer_test(),
			analyze:  i18n.prompt_optimizer_analyze()
		} as const)[k];
	}

	function historyTitle(h: History): string {
		const src = h.optimized_prompt || h.original_prompt || h.test_input || '';
		return src.replace(/\s+/g, ' ').slice(0, 80) || i18n.prompt_optimizer_empty();
	}

	function historyPreview(h: History): string {
		const out = h.optimized_prompt || h.test_output || '';
		return out.replace(/\s+/g, ' ').slice(0, 160);
	}

	function formatDate(iso: string): string {
		try {
			return new Date(iso).toLocaleString();
		} catch {
			return iso;
		}
	}

	function openModels()    { showModels = true; }
	function openVariables() { showVariables = true; }

	async function onModelsClosed() {
		showModels = false;
		await refreshAll();
	}
	function onVariablesClosed() {
		showVariables = false;
	}
</script>

<div class="flex flex-col h-full overflow-y-auto p-6" style="background:{pageBg};">
	<div class="flex items-start justify-between gap-4 mb-6 flex-wrap">
		<div
			class="flex gap-1 p-1 rounded-xl"
			style="background:{surface2}; border:1px solid {borderColor}; width:fit-content;"
		>
			{#each [['optimize',i18n.prompt_optimizer_optimize()],['history',i18n.prompt_optimizer_history()],['templates',i18n.prompt_optimizer_templates()],['favorites',i18n.prompt_optimizer_favorites()]] as const as [id, label]}
				<button
					onclick={() => { activeTab = id as Tab; }}
					class="px-4 py-1.5 rounded-lg text-sm font-medium transition-colors duration-150 cursor-pointer"
					style="background:{activeTab === id ? accent : 'transparent'}; color:{activeTab === id ? 'white' : textSecondary}; border:none;"
				>
					{label}
				</button>
			{/each}
		</div>

		<div class="flex gap-2">
			<button
				onclick={openVariables}
				class="flex items-center gap-1.5 rounded-lg px-3 py-1.5 cursor-pointer"
				style="background:{accentTint}; color:{accent}; border:1px solid {accent}30; font-size:13px; font-weight:500;"
			>
				<VariableIcon class="w-3.5 h-3.5" />
				{i18n.prompt_optimizer_variables()}
			</button>
			<button
				onclick={openModels}
				class="flex items-center gap-1.5 rounded-lg px-3 py-1.5 cursor-pointer"
				style="background:{accentTint}; color:{accent}; border:1px solid {accent}30; font-size:13px; font-weight:500;"
			>
				<SettingsIcon class="w-3.5 h-3.5" />
				{i18n.prompt_optimizer_models()}
			</button>
			<button
				onclick={refreshAll}
				disabled={loadingLists}
				aria-label={i18n.prompt_optimizer_refresh()}
				class="flex items-center justify-center rounded-lg cursor-pointer"
				style="width:32px; height:32px; background:transparent; color:{textSecondary}; border:1px solid {borderColor};"
			>
				<RefreshCwIcon class="w-3.5 h-3.5" />
			</button>
		</div>
	</div>

	{#if topError}
		<div
			class="rounded-lg px-3 py-2 mb-4"
			style="background:{dangerTint}; color:{danger}; font-size:13px;"
		>
			{topError}
		</div>
	{/if}

	{#if activeTab === 'optimize'}
		<div class="grid gap-4" style="grid-template-columns: minmax(0, 1fr) minmax(0, 1fr);">
			<div
				class="rounded-xl p-5 flex flex-col gap-3"
				style="background:{cardBg}; border:1px solid {borderColor};"
			>
				<h2 style="font-size:15px; font-weight:600; color:{textPrimary};">{i18n.prompt_optimizer_input()}</h2>

				<div>
					<label for="po-kind" style="font-size:12px; color:{textSecondary}; font-weight:500;">{i18n.prompt_optimizer_kind()}</label>
					<select
						id="po-kind"
						bind:value={optKind}
						class="w-full rounded-lg px-3 py-2 mt-1"
						style="background:{surface2}; border:1px solid {borderColor}; color:{textPrimary}; font-size:13px;"
					>
						<option value="optimize">{i18n.prompt_optimizer_optimize_rewrite_a_prompt()}</option>
						<option value="iterate">{i18n.prompt_optimizer_iterate_refine_with_feedback()}</option>
						<option value="test">{i18n.prompt_optimizer_test_run_a_prompt_with()}</option>
						<option value="analyze">{i18n.prompt_optimizer_analyze_critique_a_prompt()}</option>
					</select>
				</div>

				<div class="grid grid-cols-2 gap-3">
					<div>
						<label for="po-model" style="font-size:12px; color:{textSecondary}; font-weight:500;">{i18n.prompt_optimizer_model()}</label>
						<select
							id="po-model"
							bind:value={optModelId}
							class="w-full rounded-lg px-3 py-2 mt-1"
							style="background:{surface2}; border:1px solid {borderColor}; color:{textPrimary}; font-size:13px;"
						>
							{#if models.length === 0}
								<option value="">{i18n.prompt_optimizer_no_models_add_one()}</option>
							{:else}
								{#each models as m (m.id)}
									<option value={m.id} disabled={!m.enabled || !m.has_api_key}>
										{m.name} ({m.provider}){m.has_api_key ? '' : i18n.prompt_optimizer_no_key()}
									</option>
								{/each}
							{/if}
						</select>
					</div>
					<div>
						<label for="po-template" style="font-size:12px; color:{textSecondary}; font-weight:500;">
							{i18n.prompt_optimizer_template()} <span style="color:{textMuted};">{i18n.prompt_optimizer_optional()}</span>
						</label>
						<select
							id="po-template"
							bind:value={optTemplateId}
							class="w-full rounded-lg px-3 py-2 mt-1"
							style="background:{surface2}; border:1px solid {borderColor}; color:{textPrimary}; font-size:13px;"
						>
							<option value="">{i18n.prompt_optimizer_default_for_kind()}</option>
							{#each templatesForKind as t (t.id)}
								<option value={t.id}>{t.name}{t.built_in ? i18n.prompt_optimizer_built_in() : ''}</option>
							{/each}
						</select>
					</div>
				</div>

				<div>
					<label for="po-orig" style="font-size:12px; color:{textSecondary}; font-weight:500;">
						{optKind === 'iterate' ? i18n.prompt_optimizer_base_prompt() : optKind === 'test' ? i18n.prompt_optimizer_prompt_under_test() : i18n.prompt_optimizer_original_prompt()}
					</label>
					<textarea
						id="po-orig"
						bind:value={origPrompt}
						rows="8"
						placeholder={i18n.prompt_optimizer_write_the_prompt_you_want()}
						class="w-full rounded-lg px-3 py-2 mt-1"
						style="background:{surface2}; border:1px solid {borderColor}; color:{textPrimary}; font-size:13px;"
					></textarea>
				</div>

				{#if optKind === 'iterate'}
					<div>
						<label for="po-feedback" style="font-size:12px; color:{textSecondary}; font-weight:500;">{i18n.prompt_optimizer_feedback()}</label>
						<textarea
							id="po-feedback"
							bind:value={feedback}
							rows="4"
							placeholder={i18n.prompt_optimizer_what_should_change_in_the()}
							class="w-full rounded-lg px-3 py-2 mt-1"
							style="background:{surface2}; border:1px solid {borderColor}; color:{textPrimary}; font-size:13px;"
						></textarea>
					</div>
				{/if}

				{#if optKind === 'test'}
					<div>
						<label for="po-test-input" style="font-size:12px; color:{textSecondary}; font-weight:500;">{i18n.prompt_optimizer_test_input()}</label>
						<textarea
							id="po-test-input"
							bind:value={testInput}
							rows="4"
							placeholder={i18n.prompt_optimizer_runtime_input_the_prompt_should()}
							class="w-full rounded-lg px-3 py-2 mt-1"
							style="background:{surface2}; border:1px solid {borderColor}; color:{textPrimary}; font-size:13px;"
						></textarea>
					</div>
				{/if}

				<details>
					<summary style="font-size:12px; color:{textSecondary}; cursor:pointer; user-select:none;">
						{i18n.prompt_optimizer_variable_overlay_json()} <span style="color:{textMuted};">{i18n.prompt_optimizer_overrides_saved_variables_for_this()}</span>
					</summary>
					<textarea
						bind:value={varsOverlay}
						rows="3"
						placeholder={'{ "tone": "concise" }'}
						class="w-full rounded-lg px-3 py-2 mt-1 font-mono"
						style="background:{surface2}; border:1px solid {borderColor}; color:{textPrimary}; font-size:12px;"
					></textarea>
				</details>

				{#if runError}
					<div
						class="rounded-lg px-3 py-2"
						style="background:{dangerTint}; color:{danger}; font-size:13px;"
					>
						{runError}
					</div>
				{/if}

				<div class="flex items-center gap-2">
					{#if running}
						<button
							onclick={cancelRun}
							class="flex items-center gap-1.5 rounded-lg px-4 py-2 cursor-pointer"
							style="background:{dangerTint}; color:{danger}; border:1px solid {danger}40; font-size:13px; font-weight:600;"
						>
							<StopCircleIcon class="w-3.5 h-3.5" />
							{i18n.prompt_optimizer_stop()}
						</button>
					{:else}
						<button
							onclick={runOptimize}
							class="rounded-lg px-4 py-2 cursor-pointer"
							style="background:{accent}; color:white; border:none; font-size:13px; font-weight:600;"
						>
							{i18n.prompt_optimizer_run({ optKind: kindLabel(optKind) })}
						</button>
					{/if}
					{#if lastHistoryId && !running}
						<span style="font-size:12px; color:{success};">{i18n.prompt_optimizer_saved_to_history()}</span>
					{/if}
				</div>
			</div>

			<div
				class="rounded-xl p-5 flex flex-col gap-3"
				style="background:{cardBg}; border:1px solid {borderColor}; min-height:400px;"
			>
				<div class="flex items-center justify-between">
					<h2 style="font-size:15px; font-weight:600; color:{textPrimary};">{i18n.prompt_optimizer_output()}</h2>
					{#if running}
						<span style="font-size:12px; color:{accent};">{i18n.prompt_optimizer_streaming()}</span>
					{/if}
				</div>
				{#if output}
					<div
						class="rounded-lg px-3 py-3 flex-1 overflow-y-auto"
						style="background:{surface2}; border:1px solid {borderColor}; color:{textPrimary}; font-size:13px; white-space:pre-wrap; word-break:break-word; min-height:320px;"
					>{output}</div>
				{:else}
					<div
						class="rounded-lg px-3 py-3 flex-1 flex items-center justify-center text-center"
						style="background:{surface2}; border:1px dashed {borderColor}; color:{textMuted}; font-size:13px; min-height:320px;"
					>
						{i18n.prompt_optimizer_output_will_stream_here_once()}
					</div>
				{/if}
			</div>
		</div>

	{:else if activeTab === 'history'}
		<div class="rounded-xl p-5" style="background:{cardBg}; border:1px solid {borderColor};">
			{#if loadingLists}
				<div style="color:{textMuted}; font-size:13px;">{i18n.prompt_optimizer_loading()}</div>
			{:else if history.length === 0}
				<div style="color:{textMuted}; font-size:13px;" class="text-center py-6">
					{i18n.prompt_optimizer_no_history_yet_run_something()}
				</div>
			{:else}
				<div class="space-y-2">
					{#each history as h (h.id)}
						<div
							class="rounded-lg px-4 py-3"
							style="background:{surface2}; border:1px solid {borderColor};"
						>
							<div class="flex items-start justify-between gap-3">
								<div class="flex-1 min-w-0">
									<div class="flex items-center gap-2 flex-wrap">
										<span
											class="rounded-full px-2"
											style="background:{accentTint}; color:{accent}; font-size:11px; font-weight:600;"
										>
											{kindLabel(h.kind)}
										</span>
										<span style="font-size:11px; color:{textMuted};">{formatDate(h.created_at)}</span>
									</div>
									<div style="font-size:13px; font-weight:500; color:{textPrimary}; margin-top:4px;" class="truncate">
										{historyTitle(h)}
									</div>
									<div style="font-size:12px; color:{textSecondary}; margin-top:2px;" class="line-clamp-2">
										{historyPreview(h)}
									</div>
								</div>
								<div class="flex items-center gap-1 flex-shrink-0">
									<button
										onclick={() => toggleFavorite(h)}
										aria-label={favSet.has(h.id) ? i18n.prompt_optimizer_unfavorite() : i18n.prompt_optimizer_favorite()}
										class="rounded-lg p-1.5 cursor-pointer"
										style="background:transparent; color:{favSet.has(h.id) ? accent : textSecondary}; border:1px solid {borderColor};"
									>
										<StarIcon class="w-3.5 h-3.5" style={favSet.has(h.id) ? `fill:${accent};` : ''} />
									</button>
									<button
										onclick={() => loadFromHistory(h)}
										class="rounded-lg px-2 py-1 cursor-pointer"
										style="background:transparent; color:{textSecondary}; border:1px solid {borderColor}; font-size:12px;"
									>
										{i18n.prompt_optimizer_load()}
									</button>
									<button
										onclick={() => removeHistory(h)}
										aria-label={i18n.prompt_optimizer_delete()}
										class="rounded-lg p-1.5 cursor-pointer"
										style="background:transparent; color:{danger}; border:1px solid {borderColor};"
									>
										<Trash2Icon class="w-3.5 h-3.5" />
									</button>
								</div>
							</div>
						</div>
					{/each}
				</div>
			{/if}
		</div>

	{:else if activeTab === 'templates'}
		<div class="rounded-xl p-5" style="background:{cardBg}; border:1px solid {borderColor};">
			{#if loadingLists}
				<div style="color:{textMuted}; font-size:13px;">{i18n.prompt_optimizer_loading()}</div>
			{:else if templates.length === 0}
				<div style="color:{textMuted}; font-size:13px;" class="text-center py-6">
					{i18n.prompt_optimizer_no_templates_available()}
				</div>
			{:else}
				<div class="space-y-2">
					{#each templates as t (t.id)}
						<div
							class="rounded-lg px-4 py-3"
							style="background:{surface2}; border:1px solid {borderColor};"
						>
							<div class="flex items-center gap-2 flex-wrap">
								<span style="font-size:14px; font-weight:600; color:{textPrimary};">{t.name}</span>
								<span
									class="rounded-full px-2"
									style="background:{accentTint}; color:{accent}; font-size:11px; font-weight:600;"
								>
									{t.kind}
								</span>
								{#if t.built_in}
									<span
										class="rounded-full px-2"
										style="background:{borderColor}; color:{textMuted}; font-size:11px; font-weight:600;"
									>
										{i18n.prompt_optimizer_built_in_2()}
									</span>
								{/if}
							</div>
							{#if t.description}
								<div style="font-size:12px; color:{textSecondary}; margin-top:4px;">{t.description}</div>
							{/if}
							{#if t.variables?.length}
								<div style="font-size:11px; color:{textMuted}; margin-top:4px;">
									{i18n.prompt_optimizer_variables_2({ variables: t.variables.join(', ') })}
								</div>
							{/if}
						</div>
					{/each}
				</div>
			{/if}
		</div>

	{:else if activeTab === 'favorites'}
		<div class="rounded-xl p-5" style="background:{cardBg}; border:1px solid {borderColor};">
			{#if loadingLists}
				<div style="color:{textMuted}; font-size:13px;">{i18n.prompt_optimizer_loading()}</div>
			{:else if favorites.length === 0}
				<div style="color:{textMuted}; font-size:13px;" class="text-center py-6">
					{i18n.prompt_optimizer_no_favorites_yet_star_a()}
				</div>
			{:else}
				<div class="space-y-2">
					{#each favorites as f (f.id)}
						{@const h = history.find((x) => x.id === f.ref_id)}
						<div
							class="rounded-lg px-4 py-3 flex items-start justify-between gap-3"
							style="background:{surface2}; border:1px solid {borderColor};"
						>
							<div class="flex-1 min-w-0">
								<div class="flex items-center gap-2 flex-wrap">
									<span
										class="rounded-full px-2"
										style="background:{accentTint}; color:{accent}; font-size:11px; font-weight:600;"
									>
										{f.ref_kind}
									</span>
									<span style="font-size:11px; color:{textMuted};">{formatDate(f.created_at)}</span>
								</div>
								{#if h}
									<div style="font-size:13px; font-weight:500; color:{textPrimary}; margin-top:4px;" class="truncate">
										{historyTitle(h)}
									</div>
									<div style="font-size:12px; color:{textSecondary}; margin-top:2px;" class="line-clamp-2">
										{historyPreview(h)}
									</div>
								{:else}
									<div style="font-size:12px; color:{textMuted}; margin-top:4px;">
										{i18n.prompt_optimizer_ref({ ref_id: f.ref_id })}
									</div>
								{/if}
							</div>
							<button
								onclick={async () => {
									try { await deleteFavorite(f.id); await refreshFavorites(); }
									catch (e) { topError = e instanceof Error ? e.message : String(e); }
								}}
								aria-label={i18n.prompt_optimizer_remove_favorite()}
								class="rounded-lg p-1.5 cursor-pointer flex-shrink-0"
								style="background:transparent; color:{textSecondary}; border:1px solid {borderColor};"
							>
								<Trash2Icon class="w-3.5 h-3.5" />
							</button>
						</div>
					{/each}
				</div>
			{/if}
		</div>
	{/if}
</div>

<ModelsModal    {darkMode} open={showModels}    onClose={onModelsClosed} />
<VariablesModal {darkMode} open={showVariables} onClose={onVariablesClosed} />

<style>
	.line-clamp-2 {
		display: -webkit-box;
		-webkit-line-clamp: 2;
		-webkit-box-orient: vertical;
		overflow: hidden;
	}
</style>
