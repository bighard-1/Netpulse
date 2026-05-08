<script setup>
import { nextTick, onBeforeUnmount, onMounted, ref, watch } from "vue";

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
    grid: { left: "3%", right: "4%", bottom: "10%", containLabel: true },
    tooltip: { trigger: "axis" },
    legend: { top: 8, data: ["健康分", "可用率"] },
    xAxis: {
      type: "category",
      data: list.map((x) => new Date(x.ts || x.timestamp).toLocaleString()),
      axisLabel: { hideOverlap: true, rotate: 30 }
    },
    yAxis: { type: "value", min: 0, max: 100, axisLabel: { formatter: "{value}%" } },
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
    <div ref="chartRef" class="h-[280px] w-full"></div>
  </el-card>
</template>

