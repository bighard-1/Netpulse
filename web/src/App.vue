<script setup>
import { computed, onMounted, ref } from "vue";
import { ElMessage } from "element-plus";
import DeviceDetail from "./components/DeviceDetail.vue";
import { api } from "./services/api";

const loading = ref(false);
const devices = ref([]);
const selectedId = ref(null);
const token = ref(localStorage.getItem("netpulse_token") || "");
const currentUser = ref(JSON.parse(localStorage.getItem("netpulse_user") || "null"));

const loginVisible = ref(!token.value);
const loginForm = ref({ username: "", password: "" });

const addVisible = ref(false);
const addForm = ref({ ip: "", brand: "Huawei", community: "public", remark: "" });
const remarkVisible = ref(false);
const remarkForm = ref({ remark: "" });

const usersVisible = ref(false);
const users = ref([]);
const addUserForm = ref({ username: "", password: "", role: "user" });
const auditVisible = ref(false);
const auditLogs = ref([]);

const currentDevice = computed(() => devices.value.find((d) => d.id === selectedId.value) || null);
const isAdmin = computed(() => currentUser.value?.role === "admin");

async function doLogin() {
  const res = await api.login(loginForm.value.username, loginForm.value.password);
  localStorage.setItem("netpulse_token", res.data.token);
  localStorage.setItem("netpulse_user", JSON.stringify(res.data.user));
  token.value = res.data.token;
  currentUser.value = res.data.user;
  loginVisible.value = false;
  ElMessage.success("登录成功");
  await loadDevices();
}

function logout() {
  localStorage.removeItem("netpulse_token");
  localStorage.removeItem("netpulse_user");
  token.value = "";
  currentUser.value = null;
  devices.value = [];
  loginVisible.value = true;
}

async function loadDevices() {
  if (!token.value) return;
  loading.value = true;
  try {
    const res = await api.listDevices();
    devices.value = res.data || [];
    if (!selectedId.value && devices.value.length) selectedId.value = devices.value[0].id;
  } finally {
    loading.value = false;
  }
}

async function onAddDevice() {
  await api.addDevice(addForm.value);
  addVisible.value = false;
  addForm.value = { ip: "", brand: "Huawei", community: "public", remark: "" };
  ElMessage.success("设备已添加");
  await loadDevices();
}

async function onDeleteDevice(id) {
  await api.deleteDevice(id);
  ElMessage.success("设备已删除");
  await loadDevices();
}

function openRemarkEdit() {
  if (!currentDevice.value) return;
  remarkForm.value.remark = currentDevice.value.remark || "";
  remarkVisible.value = true;
}

async function saveRemark() {
  await api.updateDeviceRemark(currentDevice.value.id, remarkForm.value.remark);
  remarkVisible.value = false;
  ElMessage.success("备注已更新");
  await loadDevices();
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
  await api.restoreFromFile(file.raw);
  ElMessage.success("恢复完成");
  await loadDevices();
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

onMounted(loadDevices);
</script>

<template>
  <div class="min-h-screen bg-[radial-gradient(1200px_500px_at_20%_-10%,#dbeafe,transparent),radial-gradient(1200px_500px_at_90%_0%,#cffafe,transparent),#f1f5f9]">
    <header class="border-b bg-white/90 backdrop-blur">
      <div class="mx-auto flex max-w-[1500px] items-center justify-between px-5 py-3">
        <div>
          <h1 class="text-2xl font-bold text-slate-900">NetPulse 网络监控平台</h1>
          <p class="text-xs text-slate-500">多设备监控 · 审计可追踪 · 移动端协同</p>
        </div>
        <div class="flex items-center gap-2">
          <el-button type="primary" @click="addVisible = true">新增设备</el-button>
          <el-button @click="onBackup">下载备份</el-button>
          <el-upload :auto-upload="false" :show-file-list="false" accept=".gz" @change="onRestore"><el-button>恢复备份</el-button></el-upload>
          <el-button v-if="isAdmin" @click="openUsers">用户管理</el-button>
          <el-button v-if="isAdmin" @click="openAudit">审计日志</el-button>
          <el-button type="danger" plain @click="logout">退出</el-button>
        </div>
      </div>
    </header>

    <main class="mx-auto grid max-w-[1500px] grid-cols-12 gap-4 p-5">
      <section class="col-span-12 lg:col-span-4">
        <el-card class="shadow-sm">
          <template #header><div class="flex items-center justify-between"><span>设备总览</span><el-button size="small" @click="loadDevices">刷新</el-button></div></template>
          <el-table v-loading="loading" :data="devices" @row-click="(row)=>selectedId=row.id">
            <el-table-column label="状态" width="80"><template #default="{row}"><span class="inline-block h-2.5 w-2.5 rounded-full" :class="row.status==='online'?'bg-emerald-500':'bg-rose-500'"/></template></el-table-column>
            <el-table-column prop="ip" label="IP" min-width="140" />
            <el-table-column prop="brand" label="品牌" width="90" />
            <el-table-column prop="remark" label="备注" min-width="140" />
            <el-table-column label="操作" width="90"><template #default="{row}"><el-button type="danger" text @click.stop="onDeleteDevice(row.id)">删除</el-button></template></el-table-column>
          </el-table>
        </el-card>
      </section>

      <section class="col-span-12 lg:col-span-8">
        <el-empty v-if="!currentDevice" description="请选择左侧设备查看详情" />
        <div v-else class="space-y-3">
          <div class="flex justify-end"><el-button @click="openRemarkEdit">编辑设备备注</el-button></div>
          <DeviceDetail :device="currentDevice" />
        </div>
      </section>
    </main>

    <el-dialog v-model="loginVisible" title="登录 NetPulse" width="420" :close-on-click-modal="false" :show-close="false">
      <el-form label-position="top">
        <el-form-item label="用户名"><el-input v-model="loginForm.username" /></el-form-item>
        <el-form-item label="密码"><el-input v-model="loginForm.password" show-password @keyup.enter="doLogin" /></el-form-item>
      </el-form>
      <template #footer><el-button type="primary" @click="doLogin">登录</el-button></template>
    </el-dialog>

    <el-dialog v-model="addVisible" title="新增设备" width="520">
      <el-form label-position="top">
        <el-form-item label="IP"><el-input v-model="addForm.ip" placeholder="192.168.1.1" /></el-form-item>
        <el-form-item label="品牌"><el-select v-model="addForm.brand"><el-option label="Huawei" value="Huawei" /><el-option label="H3C" value="H3C" /></el-select></el-form-item>
        <el-form-item label="SNMP Community"><el-input v-model="addForm.community" /></el-form-item>
        <el-form-item label="备注"><el-input v-model="addForm.remark" /></el-form-item>
      </el-form>
      <template #footer><el-button @click="addVisible=false">取消</el-button><el-button type="primary" @click="onAddDevice">保存</el-button></template>
    </el-dialog>

    <el-dialog v-model="remarkVisible" title="编辑设备备注" width="480">
      <el-input v-model="remarkForm.remark" type="textarea" :rows="4" />
      <template #footer><el-button @click="remarkVisible=false">取消</el-button><el-button type="primary" @click="saveRemark">更新</el-button></template>
    </el-dialog>

    <el-dialog v-model="usersVisible" title="用户管理" width="760">
      <div class="mb-3 grid grid-cols-3 gap-2">
        <el-input v-model="addUserForm.username" placeholder="用户名" />
        <el-input v-model="addUserForm.password" placeholder="密码" />
        <div class="flex gap-2"><el-select v-model="addUserForm.role"><el-option value="user" label="普通用户"/><el-option value="admin" label="管理员"/></el-select><el-button type="primary" @click="createUser">创建</el-button></div>
      </div>
      <el-table :data="users"><el-table-column prop="id" label="ID" width="80"/><el-table-column prop="username" label="用户名"/><el-table-column prop="role" label="角色" width="120"/><el-table-column prop="enabled" label="启用" width="80"/></el-table>
    </el-dialog>

    <el-dialog v-model="auditVisible" title="审计日志" width="980">
      <el-table :data="auditLogs" height="520">
        <el-table-column prop="created_at" label="时间" width="190"/>
        <el-table-column prop="username" label="用户名" width="120"/>
        <el-table-column prop="action" label="动作" width="180"/>
        <el-table-column prop="method" label="方法" width="90"/>
        <el-table-column prop="path" label="路径" min-width="220"/>
        <el-table-column prop="ip" label="IP" width="160"/>
        <el-table-column prop="detail" label="详情" min-width="220"/>
      </el-table>
    </el-dialog>
  </div>
</template>
