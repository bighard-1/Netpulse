<script setup>
import { nextTick, onBeforeUnmount, onMounted, ref, watch } from "vue";
import { npAxisLabel, npAxisLine, npChartGrid, npSplitLine, npTooltip } from "../../utils/chartTheme";

const props = defineProps({
  trend: { type: Array, default: () => [] },
  refreshKey: { type: Number, default: 0 },
  loading: { type: Boolean, default: false },
  compact: { type: Boolean, default: false }
});

const chartRef = ref(null);
let chart = null;
let echartsMod = null;
let resizeObserver = null;
let redrawFrame = 0;

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

async function redraw() {
  await ensureChart();
  await nextTick();
  resize();
  render();
}

async function ensureChart() {
  if (chart || !chartRef.value) return;
  bindResizeObserver();
  if (!echartsMod) {
    echartsMod = await import("echarts");
  }
  await nextTick();
  if (!chartRef.value || chart) return;
  const box = chartRef.value.getBoundingClientRect();
  if (box.width < 40 || box.height < 40) return;
  chart = echartsMod.init(chartRef.value);
}

function scheduleRedraw() {
  if (redrawFrame) cancelAnimationFrame(redrawFrame);
  redrawFrame = requestAnimationFrame(async () => {
    redrawFrame = 0;
    await redraw();
  });
}

function bindResizeObserver() {
  if (resizeObserver || !chartRef.value) return;
  resizeObserver = new ResizeObserver(() => scheduleRedraw());
  resizeObserver.observe(chartRef.value);
}

onMounted(async () => {
  window.addEventListener("resize", scheduleRedraw);
  await redraw();
});

onBeforeUnmount(() => {
  window.removeEventListener("resize", scheduleRedraw);
  if (redrawFrame) cancelAnimationFrame(redrawFrame);
  resizeObserver?.disconnect();
  chart?.dispose();
});

watch(() => props.trend, async () => {
  scheduleRedraw();
}, { deep: true });

watch(() => props.refreshKey, async () => {
  scheduleRedraw();
});

watch(() => props.loading, async (v) => {
  if (!v) {
    scheduleRedraw();
  }
});
</script>

<template>
  <el-card>
    <template #header>
      <span class="text-lg font-semibold">全网健康趋势</span>
    </template>
    <el-skeleton :loading="props.loading" animated>
      <template #template><div :class="props.compact ? 'h-[260px] min-h-[260px]' : 'h-[320px] min-h-[320px]'" class="w-full rounded-lg bg-slate-100"></div></template>
      <template #default>
        <el-empty v-if="!(props.trend || []).length" description="暂无健康趋势数据（等待15分钟采样）" :image-size="72" />
        <div v-else ref="chartRef" :class="props.compact ? 'h-[260px] min-h-[260px]' : 'h-[320px] min-h-[320px]'" class="w-full"></div>
      </template>
    </el-skeleton>
  </el-card>
</template>
