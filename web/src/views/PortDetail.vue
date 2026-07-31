<script setup>
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from "vue";
import { useRoute, useRouter } from "vue-router";
import { api } from "../services/api";
import { formatBps } from "../utils/format";
import { getApiError } from "../utils/apiError";
import { zhCN } from "../i18n/zhCN";
import { useFeedback } from "../composables/useFeedback";
import { npAxisLabel, npAxisLine, npAxisTick, npChartGrid, npChartPalette, npEmptyGraphic, npLegend, npSplitLine, npTooltip } from "../utils/chartTheme";
import {
  formatServerTime,
  startOfServerMonth,
  startOfServerWeek,
  startOfServerYear,
  datePickerValueToServerDate
} from "../utils/serverTime";
import {
  MAX_TRAFFIC_HISTORY_MS,
  calcTrafficFetchPlan,
  decimatePoints,
  detectIntervalSwitchPoints,
  getPresetTrafficRange,
  pickTrafficUnit,
  roundUpNice,
  stabilizeTrafficPoints,
  toTrafficSeriesData,
  xAxisLabelFormatter
} from "../utils/portTraffic";

const props = defineProps({ id: { type: [String, Number], required: true } });
const route = useRoute();
const router = useRouter();
const fb = useFeedback();
const editMode = ref(localStorage.getItem("np_edit_mode") === "1");

const loading = ref(false);
const chartLoadError = ref("");
const trendBackfill = ref({ state: "not_applicable", progress_percent: 0 });
const retryingTrendBackfill = ref(false);
const customRange = ref([]);
const customStartDraft = ref(null);
const customEndDraft = ref(null);
const chartTodayRef = ref(null);
const chart7dRef = ref(null);
const chart30dRef = ref(null);
const chartCustomRef = ref(null);
const portMeta = ref({ id: props.id, name: route.query.portName || `端口-${props.id}` });
const portEdit = ref({ name: route.query.portName || "", remark: route.query.portRemark || "" });
const portBaseName = ref(String(route.query.portBaseName || route.query.portName || `端口-${props.id}`));
const portSuffix = ref("");
const savingPort = ref(false);
const terminalType = ref("telnet");
const customChartAnchorRef = ref(null);
const trafficThresholdBps = ref(0);
const chartCardActive = ref("today");
const siblingPorts = ref([]);
const currentPortSpeedMbps = ref(0);
const showRawSeries = ref(false);
const lastSeriesCache = ref({
  today: [],
  d7: [],
  d30: [],
  custom: []
});
const chartLoaded = ref({
  today: false,
  d7: false,
  d30: false,
  custom: false
});
const chartMeta = ref({
  today: { interval: "-", agg: "-" },
  d7: { interval: "-", agg: "-" },
  d30: { interval: "-", agg: "-" },
  custom: { interval: "-", agg: "-" }
});
const chartSource = ref({
  today: "metrics",
  d7: "metrics",
  d30: "metrics_1m",
  custom: "metrics"
});
const runtimePollSec = ref(60);
let charts = { today: null, d7: null, d30: null, custom: null };
let fullChartRequestSeq = 0;
let customChartRequestSeq = 0;

const pickerShortcuts = [
  { text: "本周", value: () => [startOfServerWeek(new Date()), new Date()] },
  {
    text: "上周",
    value: () => {
      const thisWeek = startOfServerWeek(new Date());
      const lastWeekStart = new Date(thisWeek);
      lastWeekStart.setDate(lastWeekStart.getDate() - 7);
      return [lastWeekStart, new Date(thisWeek.getTime() - 1000)];
    }
  },
  { text: "本月", value: () => [startOfServerMonth(new Date()), new Date()] },
  {
    text: "上月",
    value: () => {
      const thisMonth = startOfServerMonth(new Date());
      const lastMonthAnchor = new Date(thisMonth.getTime() - 1000);
      return [startOfServerMonth(lastMonthAnchor), new Date(thisMonth.getTime() - 1000)];
    }
  },
  { text: "本年", value: () => [startOfServerYear(new Date()), new Date()] },
  {
    text: "上年",
    value: () => {
      const thisYear = startOfServerYear(new Date());
      const lastYearAnchor = new Date(thisYear.getTime() - 1000);
      return [startOfServerYear(lastYearAnchor), new Date(thisYear.getTime() - 1000)];
    }
  }
];

function bpsLabel(v) {
  return formatBps(v);
}

function baseOption(title, unitInfo, planText = "") {
  return {
    animation: false,
    grid: { ...npChartGrid, top: 90, bottom: 92 },
    title: {
      text: title,
      subtext: `单位: ${unitInfo.unit}${planText ? `\n${planText}` : ""}`,
      left: 10,
      top: 10,
      textStyle: { fontSize: 14, fontWeight: 600 },
      subtextStyle: { fontSize: 12, color: "#64748b", lineHeight: 17 }
    },
    tooltip: npTooltip({
      axisPointer: { type: "line", animation: false },
      formatter(params) {
        if (!params?.length) return "";
        const ts = formatServerTime(new Date(params[0].data[0]), {
          year: "numeric",
          month: "2-digit",
          day: "2-digit",
          hour: "2-digit",
          minute: "2-digit",
          second: "2-digit"
        });
        const lines = [ts];
        for (const p of params) {
          const value = Array.isArray(p.data) ? p.data[1] : null;
          lines.push(`${p.marker}${p.seriesName}: ${value == null ? "无数据" : bpsLabel(value)}`);
        }
        lines.push(`<span style="color:#94a3b8">展示模式: ${showRawSeries.value ? "原始折线" : "稳健平滑"}</span>`);
        lines.push(`<span style="color:#94a3b8">说明: 高速端口若检测到缓存采样相位，采集端已按有效采样入库</span>`);
        return lines.join("<br/>");
      }
    }),
    legend: { ...npLegend, top: 14, right: 10, data: ["入方向", "出方向"] },
    dataZoom: [
      { type: "inside", throttle: 60, zoomOnMouseWheel: true, moveOnMouseMove: true },
      { type: "slider", height: 18, bottom: 0 }
    ],
    xAxis: {
      type: "time",
      axisLabel: {
        ...npAxisLabel,
        hideOverlap: true,
        rotate: 45,
        formatter: (value) => {
          const hhmm = formatServerTime(new Date(value), { hour: "2-digit", minute: "2-digit" });
          if (hhmm === "00:00") {
            return formatServerTime(new Date(value), { month: "2-digit", day: "2-digit", hour: "2-digit", minute: "2-digit" });
          }
          return hhmm;
        }
      },
      axisLine: npAxisLine,
      axisTick: npAxisTick
    },
    yAxis: {
      type: "value",
      min: 0,
      max: 100,
      splitNumber: 6,
      axisLabel: { ...npAxisLabel, formatter: (val) => `${(val / unitInfo.div).toFixed(2)}` },
      axisLine: npAxisLine,
      axisTick: npAxisTick,
      splitLine: npSplitLine
    },
    series: [
      {
        name: "入方向",
        type: "line",
        showSymbol: false,
        smooth: !showRawSeries.value,
        step: false,
        connectNulls: false,
        sampling: "lttb",
        progressive: 5000,
        lineStyle: { color: npChartPalette.inbound, width: 2 },
        itemStyle: { color: npChartPalette.inbound },
        data: [],
        markLine: trafficThresholdBps.value > 0 ? {
          symbol: "none",
          label: { show: true, formatter: `阈值 ${formatBps(trafficThresholdBps.value)}` },
          lineStyle: { color: npChartPalette.danger, type: "dashed" },
          data: [{ yAxis: trafficThresholdBps.value }]
        } : undefined
      },
      {
        name: "出方向",
        type: "line",
        showSymbol: false,
        smooth: !showRawSeries.value,
        step: false,
        connectNulls: false,
        sampling: "lttb",
        progressive: 5000,
        lineStyle: { color: npChartPalette.outbound, width: 2 },
        itemStyle: { color: npChartPalette.outbound },
        data: [],
        markLine: trafficThresholdBps.value > 0 ? {
          symbol: "none",
          label: { show: false },
          lineStyle: { color: npChartPalette.danger, type: "dashed" },
          data: [{ yAxis: trafficThresholdBps.value }]
        } : undefined
      }
    ]
  };
}

function calcFetchPlan(start, end) {
  return calcTrafficFetchPlan(start, end, currentPortSpeedMbps.value, runtimePollSec.value);
}

async function fetchRange(start, end) {
  const plan = calcFetchPlan(start, end);
  const spanMs = end.getTime() - start.getTime();
  if (spanMs > MAX_TRAFFIC_HISTORY_MS) {
    throw new Error("流量历史最长仅支持查询近2年");
  }
  const maxPoints = spanMs > 30 * 24 * 3600 * 1000 ? 1000 : (spanMs > 24 * 3600 * 1000 ? 1200 : 2500);
  const res = await api.getHistory("traffic", props.id, start.toISOString(), end.toISOString(), plan.interval, maxPoints);
  return {
    data: res.data.data || [],
    plan,
    source: String(res?.data?.source_table || ""),
    sampledInterval: String(res?.data?.sampled_interval || ""),
    trendBackfill: res?.data?.trend_backfill || { state: "not_applicable", progress_percent: 0 }
  };
}

async function loadRuntimePollSec() {
  const deviceID = Number(route.query.deviceId || 0);
  try {
    const runtimeRes = await api.getRuntimeSettings();
    const runtime = runtimeRes?.data || {};
    const core = Math.max(5, Number(runtime?.poll_interval_core_sec || 60));
    const agg = Math.max(5, Number(runtime?.poll_interval_agg_sec || 90));
    const access = Math.max(5, Number(runtime?.poll_interval_access_sec || 120));
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
    runtimePollSec.value = access;
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
      rawName: x.raw_name || x.name || `ifIndex-${x.index}`,
      remark: x.remark || "",
      speedMbps: Number(x.speed_mbps || 0)
    }));
  } catch {
    siblingPorts.value = [];
  }
}

async function resolvePortContext() {
  const hasName = String(route.query.portName || "").trim() !== "";
  const hasDevice = Number(route.query.deviceId || 0) > 0 && String(route.query.deviceIp || "").trim() !== "";
  const hasSpeed = Number(route.query.speedMbps || 0) > 0;
  if (hasName && hasDevice && hasSpeed) return null;
  try {
    const getter = typeof api.getInterfaceById === "function" ? api.getInterfaceById : api.getInterface;
    if (typeof getter !== "function") {
      throw new Error("端口详情接口未注册，请刷新页面后重试");
    }
    const res = await getter(props.id);
    const itf = res?.data || null;
    if (!itf) return null;
    const nextQuery = {
      ...route.query,
      deviceId: String(route.query.deviceId || itf.device_id || ""),
      deviceIp: String(route.query.deviceIp || itf.device_ip || ""),
      portName: String(route.query.portName || itf.name || `ifIndex-${itf.index}`),
      portBaseName: String(route.query.portBaseName || itf.raw_name || itf.name || `ifIndex-${itf.index}`),
      portRemark: String(route.query.portRemark || itf.remark || ""),
      speedMbps: String(route.query.speedMbps || itf.speed_mbps || 0)
    };
    const changed = Object.keys(nextQuery).some((k) => String(nextQuery[k] ?? "") !== String(route.query[k] ?? ""));
    if (changed) {
      await router.replace({ path: route.path, query: nextQuery });
    }
    return itf;
  } catch (err) {
    fb.apiError(err, "加载端口上下文失败");
    return null;
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
  const q = {
    ...route.query,
    portName: port.name,
    portBaseName: port.rawName || port.name,
    portRemark: port.remark || "",
    speedMbps: String(port.speedMbps || 0)
  };
  router.push({ path: `/port/${port.id}`, query: q });
}

function applyChart(chart, title, data, metaKey = "today") {
  if (!chart) return;
  const { inbound, outbound } = toTrafficSeriesData(data);
  const intervalSwitch = detectIntervalSwitchPoints(data);
  const hasIntervalSwitch = intervalSwitch.length > 0;
  const smoothEnabled = !showRawSeries.value;
  const stableDisplay = !showRawSeries.value;
  const inPrepared = stabilizeTrafficPoints(inbound, stableDisplay);
  const outPrepared = stabilizeTrafficPoints(outbound, stableDisplay);
  const inView = decimatePoints(inPrepared);
  const outView = decimatePoints(outPrepared);
  const hasData = inView.some((x) => x[1] != null) || outView.some((x) => x[1] != null);
  const nonNil = [
    ...inView.map((x) => x[1]).filter((v) => v != null),
    ...outView.map((x) => x[1]).filter((v) => v != null)
  ];
  const maxVal = Math.max(1, ...(nonNil.length ? nonNil : [1]));
  const unitInfo = pickTrafficUnit(maxVal);
  const meta = chartMeta.value[metaKey] || { interval: "-", agg: "-" };
  const src = chartSource.value[metaKey] || "metrics";
  const displayMode = showRawSeries.value ? "原始折线" : "稳健平滑显示";
  const planText = `采样: ${meta.interval || "原始"} · ${meta.agg || "-"} · 源: ${src} · 展示: ${displayMode}`;
  const opt = baseOption(title, unitInfo, planText);
  opt.xAxis.axisLabel.formatter = (value) => xAxisLabelFormatter(value, metaKey);
  opt.xAxis.minInterval = metaKey === "d30" ? 24 * 3600 * 1000 : undefined;
  // Give title/subtitle and dataZoom enough room to avoid clipping or overlap.
  opt.grid.top = 90;
  opt.grid.bottom = 92;
  if (Array.isArray(opt.dataZoom)) {
    opt.dataZoom[0].bottom = 30;
    opt.dataZoom[1].bottom = 30;
  }
  if (hasIntervalSwitch && !showRawSeries.value) {
    opt.title.subtext = `${opt.title.subtext} · 已自动消隐采样相位抖动`;
  }
  opt.yAxis.max = roundUpNice(maxVal * 1.1);
  opt.tooltip.confine = true;
  opt.tooltip.transitionDuration = 0;
  opt.series[0].large = true;
  opt.series[1].large = true;
  opt.series[0].smooth = smoothEnabled;
  opt.series[1].smooth = smoothEnabled;
  opt.series[0].sampling = showRawSeries.value ? "lttb" : "average";
  opt.series[1].sampling = showRawSeries.value ? "lttb" : "average";
  opt.series[0].largeThreshold = 2000;
  opt.series[1].largeThreshold = 2000;
  opt.series[0].data = inView;
  opt.series[1].data = outView;
  const gapAreas = [];
  if (gapAreas.length) {
    opt.series[0].markArea = {
      silent: true,
      itemStyle: { color: "rgba(148,163,184,0.10)" },
      data: gapAreas
    };
  }
  const speedBps = Number(currentPortSpeedMbps.value || 0) * 1_000_000;
  const refLines = [];
  if (speedBps > 0) {
    refLines.push({ yAxis: speedBps, label: { formatter: "100%速率线" }, lineStyle: { color: npChartPalette.cpu, type: "dashed" } });
    refLines.push({ yAxis: speedBps * 0.8, label: { formatter: "80%速率线" }, lineStyle: { color: npChartPalette.warning, type: "dashed" } });
  }
  if (trafficThresholdBps.value > 0) {
    refLines.push({ yAxis: trafficThresholdBps.value, label: { formatter: `阈值 ${formatBps(trafficThresholdBps.value)}` }, lineStyle: { color: npChartPalette.danger, type: "dashed" } });
  }
  if (refLines.length) {
    opt.series[0].markLine = { symbol: "none", data: refLines };
  }
  if (!hasData) {
    opt.graphic = [npEmptyGraphic("当前时间范围暂无流量数据")];
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

async function loadPresetChart(key, options = {}) {
  const range = getPresetTrafficRange(key);
  if (!range) return;
  if (chartLoaded.value[key] && !options.force) {
    await nextTick();
    charts[key]?.resize();
    return;
  }
  const seq = ++fullChartRequestSeq;
  loading.value = true;
  try {
    chartLoadError.value = "";
    const titles = { today: "当日流量", d7: "近7天流量", d30: "近30天流量" };
    const res = await fetchRange(range[0], range[1]);
    if (seq !== fullChartRequestSeq) return;
    lastSeriesCache.value[key] = res.data;
    chartMeta.value[key] = res.plan;
    chartSource.value[key] = res.source || "metrics";
    trendBackfill.value = res.trendBackfill;
    chartLoaded.value[key] = true;
    await nextTick();
    applyChart(charts[key], titles[key], res.data, key);
    charts[key]?.resize();
  } catch (err) {
    if (seq === fullChartRequestSeq) {
      chartLoadError.value = getApiError(err, "端口流量加载失败");
      fb.apiError(err, "加载端口流量失败");
    }
  } finally {
    if (seq === fullChartRequestSeq) {
      loading.value = false;
    }
  }
}

async function loadAllCharts() {
  if (chartCardActive.value === "custom") {
    await loadCustomChart();
    return;
  }
  await loadPresetChart(chartCardActive.value || "today", { force: true });
}

async function loadCustomChart(options = {}) {
  if (!customRange.value?.length || customRange.value.length !== 2) {
    return;
  }
  const seq = ++customChartRequestSeq;
  const [start, end] = customRange.value;
  if (!start || !end) return;
  if (!options.keepLoading) loading.value = true;
  try {
    chartLoadError.value = "";
    const res = await fetchRange(new Date(start), new Date(end));
    if (seq !== customChartRequestSeq) return;
    lastSeriesCache.value.custom = res.data;
    chartMeta.value.custom = res.plan;
    chartSource.value.custom = res.source || "metrics";
    trendBackfill.value = res.trendBackfill;
    chartLoaded.value.custom = true;
    chartCardActive.value = "custom";
    await nextTick();
    applyChart(charts.custom, "自定义时间段流量", res.data, "custom");
    charts.custom?.resize();
  } catch (err) {
    if (seq === customChartRequestSeq) {
      chartLoadError.value = getApiError(err, "自定义时间段流量加载失败");
      fb.apiError(err, "加载自定义时间段流量失败");
    }
  } finally {
    if (seq === customChartRequestSeq && !options.keepLoading) {
      loading.value = false;
    }
  }
}

async function retryTrendBackfill() {
  retryingTrendBackfill.value = true;
  try {
    await api.retryTrafficTrendBackfill(props.id);
    trendBackfill.value = { state: "backfilling", progress_percent: trendBackfill.value?.progress_percent || 0 };
    fb.success("历史趋势回填已重新排队，图表会自动使用已完成的部分");
    await loadAllCharts();
  } catch (err) {
    fb.apiError(err, "重新排队历史趋势回填失败");
  } finally {
    retryingTrendBackfill.value = false;
  }
}

function confirmCustomRange() {
  if (!customStartDraft.value || !customEndDraft.value) {
    fb.warn("请先选择开始与结束时间");
    return;
  }
  const start = datePickerValueToServerDate(customStartDraft.value);
  const end = datePickerValueToServerDate(customEndDraft.value);
  if (!start || !end || !Number.isFinite(start.getTime()) || !Number.isFinite(end.getTime()) || end <= start) {
    fb.warn("结束时间必须晚于开始时间");
    return;
  }
  customRange.value = [start, end];
  loadCustomChart().then(() => {
    customChartAnchorRef.value?.scrollIntoView({ behavior: "smooth", block: "start" });
  });
}

function cancelCustomRange() {
  customStartDraft.value = customRange.value?.[0] || null;
  customEndDraft.value = customRange.value?.[1] || null;
}

function resizeCharts() {
  charts.today?.resize();
  charts.d7?.resize();
  charts.d30?.resize();
  charts.custom?.resize();
}

function applyThresholdToAllCharts() {
  applyChart(charts.today, "当日流量", lastSeriesCache.value.today || [], "today");
  applyChart(charts.d7, "近7天流量", lastSeriesCache.value.d7 || [], "d7");
  applyChart(charts.d30, "近30天流量", lastSeriesCache.value.d30 || [], "d30");
  applyChart(charts.custom, "自定义时间段流量", lastSeriesCache.value.custom || [], "custom");
}

function resetChartCaches() {
  lastSeriesCache.value = { today: [], d7: [], d30: [], custom: [] };
  chartLoaded.value = { today: false, d7: false, d30: false, custom: false };
  chartMeta.value = {
    today: { interval: "-", agg: "-" },
    d7: { interval: "-", agg: "-" },
    d30: { interval: "-", agg: "-" },
    custom: { interval: "-", agg: "-" }
  };
  chartSource.value = { today: "metrics", d7: "metrics", d30: "metrics", custom: "metrics" };
  applyChart(charts.today, "当日流量", [], "today");
  applyChart(charts.d7, "近7天流量", [], "d7");
  applyChart(charts.d30, "近30天流量", [], "d30");
  applyChart(charts.custom, "自定义时间段流量", [], "custom");
}

function switchChartCard(name) {
  chartCardActive.value = name;
  nextTick(async () => {
    resizeCharts();
    if (name !== "custom") {
      await loadPresetChart(name);
    }
  });
}

async function retryActiveChart() {
  if (chartCardActive.value === "custom") {
    await loadCustomChart();
    return;
  }
  await loadPresetChart(chartCardActive.value || "today", { force: true });
}

async function loadPortMeta() {
  const resolved = await resolvePortContext();
  const fromQueryName = String(route.query.portName || "").trim();
  const fromQueryRemark = String(route.query.portRemark || "").trim();
  const fromQuerySpeed = Number(route.query.speedMbps || 0);
  if (fromQuerySpeed > 0) {
    currentPortSpeedMbps.value = fromQuerySpeed;
  }
  const hit = siblingPorts.value.find((x) => Number(x.id) === Number(props.id));
  if (hit) {
    currentPortSpeedMbps.value = Number(hit.speedMbps || 0);
  } else if (resolved) {
    currentPortSpeedMbps.value = Number(resolved.speed_mbps || 0);
  }
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
  if (hit) {
    portMeta.value = { id: props.id, name: hit.name };
    portEdit.value.name = hit.name;
    portEdit.value.remark = hit.remark || "";
  } else if (resolved) {
    const name = resolved.name || `ifIndex-${resolved.index}`;
    const base = resolved.raw_name || name;
    portMeta.value = { id: props.id, name };
    portEdit.value.name = name;
    portEdit.value.remark = resolved.remark || "";
    portBaseName.value = base;
    portSuffix.value = name.startsWith(`${base} `) ? name.slice((`${base} `).length) : "";
  } else {
    portMeta.value = { id: props.id, name: `端口-${props.id}` };
    portEdit.value.name = "";
    currentPortSpeedMbps.value = 0;
  }
}

function buildTerminalUrl() {
  const ip = String(route.query.deviceIp || "").trim();
  if (!ip) return "";
  const protocol = terminalType.value === "telnet" ? "telnet" : "ssh";
  return `${protocol}://${ip}`;
}

function openTerminal() {
  const url = buildTerminalUrl();
  if (!url) {
    fb.warn("缺少设备IP，无法打开终端");
    return;
  }
  const a = document.createElement("a");
  a.href = url;
  a.rel = "noopener";
  a.style.display = "none";
  document.body.appendChild(a);
  a.click();
  a.remove();
  fb.success(`已调用本地 ${terminalType.value === "telnet" ? "Telnet" : "SSH"} 连接`);
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
  const cmd = terminalType.value === "telnet" ? `telnet ${ip}` : `ssh ${ip}`;
  try {
    await navigator.clipboard.writeText(cmd);
    fb.success("已复制连接命令");
  } catch {
    fb.warn("复制失败，请手动复制");
  }
}

onMounted(async () => {
  await loadPortMeta();
  await loadRuntimePollSec();
  await loadSiblingPorts();
  await loadPortMeta();
  await nextTick();
  const e = await import("echarts");
  charts.today = e.init(chartTodayRef.value);
  charts.d7 = e.init(chart7dRef.value);
  charts.d30 = e.init(chart30dRef.value);
  charts.custom = e.init(chartCustomRef.value);
  applyChart(charts.today, "当日流量", [], "today");
  applyChart(charts.d7, "近7天流量", [], "d7");
  applyChart(charts.d30, "近30天流量", [], "d30");
  applyChart(charts.custom, "自定义时间段流量", [], "custom");
  customStartDraft.value = customRange.value?.[0] || null;
  customEndDraft.value = customRange.value?.[1] || null;
  await loadAllCharts();
  window.addEventListener("resize", resizeCharts);
  window.addEventListener("np-edit-mode", onEditModeEvent);
});

watch(() => props.id, async () => {
  fullChartRequestSeq += 1;
  customChartRequestSeq += 1;
  resetChartCaches();
  chartCardActive.value = "today";
  await loadPortMeta();
  await loadRuntimePollSec();
  await loadSiblingPorts();
  await loadPortMeta();
  await loadAllCharts();
});

onBeforeUnmount(() => {
  fullChartRequestSeq += 1;
  customChartRequestSeq += 1;
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
  <div class="np-view-shell np-port-detail space-y-5">
    <el-breadcrumb separator=">">
      <el-breadcrumb-item :to="{ path: '/' }">资产</el-breadcrumb-item>
      <el-breadcrumb-item :to="{ path: `/device/${route.query.deviceId || ''}` }">{{ route.query.deviceIp || '设备' }}</el-breadcrumb-item>
      <el-breadcrumb-item>{{ portMeta.name }}</el-breadcrumb-item>
    </el-breadcrumb>

    <el-card class="np-detail-hero-card">
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
            <el-switch v-model="showRawSeries" inline-prompt active-text="原始显示" inactive-text="平滑显示" @change="applyThresholdToAllCharts" />
            <el-button @click="loadAllCharts" :loading="loading">{{ zhCN.portDetail.refresh }}</el-button>
          </div>
        </div>
        <div class="space-y-3">
          <div class="flex flex-wrap items-center gap-2">
            <span class="text-xs text-slate-500">连接方式</span>
            <el-select v-model="terminalType" class="w-[180px]">
              <el-option label="Telnet（本地终端）" value="telnet" />
              <el-option label="SSH（本地终端）" value="ssh" />
            </el-select>
            <el-button type="primary" @click="openTerminal">连接设备终端</el-button>
            <el-button @click="copyTerminalTarget">复制连接命令</el-button>
          </div>
          <div class="flex flex-wrap items-center gap-2">
            <el-date-picker
              v-model="customStartDraft"
              type="datetime"
              placeholder="开始时间"
              :shortcuts="pickerShortcuts.map((x) => ({ text: x.text + '开始', value: () => x.value()[0] }))"
              class="w-[220px]"
            />
            <span class="text-xs text-slate-500">至</span>
            <el-date-picker
              v-model="customEndDraft"
              type="datetime"
              placeholder="结束时间"
              :shortcuts="pickerShortcuts.map((x) => ({ text: x.text + '结束', value: () => x.value()[1] }))"
              class="w-[220px]"
            />
            <el-button type="primary" @click="confirmCustomRange" :loading="loading">查询自定义区间</el-button>
            <el-button @click="cancelCustomRange">取消</el-button>
          </div>
        </div>
      </div>
    </el-card>

    <el-card class="np-chart-card np-traffic-card">
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

      <el-alert
        v-if="trendBackfill.state === 'backfilling' || trendBackfill.state === 'failed'"
        class="mb-3"
        :type="trendBackfill.state === 'failed' ? 'warning' : 'info'"
        show-icon
        :closable="false"
        :title="trendBackfill.state === 'failed' ? '历史趋势回填暂时停滞' : '正在补齐该端口的历史趋势'"
      >
        <template #default>
          <div class="flex flex-wrap items-center gap-2">
            <span v-if="trendBackfill.state === 'backfilling'">已完成约 {{ trendBackfill.progress_percent || 0 }}%，期间图表仅显示已归档的历史数据。</span>
            <span v-else>{{ trendBackfill.last_error || '回填任务可安全重新排队。' }}</span>
            <el-button v-if="trendBackfill.state === 'failed'" size="small" :loading="retryingTrendBackfill" @click="retryTrendBackfill">重新排队</el-button>
          </div>
        </template>
      </el-alert>

      <el-alert
        v-if="chartLoadError"
        class="mb-3"
        type="warning"
        show-icon
        :closable="false"
        title="端口流量图暂时加载失败"
      >
        <template #default>
          <div class="flex flex-wrap items-center gap-2">
            <span>{{ chartLoadError }}</span>
            <el-button size="small" @click="retryActiveChart">重试加载图表</el-button>
          </div>
        </template>
      </el-alert>

      <div v-show="chartCardActive === 'today'" ref="chartTodayRef" class="h-[360px] w-full" v-loading="loading"></div>
      <div v-show="chartCardActive === 'd7'" ref="chart7dRef" class="h-[360px] w-full" v-loading="loading"></div>
      <div v-show="chartCardActive === 'd30'" ref="chart30dRef" class="h-[360px] w-full" v-loading="loading"></div>
      <div ref="customChartAnchorRef"></div>
      <div v-show="chartCardActive === 'custom'" ref="chartCustomRef" class="h-[360px] w-full" v-loading="loading"></div>
    </el-card>
  </div>
</template>
