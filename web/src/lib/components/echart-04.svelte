<script lang="ts">
  import { Chart } from 'svelte-echarts';
  import type { EChartsOption } from 'echarts';
  import type { BarSeriesOption } from 'echarts/charts';
  import type { 
    ProcessStatusResponse,
    ChartData 
  } from '$lib/types/types';
  import * as echarts from 'echarts';
  import { init, use } from 'echarts/core';
  import { BarChart } from 'echarts/charts';
  import { CanvasRenderer } from 'echarts/renderers';
  import { onMount } from 'svelte';
  import {
    ToolboxComponent,
    TooltipComponent,
    GridComponent,
    LegendComponent,
    DataZoomComponent 
  } from 'echarts/components';

  // Register ECharts modules
  use([
    BarChart,
    CanvasRenderer,
    ToolboxComponent,
    TooltipComponent,
    GridComponent,
    LegendComponent,
    DataZoomComponent 
  ]);

  // Reactive states
  let loading = $state(true);
  let error = $state('');
  let categories = $state<string[]>([]);
  let seriesData = $state<{ name: string; data: number[] }[]>([]);
  let {chartType} = $props();
  let noData = $state(true)

  // Label style config
  const labelOption: NonNullable<BarSeriesOption>['label'] = {
    show: false,
    position: 'insideBottom',
    distance: 15,
    align: 'left',
    verticalAlign: 'middle',
    rotate: 90,
    formatter: '{c}  {name|{a}}',
    fontSize: 16,
    rich: { name: {} }
  };

  // Fetch data from backend (e.g. from /api/echart-data/demo-04)
  onMount(async () => {
  try {
    const res = await fetch('/api/v1/echart-data/demo-04');
    if (!res.ok) {
      console.log('Failed to fetch chart data (CWB_ED3_108)');
      loading = false
      noData = true
      return
    }

    // Parse JSON returned from Go API
    const json_value = await res.json() as ProcessStatusResponse;
    if (!json_value.status)
    {
      console.log('Failed retrieving data (CWB_ED3_067), error:' + 
        json_value.error_msg + ", loc:" + json_value.loc)
      loading = false
      noData = true
      return
    }

    if (json_value.num_records <= 0) {
        console.log('No data in the database (CWB_ED3_078)')
        loading = false
        noData = true
        return
    }

    // Select one specific chart type
    const chartData = json_value.map_data[chartType] as ChartData;

    if (!chartData) {
      console.log('***** Internal Error (CWB_ED3_077)')
      loading = false;
      noData = true
      return
    }

    // Extract categories and series
    categories = chartData.categories;
    seriesData = chartData.series;

    // Optionally: log for debugging
    console.log('Loaded chart data:', { categories, seriesData });

    } catch (e: any) {
        error = e.message ?? String(e);
        console.error('Error loading chart data (CWB_ED3_097):', e);
    } finally {
        loading = false;
    }
  });

  // Build options reactively from data
  const options = $derived<EChartsOption>({
  title: {
    text: `Bar Chart for ${chartType}`,
    left: 'left'
  },
  tooltip: {
    trigger: 'axis',
    axisPointer: { type: 'shadow' }
  },
  legend: {
    data: seriesData.map((s) => s.name),
    top: 30
  },
  toolbox: {
    show: true,
    orient: 'vertical',
    left: 'right',
    top: 'center',
    feature: {
      mark: { show: true },
      dataView: { show: true, readOnly: false },
      magicType: { show: true, type: ['line', 'bar', 'stack'] },
      restore: { show: true },
      saveAsImage: { show: true }
    }
  },
  xAxis: [
    {
      type: 'category',
      axisTick: { show: false },
      data: categories
    }
  ],
  yAxis: [{ type: 'value' }],
  dataZoom: [
    {
      type: 'slider',
      show: true,
      xAxisIndex: 0,
      start: 0,
      end: 50
    },
    {
      type: 'inside',
      xAxisIndex: 0
    }
  ],
  series: seriesData.map((s) => ({
    name: s.name,
    type: 'bar',
    barGap: 0,
    label: labelOption,
    emphasis: { focus: 'series' },
    data: s.data
  }))
});
</script>

<div class="w-full h-full pt-8" style="height: 100%">
  <div class="chart-wrapper" style="height: calc(100% - 2rem)"> <!-- account for pt-8 = 2rem -->
    {#if loading}
      <p>Loading chart data...</p>
    {:else if error}
      <p style="color:red">{error}</p>
    {:else if noData}
      <h2 style="font-size: 1.4rem; font-weight: bold; margin-bottom: 0.5rem;">
      Chart '{chartType}'
      </h2>
      <p style="font-size: 1rem; color: #666; margin-top: 0;">
        Chart '{chartType}' has no data
      </p>
    {:else}
      <Chart {init} {options} style="width: 100%; height: 100%;"/>
    {/if}
  </div>
</div>
