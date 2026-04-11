// Copyright 2024 The kflashback Authors
// SPDX-License-Identifier: Apache-2.0

package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gorilla/mux"

	"github.com/kflashback/kflashback/internal/diff"
	"github.com/kflashback/kflashback/internal/storage"
)

// --- Response types ---

type apiResponse struct {
	Data  interface{} `json:"data,omitempty"`
	Error string      `json:"error,omitempty"`
	Meta  *apiMeta    `json:"meta,omitempty"`
}

type apiMeta struct {
	Total  int64 `json:"total"`
	Limit  int   `json:"limit"`
	Offset int   `json:"offset"`
}

type diffResponse struct {
	FromRevision int64           `json:"fromRevision"`
	ToRevision   int64           `json:"toRevision"`
	Patch        json.RawMessage `json:"patch"`
	ChangedPaths []string        `json:"changedPaths"`
	FromSnapshot json.RawMessage `json:"fromSnapshot,omitempty"`
	ToSnapshot   json.RawMessage `json:"toSnapshot,omitempty"`
}

type revisionResponse struct {
	ID              int64           `json:"id"`
	ResourceUID     string          `json:"resourceUid"`
	APIVersion      string          `json:"apiVersion"`
	Kind            string          `json:"kind"`
	Namespace       string          `json:"namespace"`
	Name            string          `json:"name"`
	Revision        int64           `json:"revision"`
	EventType       string          `json:"eventType"`
	IsSnapshot      bool            `json:"isSnapshot"`
	ResourceVersion string          `json:"resourceVersion"`
	ChangedFields   []string        `json:"changedFields,omitempty"`
	Timestamp       time.Time       `json:"timestamp"`
	PolicyName      string          `json:"policyName"`
	SizeBytes       int64           `json:"sizeBytes"`
	Content         json.RawMessage `json:"content,omitempty"`
}

// --- Handlers ---

func (s *Server) handleGetStats(w http.ResponseWriter, r *http.Request) {
	stats, err := s.store.GetStats(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, apiResponse{Data: stats})
}

func (s *Server) handleGetKindStats(w http.ResponseWriter, r *http.Request) {
	stats, err := s.store.GetKindStats(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, apiResponse{Data: stats})
}

func (s *Server) handleListResources(w http.ResponseWriter, r *http.Request) {
	query := storage.ResourceListQuery{
		APIVersion: r.URL.Query().Get("apiVersion"),
		Kind:       r.URL.Query().Get("kind"),
		Namespace:  r.URL.Query().Get("namespace"),
		PolicyName: r.URL.Query().Get("policy"),
		Limit:      parseIntDefault(r.URL.Query().Get("limit"), 100),
		Offset:     parseIntDefault(r.URL.Query().Get("offset"), 0),
	}

	if deleted := r.URL.Query().Get("deleted"); deleted != "" {
		d := deleted == "true"
		query.IsDeleted = &d
	}

	resources, total, err := s.store.ListTrackedResources(r.Context(), query)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, apiResponse{
		Data: resources,
		Meta: &apiMeta{Total: total, Limit: query.Limit, Offset: query.Offset},
	})
}

func (s *Server) handleGetResource(w http.ResponseWriter, r *http.Request) {
	uid := mux.Vars(r)["uid"]
	resource, err := s.store.GetTrackedResource(r.Context(), uid)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if resource == nil {
		writeError(w, http.StatusNotFound, "resource not found")
		return
	}
	writeJSON(w, http.StatusOK, apiResponse{Data: resource})
}

func (s *Server) handleGetHistory(w http.ResponseWriter, r *http.Request) {
	uid := mux.Vars(r)["uid"]
	query := storage.ResourceHistoryQuery{
		UID:    uid,
		Limit:  parseIntDefault(r.URL.Query().Get("limit"), 50),
		Offset: parseIntDefault(r.URL.Query().Get("offset"), 0),
	}

	if eventType := r.URL.Query().Get("eventType"); eventType != "" {
		query.EventType = eventType
	}
	if since := r.URL.Query().Get("since"); since != "" {
		t, err := time.Parse(time.RFC3339, since)
		if err == nil {
			query.Since = &t
		}
	}
	if until := r.URL.Query().Get("until"); until != "" {
		t, err := time.Parse(time.RFC3339, until)
		if err == nil {
			query.Until = &t
		}
	}

	revisions, total, err := s.store.GetHistory(r.Context(), query)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Convert to response objects (strip large binary data from list)
	responses := make([]revisionResponse, len(revisions))
	for i, rev := range revisions {
		responses[i] = toRevisionResponse(rev, false)
	}

	writeJSON(w, http.StatusOK, apiResponse{
		Data: responses,
		Meta: &apiMeta{Total: total, Limit: query.Limit, Offset: query.Offset},
	})
}

func (s *Server) handleGetRevision(w http.ResponseWriter, r *http.Request) {
	uid := mux.Vars(r)["uid"]
	revNum, err := strconv.ParseInt(mux.Vars(r)["revision"], 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid revision number")
		return
	}

	rev, err := s.store.GetRevision(r.Context(), uid, revNum)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if rev == nil {
		writeError(w, http.StatusNotFound, "revision not found")
		return
	}

	writeJSON(w, http.StatusOK, apiResponse{Data: toRevisionResponse(*rev, true)})
}

func (s *Server) handleReconstructAtRevision(w http.ResponseWriter, r *http.Request) {
	uid := mux.Vars(r)["uid"]
	revNum, err := strconv.ParseInt(mux.Vars(r)["revision"], 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid revision number")
		return
	}

	data, err := s.reconstructResource(r.Context(), uid, revNum)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, apiResponse{Data: json.RawMessage(data)})
}

func (s *Server) handleDiffRevisions(w http.ResponseWriter, r *http.Request) {
	uid := mux.Vars(r)["uid"]
	fromRev, err := strconv.ParseInt(r.URL.Query().Get("from"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid 'from' revision")
		return
	}
	toRev, err := strconv.ParseInt(r.URL.Query().Get("to"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid 'to' revision")
		return
	}

	ctx := r.Context()

	fromData, err := s.reconstructResource(ctx, uid, fromRev)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to reconstruct 'from' revision: "+err.Error())
		return
	}

	toData, err := s.reconstructResource(ctx, uid, toRev)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to reconstruct 'to' revision: "+err.Error())
		return
	}

	patch, err := diff.ComputeMergePatch(fromData, toData)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to compute diff: "+err.Error())
		return
	}

	changedPaths, _ := diff.GetChangedPaths(fromData, toData)

	resp := diffResponse{
		FromRevision: fromRev,
		ToRevision:   toRev,
		Patch:        json.RawMessage(patch),
		ChangedPaths: changedPaths,
		FromSnapshot: json.RawMessage(fromData),
		ToSnapshot:   json.RawMessage(toData),
	}

	writeJSON(w, http.StatusOK, apiResponse{Data: resp})
}

// reconstructResource rebuilds the full resource JSON at a given revision.
func (s *Server) reconstructResource(ctx context.Context, uid string, revision int64) ([]byte, error) {
	// Find the nearest snapshot at or before this revision
	snapshot, err := s.store.GetNearestSnapshot(ctx, uid, revision)
	if err != nil {
		return nil, err
	}
	if snapshot == nil {
		return nil, fmt.Errorf("no snapshot found for resource %s at or before revision %d", uid, revision)
	}

	// Decompress snapshot if needed
	base := snapshot.Snapshot
	if decompressed, err := diff.Decompress(base); err == nil {
		base = decompressed
	}

	// If the snapshot IS the requested revision, return it directly
	if snapshot.Revision == revision {
		return base, nil
	}

	// Get all patches between the snapshot and the target revision
	patches, err := s.store.GetPatchesBetween(ctx, uid, snapshot.Revision, revision)
	if err != nil {
		return nil, err
	}

	// Apply patches sequentially
	current := base
	for _, p := range patches {
		if p.IsSnapshot {
			// If we hit another snapshot, use it as the new base
			data := p.Snapshot
			if decompressed, err := diff.Decompress(data); err == nil {
				data = decompressed
			}
			current = data
		} else if len(p.Patch) > 0 {
			current, err = diff.ApplyMergePatch(current, p.Patch)
			if err != nil {
				return nil, fmt.Errorf("applying patch at revision %d: %w", p.Revision, err)
			}
		}
	}

	return current, nil
}

// --- Helpers ---

func toRevisionResponse(rev storage.ResourceRevision, includeContent bool) revisionResponse {
	resp := revisionResponse{
		ID:              rev.ID,
		ResourceUID:     rev.ResourceUID,
		APIVersion:      rev.APIVersion,
		Kind:            rev.Kind,
		Namespace:       rev.Namespace,
		Name:            rev.Name,
		Revision:        rev.Revision,
		EventType:       string(rev.EventType),
		IsSnapshot:      rev.IsSnapshot,
		ResourceVersion: rev.ResourceVersion,
		ChangedFields:   rev.ChangedFields,
		Timestamp:       rev.Timestamp,
		PolicyName:      rev.PolicyName,
		SizeBytes:       rev.SizeBytes,
	}

	if includeContent {
		if rev.IsSnapshot && len(rev.Snapshot) > 0 {
			data := rev.Snapshot
			if decompressed, err := diff.Decompress(data); err == nil {
				data = decompressed
			}
			resp.Content = json.RawMessage(data)
		} else if len(rev.Patch) > 0 {
			resp.Content = json.RawMessage(rev.Patch)
		}
	}

	return resp
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, apiResponse{Error: message})
}

func parseIntDefault(s string, defaultVal int) int {
	if s == "" {
		return defaultVal
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return defaultVal
	}
	return v
}
