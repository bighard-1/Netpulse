<script setup>
import { computed, onBeforeUnmount, onMounted, ref } from "vue";
import { ElMessage } from "element-plus";
import { useRoute, useRouter } from "vue-router";
import { useAuthStore } from "./stores/auth";
import { useOpsStore } from "./stores/ops";
import { api, getApiError } from "./services/api";

const route = useRoute();
const router = useRouter();
const auth = useAuthStore();
const ops = useOpsStore();

const isMobile = ref(false);
const sidebarOpen = ref(true);
const loginVisible = ref(!auth.isAuthed);
const loginForm = ref({ username: "", password: "" });

const usersVisible = ref(false);
const users = ref([]);
const addUserForm = ref({ username: "", password: "", role: "user" });

const quickSearchVisible = ref(false);
const quickSearchKeyword = ref("");
const quickSearchLoading = ref(false);

const pageTitle = computed(() => String(route.meta?.title || "NetPulse"));
const isAuthed = computed(() => auth.isAuthed);
const isAdmin = computed(() => auth.isAdmin);
const currentUser = computed(() => auth.user);

const menuItems = [
  { path: "/dashboard", label: "仪表盘" },
  { path: "/assets", label: "资产中心" },
  { path: "/alerts", label: "告警与日志" },
  { path: "/settings", label: "设置" }
];

const activeMenu = computed(() => {
  if (route.path.startsWith("/assets")) return "/assets";
  if (route.path.startsWith("/alerts")) return "/alerts";
  if (route.path.startsWith("/settings")) return "/settings";
  return "/dashboard";
});

async function doLogin() {
  try {
    await auth.login(loginForm.value.username, loginForm.value.password);
    loginVisible.value = false;
    ElMessage.success("登录成功");
    router.push("/dashboard");
  } catch (err) {
    ElMessage.error(getApiError(err, "用户名或密码错误"));
  }
}

function logout() {
  auth.logout();
  loginVisible.value = true;
  loginForm.value = { username: "", password: "" };
  ElMessage.success("已退出登录");
  router.push("/dashboard");
}

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

async function runQuickSearch() {
  quickSearchLoading.value = true;
  try {
    await ops.runGlobalSearch(quickSearchKeyword.value);
  } catch (err) {
    ElMessage.error(getApiError(err, "全局搜索失败"));
  } finally {
    quickSearchLoading.value = false;
  }
}

function openQuickSearch() {
  quickSearchVisible.value = true;
  setTimeout(() => runQuickSearch(), 0);
}

function onGlobalKeydown(e) {
  if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === "k") {
    e.preventDefault();
    openQuickSearch();
  }
}

function goSearchResult(item) {
  if (item?.category === "device" && item?.id) {
    router.push(`/device/${item.id}`);
  } else if (item?.category === "interface" && item?.id) {
    router.push(`/port/${item.id}`);
  }
  quickSearchVisible.value = false;
}

function onAuthExpired() {
  auth.logout();
  loginVisible.value = true;
  loginForm.value = { username: "", password: "" };
  ElMessage.warning("登录已失效，请重新登录");
}

onMounted(() => {
  onResize();
  window.addEventListener("resize", onResize);
  window.addEventListener("keydown", onGlobalKeydown);
  window.addEventListener("netpulse-auth-expired", onAuthExpired);
});

onBeforeUnmount(() => {
  window.removeEventListener("resize", onResize);
  window.removeEventListener("keydown", onGlobalKeydown);
  window.removeEventListener("netpulse-auth-expired", onAuthExpired);
});
</script>

<template>
  <div class="np-app v2">
    <aside class="sidebar" :class="{ open: !isMobile || sidebarOpen, mobile: isMobile }">
      <div class="px-5 pb-5 pt-6">
        <div class="text-2xl font-semibold tracking-wide text-white">NetPulse</div>
        <div class="mt-1 text-xs text-slate-400">Professional O&M Edition</div>
      </div>

      <el-menu :default-active="activeMenu" class="np-menu" background-color="transparent" text-color="#94a3b8" active-text-color="#ffffff" @select="onSelectMenu">
        <el-menu-item v-for="item in menuItems" :key="item.path" :index="item.path">{{ item.label }}</el-menu-item>
      </el-menu>

      <div class="mt-auto border-t border-white/10 px-4 py-4">
        <div class="text-xs text-slate-400">当前用户</div>
        <div class="mt-1 text-sm text-slate-100">{{ currentUser?.username || "未登录" }}</div>
        <div class="mt-3 flex gap-2">
          <el-button v-if="isAdmin" size="small" @click="openUsers">用户管理</el-button>
          <el-button size="small" plain @click="openQuickSearch">Ctrl+K</el-button>
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
            <div class="text-xs text-slate-500">深海蓝高密度运维工作台</div>
          </div>
        </div>
      </header>

      <section class="np-content">
        <router-view v-if="isAuthed" />
        <el-empty v-else description="请先登录后使用 NetPulse" :image-size="88" />
      </section>
    </main>

    <el-dialog v-model="loginVisible" title="登录 NetPulse" width="420" :close-on-click-modal="false" :show-close="false">
      <el-form label-position="top">
        <el-form-item label="用户名"><el-input v-model="loginForm.username" placeholder="请输入用户名" /></el-form-item>
        <el-form-item label="密码"><el-input v-model="loginForm.password" show-password placeholder="请输入密码" @keyup.enter="doLogin" /></el-form-item>
      </el-form>
      <template #footer><el-button type="primary" class="w-full" @click="doLogin">登录</el-button></template>
    </el-dialog>

    <el-dialog v-model="quickSearchVisible" title="全局搜索 (Ctrl+K)" width="760">
      <div class="flex gap-2">
        <el-input v-model="quickSearchKeyword" placeholder="搜索 IP / 备注 / 端口名 / 设备名" @keyup.enter="runQuickSearch" />
        <el-button type="primary" :loading="quickSearchLoading" @click="runQuickSearch">搜索</el-button>
      </div>
      <el-table :data="ops.globalSearchResults" class="mt-3 np-borderless-table" max-height="420" @row-click="goSearchResult">
        <el-table-column prop="category" label="类型" width="120" />
        <el-table-column prop="title" label="标题" min-width="240" />
        <el-table-column prop="sub" label="详情" min-width="320" />
      </el-table>
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
      </el-table>
    </el-dialog>
  </div>
</template>
