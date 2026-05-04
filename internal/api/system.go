package api

import (
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

type DBConfig struct {
	Host     string
	Port     string
	User     string
	Password string
	Name     string
}

type SystemService struct {
	db DBConfig
}

func NewSystemService(db DBConfig) *SystemService {
	return &SystemService{db: db}
}

func (s *SystemService) Backup(ctx context.Context) (string, string, error) {
	ts := time.Now().Format("20060102_150405")
	filename := fmt.Sprintf("netpulse_backup_%s.sql.gz", ts)
	path := filepath.Join(os.TempDir(), filename)

	outFile, err := os.Create(path)
	if err != nil {
		return "", "", fmt.Errorf("create backup file: %w", err)
	}
	defer outFile.Close()

	gz := gzip.NewWriter(outFile)
	defer gz.Close()

	cmd := exec.CommandContext(
		ctx,
		"pg_dump",
		"-h", s.db.Host,
		"-p", s.db.Port,
		"-U", s.db.User,
		"-d", s.db.Name,
		"--no-owner",
		"--no-privileges",
	)
	cmd.Env = append(os.Environ(), "PGPASSWORD="+s.db.Password)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", "", fmt.Errorf("pg_dump stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return "", "", fmt.Errorf("pg_dump stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return "", "", fmt.Errorf("start pg_dump: %w", err)
	}

	if _, err := io.Copy(gz, stdout); err != nil {
		return "", "", fmt.Errorf("stream pg_dump to gzip: %w", err)
	}
	if err := gz.Close(); err != nil {
		return "", "", fmt.Errorf("close gzip writer: %w", err)
	}

	errOut, _ := io.ReadAll(stderr)
	if err := cmd.Wait(); err != nil {
		return "", "", fmt.Errorf("pg_dump failed: %v (%s)", err, string(errOut))
	}

	return path, filename, nil
}

func (s *SystemService) Restore(ctx context.Context, gzSQL io.Reader) error {
	psql := exec.CommandContext(
		ctx,
		"psql",
		"-h", s.db.Host,
		"-p", s.db.Port,
		"-U", s.db.User,
		"-d", s.db.Name,
		"-v", "ON_ERROR_STOP=1",
	)
	psql.Env = append(os.Environ(), "PGPASSWORD="+s.db.Password)

	stdin, err := psql.StdinPipe()
	if err != nil {
		return fmt.Errorf("psql stdin pipe: %w", err)
	}
	stderr, err := psql.StderrPipe()
	if err != nil {
		return fmt.Errorf("psql stderr pipe: %w", err)
	}

	if err := psql.Start(); err != nil {
		return fmt.Errorf("start psql: %w", err)
	}

	gzr, err := gzip.NewReader(gzSQL)
	if err != nil {
		_ = stdin.Close()
		return fmt.Errorf("open gzip stream: %w", err)
	}
	defer gzr.Close()

	resetSQL := "DROP SCHEMA IF EXISTS public CASCADE;\nCREATE SCHEMA public;\n"
	if _, err := io.WriteString(stdin, resetSQL); err != nil {
		_ = stdin.Close()
		return fmt.Errorf("write schema reset sql: %w", err)
	}

	if _, err := io.Copy(stdin, gzr); err != nil {
		_ = stdin.Close()
		return fmt.Errorf("pipe restore sql: %w", err)
	}
	if err := stdin.Close(); err != nil {
		return fmt.Errorf("close psql stdin: %w", err)
	}

	errOut, _ := io.ReadAll(stderr)
	if err := psql.Wait(); err != nil {
		return fmt.Errorf("psql restore failed: %v (%s)", err, string(errOut))
	}

	return nil
}
