<script setup>
import { computed, nextTick, onActivated, onBeforeUnmount, onMounted, ref, watch } from "vue";
import { useRouter } from "vue-router";
import { api } from "../services/api";
import { useOpsStore } from "../stores/ops";
import { formatBps } from "../utils/format";
import { normalizeStatus, statusClass, statusLabel } from "../utils/status";
import { useFeedback } from "../composables/useFeedback";
import { sortAssets } from "../utils/sortAssets";
import StatsCards from "../components/dashboard/StatsCards.vue";
import LiveEventFeed from "../components/dashboard/LiveEventFeed.vue";
import HealthTrendArea from "../components/dashboard/HealthTrendArea.vue";
import TrafficTopBar from "../components/dashboard/TrafficTopBar.vue";
import TopologyCanvas from "../components/topology/TopologyCanvas.vue";

const ops = useOpsStore();
const router = useRouter();
const fb = useFeedback();

const loading = ref(false);
const feedLoading = ref(false);
const devices = ref([]);
const onboardingReady = ref(false);
const globalKeyword = ref("");
const activeDashboardModule = ref("events");
let timer = null;
let refreshInFlight = false;
const healthTrend = ref([]);
const healthTrendError = ref("");
const deviceLoadError = ref("");
const feedLoadError = ref("");
const lastRefreshedAt = ref("");
const healthExplainVisible = ref(false);
const eventDetailVisible = ref(false);
const eventDetail = ref(null);
const eventPage = ref(1);
const eventPageSize = ref(20);
const eventFilterKeyword = ref("");
const eventFilterType = ref("all");
const eventFilterLevel = ref("all");
const eventFilterStart = ref("");
const eventFilterEnd = ref("");
const showEventFilters = ref(false);
const statusQuickFilter = ref("all");
const healthRef = ref(null);
const topNRef = ref(null);
const topologyRef = ref(null);
const chartRefreshKey = ref(0);
const topTab = ref("100m");
const topologyGraph = ref({ nodes: [], edges: [] });
const topologyLoading = ref(false);
const topologyError = ref("");
const topologyTooltipFontSize = ref(18);
const topologyLabelDisplayMode = ref("hover");
const topologyLabelDisplayTiers = ref(["core"]);
const topologyWheelZoomEnabled = ref(false);
const todoActions = computed(() => {
  const out = [];
  if (devices.value.length === 0) out.push({ key: "add", title: "添加首台资产", action: () => router.push("/assets") });
  if (activeAlerts.value > 0) out.push({ key: "alert", title: `处理 ${activeAlerts.value} 条活动告警`, action: () => router.push({ path: "/alerts", query: { tab: "alerts", status: "open" } }) });
  if (activeTopHotspots.value.length === 0) out.push({ key: "traffic", title: "等待流量采样，检查采集设置", action: () => router.push("/settings") });
  return out.slice(0, 3);
});

const onlineCount = computed(() => devices.value.filter((d) => {
  return normalizeStatus(d.status) === "online";
}).length);
const availability = computed(() => devices.value.length ? Math.round((onlineCount.value / devices.value.length) * 100) : 0);
const alertBreakdown = computed(() => {
  const all = ops.realtimeAlerts || [];
  return {
    critical: all.filter((x) => x.severity === "critical").length,
    warning: all.filter((x) => x.severity === "warning").length,
    info: all.filter((x) => x.severity === "info").length
  };
});
const activeAlerts = computed(() => alertBreakdown.value.critical + alertBreakdown.value.warning);
const healthScore = computed(() => {
  const penalty = Math.min(35, alertBreakdown.value.critical * 6 + alertBreakdown.value.warning * 2);
  return Math.max(0, Math.min(100, availability.value - penalty));
});

const top100M = computed(() => rankedPortsBySpeed(95, 1000));
const top1G = computed(() => rankedPortsBySpeed(1000, 10000));
const top10G = computed(() => rankedPortsBySpeed(10000, 0));

const filteredAlerts = computed(() => {
  const kw = eventFilterKeyword.value.trim().toLowerCase();
  const type = eventFilterType.value;
  const level = eventFilterLevel.value;
  const start = eventFilterStart.value ? new Date(eventFilterStart.value).getTime() : 0;
  const end = eventFilterEnd.value ? new Date(eventFilterEnd.value).getTime() : 0;
  return (ops.realtimeAlerts || []).filter((item) => {
    if (type !== "all" && String(item.type || item.event_type || "") !== type) return false;
    if (level !== "all") {
      const sev = String(item.severity || "").toLowerCase();
      const rawLevel = String(item.level || "").toLowerCase();
      if (sev !== level && rawLevel !== level) return false;
    }
    const ts = new Date(item.timestamp || item.created_at || 0).getTime() || 0;
    if (start && ts < start) return false;
    if (end && ts > end) return false;
    if (!kw) return true;
    return [
      item.device_name,
      item.device_ip,
      item.interface_name,
      item.interface_raw_name,
      item.interface_remark,
      item.message,
      item.code,
      item.type,
      item.level
    ].join(" ").toLowerCase().includes(kw);
  });
});

const pagedAlerts = computed(() => {
  const list = filteredAlerts.value.slice(0, 100);
  const start = (eventPage.value - 1) * eventPageSize.value;
  return list.slice(start, start + eventPageSize.value);
});

const filteredDevices = computed(() => {
  const kw = globalKeyword.value.trim().toLowerCase();
  let list = devices.value;
  if (statusQuickFilter.value !== "all") {
    list = list.filter((d) => normalizeStatus(d.status) === statusQuickFilter.value);
  }
  if (!kw) return list;
  return list.filter((d) => {
    const ports = (d.interfaces || [])
      .map((p) => `${p.name || ""} ${p.alias || ""} ${p.custom_name || ""} ${p.remark || ""} ${p.index || ""}`)
      .join(" ");
    return [d.ip, d.name, d.brand, d.remark, d.location, d.site, ports, d.status].join(" ").toLowerCase().includes(kw);
  });
});
const filteredPorts = computed(() => {
  const kw = globalKeyword.value.trim().toLowerCase();
  if (!kw) return [];
  const out = [];
  for (const d of devices.value) {
    for (const p of d.interfaces || []) {
      const text = [
        p.name || "",
        p.raw_name || "",
        p.remark || "",
        p.index || "",
        p.id || "",
        d.name || "",
        d.ip || "",
        d.brand || ""
      ].join(" ").toLowerCase();
      if (!text.includes(kw)) continue;
      out.push({
        portId: p.id,
        portName: p.name || `ifIndex-${p.index}`,
        portIndex: p.index,
        deviceId: d.id,
        deviceName: d.name || d.ip,
        deviceIP: d.ip,
        remark: p.remark || "",
        speedMbps: Number(p.speed_mbps || 0)
      });
    }
  }
  return out.slice(0, 80);
});
const showOnboarding = computed(() => onboardingReady.value && !loading.value && devices.value.length === 0);

function deviceStatusClass(row) {
  return statusClass(row);
}

function scrollToRef(elRef) {
  const el = elRef?.value;
  if (!el) return;
  el.scrollIntoView({ behavior: "smooth", block: "start" });
}

function openHealthDetail() {
  activeDashboardModule.value = "health";
}

function openAvailabilityDetail() {
  statusQuickFilter.value = "online";
  activeDashboardModule.value = "assets";
}

function openAlertsDetail(level = "") {
  router.push({ path: "/alerts", query: level ? { level } : {} });
}

function runDashboardSearch() {
  if (globalKeyword.value.trim()) {
    activeDashboardModule.value = "assets";
  }
}

function openHotspotPort(item) {
  const id = Number(item?.interfaceId || 0);
  if (!id) return;
  const q = {
    deviceId: String(item?.deviceId || ""),
    deviceIp: String(item?.deviceIp || ""),
    portName: String(item?.interfaceName || item?.portName || ""),
    portBaseName: String(item?.interfaceName || item?.portName || ""),
    portRemark: String(item?.remark || ""),
    speedMbps: String(item?.speedMbps || 0)
  };
  router.push({ path: `/port/${id}`, query: q });
}

function severityTag(sev) {
  if (sev === "critical") return "danger";
  if (sev === "warning") return "warning";
  return "success";
}

async function loadDevices(opts = {}) {
  const silent = Boolean(opts.silent);
  if (!silent) loading.value = true;
  try {
    const res = await api.listDevices();
    devices.value = sortAssets((res.data || []).map((x) => ({ ...x, location: x.location || "" })));
    deviceLoadError.value = "";
  } catch (err) {
    deviceLoadError.value = err?.response?.data?.message || err?.response?.data?.error || err?.message || "资产列表加载失败";
    if (!silent) fb.apiError(err, "加载资产失败");
  } finally {
    if (!silent) loading.value = false;
    if (!silent) onboardingReady.value = true;
  }
}

async function loadAlerts(opts = {}) {
  const silent = Boolean(opts.silent);
  if (!silent) feedLoading.value = true;
  try {
    // 事件流支持 20 条/页，共 5 页，需要拉取 100 条
    await ops.refreshRealtimeAlerts(100);
    feedLoadError.value = "";
  } catch (err) {
    feedLoadError.value = err?.response?.data?.message || err?.response?.data?.error || err?.message || "事件流加载失败";
    if (!silent) fb.apiError(err, "加载事件流失败");
  } finally {
    if (!silent) feedLoading.value = false;
  }
}

function resetEventFilters() {
  eventFilterKeyword.value = "";
  eventFilterType.value = "all";
  eventFilterLevel.value = "all";
  eventFilterStart.value = "";
  eventFilterEnd.value = "";
  eventPage.value = 1;
}

function applyEventFilters() {
  eventPage.value = 1;
}

async function refreshAll(opts = {}) {
  if (refreshInFlight) return;
  if (document.visibilityState === "hidden") return;
  refreshInFlight = true;
  const silent = Boolean(opts.silent);
  try {
    await Promise.all([loadDevices({ silent }), loadAlerts({ silent }), loadHealthTrend({ silent }), loadTopology({ silent })]);
    lastRefreshedAt.value = new Date().toLocaleTimeString("zh-CN", { hour12: false });
    chartRefreshKey.value += 1;
    const totalPages = Math.max(1, Math.ceil(Math.min(filteredAlerts.value.length, 100) / eventPageSize.value));
    if (eventPage.value > totalPages) eventPage.value = 1;
  } finally {
    refreshInFlight = false;
  }
}

async function loadTopology(opts = {}) {
  const silent = Boolean(opts.silent);
  if (!silent) topologyLoading.value = true;
  try {
    const res = await api.getTopology();
    topologyGraph.value = { nodes: res.data?.nodes || [], edges: res.data?.edges || [] };
    topologyError.value = "";
  } catch (err) {
    topologyError.value = err?.response?.data?.message || err?.response?.data?.error || err?.message || "拓扑加载失败";
    if (!silent) fb.apiError(err, "加载拓扑失败");
  } finally {
    if (!silent) topologyLoading.value = false;
  }
}

async function loadHealthTrend(opts = {}) {
  const silent = Boolean(opts.silent);
  try {
    const res = await api.getSystemHealthTrend(30);
    healthTrend.value = res?.data?.data || [];
    healthTrendError.value = "";
  } catch (err) {
    healthTrendError.value = err?.response?.data?.message || err?.response?.data?.error || err?.message || "健康趋势加载失败";
    if (!silent) fb.apiError(err, "加载健康趋势失败");
  }
}

function openDeviceDetail(row) {
  if (!row?.id) return;
  router.push(`/device/${row.id}`);
}
function openTopologyNode(node) {
  if (node?.device_id) router.push(`/device/${node.device_id}`);
}
function openPortDetail(row) {
  if (!row?.portId) return;
  router.push({
    path: `/port/${row.portId}`,
    query: {
      deviceId: String(row.deviceId),
      deviceIp: row.deviceIP,
      portName: row.portName,
      portRemark: row.remark || "",
      speedMbps: String(row.speedMbps || 0)
    }
  });
}

function loadTopologyDisplaySettings() {
  topologyTooltipFontSize.value = Math.max(16, Number(localStorage.getItem("np_topology_tooltip_font_size") || 18));
  topologyLabelDisplayMode.value = localStorage.getItem("np_topology_label_display_mode") || "hover";
  try {
    const tiers = JSON.parse(localStorage.getItem("np_topology_label_display_tiers") || "[\"core\"]");
    topologyLabelDisplayTiers.value = Array.isArray(tiers) ? tiers : ["core"];
  } catch {
    topologyLabelDisplayTiers.value = ["core"];
  }
}
function rankedPortsBySpeed(min, max) {
  const points = [];
  for (const d of devices.value) {
    for (const p of d.interfaces || []) {
      const speed = Number(p.speed_mbps || 0);
      if (speed < min) continue;
      if (max > 0 && speed >= max) continue;
      const inBps = Number(p.traffic_in_bps || 0);
      const outBps = Number(p.traffic_out_bps || 0);
      const heat = Math.max(inBps, outBps);
      if (heat <= 0) continue;
      points.push({
        deviceName: d.name || d.ip,
        deviceId: d.id,
        deviceIp: d.ip,
        interfaceName: p.name || `ifIndex-${p.index}`,
        interfaceIndex: Number(p.index || 0),
        interfaceId: p.id,
        speedMbps: speed,
        inBps,
        outBps,
        bps: inBps + outBps
      });
    }
  }
  points.sort((a, b) => b.bps - a.bps);
  return points.slice(0, 10);
}

const activeTopHotspots = computed(() => {
  if (topTab.value === "1g") return top1G.value;
  if (topTab.value === "10g") return top10G.value;
  return top100M.value;
});

function openEventDetail(event) {
  if (!event) return;
  eventDetail.value = event;
  eventDetailVisible.value = true;
}

function jumpToEventDevice() {
  const id = Number(eventDetail.value?.device_id || 0);
  if (!id) return;
  eventDetailVisible.value = false;
  router.push(`/device/${id}`);
}

function jumpToEventPort() {
  const id = Number(eventDetail.value?.interface_id || eventDetail.value?.port_id || 0);
  if (!id) return;
  eventDetailVisible.value = false;
  const q = {
    deviceId: String(eventDetail.value?.device_id || ""),
    deviceIp: String(eventDetail.value?.device_ip || ""),
    portName: String(eventDetail.value?.interface_name || eventDetail.value?.port_name || ""),
    portBaseName: String(eventDetail.value?.interface_raw_name || eventDetail.value?.interface_name || eventDetail.value?.port_name || ""),
    portRemark: String(eventDetail.value?.interface_remark || "")
  };
  router.push({ path: `/port/${id}`, query: q });
}

onMounted(async () => {
  loadTopologyDisplaySettings();
  await refreshAll();
  timer = setInterval(() => {
    refreshAll({ silent: true });
  }, 20000);
  document.addEventListener("visibilitychange", onVisibilityChange);
  window.addEventListener("np-topology-settings-changed", loadTopologyDisplaySettings);
});

onActivated(() => {
  loadTopologyDisplaySettings();
  refreshAll({ silent: true });
});

onBeforeUnmount(() => {
  if (timer) clearInterval(timer);
  document.removeEventListener("visibilitychange", onVisibilityChange);
  window.removeEventListener("np-topology-settings-changed", loadTopologyDisplaySettings);
});

function onVisibilityChange() {
  if (document.visibilityState === "visible") refreshAll({ silent: true });
}

watch(activeDashboardModule, async () => {
  await nextTick();
  chartRefreshKey.value += 1;
});
</script>

<template>
  <div class="space-y-5">
    <el-card class="np-search-hero">
      <div class="flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
        <div>
          <div class="text-lg font-semibold text-slate-900">全局搜索</div>
          <div class="text-xs text-slate-500">
            快速定位设备、端口、备注并直达详情
            <span v-if="lastRefreshedAt" class="ml-2">最近刷新：{{ lastRefreshedAt }}</span>
          </div>
        </div>
        <div class="flex w-full flex-wrap items-center gap-2 lg:w-auto">
          <el-input v-model="globalKeyword" placeholder="搜索 IP / 名称 / 备注 / 端口名" clearable class="w-full lg:w-[420px]" @keyup.enter="runDashboardSearch" />
          <el-button type="primary" @click="runDashboardSearch">搜索</el-button>
          <el-select v-model="statusQuickFilter" class="w-full lg:w-[130px]">
            <el-option label="全部状态" value="all" />
            <el-option label="仅在线" value="online" />
            <el-option label="仅离线" value="offline" />
          </el-select>
          <el-button type="primary" @click="refreshAll">刷新</el-button>
        </div>
      </div>
    </el-card>

    <el-alert
      v-if="deviceLoadError"
      type="warning"
      show-icon
      :closable="false"
      title="资产数据暂时加载失败"
    >
      <template #default>
        <div class="flex flex-wrap items-center gap-2">
          <span>{{ deviceLoadError }}</span>
          <el-button size="small" @click="loadDevices()">重试加载资产</el-button>
          <el-button
            size="small"
            type="primary"
            plain
            @click="$router.push({ path: '/alerts', query: { tab: 'asset-diagnosis' } })"
          >
            打开资产加载诊断
          </el-button>
        </div>
      </template>
    </el-alert>

    <el-card v-if="showOnboarding">
      <template #header><span class="text-lg font-semibold">首次引导</span></template>
      <el-steps :active="1" finish-status="success" align-center>
        <el-step title="连接数据库" description="在系统设置确认数据库已连接" />
        <el-step title="添加首台资产" description="前往资产中心添加设备并完成SNMP预检" />
        <el-step title="确认采集成功" description="查看设备详情中的CPU/内存与端口流量" />
      </el-steps>
      <div class="mt-4 flex gap-2">
        <el-button type="primary" @click="$router.push('/assets')">去资产中心</el-button>
        <el-button @click="$router.push('/settings')">去系统设置</el-button>
      </div>
    </el-card>

    <el-card class="np-topology-preview-card">
      <template #header>
        <div class="flex flex-wrap items-center justify-between gap-2">
          <div>
            <span class="np-section-title text-base font-semibold">拓扑图预览</span>
            <div class="mt-1 text-xs text-slate-500">默认自适应显示手动拓扑，支持拖拽移动，滚轮缩放默认关闭，10分钟自动恢复默认视图</div>
          </div>
          <div class="flex flex-wrap items-center gap-2">
            <el-button
              size="small"
              :type="topologyWheelZoomEnabled ? 'primary' : 'default'"
              plain
              @click="topologyWheelZoomEnabled = !topologyWheelZoomEnabled"
            >
              滚轮缩放{{ topologyWheelZoomEnabled ? "已开" : "已关" }}
            </el-button>
            <el-button size="small" @click="topologyRef?.fit()">自适应</el-button>
            <el-button size="small" type="primary" plain @click="$router.push('/topology')">拓扑管理</el-button>
          </div>
        </div>
      </template>
      <el-alert
        v-if="topologyError"
        class="mb-3"
        type="warning"
        show-icon
        :closable="false"
        title="拓扑图暂时加载失败"
      >
        <template #default>
          <div class="flex flex-wrap items-center gap-2">
            <span>{{ topologyError }}</span>
            <el-button size="small" @click="loadTopology()">重试加载拓扑</el-button>
          </div>
        </template>
      </el-alert>
      <TopologyCanvas
        ref="topologyRef"
        :nodes="topologyGraph.nodes"
        :edges="topologyGraph.edges"
        :loading="topologyLoading"
        :height="460"
        :tooltip-font-size="topologyTooltipFontSize"
        :label-display-mode="topologyLabelDisplayMode"
        :label-display-tiers="topologyLabelDisplayTiers"
        :wheel-zoom="topologyWheelZoomEnabled"
        auto-fit
        @node-open="openTopologyNode"
      />
    </el-card>

    <section class="np-dashboard-shell">
      <el-card class="np-module-card">
        <el-tabs v-model="activeDashboardModule" class="np-dashboard-tabs">
          <el-tab-pane label="实时事件流" name="events">
            <div class="mb-3 flex flex-wrap items-center justify-between gap-2">
              <div class="text-xs text-slate-500">默认展示最新事件，需要筛选时可展开查询条件</div>
              <el-button size="small" @click="showEventFilters = !showEventFilters">
                {{ showEventFilters ? "隐藏查询" : "展开查询" }}
              </el-button>
            </div>
            <div v-show="showEventFilters" class="np-event-query mb-3 rounded-xl border border-slate-200 bg-slate-50/80 p-3">
              <div class="grid grid-cols-1 gap-2 2xl:grid-cols-[minmax(220px,1fr)_140px_140px_190px_190px_auto]">
                <el-input
                  v-model="eventFilterKeyword"
                  placeholder="查询资产名称 / IP / 端口名称 / 事件内容"
                  clearable
                  @keyup.enter="applyEventFilters"
                />
                <el-select v-model="eventFilterType">
                  <el-option label="全部类型" value="all" />
                  <el-option label="设备状态" value="device_status" />
                  <el-option label="端口状态" value="port_status" />
                  <el-option label="设备日志" value="log" />
                  <el-option label="告警事件" value="alert" />
                  <el-option label="轮询异常" value="polling" />
                </el-select>
                <el-select v-model="eventFilterLevel">
                  <el-option label="全部级别" value="all" />
                  <el-option label="正常/信息" value="info" />
                  <el-option label="警告" value="warning" />
                  <el-option label="严重" value="critical" />
                </el-select>
                <el-date-picker
                  v-model="eventFilterStart"
                  type="datetime"
                  placeholder="开始时间"
                  format="YYYY-MM-DD HH:mm"
                  value-format="YYYY-MM-DD HH:mm:ss"
                  class="w-full"
                />
                <el-date-picker
                  v-model="eventFilterEnd"
                  type="datetime"
                  placeholder="结束时间"
                  format="YYYY-MM-DD HH:mm"
                  value-format="YYYY-MM-DD HH:mm:ss"
                  class="w-full"
                />
                <div class="flex items-center gap-2 whitespace-nowrap">
                  <el-button type="primary" @click="applyEventFilters">查询</el-button>
                  <el-button @click="resetEventFilters">重置</el-button>
                </div>
              </div>
            </div>
            <el-alert
              v-if="feedLoadError"
              class="mb-3"
              type="warning"
              show-icon
              :closable="false"
              title="实时事件流暂时加载失败"
            >
              <template #default>
                <div class="flex flex-wrap items-center gap-2">
                  <span>{{ feedLoadError }}</span>
                  <el-button size="small" @click="loadAlerts()">重试加载事件流</el-button>
                </div>
              </template>
            </el-alert>
            <LiveEventFeed
              :loading="feedLoading"
              :alerts="pagedAlerts"
              :severity-tag="severityTag"
              :page="eventPage"
              :page-size="eventPageSize"
              :total="Math.min(filteredAlerts.length, 100)"
              @update:page="(p) => (eventPage = p)"
              @open-event="openEventDetail"
            />
          </el-tab-pane>

          <el-tab-pane label="Top N 端口流量" name="top">
            <div ref="topNRef" class="min-w-0">
              <div class="mb-3 flex flex-wrap items-center justify-between gap-2">
                <span class="text-lg font-semibold">Top N 端口流量</span>
                <span class="text-xs text-slate-500">按端口速率分组展示，每档展示 10 个，点击端口可直达流量详情</span>
              </div>
              <el-tabs v-model="topTab">
                <el-tab-pane label="100M" name="100m" />
                <el-tab-pane label="1G" name="1g" />
                <el-tab-pane label="10G" name="10g" />
              </el-tabs>
              <TrafficTopBar
                :title="topTab === '100m' ? '百兆口 Top N（100M）' : (topTab === '1g' ? '千兆口 Top N（1G）' : '万兆口 Top N（10G）')"
                :hotspots="activeTopHotspots"
                :refresh-key="chartRefreshKey"
                :loading="loading"
                expanded
                @open-port="openHotspotPort"
              />
            </div>
          </el-tab-pane>

          <el-tab-pane label="资产总览" name="assets">
            <div class="mb-3 flex flex-wrap items-center justify-between gap-2">
              <div class="flex flex-wrap items-center gap-2 w-full md:w-auto">
                <span class="text-lg font-semibold">资产总览（只读）</span>
                <el-button text type="primary" @click="healthExplainVisible = true">指标口径说明</el-button>
              </div>
              <div class="text-xs text-slate-500">可点击设备或端口名称直达详情</div>
            </div>

            <el-skeleton :loading="loading" animated :rows="10">
              <template #default>
                <el-table :data="filteredDevices" class="np-borderless-table np-sticky-table" size="large" max-height="520" @row-click="openDeviceDetail">
                  <el-table-column label="状态" width="90">
                    <template #default="{ row }">
                      <el-tooltip :content="statusLabel(row)">
                        <span class="inline-flex items-center gap-1 align-middle">
                          <span class="inline-block" :class="deviceStatusClass(row)" />
                          <span class="text-xs text-slate-500">{{ statusLabel(row) }}</span>
                        </span>
                      </el-tooltip>
                    </template>
                  </el-table-column>
                  <el-table-column label="名称" min-width="160">
                    <template #default="{ row }">
                      <el-button link type="primary" @click="openDeviceDetail(row)">{{ row.name || row.ip }}</el-button>
                    </template>
                  </el-table-column>
                  <el-table-column prop="ip" label="IP" min-width="160" />
                  <el-table-column prop="brand" label="品牌" width="120" />
                  <el-table-column label="类型" width="140">
                    <template #default="{ row }">{{ row.device_type || row.type || '-' }}</template>
                  </el-table-column>
                  <el-table-column label="CPU快照" width="120">
                    <template #default="{ row }">{{ Number.isFinite(Number(row.cpu_usage)) ? `${Number(row.cpu_usage).toFixed(1)}%` : "-" }}</template>
                  </el-table-column>
                  <el-table-column label="运行时长" min-width="140">
                    <template #default="{ row }">{{ row.uptime || "-" }}</template>
                  </el-table-column>
                  <el-table-column prop="remark" label="备注" min-width="220" />
                </el-table>
                <el-divider v-if="filteredPorts.length">端口命中结果（可直接点击进入）</el-divider>
                <el-table v-if="filteredPorts.length" :data="filteredPorts" class="np-borderless-table np-sticky-table" size="small" max-height="320" @row-click="openPortDetail">
                  <el-table-column label="端口" min-width="200">
                    <template #default="{ row }">
                      <el-button link type="primary" @click="openPortDetail(row)">{{ row.portName }}</el-button>
                    </template>
                  </el-table-column>
                  <el-table-column label="所属设备" min-width="180">
                    <template #default="{ row }">
                      <el-button link type="primary" @click="openDeviceDetail({ id: row.deviceId })">{{ row.deviceName }}</el-button>
                    </template>
                  </el-table-column>
                  <el-table-column prop="deviceIP" label="设备IP" min-width="150" />
                  <el-table-column label="端口速率" width="120">
                    <template #default="{ row }">{{ row.speedMbps > 0 ? `${row.speedMbps} Mbps` : "-" }}</template>
                  </el-table-column>
                  <el-table-column prop="remark" label="端口备注" min-width="220" />
                </el-table>
              </template>
            </el-skeleton>
          </el-tab-pane>

          <el-tab-pane label="全网健康趋势" name="health">
            <div ref="healthRef">
              <el-alert
                v-if="healthTrendError"
                class="mb-3"
                type="warning"
                show-icon
                :closable="false"
                title="健康趋势暂时加载失败"
                :description="healthTrendError"
              >
                <template #default>
                  <div class="flex flex-wrap items-center gap-2">
                    <span>{{ healthTrendError }}</span>
                    <el-button size="small" @click="loadHealthTrend()">重试</el-button>
                  </div>
                </template>
              </el-alert>
              <HealthTrendArea :trend="healthTrend" :refresh-key="chartRefreshKey" :loading="loading" compact />
            </div>
          </el-tab-pane>
        </el-tabs>
      </el-card>

      <aside class="np-dashboard-kpis">
        <StatsCards
          :health-score="healthScore"
          :availability="availability"
          :online-count="onlineCount"
          :total-count="devices.length"
          :active-alerts="activeAlerts"
          :alert-breakdown="alertBreakdown"
          vertical
          @open-health="openHealthDetail"
          @open-availability="openAvailabilityDetail"
          @open-alerts="openAlertsDetail"
        />
      </aside>
    </section>

    <el-card v-if="todoActions.length">
      <template #header><span class="text-lg font-semibold">今日待处理</span></template>
      <div class="grid grid-cols-1 gap-2 md:grid-cols-3">
        <button
          v-for="x in todoActions"
          :key="x.key"
          class="rounded-xl border border-slate-200 bg-slate-50 p-3 text-left transition hover:bg-indigo-50 hover:border-indigo-200 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-indigo-500"
          @click="x.action()"
        >
          <div class="text-sm font-semibold text-slate-800">{{ x.title }}</div>
          <div class="mt-1 text-xs text-slate-500">点击立即处理</div>
        </button>
      </div>
    </el-card>
    <el-drawer v-model="eventDetailVisible" title="事件详情" size="520px">
      <div class="space-y-3" v-if="eventDetail">
        <div class="rounded-lg border border-slate-200 bg-slate-50 p-3">
          <div class="text-xs text-slate-500">设备</div>
          <div class="text-sm text-slate-800">{{ eventDetail.device_name || eventDetail.device_ip || "-" }}</div>
        </div>
        <div class="rounded-lg border border-slate-200 bg-slate-50 p-3">
          <div class="text-xs text-slate-500">时间</div>
          <div class="text-sm text-slate-800">{{ eventDetail.timestamp || eventDetail.created_at || "-" }}</div>
        </div>
        <div class="rounded-lg border border-slate-200 bg-slate-50 p-3">
          <div class="text-xs text-slate-500">级别</div>
          <div class="text-sm text-slate-800">{{ eventDetail.level || eventDetail.severity || "-" }}</div>
        </div>
        <div class="rounded-lg border border-slate-200 bg-slate-50 p-3">
          <div class="text-xs text-slate-500">事件内容</div>
          <div class="text-sm text-slate-800 break-all">{{ eventDetail.message || "-" }}</div>
        </div>
        <div class="rounded-lg border border-slate-200 bg-slate-50 p-3">
          <div class="text-xs text-slate-500">追溯字段</div>
          <div class="text-sm text-slate-800">device_id: {{ eventDetail.device_id || "-" }}</div>
          <div class="text-sm text-slate-800">interface_id: {{ eventDetail.interface_id || eventDetail.port_id || "-" }}</div>
          <div class="text-sm text-slate-800">event_id: {{ eventDetail.id || "-" }}</div>
        </div>
        <div class="flex items-center gap-2 pt-1">
          <el-button type="primary" :disabled="!eventDetail?.device_id" @click="jumpToEventDevice">定位到设备</el-button>
          <el-button :disabled="!(eventDetail?.interface_id || eventDetail?.port_id)" @click="jumpToEventPort">定位到端口</el-button>
        </div>
      </div>
    </el-drawer>

    <el-dialog v-model="healthExplainVisible" title="指标口径说明" width="760">
      <div class="space-y-2 text-sm text-slate-700">
        <p>全局健康评分：设备可用率 - 告警惩罚分（严重*6 + 警告*2，上限35）。</p>
        <p>设备可用率：在线设备数 / 设备总数。</p>
        <p>活动告警：实时事件流中严重+警告数量。</p>
        <p>流量热点：按端口最新入/出流量总和排序 Top 3。</p>
      </div>
      <template #footer>
        <el-button type="primary" @click="healthExplainVisible = false">我知道了</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
:deep(.np-search-hero .el-card__body) {
  padding-top: 14px;
  padding-bottom: 14px;
}

:deep(.np-sticky-table .el-table__header-wrapper) {
  position: sticky;
  top: 0;
  z-index: 3;
}

:deep(.np-module-card .el-card__body) {
  padding-top: 12px;
}

.np-dashboard-shell {
  display: grid;
  grid-template-columns: minmax(0, 1fr) minmax(320px, 340px);
  gap: 18px;
  align-items: start;
}

.np-dashboard-kpis {
  min-width: 0;
  position: sticky;
  top: 88px;
}

:deep(.np-dashboard-tabs > .el-tabs__header) {
  position: relative;
  z-index: 2;
  margin-bottom: 18px;
  padding-bottom: 4px;
  background: rgba(255, 255, 255, 0.94);
  backdrop-filter: blur(8px);
}

:deep(.np-dashboard-tabs .el-tabs__content) {
  min-width: 0;
  overflow: visible;
  padding-top: 4px;
}

:deep(.np-dashboard-tabs .el-tab-pane) {
  min-width: 0;
}

:deep(.np-topology-preview-card .el-card__header) {
  padding-bottom: 12px;
}

@media (max-width: 1440px) {
  .np-dashboard-shell {
    grid-template-columns: 1fr;
  }

  .np-dashboard-kpis {
    position: static;
    order: -1;
  }
}
</style>
