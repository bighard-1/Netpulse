package api

import (
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"netpulse/internal/db"
)

func RunBackupDrill(ctx context.Context, system *SystemService, repo *db.Repository) error {
	drillCtx, cancel := context.WithTimeout(ctx, 8*time.Minute)
	defer cancel()
	file, name, err := system.Backup(drillCtx)
	if err != nil {
		_ = repo.SaveBackupDrillReport(ctx, "failed", "backup failed", `{"error":"backup"}`)
		return err
	}
	defer func() { _ = os.Remove(file) }()
	raw, err := os.ReadFile(file)
	if err != nil {
		_ = repo.SaveBackupDrillReport(ctx, "failed", "read backup failed", `{"error":"read"}`)
		return err
	}
	gzr, err := gzip.NewReader(bytes.NewReader(raw))
	if err != nil {
		_ = repo.SaveBackupDrillReport(ctx, "failed", "gzip parse failed", `{"error":"gzip"}`)
		return err
	}
	plain, _ := io.ReadAll(gzr)
	_ = gzr.Close()
	ok := strings.Contains(strings.ToUpper(string(plain)), "CREATE TABLE")
	status := "ok"
	msg := "backup validation passed"
	if !ok {
		status = "failed"
		msg = "backup content check failed"
	}
	_ = repo.SaveBackupDrillReport(ctx, status, msg, fmt.Sprintf(`{"file":"%s","size":%d}`, name, len(raw)))
	return nil
}

func StartBackupDrillLoop(ctx context.Context, system *SystemService, repo *db.Repository, every time.Duration) {
	if every <= 0 {
		return
	}
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			_ = RunBackupDrill(ctx, system, repo)
		}
	}
}
