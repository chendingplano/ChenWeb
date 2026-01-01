<!-- Calendar.svelte -->
<!--script lang="ts">
  import { onMount } from 'svelte';
  import Calendar from '$lib/components/ui/calendar/calendar-02.svelte';
</script>

<div>
    <Calendar width={800} height={520} />
</div>
-->
<script lang="ts">
	import AutoForm from '$lib/form/AutoForm.svelte';
	import {SchedulePicker} from '$lib/pages/SchedulePicker';
    import { onMount } from 'svelte';

    onMount(() => {
    })
</script>

<main>
	<h1>Form Example (Dynamic Rendering)</h1>
    <AutoForm pageDef={SchedulePicker}/>
</main>