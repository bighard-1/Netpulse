import axios from "axios";
import { setServerTimezone } from "../utils/serverTime";
import { getApiError as parseApiError, getApiErrorDetail as parseApiErrorDetail, isAuthExpiredError } from "../utils/apiError";

const API_BASE =
  import.meta.env.VITE_API_BASE_URL || "http://119.40.55.18:18080/api";

const http = axios.create({
  baseURL: API_BASE,
  timeout: 20000
});
const LONG_RUNNING_TIMEOUT_MS = 10 * 60 * 1000;
const SLOW_LOG_KEY = "np_slow_api_logs";

function appendSlowApiLog(item) {
  try {
    const prev = JSON.parse(localStorage.getItem(SLOW_LOG_KEY) || "[]");
    const next = [item, ...prev].slice(0, 120);
    localStorage.setItem(SLOW_LOG_KEY, JSON.stringify(next));
    if (typeof window !== "undefined") {
      window.dispatchEvent(new CustomEvent("np-slow-api-log", { detail: item }));
    }
  } catch {}
}

function historyDiagnosticFromResponse(resp) {
  const headers = resp?.headers || {};
  const fromHeader = (key) => headers[key] || headers[key.toLowerCase()] || "";
  const data = resp?.data || {};
  const duration = Number(fromHeader("X-NetPulse-Query-Duration-Ms") || data.query_duration_ms || 0);
  const pointCount = Number(fromHeader("X-NetPulse-History-Point-Count") || data.point_count || 0);
  const cacheRaw = fromHeader("X-NetPulse-History-Cache-Hit");
  const cacheHit = cacheRaw === "" ? data.cache_hit : String(cacheRaw).toLowerCase() === "true";
  return {
    query_ms: Number.isFinite(duration) && duration > 0 ? duration : undefined,
    history_range: fromHeader("X-NetPulse-History-Range") || data.range_label || "",
    history_source: fromHeader("X-NetPulse-History-Source") || data.source_table || "",
    history_interval: fromHeader("X-NetPulse-History-Interval") || data.sampled_interval || "",
    history_cache_hit: Boolean(cacheHit),
    history_points: Number.isFinite(pointCount) && pointCount > 0 ? pointCount : undefined
  };
}

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
  config.metadata = { start: Date.now() };
  const token = normalizeToken(localStorage.getItem("netpulse_token"));
  if (token) config.headers.Authorization = `Bearer ${token}`;
  return config;
});

http.interceptors.response.use(
  (resp) => {
    const tz = resp?.headers?.["x-server-timezone"] || resp?.headers?.["X-Server-Timezone"];
    if (tz) setServerTimezone(tz);
    const cost = Date.now() - (resp?.config?.metadata?.start || Date.now());
    if (cost > 1200) {
      // Lightweight client-side perf telemetry for slow API calls.
      console.warn(`[netpulse][slow-api] ${resp?.config?.method?.toUpperCase()} ${resp?.config?.url} ${cost}ms`);
      appendSlowApiLog({
        ts: new Date().toISOString(),
        method: resp?.config?.method?.toUpperCase() || "GET",
        url: resp?.config?.url || "",
        ms: cost,
        ok: true,
        ...historyDiagnosticFromResponse(resp)
      });
    }
    return resp;
  },
  (err) => {
    const cost = Date.now() - (err?.config?.metadata?.start || Date.now());
    if (cost > 1200) {
      console.warn(`[netpulse][slow-api] ${err?.config?.method?.toUpperCase()} ${err?.config?.url} ${cost}ms (error)`);
      appendSlowApiLog({
        ts: new Date().toISOString(),
        method: err?.config?.method?.toUpperCase() || "GET",
        url: err?.config?.url || "",
        ms: cost,
        ok: false,
        ...historyDiagnosticFromResponse(err?.response)
      });
    }
    if (isAuthExpiredError(err)) {
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
  return parseApiError(err, fallback);
}

export function getApiErrorDetail(err, fallback = "请求失败") {
  return parseApiErrorDetail(err, fallback);
}

export const api = {
  login(username, password) {
    return http.post("/auth/login", { username, password });
  },
  listDevices() {
    return http.get("/devices");
  },
  globalSearch(q, options = {}) {
    const params = { q, ...(options?.params || {}) };
    return http.get("/search", { ...options, params });
  },
  getTopology() {
    return http.get("/topology");
  },
  addTopologyNode(payload) {
    return http.post("/topology/nodes", payload);
  },
  updateTopologyNode(id, payload) {
    return http.put(`/topology/nodes/${id}`, payload);
  },
  deleteTopologyNode(id) {
    return http.delete(`/topology/nodes/${id}`);
  },
  addTopologyEdge(payload) {
    return http.post("/topology/edges", payload);
  },
  updateTopologyEdge(id, payload) {
    return http.put(`/topology/edges/${id}`, payload);
  },
  deleteTopologyEdge(id) {
    return http.delete(`/topology/edges/${id}`);
  },
  async getDeviceById(id) {
    const res = await http.get(`/devices/${id}`);
    return res.data || null;
  },
  addDevice(payload) {
    return http.post("/devices", payload);
  },
  deleteDevice(id) {
    return http.delete(`/devices/${id}`, { timeout: LONG_RUNNING_TIMEOUT_MS });
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
  getInterfaceById(id) {
    return http.get(`/interfaces/${id}`);
  },
  getInterface(id) {
    return http.get(`/interfaces/${id}`);
  },
  updateInterfaceProfile(id, payload) {
    return http.put(`/interfaces/${id}`, payload);
  },
  getHistory(type, id, start, end, interval = "", maxPoints = 0) {
    const params = { type, id, start, end, interval };
    if (Number(maxPoints) > 0) {
      params.max_points = Number(maxPoints);
    }
    return http.get("/metrics/history", {
      params,
      timeout: 60000
    });
  },
  precheckDevice(payload) {
    return http.post("/devices/precheck", payload);
  },
  getDeviceCapabilities(id) {
    return http.get(`/devices/${id}/capabilities`);
  },
  getDeviceLogs(id, params = {}) {
    return http.get(`/devices/${id}/logs`, { params });
  },
  diagnoseDevice(id) {
    return http.get(`/devices/${id}/diagnose`);
  },
  diagnoseTrafficBias(id) {
    return http.get(`/devices/${id}/diagnose/traffic-bias`);
  },
  exportDiagnosis(id, format = "txt") {
    return http.get(`/devices/${id}/diagnose`, {
      params: { format, download: 1 },
      responseType: "blob"
    });
  },
  exportTrafficBiasDiagnosis(id, format = "txt") {
    return http.get(`/devices/${id}/diagnose/traffic-bias`, {
      params: { format, download: 1 },
      responseType: "blob"
    });
  },
  downloadBackup() {
    return http.get("/system/backup", {
      responseType: "blob",
      timeout: LONG_RUNNING_TIMEOUT_MS
    });
  },
  startBackupJob() {
    return http.post("/system/backup/jobs", {});
  },
  getSystemJob(id) {
    return http.get(`/system/jobs/${id}`);
  },
  listSystemJobs(limit = 30) {
    return http.get("/system/jobs", { params: { limit } });
  },
  downloadBackupJob(id) {
    return http.get(`/system/backup/jobs/${id}/download`, {
      responseType: "blob",
      timeout: LONG_RUNNING_TIMEOUT_MS
    });
  },
  restoreFromFile(file) {
    const form = new FormData();
    form.append("file", file);
    return http.post("/system/restore", form, {
      headers: { "Content-Type": "multipart/form-data" },
      timeout: LONG_RUNNING_TIMEOUT_MS
    });
  },
  startRestoreJob(file) {
    const form = new FormData();
    form.append("file", file);
    return http.post("/system/restore/jobs", form, {
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
  listRecentEvents(limit = 20, params = {}) {
    return http.get("/events/recent", { params: { ...params, limit } });
  },
  listAlertEvents(limit = 200, status = "") {
    return http.get("/alerts/events", { params: { limit, status } });
  },
  diagnoseAssetLoad() {
    return http.get("/diagnostics/asset-load", { timeout: LONG_RUNNING_TIMEOUT_MS });
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
    return http.post("/system/backup/drill", {}, { timeout: LONG_RUNNING_TIMEOUT_MS });
  },
  startBackupDrillJob() {
    return http.post("/system/backup/drill/jobs", {});
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
  getSystemHealthTrend(limit = 30) {
    return http.get("/system/health", { params: { limit } });
  },
  getSystemOps() {
    return http.get("/system/ops", { timeout: LONG_RUNNING_TIMEOUT_MS });
  },
  runSystemCleanup(payload) {
    return http.post("/system/maintenance/cleanup", payload, { timeout: LONG_RUNNING_TIMEOUT_MS });
  },
  downloadInspectionBundle() {
    return http.get("/system/inspection-bundle", {
      responseType: "blob",
      timeout: LONG_RUNNING_TIMEOUT_MS
    });
  }
};
