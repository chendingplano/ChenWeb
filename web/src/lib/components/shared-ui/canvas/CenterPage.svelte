<script lang="ts">
 import { onMount } from 'svelte';
 import { useSvelteFlow } from '@xyflow/svelte';
 let { width, height, size }: { width: number; height: number; size: { width: number; height: number } } = $props();
 const flow = useSvelteFlow();
 onMount(() => {
  let cancelled = false;
  let frame = 0;
  let attempts = 0;
  async function center() {
   if (cancelled) return;
   const zoom = Math.max(0.1, Math.min(1, (width - 80) / size.width, (height - 80) / size.height));
   const done = await flow.setViewport({ x: (width - size.width * zoom) / 2, y: (height - size.height * zoom) / 2, zoom });
   if (!done && !cancelled && attempts++ < 60) frame = requestAnimationFrame(center);
  }
  frame = requestAnimationFrame(center);
  return () => { cancelled = true; cancelAnimationFrame(frame); };
 });
</script>
