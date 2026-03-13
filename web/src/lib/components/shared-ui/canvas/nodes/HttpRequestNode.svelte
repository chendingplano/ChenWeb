<!-- web/src/lib/components/shared-ui/canvas/nodes/HttpRequestNode.svelte -->
<script lang="ts">
  import BaseNode from './BaseNode.svelte';
  let { id, data, selected }: { id: string; data: Record<string,any>; selected?: boolean } = $props();
</script>
<BaseNode {id} {selected} label={String(data.label ?? 'HTTP Request')} icon="📡"
  inputHandles={['body']} outputHandles={['response']}
  onConfigure={() => {}} onDuplicate={() => {}} onDelete={() => {}}
>
  <div style="padding:6px 10px; font-size:9px; color:#6b7280; display:flex; gap:4px; align-items:center;">
    <span style="background:#1e2535; border-radius:3px; padding:1px 5px; color:#94a3b8;">{data.method ?? 'GET'}</span>
    <span style="overflow:hidden; text-overflow:ellipsis; white-space:nowrap; max-width:110px;">{data.url ?? 'no URL'}</span>
  </div>
</BaseNode>
