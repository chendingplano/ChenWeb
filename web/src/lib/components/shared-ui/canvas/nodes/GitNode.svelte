<!-- web/src/lib/components/shared-ui/canvas/nodes/GitNode.svelte -->
<script lang="ts">
  import BaseNode from './BaseNode.svelte';
  let { id, data, selected }: { id: string; data: Record<string,any>; selected?: boolean } = $props();
</script>
<BaseNode {id} {selected} label={String(data.label ?? 'GIT')} icon="🗂"
  inputHandles={['files']} outputHandles={['result']}
  onConfigure={() => {}} onDuplicate={() => {}} onDelete={() => {}}
>
  <div style="padding:6px 10px; font-size:9px; color:#6b7280;">
    {data.operation ?? 'status'} · {data.branch ?? 'main'}
  </div>
</BaseNode>
