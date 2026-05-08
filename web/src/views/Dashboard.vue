<script setup>
import { computed, onActivated, onBeforeUnmount, onMounted, ref } from "vue";
import { useRouter } from "vue-router";
import { api } from "../services/api";
import { useOpsStore } from "../stores/ops";
import { formatBps } from "../utils/format";
import { normalizeStatus, statusClass } from "../utils/status";
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
const healthTrend = ref([]);
const eventDetailVisible = ref(false);
const eventDetail = ref(null);

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
  if (!kw) return devices.value;
  return devices.value.filter((d) => {
    const ports = (d.interfaces || [])
      .map((p) => `${p.name || ""} ${p.alias || ""} ${p.custom_name || ""} ${p.remark || ""} ${p.index || ""}`)
      .join(" ");
    return [d.ip, d.name, d.brand, d.remark, d.location, d.site, ports, d.status].join(" ").toLowerCase().includes(kw);
  });
});
const showOnboarding = computed(() => devices.value.length === 0);

function deviceStatusClass(row) {
  return statusClass(row?.status);
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
  const silent = Boolean(opts.silent);
  await Promise.all([loadDevices({ silent }), loadAlerts({ silent }), loadHealthTrend({ silent })]);
}

async function loadHealthTrend(opts = {}) {
  const silent = Boolean(opts.silent);
  try {
    const end = new Date();
    const start = new Date(end.getTime() - 30 * 24 * 3600 * 1000);
    const res = await api.getSystemHealthTrend(start.toISOString(), end.toISOString());
    healthTrend.value = res.data || [];
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
});

onActivated(() => refreshAll({ silent: true }));

onBeforeUnmount(() => {
  if (timer) clearInterval(timer);
});
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
    />

    <section class="grid grid-cols-1 gap-5 2xl:grid-cols-[3fr,2fr]">
      <HealthTrendArea :trend="healthTrend" />
      <TrafficTopBar :hotspots="trafficHotspots" />
    </section>

    <section class="grid grid-cols-1 gap-5 2xl:grid-cols-[2fr,1fr]">
      <el-card>
        <template #header>
          <div class="flex flex-wrap items-center justify-between gap-2">
            <span class="text-lg font-semibold">资产总览（只读）</span>
            <div class="flex items-center gap-2">
              <el-input v-model="globalKeyword" placeholder="搜索 IP / 名称 / 备注 / 端口名" clearable class="w-[320px]" />
              <el-button @click="refreshAll">刷新</el-button>
            </div>
          </div>
        </template>

        <el-skeleton :loading="loading" animated :rows="10">
          <template #default>
            <el-table :data="filteredDevices" class="np-borderless-table" size="large">
              <el-table-column label="状态" width="90">
                <template #default="{ row }">
                  <span class="inline-block align-middle" :class="deviceStatusClass(row)" />
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
  </div>
</template>
