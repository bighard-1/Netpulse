<script setup>
import { nextTick, onBeforeUnmount, onMounted, ref, watch } from "vue";
import { npAxisLabel, npAxisLine, npChartGrid, npSplitLine, npTooltip } from "../../utils/chartTheme";

const props = defineProps({
  trend: { type: Array, default: () => [] }
});

const chartRef = ref(null);
let chart = null;

function render() {
  if (!chart) return;
  const list = props.trend || [];
  chart.setOption({
    animation: false,
    grid: npChartGrid,
    tooltip: npTooltip(),
    legend: { top: 8, data: ["健康分", "可用率"] },
    xAxis: {
      type: "category",
      data: list.map((x) => new Date(x.ts || x.timestamp).toLocaleString()),
      axisLabel: { ...npAxisLabel, hideOverlap: true, rotate: 30 },
      axisLine: npAxisLine
    },
    yAxis: {
      type: "value",
      min: 0,
      max: 100,
      axisLabel: { ...npAxisLabel, formatter: "{value}%" },
      axisLine: npAxisLine,
      splitLine: npSplitLine
    },
    series: [
      {
        name: "健康分",
        type: "line",
        smooth: true,
        showSymbol: false,
        areaStyle: { opacity: 0.2 },
        color: "#10b981",
        data: list.map((x) => Number(x.score || 0))
      },
      {
        name: "可用率",
        type: "line",
        smooth: true,
        showSymbol: false,
        areaStyle: { opacity: 0.12 },
        color: "#f59e0b",
        data: list.map((x) => Number(x.availability || 0))
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

watch(() => props.trend, render, { deep: true });
</script>

<template>
  <el-card>
    <template #header>
      <span class="text-lg font-semibold">全网健康趋势（Area）</span>
    </template>
    <el-empty v-if="!(props.trend || []).length" description="暂无健康趋势数据（等待15分钟采样）" :image-size="72" />
    <div v-else ref="chartRef" class="h-[280px] w-full"></div>
  </el-card>
</template>
