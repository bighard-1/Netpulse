import { normalizeStatus } from "./status.js";

export function filterDashboardDevices(devices, keyword, statusFilter) {
  const kw = String(keyword || "").trim().toLowerCase();
  let list = Array.isArray(devices) ? devices : [];
  if (statusFilter && statusFilter !== "all") {
    list = list.filter((d) => normalizeStatus(d.status) === statusFilter);
  }
  if (!kw) return list;
  return list.filter((d) => {
    const ports = (d.interfaces || [])
      .map((p) => `${p.name || ""} ${p.alias || ""} ${p.custom_name || ""} ${p.remark || ""} ${p.index || ""}`)
      .join(" ");
    return [d.ip, d.name, d.brand, d.remark, d.location, d.site, ports, d.status]
      .join(" ")
      .toLowerCase()
      .includes(kw);
  });
}

export function findDashboardPorts(devices, keyword, limit = 80) {
  const kw = String(keyword || "").trim().toLowerCase();
  if (!kw) return [];
  const out = [];
  for (const d of devices || []) {
    for (const p of d.interfaces || []) {
      const text = [
        p.name || "",
        p.raw_name || "",
        p.remark || "",
        p.index || "",
        p.id || "",
        d.name || "",
        d.ip || "",
        d.brand || ""
      ].join(" ").toLowerCase();
      if (!text.includes(kw)) continue;
      out.push({
        portId: p.id,
        portName: p.name || `ifIndex-${p.index}`,
        portIndex: p.index,
        deviceId: d.id,
        deviceName: d.name || d.ip,
        deviceIP: d.ip,
        remark: p.remark || "",
        speedMbps: Number(p.speed_mbps || 0)
      });
      if (out.length >= limit) return out;
    }
  }
  return out;
}

export function rankPortsBySpeed(devices, min, max, limit = 10) {
  const points = [];
  for (const d of devices || []) {
    for (const p of d.interfaces || []) {
      const speed = Number(p.speed_mbps || 0);
      if (speed < min) continue;
      if (max > 0 && speed >= max) continue;
      const inBps = Number(p.traffic_in_bps || 0);
      const outBps = Number(p.traffic_out_bps || 0);
      const heat = Math.max(inBps, outBps);
      if (heat <= 0) continue;
      points.push({
        deviceName: d.name || d.ip,
        deviceId: d.id,
        deviceIp: d.ip,
        interfaceName: p.name || `ifIndex-${p.index}`,
        interfaceIndex: Number(p.index || 0),
        interfaceId: p.id,
        speedMbps: speed,
        inBps,
        outBps,
        bps: inBps + outBps
      });
    }
  }
  points.sort((a, b) => b.bps - a.bps);
  return points.slice(0, limit);
}
