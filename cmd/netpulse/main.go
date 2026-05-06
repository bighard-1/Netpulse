package main

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"golang.org/x/crypto/bcrypt"

	"netpulse/internal/api"
	"netpulse/internal/db"
	"netpulse/internal/snmp"
)

//go:embed all:web/dist
var embeddedWebFS embed.FS

func getenv(key, fallback string) string {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	return v
}

func main() {
	host := getenv("DB_HOST", "netpulse-db")
	port := getenv("DB_PORT", "5432")
	user := getenv("DB_USER", "postgres")
	password := getenv("DB_PASSWORD", "netpulse123")
	name := getenv("DB_NAME", "netpulse")
	sslmode := getenv("DB_SSLMODE", "disable")
	adminUser := getenv("ADMIN_USERNAME", "admin")
	adminPassword := getenv("ADMIN_PASSWORD", "admin123")
	jwtSecret := getenv("JWT_SECRET", "change-this-secret")
	syslogAddr := getenv("SYSLOG_ADDR", ":514")
	trapAddr := getenv("SNMP_TRAP_ADDR", ":9162")
	backupDrillHours := getenv("BACKUP_DRILL_EVERY_HOURS", "168")
	pollIntervalSec := getenv("SNMP_POLL_INTERVAL_SEC", "60")
	onlineWindowSec := getenv("STATUS_ONLINE_WINDOW_SEC", "300")

	dsn := fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		host, port, user, password, name, sslmode,
	)

	conn, err := sql.Open("pgx", dsn)
	if err != nil {
		log.Fatalf("open database failed: %v", err)
	}
	defer conn.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := conn.PingContext(ctx); err != nil {
		log.Fatalf("database ping failed: %v", err)
	}

	repo := db.NewRepository(conn)
	if err := repo.EnsureSchema(); err != nil {
		log.Fatalf("ensure schema failed: %v", err)
	}
	if err := repo.EnsureRuntimeSettings(context.Background(), map[string]string{
		"snmp_poll_interval_sec":   pollIntervalSec,
		"snmp_device_timeout_sec":  getenv("SNMP_DEVICE_TIMEOUT_SEC", "15"),
		"status_online_window_sec": onlineWindowSec,
		"alert_cpu_threshold":      getenv("ALERT_CPU_THRESHOLD", "90"),
		"alert_mem_threshold":      getenv("ALERT_MEM_THRESHOLD", "90"),
		"alert_webhook_url":        getenv("ALERT_WEBHOOK_URL", ""),
	}); err != nil {
		log.Fatalf("ensure runtime settings failed: %v", err)
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(adminPassword), bcrypt.DefaultCost)
	if err != nil {
		log.Fatalf("generate admin hash failed: %v", err)
	}
	if err := repo.UpsertAdmin(adminUser, string(hash)); err != nil {
		log.Fatalf("upsert admin failed: %v", err)
	}

	fmt.Println("NetPulse Server Started")

	collector := snmp.NewCollector()
	worker := snmp.NewWorker(repo, collector, 60*time.Second)
	systemSvc := api.NewSystemService(api.DBConfig{
		Host:     host,
		Port:     port,
		User:     user,
		Password: password,
		Name:     name,
	})
	handler := api.NewHandler(repo, collector, systemSvc, jwtSecret)

	runCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go worker.Start(runCtx)
	go snmp.StartSyslogServer(runCtx, repo, syslogAddr)
	go snmp.StartTrapServer(runCtx, repo, trapAddr)
	if h, err := time.ParseDuration(backupDrillHours + "h"); err == nil {
		go api.StartBackupDrillLoop(runCtx, systemSvc, repo, h)
	}

	distFS, err := fs.Sub(embeddedWebFS, "web/dist")
	if err != nil {
		log.Fatalf("load embedded web dist failed: %v", err)
	}
	staticHandler := http.FileServer(http.FS(distFS))

	rootMux := http.NewServeMux()
	rootMux.Handle("/api/", handler.Router())
	rootMux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			_, _ = fs.Stat(distFS, "index.html")
			http.ServeFileFS(w, r, distFS, "index.html")
			return
		}
		if _, err := fs.Stat(distFS, r.URL.Path[1:]); err == nil {
			staticHandler.ServeHTTP(w, r)
			return
		}
		http.ServeFileFS(w, r, distFS, "index.html")
	})

	srv := &http.Server{
		Addr:              ":8080",
		Handler:           rootMux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() {
		log.Println("HTTP server listening on :8080")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("http server failed: %v", err)
		}
	}()

	<-runCtx.Done()

	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelShutdown()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("http shutdown failed: %v", err)
	}

	log.Println("NetPulse Server Stopped")
}
