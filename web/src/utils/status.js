export function normalizeStatus(v) {
  const s = String(v || "").toLowerCase();
  if (s === "online" || s === "up" || s === "1") return "online";
  if (s === "offline" || s === "down" || s === "2") return "offline";
  return "unknown";
}

export function deriveStatusState(status, reason = "") {
  const base = normalizeStatus(status);
  const msg = String(reason || "").toLowerCase();
  if (base === "online") return "online";
  if (msg.includes("auth") || msg.includes("community") || msg.includes("authorization")) return "auth_failed";
  if (msg.includes("timeout") || msg.includes("poll") || msg.includes("snmp") || msg.includes("采集")) return "collect_failed";
  if (base === "offline") return "offline";
  return "unknown";
}

export function statusClass(v) {
  const s = typeof v === "object" && v !== null
    ? deriveStatusState(v.status, v.status_reason)
    : normalizeStatus(v);
  if (s === "online") return "status-dot-online";
  if (s === "offline") return "status-dot-offline";
  if (s === "auth_failed" || s === "collect_failed") return "status-dot-unknown";
  return "status-dot-unknown";
}

export function statusLabel(v, reason = "") {
  const s = typeof v === "object" && v !== null
    ? deriveStatusState(v.status, v.status_reason)
    : deriveStatusState(v, reason);
  if (s === "online") return "在线";
  if (s === "auth_failed") return "认证失败";
  if (s === "collect_failed") return "采集中断";
  if (s === "offline") return "离线";
  return "未知";
}
