package api

import (
	"encoding/json"
	"net"
	"net/http"
	"sync"
	"time"
)

type discoveryReq struct {
	CIDR      string `json:"cidr"`
	Community string `json:"community"`
	Brand     string `json:"brand"`
}

func (h *Handler) handleDiscoveryScan(w http.ResponseWriter, r *http.Request) {
	var req discoveryReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	_, ipnet, err := net.ParseCIDR(req.CIDR)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid cidr")
		return
	}
	ips := ipsInCIDR(ipnet, 256)
	results := make([]map[string]any, 0)
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, 32)
	for _, ip := range ips {
		wg.Add(1)
		sem <- struct{}{}
		go func(ip string) {
			defer wg.Done()
			defer func() { <-sem }()
			conn, err := net.DialTimeout("tcp", ip+":161", 600*time.Millisecond)
			up := err == nil
			if conn != nil {
				_ = conn.Close()
			}
			if up {
				mu.Lock()
				results = append(results, map[string]any{"ip": ip, "snmp": true})
				mu.Unlock()
			}
		}(ip)
	}
	wg.Wait()
	writeJSON(w, http.StatusOK, map[string]any{"cidr": req.CIDR, "results": results})
}

func ipsInCIDR(ipnet *net.IPNet, limit int) []string {
	var out []string
	ip := ipnet.IP.To4()
	if ip == nil {
		return out
	}
	for i := 1; i < 254 && len(out) < limit; i++ {
		c := make(net.IP, len(ip))
		copy(c, ip)
		c[3] = byte(i)
		if ipnet.Contains(c) {
			out = append(out, c.String())
		}
	}
	return out
}
