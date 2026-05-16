<script setup>
import * as echarts from "echarts";
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from "vue";
import { ElMessageBox } from "element-plus";
import { useRouter } from "vue-router";
import { api, getApiError } from "../services/api";
import { useOpsStore } from "../stores/ops";
import { useFeedback } from "../composables/useFeedback";
import { statusClass, statusLabel } from "../utils/status";

const ops = useOpsStore();
const router = useRouter();
const fb = useFeedback();

const loading = ref(false);
const devices = ref([]);
const globalKeyword = ref("");
const groupBy = ref("brand");
const statusFilter = ref("all");
const manageMode = ref(false);
const editMode = ref(localStorage.getItem("np_edit_mode") === "1");
const selectedRows = ref([]);
const visibleCols = ref({
  status: true, name: true, ip: true, brand: true, type: true, cpu: true, uptime: true, remark: true
});
const templates = ref([]);
const selectedTemplateId = ref(null);
const brandOptions = ["Huawei", "H3C", "Cisco", "Ruijie", "Juniper", "Other"];
const tierOptions = [
  { label: "核心层", value: "core" },
  { label: "汇聚层", value: "aggregation" },
  { label: "接入层", value: "access" }
];
const viewPresetName = ref("");
const viewPresets = ref(JSON.parse(localStorage.getItem("np_asset_view_presets") || "[]"));
const activePreset = ref("");
const pendingDelete = ref(null);
let pendingDeleteTimer = null;

const addVisible = ref(false);
const addLoading = ref(false);
const importVisible = ref(false);
const importLoading = ref(false);
const importCSV = ref("ip,name,brand,community,snmp_version,remark,snmp_port,poll_interval_sec,cpu_threshold,mem_threshold\n172.24.1.10,Core-SW-A,H3C,public,2c,核心交换机A,161,60,90,90");
const editVisible = ref(false);
const editLoading = ref(false);
const runtimeDefaults = ref({
  poll_interval_sec: 60,
  alert_cpu_threshold: 90,
  alert_mem_threshold: 90
});
const editForm = ref({
  id: null, name: "", brand: "", remark: "", maintenance_mode: false,
  device_tier: "access",
  poll_interval_sec: 60, cpu_threshold: 90, mem_threshold: 90
});
const defaultAddForm = () => ({
  ip: "",
  name: "",
  template_id: null,
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
  v3_priv_password: "",
  device_tier: "access",
  poll_interval_sec: Number(runtimeDefaults.value.poll_interval_sec || 60),
  cpu_threshold: Number(runtimeDefaults.value.alert_cpu_threshold || 90),
  mem_threshold: Number(runtimeDefaults.value.alert_mem_threshold || 90)
});
const addForm = ref(defaultAddForm());
const isSnmpV3 = computed(() => String(addForm.value.snmp_version) === "3");
const autoTemplateHint = ref(null);

const drawerLoading = ref(false);
const drawerDevice = ref(null);
const drawerPorts = ref([]);
const drawerCpuMemChartEl = ref(null);
let cpuMemChart = null;

const filteredDevices = computed(() => {
  const kw = globalKeyword.value.trim().toLowerCase();
  const byStatus = devices.value.filter((d) => statusFilter.value === "all" ? true : String(d.status || "").toLowerCase() === statusFilter.value);
  if (!kw) return byStatus;
  return byStatus.filter((d) => {
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

function deviceStatusClass(row) {
  return statusClass(row);
}

function iso(v) {
  return new Date(v).toISOString();
}

function fmtTime(v) {
  return new Date(v).toLocaleString();
}

async function loadDevices() {
  loading.value = true;
  try {
    const res = await api.listDevices();
    devices.value = (res.data || []).map((x) => ({ ...x, location: x.location || "" }));
  } catch (err) {
    fb.apiError(err, "加载资产失败");
  } finally {
    loading.value = false;
  }
}

async function loadTemplates() {
  try {
    const res = await api.listTemplates();
    templates.value = res.data || [];
  } catch {
    templates.value = [];
  }
}

async function loadRuntimeDefaults() {
  try {
    const res = await api.getRuntimeSettings();
    const x = res?.data || {};
    runtimeDefaults.value = {
      poll_interval_sec: Math.max(5, Number(x.poll_interval_access_sec || 120)),
      alert_cpu_threshold: Math.max(1, Math.min(100, Number(x.alert_cpu_threshold || 90))),
      alert_mem_threshold: Math.max(1, Math.min(100, Number(x.alert_mem_threshold || 90)))
    };
  } catch {
    runtimeDefaults.value = { poll_interval_sec: 60, alert_cpu_threshold: 90, alert_mem_threshold: 90 };
  }
}

function applyTemplateById() {
  const id = Number(selectedTemplateId.value || 0);
  if (!id) return;
  const t = templates.value.find((x) => Number(x.id) === id);
  if (!t) return;
  addForm.value.brand = t.brand || addForm.value.brand;
  addForm.value.template_id = t.id || null;
  addForm.value.snmp_version = t.snmp_version || addForm.value.snmp_version;
  addForm.value.snmp_port = Number(t.snmp_port || addForm.value.snmp_port || 161);
  addForm.value.community = t.community || addForm.value.community;
  addForm.value.v3_username = t.v3_username || "";
  addForm.value.v3_security_level = t.v3_security_level || "noAuthNoPriv";
  addForm.value.v3_auth_protocol = t.v3_auth_protocol || "SHA";
  addForm.value.v3_priv_protocol = t.v3_priv_protocol || "AES";
}

async function addDevice() {
  if (!editMode.value) return fb.warn("当前为只读模式，请先在左侧开启编辑模式");
  if (isSnmpV3.value) {
    if (!addForm.value.v3_username) return fb.warn("参数不完整", "SNMP v3 需要填写用户名");
    if (addForm.value.v3_security_level !== "noAuthNoPriv" && !addForm.value.v3_auth_password) return fb.warn("参数不完整", "SNMP v3 需要填写认证密码");
    if (addForm.value.v3_security_level === "authPriv" && !addForm.value.v3_priv_password) return fb.warn("参数不完整", "SNMP v3(authPriv) 需要填写加密密码");
  } else if (!addForm.value.community) {
    return fb.warn("参数不完整", "SNMP v1/v2c 需要填写团体字串");
  }
  addLoading.value = true;
  try {
    const pre = await api.precheckDevice(addForm.value);
    const suggested = pre?.data?.suggested_template || null;
    if ((!addForm.value.template_id || Number(addForm.value.template_id) <= 0) && suggested?.id) {
      addForm.value.template_id = Number(suggested.id);
      selectedTemplateId.value = Number(suggested.id);
      applyTemplateById();
      autoTemplateHint.value = suggested;
      fb.info(`已自动匹配模板：${suggested.name}（匹配分 ${suggested.matchScore}）`);
      // Re-run precheck with applied template params to ensure final submit consistency.
      await api.precheckDevice(addForm.value);
    }
    const addRes = await api.addDevice(addForm.value);
    const applied = addRes?.data?.auto_template;
    if (applied?.name) {
      fb.success(`资产添加成功（自动套用模板：${applied.name}）`);
    } else {
      fb.success("资产添加成功");
    }
    addVisible.value = false;
    addForm.value = defaultAddForm();
    selectedTemplateId.value = null;
    autoTemplateHint.value = null;
    await loadDevices();
  } catch (err) {
    fb.apiError(err, "添加资产失败");
  } finally {
    addLoading.value = false;
  }
}

async function removeDevice(row) {
  if (!editMode.value) return fb.warn("当前为只读模式，请先在左侧开启编辑模式");
  try {
    await ElMessageBox.confirm(`确认删除资产 ${row.name || row.ip} 吗？`, "删除确认", { type: "warning" });
    scheduleDelete(row);
  } catch (err) {
    if (err !== "cancel") fb.apiError(err, "删除资产失败");
  }
}

function scheduleDelete(row) {
  if (pendingDeleteTimer) clearTimeout(pendingDeleteTimer);
  pendingDelete.value = { ...row, expireAt: Date.now() + 5000 };
  fb.info("删除已进入5秒缓冲，可撤销");
  pendingDeleteTimer = setTimeout(async () => {
    try {
      await api.deleteDevice(row.id);
      fb.success(`资产 ${row.name || row.ip} 已删除`);
      await loadDevices();
    } catch (err) {
      fb.apiError(err, "删除资产失败");
    } finally {
      pendingDelete.value = null;
      pendingDeleteTimer = null;
    }
  }, 5000);
}

function undoDelete() {
  if (pendingDeleteTimer) clearTimeout(pendingDeleteTimer);
  pendingDeleteTimer = null;
  pendingDelete.value = null;
  fb.success("已撤销删除");
}

async function bulkRemove() {
  if (!editMode.value) return fb.warn("当前为只读模式，请先在左侧开启编辑模式");
  if (!selectedRows.value.length) return fb.warn("请先选择资产");
  let ok = 0;
  for (const row of selectedRows.value) {
    try {
      await api.deleteDevice(row.id);
      ok += 1;
    } catch {
      // ignore per-row errors
    }
  }
  fb.info("批量删除完成", `${ok}/${selectedRows.value.length}`);
  selectedRows.value = [];
  await loadDevices();
}

function openEditDevice(row) {
  editForm.value = {
    id: row.id,
    name: row.name || "",
    brand: row.brand || "",
    remark: row.remark || "",
    maintenance_mode: Boolean(row.maintenance_mode),
    device_tier: row.device_tier || "access",
    poll_interval_sec: Math.max(0, Number(row.poll_interval_sec || runtimeDefaults.value.poll_interval_sec || 60)),
    cpu_threshold: Math.max(0, Number(row.cpu_threshold || runtimeDefaults.value.alert_cpu_threshold || 90)),
    mem_threshold: Math.max(0, Number(row.mem_threshold || runtimeDefaults.value.alert_mem_threshold || 90))
  };
  editVisible.value = true;
}

async function saveEditDevice() {
  if (!editMode.value) return fb.warn("当前为只读模式，请先在左侧开启编辑模式");
  if (!editForm.value.id) return;
  editLoading.value = true;
  try {
    await api.updateDevice(editForm.value.id, {
      name: editForm.value.name || "",
      brand: editForm.value.brand || "",
      remark: editForm.value.remark || "",
      maintenance_mode: Boolean(editForm.value.maintenance_mode),
      device_tier: editForm.value.device_tier || "access",
      poll_interval_sec: Math.max(0, Number(editForm.value.poll_interval_sec || 0)),
      cpu_threshold: Math.max(0, Number(editForm.value.cpu_threshold || 0)),
      mem_threshold: Math.max(0, Number(editForm.value.mem_threshold || 0))
    });
    fb.success("资产信息已更新");
    editVisible.value = false;
    await loadDevices();
  } catch (err) {
    fb.apiError(err, "更新资产失败");
  } finally {
    editLoading.value = false;
  }
}

async function importDevices() {
  if (!editMode.value) return fb.warn("当前为只读模式，请先在左侧开启编辑模式");
  importLoading.value = true;
  try {
    const txt = String(importCSV.value || "").trim();
    if (!txt) return fb.warn("请先粘贴CSV内容");
    const res = await api.importDevicesCSV(txt);
    fb.success("批量导入完成", `成功导入 ${Number(res?.data?.created || 0)} 台`);
    importVisible.value = false;
    await loadDevices();
  } catch (err) {
    fb.apiError(err, "批量导入失败");
  } finally {
    importLoading.value = false;
  }
}

function downloadImportTemplate() {
  const sample = [
    "ip,name,brand,community,snmp_version,remark,snmp_port,poll_interval_sec,cpu_threshold,mem_threshold",
    "172.24.1.10,Core-SW-A,H3C,public,2c,核心交换机A,161,60,90,90",
    "172.24.1.11,Agg-SW-B,Huawei,public,2c,汇聚交换机B,161,120,85,88"
  ].join("\n");
  const blob = new Blob([sample], { type: "text/csv;charset=utf-8" });
  const a = document.createElement("a");
  a.href = URL.createObjectURL(blob);
  a.download = "netpulse_devices_import_template.csv";
  a.click();
  URL.revokeObjectURL(a.href);
}

function openDeviceDetail(row) {
  if (!row?.id) return;
  router.push(`/device/${row.id}`);
}

function saveCurrentPreset() {
  const name = String(viewPresetName.value || "").trim();
  if (!name) return fb.warn("请输入视图名称");
  const preset = {
    name,
    data: {
      groupBy: groupBy.value,
      statusFilter: statusFilter.value,
      visibleCols: { ...visibleCols.value },
      manageMode: manageMode.value
    }
  };
  viewPresets.value = [preset, ...viewPresets.value.filter((x) => x.name !== name)].slice(0, 20);
  localStorage.setItem("np_asset_view_presets", JSON.stringify(viewPresets.value));
  viewPresetName.value = "";
  fb.success("视图模板已保存");
}

function applyPreset(name) {
  const p = viewPresets.value.find((x) => x.name === name);
  if (!p) return;
  activePreset.value = name;
  groupBy.value = p.data.groupBy || "brand";
  statusFilter.value = p.data.statusFilter || "all";
  visibleCols.value = { ...visibleCols.value, ...(p.data.visibleCols || {}) };
  manageMode.value = Boolean(p.data.manageMode);
}

async function openQuickPeek(row) {
  ops.openQuickPeek(row.id);
  drawerLoading.value = true;
  try {
    const detail = await api.getDeviceById(row.id);
    drawerDevice.value = detail;
    drawerPorts.value = detail?.interfaces || [];
    await loadDrawerCpuMem();
  } catch (err) {
    fb.apiError(err, "加载设备详情失败");
  } finally {
    drawerLoading.value = false;
  }
}

async function loadDrawerCpuMem() {
  if (!drawerDevice.value?.id) return;
  const endTime = new Date();
  const startTime = new Date(endTime.getTime() - 24 * 3600 * 1000);
  try {
    const [cpuRes, memRes] = await Promise.all([
      api.getHistory("cpu", drawerDevice.value.id, iso(startTime), iso(endTime), "1m"),
      api.getHistory("mem", drawerDevice.value.id, iso(startTime), iso(endTime), "1m")
    ]);
    await nextTick();
    renderCpuMemChart(cpuRes.data?.data || [], memRes.data?.data || []);
  } catch (err) {
    fb.apiError(err, "加载CPU/内存趋势失败");
  }
}

function openPortTraffic(port) {
  if (!drawerDevice.value?.id) return;
  router.push({
    path: `/port/${port.id}`,
    query: {
      deviceId: String(drawerDevice.value.id),
      deviceIp: drawerDevice.value.ip,
      portName: port.name,
      portBaseName: port.raw_name || port.name,
      portRemark: port.remark || ""
    }
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

function onResize() {
  cpuMemChart?.resize();
}

onMounted(async () => {
  await Promise.all([loadRuntimeDefaults(), loadDevices(), loadTemplates()]);
  addForm.value = defaultAddForm();
  window.addEventListener("np-edit-mode", onEditModeEvent);
  window.addEventListener("resize", onResize);
});

onBeforeUnmount(() => {
  window.removeEventListener("np-edit-mode", onEditModeEvent);
  window.removeEventListener("resize", onResize);
  cpuMemChart?.dispose();
  if (pendingDeleteTimer) clearTimeout(pendingDeleteTimer);
});

function onEditModeEvent(e) {
  editMode.value = Boolean(e?.detail?.enabled);
  if (!editMode.value) manageMode.value = false;
}

watch(() => ops.isDrawerOpen, async (v) => {
  if (v) {
    await nextTick();
    cpuMemChart?.resize();
  }
});
watch(editMode, (v) => {
  if (!v) manageMode.value = false;
});
</script>

<template>
  <div class="space-y-5">
    <el-card class="np-toolbar-card">
      <template #header>
        <div class="flex flex-wrap items-center justify-between gap-2">
          <div class="flex items-center gap-2 np-toolbar-right">
            <span class="text-lg font-semibold">资产管理</span>
            <el-select v-model="groupBy" class="w-[130px]">
              <el-option label="按品牌" value="brand" />
              <el-option label="按位置" value="location" />
            </el-select>
            <el-select v-model="statusFilter" class="w-[120px]">
              <el-option label="全部状态" value="all" />
              <el-option label="在线" value="online" />
              <el-option label="离线" value="offline" />
              <el-option label="未知" value="unknown" />
            </el-select>
            <el-select v-model="activePreset" class="w-[140px]" placeholder="视图模板" @change="applyPreset">
              <el-option v-for="p in viewPresets" :key="p.name" :label="p.name" :value="p.name" />
            </el-select>
          </div>
          <div class="flex items-center gap-2">
            <el-input v-model="globalKeyword" placeholder="搜索 IP / 名称 / 备注 / 端口名" clearable class="w-[320px]" />
            <el-input v-model="viewPresetName" placeholder="保存当前视图为..." class="w-[180px]" />
            <el-button @click="saveCurrentPreset">保存视图</el-button>
            <el-popover trigger="click" placement="bottom" width="240">
              <template #reference><el-button>列设置</el-button></template>
              <div class="grid grid-cols-2 gap-2">
                <el-checkbox v-model="visibleCols.status">状态</el-checkbox>
                <el-checkbox v-model="visibleCols.name">名称</el-checkbox>
                <el-checkbox v-model="visibleCols.ip">IP</el-checkbox>
                <el-checkbox v-model="visibleCols.brand">品牌</el-checkbox>
                <el-checkbox v-model="visibleCols.type">类型</el-checkbox>
                <el-checkbox v-model="visibleCols.cpu">CPU</el-checkbox>
                <el-checkbox v-model="visibleCols.uptime">运行时长</el-checkbox>
                <el-checkbox v-model="visibleCols.remark">备注</el-checkbox>
              </div>
            </el-popover>
            <el-segmented
              v-model="manageMode"
              :disabled="!editMode"
              :options="[
                { label: '查看模式', value: false },
                { label: '管理模式', value: true }
              ]"
            />
            <el-button v-if="editMode" plain @click="importVisible = true">批量导入</el-button>
            <el-button v-if="manageMode" type="danger" plain @click="bulkRemove">批量删除</el-button>
            <el-button v-if="manageMode" type="primary" @click="addVisible = true">添加资产</el-button>
            <el-button class="np-primary-soft" @click="loadDevices">刷新</el-button>
            <el-button v-if="pendingDelete" type="warning" plain @click="undoDelete">撤销删除</el-button>
          </div>
        </div>
      </template>

      <el-skeleton :loading="loading" animated :rows="10">
        <template #default>
          <div v-for="grp in groupedDevices" :key="grp.group" class="mb-5">
            <div class="mb-2 text-sm font-semibold text-slate-600">{{ grp.group }} ({{ grp.rows.length }})</div>
            <el-table :data="grp.rows" class="np-borderless-table" size="large" @selection-change="(rows)=>selectedRows.value=rows" @row-dblclick="openQuickPeek">
              <el-table-column v-if="manageMode" type="selection" width="46" />
              <el-table-column v-if="visibleCols.status" label="状态" width="90">
                <template #default="{ row }">
                  <el-tooltip :content="statusLabel(row)">
                    <span class="inline-flex items-center gap-1 align-middle">
                      <span class="inline-block" :class="deviceStatusClass(row)" />
                      <span class="text-xs text-slate-500">{{ statusLabel(row) }}</span>
                    </span>
                  </el-tooltip>
                </template>
              </el-table-column>
              <el-table-column v-if="visibleCols.name" label="名称" min-width="180">
                <template #default="{ row }">
                  <el-button link type="primary" @click="openDeviceDetail(row)">{{ row.name || row.ip }}</el-button>
                </template>
              </el-table-column>
              <el-table-column v-if="visibleCols.ip" prop="ip" label="IP" min-width="160" />
              <el-table-column v-if="visibleCols.brand" prop="brand" label="品牌" width="120" />
              <el-table-column v-if="visibleCols.type" label="类型" width="140">
                <template #default="{ row }">
                  {{ row.device_tier === "core" ? "核心层" : row.device_tier === "aggregation" ? "汇聚层" : "接入层" }}
                </template>
              </el-table-column>
              <el-table-column v-if="visibleCols.cpu" label="CPU快照" width="120">
                <template #default="{ row }">{{ Number.isFinite(Number(row.cpu_usage)) ? `${Number(row.cpu_usage).toFixed(1)}%` : "-" }}</template>
              </el-table-column>
              <el-table-column v-if="visibleCols.uptime" label="运行时长" min-width="140">
                <template #default="{ row }">{{ row.uptime || "-" }}</template>
              </el-table-column>
              <el-table-column v-if="visibleCols.remark" prop="remark" label="备注" min-width="220" />
              <el-table-column label="操作" :width="manageMode ? 240 : 120">
                <template #default="{ row }">
                  <el-button type="primary" text @click="openQuickPeek(row)">快速预览</el-button>
                  <el-button v-if="manageMode" type="warning" text @click="openEditDevice(row)">编辑</el-button>
                  <el-button v-if="manageMode" type="danger" text @click="removeDevice(row)">删除</el-button>
                </template>
              </el-table-column>
            </el-table>
          </div>
          <el-empty
            v-if="!groupedDevices.length"
            description="暂无资产数据，请先添加资产或调整筛选条件"
            :image-size="88"
          >
            <template #default>
              <div class="mt-2 text-xs text-slate-500">建议：1) 点击“添加资产” 2) 选择模板 3) 执行预检后保存</div>
            </template>
          </el-empty>
        </template>
      </el-skeleton>
    </el-card>

    <el-drawer v-model="ops.isDrawerOpen" size="65%" direction="rtl" :with-header="true" title="设备快速预览" @close="ops.closeQuickPeek()">
      <el-skeleton :loading="drawerLoading" animated :rows="8">
        <template #default>
          <div class="mb-3 flex flex-wrap items-center justify-between gap-3">
            <div>
              <div class="text-lg font-semibold text-slate-900">{{ drawerDevice?.name || '-' }}</div>
              <div class="text-xs text-slate-500">{{ drawerDevice?.ip }} · {{ drawerDevice?.brand }}</div>
            </div>
          </div>

          <el-card class="mb-4">
            <template #header><span class="font-semibold">CPU / 内存</span></template>
            <div ref="drawerCpuMemChartEl" style="height: 240px"></div>
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

    <el-dialog v-model="addVisible" title="添加资产" width="560">
      <el-form label-position="top">
        <el-form-item label="监控模板">
          <el-select v-model="selectedTemplateId" class="w-full" clearable placeholder="可选：按模板自动填充SNMP参数" @change="applyTemplateById">
            <el-option v-for="t in templates" :key="t.id" :label="`${t.name} (${t.brand})`" :value="t.id" />
          </el-select>
          <div v-if="autoTemplateHint" class="mt-1 text-xs text-emerald-600">
            自动识别建议：{{ autoTemplateHint.name }}（分值 {{ autoTemplateHint.matchScore }}）
          </div>
        </el-form-item>
        <el-form-item label="设备IP"><el-input v-model="addForm.ip" /></el-form-item>
        <el-form-item label="资产名称"><el-input v-model="addForm.name" /></el-form-item>
        <el-form-item label="品牌">
          <el-select v-model="addForm.brand" class="w-full" filterable allow-create default-first-option>
            <el-option v-for="b in brandOptions" :key="b" :label="b" :value="b" />
          </el-select>
        </el-form-item>
        <el-form-item label="设备层级">
          <el-select v-model="addForm.device_tier" class="w-full">
            <el-option v-for="t in tierOptions" :key="`add-tier-${t.value}`" :label="t.label" :value="t.value" />
          </el-select>
        </el-form-item>
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
          <el-form-item v-if="addForm.v3_security_level !== 'noAuthNoPriv'" label="认证密码"><el-input v-model="addForm.v3_auth_password" show-password /></el-form-item>
          <el-form-item v-if="addForm.v3_security_level === 'authPriv'" label="加密协议">
            <el-select v-model="addForm.v3_priv_protocol" class="w-full">
              <el-option label="DES" value="DES" />
              <el-option label="AES" value="AES" />
            </el-select>
          </el-form-item>
          <el-form-item v-if="addForm.v3_security_level === 'authPriv'" label="加密密码"><el-input v-model="addForm.v3_priv_password" show-password /></el-form-item>
        </template>
        <el-form-item label="轮询间隔（秒）">
          <el-input-number v-model="addForm.poll_interval_sec" :min="0" :max="3600" :step="5" class="w-full" />
          <div class="text-xs text-slate-500">0 表示跟随系统默认（当前默认 {{ runtimeDefaults.poll_interval_sec }} 秒）</div>
          <div class="text-xs text-slate-500">推荐范围：核心层 30-60 秒；汇聚层 60-120 秒；接入层 120-300 秒</div>
        </el-form-item>
        <el-form-item label="CPU告警阈值（%）">
          <el-input-number v-model="addForm.cpu_threshold" :min="0" :max="100" :step="1" :precision="2" class="w-full" />
          <div class="text-xs text-slate-500">0 表示跟随系统默认（当前默认 {{ runtimeDefaults.alert_cpu_threshold }}%）</div>
        </el-form-item>
        <el-form-item label="内存告警阈值（%）">
          <el-input-number v-model="addForm.mem_threshold" :min="0" :max="100" :step="1" :precision="2" class="w-full" />
          <div class="text-xs text-slate-500">0 表示跟随系统默认（当前默认 {{ runtimeDefaults.alert_mem_threshold }}%）</div>
        </el-form-item>
        <el-form-item label="备注"><el-input v-model="addForm.remark" /></el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="addVisible = false">取消</el-button>
        <el-button type="primary" :loading="addLoading" @click="addDevice">保存</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="importVisible" title="批量导入资产" width="760">
      <div class="mb-2 flex items-center justify-between">
        <div class="text-xs text-slate-500">格式：ip,name,brand,community,snmp_version,remark,snmp_port,poll_interval_sec,cpu_threshold,mem_threshold（首行为表头）</div>
        <el-button size="small" @click="downloadImportTemplate">下载模板</el-button>
      </div>
      <el-input v-model="importCSV" type="textarea" :rows="14" placeholder="粘贴CSV内容" />
      <template #footer>
        <el-button @click="importVisible = false">取消</el-button>
        <el-button type="primary" :loading="importLoading" @click="importDevices">开始导入</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="editVisible" title="编辑资产" width="560">
      <el-form label-position="top">
        <el-form-item label="资产名称"><el-input v-model="editForm.name" /></el-form-item>
        <el-form-item label="品牌">
          <el-select v-model="editForm.brand" class="w-full" filterable allow-create default-first-option>
            <el-option v-for="b in brandOptions" :key="`edit-${b}`" :label="b" :value="b" />
          </el-select>
        </el-form-item>
        <el-form-item label="设备层级">
          <el-select v-model="editForm.device_tier" class="w-full">
            <el-option v-for="t in tierOptions" :key="`edit-tier-${t.value}`" :label="t.label" :value="t.value" />
          </el-select>
        </el-form-item>
        <el-form-item label="备注"><el-input v-model="editForm.remark" type="textarea" :rows="3" /></el-form-item>
        <el-form-item label="轮询间隔（秒）">
          <el-input-number v-model="editForm.poll_interval_sec" :min="0" :max="3600" :step="5" class="w-full" />
          <div class="text-xs text-slate-500">0 表示跟随系统默认（当前默认 {{ runtimeDefaults.poll_interval_sec }} 秒）</div>
          <div class="text-xs text-slate-500">推荐范围：核心层 30-60 秒；汇聚层 60-120 秒；接入层 120-300 秒</div>
        </el-form-item>
        <el-form-item label="CPU告警阈值（%）">
          <el-input-number v-model="editForm.cpu_threshold" :min="0" :max="100" :step="1" :precision="2" class="w-full" />
          <div class="text-xs text-slate-500">0 表示跟随系统默认（当前默认 {{ runtimeDefaults.alert_cpu_threshold }}%）</div>
        </el-form-item>
        <el-form-item label="内存告警阈值（%）">
          <el-input-number v-model="editForm.mem_threshold" :min="0" :max="100" :step="1" :precision="2" class="w-full" />
          <div class="text-xs text-slate-500">0 表示跟随系统默认（当前默认 {{ runtimeDefaults.alert_mem_threshold }}%）</div>
        </el-form-item>
        <el-form-item label="维护模式">
          <el-switch v-model="editForm.maintenance_mode" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="editVisible = false">取消</el-button>
        <el-button type="primary" :loading="editLoading" @click="saveEditDevice">保存</el-button>
      </template>
    </el-dialog>
  </div>
</template>
