<script lang="ts">
  // Source: https://github.com/bherbruck/svelte-echarts?tab=readme-ov-file
  import { Chart } from 'svelte-echarts';
  import type { EChartsOption } from 'echarts'; // 👈 import the type

  import { init, use } from 'echarts/core';
  import { BarChart } from 'echarts/charts';
  import { GridComponent, TitleComponent } from 'echarts/components';
  import { CanvasRenderer } from 'echarts/renderers';

  use([BarChart, GridComponent, CanvasRenderer, TitleComponent]);

  const options: EChartsOption = {
    title: {
      text: 'Bar Chat Example',
    },
    xAxis: {
      type: 'category',
      data: ['Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat', 'Sun'],
    },
    yAxis: {
      type: 'value',
    },
    series: [
      {
        type: 'bar',
        data: [120, 200, 150, 80, 70, 110, 130],
      },
    ],
  };
</script>

<div class="w-full h-full pt-8">
	<Chart {init} {options} style="width: 100%; height: 100%"/>
</div>
