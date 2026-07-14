<script lang="ts">
	import { theme } from '$lib/stores/theme.svelte';
	import InputsMgmtView from '$lib/components/home3/inputs-mgmt-view.svelte';

	let darkMode = $derived(theme.isDark);
</script>

<div class="inputs-standalone">
	<InputsMgmtView {darkMode} />
</div>

<style>
	.inputs-standalone {
		height: 100vh;
		overflow: hidden;
	}
</style>
