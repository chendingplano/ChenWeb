<script lang="ts">
 import { Button } from "$lib/components/ui/button/index.js";
 import { Label } from "$lib/components/ui/label/index.js";
 import { Input } from "$lib/components/ui/input/index.js";
 import * as Card from "$lib/components/ui/card/index.js";
 import type { IProcessTableRow } from "$lib/components/common-types.js";

  let { recordSelected = null as IProcessTableRow | null } = $props();

  let isDocNameModified = $derived.by(() => {
    console.log("Checking if doc_name is modified (MID_004_002)");
    if (!recordSelected || !localRecord) return false;
    return localRecord.doc_name !== recordSelected.doc_name;
  });

  // ✅ Derived: always a fresh copy of recordSelected
  let localRecord = $derived.by(() => {
    if (!recordSelected) return null;
    console.log("Deriving localRecord (MID_004_001):", $state.snapshot(recordSelected));
    return { ...recordSelected };
  });

  function handleSubmit() {
    if (!localRecord) return;
    console.log("Submitting form (MID_004_011):", $state.snapshot(localRecord));
    // Here you would typically send `localRecord` to your backend API
    // For now, just log it
  } 
</script>
 
<Card.Root class="w-full max-w-sm">
 <Card.Header>
  <Card.Title>Process Record Details</Card.Title>
  <Card.Description>Record Details</Card.Description>
  <Card.Action>
   <Button variant="link">Refresh</Button>
  </Card.Action>
 </Card.Header>
 <Card.Content>
  <form>
    <div class="flex flex-col gap-6">
      <div class="grid gap-2">
        <Label for="record_uuid">Record UUID</Label>
        <Input 
            id="record_uuid" 
            type="text"
            value = {localRecord?.record_uuid ?? ''}
            readonly 
            class="bg-gray-100 cursor-not-allowed"/>
      </div>
      <div class="grid gap-2">
        <Label for="doc_name">Document Name</Label>
        <Input 
            id="doc_name" 
            type="text" 
            value={localRecord?.doc_name} />
      </div>
      <div class="grid gap-2">
        <Label for="doc_num">Document Number</Label>
        <Input 
            id="doc_num" 
            type="text" 
            value = {localRecord?.doc_num ?? ''}
            readonly
            class="bg-gray-100 cursor-not-allowed"/>
      </div>
      <div class="grid gap-2">
        <Label for="status">Status</Label>
        <Input 
            id="status" 
            type="text" 
            value = {localRecord?.status} />
      </div>
      <div class="grid gap-2">
        <Label for="data_proc_status">Data Proc Status</Label>
        <Input 
            id="data_proc_status" 
            type="text" 
            value = {localRecord?.data_proc_status} />
      </div>
      <div class="grid gap-2">
        <Label for="data_proc_at">Data Proc At</Label>
        <Input 
            id="data_proc_at" 
            type="text" 
            value = {localRecord?.data_proc_at} 
            readonly
            class="bg-gray-100 cursor-not-allowed"/>
      </div>
      <div class="grid gap-2">
        <Label for="chun_proc_status">Chunk Proc Status</Label>
        <Input 
            id="chunk_proc_status" 
            type="text" 
            value = {localRecord?.chunk_proc_status} />
      </div>
      <div class="grid gap-2">
        <Label for="chunk_proc_at">Chunk Proc At</Label>
        <Input 
            id="chunk_proc_at" 
            type="text" 
            value = {localRecord?.chunk_proc_at} 
            readonly
            class="bg-gray-100 cursor-not-allowed"/>
      </div>
      <div class="grid gap-2">
        <Label for="embedding_proc_status">Embedding Proc Status</Label>
        <Input 
            id="embedding_proc_status" 
            type="text" 
            value = {localRecord?.embedding_proc_status} />
      </div>
      <div class="grid gap-2">
        <Label for="embedding_proc_at">Embedding Proc At</Label>
        <Input 
            id="embedding_proc_at" 
            type="text" 
            value = {localRecord?.embedding_proc_at} 
            readonly
            class="bg-gray-100 cursor-not-allowed"/>
      </div>
      <div class="grid gap-2">
        <Label for="es_proc_status">Elastic Search Proc Status</Label>
        <Input 
            id="es_proc_status" 
            type="text" 
            value = {localRecord?.es_proc_status} />
      </div>
      <div class="grid gap-2">
        <Label for="es_proc_at">Elastic Search Proc At</Label>
        <Input 
            id="es_proc_at" 
            type="text" 
            value = {localRecord?.es_proc_at} 
            readonly
            class="bg-gray-100 cursor-not-allowed"/>
      </div>
      <div class="grid gap-2">
        <Label for="parser_source_tname">Parser Source Table name</Label>
        <Input 
            id="parser_source_tname" 
            type="text" 
            value = {localRecord?.parser_source_tname ?? ''} 
            readonly
            class="bg-gray-100 cursor-not-allowed"/>
      </div>
      <div class="grid gap-2">
        <Label for="parser_content_proc_tname">Parser Processed Result Table name</Label>
        <Input 
            id="parser_content_proc_tname" 
            type="text" 
            value = {localRecord?.parser_content_proc_tname ?? ''} 
            readonly
            class="bg-gray-100 cursor-not-allowed"/>
      </div>
      <div class="grid gap-2">
        <Label for="chunk_tablename">Chunk Table name</Label>
        <Input 
            id="chunk_tablename" 
            type="text" 
            value = {localRecord?.chunk_tablename ?? ''} 
            readonly
            class="bg-gray-100 cursor-not-allowed"/>
      </div>
    </div>
  </form>
 </Card.Content>
   <Card.Footer class="flex-col gap-2">
    <Button 
        type="submit" 
        class="w-full"
        disabled={!isDocNameModified}>Submit</Button>
  </Card.Footer>
</Card.Root>