<script lang="ts">
  import * as Sidebar from "$lib/components/ui/sidebar/index.js";
  import AppSidebar from "$lib/components/app-sidebar.svelte";
  import SiteHeader from "$lib/components/site-header.svelte";
  import SectionCards01 from "$lib/components/dashboard01-section01.svelte";
  import SectionCards02 from "$lib/components/dashboard01-section02.svelte";
  import ChartAreaInteractive from "$lib/components/chart-area-interactive.svelte";
  import DataTable from "$lib/components/data-table.svelte";
  import Echart01 from "$lib/components/echart-01.svelte";
  import Echart04 from "$lib/components/echart-04.svelte";
</script>

<Sidebar.Provider>
  <Sidebar.Inset>
    <div class="flex gap-4 w-full flex-wrap sm:flex-nowrap">
        <SectionCards01 />
    </div>
    <div class="flex gap-4 w-full flex-wrap sm:flex-nowrap pt-4">
        <SectionCards02 />
    </div>
    <div class="flex gap-4 w-full flex-wrap sm:flex-nowrap pt-4 h-[400px]">
        <Echart04 chartType="status"/>
    </div>
    <div class="flex gap-4 w-full flex-wrap sm:flex-nowrap pt-4 h-[400px]">
        <Echart04 chartType="data_proc_status"/>
    </div>
    <div class="flex gap-4 w-full flex-wrap sm:flex-nowrap pt-0 h-[400px]">
        <Echart01 />
    </div>
  </Sidebar.Inset>
</Sidebar.Provider>