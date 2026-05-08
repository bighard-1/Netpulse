export function normalizeStatus(v) {
  const s = String(v || "").toLowerCase();
  if (s === "online" || s === "up" || s === "1") return "online";
  if (s === "offline" || s === "down" || s === "2") return "offline";
  return "unknown";
}

export function statusClass(v) {
  const s = normalizeStatus(v);
  if (s === "online") return "status-dot-online";
  if (s === "offline") return "status-dot-offline";
  return "status-dot-unknown";
}

export function statusLabel(v) {
  const s = normalizeStatus(v);
  if (s === "online") return "在线";
  if (s === "offline") return "离线";
  return "未知";
}

