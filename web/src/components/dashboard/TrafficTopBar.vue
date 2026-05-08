<script setup>
import { nextTick, onBeforeUnmount, onMounted, ref, watch } from "vue";
import { formatBps } from "../../utils/format";
import { npAxisLabel, npAxisLine, npChartGrid, npSplitLine, npTooltip } from "../../utils/chartTheme";

const props = defineProps({
  hotspots: { type: Array, default: () => [] }
});

const chartRef = ref(null);
let chart = null;

function render() {
  if (!chart) return;
  const list = props.hotspots || [];
  chart.setOption({
    animation: false,
    grid: npChartGrid,
    tooltip: npTooltip({
      axisPointer: { type: "shadow" },
      formatter: (params) => {
        const p = params?.[0];
        if (!p) return "";
        return `${p.name}<br/>带宽: ${formatBps(Number(p.value || 0))}`;
      }
    }),
    xAxis: {
      type: "value",
      axisLabel: { ...npAxisLabel, formatter: (v) => formatBps(v) },
      axisLine: npAxisLine,
      splitLine: npSplitLine
    },
    yAxis: {
      type: "category",
      data: list.map((x) => `${x.deviceName}/${x.interfaceName}`),
      axisLabel: { ...npAxisLabel, width: 260, overflow: "truncate" },
      axisLine: npAxisLine
    },
    series: [
      {
        type: "bar",
        data: list.map((x) => Number(x.bps || 0)),
        itemStyle: {
          color: (ctx) => {
            const v = Number(ctx.value || 0);
            const max = Number(list[0]?.bps || 1);
            return v >= max * 0.9 ? "#ef4444" : "#10b981";
          },
          borderRadius: [0, 6, 6, 0]
        }
      }
    ]
  });
}

function resize() {
  chart?.resize();
}

onMounted(async () => {
  const e = await import("echarts");
  await nextTick();
  chart = e.init(chartRef.value);
  render();
  window.addEventListener("resize", resize);
});

onBeforeUnmount(() => {
  window.removeEventListener("resize", resize);
  chart?.dispose();
});

watch(() => props.hotspots, render, { deep: true });
</script>

<template>
  <el-card>
    <template #header>
      <span class="text-lg font-semibold">Top N 端口流量排行</span>
    </template>
    <el-empty v-if="!(props.hotspots || []).length" description="暂无端口流量数据（等待采样入库）" :image-size="72" />
    <div v-else ref="chartRef" class="h-[260px] w-full"></div>
  </el-card>
</template>
