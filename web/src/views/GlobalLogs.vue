<script setup>
import { onMounted, ref } from "vue";
import { ElMessage } from "element-plus";
import { api, getApiError } from "../services/api";

const loading = ref(false);
const logs = ref([]);

async function loadLogs() {
  loading.value = true;
  try {
    const res = await api.listAuditLogs();
    logs.value = res.data || [];
  } catch (err) {
    ElMessage.error(getApiError(err, "加载日志失败"));
  } finally {
    loading.value = false;
  }
}

onMounted(loadLogs);
</script>

<template>
  <el-card>
    <template #header>
      <div class="flex items-center justify-between">
        <span class="text-lg font-semibold">Global Logs</span>
        <el-button @click="loadLogs">刷新</el-button>
      </div>
    </template>
    <el-skeleton :loading="loading" animated :rows="10">
      <template #default>
        <el-table :data="logs" class="np-borderless-table" height="650">
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
      </template>
    </el-skeleton>
  </el-card>
</template>
