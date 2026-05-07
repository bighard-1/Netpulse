<script setup>
import * as echarts from "echarts";
import { computed, nextTick, onActivated, onBeforeUnmount, onMounted, ref, watch } from "vue";
import { ElMessage } from "element-plus";
import { api, getApiError } from "../services/api";
import { useOpsStore } from "../stores/ops";
import { formatBps } from "../utils/format";
import { zhCN } from "../i18n/zhCN";
import StatsCards from "../components/dashboard/StatsCards.vue";
import LiveEventFeed from "../components/dashboard/LiveEventFeed.vue";

const ops = useOpsStore();

const loading = ref(false);
const devices = ref([]);
const feedLoading = ref(false);
const globalKeyword = ref("");
const groupBy = ref("brand");

const addVisible = ref(false);
const addLoading = ref(false);
const defaultAddForm = () => ({
  ip: "",
  name: "",
  brand: "H3C",
  community: "public",
  remark: "",
  snmp_version: "2c",
  snmp_port: 161,
  v3_username: "",
  v3_security_level: "noAuthNoPriv",
  v3_auth_protocol: "SHA",
  v3_auth_password: "",
  v3_priv_protocol: "AES",
  v3_priv_password: ""
});
const addForm = ref(defaultAddForm());
const isSnmpV3 = computed(() => String(addForm.value.snmp_version) === "3");

const drawerLoading = ref(false);
const drawerDevice = ref(null);
const drawerPorts = ref([]);
const drawerRange = ref([new Date(Date.now() - 24 * 3600 * 1000), new Date()]);
const selectedPort = ref(null);
const portDrawerVisible = ref(false);
const portTraffic = ref([]);

const healthTrendLoading = ref(false);
const healthTrend = ref([]);

const healthChartEl = ref(null);
const drawerCpuMemChartEl = ref(null);
const portTrafficChartEl = ref(null);
let healthChart = null;
let cpuMemChart = null;
let trafficChart = null;
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

const filteredDevices = computed(() => {
  const kw = globalKeyword.value.trim().toLowerCase();
  if (!kw) return devices.value;
  return devices.value.filter((d) => {
    const ports = (d.interfaces || []).map((p) => `${p.name || ""} ${p.remark || ""} ${p.index || ""}`).join(" ");
    return [d.ip, d.name, d.brand, d.remark, d.location, ports, d.status].join(" ").toLowerCase().includes(kw);
  });
});

const groupedDevices = computed(() => {
  const buckets = new Map();
  for (const d of filteredDevices.value) {
    const key = groupBy.value === "location" ? (d.location || d.site || "未分配位置") : (d.brand || "未知品牌");
    if (!buckets.has(key)) buckets.set(key, []);
    buckets.get(key).push(d);
  }
  return [...buckets.entries()].map(([group, rows]) => ({ group, rows }));
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

function iso(v) {
  return new Date(v).toISOString();
}

function fmtTime(v) {
  return new Date(v).toLocaleString();
}

function severityTag(sev) {
  if (sev === "critical") return "danger";
  if (sev === "warning") return "warning";
  return "success";
}

async function loadDevices() {
  loading.value = true;
  try {
    const res = await api.listDevices();
    devices.value = (res.data || []).map((x) => ({ ...x, location: x.location || "" }));
  } catch (err) {
    ElMessage.error(getApiError(err, "加载资产失败"));
  } finally {
    loading.value = false;
  }
}

async function loadAlerts() {
  feedLoading.value = true;
  try {
    await ops.refreshRealtimeAlerts(20);
  } catch (err) {
    ElMessage.error(getApiError(err, "加载事件流失败"));
  } finally {
    feedLoading.value = false;
  }
}

async function loadHealthTrend() {
  healthTrendLoading.value = true;
  try {
    const end = new Date();
    const start = new Date(Date.now() - 30 * 24 * 3600 * 1000);
    const res = await api.getSystemHealthTrend(iso(start), iso(end));
    healthTrend.value = res.data?.data || res.data || [];
  } catch {
    healthTrend.value = [];
  } finally {
    healthTrendLoading.value = false;
    await nextTick();
    renderHealthTrendChart();
  }
}

async function refreshAll() {
  await loadDevices();
  await loadAlerts();
  await loadHealthTrend();
}

async function addDevice() {
  if (isSnmpV3.value) {
    if (!addForm.value.v3_username) {
      ElMessage.warning("SNMP v3 需要填写用户名");
      return;
    }
    if (addForm.value.v3_security_level !== "noAuthNoPriv" && !addForm.value.v3_auth_password) {
      ElMessage.warning("SNMP v3 需要填写认证密码");
      return;
    }
    if (addForm.value.v3_security_level === "authPriv" && !addForm.value.v3_priv_password) {
      ElMessage.warning("SNMP v3(authPriv) 需要填写加密密码");
      return;
    }
  } else if (!addForm.value.community) {
    ElMessage.warning("SNMP v1/v2c 需要填写团体字串");
    return;
  }
  addLoading.value = true;
  try {
    await api.precheckDevice(addForm.value);
    await api.addDevice(addForm.value);
    ElMessage.success("资产添加成功");
    addVisible.value = false;
    addForm.value = defaultAddForm();
    await refreshAll();
  } catch (err) {
    ElMessage.error(getApiError(err, "添加资产失败"));
  } finally {
    addLoading.value = false;
  }
}

async function openQuickPeek(row) {
  ops.openQuickPeek(row.id);
  drawerLoading.value = true;
  selectedPort.value = null;
  portDrawerVisible.value = false;
  try {
    const detail = await api.getDeviceById(row.id);
    drawerDevice.value = detail;
    drawerPorts.value = detail?.interfaces || [];
    await loadDrawerCpuMem();
  } catch (err) {
    ElMessage.error(getApiError(err, "加载设备详情失败"));
  } finally {
    drawerLoading.value = false;
  }
}

async function loadDrawerCpuMem() {
  if (!drawerDevice.value?.id) return;
  const start = iso(drawerRange.value?.[0] || new Date(Date.now() - 24 * 3600 * 1000));
  const end = iso(drawerRange.value?.[1] || new Date());
  try {
    const [cpuRes, memRes] = await Promise.all([
      api.getHistory("cpu", drawerDevice.value.id, start, end, "1m"),
      api.getHistory("mem", drawerDevice.value.id, start, end, "1m")
    ]);
    await nextTick();
    renderCpuMemChart(cpuRes.data?.data || [], memRes.data?.data || []);
  } catch (err) {
    ElMessage.error(getApiError(err, "加载CPU/内存趋势失败"));
  }
}

async function openPortTraffic(port) {
  selectedPort.value = port;
  portDrawerVisible.value = true;
  try {
    const start = iso(drawerRange.value?.[0] || new Date(Date.now() - 24 * 3600 * 1000));
    const end = iso(drawerRange.value?.[1] || new Date());
    const res = await api.getHistory("traffic", port.id, start, end, "1m");
    portTraffic.value = res.data?.data || [];
    await nextTick();
    renderTrafficChart();
  } catch (err) {
    ElMessage.error(getApiError(err, "加载端口流量失败"));
  }
}

function renderHealthTrendChart() {
  if (!healthChartEl.value) return;
  if (!healthChart) healthChart = echarts.init(healthChartEl.value);
  const x = healthTrend.value.map((i) => fmtTime(i.ts || i.timestamp));
  const y = healthTrend.value.map((i) => Number(i.score || 0));
  healthChart.setOption({
    tooltip: { trigger: "axis" },
    grid: { left: "3%", right: "4%", bottom: "10%", containLabel: true },
    xAxis: { type: "category", data: x, boundaryGap: false, axisLabel: { rotate: x.length > 12 ? 45 : 0 } },
    yAxis: { type: "value", min: 0, max: 100 },
    series: [{ type: "line", data: y, smooth: true, areaStyle: {}, lineStyle: { width: 2 } }]
  });
}

function renderCpuMemChart(cpuData, memData) {
  if (!drawerCpuMemChartEl.value) return;
  if (!cpuMemChart) cpuMemChart = echarts.init(drawerCpuMemChartEl.value);
  const x = cpuData.map((i) => fmtTime(i.timestamp));
  cpuMemChart.setOption({
    tooltip: { trigger: "axis" },
    legend: { data: ["CPU", "内存"] },
    grid: { left: "3%", right: "4%", bottom: "10%", containLabel: true },
    xAxis: { type: "category", data: x, boundaryGap: false, axisLabel: { rotate: x.length > 12 ? 45 : 0 } },
    yAxis: { type: "value", min: 0, max: 100 },
    series: [
      { name: "CPU", type: "line", smooth: true, sampling: "average", data: cpuData.map((i) => Number(i.cpu_usage || 0)) },
      { name: "内存", type: "line", smooth: true, sampling: "average", data: memData.map((i) => Number(i.mem_usage || 0)) }
    ]
  });
}

function renderTrafficChart() {
  if (!portTrafficChartEl.value) return;
  if (!trafficChart) trafficChart = echarts.init(portTrafficChartEl.value);
  trafficChart.setOption({
    tooltip: { trigger: "axis" },
    legend: { data: ["入方向", "出方向"] },
    grid: { left: "3%", right: "4%", bottom: "10%", containLabel: true },
    xAxis: { type: "category", data: portTraffic.value.map((i) => fmtTime(i.timestamp)), boundaryGap: false, axisLabel: { rotate: portTraffic.value.length > 12 ? 45 : 0 } },
    yAxis: { type: "value", axisLabel: { formatter: (v) => formatBps(v) } },
    series: [
      { name: "入方向", type: "line", smooth: true, sampling: "average", data: portTraffic.value.map((i) => Number(i.traffic_in_bps || 0)) },
      { name: "出方向", type: "line", smooth: true, sampling: "average", data: portTraffic.value.map((i) => Number(i.traffic_out_bps || 0)) }
    ]
  });
}

function onResize() {
  healthChart?.resize();
  cpuMemChart?.resize();
  trafficChart?.resize();
}

onMounted(async () => {
  await refreshAll();
  timer = setInterval(refreshAll, 20000);
  window.addEventListener("resize", onResize);
});

onActivated(refreshAll);

onBeforeUnmount(() => {
  if (timer) clearInterval(timer);
  window.removeEventListener("resize", onResize);
  healthChart?.dispose();
  cpuMemChart?.dispose();
  trafficChart?.dispose();
});

watch(() => ops.isDrawerOpen, async (v) => {
  if (v) {
    await nextTick();
    cpuMemChart?.resize();
  }
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

    <el-card>
      <template #header><span class="text-lg font-semibold">{{ zhCN.deviceList.healthTrend30d }}</span></template>
      <el-skeleton :loading="healthTrendLoading" animated :rows="6">
        <template #default>
          <div ref="healthChartEl" style="height: 280px"></div>
        </template>
      </el-skeleton>
    </el-card>

    <section class="grid grid-cols-1 gap-5 2xl:grid-cols-[2fr,1fr]">
      <el-card>
        <template #header>
          <div class="flex flex-wrap items-center justify-between gap-2">
            <div class="flex items-center gap-2">
              <span class="text-lg font-semibold">资产总览</span>
              <el-select v-model="groupBy" class="w-[130px]">
                <el-option label="按品牌" value="brand" />
                <el-option label="按位置" value="location" />
              </el-select>
            </div>
            <div class="flex items-center gap-2">
              <el-input v-model="globalKeyword" placeholder="搜索 IP / 备注 / 端口名 / 设备名" clearable class="w-[320px]" />
              <el-button type="primary" @click="addVisible = true">添加资产</el-button>
              <el-button @click="refreshAll">刷新</el-button>
            </div>
          </div>
        </template>

        <el-skeleton :loading="loading" animated :rows="10">
          <template #default>
            <div v-for="grp in groupedDevices" :key="grp.group" class="mb-5">
              <div class="mb-2 text-sm font-semibold text-slate-600">{{ grp.group }} ({{ grp.rows.length }})</div>
              <el-table :data="grp.rows" class="np-borderless-table" size="large" @row-dblclick="openQuickPeek">
                <el-table-column label="状态" width="90">
                  <template #default="{ row }">
                    <span class="inline-block h-2.5 w-2.5 rounded-full" :class="row.status === 'online' ? 'status-dot-online' : (row.status === 'offline' ? 'bg-rose-500' : 'bg-amber-400')" />
                  </template>
                </el-table-column>
                <el-table-column prop="name" label="名称" min-width="160" />
                <el-table-column prop="ip" label="IP" min-width="160" />
                <el-table-column prop="brand" label="品牌" width="120" />
                <el-table-column prop="remark" label="备注" min-width="220" />
                <el-table-column label="操作" width="120">
                  <template #default="{ row }">
                    <el-button type="primary" text @click="openQuickPeek(row)">快速预览</el-button>
                  </template>
                </el-table-column>
              </el-table>
            </div>
          </template>
        </el-skeleton>
      </el-card>

      <LiveEventFeed :loading="feedLoading" :alerts="ops.realtimeAlerts" :severity-tag="severityTag" />
    </section>

    <el-drawer v-model="ops.isDrawerOpen" size="65%" direction="rtl" :with-header="true" title="设备快速预览" @close="ops.closeQuickPeek()">
      <el-skeleton :loading="drawerLoading" animated :rows="8">
        <template #default>
          <div class="mb-3 flex flex-wrap items-center justify-between gap-3">
            <div>
              <div class="text-lg font-semibold text-slate-900">{{ drawerDevice?.name || '-' }}</div>
              <div class="text-xs text-slate-500">{{ drawerDevice?.ip }} · {{ drawerDevice?.brand }}</div>
            </div>
            <el-date-picker
              v-model="drawerRange"
              type="datetimerange"
              range-separator="至"
              start-placeholder="开始"
              end-placeholder="结束"
              @change="loadDrawerCpuMem"
            />
          </div>

          <el-card class="mb-4">
            <template #header><span class="font-semibold">CPU / 内存</span></template>
            <div ref="drawerCpuMemChartEl" style="height: 260px"></div>
          </el-card>

          <el-card>
            <template #header><span class="font-semibold">端口列表（点击端口名查看流量）</span></template>
            <el-table :data="drawerPorts" size="small" max-height="380">
              <el-table-column prop="index" label="索引" width="90" />
              <el-table-column label="端口名" min-width="220">
                <template #default="{ row }">
                  <el-button link type="primary" @click="openPortTraffic(row)">{{ row.name }}</el-button>
                </template>
              </el-table-column>
              <el-table-column prop="remark" label="备注" min-width="180" />
            </el-table>
          </el-card>
        </template>
      </el-skeleton>
    </el-drawer>

    <el-drawer v-model="portDrawerVisible" size="50%" direction="rtl" :title="`端口流量 - ${selectedPort?.name || ''}`">
      <div ref="portTrafficChartEl" style="height: 420px"></div>
    </el-drawer>

    <el-dialog v-model="addVisible" title="添加资产" width="560">
      <el-form label-position="top">
        <el-form-item label="设备IP"><el-input v-model="addForm.ip" /></el-form-item>
        <el-form-item label="资产名称"><el-input v-model="addForm.name" /></el-form-item>
        <el-form-item label="品牌"><el-input v-model="addForm.brand" /></el-form-item>
        <el-form-item label="SNMP版本">
          <el-select v-model="addForm.snmp_version" class="w-full">
            <el-option label="v1" value="1" />
            <el-option label="v2c" value="2c" />
            <el-option label="v3" value="3" />
          </el-select>
        </el-form-item>
        <el-form-item label="SNMP端口"><el-input-number v-model="addForm.snmp_port" :min="1" :max="65535" class="w-full" /></el-form-item>
        <el-form-item v-if="!isSnmpV3" label="团体字串"><el-input v-model="addForm.community" /></el-form-item>
        <template v-else>
          <el-form-item label="v3 用户名"><el-input v-model="addForm.v3_username" /></el-form-item>
          <el-form-item label="安全级别">
            <el-select v-model="addForm.v3_security_level" class="w-full">
              <el-option label="noAuthNoPriv" value="noAuthNoPriv" />
              <el-option label="authNoPriv" value="authNoPriv" />
              <el-option label="authPriv" value="authPriv" />
            </el-select>
          </el-form-item>
          <el-form-item v-if="addForm.v3_security_level !== 'noAuthNoPriv'" label="认证协议">
            <el-select v-model="addForm.v3_auth_protocol" class="w-full">
              <el-option label="MD5" value="MD5" />
              <el-option label="SHA" value="SHA" />
            </el-select>
          </el-form-item>
          <el-form-item v-if="addForm.v3_security_level !== 'noAuthNoPriv'" label="认证密码">
            <el-input v-model="addForm.v3_auth_password" show-password />
          </el-form-item>
          <el-form-item v-if="addForm.v3_security_level === 'authPriv'" label="加密协议">
            <el-select v-model="addForm.v3_priv_protocol" class="w-full">
              <el-option label="DES" value="DES" />
              <el-option label="AES" value="AES" />
            </el-select>
          </el-form-item>
          <el-form-item v-if="addForm.v3_security_level === 'authPriv'" label="加密密码">
            <el-input v-model="addForm.v3_priv_password" show-password />
          </el-form-item>
        </template>
        <el-form-item label="备注"><el-input v-model="addForm.remark" /></el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="addVisible = false">取消</el-button>
        <el-button type="primary" :loading="addLoading" @click="addDevice">保存</el-button>
      </template>
    </el-dialog>
  </div>
</template>
