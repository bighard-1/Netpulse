import { defineStore } from "pinia";
import { api } from "../services/api";

function severityOf(item) {
  const text = `${item?.level || ""} ${item?.message || ""} ${item?.action || ""}`.toUpperCase();
  if (text.includes("CRITICAL") || text.includes("ERROR") || text.includes("DOWN") || text.includes("OSPF") || text.includes("BGP")) return "critical";
  if (text.includes("WARN") || text.includes("TEMP") || text.includes("INTERFACE_ERROR")) return "warning";
  return "info";
}

export const useOpsStore = defineStore("ops", {
  state: () => ({
    realtimeAlerts: [],
    loadingAlerts: false,
    globalSearchResults: [],
    isDrawerOpen: false,
    activeDeviceId: null
  }),
  actions: {
    async refreshRealtimeAlerts(limit = 20) {
      this.loadingAlerts = true;
      try {
        const res = await api.listRecentEvents(limit);
        const src = res.data?.data || res.data || [];
        const rows = src.slice(0, limit).map((x) => ({
          ...x,
          severity: severityOf(x),
          timestamp: x.created_at || x.timestamp || ""
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
      } finally {
        this.loadingAlerts = false;
      }
    },
    async runGlobalSearch(q) {
      const kw = String(q || "").trim();
      if (!kw) {
        this.globalSearchResults = [];
        return [];
      }
      const res = await api.globalSearch(kw);
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
