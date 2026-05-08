<script setup>
import { computed, onBeforeUnmount, onMounted, ref, watch } from "vue";
import { useRoute, useRouter } from "vue-router";
import { useAuthStore } from "./stores/auth";
import { useOpsStore } from "./stores/ops";
import { api } from "./services/api";
import { zhCN } from "./i18n/zhCN";
import { useFeedback } from "./composables/useFeedback";

const route = useRoute();
const router = useRouter();
const auth = useAuthStore();
const ops = useOpsStore();
const fb = useFeedback();

const isMobile = ref(false);
const sidebarOpen = ref(true);
const loginVisible = ref(!auth.isAuthed);
const loginForm = ref({ username: "", password: "" });

const usersVisible = ref(false);
const users = ref([]);
const addUserForm = ref({ username: "", password: "", role: "user" });
const editUserVisible = ref(false);
const editUserForm = ref({ id: null, username: "", password: "", role: "user" });
const permVisible = ref(false);
const permUser = ref(null);
const permValues = ref([]);
const permissionCatalog = [
  "device.read", "device.write", "metrics.read", "logs.read"
];

const quickSearchVisible = ref(false);
const quickSearchKeyword = ref("");
const quickSearchLoading = ref(false);
const quickSearchCategory = ref("all");
let quickSearchDebounce = null;
let authExpiredNoticeAt = 0;
const quickPinned = ref(JSON.parse(localStorage.getItem("np_quick_pinned") || "[]"));
const quickRecent = ref(JSON.parse(localStorage.getItem("np_quick_recent") || "[]"));
const filteredSearchResults = computed(() => {
  const list = ops.globalSearchResults || [];
  if (quickSearchCategory.value === "all") return list;
  return list.filter((x) => String(x.category || "").toLowerCase() === quickSearchCategory.value);
});

const pageTitle = computed(() => String(route.meta?.title || zhCN.app.title));
const isAuthed = computed(() => auth.isAuthed);
const isAdmin = computed(() => auth.isAdmin);
const currentUser = computed(() => auth.user);

const menuItems = [
  { path: "/dashboard", label: "仪表盘" },
  { path: "/assets", label: "资产中心" },
  { path: "/alerts", label: "告警与日志" },
  { path: "/settings", label: "设置" }
];

function searchCategoryLabel(v) {
  const x = String(v || "").toLowerCase();
  if (x === "device") return "设备";
  if (x === "interface" || x === "port") return "端口";
  if (x === "log" || x === "event") return "事件";
  if (x === "user") return "用户";
  return v || "-";
}

function escapeHtml(s) {
  return String(s || "")
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll("\"", "&quot;")
    .replaceAll("'", "&#39;");
}

function highlightText(text) {
  const kw = String(quickSearchKeyword.value || "").trim();
  const safe = escapeHtml(text);
  if (!kw) return safe;
  const reg = new RegExp(`(${kw.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")})`, "ig");
  return safe.replace(reg, "<mark class=\"np-hl\">$1</mark>");
}

const activeMenu = computed(() => {
  if (route.path.startsWith("/assets") || route.path.startsWith("/device/") || route.path.startsWith("/port/")) return "/assets";
  if (route.path.startsWith("/alerts")) return "/alerts";
  if (route.path.startsWith("/settings")) return "/settings";
  return "/dashboard";
});

async function doLogin() {
  try {
    await auth.login(loginForm.value.username, loginForm.value.password);
    loginVisible.value = false;
    fb.success("登录成功");
    router.push("/dashboard");
  } catch (err) {
    fb.apiError(err, "用户名或密码错误");
  }
}

function logout() {
  auth.logout();
  loginVisible.value = true;
  loginForm.value = { username: "", password: "" };
  fb.success("已退出登录");
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
    fb.apiError(err, "加载用户失败");
  }
}

async function createUser() {
  try {
    await api.createUser(addUserForm.value);
    fb.success("用户已创建");
    addUserForm.value = { username: "", password: "", role: "user" };
    const res = await api.listUsers();
    users.value = res.data || [];
  } catch (err) {
    fb.apiError(err, "创建用户失败");
  }
}

function openEditUser(row) {
  editUserForm.value = { id: row.id, username: row.username || "", password: "", role: row.role || "user" };
  editUserVisible.value = true;
}

async function saveEditUser() {
  try {
    await api.updateUser(editUserForm.value.id, {
      username: editUserForm.value.username,
      password: editUserForm.value.password,
      role: editUserForm.value.role
    });
    fb.success("用户已更新");
    editUserVisible.value = false;
    const res = await api.listUsers();
    users.value = res.data || [];
  } catch (err) {
    fb.apiError(err, "更新用户失败");
  }
}

async function deleteUser(row) {
  try {
    await api.deleteUser(row.id);
    fb.success("用户已删除");
    const res = await api.listUsers();
    users.value = res.data || [];
  } catch (err) {
    fb.apiError(err, "删除用户失败");
  }
}

async function openPerms(row) {
  try {
    permUser.value = row;
    const res = await api.getUserPermissions(row.id);
    permValues.value = res.data?.permissions || [];
    permVisible.value = true;
  } catch (err) {
    fb.apiError(err, "加载权限失败");
  }
}

async function savePerms() {
  try {
    if (!permUser.value?.id) return;
    await api.setUserPermissions(permUser.value.id, permValues.value);
    fb.success("权限已更新");
    permVisible.value = false;
  } catch (err) {
    fb.apiError(err, "保存权限失败");
  }
}

async function runQuickSearch() {
  quickSearchLoading.value = true;
  try {
    await ops.runGlobalSearch(quickSearchKeyword.value);
  } catch (err) {
    fb.apiError(err, "全局搜索失败");
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
  if (item?.id) {
    const nowItem = { category: item.category, id: item.id, title: item.title, sub: item.sub };
    quickRecent.value = [nowItem, ...quickRecent.value.filter((x) => !(x.category === nowItem.category && x.id === nowItem.id))].slice(0, 12);
    localStorage.setItem("np_quick_recent", JSON.stringify(quickRecent.value));
  }
  quickSearchVisible.value = false;
}

function togglePin(item) {
  const exists = quickPinned.value.find((x) => x.category === item.category && x.id === item.id);
  if (exists) {
    quickPinned.value = quickPinned.value.filter((x) => !(x.category === item.category && x.id === item.id));
  } else {
    quickPinned.value = [{ category: item.category, id: item.id, title: item.title, sub: item.sub }, ...quickPinned.value].slice(0, 20);
  }
  localStorage.setItem("np_quick_pinned", JSON.stringify(quickPinned.value));
}

function isPinned(item) {
  return !!quickPinned.value.find((x) => x.category === item.category && x.id === item.id);
}

function onAuthExpired() {
  auth.logout();
  loginVisible.value = true;
  loginForm.value = { username: "", password: "" };
  const now = Date.now();
  if (now - authExpiredNoticeAt > 3000) {
    authExpiredNoticeAt = now;
    fb.warn("登录已失效，请重新登录");
  }
}

onMounted(() => {
  onResize();
  window.addEventListener("resize", onResize);
  window.addEventListener("keydown", onGlobalKeydown);
  window.addEventListener("netpulse-auth-expired", onAuthExpired);
});

watch(quickSearchKeyword, () => {
  if (!quickSearchVisible.value) return;
  if (quickSearchDebounce) clearTimeout(quickSearchDebounce);
  quickSearchDebounce = setTimeout(() => {
    runQuickSearch();
  }, 260);
});

onBeforeUnmount(() => {
  window.removeEventListener("resize", onResize);
  window.removeEventListener("keydown", onGlobalKeydown);
  window.removeEventListener("netpulse-auth-expired", onAuthExpired);
  if (quickSearchDebounce) clearTimeout(quickSearchDebounce);
});
</script>

<template>
  <div class="np-app v2">
    <aside class="sidebar" :class="{ open: !isMobile || sidebarOpen, mobile: isMobile }">
      <div class="px-5 pb-5 pt-6">
        <div class="text-2xl font-semibold tracking-wide text-white">NetPulse</div>
        <div class="mt-1 text-xs text-slate-400">{{ zhCN.app.edition }}</div>
      </div>

      <el-menu :default-active="activeMenu" class="np-menu" background-color="transparent" text-color="#94a3b8" active-text-color="#ffffff" @select="onSelectMenu">
        <el-menu-item v-for="item in menuItems" :key="item.path" :index="item.path">{{ item.label }}</el-menu-item>
      </el-menu>

      <div class="mt-auto border-t border-white/10 px-4 py-4">
        <div class="text-xs text-slate-400">当前用户</div>
        <div class="mt-1 text-sm text-slate-100">{{ currentUser?.username || "未登录" }}</div>
        <div class="mt-3 flex gap-2">
          <el-tooltip v-if="!isAdmin" content="仅管理员可管理用户" placement="top">
            <el-button size="small" disabled>用户管理</el-button>
          </el-tooltip>
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

    <el-dialog v-model="loginVisible" class="np-login-dialog" title="登录 NetPulse" width="420" :close-on-click-modal="false" :show-close="false">
      <el-form label-position="top">
        <el-form-item label="用户名"><el-input v-model="loginForm.username" placeholder="请输入用户名" /></el-form-item>
        <el-form-item label="密码"><el-input v-model="loginForm.password" show-password placeholder="请输入密码" @keyup.enter="doLogin" /></el-form-item>
      </el-form>
      <template #footer><el-button type="primary" class="w-full" @click="doLogin">登录</el-button></template>
    </el-dialog>

    <el-dialog v-model="quickSearchVisible" title="全局搜索 (Ctrl+K)" width="760">
      <div class="mb-3">
        <div class="mb-1 text-xs text-slate-500">已收藏</div>
        <div class="flex flex-wrap gap-2">
          <el-tag v-for="x in quickPinned" :key="`pin-${x.category}-${x.id}`" class="cursor-pointer" @click="goSearchResult(x)">{{ x.title }}</el-tag>
          <span v-if="!quickPinned.length" class="text-xs text-slate-400">暂无收藏</span>
        </div>
      </div>
      <div class="mb-3">
        <div class="mb-1 text-xs text-slate-500">最近访问</div>
        <div class="flex flex-wrap gap-2">
          <el-tag v-for="x in quickRecent" :key="`recent-${x.category}-${x.id}`" type="info" class="cursor-pointer" @click="goSearchResult(x)">{{ x.title }}</el-tag>
          <span v-if="!quickRecent.length" class="text-xs text-slate-400">暂无记录</span>
        </div>
      </div>
      <div class="flex gap-2">
        <el-input v-model="quickSearchKeyword" placeholder="搜索 IP / 备注 / 端口名 / 设备名" @keyup.enter="runQuickSearch" />
        <el-select v-model="quickSearchCategory" class="w-[120px]">
          <el-option label="全部" value="all" />
          <el-option label="设备" value="device" />
          <el-option label="端口" value="interface" />
          <el-option label="日志" value="device_log" />
          <el-option label="审计" value="audit_log" />
        </el-select>
        <el-button type="primary" :loading="quickSearchLoading" @click="runQuickSearch">搜索</el-button>
      </div>
      <el-table :data="filteredSearchResults" class="mt-3 np-borderless-table" max-height="420" @row-click="goSearchResult">
        <el-table-column label="类型" width="120">
          <template #default="{ row }">{{ searchCategoryLabel(row.category) }}</template>
        </el-table-column>
        <el-table-column label="标题" min-width="240">
          <template #default="{ row }"><span v-html="highlightText(row.title)" /></template>
        </el-table-column>
        <el-table-column label="详情" min-width="320">
          <template #default="{ row }"><span v-html="highlightText(row.sub)" /></template>
        </el-table-column>
        <el-table-column label="收藏" width="90">
          <template #default="{ row }">
            <el-button text @click.stop="togglePin(row)">{{ isPinned(row) ? "取消" : "收藏" }}</el-button>
          </template>
        </el-table-column>
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
        <el-table-column label="操作" width="260">
          <template #default="{ row }">
            <el-button type="primary" text @click="openEditUser(row)">编辑</el-button>
            <el-button type="warning" text @click="openPerms(row)">权限</el-button>
            <el-button type="danger" text @click="deleteUser(row)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>
    </el-dialog>

    <el-dialog v-model="editUserVisible" title="编辑用户" width="480">
      <el-form label-position="top">
        <el-form-item label="用户名"><el-input v-model="editUserForm.username" /></el-form-item>
        <el-form-item label="新密码（可空）"><el-input v-model="editUserForm.password" show-password /></el-form-item>
        <el-form-item label="角色">
          <el-select v-model="editUserForm.role" class="w-full">
            <el-option value="user" label="普通用户" />
            <el-option value="admin" label="管理员" />
          </el-select>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="editUserVisible = false">取消</el-button>
        <el-button type="primary" @click="saveEditUser">保存</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="permVisible" :title="`权限配置 - ${permUser?.username || ''}`" width="520">
      <el-checkbox-group v-model="permValues" class="grid grid-cols-2 gap-2">
        <el-checkbox v-for="p in permissionCatalog" :key="p" :label="p">{{ p }}</el-checkbox>
      </el-checkbox-group>
      <template #footer>
        <el-button @click="permVisible = false">取消</el-button>
        <el-button type="primary" @click="savePerms">保存</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
.np-hl {
  background: #fde68a;
  padding: 0 2px;
  border-radius: 2px;
}
</style>
