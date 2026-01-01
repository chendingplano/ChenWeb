<script lang="ts">
  import { onMount } from 'svelte';

  let Calendar = $state<any>(null);
  let TimeGrid = $state<any>(null);
  let options = $state({
    view: 'timeGridWeek',
    events: [
      // your events
    ]
  });

  onMount(async () => {
    const mod = await import('@event-calendar/core');
    Calendar = mod.Calendar;
    TimeGrid = mod.TimeGrid;
  });
</script>

{#if Calendar}
  <Calendar plugins={[TimeGrid]} {options} />
{/if}