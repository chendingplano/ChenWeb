<script lang="ts">
  import MenuContent from "$lib/components/MenuContent.svelte";
  import TableWithFilters from "$lib/components/table-with-filters.svelte";
  import Table03 from "$lib/components/table-03.svelte";
  import LoginPage from "$lib/components/login-01.svelte";
  import * as Resizable from "$lib/components/ui/resizable/index.js";

  let activeView : string | null = null

  function handleMenuSelect(value: CustomEvent<string>) {
    console.log("Menu selected:", value.detail);
    activeView = value.detail;
  }
</script>

<!-- Responsive wrapper -->
<div class="w-full min-w-0 flex justify-center">
  <div class="w-full min-w-[300px] max-w-[1500px]">
    <Resizable.PaneGroup
      direction="horizontal"
      class="min-h-[900px] w-full rounded-lg border"
    >
      <Resizable.Pane defaultSize={20} class="pt-10">
        <MenuContent on:select={handleMenuSelect} />
      </Resizable.Pane>
      <Resizable.Handle withHandle />
      <Resizable.Pane defaultSize={60}>
        {#if activeView === 'documents'}
            <div class="pt-5 px-4 h-[800px] overflow-hidden">
                <TableWithFilters class="h-full w-full"/>
            </div>
        {:else if activeView === 'table-03'}
            <div class="pt-5 px-4 h-[800px] overflow-hidden">
                <Table03 class="h-full w-full"/>
            </div>
        {:else if activeView === 'login'}
            <div class="pt-5 px-4 w-[200px] h-[500px] items-center justify-center">
                <LoginPage />
            </div>
        {:else}
            <div class="flex h-full items-center justify-center">
                <span class="font-semibold text-muted-foreground">Select an item from the menu</span>
            </div>
        {/if}
        <div class="flex h-full items-center justify-center p-6">
          <span class="font-semibold">Content</span>
        </div>
      </Resizable.Pane>
      <Resizable.Handle withHandle />
      <Resizable.Pane defaultSize={20}>
        <div class="flex h-full items-center justify-center p-6">
          <span class="font-semibold">Properties</span>
        </div>
      </Resizable.Pane>
    </Resizable.PaneGroup>
  </div>
</div>