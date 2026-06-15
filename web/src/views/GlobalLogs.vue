<script setup>
import { computed, onMounted, ref } from "vue";
import { useRoute } from "vue-router";
import { ElMessage } from "element-plus";
import { api, getApiError } from "../services/api";
import { useAuthStore } from "../stores/auth";

const route = useRoute();
const auth = useAuthStore();
const loadingAudit = ref(false);
const loadingAlerts = ref(false);
const loadingDiagnosis = ref(false);
const activeTab = ref("alerts");
const audits = ref([]);
const alertEvents = ref([]);
const assetDiagnosis = ref(null);
const alertStatus = ref("");
const workflowDialog = ref(false);
const wfForm = ref({ id: null, action: "ack", assignee: "", note: "", silence_minutes: 30 });

const alertStatusOptions = [
  { label: "全部", value: "" },
  { label: "Open", value: "open" },
  { label: "Ack", value: "ack" },
  { label: "Silenced", value: "silenced" },
  { label: "Resolved", value: "resolved" }
];

const isAdmin = computed(() => Boolean(auth.isAdmin));
const canUpdateWorkflow = computed(() => true);

function levelTagType(level) {
  const l = String(level || "").toLowerCase();
  if (l.includes("error") || l.includes("critical")) return "danger";
  if (l.includes("warn")) return "warning";
  return "success";
}

async function loadAuditLogs() {
  if (!isAdmin.value) return;
  loadingAudit.value = true;
  try {
    const res = await api.listAuditLogs();
    audits.value = res.data || [];
  } catch (err) {
    ElMessage.error(getApiError(err, "加载审计日志失败"));
  } finally {
    loadingAudit.value = false;
  }
}

async function runAssetDiagnosis() {
  if (!isAdmin.value) return;
  loadingDiagnosis.value = true;
  try {
    const res = await api.diagnoseAssetLoad();
    assetDiagnosis.value = res.data || null;
  } catch (err) {
    ElMessage.error(getApiError(err, "资产加载诊断失败"));
  } finally {
    loadingDiagnosis.value = false;
  }
}

function checkTagType(status) {
  if (status === "critical") return "danger";
  if (status === "warning") return "warning";
  return "success";
}

function refreshActiveTab() {
  if (activeTab.value === "alerts") return loadAlertEvents();
  if (activeTab.value === "audit") return loadAuditLogs();
  if (activeTab.value === "asset-diagnosis") return runAssetDiagnosis();
  return Promise.resolve();
}

async function loadAlertEvents() {
  loadingAlerts.value = true;
  try {
    const res = await api.listAlertEvents(300, alertStatus.value);
    alertEvents.value = res.data?.data || [];
  } catch (err) {
    ElMessage.error(getApiError(err, "加载告警事件失败"));
  } finally {
    loadingAlerts.value = false;
  }
}

function openWorkflow(row, action) {
  wfForm.value = {
    id: row.id,
    action,
    assignee: "",
    note: "",
    silence_minutes: 30
  };
  workflowDialog.value = true;
}

async function saveWorkflow() {
  try {
    await api.updateAlertEvent(wfForm.value.id, {
      action: wfForm.value.action,
      assignee: wfForm.value.assignee,
      note: wfForm.value.note,
      silence_minutes: wfForm.value.silence_minutes
    });
    ElMessage.success("告警工作流已更新");
    workflowDialog.value = false;
    await loadAlertEvents();
  } catch (err) {
    ElMessage.error(getApiError(err, "更新告警工作流失败"));
  }
}

onMounted(async () => {
  if (route.query.tab === "asset-diagnosis" && isAdmin.value) {
    activeTab.value = "asset-diagnosis";
  }
  await Promise.all([loadAlertEvents(), loadAuditLogs()]);
  if (activeTab.value === "asset-diagnosis") {
    await runAssetDiagnosis();
  }
});
</script>

<template>
  <el-card>
    <template #header>
      <div class="flex flex-wrap items-center justify-between gap-2">
        <span class="text-lg font-semibold">告警与日志中心</span>
        <div class="flex items-center gap-2">
          <el-select v-if="activeTab === 'alerts'" v-model="alertStatus" class="w-[140px]" @change="loadAlertEvents">
            <el-option v-for="x in alertStatusOptions" :key="x.value" :label="x.label" :value="x.value" />
          </el-select>
          <el-button @click="refreshActiveTab">刷新</el-button>
        </div>
      </div>
    </template>

    <el-tabs v-model="activeTab">
      <el-tab-pane label="事件工作台" name="alerts" />
      <el-tab-pane v-if="isAdmin" label="资产/数据库诊断" name="asset-diagnosis" />
      <el-tab-pane v-if="isAdmin" label="审计日志" name="audit" />
    </el-tabs>

    <el-table v-if="activeTab === 'alerts'" :data="alertEvents" v-loading="loadingAlerts" class="np-borderless-table" height="620">
      <el-table-column prop="created_at" label="时间" width="180" />
      <el-table-column label="级别" width="100">
        <template #default="{ row }"><el-tag size="small" :type="levelTagType(row.level)">{{ row.level }}</el-tag></template>
      </el-table-column>
      <el-table-column prop="status" label="状态" width="110" />
      <el-table-column prop="device_name" label="设备" min-width="140" />
      <el-table-column prop="code" label="代码" width="130" />
      <el-table-column prop="message" label="事件内容" min-width="260" />
      <el-table-column prop="assignee" label="负责人" width="110" />
      <el-table-column label="操作" width="260">
        <template #default="{ row }">
          <el-button text type="primary" :disabled="!canUpdateWorkflow" @click="openWorkflow(row, 'ack')">确认</el-button>
          <el-button text type="warning" :disabled="!canUpdateWorkflow" @click="openWorkflow(row, 'silence')">静默</el-button>
          <el-button text type="success" :disabled="!canUpdateWorkflow" @click="openWorkflow(row, 'resolve')">恢复</el-button>
          <el-button text :disabled="!canUpdateWorkflow" @click="openWorkflow(row, 'reopen')">重开</el-button>
        </template>
      </el-table-column>
    </el-table>

    <div v-else-if="activeTab === 'asset-diagnosis'" v-loading="loadingDiagnosis" class="space-y-4">
      <el-alert
        type="info"
        show-icon
        :closable="false"
        title="用于排查首页资产加载、图表慢查询和 TimescaleDB 聚合状态"
      >
        <template #default>
          <div class="flex flex-wrap items-center gap-2">
            <span>检测会评估资产规模、关键索引、连续聚合、最新端口指标查询、完整资产接口模拟和指标入库延迟。</span>
            <el-button size="small" type="primary" @click="runAssetDiagnosis">开始检测</el-button>
          </div>
        </template>
      </el-alert>

      <el-empty v-if="!assetDiagnosis" description="尚未执行诊断，点击“开始检测”获取结果。" />

      <template v-else>
        <div class="grid grid-cols-1 gap-3 md:grid-cols-4">
          <div class="rounded-xl bg-slate-50 p-4">
            <div class="text-xs text-slate-500">总体状态</div>
            <el-tag class="mt-2" :type="checkTagType(assetDiagnosis.overall_status)">
              {{ assetDiagnosis.overall_status }}
            </el-tag>
          </div>
          <div class="rounded-xl bg-slate-50 p-4">
            <div class="text-xs text-slate-500">设备数量</div>
            <div class="mt-1 text-2xl font-semibold">{{ assetDiagnosis.device_count || 0 }}</div>
          </div>
          <div class="rounded-xl bg-slate-50 p-4">
            <div class="text-xs text-slate-500">端口数量</div>
            <div class="mt-1 text-2xl font-semibold">{{ assetDiagnosis.interface_count || 0 }}</div>
          </div>
          <div class="rounded-xl bg-slate-50 p-4">
            <div class="text-xs text-slate-500">近1小时指标样本</div>
            <div class="mt-1 text-2xl font-semibold">{{ assetDiagnosis.recent_metric_count || 0 }}</div>
          </div>
        </div>

        <el-table :data="assetDiagnosis.checks || []" class="np-borderless-table" height="380">
          <el-table-column prop="name" label="检测项" width="180" />
          <el-table-column label="状态" width="110">
            <template #default="{ row }">
              <el-tag size="small" :type="checkTagType(row.status)">{{ row.status }}</el-tag>
            </template>
          </el-table-column>
          <el-table-column prop="duration_ms" label="耗时(ms)" width="110" />
          <el-table-column prop="detail" label="检测结果" min-width="280" />
          <el-table-column prop="suggestion" label="建议" min-width="320" />
        </el-table>

        <el-card shadow="never">
          <template #header><span class="font-semibold">综合建议</span></template>
          <ul class="space-y-2 text-sm text-slate-700">
            <li v-for="item in assetDiagnosis.suggestions || []" :key="item" class="rounded-lg bg-slate-50 p-3">
              {{ item }}
            </li>
          </ul>
        </el-card>
      </template>
    </div>

    <el-table v-else :data="audits" v-loading="loadingAudit" class="np-borderless-table" height="620">
      <el-table-column prop="timestamp" label="时间" width="190" />
      <el-table-column prop="username" label="用户名" width="120" />
      <el-table-column prop="client" label="客户端" width="100" />
      <el-table-column prop="action" label="动作" width="180" />
      <el-table-column prop="status_code" label="状态码" width="90" />
      <el-table-column prop="duration_ms" label="耗时(ms)" width="110" />
      <el-table-column prop="path" label="路径" min-width="220" />
      <el-table-column prop="ip" label="IP" width="160" />
      <el-table-column prop="target" label="详情" min-width="220" />
    </el-table>
  </el-card>

  <el-dialog v-model="workflowDialog" title="告警工作流" width="460">
    <el-form label-position="top">
      <el-form-item label="动作">
        <el-select v-model="wfForm.action" class="w-full">
          <el-option label="确认(ack)" value="ack" />
          <el-option label="静默(silence)" value="silence" />
          <el-option label="恢复(resolve)" value="resolve" />
          <el-option label="重开(reopen)" value="reopen" />
        </el-select>
      </el-form-item>
      <el-form-item label="负责人">
        <el-input v-model="wfForm.assignee" placeholder="例如: oncall-a" />
      </el-form-item>
      <el-form-item label="备注">
        <el-input v-model="wfForm.note" type="textarea" :rows="3" />
      </el-form-item>
      <el-form-item v-if="wfForm.action === 'silence'" label="静默分钟数">
        <el-input-number v-model="wfForm.silence_minutes" :min="1" :max="1440" class="w-full" />
      </el-form-item>
    </el-form>
    <template #footer>
      <el-button @click="workflowDialog = false">取消</el-button>
      <el-button type="primary" @click="saveWorkflow">提交</el-button>
    </template>
  </el-dialog>
</template>
