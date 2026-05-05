<script setup>
import { onMounted, ref } from "vue";
import { useRouter } from "vue-router";
import { ElMessage } from "element-plus";
import { api } from "../services/api";

const router = useRouter();
const loading = ref(false);
const devices = ref([]);

async function loadDevices() {
  loading.value = true;
  try {
    const res = await api.listDevices();
    devices.value = res.data || [];
  } finally {
    loading.value = false;
  }
}

async function deleteDevice(id) {
  await api.deleteDevice(id);
  ElMessage.success("设备已删除");
  await loadDevices();
}

function goDetail(row) {
  router.push({ path: `/device/${row.id}`, query: { ip: row.ip } });
}

onMounted(loadDevices);
</script>

<template>
  <el-card>
    <template #header>
      <div class="flex items-center justify-between">
        <span class="text-lg font-semibold">资产列表</span>
        <el-button @click="loadDevices">刷新</el-button>
      </div>
    </template>

    <el-table v-loading="loading" :data="devices">
      <el-table-column label="状态" width="90">
        <template #default="{ row }">
          <span
            class="inline-block h-2.5 w-2.5 rounded-full"
            :class="row.status === 'online' ? 'bg-emerald-500' : (row.status === 'offline' ? 'bg-rose-500' : 'bg-amber-400')"
          />
        </template>
      </el-table-column>
      <el-table-column prop="ip" label="IP" min-width="180" />
      <el-table-column prop="brand" label="品牌" width="140" />
      <el-table-column prop="remark" label="备注" min-width="240" />
      <el-table-column label="操作" width="200">
        <template #default="{ row }">
          <el-button type="primary" text @click="goDetail(row)">查看详情</el-button>
          <el-button type="danger" text @click="deleteDevice(row.id)">删除</el-button>
        </template>
      </el-table-column>
    </el-table>
  </el-card>
</template>
