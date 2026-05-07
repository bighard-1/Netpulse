<script setup>
import { nextTick, onBeforeUnmount, onMounted, ref } from "vue";
import { useRoute } from "vue-router";
import { ElMessage } from "element-plus";
import { api, getApiError } from "../services/api";

const props = defineProps({ id: { type: [String, Number], required: true } });
const route = useRoute();

const loading = ref(false);
const range = ref([]);
const chartRef = ref(null);
const portMeta = ref({ id: props.id, name: route.query.portName || `端口-${props.id}` });
let chart = null;

const shortcuts = [
  { text: "最近 24 小时", value: () => [new Date(Date.now() - 24 * 3600 * 1000), new Date()] },
  { text: "最近 7 天", value: () => [new Date(Date.now() - 7 * 24 * 3600 * 1000), new Date()] },
  { text: "最近 30 天", value: () => [new Date(Date.now() - 30 * 24 * 3600 * 1000), new Date()] }
];

function getRange() {
  if (range.value?.length === 2) return { start: range.value[0], end: range.value[1] };
  const end = new Date();
  const start = new Date(end.getTime() - 24 * 3600 * 1000);
  return { start, end };
}

async function loadHistory() {
  loading.value = true;
  try {
    const { start, end } = getRange();
    const res = await api.getHistory("traffic", props.id, start.toISOString(), end.toISOString());
    const data = res.data.data || [];
    const hasData = data.length > 0;
    chart?.setOption({
      grid: { left: 50, right: 20, top: 40, bottom: 40 },
      tooltip: { trigger: "axis" },
      legend: { data: ["入方向 bps", "出方向 bps"] },
      title: hasData ? undefined : {
        text: "当前时间范围暂无流量数据",
        left: "center",
        top: "middle",
        textStyle: { color: "#94a3b8", fontSize: 14, fontWeight: "normal" }
      },
      xAxis: { type: "category", data: data.map((p) => p.timestamp) },
      yAxis: { type: "value" },
      series: [
        { name: "入方向 bps", type: "line", smooth: true, areaStyle: { opacity: 0.08 }, data: data.map((p) => Number(p.traffic_in_bps || 0)) },
        { name: "出方向 bps", type: "line", smooth: true, areaStyle: { opacity: 0.08 }, data: data.map((p) => Number(p.traffic_out_bps || 0)) }
      ]
    }, true);
  } catch (err) {
    ElMessage.error(getApiError(err, "加载端口流量失败"));
  } finally {
    loading.value = false;
  }
}

onMounted(async () => {
  await nextTick();
  const m = await import("echarts");
  chart = m.init(chartRef.value);
  chart.setOption({
    grid: { left: 50, right: 20, top: 40, bottom: 40 },
    tooltip: { trigger: "axis" },
    legend: { data: ["入方向 bps", "出方向 bps"] },
    xAxis: { type: "category", data: [] },
    yAxis: { type: "value" },
    series: [
      { name: "入方向 bps", type: "line", smooth: true, areaStyle: { opacity: 0.08 }, data: [] },
      { name: "出方向 bps", type: "line", smooth: true, areaStyle: { opacity: 0.08 }, data: [] }
    ]
  }, true);
  await loadHistory();
  window.addEventListener("resize", resizeChart);
});

onBeforeUnmount(() => {
  window.removeEventListener("resize", resizeChart);
  chart?.dispose();
});

function resizeChart() {
  chart?.resize();
}
</script>

<template>
  <div class="space-y-5">
    <el-breadcrumb separator=">">
      <el-breadcrumb-item :to="{ path: '/' }">资产</el-breadcrumb-item>
      <el-breadcrumb-item :to="{ path: `/device/${route.query.deviceId || ''}` }">{{ route.query.deviceIp || '设备' }}</el-breadcrumb-item>
      <el-breadcrumb-item>{{ portMeta.name }}</el-breadcrumb-item>
    </el-breadcrumb>

    <el-card>
      <div class="flex flex-wrap items-center justify-between gap-3">
        <div>
          <div class="text-xs text-slate-500">端口</div>
          <div class="text-lg font-semibold">{{ portMeta.name }}</div>
        </div>

        <el-date-picker
          v-model="range"
          type="datetimerange"
          unlink-panels
          range-separator="至"
          start-placeholder="开始时间"
          end-placeholder="结束时间"
          :shortcuts="shortcuts"
          :disabled-date="(date)=> date.getTime() < Date.now() - 3 * 365 * 24 * 3600 * 1000"
          @change="loadHistory"
        />
      </div>
    </el-card>

    <el-card>
      <template #header><span class="text-base font-semibold">端口流量（入/出）</span></template>
      <div v-loading="loading">
        <div ref="chartRef" class="h-[560px] w-full"></div>
      </div>
    </el-card>
  </div>
</template>
