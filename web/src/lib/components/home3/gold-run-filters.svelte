<script lang="ts">
	import { m } from '$lib/paraglide/messages.js';
	import type { GoldRunFilter } from './gold-run-filter.svelte';

	// The skill-version / model selects of a gold benchmark view, for the record
	// browser's extraControls slot. Colors come from the host's --panel-bg,
	// --ink-line, --text-primary, --text-muted, --brass and --font-mono.
	let { filter, onChange }: { filter: GoldRunFilter; onChange?: () => void } = $props();
</script>

<div class="gold-filters">
	<label class="gold-filter">
		<span class="gold-filter-label">{m.gold_run_filters_skill_version()}</span>
		<select
			class="gold-select"
			value={filter.skillVersion}
			onchange={(event) => {
				filter.setSkillVersion(event.currentTarget.value);
				onChange?.();
			}}
		>
			{#each filter.versions as v, idx (v)}
				<option value={v}>{idx === 0 ? m.gold_run_filters_version_newest({ version: v }) : v}</option>
			{/each}
		</select>
	</label>
	<label class="gold-filter">
		<span class="gold-filter-label">{m.gold_run_filters_model_name()}</span>
		<select
			class="gold-select"
			value={filter.modelName}
			onchange={(event) => {
				filter.setModelName(event.currentTarget.value);
				onChange?.();
			}}
		>
			<option value="">{m.gold_run_filters_all_models()}</option>
			{#each filter.modelNames as name (name)}
				<option value={name}>{name}</option>
			{/each}
		</select>
	</label>
	{#if filter.error}
		<div class="gold-filter-hint error">{filter.error}</div>
	{/if}
</div>

<style>
	.gold-filters {
		display: grid;
		grid-template-columns: minmax(0, 1fr) minmax(0, 1fr);
		gap: 0.5rem 0.75rem;
		margin-top: 0.75rem;
	}
	.gold-filter {
		display: flex;
		flex-direction: column;
		gap: 0.3rem;
		min-width: 0;
	}
	.gold-filter-label {
		font-size: 0.72rem;
		font-weight: 700;
		letter-spacing: 0.12em;
		text-transform: uppercase;
		color: var(--text-muted);
	}
	.gold-select {
		background: var(--panel-bg);
		border: 1px solid var(--ink-line);
		border-radius: 6px;
		color: var(--text-primary);
		font-family: var(--font-mono);
		font-size: 11px;
		padding: 4px 8px;
		width: 100%;
		min-width: 0;
	}
	.gold-select:focus {
		outline: none;
		border-color: var(--brass);
	}
	.gold-filter-hint {
		grid-column: 1 / -1;
		font-size: 12px;
		color: var(--brass);
	}
</style>
