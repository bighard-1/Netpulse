import { createRouter, createWebHistory } from "vue-router";

const routes = [
  {
    path: "/",
    name: "assets",
    component: () => import("../views/DeviceList.vue"),
    meta: { title: "资产总览" }
  },
  {
    path: "/logs",
    name: "global-logs",
    component: () => import("../views/GlobalLogs.vue"),
    meta: { title: "全局日志" }
  },
  {
    path: "/topology",
    name: "topology",
    component: () => import("../views/Topology.vue"),
    meta: { title: "拓扑" }
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
