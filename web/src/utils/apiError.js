export function normalizeApiError(err, fallback = "请求失败") {
  const data = err?.response?.data || {};
  const message = data.error || data.message || err?.message || fallback;
  return {
    code: data.code || "",
    message,
    hint: data.hint || "",
    status: Number(err?.response?.status || 0),
    isTimeout: String(message).toLowerCase().includes("timeout") || err?.code === "ECONNABORTED",
    isNetwork: !err?.response && Boolean(err?.request),
    isAuthExpired: isAuthExpiredError(err)
  };
}

export function getApiError(err, fallback = "请求失败") {
  return normalizeApiError(err, fallback).message;
}

export function getApiErrorDetail(err, fallback = "请求失败") {
  return normalizeApiError(err, fallback);
}

export function isAuthExpiredError(err) {
  const status = Number(err?.response?.status || 0);
  const data = err?.response?.data || {};
  const msg = String(data.error || data.message || err?.message || "").toLowerCase();
  return status === 401 && (msg.includes("invalid token") || msg.includes("missing bearer token"));
}

export function isForbiddenError(err) {
  const status = Number(err?.response?.status || 0);
  const data = err?.response?.data || {};
  const msg = String(data.error || data.message || err?.message || "").toLowerCase();
  return status === 401 || status === 403 || msg.includes("forbidden") || msg.includes("admin only");
}

export function formatApiErrorMessage(detail) {
  return [detail?.message, detail?.code ? `错误码: ${detail.code}` : "", detail?.hint ? `提示: ${detail.hint}` : ""]
    .filter(Boolean)
    .join(" | ");
}
