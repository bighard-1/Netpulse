package snmp

import (
	"context"
	"fmt"
	"log"
	"net"
	"strings"
	"time"

	"github.com/gosnmp/gosnmp"

	"netpulse/internal/db"
)

func StartSyslogServer(ctx context.Context, repo *db.Repository, addr string) {
	pc, err := net.ListenPacket("udp", addr)
	if err != nil {
		log.Printf("syslog listen failed: %v", err)
		return
	}
	defer pc.Close()
	go func() {
		<-ctx.Done()
		_ = pc.Close()
	}()
	buf := make([]byte, 8192)
	for {
		_ = pc.SetReadDeadline(time.Now().Add(2 * time.Second))
		n, remote, err := pc.ReadFrom(buf)
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				if ctx.Err() != nil {
					return
				}
				continue
			}
			if ctx.Err() != nil {
				return
			}
			continue
		}
		ip := remote.(*net.UDPAddr).IP.String()
		msg := strings.TrimSpace(string(buf[:n]))
		dev, err := repo.FindDeviceByIP(context.Background(), ip)
		if err == nil && dev != nil {
			_ = repo.AddDeviceLog(context.Background(), dev.ID, "INFO", "[SYSLOG] "+msg)
		}
	}
}

func StartTrapServer(ctx context.Context, repo *db.Repository, addr string) {
	tl := gosnmp.NewTrapListener()
	tl.OnNewTrap = func(packet *gosnmp.SnmpPacket, ua *net.UDPAddr) {
		ip := ua.IP.String()
		dev, err := repo.FindDeviceByIP(context.Background(), ip)
		if err != nil || dev == nil {
			return
		}
		msg := fmt.Sprintf("[TRAP] version=%v vars=%d", packet.Version, len(packet.Variables))
		_ = repo.AddDeviceLog(context.Background(), dev.ID, "WARNING", msg)
	}
	go func() { <-ctx.Done() }()
	if err := tl.Listen(addr); err != nil && ctx.Err() == nil {
		log.Printf("trap listen failed: %v", err)
	}
}
