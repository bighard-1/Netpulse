import { createRouter, createWebHistory } from "vue-router";

const routes = [
  {
    path: "/",
    name: "assets",
    component: () => import("../views/DeviceList.vue"),
    meta: { title: "资产列表" }
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
