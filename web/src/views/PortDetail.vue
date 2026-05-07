<script setup>
import { nextTick, onBeforeUnmount, onMounted, ref } from "vue";
import { useRoute } from "vue-router";
import { ElMessage } from "element-plus";
import { api, getApiError } from "../services/api";

const props = defineProps({ id: { type: [String, Number], required: true } });
const route = useRoute();

const loading = ref(false);
const chartTodayRef = ref(null);
const chart7dRef = ref(null);
const chartMonthRef = ref(null);
const portMeta = ref({ id: props.id, name: route.query.portName || `端口-${props.id}` });
let charts = { today: null, d7: null, month: null };

function startOfDay(d = new Date()) {
  const x = new Date(d);
  x.setHours(0, 0, 0, 0);
  return x;
}

function startOfMonth(d = new Date()) {
  const x = new Date(d.getFullYear(), d.getMonth(), 1, 0, 0, 0, 0);
  return x;
}

function bpsLabel(v) {
  const n = Number(v || 0);
  if (n >= 1e9) return `${(n / 1e9).toFixed(2)} Gbps`;
  if (n >= 1e6) return `${(n / 1e6).toFixed(2)} Mbps`;
  if (n >= 1e3) return `${(n / 1e3).toFixed(2)} Kbps`;
  return `${n.toFixed(0)} bps`;
}

function pickUnit(maxVal) {
  if (maxVal >= 1e9) return { unit: "Gbps", div: 1e9 };
  if (maxVal >= 1e6) return { unit: "Mbps", div: 1e6 };
  if (maxVal >= 1e3) return { unit: "Kbps", div: 1e3 };
  return { unit: "bps", div: 1 };
}

function baseOption(title, unitInfo) {
  return {
    animation: false,
    grid: { left: 82, right: 20, top: 42, bottom: 48 },
    title: { text: title, left: 10, top: 8, textStyle: { fontSize: 14, fontWeight: 600 } },
    tooltip: {
      trigger: "axis",
      axisPointer: { type: "line", animation: false },
      formatter(params) {
        if (!params?.length) return "";
        const ts = new Date(params[0].data[0]).toLocaleString();
        const lines = [ts];
        for (const p of params) {
          lines.push(`${p.marker}${p.seriesName}: ${bpsLabel(p.data[1])}`);
        }
        return lines.join("<br/>");
      }
    },
    legend: { top: 8, right: 10, data: ["入方向", "出方向"] },
    xAxis: {
      type: "time",
      axisLabel: { hideOverlap: true }
    },
    yAxis: {
      type: "value",
      splitNumber: 6,
      name: unitInfo.unit,
      axisLabel: { formatter: (val) => `${(val / unitInfo.div).toFixed(2)}` }
    },
    series: [
      {
        name: "入方向",
        type: "line",
        showSymbol: false,
        smooth: false,
        sampling: "lttb",
        progressive: 5000,
        data: []
      },
      {
        name: "出方向",
        type: "line",
        showSymbol: false,
        smooth: false,
        sampling: "lttb",
        progressive: 5000,
        data: []
      }
    ]
  };
}

function toSeriesData(data) {
  const inbound = [];
  const outbound = [];
  for (const p of data || []) {
    const t = new Date(p.timestamp).getTime();
    if (!Number.isFinite(t)) continue;
    inbound.push([t, Number(p.traffic_in_bps || 0)]);
    outbound.push([t, Number(p.traffic_out_bps || 0)]);
  }
  return { inbound, outbound };
}

async function fetchRange(start, end) {
  const res = await api.getHistory("traffic", props.id, start.toISOString(), end.toISOString());
  return res.data.data || [];
}

function applyChart(chart, title, data) {
  if (!chart) return;
  const { inbound, outbound } = toSeriesData(data);
  const hasData = inbound.length > 0 || outbound.length > 0;
  const maxVal = Math.max(1, ...inbound.map((x) => x[1]), ...outbound.map((x) => x[1]));
  const unitInfo = pickUnit(maxVal);
  const opt = baseOption(title, unitInfo);
  opt.series[0].data = inbound;
  opt.series[1].data = outbound;
  if (!hasData) {
    opt.graphic = [{
      type: "text",
      left: "center",
      top: "middle",
      style: { text: "当前时间范围暂无流量数据", fill: "#94a3b8", fontSize: 14 }
    }];
  }
  chart.setOption(opt, true);
}

async function loadAllCharts() {
  loading.value = true;
  try {
    const now = new Date();
    const todayStart = startOfDay(now);
    const d7Start = startOfDay(new Date(now.getTime() - 6 * 24 * 3600 * 1000));
    const monthStart = startOfMonth(now);

    const [today, d7, month] = await Promise.all([
      fetchRange(todayStart, now),
      fetchRange(d7Start, now),
      fetchRange(monthStart, now)
    ]);

    applyChart(charts.today, "当日流量", today);
    applyChart(charts.d7, "近7天流量", d7);
    applyChart(charts.month, "本月流量", month);
  } catch (err) {
    ElMessage.error(getApiError(err, "加载端口流量失败"));
  } finally {
    loading.value = false;
  }
}

function resizeCharts() {
  charts.today?.resize();
  charts.d7?.resize();
  charts.month?.resize();
}

onMounted(async () => {
  await nextTick();
  const e = await import("echarts");
  charts.today = e.init(chartTodayRef.value);
  charts.d7 = e.init(chart7dRef.value);
  charts.month = e.init(chartMonthRef.value);
  applyChart(charts.today, "当日流量", []);
  applyChart(charts.d7, "近7天流量", []);
  applyChart(charts.month, "本月流量", []);
  await loadAllCharts();
  window.addEventListener("resize", resizeCharts);
});

onBeforeUnmount(() => {
  window.removeEventListener("resize", resizeCharts);
  charts.today?.dispose();
  charts.d7?.dispose();
  charts.month?.dispose();
});
</script>

<template>
  <div class="space-y-5">
    <el-breadcrumb separator=">">
      <el-breadcrumb-item :to="{ path: '/' }">资产</el-breadcrumb-item>
      <el-breadcrumb-item :to="{ path: `/device/${route.query.deviceId || ''}` }">{{ route.query.deviceIp || '设备' }}</el-breadcrumb-item>
      <el-breadcrumb-item>{{ portMeta.name }}</el-breadcrumb-item>
    </el-breadcrumb>

    <el-card>
      <div class="flex items-center justify-between gap-2">
        <div>
          <div class="text-xs text-slate-500">端口</div>
          <div class="text-lg font-semibold">{{ portMeta.name }}</div>
        </div>
        <el-button @click="loadAllCharts" :loading="loading">刷新流量图</el-button>
      </div>
    </el-card>

    <el-card><div ref="chartTodayRef" class="h-[300px] w-full" v-loading="loading"></div></el-card>
    <el-card><div ref="chart7dRef" class="h-[300px] w-full" v-loading="loading"></div></el-card>
    <el-card><div ref="chartMonthRef" class="h-[300px] w-full" v-loading="loading"></div></el-card>
  </div>
</template>
