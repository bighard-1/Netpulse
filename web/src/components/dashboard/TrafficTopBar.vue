<script setup>
import { nextTick, onBeforeUnmount, onMounted, ref, watch } from "vue";
import { formatBps } from "../../utils/format";
import { npAxisLabel, npAxisLine, npChartGrid, npSplitLine, npTooltip } from "../../utils/chartTheme";

const props = defineProps({
  title: { type: String, default: "Top N 端口流量排行" },
  hotspots: { type: Array, default: () => [] },
  refreshKey: { type: Number, default: 0 },
  loading: { type: Boolean, default: false },
  expanded: { type: Boolean, default: false }
});
const emit = defineEmits(["open-port"]);

const chartRef = ref(null);
let chart = null;
let echartsMod = null;
let resizeObserver = null;
let redrawFrame = 0;

function render() {
  if (!chart) return;
  const list = props.hotspots || [];
  const yLabels = list.map((x) => {
    const full = `${x.deviceName}/#${Number(x.interfaceIndex || 0)} ${x.interfaceName}`;
    return full.length > 20 ? `${full.slice(0, 19)}…` : full;
  });
  chart.setOption({
    animation: false,
    grid: npChartGrid,
    tooltip: npTooltip({
      axisPointer: { type: "shadow" },
      formatter: (params) => {
        const p = params?.[0];
        if (!p) return "";
        const src = (props.hotspots || [])[Number(p.dataIndex || 0)] || {};
        return [
          `${src.deviceName || "-"} / #${Number(src.interfaceIndex || 0)} ${src.interfaceName || "-"}`,
          `入方向: ${formatBps(Number(src.inBps || 0))}`,
          `出方向: ${formatBps(Number(src.outBps || 0))}`,
          `总流量: ${formatBps(Number(src.bps || p.value || 0))}`
        ].join("<br/>");
      }
    }),
    xAxis: {
      type: "value",
      axisLabel: { ...npAxisLabel, formatter: (v) => formatBps(v) },
      axisLine: npAxisLine,
      splitLine: npSplitLine,
      splitNumber: 3
    },
    yAxis: {
      type: "category",
      data: yLabels,
      axisLabel: { ...npAxisLabel, width: 140, overflow: "truncate", interval: 0 },
      axisLine: npAxisLine
    },
    series: [
      {
        type: "bar",
        data: list.map((x) => Number(x.bps || 0)),
        barMaxWidth: 18,
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
  chart.on("click", (params) => {
    const idx = Number(params?.dataIndex ?? -1);
    if (idx < 0) return;
    const item = (props.hotspots || [])[idx];
    if (item) emit("open-port", item);
  });
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

watch(() => props.hotspots, async () => {
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
  <div>
    <div class="mb-2 text-sm font-semibold text-slate-700">{{ props.title }}</div>
    <el-skeleton :loading="props.loading" animated>
      <template #template><div :class="props.expanded ? 'h-[430px]' : 'h-[300px]'" class="w-full rounded-lg bg-slate-100"></div></template>
      <template #default>
        <el-empty v-if="!(props.hotspots || []).length" description="暂无端口流量数据（等待采样入库）" :image-size="72" />
        <div v-else class="space-y-2">
          <div ref="chartRef" :class="props.expanded ? 'h-[280px]' : 'h-[220px]'" class="w-full"></div>
          <div :class="props.expanded ? 'max-h-[260px]' : 'max-h-[150px]'" class="overflow-auto rounded-lg border border-slate-200">
            <el-table :data="props.hotspots" size="small" class="np-borderless-table">
              <el-table-column label="端口" min-width="220">
                <template #default="{ row }">
                  <el-link type="primary" @click.prevent="emit('open-port', row)">{{ row.deviceName }}/#{{ Number(row.interfaceIndex || 0) }} {{ row.interfaceName }}</el-link>
                </template>
              </el-table-column>
              <el-table-column label="入方向" width="110">
                <template #default="{ row }">{{ formatBps(Number(row.inBps || 0)) }}</template>
              </el-table-column>
              <el-table-column label="出方向" width="110">
                <template #default="{ row }">{{ formatBps(Number(row.outBps || 0)) }}</template>
              </el-table-column>
              <el-table-column label="总流量" width="110">
                <template #default="{ row }">{{ formatBps(Number(row.bps || 0)) }}</template>
              </el-table-column>
            </el-table>
          </div>
        </div>
      </template>
    </el-skeleton>
  </div>
</template>
