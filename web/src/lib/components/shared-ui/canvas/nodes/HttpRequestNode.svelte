<!-- web/src/lib/components/shared-ui/canvas/nodes/HttpRequestNode.svelte -->
<script lang="ts">
	import { m } from '$lib/paraglide/messages.js';
  import BaseNode from './BaseNode.svelte';
  let { id, data, selected }: { id: string; data: Record<string,any>; selected?: boolean } = $props();
</script>
<BaseNode {id} {selected} label={String(data.label ?? 'HTTP Request')} icon="📡"
  inputHandles={['body']} outputHandles={['response']}
>
  <div style="padding:6px 10px; font-size:9px; color:var(--cv-text-muted); display:flex; gap:4px; align-items:center;">
    <span style="background:var(--cv-surface); border-radius:3px; padding:1px 5px; color:var(--cv-text-2);">{data.method ?? 'GET'}</span>
    <span style="overflow:hidden; text-overflow:ellipsis; white-space:nowrap; max-width:110px;">{data.url ?? m.httprequestnode_no_url()}</span>
  </div>
</BaseNode>
