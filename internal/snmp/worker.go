package snmp

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"netpulse/internal/db"
)

type counterState struct {
	inOctets  uint64
	outOctets uint64
	at        time.Time
	inBps     int64
	outBps    int64
	inZeroStreak  int
	outZeroStreak int
}

type Worker struct {
	repo          *db.Repository
	collector     *Collector
	interval      time.Duration
	pollCore      time.Duration
	pollAgg       time.Duration
	pollAccess    time.Duration
	parallel      int
	deviceTimeout time.Duration
	alertWebhook  string
	cpuThreshold  float64
	memThreshold  float64
	alertMgr      *AlertManager
	calibration   map[string]float64

	mu     sync.Mutex
	last   map[string]counterState
	ifs    map[int64]string
	devUp  map[int64]bool
	portUp map[string]bool
	evts   map[string]time.Time

	lastHealthSnapshot time.Time
	lastPolled         map[int64]time.Time
}

type alertPolicy struct {
	cpuThreshold     float64
	memThreshold     float64
	trafficThreshold int64
	muted            bool
	webhook          string
}

func NewWorker(repo *db.Repository, collector *Collector, interval time.Duration) *Worker {
	p := runtime.NumCPU()
	if p < 2 {
		p = 2
	}
	if p > 16 {
		p = 16
	}
	timeoutSec := 15
	if s := os.Getenv("SNMP_DEVICE_TIMEOUT_SEC"); s != "" {
		if v, err := strconv.Atoi(s); err == nil && v > 0 {
			timeoutSec = v
		}
	}
	cpuTh := 90.0
	if s := os.Getenv("ALERT_CPU_THRESHOLD"); s != "" {
		if v, err := strconv.ParseFloat(s, 64); err == nil {
			cpuTh = v
		}
	}
	memTh := 90.0
	if s := os.Getenv("ALERT_MEM_THRESHOLD"); s != "" {
		if v, err := strconv.ParseFloat(s, 64); err == nil {
			memTh = v
		}
	}
	return &Worker{
		repo:          repo,
		collector:     collector,
		interval:      interval,
		pollCore:      interval,
		pollAgg:       interval,
		pollAccess:    interval,
		parallel:      p,
		deviceTimeout: time.Duration(timeoutSec) * time.Second,
		alertWebhook:  os.Getenv("ALERT_WEBHOOK_URL"),
		cpuThreshold:  cpuTh,
		memThreshold:  memTh,
		last:          make(map[string]counterState),
		ifs:           make(map[int64]string),
		devUp:         make(map[int64]bool),
		portUp:        make(map[string]bool),
		evts:          make(map[string]time.Time),
		alertMgr:      NewAlertManager(repo, os.Getenv("ALERT_WEBHOOK_URL")),
		calibration:   loadCalibrationMap(os.Getenv("SNMP_CALIBRATION_MAP")),
		lastPolled:    make(map[int64]time.Time),
	}
}

func (w *Worker) Start(ctx context.Context) {
	w.runOnce(ctx)
	for {
		wait := w.nextInterval(ctx)
		select {
		case <-ctx.Done():
			log.Printf("snmp worker stopped: %v", ctx.Err())
			return
		case <-time.After(wait):
			w.runOnce(ctx)
		}
	}
}

func (w *Worker) applyRuntimeSettings(ctx context.Context) {
	cfg, err := w.repo.GetRuntimeSettings(ctx)
	if err != nil {
		return
	}
	if cfg.SNMPPollIntervalSec >= 5 {
		w.interval = time.Duration(cfg.SNMPPollIntervalSec) * time.Second
	}
	if cfg.PollIntervalCoreSec >= 5 {
		w.pollCore = time.Duration(cfg.PollIntervalCoreSec) * time.Second
	}
	if cfg.PollIntervalAggSec >= 5 {
		w.pollAgg = time.Duration(cfg.PollIntervalAggSec) * time.Second
	}
	if cfg.PollIntervalAccessSec >= 5 {
		w.pollAccess = time.Duration(cfg.PollIntervalAccessSec) * time.Second
	}
	if cfg.SNMPDeviceTimeoutSec >= 2 {
		w.deviceTimeout = time.Duration(cfg.SNMPDeviceTimeoutSec) * time.Second
	}
	if cfg.AlertCPUThreshold > 0 {
		w.cpuThreshold = cfg.AlertCPUThreshold
	}
	if cfg.AlertMemThreshold > 0 {
		w.memThreshold = cfg.AlertMemThreshold
	}
	if strings.TrimSpace(cfg.SNMPCalibrationMap) != "" {
		w.calibration = loadCalibrationMap(cfg.SNMPCalibrationMap)
	}
	w.alertWebhook = cfg.AlertWebhookURL
	if w.alertMgr != nil {
		w.alertMgr.SetWebhook(cfg.AlertWebhookURL)
	}
}

func (w *Worker) nextInterval(ctx context.Context) time.Duration {
	w.applyRuntimeSettings(ctx)
	base := minDuration(w.pollCore, minDuration(w.pollAgg, w.pollAccess))
	if base < 5*time.Second {
		base = 5 * time.Second
	}
	if base > time.Hour {
		return time.Hour
	}
	return base
}

func (w *Worker) runOnce(ctx context.Context) {
	devices, err := w.repo.ListDevices(ctx)
	if err != nil {
		log.Printf("snmp worker list devices failed: %v", err)
		return
	}
	if w.alertMgr != nil {
		w.alertMgr.RefreshTopology(ctx)
	}

	sem := make(chan struct{}, w.parallel)
	var wg sync.WaitGroup
	now := time.Now()
	for _, d := range devices {
		if !w.shouldPoll(d, now) {
			continue
		}
		device := d
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			w.pollOne(ctx, device)
			w.markPolled(device.ID, now)
		}()
	}
	wg.Wait()
	w.persistSystemHealth(ctx, devices)
}

func minDuration(a, b time.Duration) time.Duration {
	if a <= 0 {
		return b
	}
	if b <= 0 || a < b {
		return a
	}
	return b
}

func (w *Worker) markPolled(deviceID int64, at time.Time) {
	w.mu.Lock()
	w.lastPolled[deviceID] = at
	w.mu.Unlock()
}

func (w *Worker) shouldPoll(d db.Device, now time.Time) bool {
	interval := w.pollIntervalForDevice(d)
	if interval < 5*time.Second {
		interval = 5 * time.Second
	}
	w.mu.Lock()
	last, ok := w.lastPolled[d.ID]
	w.mu.Unlock()
	if !ok {
		return true
	}
	return now.Sub(last) >= interval
}

func (w *Worker) pollIntervalForDevice(d db.Device) time.Duration {
	if d.PollIntervalSec > 0 {
		return time.Duration(d.PollIntervalSec) * time.Second
	}
	text := strings.ToLower(strings.TrimSpace(d.Name + " " + d.Remark))
	switch {
	case strings.Contains(text, "核心") || strings.Contains(text, "core"):
		return w.pollCore
	case strings.Contains(text, "汇聚") || strings.Contains(text, "aggregation") || strings.Contains(text, "agg"):
		return w.pollAgg
	default:
		return w.pollAccess
	}
}

func (w *Worker) pollOne(ctx context.Context, d db.Device) {
	deviceCtx, cancel := context.WithTimeout(ctx, w.deviceTimeout)
	defer cancel()
	opt := PollOptions{
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

	result, err := w.collector.PollDevice(d.IP, opt)
	if err != nil {
		w.trackDeviceState(ctx, d, false)
		log.Printf("snmp poll failed device=%d ip=%s: %v", d.ID, d.IP, err)
		code, reason := classifyPollError(err)
		pingOK, tcpOK := probeReachability(deviceCtx, d.IP, d.SNMPPort)
		if pingOK && !tcpOK {
			code = "TCP161_BLOCKED"
			reason = "设备可达但 SNMP 端口不可达，请检查 ACL/防火墙/端口映射"
		} else if !pingOK {
			code = "HOST_UNREACHABLE"
			reason = "设备网络不可达，请检查路由/VLAN/网关配置"
		}
		w.addDeviceEvent(ctx, d.ID, "ERROR", fmt.Sprintf("[%s] %s", code, reason), 2*time.Minute)
		w.emitAlert(ctx, "error", d, code, reason, "")
		return
	}
	w.trackDeviceState(ctx, d, true)
	policy := w.resolveAlertPolicy(ctx, d)
	factor := w.calibrationFactor(d)
	result.CPUUsage = calibratePercentForDevice(d, result.CPUUsage, factor)
	result.MemoryUsage = calibratePercentForDevice(d, result.MemoryUsage, factor)

	interfaces := make([]db.Interface, 0, len(result.Interfaces))
	for _, itf := range result.Interfaces {
		interfaces = append(interfaces, db.Interface{DeviceID: d.ID, Index: itf.IfIndex, Name: itf.IfName})
	}
	if digest := interfaceDigest(interfaces); !w.sameInterfaceDigest(d.ID, digest) {
		if err := w.repo.SyncInterfaces(ctx, d.ID, interfaces); err != nil {
			log.Printf("sync interfaces failed device=%d: %v", d.ID, err)
			w.addDeviceEvent(ctx, d.ID, "ERROR", fmt.Sprintf("[SYNC_FAILED] 端口同步失败: %v", err), 2*time.Minute)
			return
		}
		w.setInterfaceDigest(d.ID, digest)
	}

	mList := make([]db.InterfaceMetric, 0, len(result.Interfaces))
	for _, itf := range result.Interfaces {
		inBps, outBps := w.calcBps(d.ID, itf.IfIndex, itf.InOctets, itf.OutOctets, result.PolledAt, itf.SpeedMbps, w.pollIntervalForDevice(d))
		w.trackPortState(ctx, d, itf.IfIndex, itf.IfName, itf.OperUp)
		mList = append(mList, db.InterfaceMetric{
			IfIndex:       itf.IfIndex,
			IfName:        itf.IfName,
			CPUUsage:      result.CPUUsage,
			MemoryUsage:   result.MemoryUsage,
			StorageUsage:  result.StorageUsage,
			StorageTotal:  result.StorageTotal,
			StorageFree:   result.StorageFree,
			UptimeSec:     result.UptimeSec,
			SpeedMbps:     itf.SpeedMbps,
			OperStatus:    boolToOperStatus(itf.OperUp),
			TrafficInBps:  inBps,
			TrafficOutBps: outBps,
		})
	}

	if err := w.repo.SaveMetrics(ctx, d.ID, result.PolledAt, mList); err != nil {
		log.Printf("save metrics failed device=%d: %v", d.ID, err)
		w.addDeviceEvent(ctx, d.ID, "ERROR", fmt.Sprintf("[DB_WRITE_FAILED] 指标入库失败: %v", err), 2*time.Minute)
		return
	}
	_ = w.repo.UpsertDeviceCapability(ctx, db.DeviceCapability{
		DeviceID:          d.ID,
		SNMPVersion:       d.SNMPVersion,
		SupportsCPU:       result.CPUUsage >= 0,
		SupportsMemory:    result.MemoryUsage >= 0,
		SupportsIfTraffic: len(result.Interfaces) > 0,
		InterfaceCount:    len(result.Interfaces),
	})
	w.addDeviceEvent(ctx, d.ID, "INFO", "[OK] 设备轮询成功", 5*time.Minute)
	if policy.muted {
		w.addDeviceEvent(ctx, d.ID, "INFO", "[ALERT_MUTED] 当前处于告警静默窗口", 10*time.Minute)
		return
	}
	cpuTh := policy.cpuThreshold
	if d.CPUThreshold > 0 {
		cpuTh = d.CPUThreshold
	}
	if cpuTh <= 0 {
		cpuTh = w.cpuThreshold
	}
	memTh := policy.memThreshold
	if d.MemThreshold > 0 {
		memTh = d.MemThreshold
	}
	if memTh <= 0 {
		memTh = w.memThreshold
	}
	if result.CPUUsage >= cpuTh {
		w.emitAlert(ctx, "warning", d, "CPU_HIGH", fmt.Sprintf("CPU利用率 %.2f%% 超过阈值 %.2f%%", result.CPUUsage, cpuTh), policy.webhook)
	}
	if result.MemoryUsage >= memTh {
		w.emitAlert(ctx, "warning", d, "MEM_HIGH", fmt.Sprintf("内存利用率 %.2f%% 超过阈值 %.2f%%", result.MemoryUsage, memTh), policy.webhook)
	}
	if policy.trafficThreshold > 0 {
		var peak int64
		for _, m := range mList {
			v := m.TrafficInBps + m.TrafficOutBps
			if v > peak {
				peak = v
			}
		}
		if peak >= policy.trafficThreshold {
			w.emitAlert(ctx, "warning", d, "TRAFFIC_HIGH", fmt.Sprintf("端口峰值流量 %d bps 超过阈值 %d bps", peak, policy.trafficThreshold), policy.webhook)
		}
	}
}

func loadCalibrationMap(raw string) map[string]float64 {
	out := map[string]float64{}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return out
	}
	_ = json.Unmarshal([]byte(raw), &out)
	for k, v := range out {
		if v <= 0 || math.IsNaN(v) || math.IsInf(v, 0) {
			out[k] = 1.0
		}
		out[strings.TrimSpace(k)] = out[k]
	}
	return out
}

func boolToOperStatus(v bool) int {
	if v {
		return 1
	}
	return 2
}

func (w *Worker) calibrationFactor(d db.Device) float64 {
	if w == nil || len(w.calibration) == 0 {
		return 1.0
	}
	if v, ok := w.calibration[d.IP]; ok && v > 0 {
		return v
	}
	return 1.0
}

func calibratePercentForDevice(d db.Device, raw, factor float64) float64 {
	v := raw
	if strings.Contains(strings.ToLower(strings.TrimSpace(d.Brand)), "h3c") {
		// Some H3C models expose percentage as scaled integer (e.g. 4567 => 45.67%).
		if v > 100 && v <= 10000 {
			v = v / 100
		}
	}
	if factor <= 0 || math.IsNaN(factor) || math.IsInf(factor, 0) {
		factor = 1.0
	}
	v = v * factor
	if v < 0 {
		v = 0
	}
	if v > 100 {
		v = 100
	}
	return math.Round(v*100) / 100
}

func (w *Worker) trackDeviceState(ctx context.Context, d db.Device, up bool) {
	w.mu.Lock()
	prev, ok := w.devUp[d.ID]
	w.devUp[d.ID] = up
	w.mu.Unlock()
	if !ok {
		return
	}
	if prev != up {
		if up {
			w.addDeviceEvent(ctx, d.ID, "INFO", "[DEVICE_UP] 设备状态由离线变为在线", time.Minute)
		} else {
			w.addDeviceEvent(ctx, d.ID, "WARNING", "[DEVICE_DOWN] 设备状态由在线变为离线", time.Minute)
		}
	}
}

func (w *Worker) trackPortState(ctx context.Context, d db.Device, ifIndex int, ifName string, up bool) {
	key := interfaceKey(d.ID, ifIndex) + ":oper"
	w.mu.Lock()
	prev, ok := w.portUp[key]
	w.portUp[key] = up
	w.mu.Unlock()
	if !ok {
		return
	}
	if prev != up {
		if up {
			w.addDeviceEvent(ctx, d.ID, "INFO", fmt.Sprintf("[PORT_UP] 端口 %s(ifIndex=%d) 状态由down变为up", ifName, ifIndex), time.Minute)
		} else {
			w.addDeviceEvent(ctx, d.ID, "WARNING", fmt.Sprintf("[PORT_DOWN] 端口 %s(ifIndex=%d) 状态由up变为down", ifName, ifIndex), time.Minute)
		}
	}
}

func (w *Worker) addDeviceEvent(ctx context.Context, deviceID int64, level, msg string, suppressWindow time.Duration) {
	if suppressWindow > 0 {
		key := fmt.Sprintf("%d|%s|%s", deviceID, level, msg)
		now := time.Now()
		w.mu.Lock()
		if last, ok := w.evts[key]; ok && now.Sub(last) < suppressWindow {
			w.mu.Unlock()
			return
		}
		w.evts[key] = now
		w.mu.Unlock()
	}
	_ = w.repo.AddDeviceLog(ctx, deviceID, level, msg)
}

func (w *Worker) sameInterfaceDigest(deviceID int64, digest string) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.ifs[deviceID] == digest
}

func (w *Worker) setInterfaceDigest(deviceID int64, digest string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.ifs[deviceID] = digest
}

func interfaceDigest(interfaces []db.Interface) string {
	h := sha1.New()
	for _, itf := range interfaces {
		_, _ = h.Write([]byte(fmt.Sprintf("%d|%s\n", itf.Index, itf.Name)))
	}
	return hex.EncodeToString(h.Sum(nil))
}

func classifyPollError(err error) (string, string) {
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "connect"):
		return "CONNECT_FAILED", "设备连接失败，请检查 IP/端口可达性"
	case strings.Contains(msg, "timeout"):
		return "TIMEOUT", "SNMP 请求超时，请检查 ACL、防火墙或链路质量"
	case strings.Contains(msg, "authentication") || strings.Contains(msg, "community") || strings.Contains(msg, "authorization"):
		return "AUTH_FAILED", "SNMP 认证失败，请检查 community 或 v3 认证参数"
	case strings.Contains(msg, "counter") || strings.Contains(msg, "ifname"):
		return "OID_UNSUPPORTED", "设备 OID 读取失败，可能型号不兼容"
	default:
		return "POLL_FAILED", fmt.Sprintf("采集失败: %v", err)
	}
}

func probeReachability(ctx context.Context, ip string, port int) (bool, bool) {
	if port <= 0 {
		port = 161
	}
	pingOK := pingHost(ctx, ip)
	tcpOK := tcpProbe(ip, port, 1200*time.Millisecond)
	return pingOK, tcpOK
}

func pingHost(ctx context.Context, ip string) bool {
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

func (w *Worker) emitAlert(ctx context.Context, level string, d db.Device, code, msg, webhookOverride string) {
	if w.alertMgr == nil {
		return
	}
	devState := w.snapshotDeviceStates()
	suppressed, related := w.alertMgr.ShouldSuppress(d, devState)
	_ = w.repo.SaveAlertEvent(ctx, nil, d.ID, level, code, msg)
	if suppressed {
		_ = w.repo.AddDeviceLog(ctx, d.ID, "INFO", fmt.Sprintf("[ALERT_SUPPRESSED] %s (%s)", msg, related))
		_ = w.repo.SaveAlertEvent(ctx, nil, d.ID, "info", "ALERT_SUPPRESSED", fmt.Sprintf("%s (%s)", msg, related))
	}
	w.alertMgr.NotifyWithWebhook(webhookOverride, Alert{
		Level:      level,
		Code:       code,
		DeviceID:   d.ID,
		DeviceIP:   d.IP,
		DeviceName: d.Name,
		Brand:      d.Brand,
		Message:    msg,
		Suppressed: suppressed,
		RelatedTo:  related,
		TS:         time.Now(),
	})
}

func (w *Worker) resolveAlertPolicy(ctx context.Context, d db.Device) alertPolicy {
	out := alertPolicy{
		cpuThreshold: w.cpuThreshold,
		memThreshold: w.memThreshold,
	}
	rules, err := w.repo.ListAlertRules(ctx)
	if err != nil || len(rules) == 0 {
		return out
	}
	for _, r := range rules {
		if !r.Enabled {
			continue
		}
		if !(r.Scope == "global" || (r.DeviceID != nil && *r.DeviceID == d.ID)) {
			continue
		}
		if inMuteWindow(r.MuteStart, r.MuteEnd, time.Now()) {
			out.muted = true
		}
		if r.CPUThreshold != nil && *r.CPUThreshold > 0 {
			out.cpuThreshold = *r.CPUThreshold
		}
		if r.MemThreshold != nil && *r.MemThreshold > 0 {
			out.memThreshold = *r.MemThreshold
		}
		if r.TrafficThreshold != nil && *r.TrafficThreshold > 0 {
			out.trafficThreshold = *r.TrafficThreshold
		}
		if strings.TrimSpace(r.NotifyWebhook) != "" {
			out.webhook = strings.TrimSpace(r.NotifyWebhook)
		}
	}
	if out.webhook == "" {
		out.webhook = w.alertWebhook
	}
	return out
}

func inMuteWindow(start, end string, now time.Time) bool {
	start = strings.TrimSpace(start)
	end = strings.TrimSpace(end)
	if start == "" || end == "" {
		return false
	}
	parse := func(v string) (int, bool) {
		parts := strings.Split(v, ":")
		if len(parts) != 2 {
			return 0, false
		}
		h, err1 := strconv.Atoi(parts[0])
		m, err2 := strconv.Atoi(parts[1])
		if err1 != nil || err2 != nil || h < 0 || h > 23 || m < 0 || m > 59 {
			return 0, false
		}
		return h*60 + m, true
	}
	s, ok1 := parse(start)
	e, ok2 := parse(end)
	if !ok1 || !ok2 {
		return false
	}
	n := now.Hour()*60 + now.Minute()
	if s == e {
		return true
	}
	if s < e {
		return n >= s && n < e
	}
	return n >= s || n < e
}

func (w *Worker) snapshotDeviceStates() map[int64]bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	out := make(map[int64]bool, len(w.devUp))
	for k, v := range w.devUp {
		out[k] = v
	}
	return out
}

func (w *Worker) persistSystemHealth(ctx context.Context, devices []db.Device) {
	now := time.Now()
	if !w.lastHealthSnapshot.IsZero() && now.Sub(w.lastHealthSnapshot) < 15*time.Minute {
		return
	}
	if len(devices) == 0 {
		return
	}
	states := w.snapshotDeviceStates()
	total := len(devices)
	online := 0
	for _, d := range devices {
		if up, ok := states[d.ID]; ok && up {
			online++
		}
	}
	availability := (float64(online) / float64(total)) * 100

	alerts, _ := w.repo.ListAuditLogs(ctx, 500)
	activeAlerts := 0
	for _, a := range alerts {
		txt := strings.ToUpper(a.Action + " " + a.Target)
		if strings.Contains(txt, "ERROR") || strings.Contains(txt, "CRITICAL") || strings.Contains(txt, "DOWN") {
			activeAlerts++
		}
	}
	penalty := math.Min(35, float64(activeAlerts)*1.5)
	score := math.Max(0, math.Min(100, availability-penalty))
	if err := w.repo.SaveSystemHealthSnapshot(ctx, now, score, activeAlerts, availability); err == nil {
		w.lastHealthSnapshot = now
	}
}

func (w *Worker) calcBps(deviceID int64, ifIndex int, inOctets, outOctets uint64, now time.Time, speedMbps int, expectedInterval time.Duration) (int64, int64) {
	key := interfaceKey(deviceID, ifIndex)

	w.mu.Lock()
	defer w.mu.Unlock()

	prev, ok := w.last[key]
	if !ok {
		w.last[key] = counterState{
			inOctets: inOctets, outOctets: outOctets, at: now,
			inBps: 0, outBps: 0, inZeroStreak: 0, outZeroStreak: 0,
		}
		return 0, 0
	}

	seconds := now.Sub(prev.at).Seconds()
	if seconds <= 0 {
		return prev.inBps, prev.outBps
	}

	// Guard against scheduling jitter and long gap spikes:
	// keep the previous stable value when deltaTs is far from expected.
	if expectedInterval > 0 {
		exp := expectedInterval.Seconds()
		if seconds < exp*0.4 || seconds > exp*3.5 {
			w.last[key] = counterState{
				inOctets: inOctets, outOctets: outOctets, at: now,
				inBps: prev.inBps, outBps: prev.outBps,
				inZeroStreak: prev.inZeroStreak, outZeroStreak: prev.outZeroStreak,
			}
			return prev.inBps, prev.outBps
		}
	}

	inDelta, inDis := safeDeltaWithDiscontinuity(inOctets, prev.inOctets)
	outDelta, outDis := safeDeltaWithDiscontinuity(outOctets, prev.outOctets)
	if inDis || outDis {
		// ifCounter discontinuity/reset detected: keep previous rate to avoid false plunge.
		w.last[key] = counterState{
			inOctets: inOctets, outOctets: outOctets, at: now,
			inBps: prev.inBps, outBps: prev.outBps,
			inZeroStreak: prev.inZeroStreak, outZeroStreak: prev.outZeroStreak,
		}
		return prev.inBps, prev.outBps
	}

	maxBps := maxReasonableBpsBySpeed(speedMbps)
	rawIn := rawBps(inDelta, seconds)
	rawOut := rawBps(outDelta, seconds)
	inBps := clampOrKeepPrev(rawIn, prev.inBps, maxBps)
	outBps := clampOrKeepPrev(rawOut, prev.outBps, maxBps)
	// lightweight EWMA smoothing, reduces aliasing when polling interval changes.
	inBps, inZeroStreak := smoothRate(prev.inBps, inBps, prev.inZeroStreak)
	outBps, outZeroStreak := smoothRate(prev.outBps, outBps, prev.outZeroStreak)
	w.last[key] = counterState{
		inOctets: inOctets, outOctets: outOctets, at: now,
		inBps: inBps, outBps: outBps,
		inZeroStreak: inZeroStreak, outZeroStreak: outZeroStreak,
	}
	return inBps, outBps
}

func interfaceKey(deviceID int64, ifIndex int) string { return fmt.Sprintf("%d:%d", deviceID, ifIndex) }

func safeDeltaWithDiscontinuity(curr, prev uint64) (uint64, bool) {
	if curr < prev {
		return 0, true
	}
	return curr - prev, false
}

func rawBps(deltaOctets uint64, seconds float64) int64 {
	if seconds <= 0 {
		return 0
	}
	v := (float64(deltaOctets) * 8) / seconds
	if math.IsNaN(v) || math.IsInf(v, 0) || v < 0 {
		return 0
	}
	return int64(v)
}

func clampOrKeepPrev(curr, prev int64, maxReasonableBps float64) int64 {
	if curr <= 0 {
		return 0
	}
	if maxReasonableBps <= 0 {
		return curr
	}
	if float64(curr) > maxReasonableBps {
		// Keep last stable value instead of dropping to zero.
		// This avoids periodic fake troughs on high-speed ports.
		if prev > 0 {
			return prev
		}
		return int64(maxReasonableBps)
	}
	return curr
}

func maxReasonableBpsBySpeed(speedMbps int) float64 {
	// S12700E and high-throughput chassis may report bursty counters.
	// Keep larger headroom before treating as anomaly.
	if speedMbps > 0 {
		return float64(speedMbps) * 1_000_000 * 2.0
	}
	// Unknown speed: fallback to 100G with headroom.
	return 120_000_000_000
}

func smoothRate(prev, curr int64, zeroStreak int) (int64, int) {
	if prev <= 0 {
		if curr <= 0 {
			return 0, 0
		}
		return curr, 0
	}
	if curr <= 0 {
		// Avoid one-off false plunge while still allowing real fall-to-zero.
		zeroStreak++
		if zeroStreak <= 2 {
			return prev, zeroStreak
		}
		decay := int64(float64(prev) * 0.5)
		if decay < 1 {
			return 0, zeroStreak
		}
		return decay, zeroStreak
	}
	// EWMA alpha=0.35
	v := 0.65*float64(prev) + 0.35*float64(curr)
	if v < 0 {
		return 0, 0
	}
	return int64(v), 0
}
