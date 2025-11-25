<!-- PromptList.svelte -->
<script lang="ts">
    import {GetStoreByName} from "@chendingplano/shared"

    const InMemStore = GetStoreByName('prompt_store', 'list')
  function handleDelete(promptId : number) {
  }
</script>

<div class="prompt-list">
  <h3>Prompts ({$InMemStore.CachedRecords.length})</h3>
  
  {#if $InMemStore.CachedRecords.length === 0}
    <div class="no-results">
      No prompts found matching your criteria.
    </div>
  {:else}
    <div class="prompts-grid">
      {#each $InMemStore.CachedRecords as prompt (prompt.prompt_id)}
        <div class="prompt-card">
          <div class="prompt-header">
            <h4>{prompt.prompt_name}</h4>
            <span class={`status status-${prompt.prompt_status.toLowerCase()}`}>
              {prompt.prompt_status}
            </span>
          </div>
          
          <div class="prompt-meta">
            <div><strong>ID:</strong> {prompt.prompt_id}</div>
            <div><strong>Purpose:</strong> {prompt.prompt_purpose}</div>
            <div><strong>Source:</strong> {prompt.prompt_source}</div>
            <div><strong>Creator:</strong> {prompt.creator}</div>
            <div><strong>Created:</strong> {new Date(prompt.created_at).toLocaleDateString()}</div>
          </div>
          
          <div class="prompt-desc">
            <strong>Description:</strong> {prompt.prompt_desc}
          </div>
          
          <div class="prompt-content">
            <strong>Content:</strong>
            <div class="content-preview">{prompt.prompt_content.slice(0, 100)}...</div>
          </div>
          
          <div class="prompt-tags">
            {#if prompt.prompt_keywords}
              <div class="keywords">
                <strong>Keywords:</strong> {prompt.prompt_keywords}
              </div>
            {/if}
            {#if prompt.prompt_tags}
              <div class="tags">
                <strong>Tags:</strong> {prompt.prompt_tags}
              </div>
            {/if}
          </div>
          
          <div class="prompt-actions">
            <button class="delete-btn" on:click={() => handleDelete(prompt.prompt_id)}>
              Delete
            </button>
          </div>
        </div>
      {/each}
    </div>
  {/if}
</div>

<style>
  .prompt-list h3 {
    margin-bottom: 15px;
    color: #333;
  }
  
  .no-results {
    text-align: center;
    padding: 40px;
    color: #666;
    font-style: italic;
  }
  
  .prompts-grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(400px, 1fr));
    gap: 20px;
  }
  
  .prompt-card {
    border: 1px solid #ddd;
    border-radius: 8px;
    padding: 15px;
    background: white;
    box-shadow: 0 2px 4px rgba(0,0,0,0.1);
  }
  
  .prompt-header {
    display: flex;
    justify-content: space-between;
    align-items: flex-start;
    margin-bottom: 10px;
  }
  
  .prompt-header h4 {
    margin: 0;
    color: #007bff;
    word-break: break-word;
  }
  
  .status {
    padding: 2px 8px;
    border-radius: 12px;
    font-size: 0.8em;
    font-weight: bold;
    text-transform: uppercase;
  }
  
  .status-active {
    background: #d4edda;
    color: #155724;
  }
  
  .status-deleted {
    background: #f8d7da;
    color: #721c24;
  }
  
  .status-suspended {
    background: #fff3cd;
    color: #856404;
  }
  
  .prompt-meta {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 5px;
    font-size: 0.85em;
    color: #666;
    margin-bottom: 10px;
  }
  
  .prompt-desc {
    margin-bottom: 10px;
    font-size: 0.9em;
  }
  
  .prompt-content {
    margin-bottom: 10px;
    font-size: 0.9em;
  }
  
  .content-preview {
    background: #f8f9fa;
    padding: 8px;
    border-radius: 4px;
    margin-top: 5px;
    font-family: monospace;
    font-size: 0.8em;
    color: #555;
  }
  
  .prompt-tags {
    font-size: 0.85em;
    color: #666;
    margin-bottom: 15px;
  }
  
  .keywords, .tags {
    margin-bottom: 5px;
  }
  
  .prompt-actions {
    display: flex;
    justify-content: flex-end;
  }
  
  .delete-btn {
    padding: 5px 15px;
    background: #dc3545;
    color: white;
    border: none;
    border-radius: 4px;
    cursor: pointer;
    font-size: 0.85em;
  }
  
  .delete-btn:hover {
    background: #c82333;
  }
</style>