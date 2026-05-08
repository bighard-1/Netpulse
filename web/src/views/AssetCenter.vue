<script setup>
import * as echarts from "echarts";
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from "vue";
import { ElMessage, ElMessageBox } from "element-plus";
import { useRouter } from "vue-router";
import { api, getApiError } from "../services/api";
import { useOpsStore } from "../stores/ops";

const ops = useOpsStore();
const router = useRouter();

const loading = ref(false);
const devices = ref([]);
const globalKeyword = ref("");
const groupBy = ref("brand");

const addVisible = ref(false);
const addLoading = ref(false);
const editVisible = ref(false);
const editLoading = ref(false);
const editForm = ref({ id: null, name: "", brand: "", remark: "", maintenance_mode: false });
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
const drawerCpuMemChartEl = ref(null);
let cpuMemChart = null;

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
    ElMessage.error(getApiError(err, "加载资产失败"));
  } finally {
    loading.value = false;
  }
}

async function addDevice() {
  if (isSnmpV3.value) {
    if (!addForm.value.v3_username) return ElMessage.warning("SNMP v3 需要填写用户名");
    if (addForm.value.v3_security_level !== "noAuthNoPriv" && !addForm.value.v3_auth_password) return ElMessage.warning("SNMP v3 需要填写认证密码");
    if (addForm.value.v3_security_level === "authPriv" && !addForm.value.v3_priv_password) return ElMessage.warning("SNMP v3(authPriv) 需要填写加密密码");
  } else if (!addForm.value.community) {
    return ElMessage.warning("SNMP v1/v2c 需要填写团体字串");
  }
  addLoading.value = true;
  try {
    await api.precheckDevice(addForm.value);
    await api.addDevice(addForm.value);
    ElMessage.success("资产添加成功");
    addVisible.value = false;
    addForm.value = defaultAddForm();
    await loadDevices();
  } catch (err) {
    ElMessage.error(getApiError(err, "添加资产失败"));
  } finally {
    addLoading.value = false;
  }
}

async function removeDevice(row) {
  try {
    await ElMessageBox.confirm(`确认删除资产 ${row.name || row.ip} 吗？`, "删除确认", { type: "warning" });
    await api.deleteDevice(row.id);
    ElMessage.success("资产已删除");
    await loadDevices();
  } catch (err) {
    if (err !== "cancel") ElMessage.error(getApiError(err, "删除资产失败"));
  }
}

function openEditDevice(row) {
  editForm.value = {
    id: row.id,
    name: row.name || "",
    brand: row.brand || "",
    remark: row.remark || "",
    maintenance_mode: Boolean(row.maintenance_mode)
  };
  editVisible.value = true;
}

async function saveEditDevice() {
  if (!editForm.value.id) return;
  editLoading.value = true;
  try {
    await api.updateDevice(editForm.value.id, {
      name: editForm.value.name || "",
      brand: editForm.value.brand || "",
      remark: editForm.value.remark || "",
      maintenance_mode: Boolean(editForm.value.maintenance_mode)
    });
    ElMessage.success("资产信息已更新");
    editVisible.value = false;
    await loadDevices();
  } catch (err) {
    ElMessage.error(getApiError(err, "更新资产失败"));
  } finally {
    editLoading.value = false;
  }
}

function openDeviceDetail(row) {
  if (!row?.id) return;
  router.push(`/device/${row.id}`);
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
    ElMessage.error(getApiError(err, "加载设备详情失败"));
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
    ElMessage.error(getApiError(err, "加载CPU/内存趋势失败"));
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
  await loadDevices();
  window.addEventListener("resize", onResize);
});

onBeforeUnmount(() => {
  window.removeEventListener("resize", onResize);
  cpuMemChart?.dispose();
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
    <el-card>
      <template #header>
        <div class="flex flex-wrap items-center justify-between gap-2">
          <div class="flex items-center gap-2">
            <span class="text-lg font-semibold">资产管理</span>
            <el-select v-model="groupBy" class="w-[130px]">
              <el-option label="按品牌" value="brand" />
              <el-option label="按位置" value="location" />
            </el-select>
          </div>
          <div class="flex items-center gap-2">
            <el-input v-model="globalKeyword" placeholder="搜索 IP / 名称 / 备注 / 端口名" clearable class="w-[320px]" />
            <el-button type="primary" @click="addVisible = true">添加资产</el-button>
            <el-button @click="loadDevices">刷新</el-button>
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
              <el-table-column label="名称" min-width="180">
                <template #default="{ row }">
                  <el-button link type="primary" @click="openDeviceDetail(row)">{{ row.name || row.ip }}</el-button>
                </template>
              </el-table-column>
              <el-table-column prop="ip" label="IP" min-width="160" />
              <el-table-column prop="brand" label="品牌" width="120" />
              <el-table-column prop="remark" label="备注" min-width="220" />
              <el-table-column label="操作" width="240">
                <template #default="{ row }">
                  <el-button type="primary" text @click="openQuickPeek(row)">快速预览</el-button>
                  <el-button type="warning" text @click="openEditDevice(row)">编辑</el-button>
                  <el-button type="danger" text @click="removeDevice(row)">删除</el-button>
                </template>
              </el-table-column>
            </el-table>
          </div>
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
          <el-form-item v-if="addForm.v3_security_level !== 'noAuthNoPriv'" label="认证密码"><el-input v-model="addForm.v3_auth_password" show-password /></el-form-item>
          <el-form-item v-if="addForm.v3_security_level === 'authPriv'" label="加密协议">
            <el-select v-model="addForm.v3_priv_protocol" class="w-full">
              <el-option label="DES" value="DES" />
              <el-option label="AES" value="AES" />
            </el-select>
          </el-form-item>
          <el-form-item v-if="addForm.v3_security_level === 'authPriv'" label="加密密码"><el-input v-model="addForm.v3_priv_password" show-password /></el-form-item>
        </template>
        <el-form-item label="备注"><el-input v-model="addForm.remark" /></el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="addVisible = false">取消</el-button>
        <el-button type="primary" :loading="addLoading" @click="addDevice">保存</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="editVisible" title="编辑资产" width="560">
      <el-form label-position="top">
        <el-form-item label="资产名称"><el-input v-model="editForm.name" /></el-form-item>
        <el-form-item label="品牌"><el-input v-model="editForm.brand" /></el-form-item>
        <el-form-item label="备注"><el-input v-model="editForm.remark" type="textarea" :rows="3" /></el-form-item>
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
