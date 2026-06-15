import { defineStore } from "pinia";
import { api } from "../services/api";

function severityOf(item) {
  const text = `${item?.level || ""} ${item?.message || ""} ${item?.action || ""}`.toUpperCase();
  if (text.includes("CRITICAL") || text.includes("ERROR") || text.includes("DOWN") || text.includes("OSPF") || text.includes("BGP")) return "critical";
  if (text.includes("WARN") || text.includes("TEMP") || text.includes("INTERFACE_ERROR")) return "warning";
  return "info";
}

function parseInterfaceId(message) {
  const text = String(message || "");
  const patterns = [
    /interface[_\s-]*id[:=\s]+(\d+)/i,
    /port[_\s-]*id[:=\s]+(\d+)/i,
    /ifindex[:=\s]+(\d+)/i
  ];
  for (const p of patterns) {
    const m = text.match(p);
    if (m && m[1]) return Number(m[1]);
  }
  return null;
}

export const useOpsStore = defineStore("ops", {
  state: () => ({
    realtimeAlerts: [],
    loadingAlerts: false,
    lastRealtimeError: "",
    lastRealtimeLoadedAt: "",
    globalSearchResults: [],
    isDrawerOpen: false,
    activeDeviceId: null,
    searchReqSeq: 0
  }),
  actions: {
    async refreshRealtimeAlerts(limit = 20, params = {}) {
      this.loadingAlerts = true;
      try {
        const res = await api.listRecentEvents(limit, params);
        const src = res.data?.data || res.data || [];
        const rows = src.slice(0, limit).map((x) => ({
          ...x,
          severity: severityOf(x),
          timestamp: x.created_at || x.timestamp || "",
          interface_id: x.interface_id || x.port_id || parseInterfaceId(x.message),
          level: x.level || x.severity || ""
        }));
        // Basic alert-state machine: dedupe repeated same event within short window.
        const seen = new Map();
        const deduped = [];
        for (const row of rows) {
          const key = `${row.device_id || ""}|${row.level || ""}|${row.message || ""}`;
          const ts = new Date(row.timestamp || 0).getTime() || 0;
          const prev = seen.get(key) || 0;
          if (ts - prev < 120000) continue;
          seen.set(key, ts);
          deduped.push(row);
        }
        this.realtimeAlerts = deduped;
        this.lastRealtimeError = "";
        this.lastRealtimeLoadedAt = new Date().toISOString();
      } catch (err) {
        this.lastRealtimeError = err?.response?.data?.message || err?.response?.data?.error || err?.message || "事件流加载失败";
        // Keep the previous event list visible during transient API/database slowness.
        throw err;
      } finally {
        this.loadingAlerts = false;
      }
    },
    async runGlobalSearch(q, ctx = {}) {
      const kw = String(q || "").trim();
      if (!kw) {
        this.globalSearchResults = [];
        return [];
      }
      this.searchReqSeq += 1;
      const reqId = this.searchReqSeq;
      const params = {};
      if (Number(ctx.deviceId) > 0) params.device_id = Number(ctx.deviceId);
      const res = await api.globalSearch(kw, { params });
      if (reqId !== this.searchReqSeq) return this.globalSearchResults;
      this.globalSearchResults = res.data || [];
      return this.globalSearchResults;
    },
    openQuickPeek(deviceId) {
      this.activeDeviceId = Number(deviceId) || null;
      this.isDrawerOpen = true;
    },
    closeQuickPeek() {
      this.isDrawerOpen = false;
      this.activeDeviceId = null;
    }
  }
});
