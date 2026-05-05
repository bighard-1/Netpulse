<script setup>
import { onMounted, ref } from "vue";
import { ElMessage } from "element-plus";
import { api, getApiError } from "../services/api";

const loading = ref(false);
const topology = ref({ nodes: [], edges: [] });

async function loadTopology() {
  loading.value = true;
  try {
    const res = await api.listTopology();
    topology.value = res.data || { nodes: [], edges: [] };
  } catch (err) {
    ElMessage.error(getApiError(err, "加载拓扑失败"));
  } finally {
    loading.value = false;
  }
}

onMounted(loadTopology);
</script>

<template>
  <el-card>
    <template #header>
      <div class="flex items-center justify-between">
        <span class="text-lg font-semibold">Topology</span>
        <el-button @click="loadTopology">刷新</el-button>
      </div>
    </template>
    <el-skeleton :loading="loading" animated :rows="10">
      <template #default>
        <el-empty v-if="!topology.nodes?.length" description="暂无拓扑数据" />
        <div v-else class="grid grid-cols-1 gap-3 md:grid-cols-2">
          <el-card>
            <div class="text-sm text-slate-500">节点数量</div>
            <div class="text-2xl font-semibold">{{ topology.nodes.length }}</div>
          </el-card>
          <el-card>
            <div class="text-sm text-slate-500">连线数量</div>
            <div class="text-2xl font-semibold">{{ topology.edges.length }}</div>
          </el-card>
        </div>
      </template>
    </el-skeleton>
  </el-card>
</template>
