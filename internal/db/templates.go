package db

import (
	"context"
	"database/sql"
	"math"
	"strings"
)

func (r *Repository) CreateTemplate(ctx context.Context, t DeviceTemplate) (int64, error) {
	t.Community = r.encryptOpt(t.Community)
	t.V3AuthPassword = r.encryptOpt(t.V3AuthPassword)
	t.V3PrivPassword = r.encryptOpt(t.V3PrivPassword)
	const q = `INSERT INTO device_templates(name,brand,description,match_sysobjectid,match_sysdescr,priority,auto_enabled,snmp_version,snmp_port,community,v3_username,v3_auth_protocol,v3_auth_password,v3_priv_protocol,v3_priv_password,v3_security_level,cpu_oid,mem_oid,if_in_oid,if_out_oid) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20) RETURNING id;`
	var id int64
	if t.Priority <= 0 {
		t.Priority = 100
	}
	if err := r.db.QueryRowContext(ctx, q, t.Name, t.Brand, t.Description, t.MatchSysObjectID, t.MatchSysDescr, t.Priority, t.AutoEnabled, t.SNMPVersion, t.SNMPPort, t.Community, t.V3Username, t.V3AuthProtocol, t.V3AuthPassword, t.V3PrivProtocol, t.V3PrivPassword, t.V3SecurityLevel, t.CPUOID, t.MemOID, t.IfInOID, t.IfOutOID).Scan(&id); err != nil {
		return 0, err
	}
	return id, nil
}
func (r *Repository) ListTemplates(ctx context.Context) ([]DeviceTemplate, error) {
	const q = `SELECT id,name,brand,COALESCE(description,''),COALESCE(match_sysobjectid,''),COALESCE(match_sysdescr,''),COALESCE(priority,100),COALESCE(auto_enabled,TRUE),snmp_version,snmp_port,COALESCE(community,''),COALESCE(v3_username,''),COALESCE(v3_auth_protocol,''),COALESCE(v3_auth_password,''),COALESCE(v3_priv_protocol,''),COALESCE(v3_priv_password,''),COALESCE(v3_security_level,''),COALESCE(cpu_oid,''),COALESCE(mem_oid,''),COALESCE(if_in_oid,''),COALESCE(if_out_oid,''),created_at FROM device_templates ORDER BY priority ASC, id DESC;`
	rows, err := r.db.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []DeviceTemplate{}
	for rows.Next() {
		var t DeviceTemplate
		if err := rows.Scan(&t.ID, &t.Name, &t.Brand, &t.Description, &t.MatchSysObjectID, &t.MatchSysDescr, &t.Priority, &t.AutoEnabled, &t.SNMPVersion, &t.SNMPPort, &t.Community, &t.V3Username, &t.V3AuthProtocol, &t.V3AuthPassword, &t.V3PrivProtocol, &t.V3PrivPassword, &t.V3SecurityLevel, &t.CPUOID, &t.MemOID, &t.IfInOID, &t.IfOutOID, &t.CreatedAt); err != nil {
			return nil, err
		}
		// Do not expose template credentials to frontend list API.
		t.Community = ""
		t.V3AuthPassword = ""
		t.V3PrivPassword = ""
		out = append(out, t)
	}
	return out, rows.Err()
}
func (r *Repository) GetTemplateByID(ctx context.Context, id int64) (*DeviceTemplate, error) {
	if id <= 0 {
		return nil, nil
	}
	const q = `SELECT id,name,brand,COALESCE(description,''),COALESCE(match_sysobjectid,''),COALESCE(match_sysdescr,''),COALESCE(priority,100),COALESCE(auto_enabled,TRUE),snmp_version,snmp_port,COALESCE(community,''),COALESCE(v3_username,''),COALESCE(v3_auth_protocol,''),COALESCE(v3_auth_password,''),COALESCE(v3_priv_protocol,''),COALESCE(v3_priv_password,''),COALESCE(v3_security_level,''),COALESCE(cpu_oid,''),COALESCE(mem_oid,''),COALESCE(if_in_oid,''),COALESCE(if_out_oid,''),created_at FROM device_templates WHERE id=$1 LIMIT 1;`
	var t DeviceTemplate
	if err := r.db.QueryRowContext(ctx, q, id).Scan(&t.ID, &t.Name, &t.Brand, &t.Description, &t.MatchSysObjectID, &t.MatchSysDescr, &t.Priority, &t.AutoEnabled, &t.SNMPVersion, &t.SNMPPort, &t.Community, &t.V3Username, &t.V3AuthProtocol, &t.V3AuthPassword, &t.V3PrivProtocol, &t.V3PrivPassword, &t.V3SecurityLevel, &t.CPUOID, &t.MemOID, &t.IfInOID, &t.IfOutOID, &t.CreatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	t.Community = r.decryptOpt(t.Community)
	t.V3AuthPassword = r.decryptOpt(t.V3AuthPassword)
	t.V3PrivPassword = r.decryptOpt(t.V3PrivPassword)
	return &t, nil
}
func (r *Repository) MatchTemplateByFingerprint(ctx context.Context, brand, sysObjectID, sysDescr string) (*DeviceTemplate, int, error) {
	templates, err := r.ListTemplates(ctx)
	if err != nil {
		return nil, 0, err
	}
	br := strings.ToLower(strings.TrimSpace(brand))
	oid := strings.ToLower(strings.TrimSpace(sysObjectID))
	desc := strings.ToLower(strings.TrimSpace(sysDescr))
	bestScore := -1
	var best *DeviceTemplate
	for i := range templates {
		t := templates[i]
		if !t.AutoEnabled {
			continue
		}
		score := 0
		tb := strings.ToLower(strings.TrimSpace(t.Brand))
		if br != "" && tb != "" {
			if br == tb {
				score += 30
			} else {
				continue
			}
		}
		matchOID := strings.ToLower(strings.TrimSpace(t.MatchSysObjectID))
		matchDesc := strings.ToLower(strings.TrimSpace(t.MatchSysDescr))
		if matchOID != "" && oid != "" {
			for _, part := range strings.Split(matchOID, ",") {
				p := strings.TrimSpace(part)
				if p == "" {
					continue
				}
				if strings.Contains(oid, p) {
					score += 60
					break
				}
			}
		}
		if matchDesc != "" && desc != "" {
			for _, part := range strings.Split(matchDesc, ",") {
				p := strings.TrimSpace(part)
				if p == "" {
					continue
				}
				if strings.Contains(desc, p) {
					score += 40
					break
				}
			}
		}
		if matchOID == "" && matchDesc == "" {
			score += 10
		}
		score += int(math.Max(0, float64(200-t.Priority))) / 10
		if score > bestScore {
			bestScore = score
			cp := t
			best = &cp
		}
	}
	if bestScore < 40 {
		return nil, 0, nil
	}
	return best, bestScore, nil
}
