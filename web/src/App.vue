<script setup>
import { computed, onMounted, ref } from "vue";
import { ElMessage } from "element-plus";
import { useRoute, useRouter } from "vue-router";
import { api } from "./services/api";

const route = useRoute();
const router = useRouter();

const token = ref(localStorage.getItem("netpulse_token") || "");
const currentUser = ref(JSON.parse(localStorage.getItem("netpulse_user") || "null"));

const loginVisible = ref(!token.value);
const loginForm = ref({ username: "", password: "" });

const usersVisible = ref(false);
const users = ref([]);
const addUserForm = ref({ username: "", password: "", role: "user" });

const auditVisible = ref(false);
const auditLogs = ref([]);

const restoreLoading = ref(false);
const drillLoading = ref(false);

const isAdmin = computed(() => currentUser.value?.role === "admin");

const breadcrumbs = computed(() => {
  if (route.name === "assets") return [{ label: "资产列表", to: "/" }];

  if (route.name === "device-detail") {
    const name = route.query.ip || `设备-${route.params.id}`;
    return [
      { label: "资产列表", to: "/" },
      { label: String(name), to: route.fullPath }
    ];
  }

  if (route.name === "port-detail") {
    const device = route.query.deviceIp || route.query.deviceName || "设备";
    const port = route.query.portName || `端口-${route.params.id}`;
    const deviceId = route.query.deviceId;
    return [
      { label: "资产列表", to: "/" },
      { label: String(device), to: deviceId ? `/device/${deviceId}` : "/" },
      { label: String(port), to: route.fullPath }
    ];
  }

  return [{ label: "资产列表", to: "/" }];
});

async function doLogin() {
  const res = await api.login(loginForm.value.username, loginForm.value.password);
  localStorage.setItem("netpulse_token", res.data.token);
  localStorage.setItem("netpulse_user", JSON.stringify(res.data.user));
  token.value = res.data.token;
  currentUser.value = res.data.user;
  loginVisible.value = false;
  ElMessage.success("登录成功");
  if (route.path === "/") return;
  router.push("/");
}

function logout() {
  localStorage.removeItem("netpulse_token");
  localStorage.removeItem("netpulse_user");
  token.value = "";
  currentUser.value = null;
  loginVisible.value = true;
}

async function onBackup() {
  const res = await api.downloadBackup();
  const blobUrl = URL.createObjectURL(new Blob([res.data]));
  const a = document.createElement("a");
  a.href = blobUrl;
  a.download = "netpulse_backup.sql.gz";
  a.click();
  URL.revokeObjectURL(blobUrl);
}

async function onRestore(file) {
  restoreLoading.value = true;
  try {
    await api.restoreFromFile(file.raw);
    ElMessage.success("恢复完成");
  } finally {
    restoreLoading.value = false;
  }
}

async function runBackupDrill() {
  drillLoading.value = true;
  try {
    await api.backupDrill();
    ElMessage.success("备份校验演练完成");
  } finally {
    drillLoading.value = false;
  }
}

async function openUsers() {
  usersVisible.value = true;
  const res = await api.listUsers();
  users.value = res.data || [];
}

async function createUser() {
  await api.createUser(addUserForm.value);
  ElMessage.success("用户已创建");
  addUserForm.value = { username: "", password: "", role: "user" };
  const res = await api.listUsers();
  users.value = res.data || [];
}

async function openAudit() {
  auditVisible.value = true;
  const res = await api.listAuditLogs();
  auditLogs.value = res.data || [];
}

onMounted(() => {
  if (!token.value) loginVisible.value = true;
});
</script>

<template>
  <div class="min-h-screen bg-[radial-gradient(1200px_500px_at_20%_-10%,#dbeafe,transparent),radial-gradient(1200px_500px_at_90%_0%,#cffafe,transparent),#f1f5f9]">
    <header class="sticky top-0 z-20 border-b bg-white/95 backdrop-blur">
      <div class="mx-auto flex max-w-[1600px] flex-wrap items-center justify-between gap-3 px-5 py-3">
        <div class="flex items-center gap-6">
          <h1 class="text-2xl font-bold text-slate-900">NetPulse</h1>
          <nav class="flex items-center gap-2">
            <el-button text @click="$router.push('/')">资产</el-button>
            <el-button text @click="openAudit">审计日志</el-button>
            <el-button text @click="onBackup">下载备份</el-button>
            <el-button text :loading="drillLoading" @click="runBackupDrill">备份演练</el-button>
            <el-upload :auto-upload="false" :show-file-list="false" accept=".gz" :on-change="onRestore" :disabled="restoreLoading">
              <el-button text>恢复数据</el-button>
            </el-upload>
          </nav>
        </div>

        <div class="flex items-center gap-2">
          <el-button v-if="isAdmin" @click="openUsers">用户管理</el-button>
          <el-button type="danger" plain @click="logout">退出</el-button>
        </div>
      </div>
    </header>

    <main class="mx-auto max-w-[1600px] p-5">
      <el-card class="mb-4">
        <el-breadcrumb separator=">">
          <el-breadcrumb-item v-for="item in breadcrumbs" :key="item.label">
            <router-link :to="item.to">{{ item.label }}</router-link>
          </el-breadcrumb-item>
        </el-breadcrumb>
      </el-card>

      <router-view />
    </main>

    <el-dialog v-model="loginVisible" title="登录 NetPulse" width="420" :close-on-click-modal="false" :show-close="false">
      <el-form label-position="top">
        <el-form-item label="用户名"><el-input v-model="loginForm.username" /></el-form-item>
        <el-form-item label="密码"><el-input v-model="loginForm.password" show-password @keyup.enter="doLogin" /></el-form-item>
      </el-form>
      <template #footer><el-button type="primary" @click="doLogin">登录</el-button></template>
    </el-dialog>

    <el-dialog v-model="usersVisible" title="用户管理" width="760">
      <div class="mb-3 grid grid-cols-3 gap-2">
        <el-input v-model="addUserForm.username" placeholder="用户名" />
        <el-input v-model="addUserForm.password" placeholder="密码" />
        <div class="flex gap-2">
          <el-select v-model="addUserForm.role"><el-option value="user" label="普通用户" /><el-option value="admin" label="管理员" /></el-select>
          <el-button type="primary" @click="createUser">创建</el-button>
        </div>
      </div>
      <el-table :data="users">
        <el-table-column prop="id" label="ID" width="80" />
        <el-table-column prop="username" label="用户名" />
        <el-table-column prop="role" label="角色" width="120" />
      </el-table>
    </el-dialog>

    <el-dialog v-model="auditVisible" title="审计日志" width="980">
      <el-table :data="auditLogs" height="520">
        <el-table-column prop="timestamp" label="时间" width="190" />
        <el-table-column prop="username" label="用户名" width="120" />
        <el-table-column prop="client" label="客户端" width="100" />
        <el-table-column prop="action" label="动作" width="180" />
        <el-table-column prop="method" label="方法" width="90" />
        <el-table-column prop="status_code" label="状态码" width="90" />
        <el-table-column prop="duration_ms" label="耗时(ms)" width="110" />
        <el-table-column prop="path" label="路径" min-width="220" />
        <el-table-column prop="ip" label="IP" width="160" />
        <el-table-column prop="target" label="详情" min-width="220" />
      </el-table>
    </el-dialog>
  </div>
</template>
