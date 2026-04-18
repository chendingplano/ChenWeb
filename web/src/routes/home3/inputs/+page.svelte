<script lang="ts">
	import { page } from '$app/state';
	import InputsMgmtView from '$lib/components/home3/inputs-mgmt-view.svelte';

	let darkMode = $derived(page.url.searchParams.get('dark') !== '0');
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
