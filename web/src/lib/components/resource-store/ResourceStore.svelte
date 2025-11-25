<!-- ResourceStore.svelte -->
<script lang="ts">
    import { writable } from 'svelte/store';
    import {onMount} from 'svelte'
    import QueryArea from './QueryArea.svelte';
    import ResourceList from './ResourceList.svelte';
    import GenericForm from '$lib/components/forms/form-twocolumn.svelte';
    import UploadResources from './UploadResources.svelte';
    import {db_store} from "@chendingplano/shared"
    import type {RecordInfo} from "@chendingplano/shared"
    import {GetStoreByName} from "@chendingplano/shared"

    let no_data = true
    let is_loading = $state<boolean>(false)

    const InMemStore = GetStoreByName("resource_store", "list")
    const resourceFormData = [
    { 
        id: 'resourceName', 
        label: 'Resource Name', 
        type: 'text', 
        required: true, 
        help_text: 'letters, digits and underscores only',
        helpText: 'Enter a unique name for this prompt',
        validation: {
            required: true,
            pattern: /^[a-zA-Z0-9_]+$/,
            message: 'Resource name can only contain letters, numbers, and underscores'
        }
    },
    { 
        id: 'description', 
        label: 'Description', 
        type: 'textarea', 
        helpText: 'Provide a brief description of the prompt',
        help_text: '' ,
        validation: {
            required: true,
            message: 'Resource description is required'
        }
    },
    { 
        id: 'category', 
        label: 'Category', 
        type: 'select', 
        required: true, 
        options: ['database', 'access controls', 'ETL', 'customized business logic'], 
        help_text: '',
        helpText: 'Select the category that best fits this prompt',
        validation: {
            required: true,
            message: 'Category is required'
        }
    },
    { 
        id: 'isActive', 
        label: 'Active', 
        type: 'checkbox', 
        helpText: 'Check this box to make the prompt active',
        validation: {
            required: false,
        }
    }
  ];

  onMount(async () => {
      is_loading = true
      const selected_fields: string[] = []
      const query_rslts = await db_store.retrieveRecords("resource_store", selected_fields)

      if (!query_rslts.status) {
          alert("Failed load resources from DB, " + query_rslts.error_msg + " (" + query_rslts.loc + ")")
          console.log('Failed to fetch chart data (CWB_PST_044)');
          is_loading = false
          no_data = true
          return
      }

      const records = query_rslts.results
      InMemStore.update(currentState => ({
        ...currentState,
        CashedRecords: records
      }))
  })

  /*
  // Filter resources based on query
  function handleQuery(query : CustomEvent<any>) {
    const cached_records = $InMemStoreCrtRecords.CachedResources as RecordInfo[];
    if (!cached_records) {
        return
    }
    const filtered = cached_records.filter(resource => {
      return Object.entries(query).every(([key, value]) => {
        if (!value) return true;
        
        switch (key) {
        case 'resource_id':
             const str_value = (value as bigint)? String(value) : ''
             return resource.resource_id.toString().includes(str_value);

        case 'resource_name':
             return resource.resource_name.toLowerCase().includes((value as string).toLowerCase());

        case 'resource_desc':
             return resource[key].toLowerCase().includes((value as string).toLowerCase());

        case 'creator':
        case 'updater':
             return resource[key].toLowerCase().includes((value as string).toLowerCase());

        case 'Status':
             return resource.resource_status === value;

        case 'created_at':
        case 'updated_at':
             if (!resource[key]) return false;
             return resource[key].includes(value as string);

         default:
             return true;
        }
      });
    });
    
    $InMemStoreCrtRecords = {
        ...$InMemStoreCrtRecords,
        CachedResources: filtered
    }
  }
  */
</script>

<div class="resource-store">
  <div class="view-controls">
    <button 
      class:active={$InMemStore.CrtView === 'list'}
      onclick={() => {
        $InMemStore = {
          ...$InMemStore,
          CrtView: 'list'
        }
      }}
    >
      View Resources
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
      Add New Resource
    </button>
    <button 
      class:active={$InMemStore.CrtView === 'upload'}
      onclick={() => {
        $InMemStore = {
          ...$InMemStore,
          CrtView: 'add'
        }
      }}
    >
      Upload Resources
    </button>
  </div>
  
  {#if $InMemStore.CrtView === 'list'}
    <QueryArea />
    <ResourceList />
  {:else if $InMemStore.CrtView === 'add'}
    <GenericForm componentName="resource_store"/>
  {:else if $InMemStore.CrtView === 'upload'}
    <UploadResources/>
  {/if}
</div>

<style>
  .resource-store {
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