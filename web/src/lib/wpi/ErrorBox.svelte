<script lang="ts">
	let { title, message, items = [] }: { title: string; message?: string; items?: string[] } = $props();
</script>

<div role="alert" class="border-destructive/50 bg-destructive/5 text-destructive w-full rounded-md border px-3 py-2 text-sm">
	<p class="font-medium">{title}</p>
	{#if message}<p>{message}</p>{/if}
	{#if items.length}
		<ul class="mt-1 list-disc pl-5">
			{#each items as item, i (i)}<li class="break-words">{item}</li>{/each}
		</ul>
	{/if}
</div>
