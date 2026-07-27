<script lang="ts">
	// CDM Editor main page. Reached from the "CDM Editor" workspace app tile
	// (config/site/*.toml [[workspace.apps]] key = "cdm_editor") as well as
	// directly. Thin route delegating to CdmEditorShell, the one CDM Editor
	// design shared with /home3/cdm/[key] and the /development, Tools > CDM
	// Editor embed (content-panel.svelte).
	import { theme } from '$lib/stores/theme.svelte';
	import CdmEditorShell from '$lib/components/cdm/CdmEditorShell.svelte';

	let darkMode = $derived(theme.isDark);
</script>

<div class="cdm-standalone" class:dark={darkMode}>
	<!-- No onBack: opening a document from this route's own list is local
	     state (CdmEditorShell never navigates for it), so "back to list"
	     never leaves /home3/cdm in the first place. Only /home3/cdm/[key],
	     a real deep-linked URL, needs onBack to leave its own route. -->
	<CdmEditorShell {darkMode} routed />
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
