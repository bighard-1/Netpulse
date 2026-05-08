<script setup>
import { zhCN } from "../../i18n/zhCN";
import { formatBps } from "../../utils/format";

defineProps({
  healthScore: { type: Number, required: true },
  availability: { type: Number, required: true },
  onlineCount: { type: Number, required: true },
  totalCount: { type: Number, required: true },
  activeAlerts: { type: Number, required: true },
  alertBreakdown: { type: Object, required: true },
  trafficHotspots: { type: Array, required: true },
  storageRiskCount: { type: Number, default: 0 }
});
</script>

<template>
  <section class="grid grid-cols-1 gap-4 xl:grid-cols-5">
    <el-card>
      <div class="text-sm text-slate-500">{{ zhCN.deviceList.healthScore }}</div>
      <div class="mt-2 flex items-center gap-4">
        <el-progress type="dashboard" :percentage="healthScore" :stroke-width="8" :width="120" />
        <div class="text-3xl font-semibold text-slate-900">{{ healthScore }}</div>
      </div>
    </el-card>

    <el-card>
      <div class="text-sm text-slate-500">{{ zhCN.deviceList.availability }}</div>
      <div class="mt-3 text-3xl font-semibold text-slate-900">{{ availability }}%</div>
      <div class="mt-2 text-xs text-slate-500">在线 {{ onlineCount }} / 总数 {{ totalCount }}</div>
    </el-card>

    <el-card>
      <div class="text-sm text-slate-500">{{ zhCN.deviceList.activeAlerts }}</div>
      <div class="mt-3 text-3xl font-semibold text-slate-900">{{ activeAlerts }}</div>
      <div class="mt-3 flex gap-2 text-xs">
        <el-tag type="danger">严重 {{ alertBreakdown.critical }}</el-tag>
        <el-tag type="warning">警告 {{ alertBreakdown.warning }}</el-tag>
        <el-tag type="success">信息 {{ alertBreakdown.info }}</el-tag>
      </div>
    </el-card>

    <el-card>
      <div class="text-sm text-slate-500">{{ zhCN.deviceList.hotspots }}</div>
      <div class="mt-3 space-y-2 text-sm">
        <div v-for="h in trafficHotspots" :key="h.interfaceId" class="rounded-lg bg-slate-50 px-2 py-2">
          <div class="font-medium text-slate-700">{{ h.deviceName }} / {{ h.interfaceName }}</div>
          <div class="text-xs text-slate-500">{{ formatBps(h.bps) }}</div>
        </div>
        <el-empty v-if="!trafficHotspots.length" description="暂无热点端口" :image-size="48" />
      </div>
    </el-card>

    <el-card>
      <div class="text-sm text-slate-500">存储风险设备</div>
      <div class="mt-3 text-3xl font-semibold text-slate-900">{{ storageRiskCount }}</div>
      <div class="mt-2 text-xs text-slate-500">阈值: 使用率 ≥ 85%</div>
    </el-card>
  </section>
</template>
