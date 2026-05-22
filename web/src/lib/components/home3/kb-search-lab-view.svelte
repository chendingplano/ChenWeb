<script lang="ts">
	import { kbSearchArtifactOptions } from '$lib/components/home3/kb-search-lab-state';
	import {
		searchKbArtifacts,
		type KbSearchArtifactType,
		type KbSearchResponse
	} from '$lib/services/kbArtifactSearch';

	let { darkMode = true }: { darkMode: boolean } = $props();

	let artifactType = $state<KbSearchArtifactType>('all');
	let query = $state('');
	let inputRecordId = $state('');
	let artifactTypes = $state('');
	let categoryPath = $state('');
	let topicType = $state('');
	let sceneType = $state('');
	let provisionType = $state('');
	let productType = $state('');
	let relationType = $state('');
	let loading = $state(false);
	let error = $state('');
	let payload = $state<KbSearchResponse | null>(null);

	let pageBg = $derived(darkMode ? '#171B26' : '#F2F4F7');
	let cardBg = $derived(darkMode ? '#1F2333' : '#FFFFFF');
	let borderColor = $derived(darkMode ? '#2D3348' : '#E4E6EB');
	let accent = $derived(darkMode ? '#818CF8' : '#6366F1');
	let textPrimary = $derived(darkMode ? '#E2E8F0' : '#111827');
	let textSecondary = $derived(darkMode ? '#94A3B8' : '#6B7280');
	let textMuted = $derived(darkMode ? '#64748B' : '#9CA3AF');
	let surface2 = $derived(darkMode ? '#252A3A' : '#ECEEF2');

	async function runSearch() {
		if (!query.trim()) {
			error = 'Enter a query.';
			payload = null;
			return;
		}
		loading = true;
		error = '';
		try {
			payload = await searchKbArtifacts(artifactType, {
				q: query.trim(),
				inputRecordId,
				artifactTypes,
				categoryPath,
				topicType,
				sceneType,
				provisionType,
				productType,
				relationType
			});
		} catch (err) {
			error = err instanceof Error ? err.message : 'Search failed';
			payload = null;
		} finally {
			loading = false;
		}
	}
</script>

<section class="p-6" style="background:{pageBg}; color:{textPrimary}; min-height:100%;">
	<div
		class="rounded-2xl p-6"
		style="background:{cardBg}; border:1px solid {borderColor};"
	>
		<div class="mb-5">
			<h1 class="text-xl font-semibold">KB Search Lab</h1>
			<p class="mt-2 text-sm" style="color:{textSecondary};">
				Quick operator harness for artifact search endpoints and LLM-oriented result payloads.
			</p>
		</div>

		<div class="grid gap-4 md:grid-cols-[220px_1fr_180px_auto]">
			<label class="flex flex-col gap-2 text-sm">
				<span style="color:{textSecondary};">Artifact</span>
				<select bind:value={artifactType} class="rounded-xl px-3 py-2" style="background:{surface2}; border:1px solid {borderColor};">
					{#each kbSearchArtifactOptions as option}
						<option value={option.value}>{option.label}</option>
					{/each}
				</select>
			</label>

			<label class="flex flex-col gap-2 text-sm">
				<span style="color:{textSecondary};">Query</span>
				<input
					bind:value={query}
					class="rounded-xl px-3 py-2"
					style="background:{surface2}; border:1px solid {borderColor};"
					placeholder="energy intensity, safety protection, battery..."
					onkeydown={(event) => event.key === 'Enter' && runSearch()}
				/>
			</label>

			<label class="flex flex-col gap-2 text-sm">
				<span style="color:{textSecondary};">Record ID</span>
				<input
					bind:value={inputRecordId}
					class="rounded-xl px-3 py-2"
					style="background:{surface2}; border:1px solid {borderColor};"
					placeholder="optional"
					onkeydown={(event) => event.key === 'Enter' && runSearch()}
				/>
			</label>

			<div class="flex items-end">
				<button
					type="button"
					class="rounded-xl px-4 py-2 text-sm font-medium"
					style="background:{accent}; color:white;"
					onclick={runSearch}
					disabled={loading}
				>
					{loading ? 'Searching...' : 'Search'}
				</button>
			</div>
		</div>

		<div class="mt-4 grid gap-4 md:grid-cols-3">
			<label class="flex flex-col gap-2 text-sm">
				<span style="color:{textSecondary};">Category Path</span>
				<input bind:value={categoryPath} class="rounded-xl px-3 py-2" style="background:{surface2}; border:1px solid {borderColor};" placeholder="optional" />
			</label>

			{#if artifactType === 'all'}
				<label class="flex flex-col gap-2 text-sm">
					<span style="color:{textSecondary};">Artifact Types</span>
					<input bind:value={artifactTypes} class="rounded-xl px-3 py-2" style="background:{surface2}; border:1px solid {borderColor};" placeholder="summary,provision" />
				</label>
			{/if}

			{#if artifactType === 'topics'}
				<label class="flex flex-col gap-2 text-sm">
					<span style="color:{textSecondary};">Topic Type</span>
					<input bind:value={topicType} class="rounded-xl px-3 py-2" style="background:{surface2}; border:1px solid {borderColor};" placeholder="requirement" />
				</label>
			{/if}

			{#if artifactType === 'scene-blocks'}
				<label class="flex flex-col gap-2 text-sm">
					<span style="color:{textSecondary};">Scene Type</span>
					<input bind:value={sceneType} class="rounded-xl px-3 py-2" style="background:{surface2}; border:1px solid {borderColor};" placeholder="operation" />
				</label>
			{/if}

			{#if artifactType === 'provisions'}
				<label class="flex flex-col gap-2 text-sm">
					<span style="color:{textSecondary};">Provision Type</span>
					<input bind:value={provisionType} class="rounded-xl px-3 py-2" style="background:{surface2}; border:1px solid {borderColor};" placeholder="mandatory" />
				</label>
			{/if}

			{#if artifactType === 'products'}
				<label class="flex flex-col gap-2 text-sm">
					<span style="color:{textSecondary};">Product Type</span>
					<input bind:value={productType} class="rounded-xl px-3 py-2" style="background:{surface2}; border:1px solid {borderColor};" placeholder="equipment" />
				</label>
				<label class="flex flex-col gap-2 text-sm">
					<span style="color:{textSecondary};">Relation Type</span>
					<input bind:value={relationType} class="rounded-xl px-3 py-2" style="background:{surface2}; border:1px solid {borderColor};" placeholder="maintenance_requirement" />
				</label>
			{/if}
		</div>

		{#if error}
			<div class="mt-4 rounded-xl px-4 py-3 text-sm" style="background:rgba(239,68,68,0.12); color:#ef4444;">
				{error}
			</div>
		{/if}

		{#if payload}
			<div class="mt-6 flex items-center justify-between text-sm" style="color:{textSecondary};">
				<div>Artifact type: <span style="color:{textPrimary};">{payload.artifact_type ?? artifactType}</span></div>
				<div>Total: <span style="color:{textPrimary};">{payload.total ?? payload.results?.length ?? 0}</span></div>
			</div>

			<div class="mt-4 space-y-3">
				{#each payload.results ?? [] as result, index}
					<article class="rounded-2xl p-4" style="background:{surface2}; border:1px solid {borderColor};">
						<div class="flex items-start justify-between gap-4">
							<div>
								<div class="text-sm font-semibold">
									{result.primary_label ?? result.metric_name ?? result.topicText ?? result.summaryText ?? `Result ${index + 1}`}
								</div>
								<div class="mt-1 text-xs" style="color:{textMuted};">
									{result.artifact_type ?? payload.artifact_type ?? artifactType}
									· {result.artifact_id ?? result.id ?? 'n/a'}
									{#if result.secondary_label}
										· {result.secondary_label}
									{/if}
									{#if result.input_record_id ?? result.inputId}
										· record {result.input_record_id ?? result.inputId}
									{/if}
								</div>
							</div>
							<div class="text-xs" style="color:{textSecondary};">
								score {typeof result.score === 'number' ? result.score.toFixed(4) : 'n/a'}
							</div>
						</div>
						{#if result.snippet}
							<p class="mt-3 text-sm leading-6" style="color:{textSecondary};">{result.snippet}</p>
						{/if}
						{#if result.source_title ?? result.source_filename}
							<div class="mt-3 text-xs" style="color:{textMuted};">
								source: {result.source_title ?? result.source_filename}
							</div>
						{/if}
					</article>
				{/each}

				{#if !payload.results || payload.results.length === 0}
					<div class="rounded-2xl p-6 text-sm" style="background:{surface2}; border:1px dashed {borderColor}; color:{textMuted};">
						No results.
					</div>
				{/if}
			</div>
		{/if}
	</div>
</section>
