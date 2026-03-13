<!-- web/src/lib/components/shared-ui/canvas-01.svelte -->
<script lang="ts">
  import type { Flow } from '$lib/types/flow';

  let {
    darkMode = true,
    onClose,
    onCollapseRail,
    onRestoreRail,
  }: {
    darkMode?: boolean;
    onClose: () => void;
    onCollapseRail: () => void;
    onRestoreRail: () => void;
  } = $props();

  import { onMount, onDestroy } from 'svelte';

  onMount(() => {
    onCollapseRail();
  });

  onDestroy(() => {
    onRestoreRail();
  });
</script>

<div class="fixed inset-0 z-50 flex items-center justify-center"
  style="background: rgba(0,0,0,0.5);">
  <div style="color:white; font-size:18px;">
    Flow Canvas — coming soon
    <button onclick={onClose} style="margin-left:16px; padding:4px 8px; background:#6366f1; border:none; border-radius:4px; color:white; cursor:pointer;">
      Close
    </button>
  </div>
</div>
