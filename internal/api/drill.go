package api

import (
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
	return RunBackupDrillWithProgress(ctx, system, repo, nil)
}

type BackupDrillProgress func(progress int, message string)

func RunBackupDrillWithProgress(ctx context.Context, system *SystemService, repo *db.Repository, progress BackupDrillProgress) error {
	reportProgress := func(p int, msg string) {
		if progress != nil {
			progress(p, msg)
		}
	}
	drillCtx, cancel := context.WithTimeout(ctx, 8*time.Minute)
	defer cancel()
	reportProgress(15, "正在生成临时备份文件")
	file, name, err := system.Backup(drillCtx)
	if err != nil {
		_ = repo.SaveBackupDrillReport(ctx, "failed", "backup failed", `{"error":"backup"}`)
		return err
	}
	defer func() { _ = os.Remove(file) }()
	reportProgress(55, "备份文件已生成，正在读取文件")
	f, err := os.Open(file)
	if err != nil {
		_ = repo.SaveBackupDrillReport(ctx, "failed", "read backup failed", `{"error":"read"}`)
		return err
	}
	defer f.Close()

	stat, _ := f.Stat()
	gzr, err := gzip.NewReader(f)
	if err != nil {
		_ = repo.SaveBackupDrillReport(ctx, "failed", "gzip parse failed", `{"error":"gzip"}`)
		return err
	}
	reportProgress(70, "gzip 校验通过，正在扫描 SQL 内容")
	ok, scanErr := backupContainsCreateTable(ctx, gzr)
	closeErr := gzr.Close()
	if scanErr != nil {
		_ = repo.SaveBackupDrillReport(ctx, "failed", "backup scan failed", fmt.Sprintf(`{"error":"scan","detail":%q}`, scanErr.Error()))
		return scanErr
	}
	if closeErr != nil {
		_ = repo.SaveBackupDrillReport(ctx, "failed", "gzip close failed", fmt.Sprintf(`{"error":"gzip_close","detail":%q}`, closeErr.Error()))
		return closeErr
	}
	status := "ok"
	msg := "backup validation passed"
	if !ok {
		status = "failed"
		msg = "backup content check failed"
	}
	size := int64(0)
	if stat != nil {
		size = stat.Size()
	}
	reportProgress(90, "备份内容校验完成，正在写入演练报告")
	_ = repo.SaveBackupDrillReport(ctx, status, msg, fmt.Sprintf(`{"file":"%s","size":%d}`, name, size))
	return nil
}

func backupContainsCreateTable(ctx context.Context, r io.Reader) (bool, error) {
	buf := make([]byte, 64*1024)
	var tail string
	const needle = "CREATE TABLE"
	for {
		select {
		case <-ctx.Done():
			return false, ctx.Err()
		default:
		}
		n, err := r.Read(buf)
		if n > 0 {
			chunk := strings.ToUpper(tail + string(buf[:n]))
			if strings.Contains(chunk, needle) {
				return true, nil
			}
			if len(chunk) > len(needle) {
				tail = chunk[len(chunk)-len(needle):]
			} else {
				tail = chunk
			}
		}
		if err == io.EOF {
			return false, nil
		}
		if err != nil {
			return false, err
		}
	}
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
