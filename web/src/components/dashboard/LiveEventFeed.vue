<script setup>
import { zhCN } from "../../i18n/zhCN";

const emit = defineEmits(["open-event"]);

defineProps({
  loading: { type: Boolean, default: false },
  alerts: { type: Array, default: () => [] },
  severityTag: { type: Function, required: true }
});

function portLabel(item) {
  const raw = String(item?.interface_raw_name || item?.port_base_name || "").trim();
  const name = String(item?.interface_name || item?.port_name || "").trim();
  const idx = Number(item?.interface_index || 0);
  const base = raw || (idx > 0 ? `ifIndex=${idx}` : "");
  if (base && name && name !== base) return `${base} / ${name}`;
  return name || base;
}

function eventText(item) {
  const msg = String(item?.message || `${item?.action || ""} ${item?.target || ""}`.trim() || "-");
  const label = portLabel(item);
  if (!label) return msg;
  const isPortEvent = String(item?.type || item?.event_type || "").includes("port") || /\[PORT_/i.test(msg);
  if (!isPortEvent || msg.includes(label)) return msg;
  return `${label} · ${msg}`;
}
</script>

<template>
  <el-card>
    <template #header>
      <div class="flex items-center justify-between">
        <span class="text-lg font-semibold">{{ zhCN.deviceList.liveFeed }}</span>
        <span class="np-chip">Live</span>
      </div>
    </template>
    <el-skeleton :loading="loading" animated :rows="10">
      <template #default>
        <div class="space-y-2 np-live-feed">
          <div
            v-for="a in alerts"
            :key="a.id"
            class="log-item rounded-lg p-2 np-live-item np-live-item-clickable"
            :class="{ 'log-error': a.severity === 'critical', 'log-warning': a.severity === 'warning' }"
            @click="emit('open-event', a)"
          >
            <div class="flex items-center justify-between gap-2">
              <el-tag size="small" :type="severityTag(a.severity)">{{ a.severity === "critical" ? "严重" : (a.severity === "warning" ? "警告" : "正常") }}</el-tag>
              <div class="text-xs text-slate-500">{{ a.timestamp || a.created_at || '-' }}</div>
            </div>
            <div class="mt-1 flex flex-wrap items-center gap-1 text-sm text-slate-700">
              <span class="font-medium text-slate-800">{{ a.device_name || a.device_ip || "-" }}</span>
              <span>·</span>
              <span class="break-all">{{ eventText(a) }}</span>
            </div>
          </div>
          <el-empty v-if="!alerts.length" description="暂无事件" :image-size="64" />
        </div>
      </template>
    </el-skeleton>
  </el-card>
</template>

<style scoped>
.np-live-feed {
  max-height: 560px;
  overflow: auto;
}
.np-live-item {
  animation: npFeedIn 280ms ease-out;
}
.np-live-item-clickable {
  cursor: pointer;
}
.np-live-item-clickable:hover {
  transform: translateY(-1px);
  box-shadow: 0 12px 24px -18px rgba(15, 23, 42, 0.7);
}
@keyframes npFeedIn {
  from { opacity: 0; transform: translateY(8px); }
  to { opacity: 1; transform: translateY(0); }
}
</style>
