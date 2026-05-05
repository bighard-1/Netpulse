package snmp

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/gosnmp/gosnmp"
)

const (
	OIDCPUUsage       = ".1.3.6.1.4.1.2011.5.25.31.1.1.1.1.5"
	OIDMemoryUsage    = ".1.3.6.1.4.1.2011.5.25.31.1.1.1.1.7"
	OIDCPUUsageH3C    = ".1.3.6.1.4.1.25506.2.6.1.1.1.1.8"
	OIDMemoryUsageH3C = ".1.3.6.1.4.1.25506.2.6.1.1.1.1.12"
	OIDIfName         = ".1.3.6.1.2.1.31.1.1.1.1"
	OIDIfHCInOctets   = ".1.3.6.1.2.1.31.1.1.1.6"
	OIDIfHCOutOctets  = ".1.3.6.1.2.1.31.1.1.1.10"
	OIDIfInOctets     = ".1.3.6.1.2.1.2.2.1.10"
	OIDIfOutOctets    = ".1.3.6.1.2.1.2.2.1.16"
)

type InterfaceInfo struct {
	IfIndex int
	IfName  string
}

type InterfaceCounters struct {
	IfIndex   int
	IfName    string
	InOctets  uint64
	OutOctets uint64
}

type PollResult struct {
	CPUUsage    float64
	MemoryUsage float64
	Interfaces  []InterfaceCounters
	PolledAt    time.Time
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
			CPUOIDs:    []string{OIDCPUUsageH3C, OIDCPUUsage},
			MemoryOIDs: []string{OIDMemoryUsageH3C, OIDMemoryUsage},
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
	inMap, err := c.fetchCounterMap(client, OIDIfHCInOctets)
	if err != nil {
		return PollResult{}, err
	}
	outMap, err := c.fetchCounterMap(client, OIDIfHCOutOctets)
	if err != nil {
		return PollResult{}, err
	}

	merged := make([]InterfaceCounters, 0, len(ifNames))
	for idx, name := range ifNames {
		merged = append(merged, InterfaceCounters{
			IfIndex:   idx,
			IfName:    name,
			InOctets:  inMap[idx],
			OutOctets: outMap[idx],
		})
	}

	return PollResult{
		CPUUsage:    cpuUsage,
		MemoryUsage: memUsage,
		Interfaces:  merged,
		PolledAt:    time.Now(),
	}, nil
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
			return averageNumeric(pdus), nil
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
			return averageNumeric(pdus), nil
		}
		if err != nil {
			lastErr = err
		}
	}
	return 0, fmt.Errorf("walk memory oid failed: %w", lastErr)
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
