<script lang="ts">
	// CDM Editor main page. Reached from the "CDM Editor" workspace app tile
	// (config/site/*.toml [[workspace.apps]] key = "cdm_editor") as well as
	// directly. Thin route delegating to a view component, following the
	// home3 convention (see /home3/inputs).
	import { theme } from '$lib/stores/theme.svelte';
	import DocumentListView from '$lib/components/cdm/DocumentListView.svelte';

	let darkMode = $derived(theme.isDark);
</script>

<div class="cdm-list-standalone" class:dark={darkMode}>
	<DocumentListView {darkMode} />
</div>

<style>
	.cdm-list-standalone {
		min-height: 100vh;
		/* The two variables the shared cdm/* components read (they default to
		   a light surface when unset); set here so the editor and this page
		   agree on light/dark rather than each hardcoding one mode. */
		--cdm-surface: #fbf8f0;
		--cdm-muted: #8f8472;
		background: #f5f1e8;
		color-scheme: light;
	}
	.cdm-list-standalone.dark {
		--cdm-surface: #161a22;
		--cdm-muted: #7c7560;
		background: #0e1116;
		color-scheme: dark;
	}
</style>
