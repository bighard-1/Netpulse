<script setup>
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from "vue";
import { useRouter } from "vue-router";
import { ElMessage } from "element-plus";
import { Edit, View } from "@element-plus/icons-vue";
import { api, getApiError } from "../services/api";

const props = defineProps({ id: { type: [String, Number], required: true } });
const router = useRouter();

const loading = ref(false);
const chartLoading = ref(false);
const device = ref(null);
const portKeyword = ref("");
const remarkDialogVisible = ref(false);
const interfaceEditForm = ref({ id: null, name: "", remark: "" });
const showPortID = ref(false);
const showPortIndex = ref(false);
const diagnoseVisible = ref(false);
const diagnoseLoading = ref(false);
const diagnoseReport = ref(null);
const cpuMemRef = ref(null);
let cpuMemChart = null;

const filteredPorts = computed(() => {
  const list = device.value?.interfaces || [];
  const key = portKeyword.value.trim().toLowerCase();
  if (!key) return list;
  return list.filter((p) => [String(p.id), String(p.index), p.name || "", p.remark || ""].join(" ").toLowerCase().includes(key));
});

async function loadDevice() {
  loading.value = true;
  try {
    device.value = await api.getDeviceById(props.id);
    if (!device.value) return;
    await renderCpuMem();
  } catch (err) {
    ElMessage.error(getApiError(err, "加载设备详情失败"));
  } finally {
    loading.value = false;
  }
}

function initCpuChart() {
  if (!cpuMemChart) return;
  cpuMemChart.setOption({
    animation: false,
    grid: { left: 45, right: 20, top: 34, bottom: 30 },
    tooltip: { trigger: "axis", axisPointer: { type: "line", animation: false } },
    legend: { data: ["CPU利用率", "内存利用率"], top: 4 },
    xAxis: { type: "time" },
    yAxis: { type: "value", max: 100, axisLabel: { formatter: (v) => `${v}%` } },
    series: [
      { name: "CPU利用率", type: "line", showSymbol: false, sampling: "lttb", progressive: 2000, data: [] },
      { name: "内存利用率", type: "line", showSymbol: false, sampling: "lttb", progressive: 2000, data: [] }
    ]
  }, true);
}

async function renderCpuMem() {
  if (!device.value || !cpuMemChart) return;
  chartLoading.value = true;
  try {
    const end = new Date();
    const start = new Date(end.getTime() - 24 * 3600 * 1000);
    const [cpuRes, memRes] = await Promise.all([
      api.getHistory("cpu", device.value.id, start.toISOString(), end.toISOString()),
      api.getHistory("mem", device.value.id, start.toISOString(), end.toISOString())
    ]);

    const cpuData = (cpuRes.data.data || []).map((p) => [new Date(p.timestamp).getTime(), Number(p.cpu_usage || 0)]);
    const memData = (memRes.data.data || []).map((p) => [new Date(p.timestamp).getTime(), Number(p.mem_usage || 0)]);
    const hasData = cpuData.length || memData.length;

    cpuMemChart.setOption({
      graphic: hasData ? [] : [{
        type: "text",
        left: "center",
        top: "middle",
        style: { text: "当前时间范围暂无 CPU/内存 数据", fill: "#94a3b8", fontSize: 14 }
      }],
      series: [
        { name: "CPU利用率", data: cpuData },
        { name: "内存利用率", data: memData }
      ]
    });
  } catch (err) {
    ElMessage.error(getApiError(err, "加载性能图表失败"));
  } finally {
    chartLoading.value = false;
  }
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
  interfaceEditForm.value = { id: port.id, name: port.name, remark: port.remark || "" };
  remarkDialogVisible.value = true;
}

async function saveRemark() {
  try {
    await api.updateInterfaceProfile(interfaceEditForm.value.id, {
      name: interfaceEditForm.value.name || "",
      remark: interfaceEditForm.value.remark || ""
    });
    ElMessage.success("端口信息已更新");
    remarkDialogVisible.value = false;
    await loadDevice();
  } catch (err) {
    ElMessage.error(getApiError(err, "保存端口信息失败"));
  }
}

async function runDiagnosis() {
  diagnoseLoading.value = true;
  try {
    const res = await api.diagnoseDevice(props.id);
    diagnoseReport.value = res.data || null;
    diagnoseVisible.value = true;
  } catch (err) {
    ElMessage.error(getApiError(err, "执行诊断失败"));
  } finally {
    diagnoseLoading.value = false;
  }
}

async function exportDiagnosis(format) {
  try {
    const res = await api.exportDiagnosis(props.id, format);
    const blobUrl = URL.createObjectURL(new Blob([res.data]));
    const a = document.createElement("a");
    a.href = blobUrl;
    a.download = `netpulse_diagnose_device_${props.id}.${format}`;
    a.click();
    URL.revokeObjectURL(blobUrl);
  } catch (err) {
    ElMessage.error(getApiError(err, "导出诊断报告失败"));
  }
}

function resizeChart() {
  cpuMemChart?.resize();
}

onMounted(async () => {
  await nextTick();
  const m = await import("echarts");
  cpuMemChart = m.init(cpuMemRef.value);
  initCpuChart();
  await loadDevice();
  window.addEventListener("resize", resizeChart);
});

onBeforeUnmount(() => {
  window.removeEventListener("resize", resizeChart);
  cpuMemChart?.dispose();
});

watch(
  () => props.id,
  async () => {
    await loadDevice();
  }
);
</script>

<template>
  <div class="space-y-5" v-loading="loading">
    <el-breadcrumb separator=">">
      <el-breadcrumb-item :to="{ path: '/' }">资产</el-breadcrumb-item>
      <el-breadcrumb-item>{{ device?.ip || `设备-${props.id}` }}</el-breadcrumb-item>
    </el-breadcrumb>

    <el-card>
      <div v-if="device" class="grid grid-cols-1 gap-3 md:grid-cols-4">
        <div><div class="text-xs text-slate-500">设备 ID</div><div class="font-semibold">{{ device.id }}</div></div>
        <div><div class="text-xs text-slate-500">IP</div><div class="font-semibold">{{ device.ip }}</div></div>
        <div><div class="text-xs text-slate-500">品牌</div><div class="font-semibold">{{ device.brand }}</div></div>
        <div><div class="text-xs text-slate-500">备注</div><div class="font-semibold">{{ device.remark || '-' }}</div></div>
      </div>
      <el-empty v-else description="设备不存在" />
    </el-card>

    <el-card>
      <template #header>
        <div class="flex items-center justify-between">
          <span class="text-base font-semibold">设备诊断</span>
          <el-button type="primary" :loading="diagnoseLoading" @click="runDiagnosis">一键自助排查</el-button>
        </div>
      </template>
      <p class="text-sm text-slate-600">自动检查 SNMP 参数、网络连通、即时采集、最近入库与错误日志，并给出可能原因。</p>
    </el-card>

    <el-card>
      <template #header>
        <span class="text-base font-semibold">CPU / 内存实时利用率（24h）</span>
      </template>
      <div ref="cpuMemRef" class="h-[180px] w-full" v-loading="chartLoading"></div>
    </el-card>

    <el-card>
      <template #header>
        <div class="flex flex-wrap items-center justify-between gap-2">
          <span class="text-base font-semibold">端口列表</span>
          <div class="flex items-center gap-2">
            <el-input v-model="portKeyword" placeholder="按 id/index/名称/备注搜索" clearable class="w-[320px]" />
            <el-checkbox v-model="showPortID">显示ID</el-checkbox>
            <el-checkbox v-model="showPortIndex">显示索引</el-checkbox>
          </div>
        </div>
      </template>

      <el-skeleton :loading="loading" animated :rows="8">
        <template #default>
          <el-table :data="filteredPorts" class="np-borderless-table">
            <el-table-column v-if="showPortID" prop="id" label="ID" width="90" />
            <el-table-column v-if="showPortIndex" prop="index" label="索引" width="100" />
            <el-table-column prop="name" label="端口名称" min-width="220" />
            <el-table-column prop="remark" label="备注" min-width="220" />
            <el-table-column label="操作" width="140">
              <template #default="{ row }">
                <el-button type="primary" link :icon="View" @click="openPort(row)" />
                <el-button type="warning" link :icon="Edit" @click="openRemark(row)" />
              </template>
            </el-table-column>
          </el-table>
        </template>
      </el-skeleton>
    </el-card>

    <el-dialog v-model="remarkDialogVisible" title="编辑端口信息" width="520">
      <el-form label-position="top">
        <el-form-item label="端口名称（同资产内不可重复）"><el-input v-model="interfaceEditForm.name" /></el-form-item>
        <el-form-item label="备注"><el-input v-model="interfaceEditForm.remark" type="textarea" :rows="4" /></el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="remarkDialogVisible = false">取消</el-button>
        <el-button type="primary" @click="saveRemark">保存</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="diagnoseVisible" title="设备自助排查报告" width="860">
      <div v-if="diagnoseReport" class="space-y-3">
        <div class="grid grid-cols-1 gap-2 md:grid-cols-2">
          <div><span class="text-slate-500">设备:</span> {{ diagnoseReport.device_ip }} (ID: {{ diagnoseReport.device_id }})</div>
          <div><span class="text-slate-500">总体状态:</span> <span class="font-semibold">{{ diagnoseReport.overall_status }}</span></div>
          <div class="md:col-span-2"><span class="text-slate-500">可能原因:</span> {{ diagnoseReport.likely_cause }}</div>
        </div>
        <el-table :data="diagnoseReport.checks || []" class="np-borderless-table">
          <el-table-column prop="name" label="检查项" min-width="180" />
          <el-table-column label="结果" width="100">
            <template #default="{ row }">
              <el-tag :type="row.status === 'pass' ? 'success' : (row.status === 'warn' ? 'warning' : 'danger')">{{ row.status }}</el-tag>
            </template>
          </el-table-column>
          <el-table-column prop="message" label="说明" min-width="420" />
        </el-table>
      </div>
      <template #footer>
        <el-button @click="exportDiagnosis('txt')">导出 TXT</el-button>
        <el-button @click="exportDiagnosis('json')">导出 JSON</el-button>
        <el-button type="primary" @click="diagnoseVisible = false">关闭</el-button>
      </template>
    </el-dialog>
  </div>
</template>
