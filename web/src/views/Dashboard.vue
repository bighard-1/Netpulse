<script setup>
import { computed, onActivated, onBeforeUnmount, onMounted, ref } from "vue";
import { ElMessage } from "element-plus";
import { useRouter } from "vue-router";
import { api, getApiError } from "../services/api";
import { useOpsStore } from "../stores/ops";
import { formatBps } from "../utils/format";
import StatsCards from "../components/dashboard/StatsCards.vue";
import LiveEventFeed from "../components/dashboard/LiveEventFeed.vue";

const ops = useOpsStore();
const router = useRouter();

const loading = ref(false);
const feedLoading = ref(false);
const devices = ref([]);
const globalKeyword = ref("");
let timer = null;

const onlineCount = computed(() => devices.value.filter((d) => d.status === "online").length);
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

const filteredDevices = computed(() => {
  const kw = globalKeyword.value.trim().toLowerCase();
  if (!kw) return devices.value;
  return devices.value.filter((d) => {
    const ports = (d.interfaces || []).map((p) => `${p.name || ""} ${p.remark || ""} ${p.index || ""}`).join(" ");
    return [d.ip, d.name, d.brand, d.remark, d.location, ports, d.status].join(" ").toLowerCase().includes(kw);
  });
});

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
    if (!silent) ElMessage.error(getApiError(err, "加载资产失败"));
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
    if (!silent) ElMessage.error(getApiError(err, "加载事件流失败"));
  } finally {
    if (!silent) feedLoading.value = false;
  }
}

async function refreshAll(opts = {}) {
  const silent = Boolean(opts.silent);
  await loadDevices({ silent });
  await loadAlerts({ silent });
}

function openDeviceDetail(row) {
  if (!row?.id) return;
  router.push(`/device/${row.id}`);
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
    <StatsCards
      :health-score="healthScore"
      :availability="availability"
      :online-count="onlineCount"
      :total-count="devices.length"
      :active-alerts="activeAlerts"
      :alert-breakdown="alertBreakdown"
      :traffic-hotspots="trafficHotspots"
    />

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
                  <span class="inline-block h-2.5 w-2.5 rounded-full" :class="row.status === 'online' ? 'status-dot-online' : (row.status === 'offline' ? 'bg-rose-500' : 'bg-amber-400')" />
                </template>
              </el-table-column>
              <el-table-column label="名称" min-width="160">
                <template #default="{ row }">
                  <el-button link type="primary" @click="openDeviceDetail(row)">{{ row.name || row.ip }}</el-button>
                </template>
              </el-table-column>
              <el-table-column prop="ip" label="IP" min-width="160" />
              <el-table-column prop="brand" label="品牌" width="120" />
              <el-table-column prop="remark" label="备注" min-width="220" />
            </el-table>
          </template>
        </el-skeleton>
      </el-card>

      <LiveEventFeed :loading="feedLoading" :alerts="ops.realtimeAlerts" :severity-tag="severityTag" />
    </section>
  </div>
</template>
