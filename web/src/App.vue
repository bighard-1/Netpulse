<script setup>
import { computed, onBeforeUnmount, onMounted, ref } from "vue";
import { ElMessage } from "element-plus";
import { useRoute, useRouter } from "vue-router";
import { api, getApiError } from "./services/api";

const route = useRoute();
const router = useRouter();

const token = ref(localStorage.getItem("netpulse_token") || "");
const currentUser = ref(JSON.parse(localStorage.getItem("netpulse_user") || "null"));
const isMobile = ref(false);
const sidebarOpen = ref(true);

const loginVisible = ref(!token.value);
const loginForm = ref({ username: "", password: "" });

const usersVisible = ref(false);
const users = ref([]);
const addUserForm = ref({ username: "", password: "", role: "user" });
const editUserVisible = ref(false);
const editUserForm = ref({ id: null, username: "", password: "", role: "user" });
const permissionsVisible = ref(false);
const activePermissionUser = ref(null);
const permissionOptions = ["device.read", "device.write", "metrics.read", "logs.read"];
const selectedPermissions = ref([]);

const isAdmin = computed(() => currentUser.value?.role === "admin");
const pageTitle = computed(() => String(route.meta?.title || "NetPulse"));

const menuItems = [
  { key: "assets", path: "/", label: "资产总览" },
  { key: "logs", path: "/logs", label: "全局日志" },
  { key: "topology", path: "/topology", label: "网络拓扑" },
  { key: "settings", path: "/settings", label: "系统设置" }
];

const activeMenu = computed(() => {
  if (route.path.startsWith("/logs")) return "/logs";
  if (route.path.startsWith("/topology")) return "/topology";
  if (route.path.startsWith("/settings")) return "/settings";
  return "/";
});
const mainSidebarClass = computed(() => ({
  open: !isMobile.value || sidebarOpen.value,
  mobile: isMobile.value
}));

async function doLogin() {
  try {
    const res = await api.login(loginForm.value.username, loginForm.value.password);
    localStorage.setItem("netpulse_token", res.data.token);
    localStorage.setItem("netpulse_user", JSON.stringify(res.data.user));
    token.value = res.data.token;
    currentUser.value = res.data.user;
    loginVisible.value = false;
    ElMessage.success("登录成功");
    if (route.path === "/") return;
    router.push("/");
  } catch (err) {
    ElMessage.error(getApiError(err, "用户名或密码错误"));
  }
}

function logout() {
  localStorage.removeItem("netpulse_token");
  localStorage.removeItem("netpulse_user");
  token.value = "";
  currentUser.value = null;
  loginForm.value = { username: "", password: "" };
  loginVisible.value = true;
  ElMessage.success("已退出登录");
  router.push("/");
}

const isAuthed = computed(() => !!token.value);

function onResize() {
  isMobile.value = window.innerWidth < 960;
  if (!isMobile.value) sidebarOpen.value = true;
}

function onSelectMenu(idx) {
  router.push(idx);
  if (isMobile.value) sidebarOpen.value = false;
}

async function openUsers() {
  try {
    usersVisible.value = true;
    const res = await api.listUsers();
    users.value = res.data || [];
  } catch (err) {
    ElMessage.error(getApiError(err, "加载用户失败"));
  }
}

async function createUser() {
  try {
    await api.createUser(addUserForm.value);
    ElMessage.success("用户已创建");
    addUserForm.value = { username: "", password: "", role: "user" };
    const res = await api.listUsers();
    users.value = res.data || [];
  } catch (err) {
    ElMessage.error(getApiError(err, "创建用户失败"));
  }
}

function openEditUser(row) {
  editUserForm.value = { id: row.id, username: row.username, password: "", role: row.role };
  editUserVisible.value = true;
}

async function updateUser() {
  const payload = {
    username: editUserForm.value.username,
    role: editUserForm.value.role
  };
  if (editUserForm.value.password) payload.password = editUserForm.value.password;
  try {
    await api.updateUser(editUserForm.value.id, payload);
    ElMessage.success("用户已更新");
    editUserVisible.value = false;
    const res = await api.listUsers();
    users.value = res.data || [];
  } catch (err) {
    ElMessage.error(getApiError(err, "更新用户失败"));
  }
}

async function removeUser(row) {
  try {
    await api.deleteUser(row.id);
    ElMessage.success("用户已删除");
    const res = await api.listUsers();
    users.value = res.data || [];
  } catch (err) {
    ElMessage.error(getApiError(err, "删除用户失败"));
  }
}

async function openPermissions(row) {
  try {
    activePermissionUser.value = row;
    const res = await api.getUserPermissions(row.id);
    selectedPermissions.value = res.data?.permissions || [];
    permissionsVisible.value = true;
  } catch (err) {
    ElMessage.error(getApiError(err, "加载权限失败"));
  }
}

async function savePermissions() {
  if (!activePermissionUser.value) return;
  try {
    await api.setUserPermissions(activePermissionUser.value.id, selectedPermissions.value);
    ElMessage.success("权限已更新");
    permissionsVisible.value = false;
  } catch (err) {
    ElMessage.error(getApiError(err, "保存权限失败"));
  }
}

onMounted(() => {
  onResize();
  window.addEventListener("resize", onResize);
  if (!token.value) loginVisible.value = true;
});

onBeforeUnmount(() => {
  window.removeEventListener("resize", onResize);
});
</script>

<template>
  <div class="np-app">
    <aside class="sidebar" :class="mainSidebarClass">
      <div class="px-5 pb-5 pt-6">
        <div class="text-2xl font-semibold tracking-wide text-white">NetPulse</div>
        <div class="mt-1 text-xs text-slate-400">网络运维中心</div>
      </div>
      <el-menu
        :default-active="activeMenu"
        class="np-menu"
        background-color="transparent"
        text-color="#94a3b8"
        active-text-color="#ffffff"
        @select="onSelectMenu"
      >
        <el-menu-item v-for="item in menuItems" :key="item.key" :index="item.path">
          <span>{{ item.label }}</span>
        </el-menu-item>
      </el-menu>

      <div class="mt-auto border-t border-white/10 px-4 py-4">
        <div class="text-xs text-slate-400">当前用户</div>
        <div class="mt-1 text-sm text-slate-100">{{ currentUser?.username || "未登录" }}</div>
        <div class="mt-3 flex gap-2">
          <el-button v-if="isAdmin" size="small" @click="openUsers">用户管理</el-button>
          <el-button size="small" type="danger" plain @click="logout">退出</el-button>
        </div>
      </div>
    </aside>
    <div v-if="isMobile && sidebarOpen" class="np-overlay" @click="sidebarOpen = false"></div>

    <main class="np-main">
      <header class="np-topbar">
        <div class="flex items-center gap-3">
          <el-button v-if="isMobile" class="np-menu-trigger" @click="sidebarOpen = !sidebarOpen">菜单</el-button>
          <div>
          <h2 class="text-xl font-semibold text-slate-900">{{ pageTitle }}</h2>
          <div class="text-xs text-slate-500">专业网络运维中心</div>
          </div>
        </div>
      </header>

      <section class="np-content">
        <router-view v-if="isAuthed" />
        <el-empty v-else description="请先登录后使用 NetPulse" :image-size="88" />
      </section>
    </main>

    <el-dialog v-model="loginVisible" class="np-login-dialog" title="登录 NetPulse" width="420" :close-on-click-modal="false" :show-close="false">
      <el-form label-position="top">
        <el-form-item label="用户名"><el-input v-model="loginForm.username" placeholder="请输入用户名" /></el-form-item>
        <el-form-item label="密码"><el-input v-model="loginForm.password" show-password placeholder="请输入密码" @keyup.enter="doLogin" /></el-form-item>
      </el-form>
      <template #footer><el-button type="primary" class="w-full" @click="doLogin">登录</el-button></template>
    </el-dialog>

    <el-dialog v-model="usersVisible" title="用户管理" width="860">
      <div class="mb-3 grid grid-cols-3 gap-2">
        <el-input v-model="addUserForm.username" placeholder="用户名" />
        <el-input v-model="addUserForm.password" placeholder="密码" show-password />
        <div class="flex gap-2">
          <el-select v-model="addUserForm.role"><el-option value="user" label="普通用户" /><el-option value="admin" label="管理员" /></el-select>
          <el-button type="primary" @click="createUser">创建</el-button>
        </div>
      </div>
      <el-table :data="users">
        <el-table-column prop="id" label="ID" width="80" />
        <el-table-column prop="username" label="用户名" />
        <el-table-column prop="role" label="角色" width="120" />
        <el-table-column label="操作" width="300">
          <template #default="{ row }">
            <el-button type="primary" text @click="openEditUser(row)">编辑</el-button>
            <el-button type="warning" text @click="openPermissions(row)">权限</el-button>
            <el-button type="danger" text @click="removeUser(row)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>
    </el-dialog>

    <el-dialog v-model="editUserVisible" title="编辑用户" width="520">
      <el-form label-position="top">
        <el-form-item label="用户名"><el-input v-model="editUserForm.username" /></el-form-item>
        <el-form-item label="角色">
          <el-select v-model="editUserForm.role" class="w-full">
            <el-option value="user" label="普通用户" />
            <el-option value="admin" label="管理员" />
          </el-select>
        </el-form-item>
        <el-form-item label="重置密码（留空则不修改）"><el-input v-model="editUserForm.password" show-password /></el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="editUserVisible = false">取消</el-button>
        <el-button type="primary" @click="updateUser">保存</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="permissionsVisible" title="分配权限" width="520">
      <div class="mb-3 text-slate-600">用户：{{ activePermissionUser?.username || "-" }}</div>
      <el-checkbox-group v-model="selectedPermissions" class="grid grid-cols-2 gap-2">
        <el-checkbox v-for="perm in permissionOptions" :key="perm" :label="perm">{{ perm }}</el-checkbox>
      </el-checkbox-group>
      <template #footer>
        <el-button @click="permissionsVisible = false">取消</el-button>
        <el-button type="primary" @click="savePermissions">保存</el-button>
      </template>
    </el-dialog>
  </div>
</template>
