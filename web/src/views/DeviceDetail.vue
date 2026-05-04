<script setup>
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from "vue";
import { useRoute, useRouter } from "vue-router";
import * as echarts from "echarts";
import { api } from "../services/api";

const props = defineProps({ id: { type: [String, Number], required: true } });

const route = useRoute();
const router = useRouter();

const loading = ref(false);
const device = ref(null);
const portKeyword = ref("");
const cpuMemRef = ref(null);
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

onMounted(async () => {
  await nextTick();
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
        <div><div class="text-xs text-slate-500">Device ID</div><div class="font-semibold">{{ device.id }}</div></div>
        <div><div class="text-xs text-slate-500">IP</div><div class="font-semibold">{{ device.ip }}</div></div>
        <div><div class="text-xs text-slate-500">Brand</div><div class="font-semibold">{{ device.brand }}</div></div>
        <div><div class="text-xs text-slate-500">Remark</div><div class="font-semibold">{{ device.remark || '-' }}</div></div>
      </div>
      <el-empty v-else description="设备不存在" />
    </el-card>

    <el-card>
      <template #header>
        <span class="text-base font-semibold">CPU / Memory</span>
      </template>
      <div ref="cpuMemRef" class="h-[460px] w-full"></div>
    </el-card>

    <el-card>
      <template #header>
        <div class="flex flex-wrap items-center justify-between gap-2">
          <span class="text-base font-semibold">Ports</span>
          <el-input v-model="portKeyword" placeholder="Search by id/index/name/remark" clearable class="w-[320px]" />
        </div>
      </template>

      <el-table :data="filteredPorts" stripe>
        <el-table-column prop="id" label="ID" width="90" />
        <el-table-column prop="index" label="Index" width="100" />
        <el-table-column prop="name" label="Name" min-width="220" />
        <el-table-column prop="remark" label="Remark" min-width="220" />
        <el-table-column label="Action" width="140">
          <template #default="{ row }">
            <el-button type="primary" text @click="openPort(row)">View Port</el-button>
          </template>
        </el-table-column>
      </el-table>
    </el-card>
  </div>
</template>
