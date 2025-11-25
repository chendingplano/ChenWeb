<script lang="ts">
    import { FormItemMap } from "./FormItemMap";
    import type {ComponentDef} from "./FormTypes"
    import type {JimoResponse} from '@chendingplano/shared'
    import {GetStoreByName} from '@chendingplano/shared'

    let { componentName, schema = $bindable(), formData  = $bindable()}:
        {
            componentName: string,
            schema: ComponentDef[];
            formData: {[key: string]: unknown}
        } = $props()

    let model = $state<Record<string, unknown>>(
        Object.fromEntries(
            Object.entries(schema).map(([key, def]) => {
                const field = def as ComponentDef
                return [key, field.default ?? ""]
            }
        )));

    const {InMemStoreCrtRecords, InMemStoreCrtRecord, InMemStoreCrtView} = GetStoreByName(componentName, 'list')
    let maxFormWidth = "1200px"
    let itemComponents: any[] = [];

    const items = schema.map(def => ({
        ...def,
        Comp: FormItemMap[def.type]
    }));

    let errors          = $state<{[key:string]: string}>({});
    let error_str       = $state('');
    let isSubmitting    = $state(false);
  
    function validatePromptName(name: string) {
        const regex = /^[a-zA-Z0-9_]+$/;
        return regex.test(name);
    }
  
    function validateForm() {
        let hasError = false;
        error_str = '';

        for (let i = 0; i < itemComponents.length; i++) {
            const comp = itemComponents[i];
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
    
        try {
            // In a real app, you would call your API here
            // await promptService.createPrompt(newPrompt);
            console.log("Add prompt")
            formData.creator_loc = "CWB_APF_078"
            formData.updater_loc = "CWB_APF_078"
            const response = await fetch("/shared_api/v1/add_prompt", {
                method: "POST",
                headers: { "Content-Type": "application/json" },
                body: JSON.stringify(formData),
                credentials: 'include'
                });

            if (response.ok) {
                const errorData = await response.json() as JimoResponse;
                alert("Prompt added to the database (CWB_PST_080+" + errorData.loc + ")")

                $InMemStoreCrtRecords = {
                    ...$InMemStoreCrtRecords,
                    CachedPrompts: [...$InMemStoreCrtRecords.CachedPrompts, formData]
                }
                InMemStoreCrtView.set('list');
            }
            else {
              if (response.status === 401) {
                alert("Operation rejected (401):" + response.statusText + " (CWB_APF_092)")
                isSubmitting = false
                return
              }

              if (response.status === 404) {
                alert("Route not found (404):" + response.statusText + " (CWB_APF_098)")
                isSubmitting = false
                return
              }

              const errorData = await response.json() as JimoResponse;
              alert(errorData.error_msg + "(" + errorData.loc + ")")
              isSubmitting = false
              return
            }
          
            // Reset form
            formData = {}
          
            // Show success message
            isSubmitting = false
        } catch (NetworkError) {
            alert('Network error: ' + (NetworkError instanceof Error ? NetworkError.message : 'unknown'));
            isSubmitting = false
        }
    }
  
    function handleCancel(event: Event) {
        event.preventDefault()
        // Reset form and go back
        formData = {}
        errors = {};
    
        // Emit cancel event to parent
        // dispatch('cancel');
    }
</script>

<div class="form-container" style="--form-max-width: {maxFormWidth}">
  <h2>Add New Prompt</h2>
  
  <form onsubmit={handleSubmit}>
    <div class="form-grid">
        {#each items as item, i}
          <item.Comp
              itemDef={item}
              bind:this={itemComponents[i]}
              bind:formData={formData}
          />
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
        {isSubmitting ? 'Creating...' : 'Create Prompt'}
      </button>
    </div>
  </form>
</div>

<pre>{JSON.stringify(model, null, 2)}</pre>

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