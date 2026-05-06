<script lang="ts">
	import TreeGraphView from './tree-graph-view.svelte';
	import type { TopicCard } from './topic-types';
	let {
		darkMode = true,
		heroEyebrow = 'Semantic Web',
		heroTitle = 'Topic Graph',
		heroDescription = 'Category-first workspace for browsing topics indexed in the Semantic Web.',
		rootTabLabel = 'Semantic Web',
		loadErrorLabel = 'Semantic Web',
		itemLabelPlural = 'Topics',
		showItemsLabel = 'Show Topics',
		showItemNodes = false,
		listGraph,
		getCategoryItems
	}: {
		darkMode?: boolean;
		heroEyebrow?: string;
		heroTitle?: string;
		heroDescription?: string;
		rootTabLabel?: string;
		loadErrorLabel?: string;
		itemLabelPlural?: string;
		showItemsLabel?: string;
		showItemNodes?: boolean;
		listGraph?: () => Promise<{ results: any[] }>;
		getCategoryItems?: (categoryPath: string) => Promise<{ topics: TopicCard[] }>;
	} = $props();
</script>

<TreeGraphView
	mode="topic"
	{darkMode}
	{heroEyebrow}
	{heroTitle}
	{heroDescription}
	{rootTabLabel}
	{loadErrorLabel}
	{itemLabelPlural}
	{showItemsLabel}
	{showItemNodes}
	{listGraph}
	{getCategoryItems}
/>
