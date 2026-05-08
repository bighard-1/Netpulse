<script setup>
import { computed, onMounted, ref } from "vue";
import { ElMessage } from "element-plus";
import { api, getApiError } from "../services/api";

const loadingAudit = ref(false);
const loadingAlerts = ref(false);
const activeTab = ref("alerts");
const audits = ref([]);
const alertEvents = ref([]);
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

const canUpdateWorkflow = computed(() => true);

function levelTagType(level) {
  const l = String(level || "").toLowerCase();
  if (l.includes("error") || l.includes("critical")) return "danger";
  if (l.includes("warn")) return "warning";
  return "success";
}

async function loadAuditLogs() {
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
  await Promise.all([loadAlertEvents(), loadAuditLogs()]);
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
          <el-button @click="activeTab === 'alerts' ? loadAlertEvents() : loadAuditLogs()">刷新</el-button>
        </div>
      </div>
    </template>

    <el-tabs v-model="activeTab">
      <el-tab-pane label="事件工作台" name="alerts" />
      <el-tab-pane label="审计日志" name="audit" />
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
