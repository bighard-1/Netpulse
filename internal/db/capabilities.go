package db

import (
	"context"
	"database/sql"
	"fmt"
)

func (r *Repository) UpsertDeviceCapability(ctx context.Context, c DeviceCapability) error {
	const q = `
		INSERT INTO device_capabilities(
			device_id, snmp_version, supports_cpu, supports_memory, supports_if_traffic, interface_count, updated_at
		) VALUES($1,$2,$3,$4,$5,$6,NOW())
		ON CONFLICT(device_id) DO UPDATE SET
			snmp_version = EXCLUDED.snmp_version,
			supports_cpu = EXCLUDED.supports_cpu,
			supports_memory = EXCLUDED.supports_memory,
			supports_if_traffic = EXCLUDED.supports_if_traffic,
			interface_count = EXCLUDED.interface_count,
			updated_at = NOW();
	`
	_, err := r.db.ExecContext(ctx, q, c.DeviceID, c.SNMPVersion, c.SupportsCPU, c.SupportsMemory, c.SupportsIfTraffic, c.InterfaceCount)
	if err != nil {
		return fmt.Errorf("upsert device capability: %w", err)
	}
	_, _ = r.db.ExecContext(ctx, `
		INSERT INTO device_capability_history(
			device_id, snmp_version, supports_cpu, supports_memory, supports_if_traffic, interface_count, created_at
		) VALUES($1,$2,$3,$4,$5,$6,NOW());
	`, c.DeviceID, c.SNMPVersion, c.SupportsCPU, c.SupportsMemory, c.SupportsIfTraffic, c.InterfaceCount)
	return nil
}
func (r *Repository) GetDeviceCapability(ctx context.Context, deviceID int64) (*DeviceCapability, error) {
	const q = `
		SELECT device_id, COALESCE(snmp_version,''), supports_cpu, supports_memory, supports_if_traffic, interface_count, updated_at
		FROM device_capabilities
		WHERE device_id = $1
		LIMIT 1;
	`
	var c DeviceCapability
	err := r.db.QueryRowContext(ctx, q, deviceID).Scan(
		&c.DeviceID, &c.SNMPVersion, &c.SupportsCPU, &c.SupportsMemory, &c.SupportsIfTraffic, &c.InterfaceCount, &c.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get device capability: %w", err)
	}
	return &c, nil
}
