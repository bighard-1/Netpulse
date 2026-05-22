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
  background: linear-gradient(135deg, #ffffff 0%, #f8fafc 100%);
  min-width: 0;
}

:deep(.np-kpi-card .el-card__body) {
  min-width: 0;
  padding: 16px !important;
}
.np-kpi-card:nth-child(1) {
  background: linear-gradient(135deg, #eefbf5 0%, #ffffff 62%);
}
.np-kpi-card:nth-child(2) {
  background: linear-gradient(135deg, #eef2ff 0%, #ffffff 62%);
}
.np-kpi-card:nth-child(3) {
  background: linear-gradient(135deg, #fff7ed 0%, #ffffff 62%);
}
</style>
