package api

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
)

type SystemJob struct {
	ID         string    `json:"id"`
	Type       string    `json:"type"`
	Status     string    `json:"status"`
	Progress   int       `json:"progress"`
	Message    string    `json:"message"`
	Error      string    `json:"error,omitempty"`
	FilePath   string    `json:"-"`
	FileName   string    `json:"file_name,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
	StartedAt  time.Time `json:"started_at,omitempty"`
	FinishedAt time.Time `json:"finished_at,omitempty"`
}

func newJobID() string {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("job_%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(buf)
}

func (h *Handler) createSystemJob(jobType, message string) *SystemJob {
	now := time.Now()
	job := &SystemJob{
		ID:        newJobID(),
		Type:      jobType,
		Status:    "queued",
		Progress:  0,
		Message:   message,
		CreatedAt: now,
	}
	h.jobsMu.Lock()
	if h.jobs == nil {
		h.jobs = map[string]*SystemJob{}
	}
	h.cleanupOldJobsLocked(now)
	h.jobs[job.ID] = job
	h.jobsMu.Unlock()
	return job
}

func (h *Handler) updateSystemJob(id string, fn func(*SystemJob)) {
	h.jobsMu.Lock()
	defer h.jobsMu.Unlock()
	if job := h.jobs[id]; job != nil {
		fn(job)
	}
}

func (h *Handler) getSystemJob(id string) (*SystemJob, bool) {
	h.jobsMu.Lock()
	defer h.jobsMu.Unlock()
	job, ok := h.jobs[id]
	if !ok || job == nil {
		return nil, false
	}
	copyJob := *job
	return &copyJob, true
}

func (h *Handler) listSystemJobs(limit int) []*SystemJob {
	if limit <= 0 || limit > 200 {
		limit = 30
	}
	h.jobsMu.Lock()
	defer h.jobsMu.Unlock()
	items := make([]*SystemJob, 0, len(h.jobs))
	for _, job := range h.jobs {
		if job == nil {
			continue
		}
		copyJob := *job
		items = append(items, &copyJob)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].CreatedAt.After(items[j].CreatedAt) })
	if len(items) > limit {
		items = items[:limit]
	}
	return items
}

func (h *Handler) cleanupOldJobsLocked(now time.Time) {
	if len(h.jobs) <= 200 {
		return
	}
	for id, job := range h.jobs {
		if job == nil {
			delete(h.jobs, id)
			continue
		}
		if job.FinishedAt.IsZero() {
			continue
		}
		if now.Sub(job.FinishedAt) > 24*time.Hour {
			if job.FilePath != "" {
				_ = os.Remove(job.FilePath)
			}
			delete(h.jobs, id)
		}
	}
}

func (h *Handler) markJobRunning(id, message string) {
	h.updateSystemJob(id, func(job *SystemJob) {
		job.Status = "running"
		job.Progress = 10
		job.Message = message
		job.StartedAt = time.Now()
	})
}

func (h *Handler) markJobFailed(id string, err error) {
	msg := "任务执行失败"
	if err != nil {
		msg = err.Error()
	}
	h.updateSystemJob(id, func(job *SystemJob) {
		job.Status = "failed"
		job.Progress = 100
		job.Message = msg
		job.Error = msg
		job.FinishedAt = time.Now()
	})
}

func (h *Handler) markJobDone(id, message string) {
	h.updateSystemJob(id, func(job *SystemJob) {
		job.Status = "completed"
		job.Progress = 100
		job.Message = message
		job.FinishedAt = time.Now()
	})
}

func (h *Handler) handleStartBackupJob(w http.ResponseWriter, r *http.Request) {
	job := h.createSystemJob("backup", "备份任务已排队")
	go func(jobID string) {
		h.markJobRunning(jobID, "正在导出数据库备份")
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		defer cancel()
		path, filename, err := h.system.Backup(ctx)
		if err != nil {
			h.markJobFailed(jobID, err)
			return
		}
		h.updateSystemJob(jobID, func(job *SystemJob) {
			job.FilePath = path
			job.FileName = filename
			job.Progress = 90
			job.Message = "备份文件已生成，等待下载"
		})
		h.markJobDone(jobID, "备份完成")
	}(job.ID)
	writeJSON(w, http.StatusAccepted, job)
}

func (h *Handler) handleDownloadBackupJob(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	job, ok := h.getSystemJob(id)
	if !ok {
		writeError(w, http.StatusNotFound, "backup job not found")
		return
	}
	if job.Type != "backup" || job.Status != "completed" || job.FilePath == "" {
		writeError(w, http.StatusConflict, "backup job is not ready")
		return
	}
	f, err := os.Open(job.FilePath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("open backup file: %v", err))
		return
	}
	defer f.Close()
	filename := job.FileName
	if filename == "" {
		filename = filepath.Base(job.FilePath)
	}
	w.Header().Set("Content-Type", "application/gzip")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	if stat, statErr := f.Stat(); statErr == nil {
		w.Header().Set("Content-Length", strconv.FormatInt(stat.Size(), 10))
	}
	http.ServeContent(w, r, filename, time.Now(), f)
}

func (h *Handler) handleListSystemJobs(w http.ResponseWriter, r *http.Request) {
	limit := 30
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			limit = n
		}
	}
	writeJSON(w, http.StatusOK, h.listSystemJobs(limit))
}

func (h *Handler) handleGetSystemJob(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	job, ok := h.getSystemJob(id)
	if !ok {
		writeError(w, http.StatusNotFound, "system job not found")
		return
	}
	writeJSON(w, http.StatusOK, job)
}

func (h *Handler) handleStartRestoreJob(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(512 << 20); err != nil {
		writeError(w, http.StatusBadRequest, "invalid multipart form")
		return
	}
	file, _, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "missing file field")
		return
	}
	defer file.Close()

	header := make([]byte, 2)
	n, err := io.ReadFull(file, header)
	if err != nil || n != 2 {
		writeError(w, http.StatusBadRequest, "invalid gzip file")
		return
	}
	if header[0] != 0x1f || header[1] != 0x8b {
		writeError(w, http.StatusBadRequest, "restore file must be .sql.gz")
		return
	}

	job := h.createSystemJob("restore", "恢复任务已排队")
	tmpPath := filepath.Join(os.TempDir(), fmt.Sprintf("netpulse_restore_%s.sql.gz", job.ID))
	out, err := os.Create(tmpPath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("create restore temp file: %v", err))
		return
	}
	if _, err := io.Copy(out, io.MultiReader(bytes.NewReader(header), file)); err != nil {
		_ = out.Close()
		_ = os.Remove(tmpPath)
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("save restore file: %v", err))
		return
	}
	if err := out.Close(); err != nil {
		_ = os.Remove(tmpPath)
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("close restore file: %v", err))
		return
	}

	go func(jobID, path string) {
		defer os.Remove(path)
		h.markJobRunning(jobID, "正在恢复数据库")
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		defer cancel()
		f, err := os.Open(path)
		if err != nil {
			h.markJobFailed(jobID, err)
			return
		}
		defer f.Close()
		if err := h.system.Restore(ctx, f); err != nil {
			h.markJobFailed(jobID, err)
			return
		}
		if err := h.repo.EnsureSchema(); err != nil {
			h.markJobFailed(jobID, err)
			return
		}
		h.markJobDone(jobID, "恢复完成")
	}(job.ID, tmpPath)

	writeJSON(w, http.StatusAccepted, job)
}

func (h *Handler) handleStartBackupDrillJob(w http.ResponseWriter, r *http.Request) {
	job := h.createSystemJob("backup_drill", "备份演练任务已排队")
	go func(jobID string) {
		h.markJobRunning(jobID, "正在执行备份可恢复性演练")
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
		defer cancel()
		heartbeatDone := make(chan struct{})
		go func() {
			ticker := time.NewTicker(5 * time.Second)
			defer ticker.Stop()
			progress := 15
			for {
				select {
				case <-heartbeatDone:
					return
				case <-ctx.Done():
					return
				case <-ticker.C:
					if progress < 45 {
						progress += 5
					}
					h.updateSystemJob(jobID, func(job *SystemJob) {
						if job.Status == "running" && job.Progress < 50 {
							job.Progress = progress
							job.Message = "正在生成轻量演练备份文件，请稍候"
						}
					})
				}
			}
		}()
		progress := func(percent int, message string) {
			if percent < 10 {
				percent = 10
			}
			if percent > 99 {
				percent = 99
			}
			h.updateSystemJob(jobID, func(job *SystemJob) {
				job.Progress = percent
				job.Message = message
			})
		}
		if err := RunBackupDrillWithProgress(ctx, h.system, h.repo, progress); err != nil {
			close(heartbeatDone)
			h.markJobFailed(jobID, err)
			return
		}
		close(heartbeatDone)
		h.markJobDone(jobID, "备份演练完成")
	}(job.ID)
	writeJSON(w, http.StatusAccepted, job)
}
