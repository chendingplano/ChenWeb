<script lang="ts">
	import { page } from '$app/state';
	import DocumentEditorPage from '$lib/components/cdm/DocumentEditorPage.svelte';

	let documentKey = $derived(page.params.key);
</script>

<div class="cdm-editor-standalone">
	{#key documentKey}
		<DocumentEditorPage {documentKey} />
	{/key}
</div>

<style>
	.cdm-editor-standalone {
		max-width: 900px;
		margin: 0 auto;
		padding: 16px;
	}
</style>
