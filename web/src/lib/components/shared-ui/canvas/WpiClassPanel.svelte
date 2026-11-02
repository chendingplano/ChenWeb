<script lang="ts">
	import type { Snippet } from 'svelte';
	import { m } from '$lib/paraglide/messages.js';
	import EntityClassEditor from './EntityClassEditor.svelte';
	import type { ClassTarget } from './class-editor';
	let {
		target,
		onSaved,
		onDirty,
		children,
		tab = $bindable<'properties' | 'class'>('properties'),
		actionClass,
		actionClasses = [],
		onActionClass,
		darkMode = true
	}: {
		target: ClassTarget | null;
		onSaved: () => void;
		onDirty?: (dirty: boolean) => void;
		children: Snippet;
		tab?: 'properties' | 'class';
		actionClass?: string;
		actionClasses?: string[];
		onActionClass?: (id: string) => void;
		darkMode?: boolean;
	} = $props();
	let classEditor = $state<{ viewJson: () => boolean }>();
	export function viewClassJson(): boolean {
		return classEditor?.viewJson() ?? false;
	}
</script>

<section class="class-panel select-text">
	<div role="tablist" class="tabs" aria-label={m.wpi_class_panel_tabs()}>
		<button
			type="button"
			role="tab"
			id="wpi-properties-tab"
			aria-controls="wpi-properties-content"
			aria-selected={tab === 'properties'}
			onclick={() => (tab = 'properties')}>{m.propertiespanel_tab_properties()}</button
		>
		<button
			type="button"
			role="tab"
			id="wpi-class-tab"
			aria-controls="wpi-class-content"
			aria-selected={tab === 'class'}
			onclick={() => (tab = 'class')}>{m.wpi_class_tab()}</button
		>
	</div>
	<div
		role="tabpanel"
		id="wpi-properties-content"
		aria-labelledby="wpi-properties-tab"
		hidden={tab !== 'properties'}
		class="content"
	>
		{@render children()}
	</div>
	<div
		role="tabpanel"
		id="wpi-class-content"
		aria-labelledby="wpi-class-tab"
		hidden={tab !== 'class'}
		class="content scroll"
	>
		{#if actionClass !== undefined}
			<div class="action-class">
				<label for="wpi-action-class">{m.wpi_class_action_class()}</label>
				<select
					id="wpi-action-class"
					value={actionClass}
					onchange={(e) => onActionClass?.(e.currentTarget.value)}
				>
					<option value="">{m.wpi_class_select_action()}</option>
					{#if actionClass && !actionClasses.includes(actionClass)}<option value={actionClass}
							>{actionClass}</option
						>{/if}
					{#each actionClasses as id}<option value={id}>{id}</option>{/each}
				</select>
			</div>
		{/if}
		<EntityClassEditor
			bind:this={classEditor}
			{target}
			active={tab === 'class'}
			{onSaved}
			{onDirty}
			{darkMode}
		/>
	</div>
</section>

<style>
	.class-panel {
		height: 100%;
		display: flex;
		flex-direction: column;
		min-height: 0;
	}
	.tabs {
		display: flex;
		gap: 16px;
		padding: 0 12px;
		border-bottom: 1px solid var(--cv-divider);
		flex-shrink: 0;
	}
	.tabs button {
		background: none;
		border: none;
		border-bottom: 2px solid transparent;
		margin-bottom: -1px;
		padding: 9px 0 7px;
		font-size: 11px;
		font-weight: 500;
		color: var(--cv-text-muted);
		cursor: pointer;
	}
	.tabs button[aria-selected='true'] {
		border-bottom-color: var(--cv-accent);
		color: var(--cv-accent);
	}
	.content {
		flex: 1;
		min-height: 0;
	}
	.content[hidden] {
		display: none;
	}
	.scroll {
		overflow-y: auto;
		scrollbar-width: thin;
		scrollbar-color: var(--cv-border) transparent;
	}
	.action-class {
		padding: 12px 12px 0;
	}
	.action-class label {
		display: block;
		color: var(--cv-text-2);
		font-size: 11px;
		margin-bottom: 4px;
	}
	.action-class select {
		width: 100%;
		background: var(--cv-surface);
		color: var(--cv-text);
		border: 1px solid var(--cv-border);
		border-radius: 4px;
		padding: 5px 7px;
		font-size: 11px;
	}
</style>
