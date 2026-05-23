<script setup>
import { computed, onBeforeUnmount, onMounted, ref } from "vue";
import { api } from "../services/api";
import { useFeedback } from "../composables/useFeedback";
import { useAuthStore } from "../stores/auth";

const fb = useFeedback();
const auth = useAuthStore();
const isAdmin = computed(() => Boolean(auth.isAdmin));
const editMode = ref(localStorage.getItem("np_edit_mode") === "1");

const restoreLoading = ref(false);
const backupLoading = ref(false);
const drillLoading = ref(false);
const settingsLoading = ref(false);
const savingSettings = ref(false);
const drillReportsLoading = ref(false);
const drillReports = ref([]);
const recentJobs = ref([]);
const activeBackupJob = ref(null);
const activeRestoreJob = ref(null);
const activeDrillJob = ref(null);
const jobTimers = new Set();
const calibrationRows = ref([]);
const activeTab = ref("runtime");
const templateLoading = ref(false);
const templates = ref([]);
const saveTemplateLoading = ref(false);
const alertRuleLoading = ref(false);
const alertRules = ref([]);
const saveRuleLoading = ref(false);
const opsLoading = ref(false);
const opsSummary = ref({
  device_total: 0,
  open_alert_events: 0,
  recent_events: 0,
  recent_audits: 0,
  last_event_at: "",
  last_audit_at: "",
  last_metric_at: "",
  ingest_delay_sec: 0,
  recent_jobs: []
});
const opsDetailVisible = ref(false);
const opsDetailTitle = ref("");
const opsDetailRows = ref([]);
const opsDetailType = ref("events");
const slowApiLogs = ref([]);

function isForbidden(err) {
  const status = Number(err?.response?.status || 0);
  const msg = String(err?.response?.data?.error || err?.message || "").toLowerCase();
  return status === 401 || status === 403 || msg.includes("forbidden") || msg.includes("admin only");
}

function loadSlowApiLogs() {
  try {
    slowApiLogs.value = JSON.parse(localStorage.getItem("np_slow_api_logs") || "[]").slice(0, 30);
  } catch {
    slowApiLogs.value = [];
  }
}

function clearSlowApiLogs() {
  localStorage.removeItem("np_slow_api_logs");
  slowApiLogs.value = [];
  fb.success("已清空慢请求记录");
}

const templateForm = ref({
  name: "",
  brand: "H3C",
  description: "",
  match_sysobjectid: "",
  match_sysdescr: "",
  priority: 100,
  auto_enabled: true,
  cpu_oid: ".1.3.6.1.4.1.2011.5.25.31.1.1.1.1.5",
  mem_oid: ".1.3.6.1.4.1.2011.5.25.31.1.1.1.1.7",
  if_in_oid: ".1.3.6.1.2.1.31.1.1.1.6",
  if_out_oid: ".1.3.6.1.2.1.31.1.1.1.10"
});

const alertRuleForm = ref({
  id: null,
  name: "全局告警策略",
  scope: "global",
  device_id: null,
  cpu_threshold: 90,
  mem_threshold: 90,
  traffic_threshold: 0,
  mute_start: "",
  mute_end: "",
  notify_webhook: "",
  enabled: true
});

const runtimeForm = ref({
  poll_interval_core_sec: 60,
  poll_interval_agg_sec: 90,
  poll_interval_access_sec: 120,
  snmp_device_timeout_sec: 15,
  status_online_window_sec: 300,
  alert_cpu_threshold: 90,
  alert_mem_threshold: 90,
  alert_webhook_url: "",
  snmp_calibration_map: "{}"
});
const runtimeSnapshot = ref({});

async function loadRuntimeSettings() {
  settingsLoading.value = true;
  try {
    const res = await api.getRuntimeSettings();
    runtimeSnapshot.value = { ...(res.data || {}) };
    runtimeForm.value = {
      ...runtimeForm.value,
      ...(res.data || {})
    };
    hydrateCalibrationRows(runtimeForm.value.snmp_calibration_map);
  } catch (err) {
    if (isForbidden(err)) return;
    fb.apiError(err, "加载运行参数失败");
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
  if (!editMode.value) return fb.warn("当前为只读模式，请先在左侧开启编辑模式");
  savingSettings.value = true;
  try {
    syncCalibrationMap();
    const raw = String(runtimeForm.value.snmp_calibration_map || "{}").trim();
    JSON.parse(raw || "{}");
    const keep = runtimeSnapshot.value || {};
    // White-list editable fields; preserve hidden fields from server snapshot.
    const payload = {
      poll_interval_core_sec: Number(runtimeForm.value.poll_interval_core_sec || keep.poll_interval_core_sec || 60),
      poll_interval_agg_sec: Number(runtimeForm.value.poll_interval_agg_sec || keep.poll_interval_agg_sec || 90),
      poll_interval_access_sec: Number(runtimeForm.value.poll_interval_access_sec || keep.poll_interval_access_sec || 120),
      alert_cpu_threshold: Number(runtimeForm.value.alert_cpu_threshold || keep.alert_cpu_threshold || 90),
      alert_mem_threshold: Number(runtimeForm.value.alert_mem_threshold || keep.alert_mem_threshold || 90),
      snmp_device_timeout_sec: Number(runtimeForm.value.snmp_device_timeout_sec || keep.snmp_device_timeout_sec || 15),
      status_online_window_sec: Number(runtimeForm.value.status_online_window_sec || keep.status_online_window_sec || 300),
      alert_webhook_url: String(runtimeForm.value.alert_webhook_url || "").trim(),
      snmp_calibration_map: raw
    };
    await api.updateRuntimeSettings(payload);
    fb.success("运行参数已保存", "采集参数将自动生效");
    await loadRuntimeSettings();
  } catch (err) {
    fb.apiError(err, "保存运行参数失败");
  } finally {
    savingSettings.value = false;
  }
}

function setActiveJob(job) {
  if (!job?.type) return;
  if (job.type === "backup") activeBackupJob.value = job;
  if (job.type === "restore") activeRestoreJob.value = job;
  if (job.type === "backup_drill") activeDrillJob.value = job;
}

async function loadSystemJobs(silent = true) {
  try {
    const res = await api.listSystemJobs(20);
    recentJobs.value = res.data || [];
  } catch (err) {
    if (!silent && !isForbidden(err)) fb.apiError(err, "加载后台任务失败");
  }
}

function pollJob(id, onDone) {
  return new Promise((resolve, reject) => {
    const started = Date.now();
    const timer = window.setInterval(async () => {
      try {
        const res = await api.getSystemJob(id);
        const job = res.data || {};
        setActiveJob(job);
        await loadSystemJobs();
        if (job.status === "completed") {
          window.clearInterval(timer);
          jobTimers.delete(timer);
          if (onDone) await onDone(job);
          resolve(job);
        } else if (job.status === "failed") {
          window.clearInterval(timer);
          jobTimers.delete(timer);
          reject(new Error(job.error || job.message || "任务执行失败"));
        } else if (Date.now() - started > 40 * 60 * 1000) {
          window.clearInterval(timer);
          jobTimers.delete(timer);
          reject(new Error("后台任务超时，请在运行观测中查看状态"));
        }
      } catch (err) {
        window.clearInterval(timer);
        jobTimers.delete(timer);
        reject(err);
      }
    }, 2000);
    jobTimers.add(timer);
  });
}

async function downloadBackupBlob(jobId) {
  const res = await api.downloadBackupJob(jobId);
  const blobUrl = URL.createObjectURL(new Blob([res.data]));
  const disposition = res.headers?.["content-disposition"] || "";
  const match = disposition.match(/filename="?([^"]+)"?/i);
  const a = document.createElement("a");
  a.href = blobUrl;
  a.download = match?.[1] || "netpulse_backup.sql.gz";
  a.click();
  URL.revokeObjectURL(blobUrl);
}

async function onBackup() {
  backupLoading.value = true;
  try {
    const res = await api.startBackupJob();
    const job = res.data || {};
    activeBackupJob.value = job;
    fb.success("备份任务已启动", "完成后会自动下载备份文件");
    await pollJob(job.id, (doneJob) => downloadBackupBlob(doneJob.id));
    fb.success("备份文件已下载");
  } catch (err) {
    fb.apiError(err, "下载备份失败");
  } finally {
    backupLoading.value = false;
    await loadSystemJobs();
  }
}

const backupScopeText = "全量备份：包含资产设备、端口、历史指标(流量/CPU/内存/存储)、告警与事件、用户与权限、系统设置。下载得到的 .sql.gz 文件可在本恢复入口原样上传，无需解压或修改。";

async function onRestore(file) {
  if (!editMode.value) return fb.warn("当前为只读模式，请先在左侧开启编辑模式");
  restoreLoading.value = true;
  try {
    const res = await api.startRestoreJob(file.raw);
    const job = res.data || {};
    activeRestoreJob.value = job;
    fb.success("恢复任务已启动", "恢复完成前请勿重复提交");
    await pollJob(job.id);
    fb.success("恢复完成");
  } catch (err) {
    fb.apiError(err, "恢复失败");
  } finally {
    restoreLoading.value = false;
    await loadSystemJobs();
  }
}

async function runBackupDrill() {
  if (!editMode.value) return fb.warn("当前为只读模式，请先在左侧开启编辑模式");
  drillLoading.value = true;
  try {
    const res = await api.startBackupDrillJob();
    const job = res.data || {};
    activeDrillJob.value = job;
    fb.success("备份演练任务已启动");
    await pollJob(job.id);
    fb.success("备份演练完成");
    await loadDrillReports();
  } catch (err) {
    fb.apiError(err, "备份演练失败");
  } finally {
    drillLoading.value = false;
    await loadSystemJobs();
  }
}

async function downloadInspectionBundle() {
  try {
    const res = await api.downloadInspectionBundle();
    const blobUrl = URL.createObjectURL(res.data);
    const a = document.createElement("a");
    a.href = blobUrl;
    a.download = "netpulse_inspection_bundle.json";
    a.click();
    URL.revokeObjectURL(blobUrl);
    fb.success("巡检包已导出");
  } catch (err) {
    fb.apiError(err, "导出巡检包失败");
  }
}

async function loadDrillReports() {
  drillReportsLoading.value = true;
  try {
    const res = await api.listBackupDrillReports();
    drillReports.value = res.data || [];
  } catch (err) {
    if (isForbidden(err)) return;
    fb.apiError(err, "加载演练报告失败");
  } finally {
    drillReportsLoading.value = false;
  }
}

async function loadTemplates() {
  templateLoading.value = true;
  try {
    const res = await api.listTemplates();
    templates.value = res.data || [];
  } catch (err) {
    if (isForbidden(err)) return;
    fb.apiError(err, "加载模板失败");
  } finally {
    templateLoading.value = false;
  }
}

async function saveTemplate() {
  if (!editMode.value) return fb.warn("当前为只读模式，请先在左侧开启编辑模式");
  saveTemplateLoading.value = true;
  try {
    await api.createTemplate(templateForm.value);
    fb.success("模板已创建");
    await loadTemplates();
  } catch (err) {
    fb.apiError(err, "创建模板失败");
  } finally {
    saveTemplateLoading.value = false;
  }
}

async function loadAlertRules() {
  alertRuleLoading.value = true;
  try {
    const res = await api.listAlertRules();
    alertRules.value = res.data || [];
  } catch (err) {
    if (isForbidden(err)) return;
    fb.apiError(err, "加载告警规则失败");
  } finally {
    alertRuleLoading.value = false;
  }
}

async function loadOpsSummary() {
  opsLoading.value = true;
  try {
    const res = await api.getSystemOps();
    opsSummary.value = { ...opsSummary.value, ...(res.data || {}) };
    recentJobs.value = res.data?.recent_jobs || recentJobs.value;
  } catch (err) {
    if (isForbidden(err)) return;
    fb.apiError(err, "加载运维概况失败");
  } finally {
    opsLoading.value = false;
  }
}

async function openOpsDetail(type) {
  if (!isAdmin.value) return fb.warn("当前账号无权限查看运行观测明细");
  try {
    opsDetailType.value = type;
    opsDetailVisible.value = true;
    opsDetailRows.value = [];
    if (type === "devices") {
      opsDetailTitle.value = "设备列表明细";
      const res = await api.listDevices();
      opsDetailRows.value = res.data || [];
      return;
    }
    if (type === "alerts") {
      opsDetailTitle.value = "开放告警明细";
      const res = await api.listAlertEvents(200, "open");
      opsDetailRows.value = res.data?.data || [];
      return;
    }
    if (type === "events") {
      opsDetailTitle.value = "近期事件明细";
      const res = await api.listRecentEvents(200);
      opsDetailRows.value = res.data?.data || res.data || [];
      return;
    }
    if (type === "audits") {
      opsDetailTitle.value = "近期审计明细";
      const res = await api.listAuditLogs();
      opsDetailRows.value = res.data || [];
    }
  } catch (err) {
    if (isForbidden(err)) return;
    fb.apiError(err, "加载运行观测明细失败");
  }
}

async function saveAlertRule() {
  if (!editMode.value) return fb.warn("当前为只读模式，请先在左侧开启编辑模式");
  saveRuleLoading.value = true;
  try {
    await api.upsertAlertRule({
      id: alertRuleForm.value.id || 0,
      name: alertRuleForm.value.name || "全局告警策略",
      scope: alertRuleForm.value.scope || "global",
      device_id: alertRuleForm.value.scope === "device" ? Number(alertRuleForm.value.device_id || 0) || null : null,
      cpu_threshold: Number(alertRuleForm.value.cpu_threshold || 0) || null,
      mem_threshold: Number(alertRuleForm.value.mem_threshold || 0) || null,
      traffic_threshold: Number(alertRuleForm.value.traffic_threshold || 0) || null,
      mute_start: String(alertRuleForm.value.mute_start || "").trim(),
      mute_end: String(alertRuleForm.value.mute_end || "").trim(),
      notify_webhook: String(alertRuleForm.value.notify_webhook || "").trim(),
      enabled: Boolean(alertRuleForm.value.enabled)
    });
    fb.success("告警规则已保存");
    alertRuleForm.value.id = null;
    await loadAlertRules();
  } catch (err) {
    fb.apiError(err, "保存告警规则失败");
  } finally {
    saveRuleLoading.value = false;
  }
}

function editAlertRule(row) {
  alertRuleForm.value = {
    id: row.id || null,
    name: row.name || "告警策略",
    scope: row.scope || "global",
    device_id: row.device_id || null,
    cpu_threshold: row.cpu_threshold ?? 90,
    mem_threshold: row.mem_threshold ?? 90,
    traffic_threshold: row.traffic_threshold ?? 0,
    mute_start: row.mute_start || "",
    mute_end: row.mute_end || "",
    notify_webhook: row.notify_webhook || "",
    enabled: Boolean(row.enabled)
  };
}

function resetAlertRuleForm() {
  alertRuleForm.value = {
    id: null,
    name: "全局告警策略",
    scope: "global",
    device_id: null,
    cpu_threshold: 90,
    mem_threshold: 90,
    traffic_threshold: 0,
    mute_start: "",
    mute_end: "",
    notify_webhook: "",
    enabled: true
  };
}

async function removeAlertRule(row) {
  if (!editMode.value) return fb.warn("当前为只读模式，请先在左侧开启编辑模式");
  try {
    await api.deleteAlertRule(row.id);
    fb.success("告警规则已删除");
    if (alertRuleForm.value.id === row.id) {
      resetAlertRuleForm();
    }
    await loadAlertRules();
  } catch (err) {
    fb.apiError(err, "删除告警规则失败");
  }
}

onMounted(async () => {
  loadSlowApiLogs();
  window.addEventListener("np-edit-mode", onEditModeEvent);
  window.addEventListener("np-slow-api-log", loadSlowApiLogs);
  if (!isAdmin.value) return;
  await Promise.all([loadRuntimeSettings(), loadDrillReports(), loadTemplates(), loadAlertRules(), loadOpsSummary(), loadSystemJobs()]);
});

onBeforeUnmount(() => {
  window.removeEventListener("np-edit-mode", onEditModeEvent);
  window.removeEventListener("np-slow-api-log", loadSlowApiLogs);
  for (const timer of jobTimers) {
    window.clearInterval(timer);
  }
  jobTimers.clear();
});

function onEditModeEvent(e) {
  editMode.value = Boolean(e?.detail?.enabled);
}
</script>

<template>
  <div class="space-y-4">
    <el-alert
      v-if="!isAdmin"
      title="当前账号为只读权限，系统设置仅管理员可访问。请使用管理员账号登录或联系管理员授权。"
      type="warning"
      show-icon
      :closable="false"
    />
    <el-card>
      <template #header>
        <span class="text-lg font-semibold">系统设置中心</span>
      </template>
      <el-tabs v-model="activeTab">
        <el-tab-pane label="运行参数" name="runtime" />
        <el-tab-pane label="告警策略" name="alert" />
        <el-tab-pane label="模板中心" name="template" />
        <el-tab-pane label="备份恢复" name="backup" />
        <el-tab-pane label="运行观测" name="ops" />
      </el-tabs>
    </el-card>

    <el-card v-show="activeTab === 'runtime'">
      <template #header><span class="text-lg font-semibold">运行参数（可在线修改）</span></template>
      <el-skeleton :loading="settingsLoading" animated :rows="6">
        <template #default>
          <el-form label-position="top" class="grid grid-cols-1 gap-3 md:grid-cols-2">
            <el-form-item label="核心层默认轮询（推荐 30-90 秒）">
              <el-input-number v-model="runtimeForm.poll_interval_core_sec" :min="5" :max="3600" :step="5" class="w-full" />
            </el-form-item>
            <el-form-item label="汇聚层默认轮询（推荐 60-180 秒）">
              <el-input-number v-model="runtimeForm.poll_interval_agg_sec" :min="5" :max="3600" :step="5" class="w-full" />
            </el-form-item>
            <el-form-item label="接入层默认轮询（推荐 120-300 秒）">
              <el-input-number v-model="runtimeForm.poll_interval_access_sec" :min="5" :max="3600" :step="5" class="w-full" />
            </el-form-item>
            <el-form-item label="设备超时（秒，2-120）">
              <el-input-number v-model="runtimeForm.snmp_device_timeout_sec" :min="2" :max="120" class="w-full" />
            </el-form-item>
            <el-form-item label="在线判定窗口（秒，30-3600）">
              <el-input-number v-model="runtimeForm.status_online_window_sec" :min="30" :max="3600" :step="30" class="w-full" />
            </el-form-item>
            <el-form-item label="默认CPU告警阈值（%）">
              <el-input-number v-model="runtimeForm.alert_cpu_threshold" :min="0" :max="100" :precision="2" class="w-full" />
            </el-form-item>
            <el-form-item label="默认内存告警阈值（%）">
              <el-input-number v-model="runtimeForm.alert_mem_threshold" :min="0" :max="100" :precision="2" class="w-full" />
            </el-form-item>
            <el-form-item label="说明" class="md:col-span-2">
              <el-alert type="info" :closable="false" title="这里配置系统默认值；单台资产可在资产新增/编辑或设备详情编辑中覆盖。设为0表示跟随系统默认。" />
            </el-form-item>
            <el-form-item label="告警 Webhook（可空）" class="md:col-span-2">
              <el-input v-model="runtimeForm.alert_webhook_url" placeholder="https://example.com/webhook" />
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
            <el-button type="primary" :disabled="!editMode" :loading="savingSettings" @click="saveRuntimeSettings">保存参数</el-button>
          </div>
          <div class="mt-4 rounded-lg border border-slate-200 p-3">
            <div class="mb-2 flex items-center justify-between">
              <span class="font-semibold">前端慢请求观测（>1200ms）</span>
              <el-button size="small" @click="clearSlowApiLogs">清空</el-button>
            </div>
            <el-table :data="slowApiLogs" size="small" max-height="260" class="np-borderless-table">
              <el-table-column prop="ts" label="时间" min-width="180" />
              <el-table-column prop="method" label="方法" width="90" />
              <el-table-column prop="url" label="路径" min-width="220" />
              <el-table-column prop="ms" label="耗时(ms)" width="110" />
              <el-table-column label="结果" width="90">
                <template #default="{ row }">
                  <el-tag size="small" :type="row.ok ? 'success' : 'danger'">{{ row.ok ? "成功" : "失败" }}</el-tag>
                </template>
              </el-table-column>
            </el-table>
          </div>
        </template>
      </el-skeleton>
    </el-card>

    <el-card v-show="activeTab === 'alert'">
      <template #header><span class="text-lg font-semibold">告警策略中心（阈值/静默/通知）</span></template>
      <div class="grid grid-cols-1 gap-4 xl:grid-cols-2">
        <el-form label-position="top">
          <el-form-item label="规则名称">
            <el-input v-model="alertRuleForm.name" placeholder="如：全局告警策略" />
          </el-form-item>
          <el-form-item label="作用域">
            <el-select v-model="alertRuleForm.scope" class="w-full">
              <el-option label="全局" value="global" />
              <el-option label="单设备" value="device" />
            </el-select>
          </el-form-item>
          <el-form-item label="设备ID（作用域=单设备时）">
            <el-input-number v-model="alertRuleForm.device_id" :min="1" :max="9999999" class="w-full" :disabled="alertRuleForm.scope !== 'device'" />
          </el-form-item>
          <el-form-item label="CPU阈值（%）">
            <el-input-number v-model="alertRuleForm.cpu_threshold" :min="0" :max="100" :precision="2" class="w-full" />
          </el-form-item>
          <el-form-item label="内存阈值（%）">
            <el-input-number v-model="alertRuleForm.mem_threshold" :min="0" :max="100" :precision="2" class="w-full" />
          </el-form-item>
          <el-form-item label="流量阈值（bps）">
            <el-input-number v-model="alertRuleForm.traffic_threshold" :min="0" :max="1000000000000000" class="w-full" />
          </el-form-item>
          <el-form-item label="静默开始（HH:MM）">
            <el-input v-model="alertRuleForm.mute_start" placeholder="例如 23:00" />
          </el-form-item>
          <el-form-item label="静默结束（HH:MM）">
            <el-input v-model="alertRuleForm.mute_end" placeholder="例如 07:00" />
          </el-form-item>
          <el-form-item label="规则级Webhook（可空）" class="xl:col-span-2">
            <el-input v-model="alertRuleForm.notify_webhook" placeholder="https://example.com/webhook" />
          </el-form-item>
          <el-form-item label="启用规则">
            <el-switch v-model="alertRuleForm.enabled" />
          </el-form-item>
          <div class="flex justify-end">
            <el-button @click="resetAlertRuleForm">重置</el-button>
            <el-button type="primary" :disabled="!editMode" :loading="saveRuleLoading" @click="saveAlertRule">保存规则</el-button>
          </div>
        </el-form>

        <el-table :data="alertRules" class="np-borderless-table" height="360" v-loading="alertRuleLoading">
          <el-table-column prop="name" label="规则名" min-width="140" />
          <el-table-column prop="scope" label="作用域" width="90" />
          <el-table-column prop="device_id" label="设备ID" width="90" />
          <el-table-column prop="cpu_threshold" label="CPU%" width="90" />
          <el-table-column prop="mem_threshold" label="内存%" width="90" />
          <el-table-column prop="traffic_threshold" label="流量阈值(bps)" min-width="140" />
          <el-table-column label="静默窗口" min-width="130">
            <template #default="{ row }">{{ row.mute_start || "-" }} ~ {{ row.mute_end || "-" }}</template>
          </el-table-column>
          <el-table-column prop="enabled" label="启用" width="80" />
          <el-table-column label="操作" width="150">
            <template #default="{ row }">
              <el-button type="primary" text @click="editAlertRule(row)">编辑</el-button>
              <el-button type="danger" text :disabled="!editMode" @click="removeAlertRule(row)">删除</el-button>
            </template>
          </el-table-column>
        </el-table>
      </div>
      <div class="mt-4 rounded-lg bg-slate-50 p-3 text-sm text-slate-600">
        通知通道：当前支持 Webhook（运行参数中配置）。邮件通知可在下一步扩展 SMTP 设置后启用。
      </div>
    </el-card>

    <el-card v-show="activeTab === 'template'">
      <template #header><span class="text-lg font-semibold">模板中心（Template）</span></template>
      <div class="grid grid-cols-1 gap-4 xl:grid-cols-2">
        <el-form label-position="top">
          <el-form-item label="模板名称"><el-input v-model="templateForm.name" placeholder="如：华为交换机模板" /></el-form-item>
          <el-form-item label="厂商">
            <el-select v-model="templateForm.brand" class="w-full">
              <el-option label="H3C" value="H3C" />
              <el-option label="Huawei" value="Huawei" />
              <el-option label="Generic SNMP" value="Generic" />
            </el-select>
          </el-form-item>
          <el-form-item label="说明"><el-input v-model="templateForm.description" type="textarea" :rows="2" /></el-form-item>
          <el-form-item label="sysObjectID匹配（逗号分隔）"><el-input v-model="templateForm.match_sysobjectid" placeholder="如：1.3.6.1.4.1.2011,1.3.6.1.4.1.25506" /></el-form-item>
          <el-form-item label="sysDescr关键词（逗号分隔）"><el-input v-model="templateForm.match_sysdescr" placeholder="如：S12700E,CloudEngine,H3C S12500" /></el-form-item>
          <el-form-item label="匹配优先级（越小越优先）"><el-input-number v-model="templateForm.priority" :min="1" :max="1000" class="w-full" /></el-form-item>
          <el-form-item label="启用自动匹配"><el-switch v-model="templateForm.auto_enabled" /></el-form-item>
          <el-form-item label="CPU OID"><el-input v-model="templateForm.cpu_oid" /></el-form-item>
          <el-form-item label="内存 OID"><el-input v-model="templateForm.mem_oid" /></el-form-item>
          <el-form-item label="入方向 OID"><el-input v-model="templateForm.if_in_oid" /></el-form-item>
          <el-form-item label="出方向 OID"><el-input v-model="templateForm.if_out_oid" /></el-form-item>
          <div class="flex justify-end">
            <el-button type="primary" :disabled="!editMode" :loading="saveTemplateLoading" @click="saveTemplate">保存模板</el-button>
          </div>
        </el-form>

        <el-table :data="templates" class="np-borderless-table" height="460" v-loading="templateLoading">
          <el-table-column prop="name" label="模板名" min-width="160" />
          <el-table-column prop="brand" label="厂商" width="120" />
          <el-table-column prop="priority" label="优先级" width="90" />
          <el-table-column label="自动匹配" width="100">
            <template #default="{ row }">{{ row.auto_enabled ? "启用" : "停用" }}</template>
          </el-table-column>
          <el-table-column prop="match_sysobjectid" label="sysObjectID规则" min-width="180" />
          <el-table-column prop="match_sysdescr" label="sysDescr规则" min-width="180" />
          <el-table-column prop="description" label="说明" min-width="220" />
        </el-table>
      </div>
      <div class="mt-4 rounded-lg bg-slate-50 p-3 text-sm text-slate-600">
        建议：新增设备时先选择模板，再自动填充 OID 与采集参数，避免逐台重复配置。
      </div>
    </el-card>

    <div v-show="activeTab === 'backup'" class="grid grid-cols-1 gap-4 xl:grid-cols-2">
      <el-card>
        <template #header><span class="text-lg font-semibold">备份与恢复</span></template>
        <el-alert :title="backupScopeText" type="info" show-icon :closable="false" class="mb-3" />
        <div class="space-y-3">
          <el-button type="primary" :loading="backupLoading" @click="onBackup">下载备份</el-button>
          <el-button @click="downloadInspectionBundle">导出一键巡检包</el-button>
          <el-upload :auto-upload="false" :show-file-list="false" accept=".sql.gz,.gz" :on-change="onRestore" :disabled="restoreLoading || !editMode">
            <el-button :loading="restoreLoading">恢复数据</el-button>
          </el-upload>
          <div v-if="activeBackupJob" class="rounded-lg bg-slate-50 p-3 text-sm text-slate-600">
            <div class="mb-1 font-semibold">备份任务：{{ activeBackupJob.message }}</div>
            <el-progress :percentage="Number(activeBackupJob.progress || 0)" :status="activeBackupJob.status === 'failed' ? 'exception' : activeBackupJob.status === 'completed' ? 'success' : undefined" />
          </div>
          <div v-if="activeRestoreJob" class="rounded-lg bg-slate-50 p-3 text-sm text-slate-600">
            <div class="mb-1 font-semibold">恢复任务：{{ activeRestoreJob.message }}</div>
            <el-progress :percentage="Number(activeRestoreJob.progress || 0)" :status="activeRestoreJob.status === 'failed' ? 'exception' : activeRestoreJob.status === 'completed' ? 'success' : undefined" />
          </div>
        </div>
      </el-card>

      <el-card>
        <template #header><span class="text-lg font-semibold">备份可恢复性演练</span></template>
        <div class="space-y-3">
          <el-button :disabled="!editMode" :loading="drillLoading" @click="runBackupDrill">执行备份演练</el-button>
          <el-button :loading="drillReportsLoading" @click="loadDrillReports">刷新演练记录</el-button>
          <div v-if="activeDrillJob" class="rounded-lg bg-slate-50 p-3 text-sm text-slate-600">
            <div class="mb-1 font-semibold">演练任务：{{ activeDrillJob.message }}</div>
            <el-progress :percentage="Number(activeDrillJob.progress || 0)" :status="activeDrillJob.status === 'failed' ? 'exception' : activeDrillJob.status === 'completed' ? 'success' : undefined" />
          </div>
        </div>
        <el-table :data="drillReports" class="mt-3 np-borderless-table" height="260">
          <el-table-column prop="created_at" label="时间" width="180" />
          <el-table-column prop="status" label="状态" width="120" />
          <el-table-column prop="message" label="结果" min-width="280" />
        </el-table>
      </el-card>
    </div>

    <el-card v-show="activeTab === 'backup'">
      <template #header><span class="text-lg font-semibold">环境变量说明（推荐迁移状态）</span></template>
      <div class="text-sm leading-7 text-slate-600">
        <p>已迁移到 Web 设置：轮询间隔、采集超时、在线判定窗口、CPU/内存阈值、告警 Webhook、SNMP 校准映射。</p>
        <p>仍建议保留在环境变量：DB_*、JWT_SECRET、ADMIN_USERNAME/ADMIN_PASSWORD、SYSLOG_ADDR、SNMP_TRAP_ADDR、TZ（容器级）。</p>
      </div>
    </el-card>

    <el-card v-show="activeTab === 'ops'" v-loading="opsLoading">
      <template #header>
        <div class="flex items-center justify-between">
          <span class="text-lg font-semibold">系统运行观测</span>
          <el-button @click="loadOpsSummary">刷新</el-button>
        </div>
      </template>
      <div class="grid grid-cols-1 gap-3 md:grid-cols-3">
        <div class="cursor-pointer rounded-lg bg-slate-50 p-3 hover:bg-slate-100" @click="openOpsDetail('devices')">设备总数：<b>{{ opsSummary.device_total }}</b></div>
        <div class="cursor-pointer rounded-lg bg-slate-50 p-3 hover:bg-slate-100" @click="openOpsDetail('alerts')">开放告警：<b>{{ opsSummary.open_alert_events }}</b></div>
        <div class="cursor-pointer rounded-lg bg-slate-50 p-3 hover:bg-slate-100" @click="openOpsDetail('events')">近期事件：<b>{{ opsSummary.recent_events }}</b></div>
        <div class="cursor-pointer rounded-lg bg-slate-50 p-3 hover:bg-slate-100" @click="openOpsDetail('audits')">近期审计：<b>{{ opsSummary.recent_audits }}</b></div>
        <div class="rounded-lg bg-slate-50 p-3">最新事件时间：<b>{{ opsSummary.last_event_at || "-" }}</b></div>
        <div class="rounded-lg bg-slate-50 p-3">最新审计时间：<b>{{ opsSummary.last_audit_at || "-" }}</b></div>
        <div class="rounded-lg bg-slate-50 p-3">最新指标入库：<b>{{ opsSummary.last_metric_at || "-" }}</b></div>
        <div class="rounded-lg bg-slate-50 p-3">入库延迟：<b>{{ opsSummary.ingest_delay_sec ?? 0 }} 秒</b></div>
      </div>
      <div class="mt-4 rounded-lg border border-slate-200 p-3">
        <div class="mb-2 flex items-center justify-between">
          <span class="font-semibold">最近后台任务</span>
          <el-button size="small" @click="loadSystemJobs(false)">刷新任务</el-button>
        </div>
        <el-table :data="recentJobs" size="small" max-height="300" class="np-borderless-table">
          <el-table-column prop="created_at" label="创建时间" min-width="190" />
          <el-table-column prop="type" label="任务" width="120" />
          <el-table-column prop="status" label="状态" width="110" />
          <el-table-column prop="progress" label="进度" width="90">
            <template #default="{ row }">{{ row.progress || 0 }}%</template>
          </el-table-column>
          <el-table-column prop="message" label="说明" min-width="260" />
        </el-table>
      </div>
    </el-card>

    <el-dialog v-model="opsDetailVisible" :title="opsDetailTitle" width="960">
      <el-table v-if="opsDetailType === 'devices'" :data="opsDetailRows" class="np-borderless-table" height="520">
        <el-table-column prop="id" label="ID" width="90" />
        <el-table-column prop="name" label="名称" min-width="200" />
        <el-table-column prop="ip" label="IP" min-width="180" />
        <el-table-column prop="brand" label="品牌" width="120" />
        <el-table-column prop="status" label="状态" width="100" />
      </el-table>
      <el-table v-else-if="opsDetailType === 'alerts'" :data="opsDetailRows" class="np-borderless-table" height="520">
        <el-table-column prop="id" label="ID" width="90" />
        <el-table-column prop="level" label="级别" width="100" />
        <el-table-column prop="code" label="代码" width="160" />
        <el-table-column prop="message" label="内容" min-width="360" />
        <el-table-column prop="created_at" label="时间" width="190" />
      </el-table>
      <el-table v-else-if="opsDetailType === 'events'" :data="opsDetailRows" class="np-borderless-table" height="520">
        <el-table-column prop="id" label="ID" width="90" />
        <el-table-column prop="level" label="级别" width="100" />
        <el-table-column prop="message" label="内容" min-width="420" />
        <el-table-column prop="created_at" label="时间" width="190" />
      </el-table>
      <el-table v-else :data="opsDetailRows" class="np-borderless-table" height="520">
        <el-table-column prop="id" label="ID" width="90" />
        <el-table-column prop="action" label="动作" width="200" />
        <el-table-column prop="target" label="目标" min-width="260" />
        <el-table-column prop="timestamp" label="时间" width="190" />
      </el-table>
      <template #footer>
        <el-button type="primary" @click="opsDetailVisible = false">关闭</el-button>
      </template>
    </el-dialog>
  </div>
</template>
