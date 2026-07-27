<script lang="ts">
	import { page } from '$app/state';
	import ChevronLeftIcon from '@lucide/svelte/icons/chevron-left';
	import { theme } from '$lib/stores/theme.svelte';
	import DocumentEditorPage from '$lib/components/cdm/DocumentEditorPage.svelte';

	let documentKey = $derived(page.params.key);
	let darkMode = $derived(theme.isDark);
</script>

<div class="cdm-editor-standalone" class:dark={darkMode}>
	<!-- Chrome shared with the CDM Editor main page: a way back to it, and the
	     document key, which DocumentEditor's own header does not show (it shows
	     the editable title and the version badge). -->
	<nav class="cdm-editor-crumbs">
		<a href="/home3/cdm"><ChevronLeftIcon size={14} /> All documents</a>
		<span class="cdm-editor-key">{documentKey}</span>
	</nav>

	<div class="cdm-editor-body">
		{#key documentKey}
			<DocumentEditorPage {documentKey} />
		{/key}
	</div>
</div>

<style>
	.cdm-editor-standalone {
		min-height: 100vh;
		padding: 20px 16px 64px;
		/* Same two variables the cdm/* components read as the main page sets,
		   so both halves of the editor agree on light/dark. `color-scheme`
		   carries that to the native form controls inside DocumentEditor,
		   which style only their borders. */
		--cdm-surface: #fbf8f0;
		--cdm-muted: #8f8472;
		background: #f5f1e8;
		color: #1a1410;
		color-scheme: light;
		font-family: 'Inter Tight', system-ui, sans-serif;
	}
	.cdm-editor-standalone.dark {
		--cdm-surface: #161a22;
		--cdm-muted: #7c7560;
		background: #0e1116;
		color: #ede7d3;
		color-scheme: dark;
	}

	.cdm-editor-crumbs {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 12px;
		max-width: 900px;
		margin: 0 auto 12px;
	}
	.cdm-editor-crumbs a {
		display: inline-flex;
		align-items: center;
		gap: 3px;
		font-size: 0.82rem;
		font-weight: 600;
		color: #b8801e;
		text-decoration: none;
	}
	.dark .cdm-editor-crumbs a {
		color: #d4a24c;
	}
	.cdm-editor-crumbs a:hover {
		text-decoration: underline;
		text-underline-offset: 3px;
	}
	.cdm-editor-key {
		font-family: 'JetBrains Mono', ui-monospace, monospace;
		font-size: 0.72rem;
		color: #8f8472;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.dark .cdm-editor-key {
		color: #7c7560;
	}

	.cdm-editor-body {
		max-width: 900px;
		margin: 0 auto;
	}
</style>
