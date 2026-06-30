package api

import (
	"encoding/json"
	"net/http"
)

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	code := "ERR_GENERIC"
	hint := "如需排查，请导出自助诊断报告并提供给运维"
	switch status {
	case http.StatusBadRequest:
		code = "ERR_BAD_REQUEST"
		hint = "请检查请求参数格式和值是否正确"
	case http.StatusUnauthorized:
		code = "ERR_UNAUTHORIZED"
		hint = "请重新登录后再试"
	case http.StatusForbidden:
		code = "ERR_FORBIDDEN"
		hint = "当前账号权限不足，请联系管理员授权"
	case http.StatusNotFound:
		code = "ERR_NOT_FOUND"
		hint = "目标资源不存在或已被删除"
	case http.StatusConflict:
		code = "ERR_CONFLICT"
		hint = "当前状态暂不能执行该操作，请稍后重试"
	case http.StatusTooManyRequests:
		code = "ERR_RATE_LIMIT"
		hint = "请求过于频繁，请稍后再试"
	case http.StatusGatewayTimeout:
		code = "ERR_TIMEOUT"
		hint = "任务耗时较长，建议使用后台任务方式执行"
	case http.StatusInternalServerError:
		code = "ERR_INTERNAL"
	}
	writeJSON(w, status, map[string]string{
		"code":    code,
		"error":   msg,
		"message": msg,
		"hint":    hint,
	})
}

func writeErrorDetail(w http.ResponseWriter, status int, code, msg, hint string) {
	if code == "" || msg == "" || hint == "" {
		writeError(w, status, msg)
		return
	}
	writeJSON(w, status, map[string]string{
		"code":    code,
		"error":   msg,
		"message": msg,
		"hint":    hint,
	})
}
