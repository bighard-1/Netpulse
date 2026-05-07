<script setup>
import { computed, onMounted, ref } from "vue";
import { ElMessage } from "element-plus";
import { Delete, Plus } from "@element-plus/icons-vue";
import { api, getApiError } from "../services/api";

const loading = ref(false);
const devices = ref([]);
const links = ref([]);
const submitting = ref(false);

const form = ref({
  src_device_id: null,
  src_if_index: null,
  dst_device_id: null,
  dst_if_index: null,
  protocol: "MANUAL",
  remark: ""
});

const deviceMap = computed(() => {
  const m = new Map();
  for (const d of devices.value) m.set(d.id, d);
  return m;
});

const srcPorts = computed(() => {
  const d = deviceMap.value.get(form.value.src_device_id);
  return d?.interfaces || [];
});

const dstPorts = computed(() => {
  const d = deviceMap.value.get(form.value.dst_device_id);
  return d?.interfaces || [];
});

const topologyView = computed(() => {
  const nodes = devices.value.map((d) => ({ id: d.id, label: d.remark || d.ip }));
  const edges = links.value.map((l) => ({
    id: l.id,
    from: l.src_device_id,
    to: l.dst_device_id,
    label: `${l.protocol || "MANUAL"} ${l.src_if_index}→${l.dst_if_index}`
  }));
  return { nodes, edges };
});

function deviceLabel(id) {
  const d = deviceMap.value.get(id);
  if (!d) return `设备#${id}`;
  return `${d.remark || "未命名设备"} (${d.ip})`;
}

function portLabel(p) {
  return `${p.name || "未命名端口"} [ifIndex:${p.index}]`;
}

async function loadDevices() {
  const res = await api.listDevices();
  devices.value = res.data || [];
}

async function loadLinks() {
  const res = await api.listTopology();
  links.value = res.data || [];
}

async function loadAll() {
  loading.value = true;
  try {
    await Promise.all([loadDevices(), loadLinks()]);
  } catch (err) {
    ElMessage.error(getApiError(err, "加载拓扑失败"));
  } finally {
    loading.value = false;
  }
}

async function createLink() {
  if (!form.value.src_device_id || !form.value.dst_device_id || !form.value.src_if_index || !form.value.dst_if_index) {
    ElMessage.warning("请完整选择源设备/源端口/目标设备/目标端口");
    return;
  }
  if (form.value.src_device_id === form.value.dst_device_id && form.value.src_if_index === form.value.dst_if_index) {
    ElMessage.warning("源端口和目标端口不能相同");
    return;
  }
  submitting.value = true;
  try {
    await api.upsertTopology(form.value);
    ElMessage.success("拓扑链路已保存");
    form.value = {
      src_device_id: null,
      src_if_index: null,
      dst_device_id: null,
      dst_if_index: null,
      protocol: "MANUAL",
      remark: ""
    };
    await loadLinks();
  } catch (err) {
    ElMessage.error(getApiError(err, "保存拓扑链路失败"));
  } finally {
    submitting.value = false;
  }
}

async function removeLink(id) {
  try {
    await api.deleteTopology(id);
    ElMessage.success("链路已删除");
    await loadLinks();
  } catch (err) {
    ElMessage.error(getApiError(err, "删除链路失败"));
  }
}

onMounted(loadAll);
</script>

<template>
  <div class="space-y-5" v-loading="loading">
    <el-card>
      <template #header>
        <div class="flex items-center justify-between">
          <span class="text-lg font-semibold">手动拓扑连线</span>
          <el-button @click="loadAll">刷新</el-button>
        </div>
      </template>

      <div class="grid grid-cols-1 gap-3 xl:grid-cols-2">
        <el-card>
          <div class="mb-3 text-sm text-slate-500">源设备</div>
          <el-select v-model="form.src_device_id" filterable placeholder="选择源设备" class="w-full" @change="form.src_if_index = null">
            <el-option v-for="d in devices" :key="d.id" :label="`${d.remark || '未命名设备'} (${d.ip})`" :value="d.id" />
          </el-select>
          <div class="mb-3 mt-4 text-sm text-slate-500">源端口</div>
          <el-select v-model="form.src_if_index" filterable placeholder="选择源端口" class="w-full">
            <el-option v-for="p in srcPorts" :key="p.id" :label="portLabel(p)" :value="p.index" />
          </el-select>
        </el-card>

        <el-card>
          <div class="mb-3 text-sm text-slate-500">目标设备</div>
          <el-select v-model="form.dst_device_id" filterable placeholder="选择目标设备" class="w-full" @change="form.dst_if_index = null">
            <el-option v-for="d in devices" :key="d.id" :label="`${d.remark || '未命名设备'} (${d.ip})`" :value="d.id" />
          </el-select>
          <div class="mb-3 mt-4 text-sm text-slate-500">目标端口</div>
          <el-select v-model="form.dst_if_index" filterable placeholder="选择目标端口" class="w-full">
            <el-option v-for="p in dstPorts" :key="p.id" :label="portLabel(p)" :value="p.index" />
          </el-select>
        </el-card>
      </div>

      <div class="mt-4 grid grid-cols-1 gap-3 xl:grid-cols-[200px,1fr,140px]">
        <el-select v-model="form.protocol" class="w-full">
          <el-option label="MANUAL" value="MANUAL" />
          <el-option label="PATCH" value="PATCH" />
          <el-option label="UPLINK" value="UPLINK" />
        </el-select>
        <el-input v-model="form.remark" placeholder="链路备注（可选）" />
        <el-button type="primary" :icon="Plus" :loading="submitting" @click="createLink">保存链路</el-button>
      </div>
    </el-card>

    <div class="grid grid-cols-1 gap-5 2xl:grid-cols-[1fr,1.4fr]">
      <el-card>
        <template #header><span class="text-lg font-semibold">拓扑概览</span></template>
        <div class="grid grid-cols-1 gap-3 md:grid-cols-2">
          <el-card>
            <div class="text-sm text-slate-500">节点数量</div>
            <div class="text-2xl font-semibold">{{ topologyView.nodes.length }}</div>
          </el-card>
          <el-card>
            <div class="text-sm text-slate-500">连线数量</div>
            <div class="text-2xl font-semibold">{{ topologyView.edges.length }}</div>
          </el-card>
        </div>
      </el-card>

      <el-card>
        <template #header><span class="text-lg font-semibold">链路列表</span></template>
        <el-table :data="links" class="np-borderless-table" height="420">
          <el-table-column label="源设备" min-width="240">
            <template #default="{ row }">{{ deviceLabel(row.src_device_id) }}</template>
          </el-table-column>
          <el-table-column prop="src_if_index" label="源端口" width="90" />
          <el-table-column label="目标设备" min-width="240">
            <template #default="{ row }">{{ deviceLabel(row.dst_device_id) }}</template>
          </el-table-column>
          <el-table-column prop="dst_if_index" label="目标端口" width="90" />
          <el-table-column prop="protocol" label="类型" width="100" />
          <el-table-column prop="remark" label="备注" min-width="180" />
          <el-table-column label="操作" width="90">
            <template #default="{ row }">
              <el-button type="danger" link :icon="Delete" @click="removeLink(row.id)" />
            </template>
          </el-table-column>
        </el-table>
      </el-card>
    </div>
  </div>
</template>
