<script lang="ts" module>
	// Which page part the page control shows: the page, a pane or a leaf pane's panel.
	export type ConfigMode = 'page' | 'panes' | 'panel';
</script>

<!-- The WPI editor's Properties panel (OpenSpec wpi-property-editors, design D6):
     the property editor of the one selected entity — an element node, the page,
     a pane or a panel — and nothing otherwise. -->
<script lang="ts">
	import { m } from '$lib/paraglide/messages.js';
	import type { Node } from '@xyflow/svelte';
	import type { ElementClassSummary } from '$lib/wpi/api';
	import type { FlowLayouterConfig, JsonObject } from '$lib/wpi/types';
	import { elementEditor, moduleFallbacks, page as pageDef, pane as paneDef, panel as panelDef } from '$lib/wpi/properties';
	import { elementSource, pageSource, paneIsLeaf, panelSource, paneSource } from '$lib/wpi/properties/sources';
	import PropertyEditor from '../property-editor/PropertyEditor.svelte';
	import NodeDescriptorEditor from './NodeDescriptorEditor.svelte';
	import PaneStructure from './PaneStructure.svelte';
	import { renameProblem } from './pane-edit';

	type Point = { x: number; y: number };
	let {
		page,
		mode,
		selectedPane,
		node,
		elementClass,
		flowConfig,
		triggerActions,
		pageLayouters,
		positions,
		onPage,
		onNode,
		onLayouter,
		onRename,
		onSelectPane,
		onSplit,
		onAdd,
		onMerge,
		onDelete,
		onEditStart,
		onEditEnd
	}: {
		page: JsonObject | null;
		mode: ConfigMode | null;
		selectedPane: string | null;
		// The selected element node, if exactly one is selected.
		node: Node | null;
		elementClass: ElementClassSummary | null | undefined;
		flowConfig?: FlowLayouterConfig;
		triggerActions: string[];
		pageLayouters: JsonObject[];
		positions: { element?: Point; pane?: Point };
		onPage: (page: JsonObject) => void;
		onNode: (nodeId: string, data: Record<string, unknown>) => void;
		onLayouter: (cls: JsonObject) => void;
		onRename: (pane: string, to: string) => void;
		onSelectPane: (pane: string) => void;
		onSplit: (pane: string, split: 'rows' | 'columns') => void;
		onAdd: (pane: string) => void;
		onMerge: (pane: string) => void;
		onDelete: (pane: string) => void;
		onEditStart: () => void;
		onEditEnd: () => void;
	} = $props();

	let tab = $state<'properties' | 'node'>('properties');
	const layouterIds = $derived(pageLayouters.map((c) => String(c.descriptor_id)));
	const common = $derived({ triggerActions, pageLayouters: layouterIds });
	const module = $derived(String(elementClass?.code_module_id ?? node?.data.element ?? ''));
	const leaf = $derived(!!page && !!selectedPane && paneIsLeaf(page, selectedPane));

	function chooseLayouter(id: string) {
		const cls = pageLayouters.find((c) => c.descriptor_id === id);
		if (cls) onLayouter(cls);
	}
	function paneIdProblem(paneId: string, to: string): string | null {
		if (!page) return null;
		const problem = renameProblem(page, paneId, to);
		return problem === 'taken' ? m.wpi_pane_id_taken({ pane: to }) : problem === 'invalid' ? m.wpi_pane_id_invalid() : null;
	}
</script>

<div class="wpi-properties">
{#if node}
	{@const attrs = (node.data.attrs as JsonObject | undefined) ?? {}}
	<div role="tablist" class="tabs">
		<button type="button" role="tab" aria-selected={tab === 'properties'} onclick={() => (tab = 'properties')}>{m.propertiespanel_tab_properties()}</button>
		<button type="button" role="tab" aria-selected={tab === 'node'} onclick={() => (tab = 'node')}>{m.propertiespanel_tab_node()}</button>
	</div>
	{#key node.id}
		{#if tab === 'node'}
			<NodeDescriptorEditor {node} onUpdate={onNode} />
		{:else if elementClass === null}
			<div class="missing select-text">
				<h2>{m.propertiespanel_element_title({ name: String(node.data.element ?? '') })}</h2>
				<p>{m.propertiespanel_element_class_unavailable({ id: String(node.data.element ?? '') })}</p>
			</div>
		{:else}
			<div class="scroll">
				<PropertyEditor
					title={m.propertiespanel_element_title({ name: String(node.data.element ?? '') })}
					def={elementEditor(module)}
					source={elementSource(attrs, elementClass ?? null, moduleFallbacks(module), (next) => onNode(node.id, { ...node.data, attrs: next }))}
					context={{ ...common, pane: String(node.data.pane ?? ''), flowConfig, position: positions.element }}
					idPrefix={`prop-${node.id}`}
					{onEditStart}
					{onEditEnd}
				/>
			</div>
		{/if}
	{/key}
{:else if page && mode === 'page'}
	<div class="scroll">
		<PropertyEditor
			title={m.wpi_config_page_identity({ id: String(page.descriptor_id ?? '') })}
			def={pageDef}
			source={pageSource(page, onPage, chooseLayouter)}
			context={common}
			idPrefix="prop-page"
			{onEditStart}
			{onEditEnd}
		/>
	</div>
{:else if page && mode === 'panes' && selectedPane}
	{#key selectedPane}
		<div class="scroll">
			<PropertyEditor
				title={m.wpi_config_pane_identity({ id: selectedPane })}
				def={paneDef}
				source={paneSource(page, selectedPane, onPage, (to) => onRename(selectedPane, to))}
				context={{ ...common, leaf, position: positions.pane, paneIdProblem: (to) => paneIdProblem(selectedPane, to) }}
				idPrefix={`prop-pane-${selectedPane}`}
				{onEditStart}
				{onEditEnd}
			>
				<PaneStructure {page} {selectedPane} {onSelectPane} {onSplit} {onAdd} {onMerge} {onDelete} />
			</PropertyEditor>
		</div>
	{/key}
{:else if page && mode === 'panel' && selectedPane && leaf}
	{#key selectedPane}
		<div class="scroll">
			<PropertyEditor
				title={m.wpi_config_panel_identity({ id: selectedPane })}
				def={panelDef}
				source={panelSource(page, selectedPane, onPage)}
				context={common}
				idPrefix={`prop-panel-${selectedPane}`}
				{onEditStart}
				{onEditEnd}
			/>
		</div>
	{/key}
{/if}
</div>

<style>
	.wpi-properties {
		display: flex;
		flex-direction: column;
		height: 100%;
		border-left: 1px solid var(--cv-divider);
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
	.scroll {
		flex: 1;
		min-height: 0;
		overflow-y: auto;
		scrollbar-width: thin;
		scrollbar-color: var(--cv-border) transparent;
	}
	.missing {
		padding: 12px;
		color: var(--cv-text);
	}
	.missing h2 {
		font-size: 14px;
		font-weight: 700;
		margin: 0 0 8px;
	}
	.missing p {
		font-size: 11px;
		color: var(--cv-text-faint);
	}
</style>
