import { createRouter, createWebHistory } from "vue-router";
import DeviceList from "../views/DeviceList.vue";
import DeviceDetail from "../views/DeviceDetail.vue";
import PortDetail from "../views/PortDetail.vue";

const routes = [
  {
    path: "/",
    name: "assets",
    component: DeviceList,
    meta: { title: "资产列表" }
  },
  {
    path: "/device/:id",
    name: "device-detail",
    component: DeviceDetail,
    props: true,
    meta: { title: "设备详情" }
  },
  {
    path: "/port/:id",
    name: "port-detail",
    component: PortDetail,
    props: true,
    meta: { title: "端口详情" }
  }
];

const router = createRouter({
  history: createWebHistory(),
  routes
});

export default router;
