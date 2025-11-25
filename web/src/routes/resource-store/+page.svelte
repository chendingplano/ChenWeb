<!-- App.svelte -->
<script>
  import ResourceStore from '$lib/components/resource-store/ResourceStore.svelte';
</script>

<main>
  <h1>Resource Management</h1>
  <ResourceStore/>
</main>

<style>
  main {
    max-width: 1200px;
    margin: 0 auto;
    padding: 20px;
    font-family: Arial, sans-serif;
  }
  
  h1 {
    color: #333;
    margin-bottom: 30px;
    text-align: center;
  }
</style>