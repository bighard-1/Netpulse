<script setup>
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from "vue";
import { useRoute, useRouter } from "vue-router";
import { api } from "../services/api";
import { formatBps } from "../utils/format";
import { zhCN } from "../i18n/zhCN";
import { useFeedback } from "../composables/useFeedback";
import { npAxisLabel, npAxisLine, npChartGrid, npSplitLine, npTooltip } from "../utils/chartTheme";

const props = defineProps({ id: { type: [String, Number], required: true } });
const route = useRoute();
const router = useRouter();
const fb = useFeedback();
const editMode = ref(localStorage.getItem("np_edit_mode") === "1");

const loading = ref(false);
const customRange = ref([]);
const customRangeDraft = ref([]);
const chartTodayRef = ref(null);
const chart7dRef = ref(null);
const chart30dRef = ref(null);
const chartCustomRef = ref(null);
const portMeta = ref({ id: props.id, name: route.query.portName || `端口-${props.id}` });
const portEdit = ref({ name: route.query.portName || "", remark: route.query.portRemark || "" });
const portBaseName = ref(String(route.query.portBaseName || route.query.portName || `端口-${props.id}`));
const portSuffix = ref("");
const savingPort = ref(false);
const terminalType = ref("ssh");
const customChartAnchorRef = ref(null);
const trafficThresholdBps = ref(0);
const chartCardActive = ref("today");
const siblingPorts = ref([]);
const lastSeriesCache = ref({
  today: [],
  d7: [],
  d30: [],
  custom: []
});
const runtimePollSec = ref(60);
let charts = { today: null, d7: null, d30: null, custom: null };

function startOfDay(d = new Date()) {
  const x = new Date(d);
  x.setHours(0, 0, 0, 0);
  return x;
}

function startOfMonth(d = new Date()) {
  const x = new Date(d.getFullYear(), d.getMonth(), 1, 0, 0, 0, 0);
  return x;
}

function startOfWeek(d = new Date()) {
  const x = new Date(d);
  const day = x.getDay();
  const delta = day === 0 ? 6 : day - 1;
  x.setDate(x.getDate() - delta);
  x.setHours(0, 0, 0, 0);
  return x;
}

function startOfYear(d = new Date()) {
  return new Date(d.getFullYear(), 0, 1, 0, 0, 0, 0);
}

const pickerShortcuts = [
  { text: "本周", value: () => [startOfWeek(new Date()), new Date()] },
  {
    text: "上周",
    value: () => {
      const thisWeek = startOfWeek(new Date());
      const lastWeekStart = new Date(thisWeek);
      lastWeekStart.setDate(lastWeekStart.getDate() - 7);
      return [lastWeekStart, new Date(thisWeek.getTime() - 1000)];
    }
  },
  { text: "本月", value: () => [startOfMonth(new Date()), new Date()] },
  {
    text: "上月",
    value: () => {
      const now = new Date();
      const thisMonth = startOfMonth(now);
      return [new Date(now.getFullYear(), now.getMonth() - 1, 1, 0, 0, 0, 0), new Date(thisMonth.getTime() - 1000)];
    }
  },
  { text: "本年", value: () => [startOfYear(new Date()), new Date()] },
  {
    text: "上年",
    value: () => {
      const thisYear = startOfYear(new Date());
      return [new Date(thisYear.getFullYear() - 1, 0, 1, 0, 0, 0, 0), new Date(thisYear.getTime() - 1000)];
    }
  }
];

function bpsLabel(v) {
  return formatBps(v);
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
    grid: npChartGrid,
    title: {
      text: title,
      subtext: `单位: ${unitInfo.unit}`,
      left: 10,
      top: 8,
      textStyle: { fontSize: 14, fontWeight: 600 },
      subtextStyle: { fontSize: 12, color: "#64748b", lineHeight: 14 }
    },
    tooltip: npTooltip({
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
    }),
    legend: { top: 8, right: 10, data: ["入方向", "出方向"] },
    dataZoom: [
      { type: "inside", throttle: 60, zoomOnMouseWheel: true, moveOnMouseMove: true },
      { type: "slider", height: 18, bottom: 0 }
    ],
    xAxis: {
      type: "time",
      axisLabel: { ...npAxisLabel, hideOverlap: true, rotate: 45 },
      axisLine: npAxisLine
    },
    yAxis: {
      type: "value",
      min: 0,
      max: 100,
      splitNumber: 6,
      axisLabel: { ...npAxisLabel, formatter: (val) => `${(val / unitInfo.div).toFixed(2)}` },
      axisLine: npAxisLine,
      splitLine: npSplitLine
    },
    series: [
      {
        name: "入方向",
        type: "line",
        showSymbol: false,
        smooth: false,
        step: false,
        connectNulls: false,
        sampling: "lttb",
        progressive: 5000,
        lineStyle: { color: "#6366F1", width: 2 },
        itemStyle: { color: "#6366F1" },
        data: [],
        markLine: trafficThresholdBps.value > 0 ? {
          symbol: "none",
          label: { show: true, formatter: `阈值 ${formatBps(trafficThresholdBps.value)}` },
          lineStyle: { color: "#ef4444", type: "dashed" },
          data: [{ yAxis: trafficThresholdBps.value }]
        } : undefined
      },
      {
        name: "出方向",
        type: "line",
        showSymbol: false,
        smooth: false,
        step: false,
        connectNulls: false,
        sampling: "lttb",
        progressive: 5000,
        lineStyle: { color: "#22C55E", width: 2 },
        itemStyle: { color: "#22C55E" },
        data: [],
        markLine: trafficThresholdBps.value > 0 ? {
          symbol: "none",
          label: { show: false },
          lineStyle: { color: "#ef4444", type: "dashed" },
          data: [{ yAxis: trafficThresholdBps.value }]
        } : undefined
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
    const inV = p.traffic_in_bps == null ? null : Number(p.traffic_in_bps);
    const outV = p.traffic_out_bps == null ? null : Number(p.traffic_out_bps);
    inbound.push([t, Number.isFinite(inV) ? inV : null]);
    outbound.push([t, Number.isFinite(outV) ? outV : null]);
  }
  return { inbound, outbound };
}

function decimatePoints(points, maxPoints = 2200) {
  const arr = points || [];
  if (arr.length <= maxPoints) return arr;
  const stride = Math.max(1, Math.ceil(arr.length / maxPoints));
  const out = [];
  for (let i = 0; i < arr.length; i += stride) {
    out.push(arr[i]);
  }
  if (arr.length > 0 && out[out.length - 1] !== arr[arr.length - 1]) {
    out.push(arr[arr.length - 1]);
  }
  return out;
}

async function fetchRange(start, end) {
  const spanMs = end.getTime() - start.getTime();
  const pollSec = Math.max(5, Number(runtimePollSec.value || 60));
  let interval = `${Math.max(10, Math.round(pollSec / 5) * 5)}s`;
  if (spanMs > 180 * 24 * 3600 * 1000) interval = "1h";
  else if (spanMs > 30 * 24 * 3600 * 1000) interval = "5m";
  else if (spanMs > 7 * 24 * 3600 * 1000) interval = "2m";
  const maxPoints = spanMs > 365 * 24 * 3600 * 1000 ? 1500 : 2500;
  const res = await api.getHistory("traffic", props.id, start.toISOString(), end.toISOString(), interval, maxPoints);
  return res.data.data || [];
}

async function loadRuntimePollSec() {
  const deviceID = Number(route.query.deviceId || 0);
  try {
    const runtimeRes = await api.getRuntimeSettings();
    const runtime = runtimeRes?.data || {};
    const core = Math.max(5, Number(runtime?.poll_interval_core_sec || 60));
    const agg = Math.max(5, Number(runtime?.poll_interval_agg_sec || 90));
    const access = Math.max(5, Number(runtime?.poll_interval_access_sec || 120));
    const globalPoll = Math.max(5, Number(runtime?.snmp_poll_interval_sec || 60));
    if (deviceID > 0) {
      const d = await api.getDeviceById(deviceID);
      const perDevice = Number(d?.poll_interval_sec || 0);
      if (perDevice >= 5) {
        runtimePollSec.value = perDevice;
        return;
      }
      const text = `${String(d?.name || "")} ${String(d?.remark || "")}`.toLowerCase();
      if (text.includes("核心") || text.includes("core")) {
        runtimePollSec.value = core;
        return;
      }
      if (text.includes("汇聚") || text.includes("aggregation") || text.includes("agg")) {
        runtimePollSec.value = agg;
        return;
      }
      runtimePollSec.value = access;
      return;
    }
    runtimePollSec.value = globalPoll;
  } catch {
    runtimePollSec.value = 60;
  }
}

async function loadSiblingPorts() {
  const deviceID = Number(route.query.deviceId || 0);
  if (!deviceID) {
    siblingPorts.value = [];
    return;
  }
  try {
    const d = await api.getDeviceById(deviceID);
    const list = (d?.interfaces || []).slice().sort((a, b) => Number(a.index || 0) - Number(b.index || 0));
    siblingPorts.value = list.map((x) => ({
      id: Number(x.id),
      name: x.name || `ifIndex-${x.index}`,
      remark: x.remark || ""
    }));
  } catch {
    siblingPorts.value = [];
  }
}

const currentPortPos = computed(() => {
  const idx = siblingPorts.value.findIndex((x) => Number(x.id) === Number(props.id));
  return idx >= 0 ? idx : -1;
});
const prevPort = computed(() => {
  const i = currentPortPos.value;
  return i > 0 ? siblingPorts.value[i - 1] : null;
});
const nextPort = computed(() => {
  const i = currentPortPos.value;
  return i >= 0 && i < siblingPorts.value.length - 1 ? siblingPorts.value[i + 1] : null;
});

function jumpSibling(port) {
  if (!port?.id) return;
  const q = { ...route.query, portName: port.name, portRemark: port.remark || "" };
  router.push({ path: `/port/${port.id}`, query: q });
}

function applyChart(chart, title, data) {
  if (!chart) return;
  const { inbound, outbound } = toSeriesData(data);
  const inView = decimatePoints(inbound);
  const outView = decimatePoints(outbound);
  const hasData = inView.some((x) => x[1] != null) || outView.some((x) => x[1] != null);
  const nonNil = [
    ...inView.map((x) => x[1]).filter((v) => v != null),
    ...outView.map((x) => x[1]).filter((v) => v != null)
  ];
  const maxVal = Math.max(1, ...(nonNil.length ? nonNil : [1]));
  const unitInfo = pickUnit(maxVal);
  const opt = baseOption(title, unitInfo);
  opt.yAxis.max = maxVal * 1.1;
  opt.tooltip.confine = true;
  opt.tooltip.transitionDuration = 0;
  opt.series[0].large = true;
  opt.series[1].large = true;
  opt.series[0].largeThreshold = 2000;
  opt.series[1].largeThreshold = 2000;
  opt.series[0].data = inView;
  opt.series[1].data = outView;
  if (!hasData) {
    opt.graphic = [{
      type: "text",
      left: "center",
      top: "middle",
      style: { text: "当前时间范围暂无流量数据", fill: "#94a3b8", fontSize: 14 }
    }];
  }
  chart.setOption(opt, { notMerge: true, lazyUpdate: true, silent: true });
}

function saveChartPNG(chartKey) {
  const chart = charts[chartKey];
  if (!chart) return;
  const url = chart.getDataURL({ type: "png", pixelRatio: 2, backgroundColor: "#fff" });
  const a = document.createElement("a");
  a.href = url;
  a.download = `netpulse_port_${chartKey}_${Date.now()}.png`;
  a.click();
}

function exportChartCSV(chartKey, title) {
  const src = lastSeriesCache.value[chartKey] || [];
  if (!src.length) return fb.warn("当前图表无数据可导出");
  const lines = ["timestamp,traffic_in_bps,traffic_out_bps"];
  for (const p of src) {
    lines.push(`${p.timestamp},${p.traffic_in_bps == null ? "" : Number(p.traffic_in_bps)},${p.traffic_out_bps == null ? "" : Number(p.traffic_out_bps)}`);
  }
  const blob = new Blob([lines.join("\n")], { type: "text/csv;charset=utf-8" });
  const a = document.createElement("a");
  a.href = URL.createObjectURL(blob);
  a.download = `netpulse_${title}_${Date.now()}.csv`;
  a.click();
  URL.revokeObjectURL(a.href);
}

async function loadAllCharts() {
  loading.value = true;
  try {
    const now = new Date();
    const todayStart = startOfDay(now);
    const d7Start = startOfDay(new Date(now.getTime() - 6 * 24 * 3600 * 1000));
    const d30Start = startOfDay(new Date(now.getTime() - 29 * 24 * 3600 * 1000));

    const [today, d7, d30] = await Promise.all([
      fetchRange(todayStart, now),
      fetchRange(d7Start, now),
      fetchRange(d30Start, now)
    ]);
    lastSeriesCache.value.today = today;
    lastSeriesCache.value.d7 = d7;
    lastSeriesCache.value.d30 = d30;

    applyChart(charts.today, "当日流量", today);
    applyChart(charts.d7, "近7天流量", d7);
    applyChart(charts.d30, "近30天流量", d30);
    if (customRange.value?.length === 2) {
      await loadCustomChart();
    }
  } catch (err) {
    fb.apiError(err, "加载端口流量失败");
  } finally {
    loading.value = false;
  }
}

async function loadCustomChart() {
  if (!customRange.value?.length || customRange.value.length !== 2) {
    return;
  }
  const [start, end] = customRange.value;
  if (!start || !end) return;
  loading.value = true;
  try {
    const data = await fetchRange(new Date(start), new Date(end));
    lastSeriesCache.value.custom = data;
    applyChart(charts.custom, "自定义时间段流量", data);
  } catch (err) {
    fb.apiError(err, "加载自定义时间段流量失败");
  } finally {
    loading.value = false;
  }
}

function confirmCustomRange() {
  if (!customRangeDraft.value || customRangeDraft.value.length !== 2) {
    fb.warn("请先选择开始与结束时间");
    return;
  }
  customRange.value = [...customRangeDraft.value];
  loadCustomChart().then(() => {
    customChartAnchorRef.value?.scrollIntoView({ behavior: "smooth", block: "start" });
  });
}

function cancelCustomRange() {
  customRangeDraft.value = [...(customRange.value || [])];
}

function resizeCharts() {
  charts.today?.resize();
  charts.d7?.resize();
  charts.d30?.resize();
  charts.custom?.resize();
}

function applyThresholdToAllCharts() {
  applyChart(charts.today, "当日流量", lastSeriesCache.value.today || []);
  applyChart(charts.d7, "近7天流量", lastSeriesCache.value.d7 || []);
  applyChart(charts.d30, "近30天流量", lastSeriesCache.value.d30 || []);
  applyChart(charts.custom, "自定义时间段流量", lastSeriesCache.value.custom || []);
}

function switchChartCard(name) {
  chartCardActive.value = name;
  nextTick(() => resizeCharts());
}

async function loadPortMeta() {
  const fromQueryName = String(route.query.portName || "").trim();
  const fromQueryRemark = String(route.query.portRemark || "").trim();
  if (fromQueryName) {
    portMeta.value = { id: props.id, name: fromQueryName };
    portEdit.value.name = fromQueryName;
    portEdit.value.remark = fromQueryRemark;
    const base = String(route.query.portBaseName || fromQueryName || "").trim();
    portBaseName.value = base || fromQueryName;
    portSuffix.value = fromQueryName.startsWith(`${portBaseName.value} `)
      ? fromQueryName.slice((`${portBaseName.value} `).length)
      : "";
    return;
  }
  const hit = siblingPorts.value.find((x) => Number(x.id) === Number(props.id));
  if (hit) {
    portMeta.value = { id: props.id, name: hit.name };
    portEdit.value.name = hit.name;
    portEdit.value.remark = hit.remark || "";
  } else {
    portMeta.value = { id: props.id, name: `端口-${props.id}` };
    portEdit.value.name = "";
  }
}

function buildTerminalUrl() {
  const ip = String(route.query.deviceIp || "").trim();
  if (!ip) return "";
  const schemeTpl = {
    ssh: "ssh://{ip}",
    termius: "termius://host/{ip}",
    securecrt: "ssh2://{ip}",
    custom: localStorage.getItem("np_terminal_url_template") || "ssh://{ip}"
  };
  const tpl = schemeTpl[terminalType.value] || schemeTpl.custom;
  return String(tpl).replaceAll("{ip}", ip);
}

function openTerminal() {
  const url = buildTerminalUrl();
  if (!url) {
    fb.warn("缺少设备IP，无法打开终端");
    return;
  }
  window.open(url, "_blank", "noopener");
}

async function savePortProfile() {
  if (!editMode.value) return fb.warn("当前为只读模式，请先在左侧开启编辑模式");
  savingPort.value = true;
  try {
    const suffix = String(portSuffix.value || "").trim();
    const finalName = suffix ? `${portBaseName.value} ${suffix}` : `${portBaseName.value}`;
    await api.updateInterfaceProfile(props.id, {
      name: finalName,
      remark: portEdit.value.remark || ""
    });
    portMeta.value.name = finalName;
    portEdit.value.name = finalName;
    fb.success("端口名称/备注已保存");
  } catch (err) {
    fb.apiError(err, "保存端口信息失败");
  } finally {
    savingPort.value = false;
  }
}

async function restoreDefaultPortName() {
  if (!editMode.value) return fb.warn("当前为只读模式，请先在左侧开启编辑模式");
  savingPort.value = true;
  try {
    await api.updateInterfaceProfile(props.id, {
      name: "",
      remark: portEdit.value.remark || ""
    });
    portEdit.value.name = "";
    await loadPortMeta();
    fb.success("已恢复设备默认端口名称");
  } catch (err) {
    fb.apiError(err, "恢复默认名称失败");
  } finally {
    savingPort.value = false;
  }
}

async function copyTerminalTarget() {
  const ip = String(route.query.deviceIp || "").trim();
  if (!ip) return fb.warn("缺少设备IP");
  const cmd = `ssh ${ip}`;
  try {
    await navigator.clipboard.writeText(cmd);
    fb.success("已复制连接命令");
  } catch {
    fb.warn("复制失败，请手动复制");
  }
}

onMounted(async () => {
  await loadRuntimePollSec();
  await loadSiblingPorts();
  await loadPortMeta();
  await nextTick();
  const e = await import("echarts");
  charts.today = e.init(chartTodayRef.value);
  charts.d7 = e.init(chart7dRef.value);
  charts.d30 = e.init(chart30dRef.value);
  charts.custom = e.init(chartCustomRef.value);
  applyChart(charts.today, "当日流量", []);
  applyChart(charts.d7, "近7天流量", []);
  applyChart(charts.d30, "近30天流量", []);
  applyChart(charts.custom, "自定义时间段流量", []);
  customRangeDraft.value = [...(customRange.value || [])];
  await loadAllCharts();
  window.addEventListener("resize", resizeCharts);
  window.addEventListener("np-edit-mode", onEditModeEvent);
});

watch(() => props.id, async () => {
  await loadSiblingPorts();
  await loadPortMeta();
  await loadAllCharts();
});

onBeforeUnmount(() => {
  window.removeEventListener("resize", resizeCharts);
  window.removeEventListener("np-edit-mode", onEditModeEvent);
  charts.today?.dispose();
  charts.d7?.dispose();
  charts.d30?.dispose();
  charts.custom?.dispose();
});

function onEditModeEvent(e) {
  editMode.value = Boolean(e?.detail?.enabled);
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
      <div class="grid grid-cols-1 gap-3 xl:grid-cols-[1.3fr,1fr]">
        <div class="space-y-3">
          <div>
            <div class="text-xs text-slate-500">端口</div>
            <div class="text-lg font-semibold">{{ portMeta.name }}</div>
          </div>
          <div class="flex items-center gap-2">
            <el-button :disabled="!prevPort" @click="jumpSibling(prevPort)">上一端口</el-button>
            <el-button :disabled="!nextPort" @click="jumpSibling(nextPort)">下一端口</el-button>
          </div>
          <div class="flex flex-wrap items-center gap-2">
            <el-input :model-value="portBaseName" disabled class="w-[220px]" />
            <el-input v-model="portSuffix" placeholder="追加后缀（可空）" class="w-[220px]" />
            <el-input v-model="portEdit.remark" placeholder="端口备注" class="w-[220px]" />
            <el-button type="warning" plain :disabled="!editMode" @click="savePortProfile" :loading="savingPort">保存</el-button>
            <el-button plain :disabled="!editMode" @click="restoreDefaultPortName" :loading="savingPort">恢复默认名</el-button>
          </div>
          <div class="flex flex-wrap items-center gap-2">
            <el-input-number v-model="trafficThresholdBps" :min="0" :step="1000000" placeholder="告警阈值(bps)" />
            <el-button @click="applyThresholdToAllCharts">应用阈值线</el-button>
            <el-button @click="loadAllCharts" :loading="loading">{{ zhCN.portDetail.refresh }}</el-button>
          </div>
        </div>
        <div class="space-y-3">
          <div class="flex flex-wrap items-center gap-2">
            <span class="text-xs text-slate-500">终端跳转模板</span>
            <el-select v-model="terminalType" class="w-[180px]">
              <el-option label="系统默认 SSH" value="ssh" />
              <el-option label="Termius" value="termius" />
              <el-option label="SecureCRT" value="securecrt" />
              <el-option label="自定义模板" value="custom" />
            </el-select>
            <el-button type="primary" @click="openTerminal">连接设备终端</el-button>
            <el-button @click="copyTerminalTarget">复制 SSH</el-button>
          </div>
          <div class="flex flex-wrap items-center gap-2">
            <el-date-picker
              v-model="customRangeDraft"
              type="datetimerange"
              unlink-panels
              range-separator="至"
              start-placeholder="开始时间"
              end-placeholder="结束时间"
              :shortcuts="pickerShortcuts"
            />
            <el-button type="primary" @click="confirmCustomRange" :loading="loading">查询自定义区间</el-button>
            <el-button @click="cancelCustomRange">取消</el-button>
          </div>
        </div>
      </div>
    </el-card>

    <el-card>
      <template #header>
        <div class="flex flex-wrap items-center justify-between gap-2">
          <el-segmented
            :model-value="chartCardActive"
            :options="[
              { label: '当日流量', value: 'today' },
              { label: '近7天', value: 'd7' },
              { label: '近30天', value: 'd30' },
              { label: '自定义', value: 'custom' }
            ]"
            @change="switchChartCard"
          />
          <div class="flex items-center gap-2">
            <el-button size="small" @click="saveChartPNG(chartCardActive)">导出PNG</el-button>
            <el-button size="small" @click="exportChartCSV(chartCardActive, chartCardActive)">导出CSV</el-button>
          </div>
        </div>
      </template>

      <div v-show="chartCardActive === 'today'" ref="chartTodayRef" class="h-[360px] w-full" v-loading="loading"></div>
      <div v-show="chartCardActive === 'd7'" ref="chart7dRef" class="h-[360px] w-full" v-loading="loading"></div>
      <div v-show="chartCardActive === 'd30'" ref="chart30dRef" class="h-[360px] w-full" v-loading="loading"></div>
      <div ref="customChartAnchorRef"></div>
      <div v-show="chartCardActive === 'custom'" ref="chartCustomRef" class="h-[360px] w-full" v-loading="loading"></div>
    </el-card>
  </div>
</template>
