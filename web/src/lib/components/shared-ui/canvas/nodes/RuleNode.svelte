<!-- web/src/lib/components/shared-ui/canvas/nodes/RuleNode.svelte -->
<script lang="ts">
  import BaseNode from './BaseNode.svelte';
  let { id, data, selected }: { id: string; data: Record<string,any>; selected?: boolean } = $props();
</script>
<BaseNode {id} {selected} label={String(data.label ?? 'Rule')} icon="📋"
  inputHandles={['data']} outputHandles={['pass','fail']}
  onConfigure={() => {}} onDuplicate={() => {}} onDelete={() => {}}
>
  <div style="padding:6px 10px; font-size:9px; color:#6b7280; overflow:hidden; text-overflow:ellipsis; white-space:nowrap; max-width:160px;">
    {data.description || data.rule_expression || 'no rule'}
  </div>
</BaseNode>
