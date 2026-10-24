<script lang="ts">
	import { m } from '$lib/paraglide/messages.js';
	import type {
		MeshEntity,
		MeshProperty,
		MeshRecord,
		MeshRecordTreeNode
	} from '$lib/services/meshService';
	import { buildRecordDetails, valuesOf } from '$lib/services/meshRecord';
	import NameValueVisualizer from '$lib/components/shared-ui/name-value/name-value-visualizer.svelte';
	import type { NameValueInput } from '$lib/components/shared-ui/name-value/name-value';

	let { record, onOpenRecord }: { record: MeshRecord; onOpenRecord: (id: string) => void } =
		$props();
	let activeTab = $state('details');
	const tabs = $derived([
		{ id: 'details', label: m.mesh_record_details() },
		{ id: 'qualifiers', label: m.mesh_record_qualifiers() },
		{ id: 'trees', label: m.mesh_record_trees() },
		{ id: 'concepts', label: m.mesh_record_concepts() }
	]);
	const details = $derived(buildRecordDetails(record));
	const labels: Record<string, () => string> = {
		heading: () => m.mesh_record_heading(),
		treeNumber: () => m.mesh_record_tree_numbers(),
		identifier: () => m.mesh_record_unique_id(),
		rdfIdentifier: () => m.mesh_record_rdf_id(),
		annotation: () => m.mesh_search_annotation(),
		scopeNote: () => m.mesh_pred_scope_note(),
		entryTerms: () => m.mesh_record_entry_terms(),
		registryNumber: () => m.mesh_record_registry(),
		relatedRegistryNumber: () => m.mesh_record_related_numbers(),
		previousIndexing: () => m.mesh_pred_previous_indexing(),
		publicMeSHNote: () => m.mesh_record_public_note(),
		onlineNote: () => m.mesh_record_online_note(),
		historyNote: () => m.mesh_pred_history_note(),
		dateEstablished: () => m.mesh_record_introduced(),
		dateRevised: () => m.mesh_record_updated(),
		dateCreated: () => m.mesh_pred_date_created(),
		lexicalTag: () => m.mesh_pred_lexical_tag(),
		thesaurusID: () => m.mesh_record_thesaurus(),
		broaderDescriptor: () => m.mesh_pred_broader_descriptor(),
		pharmacologicalAction: () => m.mesh_pred_pharmacological_action(),
		preferredMappedTo: () => m.mesh_record_preferred_mapped(),
		mappedTo: () => m.mesh_search_mapped(),
		seeAlso: () => m.mesh_record_see_also(),
		prefLabel: () => m.mesh_record_preferred_label(),
		altLabel: () => m.mesh_record_alternative_labels(),
		broaderConcept: () => m.mesh_record_broader_concept(),
		narrowerConcept: () => m.mesh_record_narrower_concept(),
		relatedConcept: () => m.mesh_record_related_concept(),
		active: () => m.mesh_record_active(),
		frequency: () => m.mesh_record_frequency(),
		indexerConsiderAlso: () => m.mesh_record_consider_also(),
		nlmClassificationNumber: () => m.mesh_record_nlm_classification(),
		sortVersion: () => m.mesh_record_sort_version(),
		abbreviation: () => m.mesh_record_abbreviation(),
		conceptUI: () => m.mesh_record_concept_ui(),
		termUI: () => m.mesh_record_term_ui(),
		note: () => m.mesh_record_note(),
		casn1_label: () => m.mesh_record_cas_name(),
		entryVersion: () => m.mesh_record_entry_version(),
		source: () => m.mesh_record_source(),
		considerAlso: () => m.mesh_record_consider_also()
	};
	function labelFor(predicate: string) {
		return labels[predicate]?.() ?? predicate.replace(/([a-z])([A-Z])/g, '$1 $2');
	}
	function toFields(properties: MeshProperty[]): NameValueInput[] {
		return properties.map((property) => ({
			key: property.predicate,
			label: labelFor(property.predicate),
			values: property.values.map((value) => {
				if (property.predicate === 'rdfIdentifier')
					return { text: value.text ?? '', href: value.text };
				if (property.predicate === 'treeNumber' && value.id)
					return { text: value.id, onClick: () => (activeTab = 'trees') };
				if (value.id)
					return { text: value.label || value.id, onClick: () => onOpenRecord(value.id!) };
				return { text: value.text ?? '', lang: value.lang };
			})
		}));
	}
	function keyboardTab(event: KeyboardEvent) {
		const index = tabs.findIndex(
			(tab) => `mesh-record-tab-${tab.id}` === (event.currentTarget as HTMLElement).id
		);
		let next: number;
		if (event.key === 'ArrowRight') next = (index + 1) % tabs.length;
		else if (event.key === 'ArrowLeft') next = (index - 1 + tabs.length) % tabs.length;
		else if (event.key === 'Home') next = 0;
		else if (event.key === 'End') next = tabs.length - 1;
		else return;
		event.preventDefault();
		activeTab = tabs[next].id;
		(event.currentTarget as HTMLElement).parentElement
			?.querySelector<HTMLButtonElement>(`#mesh-record-tab-${activeTab}`)
			?.focus();
	}
	function metadata(entity: MeshEntity): MeshProperty[] {
		return (entity.properties ?? [])
			.filter((p) => !['term', 'preferredTerm', 'prefLabel'].includes(p.predicate))
			.map((p) => ({
				...p,
				predicate:
					p.predicate === 'identifier'
						? entity.kind === 'Concept'
							? 'conceptUI'
							: entity.kind === 'Term'
								? 'termUI'
								: p.predicate
						: p.predicate
			}));
	}
</script>

{#snippet treeNode(node: MeshRecordTreeNode, selected = false)}
	<span class:current={selected}>
		{#if node.record_id && !selected}<button
				class="text-link"
				type="button"
				onclick={() => onOpenRecord(node.record_id!)}>{node.label}</button
			>
		{:else}{node.label}{/if}
		<code>[{node.id}]</code>
	</span>
{/snippet}

<div class="mesh-record select-text">
	<div class="record-heading">
		<h2>{record.entity.label || record.entity.id}</h2>
		<p>{m.mesh_record_data({ release: record.release })}</p>
	</div>
	<div class="record-tabs" role="tablist" aria-label={m.mesh_record_tabs()}>
		{#each tabs as tab (tab.id)}
			<button
				id={`mesh-record-tab-${tab.id}`}
				type="button"
				role="tab"
				aria-selected={activeTab === tab.id}
				aria-controls={`mesh-record-panel-${tab.id}`}
				tabindex={activeTab === tab.id ? 0 : -1}
				onclick={() => (activeTab = tab.id)}
				onkeydown={keyboardTab}>{tab.label}</button
			>
		{/each}
	</div>
	{#each tabs as tab (tab.id)}
		<div
			class="record-panel"
			id={`mesh-record-panel-${tab.id}`}
			role="tabpanel"
			aria-labelledby={`mesh-record-tab-${tab.id}`}
			tabindex="0"
			hidden={activeTab !== tab.id}
		>
			{#if activeTab === tab.id}
				{#if activeTab === 'details'}
					<NameValueVisualizer fields={toFields(details)} />
				{:else if activeTab === 'qualifiers'}
					<h3>{m.mesh_record_allowable_qualifiers()}</h3>
					<ul class="qualifiers">
						{#each record.qualifiers as qualifier (qualifier.id)}
							<li>
								<button class="text-link" type="button" onclick={() => onOpenRecord(qualifier.id)}
									>{qualifier.label || qualifier.id}</button
								><code>{qualifier.id}</code>
							</li>
						{:else}<li class="empty">{m.mesh_record_no_qualifiers()}</li>{/each}
					</ul>
				{:else if activeTab === 'trees'}
					{#each record.trees as tree (tree.id)}
						<section class="tree-placement" aria-label={tree.id}>
							<ol class="tree-path">
								{#each tree.path as node, depth (node.id)}
									<li style={`--depth:${depth}`}>{@render treeNode(node, node.id === tree.id)}</li>
								{/each}
							</ol>
							<ul class="tree-children" style={`--depth:${tree.path.length}`}>
								{#each tree.children as child (child.id)}<li>{@render treeNode(child)}</li>{/each}
							</ul>
						</section>
					{:else}<p class="empty">{m.mesh_record_no_trees()}</p>{/each}
				{:else}
					{#each record.concepts as concept (concept.entity.id)}
						<details class="concept" open={concept.preferred}>
							<summary
								>{concept.entity.label || concept.entity.id}
								{#if concept.preferred}<span class="badge">{m.mesh_record_preferred()}</span
									>{/if}</summary
							>
							<NameValueVisualizer fields={toFields(metadata(concept.entity))} />
							<h3>{m.mesh_record_terms()}</h3>
							{#each concept.terms as term (term.id)}
								<div class="term">
									<h4>
										{term.label || term.id}
										{#if valuesOf(concept.entity, 'preferredTerm').some((v) => v.id === term.id)}<span
												class="badge">{m.mesh_record_preferred_term()}</span
											>{/if}
									</h4>
									<NameValueVisualizer fields={toFields(metadata(term))} />
								</div>
							{/each}
						</details>
					{:else}<p class="empty">{m.mesh_record_no_concepts()}</p>{/each}
				{/if}
			{/if}
		</div>
	{/each}
</div>

<style>
	.mesh-record {
		min-width: 0;
		overflow-wrap: anywhere;
		color: var(--ink);
	}
	.record-heading {
		display: flex;
		flex-wrap: wrap;
		align-items: baseline;
		gap: 0.4rem 1rem;
		margin-bottom: 1rem;
	}
	h2 {
		font-size: 1.65rem;
		font-weight: 650;
		letter-spacing: -0.02em;
	}
	.record-heading p {
		color: var(--muted);
		font-size: 1rem;
	}
	.record-tabs {
		display: flex;
		flex-wrap: wrap;
		border-bottom: 1px solid var(--line);
		gap: 0.2rem;
	}
	.record-tabs button {
		padding: 0.7rem 0.85rem;
		margin-bottom: -1px;
		color: var(--accent);
		border: 1px solid transparent;
		background: transparent;
		border-radius: 4px 4px 0 0;
		font-size: 0.875rem;
		cursor: pointer;
	}
	.record-tabs button[aria-selected='true'] {
		border-color: var(--line);
		border-bottom-color: var(--surface);
		background: var(--surface);
		color: var(--ink);
	}
	.record-panel {
		padding: 1.25rem 0 0.5rem;
	}
	.text-link {
		color: var(--accent);
		text-decoration: none;
	}
	.text-link:hover {
		text-decoration: underline;
	}
	.text-link {
		padding: 0;
		border: 0;
		background: transparent;
		font: inherit;
		text-align: left;
		cursor: pointer;
	}
	button:focus-visible,
	summary:focus-visible {
		outline: 2px solid var(--accent);
		outline-offset: 3px;
	}
	h3 {
		margin: 0.25rem 0 0.75rem;
		font-weight: 650;
		font-size: 0.95rem;
	}
	.qualifiers {
		padding-left: 1.25rem;
		columns: 2;
		line-height: 1.9;
		font-size: 0.875rem;
	}
	code {
		margin-left: 0.5rem;
		font-size: 0.75rem;
		color: var(--muted);
	}
	.tree-placement + .tree-placement {
		border-top: 1px solid var(--line);
		padding-top: 1rem;
		margin-top: 1rem;
	}
	.tree-path {
		list-style: none;
		padding: 0;
	}
	.tree-path li,
	.tree-children {
		margin-left: calc(min(var(--depth), 8) * 1rem);
	}
	.tree-path li,
	.tree-children li {
		font-size: 0.875rem;
		line-height: 1.9;
	}
	.tree-children {
		list-style: disc;
		padding-left: 1rem;
	}
	.current {
		font-weight: 650;
	}
	.concept {
		border-bottom: 1px solid var(--line);
		padding: 0.9rem 0;
	}
	summary {
		cursor: pointer;
		font-weight: 650;
		margin-bottom: 0.8rem;
	}
	.concept h3 {
		margin-top: 1rem;
	}
	.term {
		padding: 0.85rem 0 0.85rem 0.75rem;
		border-left: 2px solid var(--line);
		margin: 0.6rem 0;
	}
	h4 {
		font-weight: 600;
		margin-bottom: 0.65rem;
		font-size: 0.875rem;
	}
	.badge {
		display: inline-block;
		color: var(--muted);
		font-size: 0.75rem;
		font-weight: 400;
		margin-left: 0.6rem;
	}
	.empty {
		color: var(--muted);
		font-size: 0.875rem;
		padding: 0.5rem 0;
		list-style: none;
	}
	@media (max-width: 600px) {
		.qualifiers {
			columns: 1;
		}
		.record-tabs button {
			padding: 0.6rem 0.5rem;
			font-size: 0.8rem;
		}
		.tree-path li,
		.tree-children {
			margin-left: calc(min(var(--depth), 5) * 0.6rem);
		}
	}
</style>
