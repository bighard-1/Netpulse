import axios from "axios";

const API_BASE =
  import.meta.env.VITE_API_BASE_URL || "http://119.40.55.18:18080/api";

const http = axios.create({
  baseURL: API_BASE,
  timeout: 20000
});

function normalizeToken(raw) {
  if (!raw) return "";
  const v = String(raw).trim();
  if (!v) return "";
  if (v.startsWith('"') && v.endsWith('"')) {
    try {
      return JSON.parse(v);
    } catch {
      return v.slice(1, -1);
    }
  }
  return v;
}

http.interceptors.request.use((config) => {
  const token = normalizeToken(localStorage.getItem("netpulse_token"));
  if (token) config.headers.Authorization = `Bearer ${token}`;
  return config;
});

http.interceptors.response.use(
  (resp) => resp,
  (err) => {
    const status = err?.response?.status;
    const msg = String(err?.response?.data?.error || "").toLowerCase();
    if (
      status === 401 &&
      (msg.includes("invalid token") || msg.includes("missing bearer token"))
    ) {
      localStorage.removeItem("netpulse_token");
      localStorage.removeItem("netpulse_user");
      if (typeof window !== "undefined") {
        window.dispatchEvent(new CustomEvent("netpulse-auth-expired"));
      }
    }
    return Promise.reject(err);
  }
);

export function getApiError(err, fallback = "请求失败") {
  return (
    err?.response?.data?.error ||
    err?.response?.data?.message ||
    err?.message ||
    fallback
  );
}

export function getApiErrorDetail(err, fallback = "请求失败") {
  return {
    code: err?.response?.data?.code || "",
    message:
      err?.response?.data?.error ||
      err?.response?.data?.message ||
      err?.message ||
      fallback,
    hint: err?.response?.data?.hint || ""
  };
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
  updateDevice(id, payload) {
    return http.put(`/devices/${id}`, payload);
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
  getHistory(type, id, start, end, interval = "") {
    return http.get("/metrics/history", {
      params: { type, id, start, end, interval }
    });
  },
  precheckDevice(payload) {
    return http.post("/devices/precheck", payload);
  },
  getDeviceCapabilities(id) {
    return http.get(`/devices/${id}/capabilities`);
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
  listRecentEvents(limit = 20) {
    return http.get("/events/recent", { params: { limit } });
  },
  listAlertEvents(limit = 200, status = "") {
    return http.get("/alerts/events", { params: { limit, status } });
  },
  updateAlertEvent(id, payload) {
    return http.put(`/alerts/events/${id}`, payload);
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
  listAlertRules() {
    return http.get("/alerts/rules");
  },
  upsertAlertRule(payload) {
    return http.post("/alerts/rules", payload);
  },
  deleteAlertRule(id) {
    return http.delete(`/alerts/rules/${id}`);
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
  },
  getSystemHealthTrend(start, end) {
    return http.get("/system/health", { params: { start, end } });
  },
  getSystemOps() {
    return http.get("/system/ops");
  }
};
