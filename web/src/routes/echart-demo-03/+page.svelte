<script lang="ts">
  import { Chart } from 'svelte-echarts';
  import type { EChartsOption } from 'echarts';
  import * as echarts from 'echarts';
  import { init, use } from 'echarts/core';
  import { BarChart } from 'echarts/charts';
  import { GridComponent, TitleComponent } from 'echarts/components';
  import { CanvasRenderer } from 'echarts/renderers';
  import { onMount } from 'svelte';

  let base = +new Date(1968, 9, 3);
  let oneDay = 24 * 3600 * 1000;
  let local_date = [];
  let local_data = [Math.random() * 300];
  for (let i = 1; i < 20000; i++) {
    var now = new Date((base += oneDay));
    local_date.push([now.getFullYear(), now.getMonth() + 1, now.getDate()].join('/'));
    local_data.push(Math.round((Math.random() - 0.5) * 20 + local_data[i - 1]));
  }

  use([BarChart, GridComponent, CanvasRenderer, TitleComponent]);

  let date = $state<string[]>([]);
  let data = $state<number[]>([]);
  let loading = $state<boolean>(true);
  let error = $state<string>('');

  onMount(async () => {
    try {
      // const res = await fetch('/api/echart-data/demo-03');
      // if (!res.ok) throw new Error('Failed to fetch chart data (MID_ED3_021)');
      // const json = await res.json();
      // date = json.date;
      // data = json.data;
      date = local_date;
      data = local_data;
    } catch (e) {
      error = e.message;
    } finally {
      loading = false;
    }
  });

  const options = $derived<EChartsOption>({
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
  });
</script>

<div class="app">
  <div class="chart-wrapper">
    {#if loading}
      <p>Loading chart data...</p>
    {:else if error}
      <p style="color:red">{error}</p>
    {:else}
      <Chart {init} {options} />
    {/if}
  </div>
</div>

<style>
  .app {
    width: 100vw;
    height: 100vh;
    display: flex;
    justify-content: center;
    align-items: center;
    overflow: hidden;
  }

  .chart-wrapper {
    width: 800px;
    height: 600px;
  }
</style>