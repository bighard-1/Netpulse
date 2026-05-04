package snmp

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/gosnmp/gosnmp"
)

const (
	OIDCPUUsage      = ".1.3.6.1.4.1.2011.5.25.31.1.1.1.1.5"
	OIDMemoryUsage   = ".1.3.6.1.4.1.2011.5.25.31.1.1.1.1.7"
	OIDIfName        = ".1.3.6.1.2.1.31.1.1.1.1"
	OIDIfHCInOctets  = ".1.3.6.1.2.1.31.1.1.1.6"
	OIDIfHCOutOctets = ".1.3.6.1.2.1.31.1.1.1.10"
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

type Collector struct {
	Port      uint16
	Version   gosnmp.SnmpVersion
	Timeout   time.Duration
	Retries   int
	MaxOids   int
	MaxRep    uint32
	LocalBind string
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

func (c *Collector) newClient(ip, community string) *gosnmp.GoSNMP {
	return &gosnmp.GoSNMP{
		Target:         ip,
		Port:           c.Port,
		Community:      community,
		Version:        c.Version,
		Timeout:        c.Timeout,
		Retries:        c.Retries,
		MaxOids:        c.MaxOids,
		MaxRepetitions: c.MaxRep,
	}
}

func (c *Collector) FetchInterfaces(ip, community string) ([]InterfaceInfo, error) {
	client := c.newClient(ip, community)
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

func (c *Collector) PollDevice(ip, community string) (PollResult, error) {
	client := c.newClient(ip, community)
	if err := client.Connect(); err != nil {
		return PollResult{}, fmt.Errorf("snmp connect: %w", err)
	}
	defer client.Conn.Close()

	cpuUsage, err := c.getAverageCPU(client)
	if err != nil {
		return PollResult{}, err
	}
	memUsage, err := c.getAverageMemory(client)
	if err != nil {
		return PollResult{}, err
	}

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

func (c *Collector) getAverageCPU(client *gosnmp.GoSNMP) (float64, error) {
	pdus, err := client.BulkWalkAll(OIDCPUUsage)
	if err != nil {
		return 0, fmt.Errorf("walk cpu oid: %w", err)
	}
	return averageNumeric(pdus), nil
}

func (c *Collector) getAverageMemory(client *gosnmp.GoSNMP) (float64, error) {
	pdus, err := client.BulkWalkAll(OIDMemoryUsage)
	if err != nil {
		return 0, fmt.Errorf("walk memory oid: %w", err)
	}
	return averageNumeric(pdus), nil
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
	if err != nil {
		return nil, fmt.Errorf("walk counter oid=%s: %w", oid, err)
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
