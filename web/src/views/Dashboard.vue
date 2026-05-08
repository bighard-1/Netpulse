<script setup>
import { computed, onActivated, onBeforeUnmount, onMounted, ref } from "vue";
import { useRouter } from "vue-router";
import { api } from "../services/api";
import { useOpsStore } from "../stores/ops";
import { formatBps } from "../utils/format";
import { normalizeStatus, statusClass, statusLabel } from "../utils/status";
import { useFeedback } from "../composables/useFeedback";
import StatsCards from "../components/dashboard/StatsCards.vue";
import LiveEventFeed from "../components/dashboard/LiveEventFeed.vue";
import HealthTrendArea from "../components/dashboard/HealthTrendArea.vue";
import TrafficTopBar from "../components/dashboard/TrafficTopBar.vue";
import PhaseRoadmap from "../components/dashboard/PhaseRoadmap.vue";

const ops = useOpsStore();
const router = useRouter();
const fb = useFeedback();

const loading = ref(false);
const feedLoading = ref(false);
const devices = ref([]);
const globalKeyword = ref("");
let timer = null;
let refreshInFlight = false;
const healthTrend = ref([]);
const healthExplainVisible = ref(false);
const eventDetailVisible = ref(false);
const eventDetail = ref(null);
const statusQuickFilter = ref("all");
const healthRef = ref(null);
const hotspotRef = ref(null);
const todoActions = computed(() => {
  const out = [];
  if (devices.value.length === 0) out.push({ key: "add", title: "添加首台资产", action: () => router.push("/assets") });
  if (activeAlerts.value > 0) out.push({ key: "alert", title: `处理 ${activeAlerts.value} 条活动告警`, action: () => router.push("/alerts") });
  if (trafficHotspots.value.length === 0) out.push({ key: "traffic", title: "等待流量采样，检查采集设置", action: () => router.push("/settings") });
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

const trafficHotspots = computed(() => {
  const points = [];
  for (const d of devices.value) {
    for (const p of d.interfaces || []) {
      const heat = Number(p.traffic_in_bps || 0) + Number(p.traffic_out_bps || 0);
      if (heat > 0) points.push({ deviceName: d.name || d.ip, interfaceName: p.name, interfaceId: p.id, bps: heat });
    }
  }
  points.sort((a, b) => b.bps - a.bps);
  return points.slice(0, 3);
});

const storageRiskCount = computed(() => {
  return devices.value.filter((d) => {
    const v = Number(d.storage_usage ?? d.disk_usage ?? d.flash_usage ?? NaN);
    return Number.isFinite(v) && v >= 85;
  }).length;
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
const showOnboarding = computed(() => devices.value.length === 0);

function deviceStatusClass(row) {
  return statusClass(row);
}

function scrollToRef(elRef) {
  const el = elRef?.value;
  if (!el) return;
  el.scrollIntoView({ behavior: "smooth", block: "start" });
}

function openHealthDetail() {
  scrollToRef(healthRef);
}

function openAvailabilityDetail() {
  statusQuickFilter.value = "online";
}

function openAlertsDetail() {
  router.push("/alerts");
}

function openHotspotsDetail() {
  scrollToRef(hotspotRef);
}

function openStorageDetail() {
  router.push("/assets");
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
    devices.value = (res.data || []).map((x) => ({ ...x, location: x.location || "" }));
  } catch (err) {
    if (!silent) fb.apiError(err, "加载资产失败");
  } finally {
    if (!silent) loading.value = false;
  }
}

async function loadAlerts(opts = {}) {
  const silent = Boolean(opts.silent);
  if (!silent) feedLoading.value = true;
  try {
    await ops.refreshRealtimeAlerts(20);
  } catch (err) {
    if (!silent) fb.apiError(err, "加载事件流失败");
  } finally {
    if (!silent) feedLoading.value = false;
  }
}

async function refreshAll(opts = {}) {
  if (refreshInFlight) return;
  if (document.visibilityState === "hidden") return;
  refreshInFlight = true;
  const silent = Boolean(opts.silent);
  try {
    await Promise.all([loadDevices({ silent }), loadAlerts({ silent }), loadHealthTrend({ silent })]);
  } finally {
    refreshInFlight = false;
  }
}

async function loadHealthTrend(opts = {}) {
  const silent = Boolean(opts.silent);
  try {
    const res = await api.getSystemHealthTrend(30);
    healthTrend.value = res?.data?.data || [];
  } catch (err) {
    if (!silent) fb.apiError(err, "加载健康趋势失败");
  }
}

function openDeviceDetail(row) {
  if (!row?.id) return;
  router.push(`/device/${row.id}`);
}

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
  router.push(`/port/${id}`);
}

onMounted(async () => {
  await refreshAll();
  timer = setInterval(() => {
    refreshAll({ silent: true });
  }, 20000);
  document.addEventListener("visibilitychange", onVisibilityChange);
});

onActivated(() => refreshAll({ silent: true }));

onBeforeUnmount(() => {
  if (timer) clearInterval(timer);
  document.removeEventListener("visibilitychange", onVisibilityChange);
});

function onVisibilityChange() {
  if (document.visibilityState === "visible") refreshAll({ silent: true });
}
</script>

<template>
  <div class="space-y-5">
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

    <StatsCards
      :health-score="healthScore"
      :availability="availability"
      :online-count="onlineCount"
      :total-count="devices.length"
      :active-alerts="activeAlerts"
      :alert-breakdown="alertBreakdown"
      :traffic-hotspots="trafficHotspots"
      :storage-risk-count="storageRiskCount"
      @open-health="openHealthDetail"
      @open-availability="openAvailabilityDetail"
      @open-alerts="openAlertsDetail"
      @open-hotspots="openHotspotsDetail"
      @open-storage="openStorageDetail"
    />

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

    <section ref="healthRef" class="grid grid-cols-1 gap-5 2xl:grid-cols-[3fr,2fr]">
      <HealthTrendArea :trend="healthTrend" />
      <div ref="hotspotRef"><TrafficTopBar :hotspots="trafficHotspots" /></div>
    </section>

    <section class="grid grid-cols-1 gap-5 2xl:grid-cols-[2fr,1fr]">
      <el-card>
        <template #header>
          <div class="flex flex-wrap items-center justify-between gap-2">
            <div class="flex items-center gap-2">
              <span class="text-lg font-semibold">资产总览（只读）</span>
              <el-button text type="primary" @click="healthExplainVisible = true">指标口径说明</el-button>
            </div>
            <div class="flex items-center gap-2">
              <el-input v-model="globalKeyword" placeholder="搜索 IP / 名称 / 备注 / 端口名" clearable class="w-[320px]" />
              <el-select v-model="statusQuickFilter" class="w-[130px]">
                <el-option label="全部状态" value="all" />
                <el-option label="仅在线" value="online" />
                <el-option label="仅离线" value="offline" />
              </el-select>
              <el-button @click="refreshAll">刷新</el-button>
            </div>
          </div>
        </template>

        <el-skeleton :loading="loading" animated :rows="10">
          <template #default>
            <el-table :data="filteredDevices" class="np-borderless-table" size="large">
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
          </template>
        </el-skeleton>
      </el-card>

      <LiveEventFeed
        :loading="feedLoading"
        :alerts="ops.realtimeAlerts"
        :severity-tag="severityTag"
        @open-event="openEventDetail"
      />
    </section>

    <PhaseRoadmap />

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
