<script setup>
const phases = [
  {
    id: 1,
    title: "第一阶段：生存基础",
    goal: "确认设备活着",
    features: "ICMP、基础信息、Uptime",
    status: "done"
  },
  {
    id: 2,
    title: "第二阶段：流量核心",
    goal: "端口数据可视化",
    features: "端口状态、实时带宽、历史流量",
    status: "done"
  },
  {
    id: 3,
    title: "第三阶段：资源监控",
    goal: "性能指标预测",
    features: "CPU/内存、存储空间",
    status: "done"
  },
  {
    id: 4,
    title: "第四阶段：异常管理",
    goal: "告警系统",
    features: "阈值、通知、日志中心",
    status: "done"
  },
  {
    id: 5,
    title: "第五阶段：自动化",
    goal: "模板化效率提升",
    features: "模板绑定、OID策略库",
    status: "doing"
  }
];

function statusLabel(s) {
  if (s === "done") return "已完成";
  if (s === "doing") return "推进中";
  return "待规划";
}

function statusType(s) {
  if (s === "done") return "success";
  if (s === "doing") return "warning";
  return "info";
}
</script>

<template>
  <el-card>
    <template #header>
      <span class="text-lg font-semibold">分阶段推进路线图</span>
    </template>
    <div class="grid grid-cols-1 gap-3 xl:grid-cols-5">
      <div v-for="p in phases" :key="p.id" class="rounded-xl border border-slate-700/30 bg-slate-800/30 p-3">
        <div class="mb-2 flex items-center justify-between">
          <span class="text-sm font-semibold text-slate-100">P{{ p.id }}</span>
          <el-tag size="small" :type="statusType(p.status)">{{ statusLabel(p.status) }}</el-tag>
        </div>
        <div class="text-sm font-medium text-slate-100">{{ p.title }}</div>
        <div class="mt-1 text-xs text-slate-400">{{ p.goal }}</div>
        <div class="mt-2 text-xs text-slate-300">{{ p.features }}</div>
      </div>
    </div>
  </el-card>
</template>
