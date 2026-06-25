<script setup>
import { computed, nextTick, onBeforeUnmount, ref, watch } from "vue";
import { statusLabel } from "../../utils/status";

const props = defineProps({
  nodes: { type: Array, default: () => [] },
  edges: { type: Array, default: () => [] },
  editable: { type: Boolean, default: false },
  clickable: { type: Boolean, default: true },
  loading: { type: Boolean, default: false },
  autoFit: { type: Boolean, default: true },
  height: { type: Number, default: 360 },
  tooltipFontSize: { type: Number, default: 16 },
  labelDisplayMode: { type: String, default: "hover" },
  labelDisplayTiers: { type: Array, default: () => ["core"] },
  autoResetMs: { type: Number, default: 600000 },
  fitMaxWidth: { type: Number, default: 0 },
  fitMaxHeight: { type: Number, default: 0 },
  wheelZoom: { type: Boolean, default: true }
});

const emit = defineEmits(["node-move", "node-open", "node-delete", "edge-delete"]);

const svgRef = ref(null);
const viewBox = ref({ x: 0, y: 0, w: 1000, h: 620 });
const dragging = ref(null);
const panning = ref(null);
const nodePositions = ref({});
let resetTimer = null;

function normalizeTier(raw) {
  const text = String(raw || "").trim().toLowerCase();
  if (text.includes("core") || text.includes("核心")) return "core";
  if (text.includes("aggregation") || text.includes("aggregate") || text.includes("agg") || text.includes("汇聚")) return "aggregation";
  return "access";
}

const normalizedNodes = computed(() => {
  return (props.nodes || []).map((n, idx) => ({
    ...n,
    x: Number.isFinite(Number(nodePositions.value[n.id]?.x)) ? Number(nodePositions.value[n.id].x) : (Number.isFinite(Number(n.x)) ? Number(n.x) : 120 + (idx % 6) * 150),
    y: Number.isFinite(Number(nodePositions.value[n.id]?.y)) ? Number(nodePositions.value[n.id].y) : (Number.isFinite(Number(n.y)) ? Number(n.y) : 120 + Math.floor(idx / 6) * 130),
    label: n.label || n.device_name || n.device_ip || `节点-${n.id}`,
    status: String(n.device_status || n.status || "unknown").toLowerCase(),
    tier: normalizeTier(n.device_tier || n.tier)
  }));
});

const nodeStats = computed(() => {
  const total = normalizedNodes.value.length;
  const online = normalizedNodes.value.filter((n) => n.status === "online").length;
  const offline = normalizedNodes.value.filter((n) => n.status === "offline").length;
  return { total, online, offline, unknown: Math.max(0, total - online - offline) };
});

const nodeMap = computed(() => {
  const m = new Map();
  normalizedNodes.value.forEach((n) => m.set(Number(n.id), n));
  return m;
});

const visibleEdges = computed(() => {
  return (props.edges || [])
    .map((e) => ({ ...e, source: nodeMap.value.get(Number(e.source_node_id)), target: nodeMap.value.get(Number(e.target_node_id)) }))
    .filter((e) => e.source && e.target);
});

function statusText(n) {
  return statusLabel(n.status);
}

function nodeClass(n) {
  const status = n.status === "online" ? "is-online" : n.status === "offline" ? "is-offline" : "is-unknown";
  return [status, `is-${n.tier || "access"}`];
}

function shouldShowPersistentLabel(node) {
  if (props.labelDisplayMode === "always") return true;
  if (props.labelDisplayMode === "custom") return (props.labelDisplayTiers || []).includes(node.tier);
  return false;
}

function persistentLabelY(node) {
  if (node.tier === "access") return 38;
  if (node.tier === "aggregation") return 58;
  return 66;
}

function offlineMarkTransform(node) {
  if (node.tier === "access") return "translate(16 -16)";
  if (node.tier === "aggregation") return "translate(42 -34)";
  return "translate(42 -42)";
}

function offlineMarkSize(node) {
  return node.tier === "access" ? 13 : 15;
}

function fit() {
  if (!normalizedNodes.value.length) {
    viewBox.value = { x: 0, y: 0, w: 1000, h: 620 };
    return;
  }
  const pad = 140;
  const xs = normalizedNodes.value.map((n) => n.x);
  const ys = normalizedNodes.value.map((n) => n.y);
  const minX = Math.min(...xs) - pad;
  const maxX = Math.max(...xs) + pad;
  const minY = Math.min(...ys) - pad;
  const maxY = Math.max(...ys) + pad;
  const rawW = Math.max(520, maxX - minX);
  const rawH = Math.max(320, maxY - minY);
  const nextW = props.fitMaxWidth > 0 ? Math.min(rawW, props.fitMaxWidth) : rawW;
  const nextH = props.fitMaxHeight > 0 ? Math.min(rawH, props.fitMaxHeight) : rawH;
  const centerX = (minX + maxX) / 2;
  const centerY = (minY + maxY) / 2;
  viewBox.value = {
    x: centerX - nextW / 2,
    y: centerY - nextH / 2,
    w: nextW,
    h: nextH
  };
}

function scheduleAutoReset() {
  if (resetTimer) window.clearTimeout(resetTimer);
  if (!props.autoFit || !props.autoResetMs) return;
  resetTimer = window.setTimeout(() => {
    fit();
    scheduleAutoReset();
  }, props.autoResetMs);
}

function zoomAt(factor, center) {
  const vb = viewBox.value;
  const nextW = Math.max(220, Math.min(3600, vb.w * factor));
  const nextH = Math.max(160, Math.min(2400, vb.h * factor));
  const cx = center?.x ?? vb.x + vb.w / 2;
  const cy = center?.y ?? vb.y + vb.h / 2;
  const rx = (cx - vb.x) / vb.w;
  const ry = (cy - vb.y) / vb.h;
  viewBox.value = {
    x: cx - nextW * rx,
    y: cy - nextH * ry,
    w: nextW,
    h: nextH
  };
  scheduleAutoReset();
}

function zoom(factor) {
  zoomAt(factor);
}

function zoomIn() {
  zoom(0.82);
}

function zoomOut() {
  zoom(1.22);
}

function clientToSvg(e) {
  const svg = svgRef.value;
  if (!svg) return { x: 0, y: 0 };
  const pt = svg.createSVGPoint();
  pt.x = e.clientX;
  pt.y = e.clientY;
  const ctm = svg.getScreenCTM();
  if (!ctm) return { x: 0, y: 0 };
  return pt.matrixTransform(ctm.inverse());
}

function onWheel(e) {
  if (!props.wheelZoom) return;
  if (!normalizedNodes.value.length) return;
  e.preventDefault();
  const factor = e.deltaY < 0 ? 0.88 : 1.14;
  zoomAt(factor, clientToSvg(e));
}

function onCanvasPointerDown(e) {
  const target = e.target;
  if (target?.closest?.(".np-topology-node, .np-edge-delete, .np-node-delete")) return;
  e.preventDefault();
  const p = clientToSvg(e);
  panning.value = { x: p.x, y: p.y, viewBox: { ...viewBox.value } };
  window.addEventListener("pointermove", onPanMove);
  window.addEventListener("pointerup", onPanEnd, { once: true });
}

function onPanMove(e) {
  if (!panning.value) return;
  const p = clientToSvg(e);
  const dx = p.x - panning.value.x;
  const dy = p.y - panning.value.y;
  const vb = panning.value.viewBox;
  viewBox.value = { ...vb, x: vb.x - dx, y: vb.y - dy };
}

function onPanEnd() {
  window.removeEventListener("pointermove", onPanMove);
  panning.value = null;
  scheduleAutoReset();
}

function onNodePointerDown(e, node) {
  if (!props.editable) return;
  e.preventDefault();
  e.stopPropagation();
  const p = clientToSvg(e);
  dragging.value = { id: node.id, offsetX: p.x - node.x, offsetY: p.y - node.y };
  window.addEventListener("pointermove", onPointerMove);
  window.addEventListener("pointerup", onPointerUp, { once: true });
}

function onNodeClick(node) {
  if (!props.clickable) return;
  emit("node-open", node);
}

function onPointerMove(e) {
  if (!dragging.value) return;
  const p = clientToSvg(e);
  const node = nodeMap.value.get(Number(dragging.value.id));
  if (!node) return;
  const x = Math.round((p.x - dragging.value.offsetX) * 10) / 10;
  const y = Math.round((p.y - dragging.value.offsetY) * 10) / 10;
  nodePositions.value = { ...nodePositions.value, [node.id]: { x, y } };
}

function onPointerUp() {
  window.removeEventListener("pointermove", onPointerMove);
  const id = dragging.value?.id;
  const node = nodeMap.value.get(Number(id));
  dragging.value = null;
  if (node) emit("node-move", { id: node.id, x: node.x, y: node.y, label: node.label });
  scheduleAutoReset();
}

watch(
  () => [props.nodes?.length || 0, props.edges?.length || 0, props.nodes?.map((n) => `${n.id}:${n.x}:${n.y}`).join("|") || ""],
  async () => {
    const next = {};
    for (const n of props.nodes || []) {
      next[n.id] = { x: Number(n.x || 0), y: Number(n.y || 0) };
    }
    nodePositions.value = next;
    if (!props.autoFit) return;
    await nextTick();
    fit();
    scheduleAutoReset();
  },
  { immediate: true }
);

onBeforeUnmount(() => {
  if (resetTimer) window.clearTimeout(resetTimer);
  window.removeEventListener("pointermove", onPointerMove);
  window.removeEventListener("pointermove", onPanMove);
});

defineExpose({ fit, zoomIn, zoomOut });
</script>

<template>
  <div class="np-topology-canvas" :style="{ height: `${height}px` }">
    <div class="np-topology-stats" aria-label="拓扑节点统计">
      <span>全部 {{ nodeStats.total }}</span>
      <span class="is-online">在线 {{ nodeStats.online }}</span>
      <span class="is-offline">离线 {{ nodeStats.offline }}</span>
      <span v-if="nodeStats.unknown" class="is-unknown">未知 {{ nodeStats.unknown }}</span>
    </div>
    <div v-if="loading" class="np-topology-loading">拓扑加载中...</div>
    <svg
      ref="svgRef"
      class="np-topology-svg"
      :viewBox="`${viewBox.x} ${viewBox.y} ${viewBox.w} ${viewBox.h}`"
      role="img"
      aria-label="NetPulse 手动拓扑图"
      @wheel="onWheel"
      @pointerdown="onCanvasPointerDown"
    >
      <defs>
        <filter id="npNodeShadow" x="-40%" y="-40%" width="180%" height="180%">
          <feDropShadow dx="0" dy="10" stdDeviation="10" flood-color="#0f172a" flood-opacity="0.16" />
        </filter>
        <radialGradient id="npOrbOnline" cx="32%" cy="28%" r="72%">
          <stop offset="0%" stop-color="#d1fae5" />
          <stop offset="36%" stop-color="#34d399" />
          <stop offset="100%" stop-color="#059669" />
        </radialGradient>
        <radialGradient id="npOrbOffline" cx="32%" cy="28%" r="72%">
          <stop offset="0%" stop-color="#fee2e2" />
          <stop offset="36%" stop-color="#fb7185" />
          <stop offset="100%" stop-color="#dc2626" />
        </radialGradient>
        <radialGradient id="npOrbUnknown" cx="32%" cy="28%" r="72%">
          <stop offset="0%" stop-color="#fef3c7" />
          <stop offset="36%" stop-color="#fbbf24" />
          <stop offset="100%" stop-color="#d97706" />
        </radialGradient>
      </defs>
      <rect class="np-topology-bg" :x="viewBox.x" :y="viewBox.y" :width="viewBox.w" :height="viewBox.h" rx="22" />
      <g class="np-topology-edges">
        <g v-for="edge in visibleEdges" :key="edge.id">
          <line :x1="edge.source.x" :y1="edge.source.y" :x2="edge.target.x" :y2="edge.target.y" class="np-topology-edge" />
          <text v-if="edge.label" :x="(edge.source.x + edge.target.x) / 2" :y="(edge.source.y + edge.target.y) / 2 - 8" class="np-edge-label">
            {{ edge.label }}
          </text>
          <g v-if="editable" class="np-edge-delete" @click.stop="emit('edge-delete', edge)">
            <circle :cx="(edge.source.x + edge.target.x) / 2" :cy="(edge.source.y + edge.target.y) / 2 + 16" r="11" />
            <text :x="(edge.source.x + edge.target.x) / 2" :y="(edge.source.y + edge.target.y) / 2 + 20">×</text>
          </g>
        </g>
      </g>
      <g class="np-topology-nodes">
        <g
          v-for="node in normalizedNodes"
          :key="node.id"
          class="np-topology-node"
          :class="[nodeClass(node), { 'is-clickable': clickable }]"
          :transform="`translate(${node.x}, ${node.y})`"
          @pointerdown="onNodePointerDown($event, node)"
          @click.stop="onNodeClick(node)"
        >
          <g v-if="node.tier === 'access'" class="np-node-icon np-access-icon" filter="url(#npNodeShadow)">
            <circle class="np-node-shape np-access-orb" r="19" />
            <circle class="np-orb-gloss" cx="-6" cy="-7" r="7" />
            <circle class="np-orb-shine" cx="-10" cy="-11" r="2.6" />
          </g>
          <g v-else-if="node.tier === 'aggregation'" class="np-node-icon np-aggregation-icon" filter="url(#npNodeShadow)">
            <rect class="np-node-shape np-stack-back" x="-40" y="-34" width="80" height="26" rx="9" />
            <rect class="np-node-shape np-stack-mid" x="-45" y="-10" width="90" height="28" rx="10" />
            <rect class="np-node-shape np-stack-front" x="-40" y="15" width="80" height="24" rx="9" />
            <path class="np-node-glyph" d="M-20 -21 H20 M-24 4 H24 M-18 27 H18" />
            <path class="np-node-glyph np-link-glyph" d="M-28 -8 L-36 15 M28 -8 L36 15" />
          </g>
          <g v-else class="np-node-icon np-core-icon" filter="url(#npNodeShadow)">
            <path class="np-node-shape" d="M0,-48 L42,-24 L42,24 L0,48 L-42,24 L-42,-24 Z" />
            <circle class="np-core-hub" cx="0" cy="0" r="10" />
            <path class="np-node-glyph np-core-link" d="M0 -32 V-10 M0 10 V32 M-28 -16 L-9 -5 M9 5 L28 16 M28 -16 L9 -5 M-9 5 L-28 16" />
          </g>
          <g
            v-if="node.status === 'offline'"
            class="np-node-offline-mark"
            :transform="offlineMarkTransform(node)"
          >
            <circle :r="offlineMarkSize(node)" />
            <path d="M-5 -5 L5 5 M5 -5 L-5 5" />
          </g>
          <g
            v-if="shouldShowPersistentLabel(node)"
            class="np-node-persistent-label"
            :style="{ '--np-label-font-size': `${Math.max(12, tooltipFontSize - 1)}px` }"
          >
            <text :y="persistentLabelY(node)">{{ node.label }}</text>
          </g>
          <g class="np-node-tooltip" :style="{ '--np-tooltip-font-size': `${tooltipFontSize}px` }">
            <rect x="-112" y="-88" width="224" height="48" rx="12" />
            <text y="-69" class="np-node-title">{{ node.label }}</text>
            <text y="-52" class="np-node-subtitle">{{ statusText(node) }} · {{ node.tier === 'core' ? '核心层' : node.tier === 'aggregation' ? '汇聚层' : '接入层' }}</text>
          </g>
          <g v-if="editable" class="np-node-delete" @click.stop="emit('node-delete', node)">
            <circle cx="38" cy="-38" r="11" />
            <text x="38" y="-34">×</text>
          </g>
        </g>
      </g>
    </svg>
    <div v-if="!loading && !normalizedNodes.length" class="np-topology-empty">暂无拓扑节点，请管理员在拓扑管理中添加资产节点</div>
  </div>
</template>

<style scoped>
.np-topology-canvas {
  position: relative;
  overflow: hidden;
  border: 1px solid rgba(148, 163, 184, 0.22);
  border-radius: 18px;
  background:
    radial-gradient(circle at 20% 20%, rgba(99, 102, 241, 0.12), transparent 30%),
    linear-gradient(135deg, #f8fafc, #eef6ff);
}

.np-topology-svg {
  width: 100%;
  height: 100%;
  display: block;
  touch-action: none;
  cursor: grab;
}

.np-topology-svg:active {
  cursor: grabbing;
}

.np-topology-bg {
  fill: rgba(255, 255, 255, 0.18);
}

.np-topology-stats {
  position: absolute;
  top: 12px;
  right: 12px;
  z-index: 3;
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  max-width: min(70%, 520px);
  justify-content: flex-end;
  pointer-events: none;
}

.np-topology-stats span {
  border: 1px solid rgba(148, 163, 184, 0.26);
  border-radius: 999px;
  background: rgba(255, 255, 255, 0.88);
  padding: 4px 9px;
  color: #475569;
  font-size: 12px;
  font-weight: 700;
  box-shadow: 0 8px 18px rgba(15, 23, 42, 0.08);
}

.np-topology-stats .is-online { color: #059669; }
.np-topology-stats .is-offline { color: #dc2626; }
.np-topology-stats .is-unknown { color: #d97706; }

.np-topology-edge {
  stroke: rgba(71, 85, 105, 0.45);
  stroke-width: 3;
  stroke-linecap: round;
}

.np-edge-label {
  font-size: 13px;
  fill: #475569;
  text-anchor: middle;
  paint-order: stroke;
  stroke: white;
  stroke-width: 4px;
}

.np-edge-delete circle {
  fill: #fee2e2;
  stroke: #fecaca;
  cursor: pointer;
}

.np-edge-delete text {
  fill: #dc2626;
  font-size: 15px;
  font-weight: 700;
  text-anchor: middle;
  pointer-events: none;
}

.np-topology-node {
  cursor: default;
}

.np-node-icon .np-node-shape {
  stroke: rgba(255, 255, 255, 0.96);
  stroke-width: 4.5;
}

.np-topology-node.is-online .np-node-shape { fill: #10b981; }
.np-topology-node.is-offline .np-node-shape { fill: #ef4444; }
.np-topology-node.is-unknown .np-node-shape { fill: #f59e0b; }

.np-aggregation-icon .np-stack-back,
.np-aggregation-icon .np-stack-front {
  opacity: 0.84;
}

.np-access-icon .np-node-shape {
  stroke-width: 3.2;
}

.np-topology-node.is-online .np-access-orb { fill: url(#npOrbOnline); }
.np-topology-node.is-offline .np-access-orb { fill: url(#npOrbOffline); }
.np-topology-node.is-unknown .np-access-orb { fill: url(#npOrbUnknown); }

.np-orb-gloss {
  fill: rgba(255, 255, 255, 0.22);
  pointer-events: none;
}

.np-orb-shine {
  fill: rgba(255, 255, 255, 0.78);
  pointer-events: none;
}

.np-node-glyph {
  fill: none;
  stroke: rgba(255, 255, 255, 0.86);
  stroke-width: 4;
  stroke-linecap: round;
  stroke-linejoin: round;
  pointer-events: none;
}

.np-port-dots circle,
.np-core-hub {
  fill: rgba(255, 255, 255, 0.9);
  pointer-events: none;
}

.np-link-glyph,
.np-core-link {
  stroke-width: 3.4;
  opacity: 0.9;
}

.np-node-offline-mark {
  pointer-events: none;
  filter: drop-shadow(0 6px 10px rgba(127, 29, 29, 0.35));
}

.np-node-offline-mark circle {
  fill: #7f1d1d;
  stroke: rgba(255, 255, 255, 0.96);
  stroke-width: 2.6;
}

.np-node-offline-mark path {
  fill: none;
  stroke: #ffffff;
  stroke-linecap: round;
  stroke-width: 3;
}

.np-topology-node.is-clickable {
  cursor: pointer;
}

.np-node-tooltip {
  opacity: 0;
  transform: translateY(4px);
  pointer-events: none;
  transition: opacity 0.14s ease, transform 0.14s ease;
}

.np-node-persistent-label text {
  fill: #0f172a;
  font-size: var(--np-label-font-size, 15px);
  font-weight: 800;
  text-anchor: middle;
  dominant-baseline: middle;
  paint-order: stroke;
  stroke: rgba(255, 255, 255, 0.9);
  stroke-width: 5px;
  stroke-linejoin: round;
  pointer-events: none;
}

.np-topology-node:hover .np-node-tooltip {
  opacity: 1;
  transform: translateY(0);
}

.np-node-tooltip rect {
  fill: rgba(15, 23, 42, 0.94);
  stroke: rgba(255, 255, 255, 0.18);
  stroke-width: 1;
}

.np-node-title {
  font-size: var(--np-tooltip-font-size, 16px);
  font-weight: 800;
  fill: #ffffff;
  text-anchor: middle;
  dominant-baseline: middle;
}

.np-node-subtitle {
  font-size: calc(var(--np-tooltip-font-size, 16px) * 0.72);
  fill: #cbd5e1;
  text-anchor: middle;
  dominant-baseline: middle;
}

.np-node-delete circle {
  fill: #fee2e2;
  stroke: #fecaca;
}

.np-node-delete text {
  fill: #dc2626;
  font-size: 15px;
  font-weight: 800;
  text-anchor: middle;
  pointer-events: none;
}

.np-topology-loading,
.np-topology-empty {
  position: absolute;
  inset: 0;
  z-index: 2;
  display: grid;
  place-items: center;
  color: #64748b;
  font-size: 13px;
  background: rgba(248, 250, 252, 0.72);
  backdrop-filter: blur(3px);
}

</style>
