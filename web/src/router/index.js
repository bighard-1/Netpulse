import { createRouter, createWebHistory } from "vue-router";

const routes = [
  {
    path: "/",
    redirect: "/dashboard"
  },
  {
    path: "/dashboard",
    name: "dashboard",
    component: () => import("../views/DeviceList.vue"),
    meta: { title: "NOC 仪表盘" }
  },
  {
    path: "/assets",
    name: "assets",
    component: () => import("../views/DeviceList.vue"),
    meta: { title: "资产中心" }
  },
  {
    path: "/alerts",
    name: "alerts",
    component: () => import("../views/GlobalLogs.vue"),
    meta: { title: "告警与日志" }
  },
  {
    path: "/settings",
    name: "system-settings",
    component: () => import("../views/SystemSettings.vue"),
    meta: { title: "系统设置" }
  },
  {
    path: "/device/:id",
    name: "device-detail",
    component: () => import("../views/DeviceDetail.vue"),
    props: true,
    meta: { title: "设备详情" }
  },
  {
    path: "/port/:id",
    name: "port-detail",
    component: () => import("../views/PortDetail.vue"),
    props: true,
    meta: { title: "端口详情" }
  }
];

const router = createRouter({
  history: createWebHistory(),
  routes
});

export default router;
