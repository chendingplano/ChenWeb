<!-- web/src/lib/components/shared-ui/canvas/nodes/TextNode.svelte -->
<script lang="ts">
  import BaseNode from './BaseNode.svelte';
  let { id, data, selected }: { id: string; data: Record<string,any>; selected?: boolean } = $props();
</script>
<BaseNode {id} {selected} label={String(data.label ?? 'Text')} icon="📝"
  inputHandles={[]} outputHandles={['text']}
  onConfigure={() => {}} onDuplicate={() => {}} onDelete={() => {}}
>
  <div style="padding:6px 10px; font-size:9px; color:#6b7280; max-width:180px; overflow:hidden; text-overflow:ellipsis; white-space:nowrap;">
    {data.content ? String(data.content).slice(0,40) : 'No content yet'}
  </div>
</BaseNode>
