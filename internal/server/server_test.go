// Copyright 2024 The kflashback Authors
// SPDX-License-Identifier: Apache-2.0

package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/kflashback/kflashback/internal/diff"
	"github.com/kflashback/kflashback/internal/storage"
	"github.com/kflashback/kflashback/internal/storage/sqlite"
)

func setupTestServer(t *testing.T) (*Server, storage.Store) {
	t.Helper()
	dir := t.TempDir()
	store, err := sqlite.New(dir + "/test.db")
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	if err := store.Initialize(context.Background()); err != nil {
		t.Fatalf("failed to initialize store: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	srv := New(store, "", ":0")
	return srv, store
}

func seedData(t *testing.T, store storage.Store) {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	snapshot := []byte(`{"kind":"Deployment","metadata":{"name":"nginx","namespace":"default"},"spec":{"replicas":1}}`)
	compressed, _ := diff.Compress(snapshot)

	_ = store.UpsertTrackedResource(ctx, &storage.TrackedResourceRecord{
		UID: "uid-1", APIVersion: "apps/v1", Kind: "Deployment", Namespace: "default", Name: "nginx",
		CurrentRevision: 2, FirstSeen: now, LastSeen: now, PolicyName: "test",
	})

	_ = store.StoreRevision(ctx, &storage.ResourceRevision{
		ResourceUID: "uid-1", APIVersion: "apps/v1", Kind: "Deployment", Namespace: "default", Name: "nginx",
		Revision: 1, EventType: storage.EventCreated, Snapshot: compressed, IsSnapshot: true,
		ResourceVersion: "100", Timestamp: now, PolicyName: "test",
	})

	_ = store.StoreRevision(ctx, &storage.ResourceRevision{
		ResourceUID: "uid-1", APIVersion: "apps/v1", Kind: "Deployment", Namespace: "default", Name: "nginx",
		Revision: 2, EventType: storage.EventUpdated, Patch: []byte(`{"spec":{"replicas":3}}`),
		ResourceVersion: "200", Timestamp: now.Add(time.Minute), PolicyName: "test",
	})
}

func TestHealthz(t *testing.T) {
	srv, _ := setupTestServer(t)
	req := httptest.NewRequest("GET", "/healthz", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if w.Body.String() != "ok" {
		t.Errorf("body = %q, want ok", w.Body.String())
	}
}

func TestReadyz(t *testing.T) {
	srv, _ := setupTestServer(t)
	req := httptest.NewRequest("GET", "/readyz", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestGetStats(t *testing.T) {
	srv, store := setupTestServer(t)
	seedData(t, store)

	req := httptest.NewRequest("GET", "/api/v1/stats", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp apiResponse
	_ = json.NewDecoder(w.Body).Decode(&resp)
	if resp.Error != "" {
		t.Errorf("error = %q", resp.Error)
	}
}

func TestGetKindStats(t *testing.T) {
	srv, store := setupTestServer(t)
	seedData(t, store)

	req := httptest.NewRequest("GET", "/api/v1/stats/kinds", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestListResources(t *testing.T) {
	srv, store := setupTestServer(t)
	seedData(t, store)

	req := httptest.NewRequest("GET", "/api/v1/resources?kind=Deployment", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp apiResponse
	_ = json.NewDecoder(w.Body).Decode(&resp)
	if resp.Meta == nil || resp.Meta.Total != 1 {
		t.Errorf("expected 1 resource, got meta=%+v", resp.Meta)
	}
}

func TestGetResource(t *testing.T) {
	srv, store := setupTestServer(t)
	seedData(t, store)

	req := httptest.NewRequest("GET", "/api/v1/resources/uid-1", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestGetResource_NotFound(t *testing.T) {
	srv, _ := setupTestServer(t)

	req := httptest.NewRequest("GET", "/api/v1/resources/nonexistent", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestGetHistory(t *testing.T) {
	srv, store := setupTestServer(t)
	seedData(t, store)

	req := httptest.NewRequest("GET", "/api/v1/resources/uid-1/history?limit=10", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp apiResponse
	_ = json.NewDecoder(w.Body).Decode(&resp)
	if resp.Meta == nil || resp.Meta.Total != 2 {
		t.Errorf("expected 2 revisions, got meta=%+v", resp.Meta)
	}
}

func TestGetRevision(t *testing.T) {
	srv, store := setupTestServer(t)
	seedData(t, store)

	req := httptest.NewRequest("GET", "/api/v1/resources/uid-1/revisions/1", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestGetRevision_InvalidNumber(t *testing.T) {
	srv, _ := setupTestServer(t)

	req := httptest.NewRequest("GET", "/api/v1/resources/uid-1/revisions/abc", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestReconstructAtRevision(t *testing.T) {
	srv, store := setupTestServer(t)
	seedData(t, store)

	req := httptest.NewRequest("GET", "/api/v1/resources/uid-1/reconstruct/2", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp apiResponse
	_ = json.NewDecoder(w.Body).Decode(&resp)
	// The reconstructed resource should have replicas=3 after applying the patch
	data, _ := json.Marshal(resp.Data)
	var obj map[string]interface{}
	_ = json.Unmarshal(data, &obj)
	spec, _ := obj["spec"].(map[string]interface{})
	if spec["replicas"] != float64(3) {
		t.Errorf("reconstructed replicas = %v, want 3", spec["replicas"])
	}
}

func TestDiffRevisions(t *testing.T) {
	srv, store := setupTestServer(t)
	seedData(t, store)

	req := httptest.NewRequest("GET", "/api/v1/resources/uid-1/diff?from=1&to=2", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp struct {
		Data diffResponse `json:"data"`
	}
	_ = json.NewDecoder(w.Body).Decode(&resp)
	if resp.Data.FromRevision != 1 || resp.Data.ToRevision != 2 {
		t.Errorf("from=%d to=%d, want from=1 to=2", resp.Data.FromRevision, resp.Data.ToRevision)
	}
	if len(resp.Data.Patch) == 0 {
		t.Error("expected non-empty patch")
	}
}

func TestDiffRevisions_BadParams(t *testing.T) {
	srv, _ := setupTestServer(t)

	req := httptest.NewRequest("GET", "/api/v1/resources/uid-1/diff?from=abc&to=2", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestListResources_WithDeletedFilter(t *testing.T) {
	srv, store := setupTestServer(t)
	seedData(t, store)

	req := httptest.NewRequest("GET", "/api/v1/resources?deleted=false", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestGetHistory_WithFilters(t *testing.T) {
	srv, store := setupTestServer(t)
	seedData(t, store)

	req := httptest.NewRequest("GET", "/api/v1/resources/uid-1/history?eventType=CREATED&since=2020-01-01T00:00:00Z&until=2099-01-01T00:00:00Z", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestGetRevision_NotFound(t *testing.T) {
	srv, store := setupTestServer(t)
	seedData(t, store)

	req := httptest.NewRequest("GET", "/api/v1/resources/uid-1/revisions/999", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestReconstructAtRevision_InvalidNumber(t *testing.T) {
	srv, _ := setupTestServer(t)

	req := httptest.NewRequest("GET", "/api/v1/resources/uid-1/reconstruct/abc", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestDiffRevisions_MissingToParam(t *testing.T) {
	srv, _ := setupTestServer(t)

	req := httptest.NewRequest("GET", "/api/v1/resources/uid-1/diff?from=1&to=abc", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestReconstructAtRevision_Snapshot(t *testing.T) {
	srv, store := setupTestServer(t)
	seedData(t, store)

	// Revision 1 is a snapshot, should return directly
	req := httptest.NewRequest("GET", "/api/v1/resources/uid-1/reconstruct/1", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}
}

func TestParseIntDefault(t *testing.T) {
	tests := []struct {
		input string
		def   int
		want  int
	}{
		{"", 10, 10},
		{"5", 10, 5},
		{"abc", 10, 10},
		{"0", 10, 0},
	}
	for _, tt := range tests {
		got := parseIntDefault(tt.input, tt.def)
		if got != tt.want {
			t.Errorf("parseIntDefault(%q, %d) = %d, want %d", tt.input, tt.def, got, tt.want)
		}
	}
}

func TestReconstructAtRevision_NoSnapshot(t *testing.T) {
	srv, _ := setupTestServer(t)

	// No data seeded, so reconstruction should fail
	req := httptest.NewRequest("GET", "/api/v1/resources/uid-missing/reconstruct/1", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
}

func TestDiffRevisions_NoData(t *testing.T) {
	srv, _ := setupTestServer(t)

	req := httptest.NewRequest("GET", "/api/v1/resources/uid-missing/diff?from=1&to=2", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
}

func TestGetHistory_DefaultLimit(t *testing.T) {
	srv, store := setupTestServer(t)
	seedData(t, store)

	// No limit param — should use default 50
	req := httptest.NewRequest("GET", "/api/v1/resources/uid-1/history", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestListResources_DefaultPagination(t *testing.T) {
	srv, store := setupTestServer(t)
	seedData(t, store)

	req := httptest.NewRequest("GET", "/api/v1/resources", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestToRevisionResponse_WithPatch(t *testing.T) {
	rev := storage.ResourceRevision{
		ID: 1, ResourceUID: "uid-1", APIVersion: "apps/v1", Kind: "Deployment",
		Namespace: "default", Name: "nginx", Revision: 2, EventType: storage.EventUpdated,
		Patch: []byte(`{"spec":{"replicas":3}}`), IsSnapshot: false,
		ResourceVersion: "200", Timestamp: time.Now(), PolicyName: "test", SizeBytes: 22,
	}

	resp := toRevisionResponse(rev, true)
	if resp.Content == nil {
		t.Error("expected content for patch revision")
	}
	if resp.IsSnapshot {
		t.Error("expected IsSnapshot=false")
	}
}

func TestToRevisionResponse_NoContent(t *testing.T) {
	rev := storage.ResourceRevision{
		ID: 1, ResourceUID: "uid-1", Revision: 1, EventType: storage.EventCreated,
		Snapshot: []byte(`{}`), IsSnapshot: true,
	}

	resp := toRevisionResponse(rev, false)
	if resp.Content != nil {
		t.Error("expected no content when includeContent=false")
	}
}

func TestWriteJSON(t *testing.T) {
	w := httptest.NewRecorder()
	writeJSON(w, http.StatusOK, apiResponse{Data: "test"})

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
}

func TestWriteError(t *testing.T) {
	w := httptest.NewRecorder()
	writeError(w, http.StatusBadRequest, "bad input")

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
	var resp apiResponse
	_ = json.NewDecoder(w.Body).Decode(&resp)
	if resp.Error != "bad input" {
		t.Errorf("error = %q, want 'bad input'", resp.Error)
	}
}

func TestSpaHandler_ServesIndex(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(dir+"/index.html", []byte("<html>app</html>"), 0644); err != nil {
		t.Fatal(err)
	}

	handler := spaHandler{staticPath: dir, indexPath: "index.html"}
	req := httptest.NewRequest("GET", "/some/spa/route", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if !strings.Contains(w.Body.String(), "app") {
		t.Error("expected index.html content")
	}
}

func TestSpaHandler_ServesStaticFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(dir+"/index.html", []byte("<html>app</html>"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir+"/assets", 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dir+"/assets/style.css", []byte("body{}"), 0644); err != nil {
		t.Fatal(err)
	}

	handler := spaHandler{staticPath: dir, indexPath: "index.html"}
	req := httptest.NewRequest("GET", "/assets/style.css", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestSpaHandler_DirectoryFallsBackToIndex(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(dir+"/index.html", []byte("<html>app</html>"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir+"/subdir", 0755); err != nil {
		t.Fatal(err)
	}

	handler := spaHandler{staticPath: dir, indexPath: "index.html"}
	req := httptest.NewRequest("GET", "/subdir", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestReadyz_StoreError(t *testing.T) {
	// Create a server with a store that's been closed
	dir := t.TempDir()
	store, _ := sqlite.New(dir + "/test.db")
	_ = store.Initialize(context.Background())
	srv := New(store, "", ":0")
	store.Close() // close the store to force an error

	req := httptest.NewRequest("GET", "/readyz", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want %d", w.Code, http.StatusServiceUnavailable)
	}
}

func TestReconstructResource_SnapshotWithSubsequentSnapshot(t *testing.T) {
	srv, store := setupTestServer(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	snap1 := []byte(`{"kind":"Deployment","spec":{"replicas":1}}`)
	compressed1, _ := diff.Compress(snap1)

	snap3 := []byte(`{"kind":"Deployment","spec":{"replicas":5}}`)
	compressed3, _ := diff.Compress(snap3)

	_ = store.UpsertTrackedResource(ctx, &storage.TrackedResourceRecord{
		UID: "uid-s", APIVersion: "apps/v1", Kind: "Deployment", Namespace: "default", Name: "web",
		CurrentRevision: 3, FirstSeen: now, LastSeen: now, PolicyName: "test",
	})
	_ = store.StoreRevision(ctx, &storage.ResourceRevision{
		ResourceUID: "uid-s", APIVersion: "apps/v1", Kind: "Deployment", Namespace: "default", Name: "web",
		Revision: 1, EventType: storage.EventCreated, Snapshot: compressed1, IsSnapshot: true,
		Timestamp: now, PolicyName: "test",
	})
	_ = store.StoreRevision(ctx, &storage.ResourceRevision{
		ResourceUID: "uid-s", APIVersion: "apps/v1", Kind: "Deployment", Namespace: "default", Name: "web",
		Revision: 2, EventType: storage.EventUpdated, Patch: []byte(`{"spec":{"replicas":3}}`),
		Timestamp: now.Add(time.Minute), PolicyName: "test",
	})
	_ = store.StoreRevision(ctx, &storage.ResourceRevision{
		ResourceUID: "uid-s", APIVersion: "apps/v1", Kind: "Deployment", Namespace: "default", Name: "web",
		Revision: 3, EventType: storage.EventUpdated, Snapshot: compressed3, IsSnapshot: true,
		Timestamp: now.Add(2 * time.Minute), PolicyName: "test",
	})

	// Reconstruct at revision 3 should use the second snapshot directly
	req := httptest.NewRequest("GET", "/api/v1/resources/uid-s/reconstruct/3", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp apiResponse
	_ = json.NewDecoder(w.Body).Decode(&resp)
	data, _ := json.Marshal(resp.Data)
	var obj map[string]interface{}
	_ = json.Unmarshal(data, &obj)
	spec := obj["spec"].(map[string]interface{})
	if spec["replicas"] != float64(5) {
		t.Errorf("replicas = %v, want 5", spec["replicas"])
	}
}

func TestGetStats_StoreError(t *testing.T) {
	dir := t.TempDir()
	store, _ := sqlite.New(dir + "/test.db")
	_ = store.Initialize(context.Background())
	srv := New(store, "", ":0")
	store.Close()

	req := httptest.NewRequest("GET", "/api/v1/stats", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
}

func TestGetKindStats_StoreError(t *testing.T) {
	dir := t.TempDir()
	store, _ := sqlite.New(dir + "/test.db")
	_ = store.Initialize(context.Background())
	srv := New(store, "", ":0")
	store.Close()

	req := httptest.NewRequest("GET", "/api/v1/stats/kinds", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
}

func TestListResources_StoreError(t *testing.T) {
	dir := t.TempDir()
	store, _ := sqlite.New(dir + "/test.db")
	_ = store.Initialize(context.Background())
	srv := New(store, "", ":0")
	store.Close()

	req := httptest.NewRequest("GET", "/api/v1/resources", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
}

func TestGetResource_StoreError(t *testing.T) {
	dir := t.TempDir()
	store, _ := sqlite.New(dir + "/test.db")
	_ = store.Initialize(context.Background())
	srv := New(store, "", ":0")
	store.Close()

	req := httptest.NewRequest("GET", "/api/v1/resources/uid-1", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
}

func TestGetHistory_StoreError(t *testing.T) {
	dir := t.TempDir()
	store, _ := sqlite.New(dir + "/test.db")
	_ = store.Initialize(context.Background())
	srv := New(store, "", ":0")
	store.Close()

	req := httptest.NewRequest("GET", "/api/v1/resources/uid-1/history", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
}

func TestGetRevision_StoreError(t *testing.T) {
	dir := t.TempDir()
	store, _ := sqlite.New(dir + "/test.db")
	_ = store.Initialize(context.Background())
	srv := New(store, "", ":0")
	store.Close()

	req := httptest.NewRequest("GET", "/api/v1/resources/uid-1/revisions/1", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
}

func TestNewServerWithUI(t *testing.T) {
	dir := t.TempDir()
	store, _ := sqlite.New(dir + "/test.db")
	_ = store.Initialize(context.Background())
	t.Cleanup(func() { store.Close() })

	srv := New(store, dir, ":0")
	if srv.router == nil {
		t.Error("router should not be nil")
	}
}

func TestCORS(t *testing.T) {
	srv, _ := setupTestServer(t)

	req := httptest.NewRequest("OPTIONS", "/api/v1/stats", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	if w.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Error("missing CORS header")
	}
	if w.Code != http.StatusOK {
		t.Errorf("OPTIONS status = %d, want %d", w.Code, http.StatusOK)
	}
}
