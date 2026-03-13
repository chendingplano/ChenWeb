<!-- web/src/lib/components/shared-ui/canvas/nodes/FileNode.svelte -->
<script lang="ts">
  import BaseNode from './BaseNode.svelte';
  let { id, data, selected }: { id: string; data: Record<string,any>; selected?: boolean } = $props();
</script>
<BaseNode {id} {selected} label={String(data.label ?? 'File')} icon="📁"
  inputHandles={[]} outputHandles={['file']}
  onConfigure={() => {}} onDuplicate={() => {}} onDelete={() => {}}
>
  <div style="padding:6px 10px; font-size:9px; color:#6b7280;">
    {data.file_type ?? 'txt'} · {data.file_path ? String(data.file_path).split('/').pop() : 'no file'}
  </div>
</BaseNode>
