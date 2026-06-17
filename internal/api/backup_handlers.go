package api

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"time"
)

func (h *Handler) handleSystemBackup(w http.ResponseWriter, r *http.Request) {
	backupCtx, cancel := context.WithTimeout(r.Context(), 30*time.Minute)
	defer cancel()

	filePath, filename, err := h.system.Backup(backupCtx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer os.Remove(filePath)

	f, err := os.Open(filePath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("open backup file: %v", err))
		return
	}
	defer f.Close()

	w.Header().Set("Content-Type", "application/gzip")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	if stat, statErr := f.Stat(); statErr == nil {
		w.Header().Set("Content-Length", strconv.FormatInt(stat.Size(), 10))
	}
	http.ServeContent(w, r, filename, time.Now(), f)
}

func (h *Handler) handleSystemRestore(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(256 << 20); err != nil {
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
	restoreReader := io.MultiReader(bytes.NewReader(header), file)

	restoreCtx, cancel := context.WithTimeout(r.Context(), 20*time.Minute)
	defer cancel()

	if err := h.system.Restore(restoreCtx, restoreReader); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "restore completed"})
}
