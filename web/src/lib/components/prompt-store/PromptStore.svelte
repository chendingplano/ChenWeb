<!-- PromptStore.svelte -->
<script lang="ts">
  import {onMount} from 'svelte'
  import QueryArea from './QueryArea.svelte';
  import PromptList from './PromptList.svelte';
  import AddPromptForm from './AddPromptForm2.svelte';
  import UploadPrompts from './UploadPrompts.svelte';
  import {GetStoreByName} from "@chendingplano/shared"
  import type {JimoRequest} from '@chendingplano/shared';

  let no_data = true
  let is_loading = $state<boolean>(false)
  const InMemStore = GetStoreByName('prompt_store', 'list')

  onMount(async () => {
    try {
      const req : JimoRequest = {
        request_type:   "db_opr",
        action:         "query",
        resource_name:  "prompt_store",
        resource_opr:   "init_select",
        conditions:     "",
        resource_info:  "",
      }

      is_loading = true
      const resp = await fetch("/shared_api/v1/jimo_req", {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify(req),
            credentials: 'include'
            });

      // Parse JSON returned from Go API
      const json_value = await resp.json()

      if (!resp.ok) {
          alert("Failed load prompts from DB, " + json_value.error_msg + " (" + json_value.loc + ")")
          console.log('Failed to fetch chart data (CWB_PST_044)');
          is_loading = false
          no_data = true
          return
      }

      if (!json_value.status)
      {
        console.log('Failed retrieving data (CWB_PST_058), error:' + 
          json_value.error_msg + ", loc:" + json_value.loc)
        is_loading = false
        no_data = true
        return
      }
    } catch (e) {
		  if (e instanceof Error) {
			  console.log("Error fetching data (CWB_PST_066):", e.message);
		  } else {
			  console.log("Error fetching data (CWB_PST_068):", String(e));
		  }
    }
  })

  /*
  // Filter prompts based on query
  function handleQuery(query : CustomEvent<any>) {
    const cached_records = $InMemStore.CachedRecords;
    const filtered = cached_records.filter(record => {
      return Object.entries(query).every(([key, value]) => {
        if (!value) return true;
        
        switch (key) {
        case 'prompt_id':
             const str_value = (value as bigint)? String(value) : ''
             return record.prompt_id.toString().includes(str_value);

        case 'prompt_name':
             return record.prompt_name.toLowerCase().includes((value as string).toLowerCase());

        case 'prompt_desc':
        case 'prompt_content':
             return record[key].toLowerCase().includes((value as string).toLowerCase());

        case 'prompt_keywords':
        case 'prompt_tags':
             return record[key].toLowerCase().includes((value as string).toLowerCase());

        case 'creator':
        case 'updater':
             return record[key].toLowerCase().includes((value as string).toLowerCase());

        case 'Status':
             return record.prompt_status === value;

        case 'created_at':
        case 'updated_at':
             if (!record[key]) return false;
             return record[key].includes(value as string);

         default:
             return true;
        }
      });
    });
    
    $InMemStore = {
        ...$InMemStore,
        CachedRecords: filtered
    }
  }
  */
  
  // Add new prompt
  /*
  // Delete prompt
  function handleDeletePrompt(event : CustomEvent<number>) {
    if (confirm('Are you sure you want to delete this prompt?')) {
      const prompt_id = event.detail as number
      const stored_value = $InMemStoreCrtRecords;
      const prompts = stored_value.CachedPrompts.filter(p => p.prompt_id !== prompt_id);

      $InMemStoreCrtRecords = {
        ...$InMemStoreCrtRecords,
        CachedPrompts: prompts
      }
    }
  }
  */
</script>

<div class="prompt-store">
  <div class="view-controls">
    <button 
      class:active={$InMemStore.CrtView === 'list'}
      onclick={() => {
        $InMemStore = {
          ...$InMemStore,
          CrtView: 'list'
        }
        console.log("'list' button clicked (CWB_PST_134), CrtView:" + $InMemStore.CrtView)
      }}
    >
      View Prompts
    </button>
    <button 
      class:active={$InMemStore.CrtView === 'add'}
      onclick={() => {
        $InMemStore = {
          ...$InMemStore,
          CrtView: 'add'
        }
      }}
    >
      Add New Prompt
    </button>
    <button 
      class:active={$InMemStore.CrtView === 'upload'}
      onclick={() => {
        $InMemStore = {
          ...$InMemStore,
          CrtView: 'upload'
        }
      }}
    >
      Upload Prompts
    </button>
  </div>
  
  {#if $InMemStore.CrtView === 'list'}
    <QueryArea/>
    <PromptList/>
  {:else if $InMemStore.CrtView === 'add'}
    <AddPromptForm/>
  {:else if $InMemStore.CrtView === 'upload'}
    <UploadPrompts />
  {/if}
</div>

<style>
  .prompt-store {
    margin-top: 20px;
  }
  
  .view-controls {
    display: flex;
    gap: 10px;
    margin-bottom: 20px;
    border-bottom: 1px solid #ddd;
    padding-bottom: 10px;
  }
  
  .view-controls button {
    padding: 10px 20px;
    border: 1px solid #ddd;
    background: white;
    cursor: pointer;
    border-radius: 4px;
  }
  
  .view-controls button.active {
    background: #007bff;
    color: white;
    border-color: #007bff;
  }
  
  .view-controls button:hover {
    background: #f8f9fa;
  }
  
  .view-controls button.active:hover {
    background: #0056b3;
  }
</style>