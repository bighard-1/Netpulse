export function formatBps(value, precision = 2) {
  const n = Number(value || 0);
  if (!Number.isFinite(n)) return "0 bps";
  const sign = n < 0 ? "-" : "";
  let abs = Math.abs(n);
  const units = ["bps", "Kbps", "Mbps", "Gbps", "Tbps"];
  let idx = 0;
  while (abs >= 1024 && idx < units.length - 1) {
    abs /= 1024;
    idx += 1;
  }
  if (idx === 0) return `${sign}${Math.round(abs)} ${units[idx]}`;
  return `${sign}${abs.toFixed(precision)} ${units[idx]}`;
}

