package snmp

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"netpulse/internal/db"
)

type Alert struct {
	Level      string    `json:"level"`
	Code       string    `json:"code"`
	DeviceID   int64     `json:"device_id"`
	DeviceIP   string    `json:"device_ip"`
	DeviceName string    `json:"device_name"`
	Brand      string    `json:"brand"`
	Message    string    `json:"message"`
	Suppressed bool      `json:"suppressed"`
	RelatedTo  string    `json:"related_to,omitempty"`
	TS         time.Time `json:"ts"`
}

type WebhookConfig struct {
	Provider string `json:"provider"`
	Endpoint string `json:"endpoint"`
	Secret   string `json:"secret,omitempty"`
	Enabled  bool   `json:"enabled"`
}

type AlertManager struct {
	repo          *db.Repository
	webhook       string
	httpClient    *http.Client
	upstreamLinks map[int64][]int64
}

func NewAlertManager(repo *db.Repository, webhook string) *AlertManager {
	return &AlertManager{
		repo:          repo,
		webhook:       strings.TrimSpace(webhook),
		httpClient:    &http.Client{Timeout: 5 * time.Second},
		upstreamLinks: map[int64][]int64{},
	}
}

func (m *AlertManager) SetWebhook(url string) { m.webhook = strings.TrimSpace(url) }

func (m *AlertManager) RefreshTopology(ctx context.Context) {
	links, err := m.repo.ListTopologyLinks(ctx)
	if err != nil {
		return
	}
	next := make(map[int64][]int64)
	for _, l := range links {
		next[l.DstDeviceID] = append(next[l.DstDeviceID], l.SrcDeviceID)
	}
	m.upstreamLinks = next
}

func (m *AlertManager) ShouldSuppress(d db.Device, devUp map[int64]bool) (bool, string) {
	if d.MaintenanceMode {
		return true, "maintenance_mode"
	}
	ups := m.upstreamLinks[d.ID]
	for _, upID := range ups {
		if isUp, ok := devUp[upID]; ok && !isUp {
			return true, fmt.Sprintf("upstream_device_down:%d", upID)
		}
	}
	return false, ""
}

func (m *AlertManager) Notify(a Alert) {
	// Placeholder hook. Future providers: DingTalk/Slack/Telegram.
	if strings.TrimSpace(m.webhook) == "" {
		return
	}
	body := fmt.Sprintf(`{"level":"%s","code":"%s","device_id":%d,"ip":"%s","name":"%s","brand":"%s","message":"%s","suppressed":%t,"related_to":"%s","ts":"%s"}`,
		a.Level, a.Code, a.DeviceID, a.DeviceIP, strings.ReplaceAll(a.DeviceName, `"`, `'`), a.Brand,
		strings.ReplaceAll(a.Message, `"`, `'`), a.Suppressed, strings.ReplaceAll(a.RelatedTo, `"`, `'`), a.TS.Format(time.RFC3339))
	req, _ := http.NewRequest(http.MethodPost, m.webhook, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := m.httpClient.Do(req)
	if err == nil && resp != nil {
		_ = resp.Body.Close()
	}
}
