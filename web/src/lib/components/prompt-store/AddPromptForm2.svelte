<!-- src/lib/components/prompt-store/AddPromptForm.svelte -->
<script lang="ts">
	  import { AlphaFormat } from "three";
    import type {JimoResponse} from '@chendingplano/shared';
    import {GetStoreByName, type InMemStoreDef} from "@chendingplano/shared"
    import { writable, type Writable } from 'svelte/store';
  
  let errors        = $state<{[key: string]: string}>({});
  let error_str     = $state('');
  let isSubmitting  = $state(false);
  
  const InMemStore  = GetStoreByName("prompt_store") as Writable<InMemStoreDef>
  const statusOptions = ['Active', 'Suspended'];
  const sourceOptions = ['User Entered', 'Upload', 'Web Crawled'];
  
  function validatePromptName(name: string) {
    const regex = /^[a-zA-Z0-9_]+$/;
    return regex.test(name);
  }
  
  function validateForm() {
    error_str = ''
    const crt_record = $InMemStore.CrtRecord

    if (!crt_record.prompt_name.trim()) {
      errors.PromptName = 'Prompt name is required';
      error_str += errors.PromptName + (error_str === '') ? '' : ","
    } else if (!validatePromptName(crt_record.prompt_name)) {
      errors.PromptName = 'Prompt name can only contain letters, numbers, and underscores';
      error_str += errors.PromptName + (error_str === '') ? '' : ","
    }
    
    if (!crt_record.prompt_content.trim()) {
      errors.PromptContent = 'Prompt content is required';
      error_str += errors.PromptContent + (error_str === '') ? '' : ","
    }
    
    return Object.keys(errors).length === 0;
  }
  
  async function handleSubmit(event : Event) {
    event.preventDefault()
    if (!validateForm()) {
      alert("Form check failed:" + error_str)
      return;
    }
    
    isSubmitting = true;
    
    try {
        // In a real app, you would call your API here
        // await promptService.createPrompt(newPrompt);
        console.log("Add prompt")
        const crt_record = $InMemStore.CrtRecord
        crt_record.creator_loc = "CWB_APF_078"
        crt_record.updater_loc = "CWB_APF_078"
        const response = await fetch("/shared_api/v1/add_prompt", {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify(crt_record),
            credentials: 'include'
            });

        if (response.ok) {
            const errorData = await response.json() as JimoResponse;
            alert("Prompt added to the database (CWB_PST_080+" + errorData.loc + ")")
            const stored_value = $InMemStore

            $InMemStore = {
                ...$InMemStore,
                CachedRecords: [...$InMemStore.CachedRecords, crt_record],
                CrtView: 'list',
                CrtRecord: {}
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

          const errorData = await response.json() as JimoResponse;
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
  
  function handleCancel(event: Event) {
      event.preventDefault()
      $InMemStore = {
          ...$InMemStore,
          CrtRecord: {}
      }

      // Reset form and go back
      errors = {};
  }
</script>

<div class="add-prompt-form">
  <h2>Add New Prompt</h2>
  
  <form onsubmit={handleSubmit}>
    <div class="form-grid">
      <!-- Required Fields -->
      <div class="form-group">
        <label for="promptName">Prompt Name *</label>
        <input
          id="promptName"
          type="text"
          bind:value={$InMemStore.CrtRecord}
          class:error={errors.PromptName}
          placeholder="e.g., creative_writing_assistant"
          maxlength="100"
        />
        {#if errors.PromptName}
          <span class="error-message">{errors.PromptName}</span>
        {/if}
        <div class="help-text">Only letters, numbers, and underscores allowed</div>
      </div>
      
      <div class="form-group">
        <label for="promptDesc">Description</label>
        <textarea
          id="promptDesc"
          bind:value={$InMemStore.CrtRecord.prompt_desc}
          class:error={errors.PromptDesc}
          placeholder="Brief description of the prompt's purpose"
          rows="3">
        </textarea>
        {#if errors.PromptDesc}
          <span class="error-message">{errors.PromptDesc}</span>
        {/if}
      </div>
      
      <div class="form-group full-width">
        <label for="promptContent">Prompt Content *</label>
        <textarea
          id="promptContent"
          bind:value={$InMemStore.CrtRecord.prompt_content}
          class:error={errors.PromptContent}
          placeholder="Enter the full prompt content..."
          rows="6">
        </textarea>
        {#if errors.PromptContent}
          <span class="error-message">{errors.PromptContent}</span>
        {/if}
      </div>
      
      <div class="form-group">
        <label for="status">Status</label>
        <select id="status" bind:value={$InMemStore.CrtRecord.prompt_status}>
          {#each statusOptions as option}
            <option value={option}>{option}</option>
          {/each}
        </select>
      </div>
      
      <div class="form-group">
        <label for="purpose">Purpose</label>
        <input
          id="purpose"
          type="text"
          bind:value={$InMemStore.CrtRecord.prompt_purpose}
          class:error={errors.PromptPurpose}
          placeholder="e.g., Writing, Coding, Analysis"
        />
        {#if errors.PromptPurpose}
          <span class="error-message">{errors.PromptPurpose}</span>
        {/if}
      </div>
      
      <div class="form-group">
        <label for="source">Source</label>
        <select id="source" bind:value={$InMemStore.CrtRecord.prompt_source}>
          {#each sourceOptions as option}
            <option value={option}>{option}</option>
          {/each}
        </select>
      </div>
      
      <!-- Optional Fields -->
      <div class="form-group">
        <label for="keywords">Keywords</label>
        <input
          id="keywords"
          type="text"
          bind:value={$InMemStore.CrtRecord.prompt_keywords}
          placeholder="comma,separated,keywords"
        />
        <div class="help-text">Separate with commas for better search</div>
      </div>
      
      <div class="form-group">
        <label for="tags">Tags</label>
        <input
          id="tags"
          type="text"
          bind:value={$InMemStore.CrtRecord.prompt_tags}
          placeholder="comma,separated,tags"
        />
        <div class="help-text">Separate with commas for categorization</div>
      </div>
      
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
  
  .full-width {
    grid-column: 1 / -1;
  }
  
  .form-group {
    display: flex;
    flex-direction: column;
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
</style>