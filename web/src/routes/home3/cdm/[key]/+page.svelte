<script lang="ts">
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { theme } from '$lib/stores/theme.svelte';
	import CdmEditorShell from '$lib/components/cdm/CdmEditorShell.svelte';

	let documentKey = $derived(page.params.key);
	let darkMode = $derived(theme.isDark);
</script>

<div class="cdm-standalone" class:dark={darkMode}>
	<!-- CdmEditorShell renders its own "All documents" crumb + document key
	     once the document loads; onBack sends that crumb's click back to
	     this route's own list rather than just flipping local state, so the
	     URL and the visible pane (list vs. editor) never disagree. -->
	<CdmEditorShell
		{darkMode}
		routed
		initialDocumentKey={documentKey}
		onBack={() => goto('/home3/cdm')}
	/>
</div>

<style>
	.cdm-standalone {
		height: 100vh;
		color-scheme: light;
	}
	.cdm-standalone.dark {
		color-scheme: dark;
	}
</style>
