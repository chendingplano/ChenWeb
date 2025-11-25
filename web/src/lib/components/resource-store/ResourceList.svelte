<!-- PromptList.svelte -->
<script lang="ts">
  import {GetStoreByName} from "@chendingplano/shared"
  
  const InMemStore = GetStoreByName("resource_store", "list")
  function handleDelete(resourceId : number) {
  }
</script>

<div class="prompt-list">
  <h3>Resources ({$InMemStore.CachedRecords.length})</h3>
  
  {#if $InMemStore.CachedRecords.length === 0}
    <div class="no-results">
      No resources found matching your criteria.
    </div>
  {:else}
    <div class="prompts-grid">
      {#each $InMemStore.CachedRecords as resource (resource.resource_id)}
        <div class="prompt-card">
          <div class="prompt-header">
            <h4>{resource.resource_name}</h4>
            <span class={`status status-${resource.resource_status.toLowerCase()}`}>
              {resource.resource_status}
            </span>
          </div>
          
          <div class="prompt-meta">
            <div><strong>ID:</strong> {resource.resource_id}</div>
            <div><strong>Resource Name:</strong> {resource.resource_name}</div>
            <div><strong>Resource Opr:</strong> {resource.resource_opr}</div>
            <div><strong>Creator:</strong> {resource.creator}</div>
            <div><strong>Created:</strong> {new Date(resource.created_at).toLocaleDateString()}</div>
          </div>
          
          <div class="prompt-desc">
            <strong>Description:</strong> {resource.resource_desc}
          </div>
          
          <!--
          <div class="prompt-content">
            <strong>Content:</strong>
            <div class="content-preview">{resource.resource_content.slice(0, 100)}...</div>
          </div>
          
          <div class="prompt-tags">
            {#if resource.resource_keywords}
              <div class="keywords">
                <strong>Keywords:</strong> {resource.resource_keywords}
              </div>
            {/if}
            {#if resource.resource_tags}
              <div class="tags">
                <strong>Tags:</strong> {resource.resource_tags}
              </div>
            {/if}
          </div>
          -->
          
          <div class="prompt-actions">
            <button class="delete-btn" on:click={() => handleDelete(resource.resource_id)}>
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