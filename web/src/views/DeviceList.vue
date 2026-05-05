<script setup>
import { onMounted, ref } from "vue";
import { useRouter } from "vue-router";
import { ElMessage } from "element-plus";
import { api } from "../services/api";

const router = useRouter();
const loading = ref(false);
const devices = ref([]);
const addVisible = ref(false);
const addLoading = ref(false);
const addForm = ref({ ip: "", brand: "H3C", community: "public", remark: "" });

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

async function addDevice() {
  addLoading.value = true;
  try {
    await api.addDevice(addForm.value);
    ElMessage.success("资产添加成功");
    addVisible.value = false;
    addForm.value = { ip: "", brand: "H3C", community: "public", remark: "" };
    await loadDevices();
  } finally {
    addLoading.value = false;
  }
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
        <div class="flex items-center gap-2">
          <el-button type="primary" @click="addVisible = true">添加资产</el-button>
          <el-button @click="loadDevices">刷新</el-button>
        </div>
      </div>
    </template>

    <el-table v-loading="loading" :data="devices">
      <el-table-column label="状态" width="90">
        <template #default="{ row }">
          <el-tooltip :content="row.status_reason || '暂无状态说明'" placement="top">
            <span
              class="inline-block h-2.5 w-2.5 rounded-full"
              :class="row.status === 'online' ? 'bg-emerald-500' : (row.status === 'offline' ? 'bg-rose-500' : 'bg-amber-400')"
            />
          </el-tooltip>
        </template>
      </el-table-column>
      <el-table-column label="状态说明" min-width="240">
        <template #default="{ row }">
          <span class="text-slate-600">{{ row.status_reason || "-" }}</span>
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

    <el-dialog v-model="addVisible" title="添加资产" width="520">
      <el-form label-position="top">
        <el-form-item label="设备 IP">
          <el-input v-model="addForm.ip" placeholder="例如 172.24.134.45" />
        </el-form-item>
        <el-form-item label="品牌">
          <el-select v-model="addForm.brand" class="w-full">
            <el-option label="H3C" value="H3C" />
            <el-option label="Huawei" value="Huawei" />
          </el-select>
        </el-form-item>
        <el-form-item label="SNMP Community">
          <el-input v-model="addForm.community" placeholder="默认 public" />
        </el-form-item>
        <el-form-item label="备注">
          <el-input v-model="addForm.remark" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="addVisible = false">取消</el-button>
        <el-button type="primary" :loading="addLoading" @click="addDevice">保存</el-button>
      </template>
    </el-dialog>
  </el-card>
</template>
