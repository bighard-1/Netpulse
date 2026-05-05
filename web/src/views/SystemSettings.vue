<script setup>
import { ref } from "vue";
import { ElMessage } from "element-plus";
import { api, getApiError } from "../services/api";

const restoreLoading = ref(false);
const drillLoading = ref(false);

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
  } catch (err) {
    ElMessage.error(getApiError(err, "备份演练失败"));
  } finally {
    drillLoading.value = false;
  }
}
</script>

<template>
  <div class="grid grid-cols-1 gap-4 md:grid-cols-2">
    <el-card>
      <template #header><span class="text-lg font-semibold">Backup / Restore</span></template>
      <div class="space-y-3">
        <el-button type="primary" @click="onBackup">下载备份</el-button>
        <el-upload :auto-upload="false" :show-file-list="false" accept=".gz" :on-change="onRestore" :disabled="restoreLoading">
          <el-button>恢复数据</el-button>
        </el-upload>
      </div>
    </el-card>

    <el-card>
      <template #header><span class="text-lg font-semibold">Backup Drill</span></template>
      <el-button :loading="drillLoading" @click="runBackupDrill">执行备份可恢复性演练</el-button>
    </el-card>
  </div>
</template>
