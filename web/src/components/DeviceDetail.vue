<script setup>
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from "vue";
import * as echarts from "echarts";
import { api } from "../services/api";

const props = defineProps({
  device: {
    type: Object,
    required: true
  }
});

const range = ref([]);
const loading = ref(false);
const cpuMemRef = ref(null);
let cpuMemChart = null;
let timer = null;

const logs = ref([]);
const interfaces = ref([]);

const timeShortcuts = [
  {
    text: "Last 1 Hour",
    value: () => [new Date(Date.now() - 3600 * 1000), new Date()]
  },
  {
    text: "Last 24 Hours",
    value: () => [new Date(Date.now() - 24 * 3600 * 1000), new Date()]
  },
  {
    text: "Last 7 Days",
    value: () => [new Date(Date.now() - 7 * 24 * 3600 * 1000), new Date()]
  }
];

const rangeParams = computed(() => {
  if (!range.value?.length) {
    const end = new Date();
    const start = new Date(end.getTime() - 24 * 3600 * 1000);
    return { start, end };
  }
  return { start: range.value[0], end: range.value[1] };
});

const interfaceCards = computed(() =>
  interfaces.value.map((itf) => {
    const last = itf.data[itf.data.length - 1] || {};
    return {
      id: itf.id,
      name: itf.name || `if-${itf.id}`,
      inBps: Number(last.traffic_in_bps || 0),
      outBps: Number(last.traffic_out_bps || 0)
    };
  })
);

function fmtBps(v) {
  if (v > 1e9) return `${(v / 1e9).toFixed(2)} Gbps`;
  if (v > 1e6) return `${(v / 1e6).toFixed(2)} Mbps`;
  if (v > 1e3) return `${(v / 1e3).toFixed(2)} Kbps`;
  return `${v.toFixed(0)} bps`;
}

async function fetchChart() {
  const { start, end } = rangeParams.value;
  const [cpuRes, memRes] = await Promise.all([
    api.getHistory("cpu", props.device.id, start.toISOString(), end.toISOString()),
    api.getHistory("mem", props.device.id, start.toISOString(), end.toISOString())
  ]);

  const cpuData = cpuRes.data.data || [];
  const memData = memRes.data.data || [];
  const xAxis = cpuData.map((p) => p.timestamp);
  const cpu = cpuData.map((p) => Number(p.cpu_usage || 0));
  const mem = memData.map((p) => Number(p.mem_usage || 0));

  cpuMemChart?.setOption({
    tooltip: { trigger: "axis" },
    legend: { data: ["CPU %", "Memory %"] },
    xAxis: { type: "category", data: xAxis },
    yAxis: { type: "value", max: 100 },
    series: [
      { name: "CPU %", type: "line", smooth: true, data: cpu },
      { name: "Memory %", type: "line", smooth: true, data: mem }
    ]
  });
}

async function fetchInterfaces() {
  const { start, end } = rangeParams.value;
  const list = props.device?.interfaces || [];
  const tasks = list.map(async (itf) => {
    const his = await api.getHistory(
      "traffic",
      itf.id,
      start.toISOString(),
      end.toISOString()
    );
    return { ...itf, data: his.data.data || [] };
  });
  interfaces.value = await Promise.all(tasks);
}

async function fetchLogs() {
  const res = await api.getDeviceLogs(props.device.id);
  logs.value = res.data || [];
}

async function refreshAll() {
  loading.value = true;
  try {
    await Promise.all([fetchChart(), fetchInterfaces(), fetchLogs()]);
  } finally {
    loading.value = false;
  }
}

onMounted(async () => {
  await nextTick();
  cpuMemChart = echarts.init(cpuMemRef.value);
  await refreshAll();
  timer = setInterval(refreshAll, 60000);
  window.addEventListener("resize", () => cpuMemChart?.resize());
});

onBeforeUnmount(() => {
  if (timer) clearInterval(timer);
  cpuMemChart?.dispose();
});

watch(
  () => props.device.id,
  () => refreshAll()
);
</script>

<template>
  <div class="space-y-4">
    <el-card>
      <div class="flex flex-wrap items-center justify-between gap-3">
        <div>
          <div class="text-lg font-semibold">Device #{{ device.id }} - {{ device.ip }}</div>
          <div class="text-sm text-slate-500">{{ device.brand }} | {{ device.remark || "-" }}</div>
        </div>
        <el-date-picker
          v-model="range"
          type="datetimerange"
          unlink-panels
          range-separator="to"
          start-placeholder="Start"
          end-placeholder="End"
          :shortcuts="timeShortcuts"
          @change="refreshAll"
        />
      </div>
    </el-card>

    <el-card v-loading="loading">
      <template #header>
        <span>CPU & Memory</span>
      </template>
      <div ref="cpuMemRef" class="h-80 w-full"></div>
    </el-card>

    <div class="grid grid-cols-1 gap-3 md:grid-cols-2 xl:grid-cols-3">
      <el-card v-for="itf in interfaceCards" :key="itf.id">
        <template #header>
          <span>{{ itf.name }}</span>
        </template>
        <div class="space-y-1 text-sm">
          <div>In: <span class="font-semibold text-emerald-600">{{ fmtBps(itf.inBps) }}</span></div>
          <div>Out: <span class="font-semibold text-blue-600">{{ fmtBps(itf.outBps) }}</span></div>
        </div>
      </el-card>
    </div>

    <el-card>
      <template #header>
        <span>Last 100 Logs</span>
      </template>
      <el-table :data="logs" height="320">
        <el-table-column prop="created_at" label="Time" min-width="180" />
        <el-table-column prop="level" label="Level" width="100" />
        <el-table-column prop="message" label="Message" min-width="300" />
      </el-table>
    </el-card>
  </div>
</template>
