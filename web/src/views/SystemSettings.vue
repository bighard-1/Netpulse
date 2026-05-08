<script setup>
import { onMounted, ref } from "vue";
import { ElMessage } from "element-plus";
import { api, getApiError } from "../services/api";

const restoreLoading = ref(false);
const drillLoading = ref(false);
const settingsLoading = ref(false);
const savingSettings = ref(false);
const drillReportsLoading = ref(false);
const drillReports = ref([]);
const calibrationRows = ref([]);

const runtimeForm = ref({
  snmp_poll_interval_sec: 60,
  snmp_device_timeout_sec: 15,
  status_online_window_sec: 300,
  alert_cpu_threshold: 90,
  alert_mem_threshold: 90,
  alert_webhook_url: "",
  snmp_calibration_map: "{}",
  terminal_url_template: "ssh://{ip}"
});

async function loadRuntimeSettings() {
  settingsLoading.value = true;
  try {
    const res = await api.getRuntimeSettings();
    runtimeForm.value = {
      ...runtimeForm.value,
      ...(res.data || {})
    };
    runtimeForm.value.terminal_url_template = localStorage.getItem("np_terminal_url_template") || runtimeForm.value.terminal_url_template || "ssh://{ip}";
    hydrateCalibrationRows(runtimeForm.value.snmp_calibration_map);
  } catch (err) {
    ElMessage.error(getApiError(err, "加载运行参数失败"));
  } finally {
    settingsLoading.value = false;
  }
}

function hydrateCalibrationRows(raw) {
  try {
    const obj = JSON.parse(String(raw || "{}"));
    calibrationRows.value = Object.entries(obj).map(([ip, factor]) => ({ ip, factor: Number(factor || 1) }));
  } catch {
    calibrationRows.value = [];
  }
}

function syncCalibrationMap() {
  const obj = {};
  for (const row of calibrationRows.value) {
    const ip = String(row.ip || "").trim();
    const f = Number(row.factor || 1);
    if (!ip || !Number.isFinite(f) || f <= 0) continue;
    obj[ip] = f;
  }
  runtimeForm.value.snmp_calibration_map = JSON.stringify(obj);
}

function addCalibrationRow() {
  calibrationRows.value.push({ ip: "", factor: 1 });
}

function removeCalibrationRow(i) {
  calibrationRows.value.splice(i, 1);
  syncCalibrationMap();
}

async function saveRuntimeSettings() {
  savingSettings.value = true;
  try {
    syncCalibrationMap();
    const raw = String(runtimeForm.value.snmp_calibration_map || "{}").trim();
    JSON.parse(raw || "{}");
    localStorage.setItem("np_terminal_url_template", runtimeForm.value.terminal_url_template || "ssh://{ip}");
    await api.updateRuntimeSettings(runtimeForm.value);
    ElMessage.success("运行参数已保存，采集参数将自动生效");
    await loadRuntimeSettings();
  } catch (err) {
    ElMessage.error(getApiError(err, "保存运行参数失败"));
  } finally {
    savingSettings.value = false;
  }
}

async function onBackup() {
  try {
    const res = await api.downloadBackup();
    const blobUrl = URL.createObjectURL(new Blob([res.data]));
    const a = document.createElement("a");
    a.href = blobUrl;
    a.download = "netpulse_backup.sql.gz";
    a.click();
    URL.revokeObjectURL(blobUrl);
  } catch (err) {
    ElMessage.error(getApiError(err, "下载备份失败"));
  }
}

async function onRestore(file) {
  restoreLoading.value = true;
  try {
    await api.restoreFromFile(file.raw);
    ElMessage.success("恢复完成");
  } catch (err) {
    ElMessage.error(getApiError(err, "恢复失败"));
  } finally {
    restoreLoading.value = false;
  }
}

async function runBackupDrill() {
  drillLoading.value = true;
  try {
    await api.backupDrill();
    ElMessage.success("备份演练完成");
    await loadDrillReports();
  } catch (err) {
    ElMessage.error(getApiError(err, "备份演练失败"));
  } finally {
    drillLoading.value = false;
  }
}

async function loadDrillReports() {
  drillReportsLoading.value = true;
  try {
    const res = await api.listBackupDrillReports();
    drillReports.value = res.data || [];
  } catch (err) {
    ElMessage.error(getApiError(err, "加载演练报告失败"));
  } finally {
    drillReportsLoading.value = false;
  }
}

onMounted(async () => {
  await Promise.all([loadRuntimeSettings(), loadDrillReports()]);
});
</script>

<template>
  <div class="space-y-4">
    <el-card>
      <template #header><span class="text-lg font-semibold">运行参数（可在线修改）</span></template>
      <el-skeleton :loading="settingsLoading" animated :rows="6">
        <template #default>
          <el-form label-position="top" class="grid grid-cols-1 gap-3 md:grid-cols-2">
            <el-form-item label="轮询间隔（秒，5-3600）">
              <el-input-number v-model="runtimeForm.snmp_poll_interval_sec" :min="5" :max="3600" :step="5" class="w-full" />
            </el-form-item>
            <el-form-item label="设备超时（秒，2-120）">
              <el-input-number v-model="runtimeForm.snmp_device_timeout_sec" :min="2" :max="120" class="w-full" />
            </el-form-item>
            <el-form-item label="在线判定窗口（秒，30-3600）">
              <el-input-number v-model="runtimeForm.status_online_window_sec" :min="30" :max="3600" :step="30" class="w-full" />
            </el-form-item>
            <el-form-item label="CPU告警阈值（%）">
              <el-input-number v-model="runtimeForm.alert_cpu_threshold" :min="1" :max="100" :precision="2" class="w-full" />
            </el-form-item>
            <el-form-item label="内存告警阈值（%）">
              <el-input-number v-model="runtimeForm.alert_mem_threshold" :min="1" :max="100" :precision="2" class="w-full" />
            </el-form-item>
            <el-form-item label="告警 Webhook（可空）" class="md:col-span-2">
              <el-input v-model="runtimeForm.alert_webhook_url" placeholder="https://example.com/webhook" />
            </el-form-item>
            <el-form-item label="终端跳转模板（全局）" class="md:col-span-2">
              <el-input v-model="runtimeForm.terminal_url_template" placeholder="ssh://{ip} 或 http://webssh.local/?host={ip}" />
            </el-form-item>
            <el-form-item label="SNMP 校准映射(JSON)" class="md:col-span-2">
              <el-input
                v-model="runtimeForm.snmp_calibration_map"
                type="textarea"
                :rows="4"
                placeholder='例如: {"172.24.134.45":1.00,"172.24.134.46":0.97}'
              />
            </el-form-item>
            <el-form-item label="按设备编辑校准倍率" class="md:col-span-2">
              <div class="w-full space-y-2">
                <div v-for="(row, idx) in calibrationRows" :key="idx" class="flex items-center gap-2">
                  <el-input v-model="row.ip" placeholder="设备IP" @change="syncCalibrationMap" />
                  <el-input-number v-model="row.factor" :min="0.01" :max="10" :step="0.01" :precision="2" @change="syncCalibrationMap" />
                  <el-button type="danger" text @click="removeCalibrationRow(idx)">删除</el-button>
                </div>
                <el-button @click="addCalibrationRow">新增一行</el-button>
              </div>
            </el-form-item>
          </el-form>
          <div class="flex justify-end">
            <el-button type="primary" :loading="savingSettings" @click="saveRuntimeSettings">保存参数</el-button>
          </div>
        </template>
      </el-skeleton>
    </el-card>

    <div class="grid grid-cols-1 gap-4 xl:grid-cols-2">
      <el-card>
        <template #header><span class="text-lg font-semibold">备份与恢复</span></template>
        <div class="space-y-3">
          <el-button type="primary" @click="onBackup">下载备份</el-button>
          <el-upload :auto-upload="false" :show-file-list="false" accept=".gz" :on-change="onRestore" :disabled="restoreLoading">
            <el-button>恢复数据</el-button>
          </el-upload>
        </div>
      </el-card>

      <el-card>
        <template #header><span class="text-lg font-semibold">备份可恢复性演练</span></template>
        <div class="space-y-3">
          <el-button :loading="drillLoading" @click="runBackupDrill">执行备份演练</el-button>
          <el-button :loading="drillReportsLoading" @click="loadDrillReports">刷新演练记录</el-button>
        </div>
        <el-table :data="drillReports" class="mt-3 np-borderless-table" height="260">
          <el-table-column prop="created_at" label="时间" width="180" />
          <el-table-column prop="status" label="状态" width="120" />
          <el-table-column prop="message" label="结果" min-width="280" />
        </el-table>
      </el-card>
    </div>

    <el-card>
      <template #header><span class="text-lg font-semibold">环境变量说明（推荐迁移状态）</span></template>
      <div class="text-sm leading-7 text-slate-600">
        <p>已迁移到 Web 设置：轮询间隔、采集超时、在线判定窗口、CPU/内存阈值、告警 Webhook、SNMP 校准映射。</p>
        <p>仍建议保留在环境变量：DB_*、JWT_SECRET、ADMIN_USERNAME/ADMIN_PASSWORD、SYSLOG_ADDR、SNMP_TRAP_ADDR、TZ（容器级）。</p>
      </div>
    </el-card>
  </div>
</template>
