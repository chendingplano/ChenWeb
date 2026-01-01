<!-- src/lib/components/prompt-store/AddPromptForm.svelte -->
<script lang="ts">
	  import AutoForm from '$lib/form/AutoForm.svelte';
	  import {PromptStoreFormPage} from '$lib/pages/PromptStoreForm';
    import { onMount } from 'svelte';

    onMount(() => {
    })
</script>

<main>
    <AutoForm pageDef={PromptStoreFormPage}/>
</main>