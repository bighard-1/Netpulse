package snmp

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"log"
	"math"
	"net"
	"net/http"
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
}

type Worker struct {
	repo          *db.Repository
	collector     *Collector
	interval      time.Duration
	parallel      int
	deviceTimeout time.Duration
	alertWebhook  string
	cpuThreshold  float64
	memThreshold  float64

	mu   sync.Mutex
	last map[string]counterState
	ifs  map[int64]string
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
		parallel:      p,
		deviceTimeout: time.Duration(timeoutSec) * time.Second,
		alertWebhook:  os.Getenv("ALERT_WEBHOOK_URL"),
		cpuThreshold:  cpuTh,
		memThreshold:  memTh,
		last:          make(map[string]counterState),
		ifs:           make(map[int64]string),
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
	if cfg.SNMPDeviceTimeoutSec >= 2 {
		w.deviceTimeout = time.Duration(cfg.SNMPDeviceTimeoutSec) * time.Second
	}
	if cfg.AlertCPUThreshold > 0 {
		w.cpuThreshold = cfg.AlertCPUThreshold
	}
	if cfg.AlertMemThreshold > 0 {
		w.memThreshold = cfg.AlertMemThreshold
	}
	w.alertWebhook = cfg.AlertWebhookURL
}

func (w *Worker) nextInterval(ctx context.Context) time.Duration {
	w.applyRuntimeSettings(ctx)
	if w.interval < 5*time.Second {
		return 5 * time.Second
	}
	if w.interval > time.Hour {
		return time.Hour
	}
	return w.interval
}

func (w *Worker) runOnce(ctx context.Context) {
	devices, err := w.repo.ListDevices(ctx)
	if err != nil {
		log.Printf("snmp worker list devices failed: %v", err)
		return
	}

	sem := make(chan struct{}, w.parallel)
	var wg sync.WaitGroup
	for _, d := range devices {
		device := d
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			w.pollOne(ctx, device)
		}()
	}
	wg.Wait()
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
		_ = w.repo.AddDeviceLog(ctx, d.ID, "ERROR", fmt.Sprintf("[%s] %s", code, reason))
		w.sendAlert("error", d, code, reason)
		return
	}

	interfaces := make([]db.Interface, 0, len(result.Interfaces))
	for _, itf := range result.Interfaces {
		interfaces = append(interfaces, db.Interface{DeviceID: d.ID, Index: itf.IfIndex, Name: itf.IfName})
	}
	if digest := interfaceDigest(interfaces); !w.sameInterfaceDigest(d.ID, digest) {
		if err := w.repo.SyncInterfaces(ctx, d.ID, interfaces); err != nil {
			log.Printf("sync interfaces failed device=%d: %v", d.ID, err)
			_ = w.repo.AddDeviceLog(ctx, d.ID, "ERROR", fmt.Sprintf("[SYNC_FAILED] 端口同步失败: %v", err))
			return
		}
		w.setInterfaceDigest(d.ID, digest)
	}

	mList := make([]db.InterfaceMetric, 0, len(result.Interfaces))
	for _, itf := range result.Interfaces {
		inBps, outBps := w.calcBps(d.ID, itf.IfIndex, itf.InOctets, itf.OutOctets, result.PolledAt)
		mList = append(mList, db.InterfaceMetric{
			IfIndex:       itf.IfIndex,
			IfName:        itf.IfName,
			CPUUsage:      result.CPUUsage,
			MemoryUsage:   result.MemoryUsage,
			TrafficInBps:  inBps,
			TrafficOutBps: outBps,
		})
	}

	if err := w.repo.SaveMetrics(ctx, d.ID, result.PolledAt, mList); err != nil {
		log.Printf("save metrics failed device=%d: %v", d.ID, err)
		_ = w.repo.AddDeviceLog(ctx, d.ID, "ERROR", fmt.Sprintf("[DB_WRITE_FAILED] 指标入库失败: %v", err))
		return
	}
	_ = w.repo.AddDeviceLog(ctx, d.ID, "INFO", "[OK] 设备轮询成功")
	if result.CPUUsage >= w.cpuThreshold {
		w.sendAlert("warning", d, "CPU_HIGH", fmt.Sprintf("CPU利用率 %.2f%% 超过阈值 %.2f%%", result.CPUUsage, w.cpuThreshold))
	}
	if result.MemoryUsage >= w.memThreshold {
		w.sendAlert("warning", d, "MEM_HIGH", fmt.Sprintf("内存利用率 %.2f%% 超过阈值 %.2f%%", result.MemoryUsage, w.memThreshold))
	}
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

func (w *Worker) sendAlert(level string, d db.Device, code, msg string) {
	if strings.TrimSpace(w.alertWebhook) == "" {
		return
	}
	body := fmt.Sprintf(`{"level":"%s","code":"%s","device_id":%d,"ip":"%s","brand":"%s","message":"%s","ts":"%s"}`,
		level, code, d.ID, d.IP, d.Brand, strings.ReplaceAll(msg, `"`, `'`), time.Now().Format(time.RFC3339))
	req, _ := http.NewRequest(http.MethodPost, w.alertWebhook, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err == nil && resp != nil {
		_ = resp.Body.Close()
	}
}

func (w *Worker) calcBps(deviceID int64, ifIndex int, inOctets, outOctets uint64, now time.Time) (int64, int64) {
	key := interfaceKey(deviceID, ifIndex)

	w.mu.Lock()
	defer w.mu.Unlock()

	prev, ok := w.last[key]
	w.last[key] = counterState{inOctets: inOctets, outOctets: outOctets, at: now}
	if !ok {
		return 0, 0
	}

	seconds := now.Sub(prev.at).Seconds()
	if seconds <= 0 {
		return 0, 0
	}

	inDelta := safeDelta(inOctets, prev.inOctets)
	outDelta := safeDelta(outOctets, prev.outOctets)
	inBps := safeBps(inDelta, seconds)
	outBps := safeBps(outDelta, seconds)
	return inBps, outBps
}

func interfaceKey(deviceID int64, ifIndex int) string { return fmt.Sprintf("%d:%d", deviceID, ifIndex) }

func safeDelta(curr, prev uint64) uint64 {
	if curr < prev {
		return 0
	}
	return curr - prev
}

func safeBps(deltaOctets uint64, seconds float64) int64 {
	if seconds <= 0 {
		return 0
	}
	v := (float64(deltaOctets) * 8) / seconds
	if math.IsNaN(v) || math.IsInf(v, 0) || v < 0 {
		return 0
	}
	const maxReasonableBps = float64(9_000_000_000_000_000)
	if v > maxReasonableBps {
		return 0
	}
	return int64(v)
}
