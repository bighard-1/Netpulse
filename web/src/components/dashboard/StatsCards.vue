<script setup>
import { zhCN } from "../../i18n/zhCN";
const emit = defineEmits(["open-health", "open-availability", "open-alerts"]);

const props = defineProps({
  healthScore: { type: Number, required: true },
  availability: { type: Number, required: true },
  onlineCount: { type: Number, required: true },
  totalCount: { type: Number, required: true },
  activeAlerts: { type: Number, required: true },
  alertBreakdown: { type: Object, required: true },
  vertical: { type: Boolean, default: false }
});
</script>

<template>
  <section class="grid grid-cols-1" :class="props.vertical ? 'gap-3' : 'gap-4 lg:grid-cols-3'">
    <el-card class="np-kpi-card cursor-pointer" @click="emit('open-health')">
      <div class="np-kpi-title text-sm text-slate-500">{{ zhCN.deviceList.healthScore }}</div>
      <div class="mt-2 flex items-center gap-3">
        <el-progress type="dashboard" :percentage="healthScore" :stroke-width="8" :width="props.vertical ? 82 : 120" />
        <div>
          <div class="np-kpi-value font-semibold text-slate-900" :class="props.vertical ? 'text-2xl' : 'text-3xl'">{{ healthScore }}</div>
          <div class="mt-1 text-xs text-slate-500">综合评分 / 100</div>
        </div>
      </div>
    </el-card>

    <el-card class="np-kpi-card cursor-pointer" @click="emit('open-availability')">
      <div class="np-kpi-title text-sm text-slate-500">{{ zhCN.deviceList.availability }}</div>
      <div class="mt-3 flex items-center justify-between">
        <div class="np-kpi-value font-semibold text-slate-900" :class="props.vertical ? 'text-2xl' : 'text-3xl'">{{ availability }}%</div>
        <span class="np-chip">SLA 可视化</span>
      </div>
      <div class="mt-2 text-xs text-slate-500">在线 {{ onlineCount }} / 总数 {{ totalCount }}</div>
    </el-card>

    <el-card class="np-kpi-card cursor-pointer" @click="emit('open-alerts')">
      <div class="np-kpi-title text-sm text-slate-500">{{ zhCN.deviceList.activeAlerts }}</div>
      <div class="mt-3 flex items-center justify-between">
        <div class="np-kpi-value font-semibold text-slate-900" :class="props.vertical ? 'text-2xl' : 'text-3xl'">{{ activeAlerts }}</div>
        <span class="np-chip">实时</span>
      </div>
      <div class="mt-3 flex flex-wrap gap-2 text-xs">
        <el-tag type="danger" class="cursor-pointer" @click.stop="emit('open-alerts', 'critical')">严重 {{ alertBreakdown.critical }}</el-tag>
        <el-tag type="warning" class="cursor-pointer" @click.stop="emit('open-alerts', 'warning')">警告 {{ alertBreakdown.warning }}</el-tag>
        <el-tag type="success" class="cursor-pointer" @click.stop="emit('open-alerts', 'info')">信息 {{ alertBreakdown.info }}</el-tag>
      </div>
    </el-card>

  </section>
</template>

<style scoped>
.np-kpi-card {
  position: relative;
  overflow: hidden;
  border: 1px solid rgba(148, 163, 184, 0.22) !important;
  background:
    radial-gradient(circle at 92% 12%, rgba(37, 99, 235, 0.12), transparent 30%),
    linear-gradient(135deg, rgba(255, 255, 255, 0.98) 0%, rgba(248, 250, 252, 0.9) 100%);
  min-width: 0;
  transition: transform 0.18s ease, box-shadow 0.18s ease, border-color 0.18s ease;
}

.np-kpi-card::after {
  content: "";
  position: absolute;
  inset: 0;
  pointer-events: none;
  background: linear-gradient(115deg, rgba(255, 255, 255, 0.28), transparent 36%, rgba(255, 255, 255, 0.16) 72%, transparent);
}

.np-kpi-card:hover {
  transform: translateY(-2px);
  border-color: rgba(37, 99, 235, 0.28) !important;
}

:deep(.np-kpi-card .el-card__body) {
  position: relative;
  z-index: 1;
  min-width: 0;
  padding: 18px !important;
}
.np-kpi-card:nth-child(1) {
  background:
    radial-gradient(circle at 88% 12%, rgba(20, 184, 166, 0.2), transparent 33%),
    linear-gradient(135deg, #ecfdf5 0%, #ffffff 64%);
}
.np-kpi-card:nth-child(2) {
  background:
    radial-gradient(circle at 88% 12%, rgba(99, 102, 241, 0.2), transparent 33%),
    linear-gradient(135deg, #eef2ff 0%, #ffffff 64%);
}
.np-kpi-card:nth-child(3) {
  background:
    radial-gradient(circle at 88% 12%, rgba(249, 115, 22, 0.18), transparent 33%),
    linear-gradient(135deg, #fff7ed 0%, #ffffff 64%);
}
</style>
