const STORAGE_KEY = "np_server_timezone";
const DEFAULT_TZ = "Asia/Shanghai";

let serverTimezone = (() => {
  try {
    return localStorage.getItem(STORAGE_KEY) || DEFAULT_TZ;
  } catch {
    return DEFAULT_TZ;
  }
})();

function tzParts(date, timeZone) {
  const d = date instanceof Date ? date : new Date(date);
  const fmt = new Intl.DateTimeFormat("en-CA", {
    timeZone,
    hour12: false,
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit"
  });
  const parts = fmt.formatToParts(d);
  const out = {};
  for (const p of parts) {
    if (p.type !== "literal") out[p.type] = p.value;
  }
  return {
    year: Number(out.year),
    month: Number(out.month),
    day: Number(out.day),
    hour: Number(out.hour),
    minute: Number(out.minute),
    second: Number(out.second)
  };
}

function tzOffsetMinutes(date, timeZone) {
  const p = tzParts(date, timeZone);
  const asUTC = Date.UTC(p.year, p.month - 1, p.day, p.hour, p.minute, p.second);
  return Math.round((asUTC - date.getTime()) / 60000);
}

function dateFromTZFields(timeZone, year, month, day, hour = 0, minute = 0, second = 0) {
  const guessUTC = Date.UTC(year, month - 1, day, hour, minute, second, 0);
  const guessDate = new Date(guessUTC);
  const offsetMin = tzOffsetMinutes(guessDate, timeZone);
  return new Date(guessUTC - offsetMin * 60000);
}

export function datePickerValueToServerDate(value) {
  const d = value instanceof Date ? value : new Date(value);
  if (!Number.isFinite(d.getTime())) return null;
  // Element Plus date pickers expose the fields the user selected in the
  // browser's local timezone. Rebuild those fields in the server timezone so
  // "00:00" means server-side midnight instead of the operator's local offset.
  return dateFromTZFields(
    getServerTimezone(),
    d.getFullYear(),
    d.getMonth() + 1,
    d.getDate(),
    d.getHours(),
    d.getMinutes(),
    d.getSeconds()
  );
}

export function setServerTimezone(tz) {
  const clean = String(tz || "").trim();
  if (!clean) return;
  try {
    new Intl.DateTimeFormat("en-US", { timeZone: clean }).format(new Date());
  } catch {
    return;
  }
  serverTimezone = clean;
  try {
    localStorage.setItem(STORAGE_KEY, clean);
  } catch {}
}

export function getServerTimezone() {
  return serverTimezone || DEFAULT_TZ;
}

export function formatServerTime(value, options = {}) {
  const d = value instanceof Date ? value : new Date(value);
  if (!Number.isFinite(d.getTime())) return "-";
  return new Intl.DateTimeFormat("zh-CN", {
    timeZone: getServerTimezone(),
    hour12: false,
    ...options
  }).format(d);
}

export function startOfServerDay(d = new Date()) {
  const p = tzParts(d, getServerTimezone());
  return dateFromTZFields(getServerTimezone(), p.year, p.month, p.day, 0, 0, 0);
}

export function startOfServerMonth(d = new Date()) {
  const p = tzParts(d, getServerTimezone());
  return dateFromTZFields(getServerTimezone(), p.year, p.month, 1, 0, 0, 0);
}

export function startOfServerWeek(d = new Date()) {
  const p = tzParts(d, getServerTimezone());
  const local = new Date(Date.UTC(p.year, p.month - 1, p.day, 0, 0, 0));
  const day = local.getUTCDay();
  const delta = day === 0 ? 6 : day - 1;
  local.setUTCDate(local.getUTCDate() - delta);
  return dateFromTZFields(
    getServerTimezone(),
    local.getUTCFullYear(),
    local.getUTCMonth() + 1,
    local.getUTCDate(),
    0, 0, 0
  );
}

export function startOfServerYear(d = new Date()) {
  const p = tzParts(d, getServerTimezone());
  return dateFromTZFields(getServerTimezone(), p.year, 1, 1, 0, 0, 0);
}
