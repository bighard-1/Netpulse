<script setup>
import { computed, onActivated, onMounted, ref, watch } from "vue";
import { useRouter } from "vue-router";
import { ElMessage } from "element-plus";
import { api, getApiError } from "../services/api";

const router = useRouter();
const loading = ref(false);
const devices = ref([]);
const feedLoading = ref(false);
const criticalFeed = ref([]);

const addVisible = ref(false);
const addLoading = ref(false);
const addForm = ref({ ip: "", brand: "H3C", community: "public", remark: "", snmp_version: "2c", snmp_port: 161 });

const remarkVisible = ref(false);
const remarkLoading = ref(false);
const remarkForm = ref({ id: null, ip: "", remark: "" });

const totalDevices = computed(() => devices.value.length);
const onlineDevices = computed(() => devices.value.filter((d) => d.status === "online").length);
const offlineDevices = computed(() => devices.value.filter((d) => d.status === "offline" || d.status === "unknown").length);
const avgLatency = computed(() => {
  const samples = criticalFeed.value
    .map((x) => Number(x.duration_ms || 0))
    .filter((x) => Number.isFinite(x) && x > 0);
  if (!samples.length) return "--";
  return `${Math.round(samples.reduce((a, b) => a + b, 0) / samples.length)} ms`;
});

function severityOf(log) {
  const level = String(log.level || "").toUpperCase();
  const msg = String(log.message || "").toUpperCase();
  if (level === "ERROR" || msg.includes("OFFLINE") || msg.includes("DOWN") || msg.includes("TEMP") || msg.includes("POWER")) return "error";
  if (level === "WARNING" || msg.includes("OSPF") || msg.includes("BGP") || msg.includes("FLAP")) return "warning";
  return "info";
}

const feedView = computed(() => {
  return criticalFeed.value.map((item) => ({
    ...item,
    severity: severityOf(item)
  }));
});

async function loadDevices() {
  loading.value = true;
  try {
    const res = await api.listDevices();
    const rows = (res.data || []).map((row) => {
      const s = String(row.status || "unknown").toLowerCase();
      return {
        ...row,
        status: s === "online" || s === "offline" ? s : "unknown"
      };
    });
    devices.value = rows;
  } catch (err) {
    ElMessage.error(getApiError(err, "加载资产列表失败"));
  } finally {
    loading.value = false;
  }
}

async function loadFeed() {
  feedLoading.value = true;
  try {
    const picked = (devices.value || []).slice(0, 12);
    if (!picked.length) {
      criticalFeed.value = [];
      return;
    }
    const logsRes = await Promise.all(picked.map((d) => api.getDeviceLogs(d.id)));
    const merged = logsRes.flatMap((x) => x.data || []);
    merged.sort((a, b) => new Date(b.created_at || 0).getTime() - new Date(a.created_at || 0).getTime());
    criticalFeed.value = merged.slice(0, 20);
  } catch (err) {
    ElMessage.error(getApiError(err, "加载设备日志失败"));
  } finally {
    feedLoading.value = false;
  }
}

async function refreshAll() {
  await loadDevices();
  await loadFeed();
}

async function deleteDevice(id) {
  try {
    await api.deleteDevice(id);
    ElMessage.success("设备已删除");
    await refreshAll();
  } catch (err) {
    ElMessage.error(getApiError(err, "删除设备失败"));
  }
}

async function addDevice() {
  addLoading.value = true;
  try {
    if (!addForm.value.ip || !addForm.value.brand) {
      ElMessage.warning("请填写必填参数：IP、品牌");
      return;
    }
    await api.addDevice(addForm.value);
    ElMessage.success("资产添加成功");
    addVisible.value = false;
    addForm.value = { ip: "", brand: "H3C", community: "public", remark: "", snmp_version: "2c", snmp_port: 161 };
    await refreshAll();
  } catch (err) {
    ElMessage.error(getApiError(err, "保存资产失败"));
  } finally {
    addLoading.value = false;
  }
}

function openRemark(row) {
  remarkForm.value = { id: row.id, ip: row.ip, remark: row.remark || "" };
  remarkVisible.value = true;
}

async function saveRemark() {
  remarkLoading.value = true;
  try {
    await api.updateDeviceRemark(remarkForm.value.id, remarkForm.value.remark || "");
    ElMessage.success("设备备注已更新");
    remarkVisible.value = false;
    await loadDevices();
  } catch (err) {
    ElMessage.error(getApiError(err, "更新备注失败"));
  } finally {
    remarkLoading.value = false;
  }
}

function goDetail(row) {
  router.push({ path: `/device/${row.id}`, query: { ip: row.ip } });
}

onMounted(refreshAll);
onActivated(refreshAll);

watch(
  () => router.currentRoute.value.path,
  async (path) => {
    if (path === "/") {
      await refreshAll();
    }
  }
);
</script>

<template>
  <div class="space-y-5">
    <section class="grid grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-4">
      <el-card class="np-stat np-stat-total">
        <div class="np-stat-title">Total Devices</div>
        <div class="np-stat-value">{{ totalDevices }}</div>
      </el-card>
      <el-card class="np-stat np-stat-online">
        <div class="np-stat-title">Online</div>
        <div class="np-stat-value flex items-center gap-2"><span class="status-dot-online"></span>{{ onlineDevices }}</div>
      </el-card>
      <el-card class="np-stat np-stat-offline">
        <div class="np-stat-title">Offline / Alerts</div>
        <div class="np-stat-value">{{ offlineDevices }}</div>
      </el-card>
      <el-card class="np-stat np-stat-latency">
        <div class="np-stat-title">Avg Latency</div>
        <div class="np-stat-value">{{ avgLatency }}</div>
      </el-card>
    </section>

    <section class="grid grid-cols-1 gap-5 2xl:grid-cols-[2fr,1fr]">
      <el-card>
        <template #header>
          <div class="flex items-center justify-between">
            <span class="text-lg font-semibold">资产列表</span>
            <div class="flex items-center gap-2">
              <el-button type="primary" @click="addVisible = true">添加资产</el-button>
              <el-button @click="refreshAll">刷新</el-button>
            </div>
          </div>
        </template>

        <el-skeleton :loading="loading" animated :rows="8">
          <template #default>
            <el-table :data="devices" class="np-borderless-table">
              <el-table-column label="状态" width="90">
                <template #default="{ row }">
                  <el-tooltip :content="row.status_reason || '暂无状态说明'" placement="top">
                    <span
                      class="inline-block h-2.5 w-2.5 rounded-full"
                      :class="row.status === 'online' ? 'status-dot-online' : (row.status === 'offline' ? 'bg-rose-500' : 'bg-amber-400')"
                    />
                  </el-tooltip>
                </template>
              </el-table-column>
              <el-table-column prop="ip" label="IP" min-width="160" />
              <el-table-column prop="brand" label="品牌" width="120" />
              <el-table-column prop="remark" label="备注" min-width="220" />
              <el-table-column label="操作" width="280">
                <template #default="{ row }">
                  <el-button type="primary" text @click="goDetail(row)">查看详情</el-button>
                  <el-button type="warning" text @click="openRemark(row)">编辑备注</el-button>
                  <el-button type="danger" text @click="deleteDevice(row.id)">删除</el-button>
                </template>
              </el-table-column>
            </el-table>
          </template>
        </el-skeleton>
      </el-card>

      <el-card>
        <template #header>
          <span class="text-lg font-semibold">Latest Critical Logs</span>
        </template>
        <el-skeleton :loading="feedLoading" animated :rows="10">
          <template #default>
            <div class="space-y-2">
              <div v-for="item in feedView" :key="`${item.id}-${item.timestamp}`" class="log-item rounded-lg p-2" :class="{
                'log-error': item.severity === 'error',
                'log-warning': item.severity === 'warning'
              }">
                <div class="flex items-center justify-between gap-2">
                  <div class="text-xs font-semibold uppercase text-slate-600">{{ item.severity }}</div>
                  <div class="text-xs text-slate-500">{{ item.created_at || item.timestamp || '-' }}</div>
                </div>
                <div class="mt-1 text-sm text-slate-700">{{ item.message || `${item.action || ''} ${item.target || item.path || ''}` }}</div>
              </div>
              <el-empty v-if="!feedView.length" description="暂无关键事件" :image-size="72" />
            </div>
          </template>
        </el-skeleton>
      </el-card>
    </section>

    <el-dialog v-model="addVisible" title="添加资产" width="560">
      <el-form label-position="top">
        <el-form-item label="设备 IP"><el-input v-model="addForm.ip" placeholder="例如 172.24.134.45" /></el-form-item>
        <el-form-item label="品牌">
          <el-select v-model="addForm.brand" class="w-full">
            <el-option label="H3C" value="H3C" />
            <el-option label="Huawei" value="Huawei" />
          </el-select>
        </el-form-item>
        <el-form-item label="SNMP 版本">
          <el-select v-model="addForm.snmp_version" class="w-full">
            <el-option label="v1" value="1" />
            <el-option label="v2c" value="2c" />
            <el-option label="v3" value="3" />
          </el-select>
        </el-form-item>
        <el-form-item label="SNMP Community"><el-input v-model="addForm.community" placeholder="默认 public" /></el-form-item>
        <el-form-item label="备注"><el-input v-model="addForm.remark" /></el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="addVisible = false">取消</el-button>
        <el-button type="primary" :loading="addLoading" @click="addDevice">保存</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="remarkVisible" title="编辑资产备注" width="520">
      <el-form label-position="top">
        <el-form-item label="资产 IP"><el-input :model-value="remarkForm.ip" disabled /></el-form-item>
        <el-form-item label="备注"><el-input v-model="remarkForm.remark" type="textarea" :rows="4" /></el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="remarkVisible = false">取消</el-button>
        <el-button type="primary" :loading="remarkLoading" @click="saveRemark">保存</el-button>
      </template>
    </el-dialog>
  </div>
</template>
