<!-- web/src/lib/components/shared-ui/canvas/nodes/MediaNode.svelte -->
<script lang="ts">
  import BaseNode from './BaseNode.svelte';
  let { id, data, selected }: { id: string; data: Record<string,any>; selected?: boolean } = $props();
</script>
<BaseNode {id} {selected} label={String(data.label ?? 'Media')} icon="🎬"
  inputHandles={[]} outputHandles={['media']}
  onConfigure={() => {}} onDuplicate={() => {}} onDelete={() => {}}
>
  <div style="padding:6px 10px; font-size:9px; color:var(--cv-text-muted);">
    {data.media_type ?? 'image'}
  </div>
</BaseNode>
