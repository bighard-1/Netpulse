package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os/exec"
	"sort"
	"strings"
	"time"

	"netpulse/internal/db"
	"netpulse/internal/snmp"
)

type diagnoseCheck struct {
	Name    string `json:"name"`
	Status  string `json:"status"`
	Message string `json:"message"`
}

type diagnoseReport struct {
	DeviceID      int64           `json:"device_id"`
	DeviceIP      string          `json:"device_ip"`
	Brand         string          `json:"brand"`
	GeneratedAt   time.Time       `json:"generated_at"`
	OverallStatus string          `json:"overall_status"`
	LikelyCause   string          `json:"likely_cause"`
	Checks        []diagnoseCheck `json:"checks"`
}

func (h *Handler) handleDiagnoseDevice(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid device id")
		return
	}
	d, err := h.repo.GetDeviceByID(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if d == nil {
		writeError(w, http.StatusNotFound, "device not found")
		return
	}

	report := diagnoseReport{
		DeviceID:    d.ID,
		DeviceIP:    d.IP,
		Brand:       d.Brand,
		GeneratedAt: time.Now(),
		Checks:      make([]diagnoseCheck, 0, 10),
	}

	add := func(name, status, msg string) {
		report.Checks = append(report.Checks, diagnoseCheck{Name: name, Status: status, Message: msg})
	}

	if d.SNMPVersion == "3" {
		if strings.TrimSpace(d.V3Username) == "" {
			add("SNMP参数完整性", "fail", "SNMPv3 缺少用户名")
		} else {
			add("SNMP参数完整性", "pass", "SNMPv3 参数已填写")
		}
	} else {
		if strings.TrimSpace(d.Community) == "" {
			add("SNMP参数完整性", "fail", "SNMPv1/v2c 缺少 community")
		} else {
			add("SNMP参数完整性", "pass", "community 已填写")
		}
	}

	pingOK := pingHost(d.IP)
	if pingOK {
		add("网络可达性(Ping)", "pass", "设备可 ping 通")
	} else {
		add("网络可达性(Ping)", "warn", "设备 ping 不通（部分设备禁 ping 属正常）")
	}

	port := d.SNMPPort
	if port <= 0 {
		port = 161
	}
	tcpOK := tcpProbe(d.IP, port, 1500*time.Millisecond)
	if tcpOK {
		add("端口连通性(TCP探测)", "pass", fmt.Sprintf("%d 端口可连", port))
	} else {
		add("端口连通性(TCP探测)", "warn", fmt.Sprintf("%d 端口不可连（SNMP为UDP，此项仅辅助）", port))
	}

	opt := snmp.PollOptions{
		Brand:       d.Brand,
		SNMPVersion: d.SNMPVersion,
		Port:        d.SNMPPort,
		Community:   d.Community,
		V3Username:  d.V3Username,
		V3AuthProto: d.V3AuthProto,
		V3AuthPass:  d.V3AuthPass,
		V3PrivProto: d.V3PrivProto,
		V3PrivPass:  d.V3PrivPass,
		V3SecLevel:  d.V3SecLevel,
	}

	pollRes, pollErr := h.collector.PollDevice(d.IP, opt)
	if pollErr != nil {
		add("SNMP即时采集", "fail", pollErr.Error())
	} else {
		add("SNMP即时采集", "pass", fmt.Sprintf("采集成功: CPU %.2f%%, MEM %.2f%%", pollRes.CPUUsage, pollRes.MemoryUsage))
		if len(pollRes.Interfaces) == 0 {
			add("接口采集", "warn", "SNMP可通，但未读取到接口列表")
		} else {
			add("接口采集", "pass", fmt.Sprintf("读取到 %d 个接口", len(pollRes.Interfaces)))
		}
	}

	if d.LastMetricAt == nil {
		add("最近入库时间", "fail", "无指标入库记录")
	} else if time.Since(*d.LastMetricAt) > 5*time.Minute {
		add("最近入库时间", "warn", fmt.Sprintf("最近入库时间较早: %s", d.LastMetricAt.Format(time.RFC3339)))
	} else {
		add("最近入库时间", "pass", fmt.Sprintf("最近入库时间正常: %s", d.LastMetricAt.Format(time.RFC3339)))
	}

	logs, _ := h.repo.GetDeviceLogs(r.Context(), d.ID)
	latest := latestErrors(logs, 5)
	if len(latest) == 0 {
		add("最近错误日志", "pass", "最近无 ERROR 日志")
	} else {
		add("最近错误日志", "warn", strings.Join(latest, " | "))
	}

	report.OverallStatus = "healthy"
	report.LikelyCause = "未发现明显故障"
	for _, c := range report.Checks {
		if c.Status == "fail" {
			report.OverallStatus = "unhealthy"
			break
		}
		if c.Status == "warn" && report.OverallStatus != "unhealthy" {
			report.OverallStatus = "degraded"
		}
	}
	if report.OverallStatus == "unhealthy" {
		report.LikelyCause = "SNMP参数错误、ACL限制、网络不可达、或设备OID不兼容"
	} else if report.OverallStatus == "degraded" {
		report.LikelyCause = "设备可采集但存在链路抖动、日志告警或入库延迟"
	}

	if strings.EqualFold(r.URL.Query().Get("format"), "txt") {
		text := renderReportText(report)
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=netpulse_diagnose_device_%d.txt", d.ID))
		_, _ = w.Write([]byte(text))
		return
	}
	if strings.EqualFold(r.URL.Query().Get("format"), "json") && strings.EqualFold(r.URL.Query().Get("download"), "1") {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=netpulse_diagnose_device_%d.json", d.ID))
		_ = json.NewEncoder(w).Encode(report)
		return
	}

	writeJSON(w, http.StatusOK, report)
}

func latestErrors(logs []db.DeviceLog, limit int) []string {
	if limit <= 0 {
		limit = 5
	}
	out := make([]string, 0, limit)
	for _, l := range logs {
		if strings.EqualFold(l.Level, "ERROR") {
			out = append(out, fmt.Sprintf("%s %s", l.CreatedAt.Format(time.RFC3339), l.Message))
			if len(out) >= limit {
				break
			}
		}
	}
	sort.Strings(out)
	return out
}

func renderReportText(r diagnoseReport) string {
	b := &strings.Builder{}
	fmt.Fprintf(b, "NetPulse 设备自助排查报告\n")
	fmt.Fprintf(b, "设备ID: %d\n设备IP: %s\n品牌: %s\n时间: %s\n", r.DeviceID, r.DeviceIP, r.Brand, r.GeneratedAt.Format(time.RFC3339))
	fmt.Fprintf(b, "总体状态: %s\n可能原因: %s\n\n", r.OverallStatus, r.LikelyCause)
	for i, c := range r.Checks {
		fmt.Fprintf(b, "%d. [%s] %s - %s\n", i+1, strings.ToUpper(c.Status), c.Name, c.Message)
	}
	return b.String()
}

func pingHost(ip string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "ping", "-c", "1", "-W", "1", ip)
	return cmd.Run() == nil
}

func tcpProbe(ip string, port int, timeout time.Duration) bool {
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", ip, port), timeout)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}
