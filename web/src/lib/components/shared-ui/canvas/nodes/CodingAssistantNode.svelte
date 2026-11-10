<!-- web/src/lib/components/shared-ui/canvas/nodes/CodingAssistantNode.svelte -->
<script lang="ts">
  import BaseNode from './BaseNode.svelte';
  let { id, data, selected }: { id: string; data: Record<string,any>; selected?: boolean } = $props();
</script>
<BaseNode {id} {selected} label={String(data.label ?? 'Coding Assistant')} icon="💻"
  inputHandles={['code','context']} outputHandles={['code','explanation']}
>
  <div style="padding:6px 10px; font-size:9px; color:var(--cv-text-muted);">
    {data.language ?? 'typescript'} · {data.model ?? ''}
  </div>
</BaseNode>
