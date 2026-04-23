// Copyright 2024 The kflashback Authors
// SPDX-License-Identifier: Apache-2.0

package server

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/gorilla/mux"

	"github.com/kflashback/kflashback/internal/ai"
	"github.com/kflashback/kflashback/internal/diff"
	"github.com/kflashback/kflashback/internal/storage"
)

// registerAIRoutes registers AI-powered API routes.
// Routes are always registered but return 503 if AI is not configured.
func (s *Server) registerAIRoutes(api *mux.Router) {
	api.HandleFunc("/ai/summarize/{uid}/revisions/{revision}", s.requireAI(s.handleAISummarize)).Methods("GET", "OPTIONS")
	api.HandleFunc("/ai/summarize/{uid}/diff", s.requireAI(s.handleAISummarizeDiff)).Methods("GET", "OPTIONS")
	api.HandleFunc("/ai/anomalies", s.requireAI(s.handleAIAnomalies)).Methods("GET", "OPTIONS")
	api.HandleFunc("/ai/query", s.requireAI(s.handleAIQuery)).Methods("POST", "OPTIONS")
}

// requireAI wraps a handler and returns 503 if AI is not configured.
func (s *Server) requireAI(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.ai == nil {
			writeError(w, http.StatusServiceUnavailable, "AI features are not enabled. Configure the AI provider in KFlashbackConfig or use --ai-enabled flag.")
			return
		}
		next(w, r)
	}
}

// handleAISummarize generates a human-readable summary of a specific revision's changes.
func (s *Server) handleAISummarize(w http.ResponseWriter, r *http.Request) {
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

	// Get the patch content for the summary
	var patch json.RawMessage
	if rev.IsSnapshot && len(rev.Snapshot) > 0 {
		data := rev.Snapshot
		if decompressed, err := diff.Decompress(data); err == nil {
			data = decompressed
		}
		patch = json.RawMessage(data)
	} else if len(rev.Patch) > 0 {
		patch = json.RawMessage(rev.Patch)
	}

	summarizer := ai.NewSummarizer(s.ai)
	summary, err := summarizer.SummarizeChange(r.Context(), rev.Kind, rev.Name, rev.Namespace, string(rev.EventType), patch)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "AI summarization failed: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, apiResponse{Data: map[string]interface{}{
		"revision": revNum,
		"summary":  summary,
	}})
}

// handleAISummarizeDiff generates a summary comparing two revisions.
func (s *Server) handleAISummarizeDiff(w http.ResponseWriter, r *http.Request) {
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

	// Get resource info for context
	resource, _ := s.store.GetTrackedResource(ctx, uid)
	kind, name, namespace := "Resource", uid, ""
	if resource != nil {
		kind = resource.Kind
		name = resource.Name
		namespace = resource.Namespace
	}

	summarizer := ai.NewSummarizer(s.ai)
	summary, err := summarizer.SummarizeDiff(ctx, kind, name, namespace, fromData, toData)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "AI summarization failed: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, apiResponse{Data: map[string]interface{}{
		"fromRevision": fromRev,
		"toRevision":   toRev,
		"summary":      summary,
	}})
}

// handleAIAnomalies detects anomalies in recent changes.
func (s *Server) handleAIAnomalies(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	hours := parseIntDefault(r.URL.Query().Get("hours"), 24)
	limit := parseIntDefault(r.URL.Query().Get("limit"), 100)

	since := time.Now().Add(-time.Duration(hours) * time.Hour)
	revisions, _, err := s.store.GetHistory(ctx, storage.ResourceHistoryQuery{
		Since: &since,
		Limit: limit,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Build compact change list for analysis
	events := make([]ai.ChangeEvent, len(revisions))
	for i, rev := range revisions {
		events[i] = ai.ChangeEvent{
			Kind:      rev.Kind,
			Name:      rev.Name,
			Namespace: rev.Namespace,
			Revision:  rev.Revision,
			EventType: string(rev.EventType),
			Timestamp: rev.Timestamp,
		}
	}

	detector := ai.NewAnomalyDetector(s.ai)
	report, err := detector.AnalyzeChanges(ctx, events)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "AI anomaly detection failed: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, apiResponse{Data: report})
}

// handleAIQuery answers natural language questions about cluster changes.
func (s *Server) handleAIQuery(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Question string `json:"question"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Question == "" {
		writeError(w, http.StatusBadRequest, "question is required")
		return
	}
	if len(req.Question) > 2000 {
		writeError(w, http.StatusBadRequest, "question too long (max 2000 characters)")
		return
	}

	// Validate the question is relevant and not a prompt injection
	if err := ai.ValidateQuestion(req.Question); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	engine := ai.NewQueryEngine(s.ai, s.store, s.aiContextMode)
	result, err := engine.Ask(r.Context(), req.Question)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "AI query failed: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, apiResponse{Data: result})
}
