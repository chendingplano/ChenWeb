<!-- src/routes/+page.svelte -->
<script lang="ts">
  import { onMount } from 'svelte';

  let ocrResult : Record<string, string> | null = null;
  let isLoading = false;
  let error : string | null = null;

  // We'll import scribe dynamically inside onMount
  // to ensure it only runs in the browser
  onMount(async () => {
    try {
      isLoading = true;
      error = null;

      // Dynamically import scribe.js-ocr (only in browser)
      const scribe = await import('scribe.js-ocr');

      const result = await scribe.default.extractText([
        'https://tesseract.projectnaptha.com/img/eng_bw.png'
      ]);

      ocrResult = result;
      console.log('OCR Result:', result);
    } catch (err) {
      if (err instanceof Error) {
        error = err.message
      } else {
        error = 'OCR failed: unknown error'
      }
      console.error('OCR Error:', err);
    } finally {
      isLoading = false;
    }
  });
</script>

<h1>Scribe.js OCR in SvelteKit</h1>

{#if isLoading}
  <p>Running OCR... (this may take a few seconds)</p>
{:else if error}
  <p style="color: red;">Error: {error}</p>
{:else if ocrResult}
  <h2>OCR Result:</h2>
  <pre>{JSON.stringify(ocrResult, null, 2)}</pre>
{/if}
