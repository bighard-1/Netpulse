package snmp

import (
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/gosnmp/gosnmp"
)

const (
	OIDCPUUsage       = ".1.3.6.1.4.1.2011.5.25.31.1.1.1.1.5"
	OIDMemoryUsage    = ".1.3.6.1.4.1.2011.5.25.31.1.1.1.1.7"
	OIDCPUUsageH3C    = ".1.3.6.1.4.1.25506.2.6.1.1.1.1.6"
	OIDMemoryUsageH3C = ".1.3.6.1.4.1.25506.2.6.1.1.1.1.8"
	OIDCPUAvgH3C      = ".1.3.6.1.4.1.25506.2.6.1.1.1.1.26"
	OIDMemAvgH3C      = ".1.3.6.1.4.1.25506.2.6.1.1.1.1.27"
	OIDCPU1MinH3C     = ".1.3.6.1.4.1.25506.2.6.1.1.1.1.33"
	OIDIfName         = ".1.3.6.1.2.1.31.1.1.1.1"
	OIDIfHCInOctets   = ".1.3.6.1.2.1.31.1.1.1.6"
	OIDIfHCOutOctets  = ".1.3.6.1.2.1.31.1.1.1.11"
	OIDIfInOctets     = ".1.3.6.1.2.1.2.2.1.10"
	OIDIfOutOctets    = ".1.3.6.1.2.1.2.2.1.16"
	OIDIfOperStatus   = ".1.3.6.1.2.1.2.2.1.8"
	OIDIfSpeed        = ".1.3.6.1.2.1.2.2.1.5"
	OIDIfHighSpeed    = ".1.3.6.1.2.1.31.1.1.1.15"
	OIDHrStorageType  = ".1.3.6.1.2.1.25.2.3.1.2"
	OIDHrStorageAlloc = ".1.3.6.1.2.1.25.2.3.1.4"
	OIDHrStorageSize  = ".1.3.6.1.2.1.25.2.3.1.5"
	OIDHrStorageUsed  = ".1.3.6.1.2.1.25.2.3.1.6"
	OIDSysDescr       = ".1.3.6.1.2.1.1.1.0"
	OIDSysObjectID    = ".1.3.6.1.2.1.1.2.0"
)

type InterfaceInfo struct {
	IfIndex int
	IfName  string
}

type InterfaceCounters struct {
	IfIndex         int
	IfName          string
	InOctets        uint64
	OutOctets       uint64
	HCInOctets      uint64
	HCOutOctets     uint64
	LegacyInOctets  uint64
	LegacyOutOctets uint64
	OperUp          bool
	SpeedMbps       int
}

type PollResult struct {
	CPUUsage     float64
	MemoryUsage  float64
	StorageUsage float64
	StorageTotal float64
	StorageFree  float64
	UptimeSec    int64
	Interfaces   []InterfaceCounters
	PolledAt     time.Time
}

type CounterCompareItem struct {
	IfIndex         int     `json:"if_index"`
	IfName          string  `json:"if_name"`
	HCInOctets      uint64  `json:"hc_in_octets"`
	LegacyInOctets  uint64  `json:"legacy_in_octets"`
	HCOutOctets     uint64  `json:"hc_out_octets"`
	LegacyOutOctets uint64  `json:"legacy_out_octets"`
	InRatio         float64 `json:"in_ratio"`
	OutRatio        float64 `json:"out_ratio"`
}

type CounterCompareResult struct {
	PolledAt   time.Time            `json:"polled_at"`
	Interfaces []CounterCompareItem `json:"interfaces"`
}

type SystemIdentity struct {
	SysObjectID string
	SysDescr    string
}

type OIDProfile struct {
	CPUOIDs    []string
	MemoryOIDs []string
}

type Collector struct {
	Port      uint16
	Version   gosnmp.SnmpVersion
	Timeout   time.Duration
	Retries   int
	MaxOids   int
	MaxRep    uint32
	LocalBind string
}

type PollOptions struct {
	Brand       string
	SNMPVersion string
	Port        int
	Community   string
	V3Username  string
	V3AuthProto string
	V3AuthPass  string
	V3PrivProto string
	V3PrivPass  string
	V3SecLevel  string
}

func NewCollector() *Collector {
	return &Collector{
		Port:    161,
		Version: gosnmp.Version2c,
		Timeout: 5 * time.Second,
		Retries: 1,
		MaxOids: gosnmp.MaxOids,
		MaxRep:  20,
	}
}

func selectOIDProfile(brand string) OIDProfile {
	b := strings.ToLower(strings.TrimSpace(brand))
	switch b {
	case "h3c":
		return OIDProfile{
			CPUOIDs:    []string{OIDCPU1MinH3C, OIDCPUAvgH3C, OIDCPUUsageH3C, OIDCPUUsage},
			MemoryOIDs: []string{OIDMemAvgH3C, OIDMemoryUsageH3C, OIDMemoryUsage},
		}
	case "huawei":
		return OIDProfile{
			CPUOIDs:    []string{OIDCPUUsage, OIDCPUUsageH3C},
			MemoryOIDs: []string{OIDMemoryUsage, OIDMemoryUsageH3C},
		}
	default:
		return OIDProfile{
			CPUOIDs:    []string{OIDCPUUsage, OIDCPUUsageH3C},
			MemoryOIDs: []string{OIDMemoryUsage, OIDMemoryUsageH3C},
		}
	}
}

func (c *Collector) newClient(ip string, opt PollOptions) *gosnmp.GoSNMP {
	client := &gosnmp.GoSNMP{
		Target:         ip,
		Port:           c.Port,
		Version:        c.Version,
		Timeout:        c.Timeout,
		Retries:        c.Retries,
		MaxOids:        c.MaxOids,
		MaxRepetitions: c.MaxRep,
	}
	if opt.Port > 0 {
		client.Port = uint16(opt.Port)
	}
	switch strings.ToLower(strings.TrimSpace(opt.SNMPVersion)) {
	case "1", "v1":
		client.Version = gosnmp.Version1
		client.Community = opt.Community
	case "3", "v3":
		client.Version = gosnmp.Version3
		client.SecurityModel = gosnmp.UserSecurityModel
		client.MsgFlags = v3MsgFlags(opt.V3SecLevel)
		client.SecurityParameters = &gosnmp.UsmSecurityParameters{
			UserName:                 opt.V3Username,
			AuthenticationProtocol:   v3AuthProtocol(opt.V3AuthProto),
			AuthenticationPassphrase: opt.V3AuthPass,
			PrivacyProtocol:          v3PrivProtocol(opt.V3PrivProto),
			PrivacyPassphrase:        opt.V3PrivPass,
		}
	default:
		client.Version = gosnmp.Version2c
		client.Community = opt.Community
	}
	return client
}

func (c *Collector) FetchInterfaces(ip, community string) ([]InterfaceInfo, error) {
	return c.FetchInterfacesWithOptions(ip, PollOptions{SNMPVersion: "2c", Community: community})
}

func (c *Collector) FetchInterfacesWithOptions(ip string, opt PollOptions) ([]InterfaceInfo, error) {
	client := c.newClient(ip, opt)
	if err := client.Connect(); err != nil {
		return nil, fmt.Errorf("snmp connect: %w", err)
	}
	defer client.Conn.Close()

	pdus, err := client.BulkWalkAll(OIDIfName)
	if err != nil {
		return nil, fmt.Errorf("walk ifName: %w", err)
	}

	out := make([]InterfaceInfo, 0, len(pdus))
	for _, pdu := range pdus {
		ifIndex, err := oidIndex(pdu.Name)
		if err != nil {
			continue
		}
		out = append(out, InterfaceInfo{
			IfIndex: ifIndex,
			IfName:  pduToString(pdu),
		})
	}
	return out, nil
}

func (c *Collector) DetectSystemIdentity(ip string, opt PollOptions) (SystemIdentity, error) {
	client := c.newClient(ip, opt)
	if err := client.Connect(); err != nil {
		return SystemIdentity{}, fmt.Errorf("snmp connect: %w", err)
	}
	defer client.Conn.Close()
	resp, err := client.Get([]string{OIDSysObjectID, OIDSysDescr})
	if err != nil {
		return SystemIdentity{}, fmt.Errorf("get system identity: %w", err)
	}
	out := SystemIdentity{}
	for _, v := range resp.Variables {
		switch v.Name {
		case OIDSysObjectID:
			out.SysObjectID = pduToString(v)
		case OIDSysDescr:
			out.SysDescr = pduToString(v)
		}
	}
	return out, nil
}

func (c *Collector) CompareInterfaceCounters(ip string, opt PollOptions, limit int) (CounterCompareResult, error) {
	client := c.newClient(ip, opt)
	if err := client.Connect(); err != nil {
		return CounterCompareResult{}, fmt.Errorf("snmp connect: %w", err)
	}
	defer client.Conn.Close()

	ifNames, err := c.fetchIfNames(client)
	if err != nil {
		return CounterCompareResult{}, err
	}
	hcIn, err := c.fetchCounterMap(client, OIDIfHCInOctets)
	if err != nil {
		return CounterCompareResult{}, err
	}
	hcOut, err := c.fetchCounterMap(client, OIDIfHCOutOctets)
	if err != nil {
		return CounterCompareResult{}, err
	}
	legacyIn, err := c.fetchCounterMap(client, OIDIfInOctets)
	if err != nil {
		return CounterCompareResult{}, err
	}
	legacyOut, err := c.fetchCounterMap(client, OIDIfOutOctets)
	if err != nil {
		return CounterCompareResult{}, err
	}

	out := make([]CounterCompareItem, 0, len(ifNames))
	for ifIndex, ifName := range ifNames {
		item := CounterCompareItem{
			IfIndex:         ifIndex,
			IfName:          ifName,
			HCInOctets:      hcIn[ifIndex],
			LegacyInOctets:  legacyIn[ifIndex],
			HCOutOctets:     hcOut[ifIndex],
			LegacyOutOctets: legacyOut[ifIndex],
		}
		if item.LegacyInOctets > 0 {
			item.InRatio = float64(item.HCInOctets) / float64(item.LegacyInOctets)
		}
		if item.LegacyOutOctets > 0 {
			item.OutRatio = float64(item.HCOutOctets) / float64(item.LegacyOutOctets)
		}
		out = append(out, item)
	}
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return CounterCompareResult{
		PolledAt:   time.Now(),
		Interfaces: out,
	}, nil
}

func (c *Collector) PollDevice(ip string, opt PollOptions) (PollResult, error) {
	client := c.newClient(ip, opt)
	if err := client.Connect(); err != nil {
		return PollResult{}, fmt.Errorf("snmp connect: %w", err)
	}
	defer client.Conn.Close()

	profile := selectOIDProfile(opt.Brand)
	cpuUsage, _ := c.getAverageCPU(client, profile.CPUOIDs)
	memUsage, _ := c.getAverageMemory(client, profile.MemoryOIDs)

	ifNames, err := c.fetchIfNames(client)
	if err != nil {
		return PollResult{}, err
	}
	merged, err := c.fetchInterfaceMetrics(client, ifNames, opt)
	if err != nil {
		return PollResult{}, err
	}

	stUsage, stTotal, stFree := c.fetchStorageSummary(client)
	uptimeSec := c.fetchSysUptimeSec(client)

	return PollResult{
		CPUUsage:     cpuUsage,
		MemoryUsage:  memUsage,
		StorageUsage: stUsage,
		StorageTotal: stTotal,
		StorageFree:  stFree,
		UptimeSec:    uptimeSec,
		Interfaces:   merged,
		PolledAt:     time.Now(),
	}, nil
}

// fetchInterfaceMetrics decides whether each port should use HC(64-bit) or legacy(32-bit)
// counters by speed and SNMP version:
// 1) ifHighSpeed/ifSpeed >= 1000Mbps: MUST use HC when available.
// 2) <=100Mbps: prefer HC on v2c/v3, fallback to legacy.
// 3) SNMP v1 on high-speed ports: warn because Counter64 is not supported.
func (c *Collector) fetchInterfaceMetrics(client *gosnmp.GoSNMP, ifNames map[int]string, opt PollOptions) ([]InterfaceCounters, error) {
	hcInMap, err := c.fetchCounterMapStrict(client, OIDIfHCInOctets)
	if err != nil {
		hcInMap = map[int]uint64{}
	}
	hcOutMap, err := c.fetchCounterMapStrict(client, OIDIfHCOutOctets)
	if err != nil {
		hcOutMap = map[int]uint64{}
	}
	legacyInMap, err := c.fetchCounterMapStrict(client, OIDIfInOctets)
	if err != nil {
		legacyInMap = map[int]uint64{}
	}
	legacyOutMap, err := c.fetchCounterMapStrict(client, OIDIfOutOctets)
	if err != nil {
		legacyOutMap = map[int]uint64{}
	}

	if len(hcInMap) == 0 && len(legacyInMap) == 0 {
		return nil, fmt.Errorf("walk counter failed: inbound counters unavailable")
	}
	if len(hcOutMap) == 0 && len(legacyOutMap) == 0 {
		return nil, fmt.Errorf("walk counter failed: outbound counters unavailable")
	}

	statusMap, _ := c.fetchIfOperStatus(client)
	speedMap, _ := c.fetchIfSpeedMbps(client)
	isV1 := strings.EqualFold(strings.TrimSpace(opt.SNMPVersion), "1") || strings.EqualFold(strings.TrimSpace(opt.SNMPVersion), "v1")

	merged := make([]InterfaceCounters, 0, len(ifNames))
	for idx, name := range ifNames {
		speed := speedMap[idx]
		hcIn, hcInOK := hcInMap[idx]
		hcOut, hcOutOK := hcOutMap[idx]
		legacyIn, legacyInOK := legacyInMap[idx]
		legacyOut, legacyOutOK := legacyOutMap[idx]
		hcPairOK := hcInOK && hcOutOK
		legacyPairOK := legacyInOK && legacyOutOK

		var inOct, outOct uint64
		switch {
		case speed >= 1000:
			// High-speed ports: use HC Counter64 pair whenever available.
			if hcPairOK {
				inOct = hcIn
				outOct = hcOut
			} else if legacyPairOK {
				if isV1 {
					log.Printf("snmp warning device version=v1 ifIndex=%d ifName=%s speed=%dMbps: Counter64 unavailable on SNMP v1, fallback to legacy", idx, name, speed)
				} else {
					log.Printf("snmp warning ifIndex=%d ifName=%s speed=%dMbps: HC counter missing, fallback to legacy", idx, name, speed)
				}
				inOct = legacyIn
				outOct = legacyOut
			} else {
				inOct, outOct = 0, 0
			}
		default:
			// Low-speed ports: still prefer HC when pair is available; fallback to legacy pair.
			if hcPairOK {
				inOct = hcIn
				outOct = hcOut
			} else if legacyPairOK {
				inOct = legacyIn
				outOct = legacyOut
			} else {
				inOct, outOct = 0, 0
			}
		}

		merged = append(merged, InterfaceCounters{
			IfIndex:         idx,
			IfName:          name,
			InOctets:        inOct,
			OutOctets:       outOct,
			HCInOctets:      hcIn,
			HCOutOctets:     hcOut,
			LegacyInOctets:  legacyIn,
			LegacyOutOctets: legacyOut,
			OperUp:          statusMap[idx],
			SpeedMbps:       speed,
		})
	}

	return merged, nil
}

func (c *Collector) fetchSysUptimeSec(client *gosnmp.GoSNMP) int64 {
	pdus, err := client.Get([]string{".1.3.6.1.2.1.1.3.0"})
	if err != nil || len(pdus.Variables) == 0 {
		return 0
	}
	ticks := toUint64(pdus.Variables[0].Value)
	if ticks == 0 {
		return 0
	}
	return int64(ticks / 100)
}

func (c *Collector) fetchIfSpeedMbps(client *gosnmp.GoSNMP) (map[int]int, error) {
	out := map[int]int{}
	high, err := client.BulkWalkAll(OIDIfHighSpeed)
	if err == nil {
		for _, p := range high {
			idx, e := oidIndex(p.Name)
			if e != nil {
				continue
			}
			v := int(toUint64(p.Value))
			if v > 0 {
				out[idx] = v
			}
		}
	}
	if len(out) > 0 {
		return out, nil
	}
	legacy, err := client.BulkWalkAll(OIDIfSpeed)
	if err != nil {
		return out, err
	}
	for _, p := range legacy {
		idx, e := oidIndex(p.Name)
		if e != nil {
			continue
		}
		bps := toUint64(p.Value)
		if bps == 0 {
			continue
		}
		mbps := int(bps / 1_000_000)
		if mbps <= 0 {
			mbps = 1
		}
		out[idx] = mbps
	}
	return out, nil
}

func (c *Collector) fetchStorageSummary(client *gosnmp.GoSNMP) (float64, float64, float64) {
	types, err1 := client.BulkWalkAll(OIDHrStorageType)
	allocs, err2 := client.BulkWalkAll(OIDHrStorageAlloc)
	sizes, err3 := client.BulkWalkAll(OIDHrStorageSize)
	useds, err4 := client.BulkWalkAll(OIDHrStorageUsed)
	if err1 != nil || err2 != nil || err3 != nil || err4 != nil {
		return 0, 0, 0
	}
	typeByIdx := map[int]string{}
	for _, p := range types {
		idx, err := oidIndex(p.Name)
		if err != nil {
			continue
		}
		typeByIdx[idx] = pduToString(p)
	}
	allocByIdx := map[int]uint64{}
	for _, p := range allocs {
		idx, err := oidIndex(p.Name)
		if err != nil {
			continue
		}
		allocByIdx[idx] = toUint64(p.Value)
	}
	sizeByIdx := map[int]uint64{}
	for _, p := range sizes {
		idx, err := oidIndex(p.Name)
		if err != nil {
			continue
		}
		sizeByIdx[idx] = toUint64(p.Value)
	}
	usedByIdx := map[int]uint64{}
	for _, p := range useds {
		idx, err := oidIndex(p.Name)
		if err != nil {
			continue
		}
		usedByIdx[idx] = toUint64(p.Value)
	}

	var totalBytes float64
	var usedBytes float64
	for idx, size := range sizeByIdx {
		alloc := allocByIdx[idx]
		used := usedByIdx[idx]
		if size == 0 || alloc == 0 {
			continue
		}
		stType := typeByIdx[idx]
		if !includeStorageType(stType) {
			continue
		}
		total := float64(size) * float64(alloc)
		use := float64(used) * float64(alloc)
		if total <= 0 || use < 0 {
			continue
		}
		totalBytes += total
		usedBytes += use
	}
	if totalBytes <= 0 {
		return 0, 0, 0
	}
	usage := (usedBytes / totalBytes) * 100
	if usage < 0 {
		usage = 0
	}
	if usage > 100 {
		usage = 100
	}
	return usage, totalBytes, totalBytes - usedBytes
}

func includeStorageType(storageTypeOID string) bool {
	t := strings.TrimSpace(storageTypeOID)
	if t == "" {
		return true
	}
	// HOST-RESOURCES-MIB::hrStorageTypes:
	// .2 ram, .3 virtualMemory, .4 fixedDisk, .5 removableDisk, .8 ramDisk, .9 flashMemory, .10 networkDisk.
	if strings.HasSuffix(t, ".2") || strings.HasSuffix(t, ".3") {
		return false
	}
	return strings.HasSuffix(t, ".4") ||
		strings.HasSuffix(t, ".5") ||
		strings.HasSuffix(t, ".8") ||
		strings.HasSuffix(t, ".9") ||
		strings.HasSuffix(t, ".10")
}

func v3MsgFlags(level string) gosnmp.SnmpV3MsgFlags {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "authnopriv", "auth_no_priv":
		return gosnmp.AuthNoPriv
	case "authpriv", "auth_priv":
		return gosnmp.AuthPriv
	default:
		return gosnmp.NoAuthNoPriv
	}
}

func v3AuthProtocol(p string) gosnmp.SnmpV3AuthProtocol {
	switch strings.ToLower(strings.TrimSpace(p)) {
	case "sha", "sha1":
		return gosnmp.SHA
	default:
		return gosnmp.MD5
	}
}

func v3PrivProtocol(p string) gosnmp.SnmpV3PrivProtocol {
	switch strings.ToLower(strings.TrimSpace(p)) {
	case "aes", "aes128":
		return gosnmp.AES
	default:
		return gosnmp.DES
	}
}

func (c *Collector) getAverageCPU(client *gosnmp.GoSNMP, oids []string) (float64, error) {
	var lastErr error
	for _, oid := range oids {
		pdus, err := client.BulkWalkAll(oid)
		if err == nil && len(pdus) > 0 {
			return averagePercent(pdus), nil
		}
		if err != nil {
			lastErr = err
		}
	}
	return 0, fmt.Errorf("walk cpu oid failed: %w", lastErr)
}

func (c *Collector) getAverageMemory(client *gosnmp.GoSNMP, oids []string) (float64, error) {
	var lastErr error
	for _, oid := range oids {
		pdus, err := client.BulkWalkAll(oid)
		if err == nil && len(pdus) > 0 {
			return averagePercent(pdus), nil
		}
		if err != nil {
			lastErr = err
		}
	}
	return 0, fmt.Errorf("walk memory oid failed: %w", lastErr)
}

func (c *Collector) fetchIfOperStatus(client *gosnmp.GoSNMP) (map[int]bool, error) {
	pdus, err := client.BulkWalkAll(OIDIfOperStatus)
	if err != nil {
		return nil, err
	}
	out := map[int]bool{}
	for _, p := range pdus {
		idx, err := oidIndex(p.Name)
		if err != nil {
			continue
		}
		out[idx] = toUint64(p.Value) == 1
	}
	return out, nil
}

func (c *Collector) fetchIfNames(client *gosnmp.GoSNMP) (map[int]string, error) {
	pdus, err := client.BulkWalkAll(OIDIfName)
	if err != nil {
		return nil, fmt.Errorf("walk ifName: %w", err)
	}
	m := make(map[int]string, len(pdus))
	for _, p := range pdus {
		idx, err := oidIndex(p.Name)
		if err != nil {
			continue
		}
		m[idx] = pduToString(p)
	}
	return m, nil
}

func (c *Collector) fetchCounterMap(client *gosnmp.GoSNMP, oid string) (map[int]uint64, error) {
	pdus, err := client.BulkWalkAll(oid)
	if err != nil || len(pdus) == 0 {
		fallback := ""
		if oid == OIDIfHCInOctets {
			fallback = OIDIfInOctets
		} else if oid == OIDIfHCOutOctets {
			fallback = OIDIfOutOctets
		}
		if fallback != "" {
			pdus, err = client.BulkWalkAll(fallback)
		}
	}
	if err != nil {
		return nil, fmt.Errorf("walk counter oid=%s failed: %w", oid, err)
	}
	m := make(map[int]uint64, len(pdus))
	for _, p := range pdus {
		idx, err := oidIndex(p.Name)
		if err != nil {
			continue
		}
		m[idx] = toUint64(p.Value)
	}
	return m, nil
}

func (c *Collector) fetchCounterMapStrict(client *gosnmp.GoSNMP, oid string) (map[int]uint64, error) {
	pdus, err := client.BulkWalkAll(oid)
	if err != nil {
		return nil, fmt.Errorf("walk counter oid=%s failed: %w", oid, err)
	}
	m := make(map[int]uint64, len(pdus))
	for _, p := range pdus {
		idx, err := oidIndex(p.Name)
		if err != nil {
			continue
		}
		m[idx] = toUint64(p.Value)
	}
	return m, nil
}

func averageNumeric(pdus []gosnmp.SnmpPDU) float64 {
	if len(pdus) == 0 {
		return 0
	}
	var sum float64
	var n float64
	for _, p := range pdus {
		sum += float64(toUint64(p.Value))
		n++
	}
	if n == 0 {
		return 0
	}
	return sum / n
}

func normalizePercent(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v <= 100 {
		return v
	}
	// Some devices return hundredths (e.g. 7534 => 75.34)
	if v <= 10000 {
		return v / 100
	}
	return 100
}

func averagePercent(pdus []gosnmp.SnmpPDU) float64 {
	if len(pdus) == 0 {
		return 0
	}
	values := make([]float64, 0, len(pdus))
	maxV := 0.0
	for _, p := range pdus {
		v := normalizePercent(float64(toUint64(p.Value)))
		if v < 0 || v > 100 {
			continue
		}
		values = append(values, v)
		if v > maxV {
			maxV = v
		}
		idx, err := oidIndex(p.Name)
		if err == nil && idx == 1 {
			// Prefer overall/main entity value when present.
			return v
		}
	}
	if len(values) == 0 {
		return 0
	}
	// Fallback to max to better match "display" commands on many vendors.
	return maxV
}

func oidIndex(oid string) (int, error) {
	parts := strings.Split(strings.TrimPrefix(oid, "."), ".")
	if len(parts) == 0 {
		return 0, fmt.Errorf("invalid oid: %s", oid)
	}
	return strconv.Atoi(parts[len(parts)-1])
}

func pduToString(pdu gosnmp.SnmpPDU) string {
	if b, ok := pdu.Value.([]byte); ok {
		return string(b)
	}
	return fmt.Sprintf("%v", pdu.Value)
}

func toUint64(v any) uint64 {
	switch x := v.(type) {
	case uint64:
		return x
	case uint32:
		return uint64(x)
	case uint:
		return uint64(x)
	case int:
		if x < 0 {
			return 0
		}
		return uint64(x)
	case int64:
		if x < 0 {
			return 0
		}
		return uint64(x)
	case int32:
		if x < 0 {
			return 0
		}
		return uint64(x)
	case int16:
		if x < 0 {
			return 0
		}
		return uint64(x)
	case uint16:
		return uint64(x)
	case int8:
		if x < 0 {
			return 0
		}
		return uint64(x)
	case uint8:
		return uint64(x)
	case string:
		n, _ := strconv.ParseUint(x, 10, 64)
		return n
	default:
		return 0
	}
}
