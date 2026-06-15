package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"netpulse/internal/db"
)

const topologyCacheTTL = 30 * time.Second

type topologyNodeRequest struct {
	DeviceID int64   `json:"device_id"`
	Label    string  `json:"label"`
	X        float64 `json:"x"`
	Y        float64 `json:"y"`
}

type topologyEdgeRequest struct {
	SourceNodeID int64  `json:"source_node_id"`
	TargetNodeID int64  `json:"target_node_id"`
	Label        string `json:"label"`
	Remark       string `json:"remark"`
}

func (h *Handler) handleGetTopology(w http.ResponseWriter, r *http.Request) {
	if graph, ok := h.getCachedTopology(); ok {
		writeJSON(w, http.StatusOK, graph)
		return
	}
	graph, err := h.repo.GetTopologyGraph(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.setCachedTopology(graph)
	writeJSON(w, http.StatusOK, graph)
}

func (h *Handler) getCachedTopology() (*db.TopologyGraph, bool) {
	h.cacheMu.Lock()
	defer h.cacheMu.Unlock()
	if h.topology.graph == nil || time.Now().After(h.topology.expiresAt) {
		h.cacheStats.TopologyMiss++
		return nil, false
	}
	h.cacheStats.TopologyHits++
	return h.topology.graph, true
}

func (h *Handler) setCachedTopology(graph *db.TopologyGraph) {
	h.cacheMu.Lock()
	h.topology = topologyCacheEntry{graph: graph, expiresAt: time.Now().Add(topologyCacheTTL)}
	h.cacheMu.Unlock()
}

func (h *Handler) invalidateTopologyCache() {
	h.cacheMu.Lock()
	h.topology = topologyCacheEntry{}
	h.cacheMu.Unlock()
}

func (h *Handler) handleCreateTopologyNode(w http.ResponseWriter, r *http.Request) {
	var req topologyNodeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	id, err := h.repo.AddTopologyNode(r.Context(), db.TopologyNode{
		DeviceID: req.DeviceID,
		Label:    req.Label,
		X:        req.X,
		Y:        req.Y,
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	h.invalidateTopologyCache()
	writeJSON(w, http.StatusOK, map[string]any{"id": id})
}

func (h *Handler) handleUpdateTopologyNode(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "invalid topology node id")
		return
	}
	var req topologyNodeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	err = h.repo.UpdateTopologyNode(r.Context(), db.TopologyNode{ID: id, Label: req.Label, X: req.X, Y: req.Y})
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "topology node not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	h.invalidateTopologyCache()
	writeJSON(w, http.StatusOK, map[string]any{"message": "topology node updated"})
}

func (h *Handler) handleDeleteTopologyNode(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "invalid topology node id")
		return
	}
	if err := h.repo.DeleteTopologyNode(r.Context(), id); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	h.invalidateTopologyCache()
	writeJSON(w, http.StatusOK, map[string]any{"message": "topology node deleted"})
}

func (h *Handler) handleCreateTopologyEdge(w http.ResponseWriter, r *http.Request) {
	var req topologyEdgeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	id, err := h.repo.AddTopologyEdge(r.Context(), db.TopologyEdge{
		SourceNodeID: req.SourceNodeID,
		TargetNodeID: req.TargetNodeID,
		Label:        req.Label,
		Remark:       req.Remark,
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	h.invalidateTopologyCache()
	writeJSON(w, http.StatusOK, map[string]any{"id": id})
}

func (h *Handler) handleUpdateTopologyEdge(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "invalid topology edge id")
		return
	}
	var req topologyEdgeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	err = h.repo.UpdateTopologyEdge(r.Context(), db.TopologyEdge{
		ID:           id,
		SourceNodeID: req.SourceNodeID,
		TargetNodeID: req.TargetNodeID,
		Label:        req.Label,
		Remark:       req.Remark,
	})
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "topology edge not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	h.invalidateTopologyCache()
	writeJSON(w, http.StatusOK, map[string]any{"message": "topology edge updated"})
}

func (h *Handler) handleDeleteTopologyEdge(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "invalid topology edge id")
		return
	}
	if err := h.repo.DeleteTopologyEdge(r.Context(), id); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	h.invalidateTopologyCache()
	writeJSON(w, http.StatusOK, map[string]any{"message": "topology edge deleted"})
}
