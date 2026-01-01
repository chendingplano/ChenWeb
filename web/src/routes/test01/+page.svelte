<script lang="ts">
	import AutoForm from '$lib/form/AutoForm.svelte';
	import {PromptStoreFormPage} from '$lib/pages/PromptStoreForm';
    import { onMount } from 'svelte';

    onMount(() => {
    })
</script>

<main>
	<h1>Form Example (Dynamic Rendering)</h1>
    <AutoForm pageDef={PromptStoreFormPage}/>
</main>