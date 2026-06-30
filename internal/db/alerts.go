package db

import (
	"context"
	"fmt"
	"strings"
)

func (r *Repository) SaveAlertEvent(ctx context.Context, ruleID *int64, deviceID int64, level, code, message string) error {
	const q = `INSERT INTO alert_events(rule_id,device_id,level,code,message,status) VALUES($1,$2,$3,$4,$5,'open');`
	_, err := r.db.ExecContext(ctx, q, ruleID, deviceID, level, code, message)
	return err
}
func (r *Repository) ListAlertEvents(ctx context.Context, limit int, status string) ([]AlertEvent, error) {
	if limit <= 0 || limit > 1000 {
		limit = 200
	}
	const q = `
		SELECT ae.id, ae.rule_id, ae.device_id, host(d.ip), COALESCE(d.name,host(d.ip)),
		       ae.level, ae.code, ae.message, ae.status, COALESCE(ae.assignee,''), COALESCE(ae.note,''),
		       ae.silenced_until, ae.acknowledged_at, ae.resolved_at, ae.created_at
		FROM alert_events ae
		JOIN devices d ON d.id = ae.device_id
		WHERE d.deleted_at IS NULL
		  AND ($2='' OR ae.status=$2)
		ORDER BY ae.created_at DESC
		LIMIT $1;
	`
	rows, err := r.db.QueryContext(ctx, q, limit, strings.TrimSpace(status))
	if err != nil {
		return nil, fmt.Errorf("list alert events: %w", err)
	}
	defer rows.Close()
	out := make([]AlertEvent, 0, limit)
	for rows.Next() {
		var a AlertEvent
		if err := rows.Scan(&a.ID, &a.RuleID, &a.DeviceID, &a.DeviceIP, &a.DeviceName, &a.Level, &a.Code, &a.Message, &a.Status, &a.Assignee, &a.Note, &a.SilencedUntil, &a.AcknowledgedAt, &a.ResolvedAt, &a.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan alert event: %w", err)
		}
		out = append(out, a)
	}
	return out, rows.Err()
}
func (r *Repository) UpdateAlertEventWorkflow(ctx context.Context, id int64, action, assignee, note string, silenceMinutes int) error {
	action = strings.TrimSpace(strings.ToLower(action))
	if id <= 0 || action == "" {
		return fmt.Errorf("invalid alert workflow update")
	}
	switch action {
	case "ack":
		_, err := r.db.ExecContext(ctx, `UPDATE alert_events SET status='ack', assignee=$2, note=$3, acknowledged_at=NOW() WHERE id=$1;`, id, strings.TrimSpace(assignee), strings.TrimSpace(note))
		return err
	case "resolve":
		_, err := r.db.ExecContext(ctx, `UPDATE alert_events SET status='resolved', assignee=$2, note=$3, resolved_at=NOW() WHERE id=$1;`, id, strings.TrimSpace(assignee), strings.TrimSpace(note))
		return err
	case "reopen":
		_, err := r.db.ExecContext(ctx, `UPDATE alert_events SET status='open', note=$2 WHERE id=$1;`, id, strings.TrimSpace(note))
		return err
	case "silence":
		if silenceMinutes <= 0 {
			silenceMinutes = 30
		}
		_, err := r.db.ExecContext(ctx, `UPDATE alert_events SET status='silenced', assignee=$2, note=$3, silenced_until=NOW() + ($4 || ' minutes')::interval WHERE id=$1;`, id, strings.TrimSpace(assignee), strings.TrimSpace(note), silenceMinutes)
		return err
	default:
		return fmt.Errorf("unsupported alert action")
	}
}
func (r *Repository) UpsertAlertRule(ctx context.Context, ar AlertRule) (int64, error) {
	if ar.ID == 0 {
		const ins = `INSERT INTO alert_rules(name,scope,device_id,cpu_threshold,mem_threshold,traffic_threshold,mute_start,mute_end,notify_webhook,enabled) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) RETURNING id;`
		var id int64
		if err := r.db.QueryRowContext(ctx, ins, ar.Name, ar.Scope, ar.DeviceID, ar.CPUThreshold, ar.MemThreshold, ar.TrafficThreshold, ar.MuteStart, ar.MuteEnd, ar.NotifyWebhook, ar.Enabled).Scan(&id); err != nil {
			return 0, err
		}
		return id, nil
	}
	const up = `UPDATE alert_rules SET name=$2,scope=$3,device_id=$4,cpu_threshold=$5,mem_threshold=$6,traffic_threshold=$7,mute_start=$8,mute_end=$9,notify_webhook=$10,enabled=$11 WHERE id=$1;`
	_, err := r.db.ExecContext(ctx, up, ar.ID, ar.Name, ar.Scope, ar.DeviceID, ar.CPUThreshold, ar.MemThreshold, ar.TrafficThreshold, ar.MuteStart, ar.MuteEnd, ar.NotifyWebhook, ar.Enabled)
	return ar.ID, err
}
func (r *Repository) ListAlertRules(ctx context.Context) ([]AlertRule, error) {
	const q = `SELECT id,name,scope,device_id,cpu_threshold,mem_threshold,traffic_threshold,COALESCE(mute_start,''),COALESCE(mute_end,''),COALESCE(notify_webhook,''),enabled,created_at FROM alert_rules ORDER BY id DESC;`
	rows, err := r.db.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []AlertRule{}
	for rows.Next() {
		var a AlertRule
		if err := rows.Scan(&a.ID, &a.Name, &a.Scope, &a.DeviceID, &a.CPUThreshold, &a.MemThreshold, &a.TrafficThreshold, &a.MuteStart, &a.MuteEnd, &a.NotifyWebhook, &a.Enabled, &a.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}
func (r *Repository) DeleteAlertRule(ctx context.Context, id int64) error {
	if id <= 0 {
		return fmt.Errorf("invalid alert rule id")
	}
	_, err := r.db.ExecContext(ctx, `DELETE FROM alert_rules WHERE id=$1;`, id)
	if err != nil {
		return fmt.Errorf("delete alert rule: %w", err)
	}
	return nil
}
