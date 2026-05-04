import axios from "axios";

const http = axios.create({
  baseURL: "/api",
  timeout: 20000
});

export const api = {
  listDevices() {
    return http.get("/devices");
  },
  addDevice(payload) {
    return http.post("/devices", payload);
  },
  deleteDevice(id) {
    return http.delete(`/devices/${id}`);
  },
  updateDeviceRemark(id, remark) {
    return http.put(`/devices/${id}/remark`, { remark });
  },
  updateInterfaceRemark(id, remark) {
    return http.put(`/interfaces/${id}/remark`, { remark });
  },
  getHistory(type, id, start, end) {
    return http.get("/metrics/history", {
      params: { type, id, start, end }
    });
  },
  getDeviceLogs(id) {
    return http.get(`/devices/${id}/logs`);
  },
  downloadBackup() {
    return http.get("/system/backup", { responseType: "blob" });
  },
  restoreFromFile(file) {
    const form = new FormData();
    form.append("file", file);
    return http.post("/system/restore", form, {
      headers: { "Content-Type": "multipart/form-data" }
    });
  }
};

