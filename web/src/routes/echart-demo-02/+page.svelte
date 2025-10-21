<script lang="ts">
  // Source: https://github.com/bherbruck/svelte-echarts?tab=readme-ov-file
  // This example shows the Large Area Chart. Below is what you need to do
  // to show a specific echart:
  //   - Go to https://echarts.apache.org/examples/en/index.html to find the echart you want
  //   - View the TS code
  //   - Copy the option and paste it to "EChartsOption" below
  //   - If it has script to generate data, copy the code to this file.
  //   - Good Luck!
  import { Chart } from 'svelte-echarts';
  import type { EChartsOption } from 'echarts'; // 👈 import the type
  import * as echarts from 'echarts';

  import { init, use } from 'echarts/core';
  import { BarChart } from 'echarts/charts';
  import { GridComponent, TitleComponent } from 'echarts/components';
  import { CanvasRenderer } from 'echarts/renderers';

  use([BarChart, GridComponent, CanvasRenderer, TitleComponent]);

  let base = +new Date(1968, 9, 3);
  let oneDay = 24 * 3600 * 1000;
  let date = [];
  let data = [Math.random() * 300];
  for (let i = 1; i < 20000; i++) {
    var now = new Date((base += oneDay));
    date.push([now.getFullYear(), now.getMonth() + 1, now.getDate()].join('/'));
    data.push(Math.round((Math.random() - 0.5) * 20 + data[i - 1]));
  }

  const options: EChartsOption = {
  tooltip: {
    trigger: 'axis',
    position: function (pt) {
      return [pt[0], '10%'];
    }
  },
  title: {
    left: 'center',
    text: 'Large Area Chart'
  },
  toolbox: {
    feature: {
      dataZoom: {
        yAxisIndex: 'none'
      },
      restore: {},
      saveAsImage: {}
    }
  },
  xAxis: {
    type: 'category',
    boundaryGap: false,
    data: date
  },
  yAxis: {
    type: 'value',
    boundaryGap: [0, '100%']
  },
  dataZoom: [
    {
      type: 'inside',
      start: 0,
      end: 10
    },
    {
      start: 0,
      end: 10
    }
  ],
  series: [
    {
      name: 'Fake Data',
      type: 'line',
      symbol: 'none',
      sampling: 'lttb',
      itemStyle: {
        color: 'rgb(255, 70, 131)'
      },
      areaStyle: {
        color: new echarts.graphic.LinearGradient(0, 0, 0, 1, [
          {
            offset: 0,
            color: 'rgb(255, 158, 68)'
          },
          {
            offset: 1,
            color: 'rgb(255, 70, 131)'
          }
        ])
      },
      data: data
    }
  ]

  };
</script>

<div class="app">
	<div class="chart-wrapper">
		<Chart {init} {options} />
	</div>
</div>

<style>
  .app {
    width: 100vw;
    height: 100vh;
    display: flex;
    justify-content: center;
    align-items: center;
    /* Optional: prevent scrollbars if content overflows */
    overflow: hidden;
  }

  .chart-wrapper {
    width: 800px;   /* your fixed width */
    height: 600px;  /* your fixed height */
    /* Ensures the Chart fills the wrapper */
  }
</style>


