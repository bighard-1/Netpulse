<script setup>
import { zhCN } from "../../i18n/zhCN";

const emit = defineEmits(["open-event"]);

defineProps({
  loading: { type: Boolean, default: false },
  alerts: { type: Array, default: () => [] },
  severityTag: { type: Function, required: true }
});
</script>

<template>
  <el-card>
    <template #header><span class="text-lg font-semibold">{{ zhCN.deviceList.liveFeed }}</span></template>
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
            <div class="mt-1 text-sm text-slate-700">{{ a.device_name || a.device_ip || "-" }} · {{ a.message || (a.action + " " + (a.target || "")) }}</div>
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
}
@keyframes npFeedIn {
  from { opacity: 0; transform: translateY(8px); }
  to { opacity: 1; transform: translateY(0); }
}
</style>
