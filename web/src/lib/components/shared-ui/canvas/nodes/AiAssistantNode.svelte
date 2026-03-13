<!-- web/src/lib/components/shared-ui/canvas/nodes/AiAssistantNode.svelte -->
<script lang="ts">
  import BaseNode from './BaseNode.svelte';
  let { id, data, selected }: { id: string; data: Record<string,any>; selected?: boolean } = $props();
</script>
<BaseNode {id} {selected} label={String(data.label ?? 'AI Assistant')} icon="🤖"
  inputHandles={['text','context']} outputHandles={['response']}
  onConfigure={() => {}} onDuplicate={() => {}} onDelete={() => {}}
>
  <div style="padding:6px 10px; font-size:9px; color:#6b7280;">
    {data.model ?? 'gpt-4o'}
    {#if data.system_prompt}<span style="margin-left:4px;color:#4b5563;">· prompt set</span>{/if}
  </div>
</BaseNode>
