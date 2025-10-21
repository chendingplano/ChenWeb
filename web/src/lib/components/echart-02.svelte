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
    grid: {
    top: '28%',    // reduce top margin
    bottom: '20%',
    left: '12%',
    right: '8%'
    },
    title: {
      text: 'Bar Chat Example',
      textStyle: {
      fontSize: 14,        // 👈 font size in pixels
      fontWeight: 'bold', // 'normal', 'bold', or number (e.g., 600)
      color: '#333',        // optional: text color
      fontFamily: 'Inter, sans-serif' // optional
      },
      left: 'left',      // optional: position of the title
      top: '0%'           // optional: position of the title
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

<div class="w-full h-[130px]">
	<Chart {init} {options} style="pt-16 width: 100%; height: 100%"/>
</div>
