package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"netpulse/internal/db"
	"netpulse/internal/snmp"
)

type addDeviceRequest struct {
	IP              string  `json:"ip"`
	Name            string  `json:"name"`
	TemplateID      *int64  `json:"template_id,omitempty"`
	Brand           string  `json:"brand"`
	Community       string  `json:"community"`
	ReadCommunity   string  `json:"read_community"`
	WriteCommunity  string  `json:"write_community"`
	SNMPVersion     string  `json:"snmp_version"`
	SNMPPort        int     `json:"snmp_port"`
	V3Username      string  `json:"v3_username"`
	V3AuthProtocol  string  `json:"v3_auth_protocol"`
	V3AuthPassword  string  `json:"v3_auth_password"`
	V3PrivProtocol  string  `json:"v3_priv_protocol"`
	V3PrivPassword  string  `json:"v3_priv_password"`
	V3SecurityLevel string  `json:"v3_security_level"`
	DeviceTier      string  `json:"device_tier"`
	PollIntervalSec int     `json:"poll_interval_sec"`
	CPUThreshold    float64 `json:"cpu_threshold"`
	MemThreshold    float64 `json:"mem_threshold"`
	Remark          string  `json:"remark"`
}

func (req *addDeviceRequest) normalizeCommunities() {
	req.ReadCommunity = strings.TrimSpace(req.ReadCommunity)
	req.WriteCommunity = strings.TrimSpace(req.WriteCommunity)
	req.Community = strings.TrimSpace(req.Community)
	if req.ReadCommunity == "" {
		req.ReadCommunity = req.Community
	}
	req.Community = req.ReadCommunity
}

func validateSNMPRequest(req addDeviceRequest) error {
	if req.SNMPVersion != "3" {
		if strings.TrimSpace(req.ReadCommunity) == "" {
			return fmt.Errorf("snmp v1/v2c requires read_community")
		}
		return nil
	}
	if strings.TrimSpace(req.V3Username) == "" {
		return fmt.Errorf("snmp v3 requires v3_username")
	}
	level := strings.TrimSpace(req.V3SecurityLevel)
	if level == "" {
		level = "noAuthNoPriv"
	}
	switch level {
	case "noAuthNoPriv":
		return nil
	case "authNoPriv", "authPriv":
		if strings.TrimSpace(req.V3AuthProtocol) == "" {
			return fmt.Errorf("snmp v3 requires v3_auth_protocol for selected security level")
		}
		if strings.TrimSpace(req.V3AuthPassword) == "" {
			return fmt.Errorf("snmp v3 requires v3_auth_password for selected security level")
		}
		if level == "authPriv" {
			if strings.TrimSpace(req.V3PrivProtocol) == "" {
				return fmt.Errorf("snmp v3 authPriv requires v3_priv_protocol")
			}
			if strings.TrimSpace(req.V3PrivPassword) == "" {
				return fmt.Errorf("snmp v3 authPriv requires v3_priv_password")
			}
		}
		return nil
	default:
		return fmt.Errorf("invalid snmp v3 security level")
	}
}

func pollOptionsFromAddDeviceRequest(req addDeviceRequest) snmp.PollOptions {
	return snmp.PollOptions{
		Brand:       req.Brand,
		SNMPVersion: req.SNMPVersion,
		Port:        req.SNMPPort,
		Community:   req.ReadCommunity,
		V3Username:  req.V3Username,
		V3AuthProto: req.V3AuthProtocol,
		V3AuthPass:  req.V3AuthPassword,
		V3PrivProto: req.V3PrivProtocol,
		V3PrivPass:  req.V3PrivPassword,
		V3SecLevel:  req.V3SecurityLevel,
	}
}

func (h *Handler) handlePrecheckDevice(w http.ResponseWriter, r *http.Request) {
	var req addDeviceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	if req.IP == "" {
		writeError(w, http.StatusBadRequest, "ip is required")
		return
	}
	if req.SNMPVersion == "" {
		req.SNMPVersion = "2c"
	}
	if req.SNMPPort <= 0 {
		req.SNMPPort = 161
	}
	req.normalizeCommunities()
	req.DeviceTier = normalizeDeviceTier(req.DeviceTier)
	if req.PollIntervalSec < 0 {
		req.PollIntervalSec = 0
	}
	if req.PollIntervalSec > 3600 {
		req.PollIntervalSec = 3600
	}
	if req.CPUThreshold < 0 {
		req.CPUThreshold = 0
	}
	if req.CPUThreshold > 100 {
		req.CPUThreshold = 100
	}
	if req.MemThreshold < 0 {
		req.MemThreshold = 0
	}
	if req.MemThreshold > 100 {
		req.MemThreshold = 100
	}
	if err := validateSNMPRequest(req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	opt := pollOptionsFromAddDeviceRequest(req)
	poll, err := h.collector.PollDevice(req.IP, opt)
	if err != nil {
		msg := strings.ToLower(err.Error())
		hint := "请检查SNMP参数"
		switch {
		case strings.Contains(msg, "timeout"):
			hint = "设备响应超时，请检查网络连通、ACL、防火墙或SNMP端口"
		case strings.Contains(msg, "authentication"), strings.Contains(msg, "community"), strings.Contains(msg, "authorization"):
			hint = "认证失败，请核对v3用户名/认证协议/密码或v1/v2c团体字串"
		case strings.Contains(msg, "connect"):
			hint = "连接失败，请检查IP与端口可达性"
		case strings.Contains(msg, "oid"), strings.Contains(msg, "ifname"), strings.Contains(msg, "counter"):
			hint = "设备OID读取异常，请检查型号兼容与SNMP视图权限"
		}
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"code":    "ERR_SNMP_PRECHECK",
			"error":   err.Error(),
			"message": "snmp precheck failed",
			"hint":    hint,
		})
		return
	}
	identity, _ := h.collector.DetectSystemIdentity(req.IP, opt)
	var suggested map[string]any
	if t, score, _ := h.repo.MatchTemplateByFingerprint(r.Context(), req.Brand, identity.SysObjectID, identity.SysDescr); t != nil {
		suggested = map[string]any{
			"id":         t.ID,
			"name":       t.Name,
			"brand":      t.Brand,
			"priority":   t.Priority,
			"matchScore": score,
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"message":            "snmp precheck ok",
		"cpu_usage":          poll.CPUUsage,
		"mem_usage":          poll.MemoryUsage,
		"interfaces":         len(poll.Interfaces),
		"sys_objectid":       identity.SysObjectID,
		"sys_descr":          identity.SysDescr,
		"suggested_template": suggested,
	})
}
func (h *Handler) handleAddDevice(w http.ResponseWriter, r *http.Request) {
	var req addDeviceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	if req.IP == "" || req.Brand == "" {
		writeError(w, http.StatusBadRequest, "ip, brand are required")
		return
	}
	if req.SNMPVersion == "" {
		req.SNMPVersion = "2c"
	}
	if req.Name == "" {
		req.Name = req.IP
	}
	if req.SNMPPort <= 0 {
		req.SNMPPort = 161
	}
	req.normalizeCommunities()
	applyTemplateIfNeeded := func(t *db.DeviceTemplate) {
		if t == nil {
			return
		}
		if strings.TrimSpace(req.Brand) == "" {
			req.Brand = t.Brand
		}
		if strings.TrimSpace(req.SNMPVersion) == "" {
			req.SNMPVersion = t.SNMPVersion
		}
		if req.SNMPPort <= 0 {
			req.SNMPPort = t.SNMPPort
		}
		if strings.TrimSpace(req.ReadCommunity) == "" {
			req.ReadCommunity = t.Community
			req.Community = t.Community
		}
		if strings.TrimSpace(req.V3Username) == "" {
			req.V3Username = t.V3Username
		}
		if strings.TrimSpace(req.V3SecurityLevel) == "" {
			req.V3SecurityLevel = t.V3SecurityLevel
		}
		if strings.TrimSpace(req.V3AuthProtocol) == "" {
			req.V3AuthProtocol = t.V3AuthProtocol
		}
		if strings.TrimSpace(req.V3AuthPassword) == "" {
			req.V3AuthPassword = t.V3AuthPassword
		}
		if strings.TrimSpace(req.V3PrivProtocol) == "" {
			req.V3PrivProtocol = t.V3PrivProtocol
		}
		if strings.TrimSpace(req.V3PrivPassword) == "" {
			req.V3PrivPassword = t.V3PrivPassword
		}
	}
	if req.TemplateID != nil && *req.TemplateID > 0 {
		if t, err := h.repo.GetTemplateByID(r.Context(), *req.TemplateID); err == nil && t != nil {
			applyTemplateIfNeeded(t)
		}
	}
	if err := validateSNMPRequest(req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	opt := pollOptionsFromAddDeviceRequest(req)
	var autoTemplate map[string]any
	if req.TemplateID == nil || *req.TemplateID <= 0 {
		if identity, err := h.collector.DetectSystemIdentity(req.IP, opt); err == nil {
			if t, score, _ := h.repo.MatchTemplateByFingerprint(r.Context(), req.Brand, identity.SysObjectID, identity.SysDescr); t != nil {
				req.TemplateID = &t.ID
				applyTemplateIfNeeded(t)
				opt = pollOptionsFromAddDeviceRequest(req)
				autoTemplate = map[string]any{
					"id":          t.ID,
					"name":        t.Name,
					"brand":       t.Brand,
					"priority":    t.Priority,
					"matchScore":  score,
					"sysObjectID": identity.SysObjectID,
				}
			}
		}
	}

	deviceID, err := h.repo.AddDevice(r.Context(), db.Device{
		IP:              req.IP,
		Name:            req.Name,
		TemplateID:      req.TemplateID,
		Brand:           req.Brand,
		Community:       req.ReadCommunity,
		WriteCommunity:  req.WriteCommunity,
		SNMPVersion:     req.SNMPVersion,
		SNMPPort:        req.SNMPPort,
		V3Username:      req.V3Username,
		V3AuthProto:     req.V3AuthProtocol,
		V3AuthPass:      req.V3AuthPassword,
		V3PrivProto:     req.V3PrivProtocol,
		V3PrivPass:      req.V3PrivPassword,
		V3SecLevel:      req.V3SecurityLevel,
		DeviceTier:      normalizeDeviceTier(req.DeviceTier),
		PollIntervalSec: req.PollIntervalSec,
		CPUThreshold:    req.CPUThreshold,
		MemThreshold:    req.MemThreshold,
		Remark:          req.Remark,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Trigger immediate SNMP interface discovery.
	ifs, err := h.collector.FetchInterfacesWithOptions(req.IP, opt)
	if err == nil {
		list := make([]db.Interface, 0, len(ifs))
		for _, itf := range ifs {
			list = append(list, db.Interface{
				DeviceID: deviceID,
				Index:    itf.IfIndex,
				Name:     itf.IfName,
			})
		}
		_ = h.repo.SyncInterfaces(context.Background(), deviceID, list)
	}
	// Trigger immediate polling once, so status and charts are available without waiting for next worker tick.
	if poll, err := h.collector.PollDevice(req.IP, opt); err == nil {
		metrics := make([]db.InterfaceMetric, 0, len(poll.Interfaces))
		for _, itf := range poll.Interfaces {
			metrics = append(metrics, db.InterfaceMetric{
				IfIndex:        itf.IfIndex,
				IfName:         itf.IfName,
				CPUUsage:       poll.CPUUsage,
				MemoryUsage:    poll.MemoryUsage,
				TrafficInBps:   nil,
				TrafficOutBps:  nil,
				TrafficInStat:  "INITIALIZING",
				TrafficOutStat: "INITIALIZING",
			})
		}
		if len(metrics) > 0 {
			_ = h.repo.SaveMetrics(context.Background(), deviceID, poll.PolledAt, metrics)
		}
		_ = h.repo.AddDeviceLog(context.Background(), deviceID, "INFO", "[OK] 设备添加后首次采集成功")
	} else {
		_ = h.repo.AddDeviceLog(context.Background(), deviceID, "ERROR", fmt.Sprintf("[POLL_FAILED] 设备添加后首次采集失败: %v", err))
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"id":               deviceID,
		"message":          "device created",
		"auto_template":    autoTemplate,
		"template_applied": req.TemplateID,
	})
}
func normalizeDeviceTier(raw string) string {
	v := strings.ToLower(strings.TrimSpace(raw))
	switch v {
	case "core", "核心":
		return "core"
	case "aggregation", "agg", "汇聚":
		return "aggregation"
	case "access", "接入", "":
		return "access"
	default:
		return "access"
	}
}
