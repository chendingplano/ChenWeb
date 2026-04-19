<script lang="ts">
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

		if (!optModelId) { runError = 'Choose a model first'; return; }
		if (optKind === 'optimize' || optKind === 'analyze') {
			if (!origPrompt.trim()) { runError = 'Original prompt is required'; return; }
		}
		if (optKind === 'iterate') {
			if (!origPrompt.trim()) { runError = 'Base prompt is required'; return; }
			if (!feedback.trim()) { runError = 'Feedback is required for iterate'; return; }
		}
		if (optKind === 'test') {
			if (!origPrompt.trim()) { runError = 'Prompt under test is required'; return; }
			if (!testInput.trim()) { runError = 'Test input is required'; return; }
		}

		let parsedVars: Record<string, unknown> = {};
		const t = varsOverlay.trim();
		if (t) {
			try {
				const p = JSON.parse(t);
				if (p && typeof p === 'object' && !Array.isArray(p)) {
					parsedVars = p as Record<string, unknown>;
				} else {
					runError = 'Variables overlay must be a JSON object';
					return;
				}
			} catch {
				runError = 'Variables overlay is not valid JSON';
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
		if (!confirm('Delete this history entry?')) return;
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
			optimize: 'Optimize',
			iterate:  'Iterate',
			test:     'Test',
			analyze:  'Analyze'
		} as const)[k];
	}

	function historyTitle(h: History): string {
		const src = h.optimized_prompt || h.original_prompt || h.test_input || '';
		return src.replace(/\s+/g, ' ').slice(0, 80) || '(empty)';
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
			{#each [['optimize','Optimize'],['history','History'],['templates','Templates'],['favorites','Favorites']] as const as [id, label]}
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
				Variables
			</button>
			<button
				onclick={openModels}
				class="flex items-center gap-1.5 rounded-lg px-3 py-1.5 cursor-pointer"
				style="background:{accentTint}; color:{accent}; border:1px solid {accent}30; font-size:13px; font-weight:500;"
			>
				<SettingsIcon class="w-3.5 h-3.5" />
				Models
			</button>
			<button
				onclick={refreshAll}
				disabled={loadingLists}
				aria-label="Refresh"
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
				<h2 style="font-size:15px; font-weight:600; color:{textPrimary};">Input</h2>

				<div>
					<label for="po-kind" style="font-size:12px; color:{textSecondary}; font-weight:500;">Kind</label>
					<select
						id="po-kind"
						bind:value={optKind}
						class="w-full rounded-lg px-3 py-2 mt-1"
						style="background:{surface2}; border:1px solid {borderColor}; color:{textPrimary}; font-size:13px;"
					>
						<option value="optimize">Optimize — rewrite a prompt</option>
						<option value="iterate">Iterate — refine with feedback</option>
						<option value="test">Test — run a prompt with input</option>
						<option value="analyze">Analyze — critique a prompt</option>
					</select>
				</div>

				<div class="grid grid-cols-2 gap-3">
					<div>
						<label for="po-model" style="font-size:12px; color:{textSecondary}; font-weight:500;">Model</label>
						<select
							id="po-model"
							bind:value={optModelId}
							class="w-full rounded-lg px-3 py-2 mt-1"
							style="background:{surface2}; border:1px solid {borderColor}; color:{textPrimary}; font-size:13px;"
						>
							{#if models.length === 0}
								<option value="">(no models — add one)</option>
							{:else}
								{#each models as m (m.id)}
									<option value={m.id} disabled={!m.enabled || !m.has_api_key}>
										{m.name} ({m.provider}){m.has_api_key ? '' : ' — no key'}
									</option>
								{/each}
							{/if}
						</select>
					</div>
					<div>
						<label for="po-template" style="font-size:12px; color:{textSecondary}; font-weight:500;">
							Template <span style="color:{textMuted};">(optional)</span>
						</label>
						<select
							id="po-template"
							bind:value={optTemplateId}
							class="w-full rounded-lg px-3 py-2 mt-1"
							style="background:{surface2}; border:1px solid {borderColor}; color:{textPrimary}; font-size:13px;"
						>
							<option value="">(default for kind)</option>
							{#each templatesForKind as t (t.id)}
								<option value={t.id}>{t.name}{t.built_in ? ' · built-in' : ''}</option>
							{/each}
						</select>
					</div>
				</div>

				<div>
					<label for="po-orig" style="font-size:12px; color:{textSecondary}; font-weight:500;">
						{optKind === 'iterate' ? 'Base prompt' : optKind === 'test' ? 'Prompt under test' : 'Original prompt'}
					</label>
					<textarea
						id="po-orig"
						bind:value={origPrompt}
						rows="8"
						placeholder="Write the prompt you want to work with…"
						class="w-full rounded-lg px-3 py-2 mt-1"
						style="background:{surface2}; border:1px solid {borderColor}; color:{textPrimary}; font-size:13px;"
					></textarea>
				</div>

				{#if optKind === 'iterate'}
					<div>
						<label for="po-feedback" style="font-size:12px; color:{textSecondary}; font-weight:500;">Feedback</label>
						<textarea
							id="po-feedback"
							bind:value={feedback}
							rows="4"
							placeholder="What should change in the next iteration?"
							class="w-full rounded-lg px-3 py-2 mt-1"
							style="background:{surface2}; border:1px solid {borderColor}; color:{textPrimary}; font-size:13px;"
						></textarea>
					</div>
				{/if}

				{#if optKind === 'test'}
					<div>
						<label for="po-test-input" style="font-size:12px; color:{textSecondary}; font-weight:500;">Test input</label>
						<textarea
							id="po-test-input"
							bind:value={testInput}
							rows="4"
							placeholder="Runtime input the prompt should be tested against"
							class="w-full rounded-lg px-3 py-2 mt-1"
							style="background:{surface2}; border:1px solid {borderColor}; color:{textPrimary}; font-size:13px;"
						></textarea>
					</div>
				{/if}

				<details>
					<summary style="font-size:12px; color:{textSecondary}; cursor:pointer; user-select:none;">
						Variable overlay (JSON) <span style="color:{textMuted};">— overrides saved variables for this run only</span>
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
							Stop
						</button>
					{:else}
						<button
							onclick={runOptimize}
							class="rounded-lg px-4 py-2 cursor-pointer"
							style="background:{accent}; color:white; border:none; font-size:13px; font-weight:600;"
						>
							Run {kindLabel(optKind)}
						</button>
					{/if}
					{#if lastHistoryId && !running}
						<span style="font-size:12px; color:{success};">Saved to history</span>
					{/if}
				</div>
			</div>

			<div
				class="rounded-xl p-5 flex flex-col gap-3"
				style="background:{cardBg}; border:1px solid {borderColor}; min-height:400px;"
			>
				<div class="flex items-center justify-between">
					<h2 style="font-size:15px; font-weight:600; color:{textPrimary};">Output</h2>
					{#if running}
						<span style="font-size:12px; color:{accent};">Streaming…</span>
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
						Output will stream here once you run the selected operation.
					</div>
				{/if}
			</div>
		</div>

	{:else if activeTab === 'history'}
		<div class="rounded-xl p-5" style="background:{cardBg}; border:1px solid {borderColor};">
			{#if loadingLists}
				<div style="color:{textMuted}; font-size:13px;">Loading…</div>
			{:else if history.length === 0}
				<div style="color:{textMuted}; font-size:13px;" class="text-center py-6">
					No history yet — run something from the Optimize tab.
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
										aria-label={favSet.has(h.id) ? 'Unfavorite' : 'Favorite'}
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
										Load
									</button>
									<button
										onclick={() => removeHistory(h)}
										aria-label="Delete"
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
				<div style="color:{textMuted}; font-size:13px;">Loading…</div>
			{:else if templates.length === 0}
				<div style="color:{textMuted}; font-size:13px;" class="text-center py-6">
					No templates available.
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
										built-in
									</span>
								{/if}
							</div>
							{#if t.description}
								<div style="font-size:12px; color:{textSecondary}; margin-top:4px;">{t.description}</div>
							{/if}
							{#if t.variables?.length}
								<div style="font-size:11px; color:{textMuted}; margin-top:4px;">
									Variables: {t.variables.join(', ')}
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
				<div style="color:{textMuted}; font-size:13px;">Loading…</div>
			{:else if favorites.length === 0}
				<div style="color:{textMuted}; font-size:13px;" class="text-center py-6">
					No favorites yet — star a history entry to save it.
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
										ref: {f.ref_id}
									</div>
								{/if}
							</div>
							<button
								onclick={async () => {
									try { await deleteFavorite(f.id); await refreshFavorites(); }
									catch (e) { topError = e instanceof Error ? e.message : String(e); }
								}}
								aria-label="Remove favorite"
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
