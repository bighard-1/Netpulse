<script setup>
import { computed, onBeforeUnmount, onMounted, ref } from "vue";
import { ElMessage, ElMessageBox } from "element-plus";
import { Delete, Plus, Refresh, ZoomIn, ZoomOut } from "@element-plus/icons-vue";
import { useAuthStore } from "../stores/auth";
import { api, getApiError } from "../services/api";
import TopologyCanvas from "../components/topology/TopologyCanvas.vue";

const auth = useAuthStore();
const graphRef = ref(null);
const loading = ref(false);
const saving = ref(false);
const devices = ref([]);
const graph = ref({ nodes: [], edges: [] });
const editMode = ref(localStorage.getItem("np_edit_mode") === "1");
const tooltipFontSize = ref(Number(localStorage.getItem("np_topology_tooltip_font_size") || 18));
const pendingNodePositions = ref({});
function loadStoredLabelTiers() {
  try {
    const parsed = JSON.parse(localStorage.getItem("np_topology_label_display_tiers") || "[\"core\"]");
    return Array.isArray(parsed) ? parsed : ["core"];
  } catch {
    return ["core"];
  }
}

const labelDisplayMode = ref(localStorage.getItem("np_topology_label_display_mode") || "hover");
const labelDisplayTiers = ref(loadStoredLabelTiers());

const labelModeOptions = [
  { label: "鼠标悬浮显示", value: "hover" },
  { label: "常显", value: "always" },
  { label: "自定义", value: "custom" }
];

const tierOptions = [
  { label: "核心层", value: "core" },
  { label: "汇聚层", value: "aggregation" },
  { label: "接入层", value: "access" }
];

const nodeForm = ref({ device_id: null, label: "" });
const edgeForm = ref({ source_node_id: null, target_node_id: null, label: "", remark: "" });

const isAdmin = computed(() => Boolean(auth.isAdmin));
const canEdit = computed(() => Boolean(isAdmin.value && editMode.value));
const nodeDeviceIds = computed(() => new Set((graph.value.nodes || []).map((n) => Number(n.device_id))));
const availableDevices = computed(() => (devices.value || []).filter((d) => !nodeDeviceIds.value.has(Number(d.id))));
const nodeOptions = computed(() => (graph.value.nodes || []).map((n) => ({ label: `${n.label || n.device_name || n.device_ip} (${n.device_ip || "-"})`, value: n.id })));
const manageableNodes = computed(() => (graph.value.nodes || []).map((n) => ({
  ...n,
  displayLabel: n.label || n.device_name || n.device_ip || `节点-${n.id}`,
  displayIp: n.device_ip || "-"
})));
const hasUnsavedLayout = computed(() => Object.keys(pendingNodePositions.value).length > 0);

function onTooltipFontSizeChange(value) {
  tooltipFontSize.value = Number(value || 18);
  localStorage.setItem("np_topology_tooltip_font_size", String(tooltipFontSize.value));
  window.dispatchEvent(new CustomEvent("np-topology-settings-changed"));
}

function onLabelDisplayModeChange(value) {
  labelDisplayMode.value = value || "hover";
  localStorage.setItem("np_topology_label_display_mode", labelDisplayMode.value);
  window.dispatchEvent(new CustomEvent("np-topology-settings-changed"));
}

function onLabelDisplayTiersChange(value) {
  labelDisplayTiers.value = Array.isArray(value) ? value : [];
  localStorage.setItem("np_topology_label_display_tiers", JSON.stringify(labelDisplayTiers.value));
  window.dispatchEvent(new CustomEvent("np-topology-settings-changed"));
}

function deviceOptionLabel(d) {
  return `${d.name || d.ip} (${d.ip})`;
}

function layoutPoint() {
  const count = graph.value.nodes?.length || 0;
  const angle = (count / Math.max(1, count + 1)) * Math.PI * 2;
  return {
    x: Math.round(500 + Math.cos(angle) * 260),
    y: Math.round(300 + Math.sin(angle) * 180)
  };
}

async function loadAll() {
  loading.value = true;
  try {
    const [devRes, topoRes] = await Promise.all([api.listDevices(), api.getTopology()]);
    devices.value = devRes.data || [];
    graph.value = { nodes: topoRes.data?.nodes || [], edges: topoRes.data?.edges || [] };
    pendingNodePositions.value = {};
  } catch (err) {
    ElMessage.error(getApiError(err, "加载拓扑失败"));
  } finally {
    loading.value = false;
  }
}

async function addNode() {
  if (!canEdit.value) return;
  if (!nodeForm.value.device_id) return ElMessage.warning("请选择要加入拓扑的资产");
  const p = layoutPoint();
  saving.value = true;
  try {
    await api.addTopologyNode({ device_id: nodeForm.value.device_id, label: nodeForm.value.label, x: p.x, y: p.y });
    nodeForm.value = { device_id: null, label: "" };
    ElMessage.success("拓扑节点已添加");
    await loadAll();
  } catch (err) {
    ElMessage.error(getApiError(err, "添加拓扑节点失败"));
  } finally {
    saving.value = false;
  }
}

async function addEdge() {
  if (!canEdit.value) return;
  if (!edgeForm.value.source_node_id || !edgeForm.value.target_node_id) return ElMessage.warning("请选择源节点和目标节点");
  if (edgeForm.value.source_node_id === edgeForm.value.target_node_id) return ElMessage.warning("源节点和目标节点不能相同");
  saving.value = true;
  try {
    await api.addTopologyEdge(edgeForm.value);
    edgeForm.value = { source_node_id: null, target_node_id: null, label: "", remark: "" };
    ElMessage.success("拓扑连线已保存");
    await loadAll();
  } catch (err) {
    ElMessage.error(getApiError(err, "保存拓扑连线失败"));
  } finally {
    saving.value = false;
  }
}

async function saveNodePosition(node) {
  if (!canEdit.value) return;
  graph.value = {
    ...graph.value,
    nodes: (graph.value.nodes || []).map((n) => Number(n.id) === Number(node.id) ? { ...n, x: node.x, y: node.y } : n)
  };
  pendingNodePositions.value = {
    ...pendingNodePositions.value,
    [node.id]: { id: node.id, label: node.label || "", x: node.x, y: node.y }
  };
}

async function saveTopologyLayout() {
  if (!canEdit.value || !hasUnsavedLayout.value) return;
  saving.value = true;
  try {
    const changes = Object.values(pendingNodePositions.value);
    await Promise.all(changes.map((node) => api.updateTopologyNode(node.id, { label: node.label || "", x: node.x, y: node.y })));
    pendingNodePositions.value = {};
    ElMessage.success("拓扑布局已保存");
    await loadAll();
  } catch (err) {
    ElMessage.error(getApiError(err, "保存拓扑布局失败"));
  } finally {
    saving.value = false;
  }
}

async function cancelTopologyLayout() {
  if (!hasUnsavedLayout.value) return;
  pendingNodePositions.value = {};
  await loadAll();
  graphRef.value?.fit?.();
  ElMessage.info("已取消未保存的布局调整");
}

async function removeNode(node) {
  if (!canEdit.value) return;
  try {
    await ElMessageBox.confirm(`确定从拓扑中移除 ${node.label || node.device_name || node.device_ip} 吗？相关连线也会移除。`, "移除拓扑节点", { type: "warning" });
    await api.deleteTopologyNode(node.id);
    ElMessage.success("拓扑节点已移除");
    await loadAll();
  } catch (err) {
    if (err !== "cancel") ElMessage.error(getApiError(err, "移除拓扑节点失败"));
  }
}

async function removeEdge(edge) {
  if (!canEdit.value) return;
  try {
    await api.deleteTopologyEdge(edge.id);
    ElMessage.success("拓扑连线已删除");
    await loadAll();
  } catch (err) {
    ElMessage.error(getApiError(err, "删除拓扑连线失败"));
  }
}

function onEditMode(e) {
  editMode.value = Boolean(e?.detail?.enabled);
}

onMounted(() => {
  loadAll();
  window.addEventListener("np-edit-mode", onEditMode);
});

onBeforeUnmount(() => {
  window.removeEventListener("np-edit-mode", onEditMode);
});
</script>

<template>
  <div class="space-y-5">
    <el-card>
      <template #header>
        <div class="flex flex-wrap items-center justify-between gap-3">
          <div>
            <div class="text-lg font-semibold text-slate-900">拓扑图管理</div>
            <div class="mt-1 text-xs text-slate-500">手动维护资产节点与连线，节点状态随资产在线/离线自动变化</div>
          </div>
          <div class="flex flex-wrap items-center justify-end gap-2">
            <span class="text-xs text-slate-500">名称显示</span>
            <el-radio-group :model-value="labelDisplayMode" size="small" @update:model-value="onLabelDisplayModeChange">
              <el-radio-button v-for="item in labelModeOptions" :key="item.value" :label="item.value">{{ item.label }}</el-radio-button>
            </el-radio-group>
            <el-checkbox-group
              v-if="labelDisplayMode === 'custom'"
              :model-value="labelDisplayTiers"
              size="small"
              @update:model-value="onLabelDisplayTiersChange"
            >
              <el-checkbox-button v-for="item in tierOptions" :key="item.value" :label="item.value">{{ item.label }}</el-checkbox-button>
            </el-checkbox-group>
            <span class="text-xs text-slate-500">悬浮字号</span>
            <el-input-number
              :model-value="tooltipFontSize"
              :min="14"
              :max="28"
              :step="1"
              size="small"
              controls-position="right"
              style="width: 104px"
              @update:model-value="onTooltipFontSizeChange"
            />
            <el-button :icon="ZoomOut" @click="graphRef?.zoomOut()">缩小</el-button>
            <el-button :icon="ZoomIn" @click="graphRef?.zoomIn()">放大</el-button>
            <el-button @click="graphRef?.fit()">自适应</el-button>
            <el-button v-if="canEdit" type="primary" :disabled="!hasUnsavedLayout" :loading="saving" @click="saveTopologyLayout">保存布局</el-button>
            <el-button v-if="canEdit" :disabled="!hasUnsavedLayout || saving" @click="cancelTopologyLayout">取消</el-button>
            <el-button :icon="Refresh" @click="loadAll">刷新</el-button>
          </div>
        </div>
      </template>
      <TopologyCanvas
        ref="graphRef"
        :nodes="graph.nodes"
        :edges="graph.edges"
        :editable="canEdit"
        :loading="loading"
        :height="520"
        :clickable="false"
        :tooltip-font-size="tooltipFontSize"
        :label-display-mode="labelDisplayMode"
        :label-display-tiers="labelDisplayTiers"
        auto-fit
        @node-move="saveNodePosition"
        @node-delete="removeNode"
        @edge-delete="removeEdge"
      />
    </el-card>

    <el-alert v-if="!isAdmin" type="info" show-icon :closable="false" title="当前为普通用户，只能查看拓扑，管理按钮已隐藏。" />
    <el-alert v-else-if="!editMode" type="warning" show-icon :closable="false" title="当前为只读模式，请在左侧快捷操作中进入编辑模式后再修改拓扑。" />

    <el-card v-if="isAdmin && editMode">
      <template #header><span class="text-lg font-semibold">编辑拓扑</span></template>
      <div class="grid grid-cols-1 gap-4 2xl:grid-cols-2">
        <div class="rounded-xl border border-slate-200 bg-slate-50 p-4">
          <div class="mb-3 font-semibold text-slate-800">添加资产节点</div>
          <div class="grid grid-cols-1 gap-3 md:grid-cols-[1fr,220px,120px]">
            <el-select v-model="nodeForm.device_id" filterable placeholder="选择已添加资产" class="w-full">
              <el-option v-for="d in availableDevices" :key="d.id" :label="deviceOptionLabel(d)" :value="d.id" />
            </el-select>
            <el-input v-model="nodeForm.label" placeholder="节点显示名（可空）" />
            <el-button type="primary" :icon="Plus" :loading="saving" @click="addNode">添加节点</el-button>
          </div>
          <div v-if="!availableDevices.length" class="mt-2 text-xs text-slate-500">所有资产都已加入拓扑，新增节点前请先添加新资产或移除已有拓扑节点。</div>
        </div>

        <div class="rounded-xl border border-slate-200 bg-slate-50 p-4">
          <div class="mb-3 font-semibold text-slate-800">移除资产节点</div>
          <el-table :data="manageableNodes" size="small" max-height="220" empty-text="暂无可移除节点">
            <el-table-column label="节点">
              <template #default="{ row }">
                <div class="font-medium text-slate-800">{{ row.displayLabel }}</div>
                <div class="text-xs text-slate-500">{{ row.displayIp }}</div>
              </template>
            </el-table-column>
            <el-table-column label="操作" width="96" align="right">
              <template #default="{ row }">
                <el-button :icon="Delete" type="danger" link :disabled="saving" @click="removeNode(row)">删除</el-button>
              </template>
            </el-table-column>
          </el-table>
        </div>

        <div class="rounded-xl border border-slate-200 bg-slate-50 p-4">
          <div class="mb-3 font-semibold text-slate-800">添加节点连线</div>
          <div class="grid grid-cols-1 gap-3 lg:grid-cols-2">
            <el-select v-model="edgeForm.source_node_id" filterable placeholder="源节点" class="w-full">
              <el-option v-for="n in nodeOptions" :key="n.value" :label="n.label" :value="n.value" />
            </el-select>
            <el-select v-model="edgeForm.target_node_id" filterable placeholder="目标节点" class="w-full">
              <el-option v-for="n in nodeOptions" :key="n.value" :label="n.label" :value="n.value" />
            </el-select>
            <el-input v-model="edgeForm.label" placeholder="链路标签（可空）" />
            <el-input v-model="edgeForm.remark" placeholder="链路备注（可空）" />
          </div>
          <div class="mt-3 flex justify-end">
            <el-button type="primary" :loading="saving" @click="addEdge">保存连线</el-button>
          </div>
        </div>
      </div>
    </el-card>
  </div>
</template>
