<!-- QueryArea.svelte -->
<script>
  let query = {
    PromptID: '',
    PromptName: '',
    PromptDesc: '',
    PromptContent: '',
    Status: '',
    PromptKeywords: '',
    PromptTags: '',
    Creator: '',
    Updater: '',
    CreatedAt: '',
    UpdatedAt: ''
  };
  
  const statusOptions = ['', 'Active', 'Deleted', 'Suspended'];
  
  function handleInlineFilter() {
    console.log("Inline Filtering")
  }
  
  function handleDBSearch() {
    console.log("DB Search")
  }
  
  function handleReset() {
    query = {
      PromptID: '',
      PromptName: '',
      PromptDesc: '',
      PromptContent: '',
      Status: '',
      PromptKeywords: '',
      PromptTags: '',
      Creator: '',
      Updater: '',
      CreatedAt: '',
      UpdatedAt: ''
    };
  }
</script>

<div class="query-area">
  <h3>Search Prompts</h3>
  <div class="query-grid">
    <div class="query-field">
      <label for="prompt_id">Prompt ID:</label>
      <input 
        id="prompt_id"
        type="text" 
        bind:value={query.PromptID}
        placeholder="Enter Prompt ID"
      />
    </div>
    
    <div class="query-field">
      <label for="prompt_name">Prompt Name:</label>
      <input 
        id="prompt_name"
        type="text" 
        bind:value={query.PromptName}
        placeholder="Enter Prompt Name"
      />
    </div>
    
    <div class="query-field">
      <label for="prompt_desc">Description:</label>
      <input 
        id="prompt_desc"
        type="text" 
        bind:value={query.PromptDesc}
        placeholder="Search in description"
      />
    </div>
    
    <div class="query-field">
      <label for="prompt_content">Content:</label>
      <input 
        id="prompt_content"
        type="text" 
        bind:value={query.PromptContent}
        placeholder="Search in content"
      />
    </div>
    
    <div class="query-field">
      <label for="prompt_status">Status:</label>
      <select id="prompt_status" bind:value={query.Status}>
        {#each statusOptions as option}
          <option value={option}>{option || 'All Status'}</option>
        {/each}
      </select>
    </div>
    
    <div class="query-field">
      <label for="prompt_keywords">Keywords:</label>
      <input 
        id="prompt_keywords"
        type="text" 
        bind:value={query.PromptKeywords}
        placeholder="Search keywords"
      />
    </div>
    
    <div class="query-field">
      <label for="prompt_tags">Tags:</label>
      <input 
        id="prompt_tags"
        type="text" 
        bind:value={query.PromptTags}
        placeholder="Search tags"
      />
    </div>
    
    <div class="query-field">
      <label for="prompt_creator">Creator:</label>
      <input 
        id="prompt_creator"
        type="text" 
        bind:value={query.Creator}
        placeholder="Search by creator"
      />
    </div>
    
    <div class="query-field">
      <label for="prompt_updater">Updater:</label>
      <input 
        id="prompt_updater"
        type="text" 
        bind:value={query.Updater}
        placeholder="Search by updater"
      />
    </div>
    
    <div class="query-field">
      <label for="creator">Created After:</label>
      <input 
        id="creator"
        type="date" 
        bind:value={query.CreatedAt}
      />
    </div>
    
    <div class="query-field">
      <label for="updater">Updated After:</label>
      <input 
        id="updater"
        type="date" 
        bind:value={query.UpdatedAt}
      />
    </div>
  </div>
  
  <div class="query-actions">
    <button on:click={handleInlineFilter} class="search-btn">Inline Filter</button>
    <button on:click={handleDBSearch} class="search-btn">Search in DB</button>
    <button on:click={handleReset} class="reset-btn">Reset</button>
  </div>
</div>

<style>
  .query-area {
    background: #f8f9fa;
    padding: 20px;
    border-radius: 8px;
    margin-bottom: 20px;
  }
  
  .query-area h3 {
    margin: 0 0 15px 0;
    color: #333;
  }
  
  .query-grid {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(250px, 1fr));
    gap: 15px;
    margin-bottom: 15px;
  }
  
  .query-field {
    display: flex;
    flex-direction: column;
  }
  
  .query-field label {
    font-weight: bold;
    margin-bottom: 5px;
    color: #555;
    font-size: 0.9em;
  }
  
  .query-field input,
  .query-field select {
    padding: 8px;
    border: 1px solid #ddd;
    border-radius: 4px;
    font-size: 0.9em;
  }
  
  .query-actions {
    display: flex;
    justify-content: flex-end;
    gap: 10px;
  }
  
  .search-btn, .reset-btn {
    padding: 10px 20px;
    border: none;
    border-radius: 4px;
    cursor: pointer;
    font-size: 0.9em;
  }
  
  .search-btn {
    background: #007bff;
    color: white;
  }
  
  .search-btn:hover {
    background: #0056b3;
  }
  
  .reset-btn {
    background: #6c757d;
    color: white;
  }
  
  .reset-btn:hover {
    background: #545b62;
  }
</style>