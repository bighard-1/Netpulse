import axios from "axios";

const API_BASE =
  import.meta.env.VITE_API_BASE_URL || "http://119.40.55.18:18080/api";

const http = axios.create({
  baseURL: API_BASE,
  timeout: 20000
});

http.interceptors.request.use((config) => {
  const token = localStorage.getItem("netpulse_token");
  if (token) config.headers.Authorization = `Bearer ${token}`;
  return config;
});

export function getApiError(err, fallback = "请求失败") {
  return (
    err?.response?.data?.error ||
    err?.response?.data?.message ||
    err?.message ||
    fallback
  );
}

export const api = {
  login(username, password) {
    return http.post("/auth/login", { username, password });
  },
  listDevices() {
    return http.get("/devices");
  },
  globalSearch(q) {
    return http.get("/search", { params: { q } });
  },
  async getDeviceById(id) {
    const res = await http.get(`/devices/${id}`);
    return res.data || null;
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
  updateInterfaceProfile(id, payload) {
    return http.put(`/interfaces/${id}`, payload);
  },
  getHistory(type, id, start, end) {
    return http.get("/metrics/history", {
      params: { type, id, start, end }
    });
  },
  getDeviceLogs(id) {
    return http.get(`/devices/${id}/logs`);
  },
  diagnoseDevice(id) {
    return http.get(`/devices/${id}/diagnose`);
  },
  exportDiagnosis(id, format = "txt") {
    return http.get(`/devices/${id}/diagnose`, {
      params: { format, download: 1 },
      responseType: "blob"
    });
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
  },
  listUsers() {
    return http.get("/admin/users");
  },
  createUser(payload) {
    return http.post("/admin/users", payload);
  },
  updateUser(id, payload) {
    return http.put(`/users/${id}`, payload);
  },
  deleteUser(id) {
    return http.delete(`/users/${id}`);
  },
  getUserPermissions(id) {
    return http.get(`/users/${id}/permissions`);
  },
  setUserPermissions(id, permissions) {
    return http.put(`/users/${id}/permissions`, { permissions });
  },
  listAuditLogs() {
    return http.get("/audit/logs");
  },
  importDevicesCSV(csvText) {
    return http.post("/devices/import", csvText, {
      headers: { "Content-Type": "text/csv" }
    });
  },
  listTemplates() {
    return http.get("/templates");
  },
  createTemplate(payload) {
    return http.post("/templates", payload);
  },
  listTopology() {
    return http.get("/topology");
  },
  upsertTopology(payload) {
    return http.post("/topology", payload);
  },
  deleteTopology(id) {
    return http.delete(`/topology/${id}`);
  },
  listAlertRules() {
    return http.get("/alerts/rules");
  },
  upsertAlertRule(payload) {
    return http.post("/alerts/rules", payload);
  },
  discoveryScan(payload) {
    return http.post("/discovery/scan", payload);
  },
  backupDrill() {
    return http.post("/system/backup/drill");
  },
  listBackupDrillReports() {
    return http.get("/system/backup/drill/reports");
  },
  getRuntimeSettings() {
    return http.get("/settings/runtime");
  },
  updateRuntimeSettings(payload) {
    return http.put("/settings/runtime", payload);
  }
};
