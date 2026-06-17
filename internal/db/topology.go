package db

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

func (r *Repository) UpsertTopologyLink(ctx context.Context, l TopologyLink) (int64, error) {
	if l.ID == 0 {
		const ins = `INSERT INTO topology_links(src_device_id,src_if_index,dst_device_id,dst_if_index,protocol,remark) VALUES($1,$2,$3,$4,$5,$6) RETURNING id;`
		var id int64
		if err := r.db.QueryRowContext(ctx, ins, l.SrcDeviceID, l.SrcIfIndex, l.DstDeviceID, l.DstIfIndex, l.Protocol, l.Remark).Scan(&id); err != nil {
			return 0, err
		}
		return id, nil
	}
	const up = `UPDATE topology_links SET src_device_id=$2,src_if_index=$3,dst_device_id=$4,dst_if_index=$5,protocol=$6,remark=$7 WHERE id=$1;`
	_, err := r.db.ExecContext(ctx, up, l.ID, l.SrcDeviceID, l.SrcIfIndex, l.DstDeviceID, l.DstIfIndex, l.Protocol, l.Remark)
	return l.ID, err
}

func (r *Repository) ListTopologyLinks(ctx context.Context) ([]TopologyLink, error) {
	const q = `SELECT id,src_device_id,src_if_index,dst_device_id,dst_if_index,protocol,COALESCE(remark,''),created_at FROM topology_links ORDER BY id DESC;`
	rows, err := r.db.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []TopologyLink{}
	for rows.Next() {
		var t TopologyLink
		if err := rows.Scan(&t.ID, &t.SrcDeviceID, &t.SrcIfIndex, &t.DstDeviceID, &t.DstIfIndex, &t.Protocol, &t.Remark, &t.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (r *Repository) DeleteTopologyLink(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM topology_links WHERE id=$1;`, id)
	return err
}

func (r *Repository) GetTopologyGraph(ctx context.Context) (*TopologyGraph, error) {
	runtime, _ := r.GetRuntimeSettings(ctx)
	onlineWindow := time.Duration(runtime.StatusOnlineWindowSec) * time.Second
	if onlineWindow <= 0 {
		onlineWindow = 5 * time.Minute
	}
	const nq = `
		SELECT n.id, n.device_id, COALESCE(NULLIF(n.label,''), d.name, host(d.ip)), n.x, n.y,
		       COALESCE(d.name, host(d.ip)), host(d.ip), COALESCE(d.brand, ''), COALESCE(d.device_tier, 'access'),
		       lm.ts, COALESCE(dl.message, ''), n.created_at, n.updated_at
		FROM topology_nodes n
		JOIN devices d ON d.id = n.device_id
		LEFT JOIN device_latest_metrics lm ON lm.device_id = d.id
		LEFT JOIN LATERAL (
			SELECT message
			FROM device_logs
			WHERE device_id = d.id
			ORDER BY created_at DESC
			LIMIT 1
		) dl ON TRUE
		ORDER BY n.id;
	`
	rows, err := r.db.QueryContext(ctx, nq)
	if err != nil {
		return nil, fmt.Errorf("query topology nodes: %w", err)
	}
	defer rows.Close()
	graph := &TopologyGraph{Nodes: []TopologyNode{}, Edges: []TopologyEdge{}}
	now := time.Now()
	for rows.Next() {
		var n TopologyNode
		var lastMetricAt *time.Time
		if err := rows.Scan(&n.ID, &n.DeviceID, &n.Label, &n.X, &n.Y, &n.DeviceName, &n.DeviceIP, &n.DeviceBrand, &n.DeviceTier, &lastMetricAt, &n.StatusReason, &n.CreatedAt, &n.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan topology node: %w", err)
		}
		n.DeviceStatus = "unknown"
		if lastMetricAt != nil {
			if now.Sub(*lastMetricAt) <= onlineWindow {
				n.DeviceStatus = "online"
				n.StatusReason = ""
			} else {
				n.DeviceStatus = "offline"
			}
		}
		graph.Nodes = append(graph.Nodes, n)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate topology nodes: %w", err)
	}
	const eq = `
		SELECT id, source_node_id, target_node_id, COALESCE(label,''), COALESCE(remark,''), created_at, updated_at
		FROM topology_edges
		ORDER BY id;
	`
	eRows, err := r.db.QueryContext(ctx, eq)
	if err != nil {
		return nil, fmt.Errorf("query topology edges: %w", err)
	}
	defer eRows.Close()
	for eRows.Next() {
		var e TopologyEdge
		if err := eRows.Scan(&e.ID, &e.SourceNodeID, &e.TargetNodeID, &e.Label, &e.Remark, &e.CreatedAt, &e.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan topology edge: %w", err)
		}
		graph.Edges = append(graph.Edges, e)
	}
	if err := eRows.Err(); err != nil {
		return nil, fmt.Errorf("iterate topology edges: %w", err)
	}
	return graph, nil
}

func (r *Repository) AddTopologyNode(ctx context.Context, n TopologyNode) (int64, error) {
	if n.DeviceID <= 0 {
		return 0, fmt.Errorf("device_id is required")
	}
	const q = `
		INSERT INTO topology_nodes(device_id, label, x, y)
		VALUES($1, NULLIF($2,''), $3, $4)
		ON CONFLICT (device_id) DO UPDATE
		SET label = COALESCE(NULLIF(EXCLUDED.label,''), topology_nodes.label),
		    x = EXCLUDED.x,
		    y = EXCLUDED.y,
		    updated_at = NOW()
		RETURNING id;
	`
	var id int64
	if err := r.db.QueryRowContext(ctx, q, n.DeviceID, strings.TrimSpace(n.Label), n.X, n.Y).Scan(&id); err != nil {
		return 0, fmt.Errorf("add topology node: %w", err)
	}
	return id, nil
}

func (r *Repository) UpdateTopologyNode(ctx context.Context, n TopologyNode) error {
	if n.ID <= 0 {
		return fmt.Errorf("invalid topology node id")
	}
	const q = `
		UPDATE topology_nodes
		SET label = NULLIF($2,''),
		    x = $3,
		    y = $4,
		    updated_at = NOW()
		WHERE id = $1;
	`
	res, err := r.db.ExecContext(ctx, q, n.ID, strings.TrimSpace(n.Label), n.X, n.Y)
	if err != nil {
		return fmt.Errorf("update topology node: %w", err)
	}
	if affected, _ := res.RowsAffected(); affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *Repository) DeleteTopologyNode(ctx context.Context, id int64) error {
	if id <= 0 {
		return fmt.Errorf("invalid topology node id")
	}
	_, err := r.db.ExecContext(ctx, `DELETE FROM topology_nodes WHERE id=$1;`, id)
	if err != nil {
		return fmt.Errorf("delete topology node: %w", err)
	}
	return nil
}

func (r *Repository) AddTopologyEdge(ctx context.Context, e TopologyEdge) (int64, error) {
	if e.SourceNodeID <= 0 || e.TargetNodeID <= 0 {
		return 0, fmt.Errorf("source_node_id and target_node_id are required")
	}
	if e.SourceNodeID == e.TargetNodeID {
		return 0, fmt.Errorf("source and target nodes cannot be the same")
	}
	const q = `
		INSERT INTO topology_edges(source_node_id, target_node_id, label, remark)
		VALUES($1, $2, NULLIF($3,''), NULLIF($4,''))
		ON CONFLICT (source_node_id, target_node_id) DO UPDATE
		SET label = COALESCE(NULLIF(EXCLUDED.label,''), topology_edges.label),
		    remark = COALESCE(NULLIF(EXCLUDED.remark,''), topology_edges.remark),
		    updated_at = NOW()
		RETURNING id;
	`
	var id int64
	if err := r.db.QueryRowContext(ctx, q, e.SourceNodeID, e.TargetNodeID, strings.TrimSpace(e.Label), strings.TrimSpace(e.Remark)).Scan(&id); err != nil {
		return 0, fmt.Errorf("add topology edge: %w", err)
	}
	return id, nil
}

func (r *Repository) UpdateTopologyEdge(ctx context.Context, e TopologyEdge) error {
	if e.ID <= 0 {
		return fmt.Errorf("invalid topology edge id")
	}
	if e.SourceNodeID <= 0 || e.TargetNodeID <= 0 {
		return fmt.Errorf("source_node_id and target_node_id are required")
	}
	if e.SourceNodeID == e.TargetNodeID {
		return fmt.Errorf("source and target nodes cannot be the same")
	}
	const q = `
		UPDATE topology_edges
		SET source_node_id=$2,
		    target_node_id=$3,
		    label=NULLIF($4,''),
		    remark=NULLIF($5,''),
		    updated_at=NOW()
		WHERE id=$1;
	`
	res, err := r.db.ExecContext(ctx, q, e.ID, e.SourceNodeID, e.TargetNodeID, strings.TrimSpace(e.Label), strings.TrimSpace(e.Remark))
	if err != nil {
		return fmt.Errorf("update topology edge: %w", err)
	}
	if affected, _ := res.RowsAffected(); affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *Repository) DeleteTopologyEdge(ctx context.Context, id int64) error {
	if id <= 0 {
		return fmt.Errorf("invalid topology edge id")
	}
	_, err := r.db.ExecContext(ctx, `DELETE FROM topology_edges WHERE id=$1;`, id)
	if err != nil {
		return fmt.Errorf("delete topology edge: %w", err)
	}
	return nil
}
