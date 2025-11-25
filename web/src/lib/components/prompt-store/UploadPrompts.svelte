<!-- src/lib/components/prompt-store/UploadPrompts.svelte -->
<script lang="ts">
  import { createEventDispatcher } from 'svelte';
  
  const dispatch = createEventDispatcher();
  
  const null_data = {
        processed: 0,
        created: 0,
        errors: 0,
        duplicates: 0,
        failedItems: []
  }

  type UploadErrorResult = {
    success: boolean;
    message: string;
    data: {
        processed: number,
        created: number,
        errors: number,
        duplicates: number,
        failedItems: string[]
    };
  };

  let selectedFile : File | null = null;
  let isUploading = false;
  let uploadProgress = 0;
  let uploadResultFile : File | null = null;
  let uploadResult : UploadErrorResult | null = null;
  let dragOver = false;
  
  const supportedFormats = ['.json', '.csv', '.txt'];
  const maxFileSize = 10 * 1024 * 1024; // 10MB

  function handleFileSelect(event : Event) {
    const file = (event.target as HTMLInputElement).files?.[0] || null;
    if (file != null) {
        processFile(file);
    }
  }
  
  function handleDragOver(event : DragEvent) {
    event.preventDefault();
    dragOver = true;
  }
  
  function handleDragLeave(event : DragEvent) {
    event.preventDefault();
    dragOver = false;
  }
  
  function handleDrop(event : DragEvent) {
    event.preventDefault();
    dragOver = false;
    
    const file = event.dataTransfer && event.dataTransfer.files.length > 0 ? event.dataTransfer.files[0] : null;
    if (file != null) {
        processFile(file);
    }
  }
  
  function processFile(file: File) {
    if (!file) {
      selectedFile = null;
      uploadResult = null;
      return;
    }
    
    // Validate file size
    if (file.size > maxFileSize) {
      uploadResult = {
        success: false,
        message: `File size exceeds ${maxFileSize / 1024 / 1024}MB limit`,
        data: null_data
      };
      selectedFile = null;
      return;
    }
    
    // Validate file extension
    const fileExtension = '.' + (file.name.split('.').pop()?.toLowerCase() ?? '');
    if (!supportedFormats.includes(fileExtension)) {
      uploadResult = {
        success: false,
        message: `Unsupported file format. Supported formats: ${supportedFormats.join(', ')}`,
        data: null_data
      };
      selectedFile = null;
      return;
    }
    
    selectedFile = file;
    uploadResult = null;
    uploadProgress = 0;
  }
  
  async function handleUpload() {
    if (!selectedFile) return;
    
    isUploading = true;
    uploadProgress = 0;
    uploadResult = null;
    
    try {
      // Simulate upload progress
      const progressInterval = setInterval(() => {
        uploadProgress = Math.min(uploadProgress + 10, 90);
      }, 200);
      
      // In a real app, you would use FormData and fetch
      // const formData = new FormData();
      // formData.append('file', selectedFile);
      // 
      // const response = await fetch('/api/prompts/upload', {
      //   method: 'POST',
      //   body: formData
      // });
      // 
      // const result = await response.json();
      
      // Simulate API call delay
      await new Promise(resolve => setTimeout(resolve, 2000));
      clearInterval(progressInterval);
      uploadProgress = 100;
      
      // Mock successful response
      const mockResult : UploadErrorResult = {
        success: true,
        message: 'File uploaded successfully!',
        data: {
          processed: 5,
          created: 5,
          errors: 0,
          duplicates: 0,
          failedItems: []
        }
      };
      
      uploadResult = mockResult;
      
      // Emit success event to parent
      if (mockResult.success) {
        dispatch('uploadSuccess', mockResult.data);
      }
      
    } catch (error) {
      console.error('Upload error:', error);
      uploadResult = {
        success: false,
        message: 'Upload failed. Please try again.',
        data: null_data
      };
    } finally {
      isUploading = false;
    }
  }
  
  function handleCancel() {
    selectedFile = null;
    uploadResult = null;
    uploadProgress = 0;
    dispatch('cancel');
  }
  
  function clearFile() {
    selectedFile = null;
    uploadResult = null;
    uploadProgress = 0;
  }
  
  function downloadTemplate() {
    // Create and download a template file
    const template = {
      format: "JSON array of prompt objects",
      fields: [
        "PromptName (required): string, letters/numbers/underscores only",
        "PromptDesc (required): string",
        "PromptContent (required): string", 
        "Status: 'Active' or 'Suspended'",
        "PromptPurpose: string",
        "PromptSource: 'User Entered', 'Upload', or 'Web Crawled'",
        "PromptKeywords: comma-separated string",
        "PromptTags: comma-separated string",
        "Creator: string"
      ],
      example: [
        {
          "PromptName": "creative_writing",
          "PromptDesc": "Creative writing assistant",
          "PromptContent": "You are a creative writing assistant...",
          "Status": "Active",
          "PromptPurpose": "Writing",
          "PromptSource": "User Entered",
          "PromptKeywords": "writing,creative,assistant",
          "PromptTags": "writing,creative",
          "Creator": "user1"
        },
        {
          "PromptName": "code_reviewer",
          "PromptDesc": "Code review assistant",
          "PromptContent": "You are an expert code reviewer...",
          "Status": "Active",
          "PromptPurpose": "Programming",
          "PromptSource": "User Entered",
          "PromptKeywords": "code,review,programming",
          "PromptTags": "programming,code",
          "Creator": "user2"
        }
      ]
    };
    
    const blob = new Blob([JSON.stringify(template, null, 2)], { type: 'application/json' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = 'prompt-template.json';
    document.body.appendChild(a);
    a.click();
    document.body.removeChild(a);
    URL.revokeObjectURL(url);
  }
  
  function formatFileSize(bytes: number) {
    if (bytes === 0) return '0 Bytes';
    const k = 1024;
    const sizes = ['Bytes', 'KB', 'MB', 'GB'];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
  }
</script>

<div class="upload-prompts">
  <h2>Upload Prompts</h2>
  
  <div class="upload-info">
    <h3>Supported Formats</h3>
    <div class="format-list">
      <div class="format-item">
        <strong>JSON</strong>
        <span>Array of prompt objects</span>
      </div>
      <div class="format-item">
        <strong>CSV</strong>
        <span>Comma-separated values with headers</span>
      </div>
      <div class="format-item">
        <strong>TXT</strong>
        <span>One prompt per line (basic format)</span>
      </div>
    </div>
    
    <div class="file-requirements">
      <h4>File Requirements:</h4>
      <ul>
        <li>Maximum file size: 10MB</li>
        <li>Required fields: PromptName, PromptDesc, PromptContent, Creator</li>
        <li>PromptName must contain only letters, numbers, and underscores</li>
        <li>Duplicate prompt names will be skipped</li>
      </ul>
    </div>
    
    <button on:click={downloadTemplate} class="template-btn">
      <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor">
        <path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/>
        <polyline points="17 8 12 3 7 8"/>
        <line x1="12" y1="3" x2="12" y2="15"/>
      </svg>
      Download Template
    </button>
  </div>
  
  <div class="upload-area">
    <div 
      role="region"
      aria-label="File upload area"
      class="file-drop-zone"
      class:active={selectedFile}
      class:drag-over={dragOver}
      on:dragover={handleDragOver}
      on:dragleave={handleDragLeave}
      on:drop={handleDrop}
    >
      <input
        type="file"
        id="fileInput"
        accept=".json,.csv,.txt"
        on:change={handleFileSelect}
        style="display: none;"
      />
      
      {#if !selectedFile}
        <div class="drop-message" role="group" aria-label="File selection options">
          <svg width="48" height="48" viewBox="0 0 24 24" fill="none" stroke="currentColor">
            <path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/>
            <polyline points="17 8 12 3 7 8"/>
            <line x1="12" y1="3" x2="12" y2="15"/>
          </svg>
          <p>Choose a file or drag it here</p>
          <button 
            on:click={() => {
              const fileInput = document.getElementById('fileInput');
              if (fileInput) {
                fileInput.click()
              }
            }}
            class="browse-btn"
          >
            Browse Files
          </button>
          <div class="file-hint">Supports: {supportedFormats.join(', ')}</div>
        </div>
      {:else}
        <div class="file-selected" role="group" aria-label="Selected file information">
          <div class="file-icon">
            <svg width="32" height="32" viewBox="0 0 24 24" fill="none" stroke="currentColor">
              <path d="M13 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V9z"/>
              <polyline points="13 2 13 9 20 9"/>
            </svg>
          </div>
          <div class="file-details">
            <div class="file-name">{selectedFile.name}</div>
            <div class="file-size">{formatFileSize(selectedFile.size)}</div>
          </div>
          <button on:click={clearFile} class="clear-btn" title="Remove file" aria-label="Remove selected file">
            <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor">
              <line x1="18" y1="6" x2="6" y2="18"/>
              <line x1="6" y1="6" x2="18" y2="18"/>
            </svg>
          </button>
        </div>
      {/if}
    </div>
    
    {#if isUploading}
      <div class="upload-progress">
        <div class="progress-bar">
          <div 
            class="progress-fill" 
            style={`width: ${uploadProgress}%`}
          ></div>
        </div>
        <div class="progress-text">Uploading... {uploadProgress}%</div>
      </div>
    {/if}
    
    {#if uploadResult && uploadResult as UploadErrorResult}
      <div class="upload-result" class:success={uploadResult.success} class:error={!uploadResult.success}>
        <div class="result-icon">
          {#if uploadResult.success}
            <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor">
              <polyline points="20 6 9 17 4 12"/>
            </svg>
          {:else}
            <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor">
              <circle cx="12" cy="12" r="10"/>
              <line x1="15" y1="9" x2="9" y2="15"/>
              <line x1="9" y1="9" x2="15" y2="15"/>
            </svg>
          {/if}
        </div>
        <div class="result-message">
          <strong>{uploadResult.success ? 'Success!' : 'Error'}</strong>
          <p>{uploadResult.message}</p>
          {#if uploadResult.success && uploadResult.data}
            <div class="result-details">
              <div>Processed: {uploadResult.data.processed}</div>
              <div>Created: {uploadResult.data.created}</div>
              <div>Errors: {uploadResult.data.errors}</div>
              <div>Duplicates: {uploadResult.data.duplicates}</div>
            </div>
          {/if}
        </div>
      </div>
    {/if}
    
    <div class="upload-actions">
      <button
        type="button"
        on:click={handleCancel}
        class="cancel-btn"
        disabled={isUploading}
      >
        Cancel
      </button>
      <button
        type="button"
        on:click={handleUpload}
        class="upload-btn"
        disabled={!selectedFile || isUploading}
      >
        {isUploading ? 'Uploading...' : 'Upload File'}
      </button>
    </div>
  </div>
</div>

<style>
  .upload-prompts {
    background: white;
    padding: 30px;
    border-radius: 8px;
    box-shadow: 0 2px 10px rgba(0, 0, 0, 0.1);
    max-width: 800px;
    margin: 0 auto;
  }

  .upload-prompts h2 {
    margin-bottom: 25px;
    color: #333;
    border-bottom: 2px solid #007bff;
    padding-bottom: 10px;
    font-size: 1.5em;
    font-weight: 600;
  }

  .upload-info {
    margin-bottom: 30px;
    padding: 25px;
    background: #f8f9fa;
    border-radius: 8px;
    border-left: 4px solid #007bff;
  }

  .upload-info h3 {
    margin-bottom: 15px;
    color: #495057;
    font-size: 1.2em;
    font-weight: 600;
  }

  .upload-info h4 {
    margin: 20px 0 10px 0;
    color: #495057;
    font-size: 1em;
    font-weight: 600;
  }

  .format-list {
    display: flex;
    flex-direction: column;
    gap: 12px;
    margin-bottom: 20px;
  }

  .format-item {
    display: flex;
    justify-content: space-between;
    align-items: center;
    padding: 12px 16px;
    background: white;
    border-radius: 6px;
    border-left: 4px solid #007bff;
    box-shadow: 0 1px 3px rgba(0, 0, 0, 0.1);
    transition: transform 0.2s ease, box-shadow 0.2s ease;
  }

  .format-item:hover {
    transform: translateY(-1px);
    box-shadow: 0 2px 6px rgba(0, 0, 0, 0.15);
  }

  .format-item strong {
    color: #007bff;
    font-weight: 600;
  }

  .format-item span {
    color: #6c757d;
    font-size: 0.9em;
    text-align: right;
  }

  .file-requirements ul {
    color: #6c757d;
    font-size: 0.9em;
    line-height: 1.6;
    padding-left: 20px;
  }

  .file-requirements li {
    margin-bottom: 6px;
    position: relative;
  }

  .file-requirements li::before {
    content: "•";
    color: #007bff;
    font-weight: bold;
    position: absolute;
    left: -15px;
  }

  .template-btn {
    display: inline-flex;
    align-items: center;
    gap: 8px;
    padding: 12px 20px;
    background: #28a745;
    color: white;
    border: none;
    border-radius: 6px;
    cursor: pointer;
    font-size: 0.9em;
    font-weight: 500;
    transition: all 0.3s ease;
    box-shadow: 0 2px 4px rgba(40, 167, 69, 0.2);
  }

  .template-btn:hover {
    background: #218838;
    transform: translateY(-1px);
    box-shadow: 0 4px 8px rgba(40, 167, 69, 0.3);
  }

  .template-btn:active {
    transform: translateY(0);
  }

  .upload-area {
    border: 2px dashed #dee2e6;
    border-radius: 12px;
    padding: 40px 30px;
    text-align: center;
    transition: all 0.3s ease;
    background: #fafbfc;
  }

  .file-drop-zone {
    transition: all 0.3s ease;
  }

  .file-drop-zone.drag-over {
    border-color: #007bff;
    background: #f0f8ff;
    box-shadow: 0 0 0 4px rgba(0, 123, 255, 0.1);
  }

  .file-drop-zone.active {
    border-style: solid;
    border-color: #28a745;
    background: #f8fff9;
  }

  .drop-message {
    color: #6c757d;
  }

  .drop-message svg {
    color: #adb5bd;
    margin-bottom: 20px;
    opacity: 0.7;
  }

  .drop-message p {
    margin-bottom: 20px;
    font-size: 1.1em;
    font-weight: 500;
  }

  .browse-btn {
    padding: 12px 24px;
    background: #007bff;
    color: white;
    border: none;
    border-radius: 6px;
    cursor: pointer;
    font-size: 0.95em;
    font-weight: 500;
    transition: all 0.3s ease;
    box-shadow: 0 2px 4px rgba(0, 123, 255, 0.2);
  }

  .browse-btn:hover {
    background: #0056b3;
    transform: translateY(-1px);
    box-shadow: 0 4px 8px rgba(0, 123, 255, 0.3);
  }

  .file-hint {
    margin-top: 15px;
    font-size: 0.85em;
    color: #adb5bd;
    font-style: italic;
  }

  .file-selected {
    display: flex;
    align-items: center;
    gap: 16px;
    padding: 24px;
    background: white;
    border-radius: 8px;
    box-shadow: 0 2px 8px rgba(0, 0, 0, 0.1);
    border: 1px solid #e9ecef;
  }

  .file-icon {
    color: #495057;
    flex-shrink: 0;
  }

  .file-details {
    flex: 1;
    text-align: left;
  }

  .file-name {
    font-weight: 600;
    color: #495057;
    margin-bottom: 6px;
    font-size: 1.05em;
    word-break: break-all;
  }

  .file-size {
    color: #6c757d;
    font-size: 0.9em;
  }

  .clear-btn {
    background: none;
    border: none;
    color: #6c757d;
    cursor: pointer;
    padding: 8px;
    border-radius: 4px;
    transition: all 0.3s ease;
    flex-shrink: 0;
  }

  .clear-btn:hover {
    background: #e9ecef;
    color: #dc3545;
    transform: scale(1.1);
  }

  .upload-progress {
    margin-top: 25px;
    padding: 20px;
    background: #f8f9fa;
    border-radius: 8px;
  }

  .progress-bar {
    width: 100%;
    height: 10px;
    background: #e9ecef;
    border-radius: 5px;
    overflow: hidden;
    margin-bottom: 12px;
    box-shadow: inset 0 1px 2px rgba(0, 0, 0, 0.1);
  }

  .progress-fill {
    height: 100%;
    background: linear-gradient(90deg, #007bff, #0056b3);
    border-radius: 5px;
    transition: width 0.3s ease;
    position: relative;
    overflow: hidden;
  }

  .progress-fill::after {
    content: '';
    position: absolute;
    top: 0;
    left: -100%;
    width: 100%;
    height: 100%;
    background: linear-gradient(90deg, transparent, rgba(255, 255, 255, 0.4), transparent);
    animation: shimmer 1.5s infinite;
  }

  @keyframes shimmer {
    0% { left: -100%; }
    100% { left: 100%; }
  }

  .progress-text {
    color: #495057;
    font-size: 0.9em;
    font-weight: 500;
  }

  .upload-result {
    display: flex;
    align-items: flex-start;
    gap: 15px;
    padding: 20px;
    margin-top: 20px;
    border-radius: 8px;
    border-left: 4px solid;
  }

  .upload-result.success {
    background: #f8fff9;
    border-left-color: #28a745;
    color: #155724;
  }

  .upload-result.error {
    background: #fff5f5;
    border-left-color: #dc3545;
    color: #721c24;
  }

  .result-icon {
    flex-shrink: 0;
    margin-top: 2px;
  }

  .result-message {
    flex: 1;
    text-align: left;
  }

  .result-message strong {
    display: block;
    margin-bottom: 8px;
    font-size: 1.1em;
  }

  .result-message p {
    margin-bottom: 12px;
    line-height: 1.5;
  }

  .result-details {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(120px, 1fr));
    gap: 10px;
    padding: 12px;
    background: rgba(255, 255, 255, 0.7);
    border-radius: 6px;
    font-size: 0.85em;
  }

  .result-details div {
    text-align: center;
    padding: 6px;
    background: white;
    border-radius: 4px;
    box-shadow: 0 1px 2px rgba(0, 0, 0, 0.1);
  }

  .upload-actions {
    display: flex;
    justify-content: flex-end;
    gap: 15px;
    margin-top: 25px;
    padding-top: 20px;
    border-top: 1px solid #e9ecef;
  }

  .cancel-btn,
  .upload-btn {
    padding: 12px 30px;
    border: none;
    border-radius: 6px;
    cursor: pointer;
    font-size: 0.95em;
    font-weight: 500;
    transition: all 0.3s ease;
    min-width: 120px;
  }

  .cancel-btn {
    background: #6c757d;
    color: white;
    box-shadow: 0 2px 4px rgba(108, 117, 125, 0.2);
  }

  .cancel-btn:hover:not(:disabled) {
    background: #545b62;
    transform: translateY(-1px);
    box-shadow: 0 4px 8px rgba(108, 117, 125, 0.3);
  }

  .upload-btn {
    background: #007bff;
    color: white;
    box-shadow: 0 2px 4px rgba(0, 123, 255, 0.2);
  }

  .upload-btn:hover:not(:disabled) {
    background: #0056b3;
    transform: translateY(-1px);
    box-shadow: 0 4px 8px rgba(0, 123, 255, 0.3);
  }

  .cancel-btn:disabled,
  .upload-btn:disabled {
    opacity: 0.6;
    cursor: not-allowed;
    transform: none !important;
    box-shadow: 0 2px 4px rgba(0, 0, 0, 0.1) !important;
  }

  /* Responsive design */
  @media (max-width: 768px) {
    .upload-prompts {
      padding: 20px;
      margin: 10px;
    }

    .upload-info {
      padding: 20px;
    }

    .format-item {
      flex-direction: column;
      align-items: flex-start;
      gap: 8px;
    }

    .format-item span {
      text-align: left;
    }

    .upload-area {
      padding: 30px 20px;
    }

    .file-selected {
      padding: 20px;
      flex-direction: column;
      text-align: center;
      gap: 12px;
    }

    .file-details {
      text-align: center;
    }

    .upload-actions {
      flex-direction: column-reverse;
    }

    .cancel-btn,
    .upload-btn {
      width: 100%;
    }

    .result-details {
      grid-template-columns: 1fr 1fr;
    }
  }

  @media (max-width: 480px) {
    .upload-prompts {
      padding: 15px;
    }

    .upload-info {
      padding: 15px;
    }

    .upload-area {
      padding: 20px 15px;
    }

    .result-details {
      grid-template-columns: 1fr;
    }
  }
</style>