package snmp

import (
	"context"
	"fmt"
	"log"
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
	repo      *db.Repository
	collector *Collector
	interval  time.Duration

	mu   sync.Mutex
	last map[string]counterState
}

func NewWorker(repo *db.Repository, collector *Collector, interval time.Duration) *Worker {
	return &Worker{
		repo:      repo,
		collector: collector,
		interval:  interval,
		last:      make(map[string]counterState),
	}
}

func (w *Worker) Start(ctx context.Context) {
	w.runOnce(ctx)

	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Printf("snmp worker stopped: %v", ctx.Err())
			return
		case <-ticker.C:
			w.runOnce(ctx)
		}
	}
}

func (w *Worker) runOnce(ctx context.Context) {
	devices, err := w.repo.ListDevices(ctx)
	if err != nil {
		log.Printf("snmp worker list devices failed: %v", err)
		return
	}

	for _, d := range devices {
		result, err := w.collector.PollDevice(d.IP, d.Community)
		if err != nil {
			log.Printf("snmp poll failed device=%d ip=%s: %v", d.ID, d.IP, err)
			continue
		}

		interfaces := make([]db.Interface, 0, len(result.Interfaces))
		for _, itf := range result.Interfaces {
			interfaces = append(interfaces, db.Interface{
				DeviceID: d.ID,
				Index:    itf.IfIndex,
				Name:     itf.IfName,
			})
		}
		if err := w.repo.SyncInterfaces(ctx, d.ID, interfaces); err != nil {
			log.Printf("sync interfaces failed device=%d: %v", d.ID, err)
			continue
		}

		mList := make([]db.InterfaceMetric, 0, len(result.Interfaces))
		for _, itf := range result.Interfaces {
			inBps, outBps := w.calcBps(d.ID, itf.IfIndex, itf.InOctets, itf.OutOctets, result.PolledAt)
			mList = append(mList, db.InterfaceMetric{
				IfIndex:       itf.IfIndex,
				CPUUsage:      result.CPUUsage,
				MemoryUsage:   result.MemoryUsage,
				TrafficInBps:  inBps,
				TrafficOutBps: outBps,
			})
		}

		if err := w.repo.SaveMetrics(ctx, d.ID, result.PolledAt, mList); err != nil {
			log.Printf("save metrics failed device=%d: %v", d.ID, err)
			continue
		}
	}
}

func (w *Worker) calcBps(deviceID int64, ifIndex int, inOctets, outOctets uint64, now time.Time) (int64, int64) {
	key := interfaceKey(deviceID, ifIndex)

	w.mu.Lock()
	defer w.mu.Unlock()

	prev, ok := w.last[key]
	w.last[key] = counterState{
		inOctets:  inOctets,
		outOctets: outOctets,
		at:        now,
	}

	if !ok {
		return 0, 0
	}

	seconds := now.Sub(prev.at).Seconds()
	if seconds <= 0 {
		return 0, 0
	}

	inDelta := safeDelta(inOctets, prev.inOctets)
	outDelta := safeDelta(outOctets, prev.outOctets)

	inBps := int64(float64(inDelta*8) / seconds)
	outBps := int64(float64(outDelta*8) / seconds)
	return inBps, outBps
}

func interfaceKey(deviceID int64, ifIndex int) string {
	return fmt.Sprintf("%d:%d", deviceID, ifIndex)
}

func safeDelta(curr, prev uint64) uint64 {
	if curr < prev {
		return 0
	}
	return curr - prev
}
