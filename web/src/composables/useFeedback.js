import { ElNotification } from "element-plus";
import { getApiErrorDetail } from "../services/api";

export function useFeedback() {
  function success(title, message = "") {
    ElNotification({ type: "success", title, message, duration: 2200 });
  }

  function info(title, message = "") {
    ElNotification({ type: "info", title, message, duration: 2200 });
  }

  function warn(title, message = "") {
    ElNotification({ type: "warning", title, message, duration: 2600 });
  }

  function apiError(err, fallback = "操作失败") {
    const d = getApiErrorDetail(err, fallback);
    const msgLower = String(d.message || "").toLowerCase();
    if (msgLower.includes("invalid token") || msgLower.includes("missing bearer token")) {
      return;
    }
    const msg = [d.message, d.code ? `错误码: ${d.code}` : "", d.hint ? `提示: ${d.hint}` : ""]
      .filter(Boolean)
      .join(" | ");
    ElNotification({ type: "error", title: fallback, message: msg, duration: 4200 });
  }

  return { success, info, warn, apiError };
}
