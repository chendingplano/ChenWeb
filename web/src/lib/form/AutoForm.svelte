<script lang="ts">
    import { FormItemMap } from "./FormItemMap";
    import type {PageDef, ComponentDef} from "./FormTypes"
    import type {JimoResponse} from '@chendingplano/shared'
    import {GetStoreByName} from '@chendingplano/shared'
    import {db_store} from "@chendingplano/shared"
    import { onMount } from 'svelte';

    let { pageDef }:
        {
            pageDef: PageDef,
        } = $props()

    const storeName = pageDef.storeName
    const formItems = pageDef.components as ComponentDef[]
    const InMemStore = GetStoreByName(storeName)
    const storeLabel = pageDef.storeLabel || storeName
    const selectedFieldNames = formItems.map((form_item) => {
        if (!form_item.fieldName) {
            alert("Missing fieldName (CWB_AFM_018):" + form_item)
        }
        else {
          return form_item.fieldName
        }
    })

    let formData = $state<Record<string, unknown>>({})
    let loadedComponents = $state<(any | null)[]>(Array(formItems.length).fill(null));

    let defaultFormData = $state<Record<string, unknown>>(
        Object.fromEntries(
            Object.entries(formItems).map(([key, def]) => {
                const field = def as ComponentDef
                return [key, field.default ?? ""]
            }
        )));

    let maxFormWidth = "1200px"

    let items = $state(formItems.map(def => ({
        ...def,
    })));

    $effect(() => {
        items.forEach((item, i) => {
            if (!item.Comp) {
                const loader = FormItemMap[item.type];
                if (loader) {
                    loader().then(mod => {
                        items[i].Comp = mod.default;   // triggers reactive update
                    });
                }
            }
        });
    });

    let errors          = $state<{[key:string]: string}>({});
    let error_str       = $state('');
    let isSubmitting    = $state(false);
  
    onMount(() => {
        console.log("In AutoForm (CWB_AFM_061)")
    })

    function validateForm() {
        let hasError = false;
        error_str = '';

        for (let i = 0; i < loadedComponents.length; i++) {
            const comp = loadedComponents[i];
            if (comp?.checkValue) {
                const errorMsg: string = comp.checkValue();
                if (errorMsg !== '') {
                    hasError = true;
                    if (!error_str) {
                      error_str = errorMsg
                    }
                    else {
                      error_str += "\n  - " + errorMsg
                    }
                }
            }
        }

        return !hasError;
    }
  
    async function handleSubmit(event : Event) {
        event.preventDefault()
        if (!validateForm()) {
            alert("Form check failed (CWB_AFM_084):\n  - " + error_str)
            return;
        }
    
        isSubmitting = true;
        formData.creator_loc = "CWB_APF_078"
        formData.updater_loc = "CWB_APF_078"
        const response = await db_store.saveRecord("prompt_store", formData) as JimoResponse
        if (response.status) {
            alert("Prompt added to the database (CWB_AFM_098:" + response.loc + ")")
            $InMemStore = {
                ...$InMemStore,
                CachedRecords: [...$InMemStore.CachedRecords, formData],
                CrtView: 'list',
                CrtRecord: {}
            }

            isSubmitting = false
            return
        }

        alert(response.error_msg + "(" + response.loc + ":CWB_AFM_117)")
        isSubmitting = false
          
        // Reset form
        formData = defaultFormData
    }

    function resetFormData() {
        formData = {}
    }
  
    function handleCancel(event: Event) {
        event.preventDefault()
        // Reset form and go back
        formData = {}
        errors = {};
    }
</script>

<div class="form-container" style="--form-max-width: {maxFormWidth}">
  <h2>Add New {storeLabel}</h2>
  
  <form onsubmit={handleSubmit}>
    <div class="form-grid">
        {#each items as item, i}
            {#if items[i].Comp}
                <item.Comp
                    itemDef={items[i]}
                    bind:this={loadedComponents[i]}
                    bind:formData={formData}
                />
            {:else}
                <div class="loading-placeholder">
                    Loading…
                </div>
            {/if}
        {/each}
    </div>
    
    <div class="form-actions">
      <button
        type="button"
        onclick={handleCancel}
        class="cancel-btn"
        disabled={isSubmitting}
      >
        Cancel
      </button>
      <button
        type="submit"
        class="submit-btn"
        disabled={isSubmitting}
      >
        {isSubmitting ? 'Creating...' : `Create ${storeLabel}`}
      </button>
    </div>
  </form>
</div>

<pre>{JSON.stringify(formData, null, 2)}</pre>

<style>
  :root {
    --form-max-width: 640px;
  }

  .form-container {
    width: 100%;
    max-width: var(--form-max-width); /* ← variable */
    margin: 0 auto;                  /* ← centers when wider space */
    padding: 1rem;
  }

  .form-container h2 {
    margin-bottom: 25px;
    color: #333;
    border-bottom: 2px solid #007bff;
    padding-bottom: 10px;
    align-content: center;
    text-align: center;
    font-size:large;
    font-style: bold;
  }
  
  /* Make it one-column on small screens */
  @media (max-width: 600px) {
    .form-grid {
      grid-template-columns: 1fr;
    }
  }

  .form-grid {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 20px;
    margin-bottom: 25px;
  }
  
  .form-actions {
    display: flex;
    justify-content: flex-end;
    gap: 15px;
    padding-top: 20px;
    border-top: 1px solid #eee;
  }
  
  .cancel-btn,
  .submit-btn {
    padding: 12px 30px;
    border: none;
    border-radius: 4px;
    cursor: pointer;
    font-size: 0.9em;
    transition: all 0.3s;
  }
  
  .cancel-btn {
    background: #6c757d;
    color: white;
  }
  
  .cancel-btn:hover:not(:disabled) {
    background: #545b62;
  }
  
  .submit-btn {
    background: #007bff;
    color: white;
  }
  
  .submit-btn:hover:not(:disabled) {
    background: #0056b3;
  }
  
  .cancel-btn:disabled,
  .submit-btn:disabled {
    opacity: 0.6;
    cursor: not-allowed;
  }
</style>