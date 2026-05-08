<script setup>
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from "vue";
import { useRouter } from "vue-router";
import { Edit } from "@element-plus/icons-vue";
import { api } from "../services/api";
import { useFeedback } from "../composables/useFeedback";

const props = defineProps({ id: { type: [String, Number], required: true } });
const router = useRouter();
const fb = useFeedback();

const loading = ref(false);
const chartLoading = ref(false);
const logsLoading = ref(false);
const recentLogs = ref([]);
const logLimit = ref(10);
const device = ref(null);
const portKeyword = ref("");
const deviceEditVisible = ref(false);
const deviceEditForm = ref({ id: null, name: "", brand: "", remark: "", maintenance_mode: false });
const showPortID = ref(false);
const showPortIndex = ref(false);
const showAdvanced = ref(["logs"]);
const diagnoseVisible = ref(false);
const diagnoseLoading = ref(false);
const diagnoseReport = ref(null);
const capability = ref(null);
const cpuMemRef = ref(null);
const storageRef = ref(null);
const terminalType = ref("ssh");
let cpuMemChart = null;
let storageChart = null;
const storageSummary = ref({
  usage: null,
  total: null,
  free: null,
  source: "-"
});
const cpuSeries = ref([]);
const memSeries = ref([]);
const storageSeries = ref([]);

const filteredPorts = computed(() => {
  const list = device.value?.interfaces || [];
  const key = portKeyword.value.trim().toLowerCase();
  if (!key) return list;
  return list.filter((p) => [String(p.id), String(p.index), p.name || "", p.remark || ""].join(" ").toLowerCase().includes(key));
});
const logsDisplay = computed(() => recentLogs.value.slice(0, logLimit.value));
const perfKpi = computed(() => {
  function calc(series) {
    if (!series.length) return { current: null, peak: null, delta: null };
    const current = Number(series[series.length - 1][1] || 0);
    const peak = Math.max(...series.map((x) => Number(x[1] || 0)));
    const prev = Number(series[0][1] || 0);
    return { current, peak, delta: current - prev };
  }
  return {
    cpu: calc(cpuSeries.value),
    mem: calc(memSeries.value),
    storage: calc(storageSeries.value)
  };
});

function interfaceStatusClass(row) {
  const raw = row?.status ?? row?.oper_status ?? row?.if_oper_status ?? row?.admin_status ?? row?.if_admin_status;
  const s = String(raw ?? "").toLowerCase();
  if (raw === 1 || s === "1" || s === "up" || s === "online") return "status-dot-online";
  if (raw === 2 || s === "2" || s === "down" || s === "offline") return "status-dot-offline";
  return "status-dot-unknown";
}

function deviceStatusClass() {
  const s = String(device.value?.status || "").toLowerCase();
  if (s === "online" || s === "up") return "status-dot-online";
  if (s === "offline" || s === "down") return "status-dot-offline";
  return "status-dot-unknown";
}

async function loadDevice() {
  loading.value = true;
  try {
    device.value = await api.getDeviceById(props.id);
    if (!device.value) return;
    await Promise.all([renderCpuMem(), renderStorage(), loadLogs(), loadCapability()]);
  } catch (err) {
    fb.apiError(err, "加载设备详情失败");
  } finally {
    loading.value = false;
  }
}

async function loadCapability() {
  if (!props.id) return;
  try {
    const res = await api.getDeviceCapabilities(props.id);
    capability.value = res.data || null;
  } catch {
    capability.value = null;
  }
}

async function loadLogs() {
  if (!props.id) return;
  logsLoading.value = true;
  try {
    const res = await api.getDeviceLogs(props.id);
    recentLogs.value = res.data || [];
  } finally {
    logsLoading.value = false;
  }
}

function initCpuChart() {
  if (!cpuMemChart) return;
  cpuMemChart.setOption({
    animation: false,
    grid: { left: 45, right: 20, top: 34, bottom: 30 },
    tooltip: { trigger: "axis", axisPointer: { type: "line", animation: false } },
    legend: { data: ["CPU利用率", "内存利用率"], top: 4 },
    xAxis: { type: "time" },
    yAxis: { type: "value", max: 100, axisLabel: { formatter: (v) => `${v}%` } },
    series: [
      { name: "CPU利用率", type: "line", showSymbol: false, sampling: "lttb", progressive: 2000, data: [] },
      { name: "内存利用率", type: "line", showSymbol: false, sampling: "lttb", progressive: 2000, data: [] }
    ]
  }, true);
}

function initStorageChart() {
  if (!storageChart) return;
  storageChart.setOption({
    animation: false,
    grid: { left: 45, right: 20, top: 34, bottom: 30 },
    tooltip: { trigger: "axis", axisPointer: { type: "line", animation: false } },
    legend: { data: ["存储利用率"], top: 4 },
    xAxis: { type: "time" },
    yAxis: { type: "value", max: 100, axisLabel: { formatter: (v) => `${v}%` } },
    series: [{ name: "存储利用率", type: "line", showSymbol: false, sampling: "lttb", progressive: 2000, data: [] }]
  }, true);
}

async function renderCpuMem() {
  if (!device.value || !cpuMemChart) return;
  chartLoading.value = true;
  try {
    const end = new Date();
    const start = new Date(end.getTime() - 24 * 3600 * 1000);
    const interval = "1m";
    const [cpuRes, memRes] = await Promise.all([
      api.getHistory("cpu", device.value.id, start.toISOString(), end.toISOString(), interval),
      api.getHistory("mem", device.value.id, start.toISOString(), end.toISOString(), interval)
    ]);

    const cpuData = (cpuRes.data.data || []).map((p) => [new Date(p.timestamp).getTime(), Number(p.cpu_usage || 0)]);
    const memData = (memRes.data.data || []).map((p) => [new Date(p.timestamp).getTime(), Number(p.mem_usage || 0)]);
    cpuSeries.value = cpuData;
    memSeries.value = memData;
    const hasData = cpuData.length || memData.length;

    cpuMemChart.setOption({
      graphic: hasData ? [] : [{
        type: "text",
        left: "center",
        top: "middle",
        style: { text: "当前时间范围暂无 CPU/内存 数据", fill: "#94a3b8", fontSize: 14 }
      }],
      series: [
        { name: "CPU利用率", data: cpuData },
        { name: "内存利用率", data: memData }
      ]
    });
  } catch (err) {
    fb.apiError(err, "加载性能图表失败");
  } finally {
    chartLoading.value = false;
  }
}

async function renderStorage() {
  if (!device.value || !storageChart) return;
  storageSummary.value.usage = Number.isFinite(Number(device.value.storage_usage ?? device.value.disk_usage ?? device.value.flash_usage))
    ? Number(device.value.storage_usage ?? device.value.disk_usage ?? device.value.flash_usage)
    : null;
  storageSummary.value.total = Number.isFinite(Number(device.value.storage_total ?? device.value.disk_total))
    ? Number(device.value.storage_total ?? device.value.disk_total)
    : null;
  storageSummary.value.free = Number.isFinite(Number(device.value.storage_free ?? device.value.disk_free))
    ? Number(device.value.storage_free ?? device.value.disk_free)
    : null;
  storageSummary.value.source = storageSummary.value.usage !== null ? "设备快照字段" : "-";

  try {
    const end = new Date();
    const start = new Date(end.getTime() - 24 * 3600 * 1000);
    const interval = "1m";
    const res = await api.getHistory("storage", device.value.id, start.toISOString(), end.toISOString(), interval);
    const points = (res.data.data || []).map((p) => {
      const usage = Number(p.storage_usage ?? p.disk_usage ?? p.usage ?? NaN);
      return [new Date(p.timestamp).getTime(), Number.isFinite(usage) ? usage : 0];
    }).filter((x) => Number.isFinite(x[0]));
    storageSeries.value = points;

    if (points.length > 0) {
      storageSummary.value.usage = points[points.length - 1][1];
      storageSummary.value.source = "历史指标";
    }

    storageChart.setOption({
      graphic: points.length ? [] : [{
        type: "text",
        left: "center",
        top: "middle",
        style: { text: "当前时间范围暂无存储数据", fill: "#94a3b8", fontSize: 14 }
      }],
      series: [{ name: "存储利用率", data: points }]
    });
  } catch {
    storageSeries.value = [];
    storageChart.setOption({
      graphic: [{
        type: "text",
        left: "center",
        top: "middle",
        style: { text: "存储监控暂未接入该设备模板", fill: "#94a3b8", fontSize: 14 }
      }],
      series: [{ name: "存储利用率", data: [] }]
    });
  }
}

function openPort(port) {
  router.push({
    path: `/port/${port.id}`,
    query: {
      deviceId: String(device.value.id),
      deviceIp: device.value.ip,
      portName: port.name,
      portRemark: port.remark || ""
    }
  });
}

function buildTerminalUrl() {
  const ip = String(device.value?.ip || "").trim();
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

function openDeviceEdit() {
  if (!device.value) return;
  deviceEditForm.value = {
    id: device.value.id,
    name: device.value.name || "",
    brand: device.value.brand || "",
    remark: device.value.remark || "",
    maintenance_mode: Boolean(device.value.maintenance_mode)
  };
  deviceEditVisible.value = true;
}

async function saveDeviceEdit() {
  try {
    await api.updateDevice(deviceEditForm.value.id, {
      name: deviceEditForm.value.name || "",
      brand: deviceEditForm.value.brand || "",
      remark: deviceEditForm.value.remark || "",
      maintenance_mode: Boolean(deviceEditForm.value.maintenance_mode)
    });
    fb.success("设备信息已更新");
    deviceEditVisible.value = false;
    await loadDevice();
  } catch (err) {
    fb.apiError(err, "保存设备信息失败");
  }
}

async function runDiagnosis() {
  diagnoseLoading.value = true;
  try {
    const res = await api.diagnoseDevice(props.id);
    diagnoseReport.value = res.data || null;
    diagnoseVisible.value = true;
  } catch (err) {
    fb.apiError(err, "执行诊断失败");
  } finally {
    diagnoseLoading.value = false;
  }
}

async function exportDiagnosis(format) {
  try {
    const res = await api.exportDiagnosis(props.id, format);
    const blobUrl = URL.createObjectURL(new Blob([res.data]));
    const a = document.createElement("a");
    a.href = blobUrl;
    a.download = `netpulse_diagnose_device_${props.id}.${format}`;
    a.click();
    URL.revokeObjectURL(blobUrl);
  } catch (err) {
    fb.apiError(err, "导出诊断报告失败");
  }
}

function resizeChart() {
  cpuMemChart?.resize();
  storageChart?.resize();
}

onMounted(async () => {
  await nextTick();
  const m = await import("echarts");
  cpuMemChart = m.init(cpuMemRef.value);
  storageChart = m.init(storageRef.value);
  initCpuChart();
  initStorageChart();
  await loadDevice();
  window.addEventListener("resize", resizeChart);
});

onBeforeUnmount(() => {
  window.removeEventListener("resize", resizeChart);
  cpuMemChart?.dispose();
  storageChart?.dispose();
});

watch(
  () => props.id,
  async () => {
    await loadDevice();
  }
);
</script>

<template>
  <div class="space-y-5" v-loading="loading">
    <el-breadcrumb separator=">">
      <el-breadcrumb-item :to="{ path: '/' }">资产</el-breadcrumb-item>
      <el-breadcrumb-item>{{ device?.ip || `设备-${props.id}` }}</el-breadcrumb-item>
    </el-breadcrumb>

    <el-card>
      <template #header>
        <div class="flex items-center justify-between">
          <span class="text-base font-semibold">设备基础信息</span>
          <div class="flex items-center gap-2">
            <el-select v-model="terminalType" class="w-[180px]">
              <el-option label="系统默认 SSH" value="ssh" />
              <el-option label="Termius" value="termius" />
              <el-option label="SecureCRT" value="securecrt" />
              <el-option label="自定义模板" value="custom" />
            </el-select>
            <el-button type="primary" plain @click="openTerminal">连接设备终端</el-button>
            <el-button type="primary" plain :icon="Edit" @click="openDeviceEdit">编辑设备名称/备注</el-button>
          </div>
        </div>
      </template>
      <div v-if="device" class="mb-3 flex items-center gap-3">
        <span class="inline-block align-middle" :class="deviceStatusClass()" />
        <el-tag :type="String(device?.status || '').toLowerCase() === 'online' ? 'success' : (String(device?.status || '').toLowerCase() === 'offline' ? 'danger' : 'warning')">
          {{ String(device?.status || 'unknown').toUpperCase() }}
        </el-tag>
        <el-tag type="info">端口总数 {{ (device.interfaces || []).length }}</el-tag>
        <el-tag type="primary">维护模式 {{ device.maintenance_mode ? "开启" : "关闭" }}</el-tag>
      </div>
      <div v-if="device" class="grid grid-cols-1 gap-3 md:grid-cols-4">
        <div><div class="text-xs text-slate-500">设备 ID</div><div class="font-semibold">{{ device.id }}</div></div>
        <div><div class="text-xs text-slate-500">名称</div><div class="font-semibold">{{ device.name || '-' }}</div></div>
        <div><div class="text-xs text-slate-500">IP</div><div class="font-semibold">{{ device.ip }}</div></div>
        <div><div class="text-xs text-slate-500">品牌</div><div class="font-semibold">{{ device.brand }}</div></div>
        <div><div class="text-xs text-slate-500">备注</div><div class="font-semibold">{{ device.remark || '-' }}</div></div>
      </div>
      <el-empty v-else description="设备不存在" />
    </el-card>

    <el-card>
      <template #header>
        <div class="flex flex-wrap items-center justify-between gap-2">
          <span class="text-base font-semibold">端口列表</span>
          <div class="flex items-center gap-2">
            <el-input v-model="portKeyword" placeholder="按 id/index/名称/备注搜索" clearable class="w-[320px]" />
            <el-checkbox v-model="showPortID">显示ID</el-checkbox>
            <el-checkbox v-model="showPortIndex">显示索引</el-checkbox>
          </div>
        </div>
      </template>

      <el-skeleton :loading="loading" animated :rows="8">
        <template #default>
          <el-table :data="filteredPorts" class="np-borderless-table">
            <el-table-column label="状态" width="90">
              <template #default="{ row }">
                <span class="inline-block align-middle" :class="interfaceStatusClass(row)" />
              </template>
            </el-table-column>
            <el-table-column v-if="showPortID" prop="id" label="ID" width="90" />
            <el-table-column v-if="showPortIndex" prop="index" label="索引" width="100" />
            <el-table-column label="端口名称" min-width="220">
              <template #default="{ row }">
                <el-button link type="primary" @click="openPort(row)">{{ row.name || `ifIndex-${row.index}` }}</el-button>
              </template>
            </el-table-column>
            <el-table-column prop="remark" label="备注" min-width="220" />
          </el-table>
          <el-empty v-if="!filteredPorts.length" description="无匹配端口，请调整搜索条件" :image-size="72" />
        </template>
      </el-skeleton>
    </el-card>

    <el-card>
      <template #header>
        <span class="text-base font-semibold">CPU / 内存实时利用率（24h）</span>
      </template>
      <div class="mb-3 grid grid-cols-1 gap-2 text-sm md:grid-cols-3">
        <div>CPU: 当前 <b>{{ Number.isFinite(perfKpi.cpu.current) ? `${perfKpi.cpu.current.toFixed(1)}%` : '-' }}</b> / 峰值 <b>{{ Number.isFinite(perfKpi.cpu.peak) ? `${perfKpi.cpu.peak.toFixed(1)}%` : '-' }}</b> / 24h变化 <b>{{ Number.isFinite(perfKpi.cpu.delta) ? `${perfKpi.cpu.delta.toFixed(1)}%` : '-' }}</b></div>
        <div>内存: 当前 <b>{{ Number.isFinite(perfKpi.mem.current) ? `${perfKpi.mem.current.toFixed(1)}%` : '-' }}</b> / 峰值 <b>{{ Number.isFinite(perfKpi.mem.peak) ? `${perfKpi.mem.peak.toFixed(1)}%` : '-' }}</b> / 24h变化 <b>{{ Number.isFinite(perfKpi.mem.delta) ? `${perfKpi.mem.delta.toFixed(1)}%` : '-' }}</b></div>
        <div>存储: 当前 <b>{{ Number.isFinite(perfKpi.storage.current) ? `${perfKpi.storage.current.toFixed(1)}%` : '-' }}</b> / 峰值 <b>{{ Number.isFinite(perfKpi.storage.peak) ? `${perfKpi.storage.peak.toFixed(1)}%` : '-' }}</b> / 24h变化 <b>{{ Number.isFinite(perfKpi.storage.delta) ? `${perfKpi.storage.delta.toFixed(1)}%` : '-' }}</b></div>
      </div>
      <div ref="cpuMemRef" class="h-[180px] w-full" v-loading="chartLoading"></div>
    </el-card>

    <el-card>
      <template #header>
        <div class="flex items-center justify-between">
          <span class="text-base font-semibold">存储空间监控（24h）</span>
          <el-tag :type="Number(storageSummary.usage || 0) >= 85 ? 'danger' : 'success'">
            当前: {{ Number.isFinite(Number(storageSummary.usage)) ? `${Number(storageSummary.usage).toFixed(1)}%` : '-' }}
          </el-tag>
        </div>
      </template>
      <div class="mb-3 grid grid-cols-1 gap-2 text-sm md:grid-cols-4">
        <div>数据来源：<b>{{ storageSummary.source }}</b></div>
        <div>使用率：<b>{{ Number.isFinite(Number(storageSummary.usage)) ? `${Number(storageSummary.usage).toFixed(1)}%` : "-" }}</b></div>
        <div>总容量：<b>{{ Number.isFinite(Number(storageSummary.total)) ? Number(storageSummary.total).toFixed(0) : "-" }}</b></div>
        <div>剩余容量：<b>{{ Number.isFinite(Number(storageSummary.free)) ? Number(storageSummary.free).toFixed(0) : "-" }}</b></div>
      </div>
      <div ref="storageRef" class="h-[180px] w-full"></div>
    </el-card>

    <el-card>
      <el-collapse v-model="showAdvanced" class="border-none">
        <el-collapse-item title="设备能力矩阵" name="capability">
          <div v-if="capability" class="grid grid-cols-1 gap-3 md:grid-cols-4 text-sm">
            <div>SNMP版本：<b>{{ capability.snmp_version || "-" }}</b></div>
            <div>CPU采集：<b>{{ capability.supports_cpu ? "支持" : "不支持" }}</b></div>
            <div>内存采集：<b>{{ capability.supports_memory ? "支持" : "不支持" }}</b></div>
            <div>端口流量：<b>{{ capability.supports_if_traffic ? "支持" : "不支持" }}</b></div>
            <div>存储采集：<b>{{ Number.isFinite(Number(storageSummary.usage)) ? "支持" : "待接入模板" }}</b></div>
          </div>
          <div v-else class="text-sm text-slate-500">暂无能力数据，等待设备完成一次轮询后显示。</div>
        </el-collapse-item>

        <el-collapse-item name="diagnose">
          <template #title>
            <div class="flex w-full items-center justify-between pr-3">
              <span>设备诊断</span>
              <el-button type="primary" size="small" :loading="diagnoseLoading" @click.stop="runDiagnosis">一键自助排查</el-button>
            </div>
          </template>
          <p class="text-sm text-slate-600">自动检查 SNMP 参数、网络连通、即时采集、最近入库与错误日志，并给出可能原因。</p>
        </el-collapse-item>

        <el-collapse-item name="logs">
          <template #title>
            <div class="flex w-full items-center justify-between pr-3">
              <span>设备日志（默认展示10条）</span>
              <div class="flex items-center gap-2" @click.stop>
                <el-select v-model="logLimit" class="w-[140px]">
                  <el-option :value="10" label="10条" />
                  <el-option :value="20" label="20条" />
                  <el-option :value="50" label="50条" />
                  <el-option :value="100" label="100条" />
                </el-select>
                <el-button @click="loadLogs" :loading="logsLoading">刷新日志</el-button>
              </div>
            </div>
          </template>
          <el-table :data="logsDisplay" class="np-borderless-table" height="280" v-loading="logsLoading">
            <el-table-column prop="created_at" label="时间" width="190" />
            <el-table-column prop="level" label="级别" width="100" />
            <el-table-column prop="message" label="内容" min-width="480" />
          </el-table>
        </el-collapse-item>
      </el-collapse>
    </el-card>

    <el-dialog v-model="deviceEditVisible" title="编辑设备信息" width="520">
      <el-form label-position="top">
        <el-form-item label="设备名称"><el-input v-model="deviceEditForm.name" /></el-form-item>
        <el-form-item label="品牌"><el-input v-model="deviceEditForm.brand" /></el-form-item>
        <el-form-item label="备注"><el-input v-model="deviceEditForm.remark" type="textarea" :rows="4" /></el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="deviceEditVisible = false">取消</el-button>
        <el-button type="primary" @click="saveDeviceEdit">保存</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="diagnoseVisible" title="设备自助排查报告" width="860">
      <div v-if="diagnoseReport" class="space-y-3">
        <div class="grid grid-cols-1 gap-2 md:grid-cols-2">
          <div><span class="text-slate-500">设备:</span> {{ diagnoseReport.device_ip }} (ID: {{ diagnoseReport.device_id }})</div>
          <div><span class="text-slate-500">总体状态:</span> <span class="font-semibold">{{ diagnoseReport.overall_status }}</span></div>
          <div class="md:col-span-2"><span class="text-slate-500">可能原因:</span> {{ diagnoseReport.likely_cause }}</div>
        </div>
        <el-table :data="diagnoseReport.checks || []" class="np-borderless-table">
          <el-table-column prop="name" label="检查项" min-width="180" />
          <el-table-column label="结果" width="100">
            <template #default="{ row }">
              <el-tag :type="row.status === 'pass' ? 'success' : (row.status === 'warn' ? 'warning' : 'danger')">{{ row.status }}</el-tag>
            </template>
          </el-table-column>
          <el-table-column prop="message" label="说明" min-width="420" />
        </el-table>
      </div>
      <template #footer>
        <el-button @click="exportDiagnosis('txt')">导出 TXT</el-button>
        <el-button @click="exportDiagnosis('json')">导出 JSON</el-button>
        <el-button type="primary" @click="diagnoseVisible = false">关闭</el-button>
      </template>
    </el-dialog>
  </div>
</template>
