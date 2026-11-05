<script lang="ts" module>
	// Which page part the page control shows: the page, a pane or a leaf pane's panel.
	export type ConfigMode = 'app' | 'page' | 'panes' | 'panel';
</script>

<!-- The WPI editor's Properties panel (OpenSpec wpi-property-editors, design D6):
     the property editor of the one selected entity — an element node, the page,
     a pane or a panel — and nothing otherwise. -->
<script lang="ts">
	import { m } from '$lib/paraglide/messages.js';
	import type { Node } from '@xyflow/svelte';
	import type { ElementClassSummary } from '$lib/wpi/api';
	import type { FlowLayouterConfig, JsonObject, PageDescriptor } from '$lib/wpi/types';
	import { elementEditor, moduleFallbacks, page as pageDef, pane as paneDef, panel as panelDef } from '$lib/wpi/properties';
	import { withClassEditors, elementClassDefinitions } from '$lib/wpi/properties/class-definition';
	import { elementSource, pageSource, paneIsLeaf, panelSource, paneSource } from '$lib/wpi/properties/sources';
	import { withValidatorGroups, withValidatorSource } from '$lib/wpi/properties/validators';
	import type { ValidatorClass } from '$lib/wpi/validators';
	import PropertyEditor from '../property-editor/PropertyEditor.svelte';
	import PaneStructure from './PaneStructure.svelte';
	import { renameProblem } from './pane-edit';
	import type { EditorDef } from '$lib/wpi/properties/types';

	const appDef: EditorDef = { groups: [{ id: 'app', label: m.wpi_app_properties, properties: [
		{ name: 'descriptor_id', label: m.wpi_prop_descriptor_id, editor: 'text' },
		{ name: 'initial_page', label: m.wpi_app_initial_page, editor: 'text' },
		{ name: 'app_name', label: m.wpi_app_name, editor: 'string-editor', options: { mode: 'multilingual', languages: ['en', 'zh-cn'] } },
		{ name: 'app_description', label: m.wpi_prop_description, editor: 'string-editor', options: { mode: 'multilingual', languages: ['en', 'zh-cn'] } },
		{ name: 'keywords', label: m.wpi_prop_keywords, editor: 'string-list' },
		{ name: 'tags', label: m.wpi_prop_page_tags, editor: 'string-list' }
	] }] };

	type Point = { x: number; y: number };
	let {
		page,
		app,
		appReadOnly = false,
		mode,
		selectedPane,
		node,
		elementClass,
		flowConfig,
		triggerActions,
		pageLayouters,
		validatorClasses = [],
		positions,
		onPage,
		onApp,
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
		app: JsonObject | null;
		appReadOnly?: boolean;
		mode: ConfigMode | null;
		selectedPane: string | null;
		// The selected element node, if exactly one is selected.
		node: Node | null;
		elementClass: ElementClassSummary | null | undefined;
		flowConfig?: FlowLayouterConfig;
		triggerActions: string[];
		pageLayouters: JsonObject[];
		// Offered by an element's Validators group (OpenSpec wpi-primitive-validators).
		validatorClasses?: ValidatorClass[];
		positions: { element?: Point; pane?: Point };
		onPage: (page: JsonObject) => void;
		onApp: (app: JsonObject) => void;
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

	const layouterIds = $derived(pageLayouters.map((c) => String(c.descriptor_id)));
	const common = $derived({ triggerActions, pageLayouters: layouterIds, validatorClasses });
	const module = $derived(String(elementClass?.code_module_id ?? node?.data.element ?? ''));
	const leaf = $derived(!!page && !!selectedPane && paneIsLeaf(page, selectedPane));
	const panelClassDefinitions = $derived((page as unknown as PageDescriptor | null)?.panels?.[selectedPane ?? '']?.elem_layouter?.class?.attribute_definitions as JsonObject | undefined);

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
	{#key node.id}
		{#if elementClass === null}
			<div class="missing select-text">
				<h2>{m.propertiespanel_element_title({ name: String(node.data.element ?? '') })}</h2>
				<p>{m.propertiespanel_element_class_unavailable({ id: String(node.data.element ?? '') })}</p>
			</div>
		{:else}
			{@const source = withValidatorSource(elementSource(attrs, elementClass ?? null, moduleFallbacks(module), (next) => onNode(node.id, { ...node.data, attrs: next })))}
			<div class="scroll">
				{#if node.data.instance_id}
					<div class="instance-id select-text" title={m.wpi_instance_id_note()}>
						<span class="label">{m.wpi_prop_instance_id()}</span>
						<code>{String(node.data.instance_id)}</code>
					</div>
				{/if}
				<PropertyEditor
					title={m.propertiespanel_element_title({ name: String(node.data.element ?? '') })}
					def={withValidatorGroups(withClassEditors(elementEditor(module), elementClassDefinitions(elementClass)), source.get('validators'))}
					{source}
					context={{ ...common, pane: String(node.data.pane ?? ''), flowConfig, position: positions.element }}
					idPrefix={`prop-${node.id}`}
					{onEditStart}
					{onEditEnd}
				/>
			</div>
		{/if}
	{/key}
{:else if app && mode === 'app'}
	<div class="scroll">
		<PropertyEditor title={m.wpi_app_properties()} def={appDef} source={appReadOnly ? { ...pageSource(app, onApp, () => {}), fixed: () => true } : pageSource(app, onApp, () => {})}
			context={common} idPrefix="prop-app" {onEditStart} {onEditEnd} />
	</div>
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
				def={withClassEditors(panelDef, panelClassDefinitions, '/config')}
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
	.scroll {
		flex: 1;
		min-height: 0;
		overflow-y: auto;
		scrollbar-width: thin;
		scrollbar-color: var(--cv-border) transparent;
	}
	.instance-id {
		display: flex;
		gap: 8px;
		align-items: baseline;
		padding: 8px 12px 0;
		font-size: 11px;
		color: var(--cv-text);
	}
	.instance-id .label {
		color: var(--cv-text-faint);
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
