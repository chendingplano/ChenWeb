<!-- src/lib/components/resource-store/AddResourceForm.svelte -->
<script lang="ts">
	import { AlphaFormat } from "three";
  import {GetStoreByName} from "@chendingplano/shared"
  
  const InMemStore = GetStoreByName("resource_map", 'list')
  let form_errors : { [key: string]: string}= {};
  let error_str : string = '';
  let isSubmitting = false;
  
  const formFields = [
    { 
        id: 'resourceName', 
        label: 'Resource Name', 
        type: 'text', 
        required: true, 
        help_text: 'letters, digits and underscores only',
        helpText: 'Enter a unique name for this prompt'
    },
    { 
        id: 'description', 
        label: 'Description', 
        type: 'textarea', 
        helpText: 'Provide a brief description of the prompt',
        help_text: '' 
    },
    { 
        id: 'category', 
        label: 'Category', 
        type: 'select', 
        required: true, 
        options: ['database', 'access controls', 'ETL', 'customized business logic'], 
        help_text: '',
        helpText: 'Select the category that best fits this prompt'
    },
    { 
        id: 'isActive', 
        label: 'Active', 
        type: 'checkbox', 
        helpText: 'Check this box to make the prompt active'
    }
  ];

  // State for tracking which tooltip is visible
  let activeTooltip: string | null = null;
  let tooltipPosition = { x: 0, y: 0 };
  
  function showTooltip(fieldId: string, e: MouseEvent) {
    activeTooltip = fieldId;
    updateTooltipPosition(e);
  }
  
  function updateTooltipPosition(e: MouseEvent) {
    tooltipPosition = {
      x: e.pageX + 10,
      y: e.pageY - 30
    };
  }
  
  function hideTooltip() {
    activeTooltip = null;
  }
  
  function validateResourceName(name: string) {
    const regex = /^[a-zA-Z0-9_]+$/;
    return regex.test(name);
  }
  
  function validateForm() {
    error_str = ''
    const crt_record = $InMemStore.CrtRecord
    if (!crt_record.resource_name.trim()) {
      form_errors.ResourceName = 'Resource name is required';
      error_str += form_errors.ResourceName + (error_str === '') ? '' : ","
    } else if (!validateResourceName(crt_record.resource_name)) {
      form_errors.ResourceName = 'Resource name can only contain letters, numbers, and underscores';
      error_str += form_errors.ResourceName + (error_str === '') ? '' : ","
    }
    
    if (!crt_record.prompt_content.trim()) {
      form_errors.PromptContent = 'Prompt content is required';
      error_str += form_errors.PromptContent + (error_str === '') ? '' : ","
    }
    
    return Object.keys(form_errors).length === 0;
  }
  
  async function handleSubmit(event : Event) {
    event.preventDefault();

    if (!validateForm()) {
      alert("Form check failed:" + error_str)
      return;
    }
    
    isSubmitting = true;
    
    try {
        // In a real app, you would call your API here
        console.log("Add prompt")
        const response = await fetch("/shared_api/v1/add_prompt", {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify($InMemStore.CrtRecord),
            credentials: 'include'
            });

        if (response.ok) {
            const errorData = await response.json();
            alert("Prompt added to the database (CWB_PST_080+" + errorData.loc + ")")
            $InMemStore = {
                ...$InMemStore,
                CachedRecords: [...$InMemStore.CachedRecords, $InMemStore.CrtRecord],
                CrtRecord: {},
                CrtView: 'list'
            }
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

          const errorData = await response.json();
          alert(errorData.error_msg + "(" + errorData.loc + ")")
          isSubmitting = false
          return
        }
      
        // Show success message
        isSubmitting = false
    } catch (NetworkError) {
        alert('Network error: ' + (NetworkError instanceof Error ? NetworkError.message : 'unknown'));
        isSubmitting = false
    }
  }

  function handleCancel() {
    // Reset form and go back
    form_errors = {};
    
    // Emit cancel event to parent
    // dispatch('cancel');
  }
</script>

<div class="add-prompt-form">
  <h2>Add New Resource</h2>
  
  <form onsubmit={handleSubmit}>
    <div class="form-grid">
      {#each formFields as field}
        <div class="form-group">
          <label for={field.id}>{field.label}{field.required ? ' *' : ''}>
              <span 
                class="help-icon"
                role="button"
                aria-label="Show help for {field.label}"
                tabindex="0"
                onmouseenter={(e) => showTooltip(field.id, e)}
                onmouseleave={hideTooltip}
                onmousemove={(e) => updateTooltipPosition(e)}
            >
              ?
              </span>
            </label>
          
          {#if field.type === 'select'}
            <select 
                bind:value={$InMemStore.CrtRecord}
                class:error={form_errors[field.id]}>
              <option value="">Select an option</option>
              {#each field.options as option}
                <option value={option}>{option}</option>
              {/each}
            </select>
            {#if field.helpText}
              <div class="help-text">{field.helpText}</div>
            {/if}
          {:else if field.type === 'checkbox'}
            <div class="checkbox-container">
              <input
                id={field.id}
                type="checkbox"
                bind:checked={$InMemStore.CrtRecord[field.id]}
                class:error={form_errors[field.id]}
              />
              <label for={field.id} class="checkbox-label">{field.label}</label>
            </div>
            {#if field.helpText}
              <div class="help-text">{field.helpText}</div>
            {/if}
          {:else if field.type === 'textarea'}
            <textarea
              id={field.id}
              bind:value={$InMemStore.CrtRecord[field.id]}
              class:error={form_errors.PromptDesc}
              placeholder="Brief description of the prompt's purpose"
              rows="3">
            </textarea>
            {#if field.helpText}
              <div class="help-text">{field.helpText}</div>
            {/if}
            {#if form_errors[field.id]}
              <span class="error-message">{form_errors[field.id]}</span>
            {/if}
          {:else}
            <input
              id={field.id}
              type={field.type}
              bind:value={$InMemStore.CrtRecord[field.id]}
              class:error={form_errors[field.id]}
            />
            {#if field.helpText}
              <div class="help-text">{field.helpText}</div>
            {/if}
          {/if}

          {#if activeTooltip === field.id}
            <div 
              class="tooltip"
              style="left: {tooltipPosition.x}px; top: {tooltipPosition.y}px;"
            >
              {field.helpText}
            </div>
    {/if}
        </div>
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
  
  <pre>{JSON.stringify($InMemStore.CrtRecord, null, 2)}</pre>
</div>

<style>
  .add-prompt-form {
    background: white;
    padding: 30px;
    border-radius: 8px;
    box-shadow: 0 2px 10px rgba(0,0,0,0.1);
  }
  
  .add-prompt-form h2 {
    margin-bottom: 25px;
    color: #333;
    border-bottom: 2px solid #007bff;
    padding-bottom: 10px;
  }
  
  .form-grid {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 20px;
    margin-bottom: 25px;
  }
  
  .form-group {
    display: flex;
    flex-direction: column;
    position: relative;
  }
  
  .form-group label {
    font-weight: bold;
    margin-bottom: 8px;
    color: #555;
    font-size: 0.9em;
  }
  
  .form-group input,
  .form-group select,
  .form-group textarea {
    padding: 10px;
    border: 1px solid #ddd;
    border-radius: 4px;
    font-size: 0.9em;
    transition: border-color 0.3s;
  }

  .form-group input:focus,
  .form-group select:focus,
  .form-group textarea:focus {
    outline: none;
    border-color: #007bff;
    box-shadow: 0 0 0 2px rgba(0,123,255,0.25);
  }
  
  .form-group input.error,
  .form-group textarea.error {
    border-color: #dc3545;
  }
  
  .error-message {
    color: #dc3545;
    font-size: 0.8em;
    margin-top: 5px;
  }
  
  .help-text {
    color: #6c757d;
    font-size: 0.8em;
    margin-top: 5px;
    font-style: italic;
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

  .checkbox-container {
    display: flex;
    align-items: top;
    gap: 0.5rem; /* Space between checkbox and label */
  }

  .checkbox-label {
    margin: 8px;
    user-select: none; /* Prevents text selection when clicking label */
  }

  .help-icon {
    display: inline-block;
    width: 16px;
    height: 16px;
    background-color: #007bff;
    color: white;
    border-radius: 50%;
    font-size: 10px;
    text-align: center;
    line-height: 16px;
    margin-left: 5px;
    cursor: help;
    user-select: none;
  }
  
  .tooltip {
    position: fixed;
    background-color: #333;
    color: white;
    padding: 8px 12px;
    border-radius: 4px;
    font-size: 12px;
    max-width: 300px; /* Increased maximum width */
    min-width: 150px; /* Ensure minimum readable width */
    z-index: 1000;
    pointer-events: none;
    word-wrap: break-word; /* Break long words */
    word-break: break-word; /* Break words that are too long */
    white-space: normal; /* Allow text wrapping */
    line-height: 1.4; /* Better readability for multi-line text */
    box-shadow: 0 2px 8px rgba(0, 0, 0, 0.2);
  }
  
  .tooltip::after {
    content: '';
    position: absolute;
    top: -5px;
    left: 10px;
    border-width: 5px;
    border-style: solid;
    border-color: transparent transparent #333 transparent;
  }

  .checkbox-container {
    display: flex;
    align-items: center;
    gap: 0.5rem;
  }
  
  .checkbox-label {
    margin: 8px;
    user-select: none;
    line-height: 1;
  }
</style>