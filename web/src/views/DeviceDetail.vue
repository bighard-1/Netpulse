<script setup>
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from "vue";
import { useRoute, useRouter } from "vue-router";
import { ElMessage } from "element-plus";
import { api } from "../services/api";

const props = defineProps({ id: { type: [String, Number], required: true } });

const route = useRoute();
const router = useRouter();

const loading = ref(false);
const device = ref(null);
const portKeyword = ref("");
const remarkDialogVisible = ref(false);
const remarkForm = ref({ id: null, name: "", remark: "" });
const cpuMemRef = ref(null);
let echarts = null;
let cpuMemChart = null;

const filteredPorts = computed(() => {
  const list = device.value?.interfaces || [];
  const key = portKeyword.value.trim().toLowerCase();
  if (!key) return list;
  return list.filter((p) =>
    [String(p.id), String(p.index), p.name || "", p.remark || ""]
      .join(" ")
      .toLowerCase()
      .includes(key)
  );
});

async function loadDevice() {
  loading.value = true;
  try {
    device.value = await api.getDeviceById(props.id);
    if (!device.value) return;
    await renderCpuMem();
  } finally {
    loading.value = false;
  }
}

async function renderCpuMem() {
  if (!device.value) return;
  const end = new Date();
  const start = new Date(end.getTime() - 24 * 3600 * 1000);
  const [cpuRes, memRes] = await Promise.all([
    api.getHistory("cpu", device.value.id, start.toISOString(), end.toISOString()),
    api.getHistory("mem", device.value.id, start.toISOString(), end.toISOString())
  ]);

  const cpuData = cpuRes.data.data || [];
  const memData = memRes.data.data || [];
  cpuMemChart?.setOption({
    grid: { left: 45, right: 20, top: 40, bottom: 40 },
    tooltip: { trigger: "axis" },
    legend: { data: ["CPU %", "Memory %"] },
    xAxis: { type: "category", data: cpuData.map((p) => p.timestamp) },
    yAxis: { type: "value", max: 100 },
    series: [
      { name: "CPU %", type: "line", smooth: true, data: cpuData.map((p) => Number(p.cpu_usage || 0)) },
      { name: "Memory %", type: "line", smooth: true, data: memData.map((p) => Number(p.mem_usage || 0)) }
    ]
  });
}

function openPort(port) {
  router.push({
    path: `/port/${port.id}`,
    query: {
      deviceId: String(device.value.id),
      deviceIp: device.value.ip,
      portName: port.name
    }
  });
}

function openRemark(port) {
  remarkForm.value = { id: port.id, name: port.name, remark: port.remark || "" };
  remarkDialogVisible.value = true;
}

async function saveRemark() {
  await api.updateInterfaceRemark(remarkForm.value.id, remarkForm.value.remark || "");
  ElMessage.success("端口备注已更新");
  remarkDialogVisible.value = false;
  await loadDevice();
}

onMounted(async () => {
  await nextTick();
  const m = await import("echarts");
  echarts = m;
  cpuMemChart = echarts.init(cpuMemRef.value);
  await loadDevice();
  window.addEventListener("resize", resizeChart);
});

onBeforeUnmount(() => {
  window.removeEventListener("resize", resizeChart);
  cpuMemChart?.dispose();
});

function resizeChart() {
  cpuMemChart?.resize();
}

watch(
  () => props.id,
  async () => {
    await loadDevice();
  }
);
</script>

<template>
  <div class="space-y-4" v-loading="loading">
    <el-card>
      <div v-if="device" class="grid grid-cols-1 gap-2 md:grid-cols-4">
        <div><div class="text-xs text-slate-500">设备 ID</div><div class="font-semibold">{{ device.id }}</div></div>
        <div><div class="text-xs text-slate-500">IP</div><div class="font-semibold">{{ device.ip }}</div></div>
        <div><div class="text-xs text-slate-500">品牌</div><div class="font-semibold">{{ device.brand }}</div></div>
        <div><div class="text-xs text-slate-500">备注</div><div class="font-semibold">{{ device.remark || '-' }}</div></div>
      </div>
      <el-empty v-else description="设备不存在" />
    </el-card>

    <el-card>
      <template #header>
        <span class="text-base font-semibold">CPU / 内存</span>
      </template>
      <div ref="cpuMemRef" class="h-[460px] w-full"></div>
    </el-card>

    <el-card>
      <template #header>
        <div class="flex flex-wrap items-center justify-between gap-2">
          <span class="text-base font-semibold">端口列表</span>
          <el-input v-model="portKeyword" placeholder="按 id/index/名称/备注搜索" clearable class="w-[320px]" />
        </div>
      </template>

      <el-table :data="filteredPorts" stripe>
        <el-table-column prop="id" label="ID" width="90" />
        <el-table-column prop="index" label="索引" width="100" />
        <el-table-column prop="name" label="端口名称" min-width="220" />
        <el-table-column prop="remark" label="备注" min-width="220" />
        <el-table-column label="操作" width="220">
          <template #default="{ row }">
            <el-button type="primary" text @click="openPort(row)">查看端口</el-button>
            <el-button type="warning" text @click="openRemark(row)">编辑备注</el-button>
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <el-dialog v-model="remarkDialogVisible" title="编辑端口备注" width="520">
      <el-form label-position="top">
        <el-form-item label="端口名称">
          <el-input :model-value="remarkForm.name" disabled />
        </el-form-item>
        <el-form-item label="备注">
          <el-input v-model="remarkForm.remark" type="textarea" :rows="4" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="remarkDialogVisible = false">取消</el-button>
        <el-button type="primary" @click="saveRemark">保存</el-button>
      </template>
    </el-dialog>
  </div>
</template>
