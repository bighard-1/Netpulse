import { formatServerTime, startOfServerDay } from "./serverTime.js";

export const MAX_TRAFFIC_HISTORY_MS = 730 * 24 * 3600 * 1000;

export function pickTrafficUnit(maxVal) {
  if (maxVal >= 1e9) return { unit: "Gbps", div: 1e9 };
  if (maxVal >= 1e6) return { unit: "Mbps", div: 1e6 };
  if (maxVal >= 1e3) return { unit: "Kbps", div: 1e3 };
  return { unit: "bps", div: 1 };
}

export function roundUpNice(v) {
  if (!Number.isFinite(v) || v <= 0) return 10;
  const exp = Math.floor(Math.log10(v));
  const base = 10 ** exp;
  const n = v / base;
  let step = 10;
  if (n <= 1) step = 1;
  else if (n <= 2) step = 2;
  else if (n <= 5) step = 5;
  return step * base;
}

export function toTrafficSeriesData(data) {
  const inbound = [];
  const outbound = [];
  for (const p of data || []) {
    const t = new Date(p.timestamp).getTime();
    if (!Number.isFinite(t)) continue;
    const inV = p.traffic_in_bps == null ? null : Number(p.traffic_in_bps);
    const outV = p.traffic_out_bps == null ? null : Number(p.traffic_out_bps);
    inbound.push([t, Number.isFinite(inV) ? inV : null]);
    outbound.push([t, Number.isFinite(outV) ? outV : null]);
  }
  return { inbound, outbound };
}

export function fillShortNullGaps(points, maxGap = 1) {
  const arr = (points || []).slice();
  if (!arr.length) return arr;
  let i = 0;
  while (i < arr.length) {
    if (arr[i][1] != null) {
      i += 1;
      continue;
    }
    const gapStart = i;
    while (i < arr.length && arr[i][1] == null) i += 1;
    const gapEnd = i - 1;
    const gapLen = gapEnd - gapStart + 1;
    const prev = gapStart - 1 >= 0 ? arr[gapStart - 1][1] : null;
    const next = i < arr.length ? arr[i][1] : null;
    if (gapLen <= maxGap && prev != null && next != null) {
      for (let k = gapStart; k <= gapEnd; k++) {
        const ratio = (k - gapStart + 1) / (gapLen + 1);
        arr[k][1] = Number(prev) + (Number(next) - Number(prev)) * ratio;
      }
    }
  }
  return arr;
}

export function compactValidPoints(points) {
  return (points || []).filter((x) => x?.[1] != null && Number.isFinite(Number(x[1])));
}

export function decimatePoints(points, maxPoints = 2200) {
  const arr = points || [];
  if (arr.length <= maxPoints) return arr;
  const stride = Math.max(1, Math.ceil(arr.length / maxPoints));
  const out = [];
  for (let i = 0; i < arr.length; i += stride) {
    out.push(arr[i]);
  }
  if (arr.length > 0 && out[out.length - 1] !== arr[arr.length - 1]) {
    out.push(arr[arr.length - 1]);
  }
  return out;
}

function medianOf(values) {
  const arr = values.filter((v) => Number.isFinite(v)).sort((a, b) => a - b);
  if (!arr.length) return null;
  const mid = Math.floor(arr.length / 2);
  return arr.length % 2 ? arr[mid] : (arr[mid - 1] + arr[mid]) / 2;
}

export function stabilizeTrafficPoints(points, enabled) {
  const arr = points || [];
  if (!enabled || arr.length < 5) return arr;
  const values = arr.map((x) => x[1]);
  const medianed = values.map((v, i) => {
    if (v == null || !Number.isFinite(Number(v))) return null;
    const window = [];
    for (let k = Math.max(0, i - 2); k <= Math.min(values.length - 1, i + 2); k += 1) {
      if (values[k] != null) window.push(Number(values[k]));
    }
    return medianOf(window);
  });

  let previous = null;
  return arr.map(([ts, value], i) => {
    if (value == null || !Number.isFinite(Number(value))) {
      previous = null;
      return [ts, null];
    }
    const baseline = Number(medianed[i] ?? value);
    const smoothed = previous == null ? baseline : previous * 0.62 + baseline * 0.38;
    previous = smoothed;
    return [ts, smoothed];
  });
}

export function isHighSpeedCacheSensitive(speedMbps, pollSec) {
  return Number(speedMbps || 0) >= 1000 && Math.max(5, Number(pollSec || 60)) <= 120;
}

export function calcTrafficFetchPlan(start, end, speedMbps, pollSec) {
  const spanMs = end.getTime() - start.getTime();
  let interval = "";
  const highSpeedStable = isHighSpeedCacheSensitive(speedMbps, pollSec);
  if (spanMs > 180 * 24 * 3600 * 1000) interval = "6h";
  else if (spanMs > 30 * 24 * 3600 * 1000) interval = "2h";
  else if (spanMs > 7 * 24 * 3600 * 1000) interval = "1h";
  else if (spanMs > 24 * 3600 * 1000) interval = "15m";
  else if (highSpeedStable) interval = "2m";
  const agg = highSpeedStable && interval ? "高速端口稳健聚合" : (interval ? "time_bucket聚合" : "原始采样点");
  return { interval, agg };
}

export function xAxisLabelFormatter(value, metaKey) {
  const dt = new Date(value);
  const hhmm = formatServerTime(dt, { hour: "2-digit", minute: "2-digit" });
  const dayText = String(formatServerTime(dt, { day: "2-digit" }));
  const day = Number(dayText.replace(/\D/g, ""));
  const md = formatServerTime(dt, { month: "2-digit", day: "2-digit" });
  if (metaKey === "today") return hhmm;
  if (metaKey === "d7") return hhmm === "00:00" ? md : hhmm;
  if (metaKey === "d30") {
    return day === 1 || day % 5 === 0 ? `${day}日` : "";
  }
  return hhmm === "00:00" ? md : hhmm;
}

export function detectIntervalSwitchPoints(rawData) {
  const out = [];
  const arr = (rawData || [])
    .map((x) => ({ ts: new Date(x.timestamp).getTime() }))
    .filter((x) => Number.isFinite(x.ts))
    .sort((a, b) => a.ts - b.ts);
  if (arr.length < 4) return out;
  let prevGap = 0;
  for (let i = 1; i < arr.length; i++) {
    const gapSec = Math.round((arr[i].ts - arr[i - 1].ts) / 1000);
    if (gapSec <= 0) continue;
    if (prevGap > 0) {
      const ratio = gapSec / prevGap;
      if (ratio >= 1.8 || ratio <= 0.56) {
        out.push({
          ts: arr[i].ts,
          label: `${prevGap}s→${gapSec}s`
        });
      }
    }
    prevGap = gapSec;
  }
  return out.slice(0, 12);
}

export function getPresetTrafficRange(key) {
  const now = new Date();
  if (key === "today") return [startOfServerDay(now), now];
  if (key === "d7") return [startOfServerDay(new Date(now.getTime() - 6 * 24 * 3600 * 1000)), now];
  if (key === "d30") return [startOfServerDay(new Date(now.getTime() - 29 * 24 * 3600 * 1000)), now];
  return null;
}
