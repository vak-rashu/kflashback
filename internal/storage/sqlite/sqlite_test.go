// Copyright 2024 The kflashback Authors
// SPDX-License-Identifier: Apache-2.0

package sqlite

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/kflashback/kflashback/internal/storage"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	s, err := New(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	if err := s.Initialize(context.Background()); err != nil {
		t.Fatalf("failed to initialize store: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestInitialize(t *testing.T) {
	s := newTestStore(t)
	// calling Initialize twice should be idempotent
	if err := s.Initialize(context.Background()); err != nil {
		t.Fatalf("second Initialize failed: %v", err)
	}
}

func TestStoreAndGetRevision(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	rev := &storage.ResourceRevision{
		ResourceUID:     "uid-1",
		APIVersion:      "apps/v1",
		Kind:            "Deployment",
		Namespace:       "default",
		Name:            "nginx",
		Revision:        1,
		EventType:       storage.EventCreated,
		Snapshot:        []byte(`{"kind":"Deployment","metadata":{"name":"nginx"}}`),
		IsSnapshot:      true,
		ResourceVersion: "100",
		ChangedFields:   []string{"spec.replicas"},
		Timestamp:       time.Now().UTC().Truncate(time.Second),
		PolicyName:      "test-policy",
	}

	if err := s.StoreRevision(ctx, rev); err != nil {
		t.Fatalf("StoreRevision: %v", err)
	}

	got, err := s.GetRevision(ctx, "uid-1", 1)
	if err != nil {
		t.Fatalf("GetRevision: %v", err)
	}
	if got == nil {
		t.Fatal("GetRevision returned nil")
	}
	if got.ResourceUID != "uid-1" {
		t.Errorf("ResourceUID = %q, want %q", got.ResourceUID, "uid-1")
	}
	if got.Kind != "Deployment" {
		t.Errorf("Kind = %q, want %q", got.Kind, "Deployment")
	}
	if got.Revision != 1 {
		t.Errorf("Revision = %d, want %d", got.Revision, 1)
	}
	if got.EventType != storage.EventCreated {
		t.Errorf("EventType = %q, want %q", got.EventType, storage.EventCreated)
	}
	if len(got.ChangedFields) != 1 || got.ChangedFields[0] != "spec.replicas" {
		t.Errorf("ChangedFields = %v, want [spec.replicas]", got.ChangedFields)
	}
}

func TestGetRevision_NotFound(t *testing.T) {
	s := newTestStore(t)
	got, err := s.GetRevision(context.Background(), "nonexistent", 1)
	if err != nil {
		t.Fatalf("GetRevision: %v", err)
	}
	if got != nil {
		t.Error("expected nil for nonexistent revision")
	}
}

func TestGetLatestRevision(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	for i := int64(1); i <= 3; i++ {
		_ = s.StoreRevision(ctx, &storage.ResourceRevision{
			ResourceUID: "uid-1",
			APIVersion:  "apps/v1",
			Kind:        "Deployment",
			Namespace:   "default",
			Name:        "nginx",
			Revision:    i,
			EventType:   storage.EventUpdated,
			Snapshot:    []byte(`{}`),
			IsSnapshot:  i == 1,
			Timestamp:   now.Add(time.Duration(i) * time.Minute),
			PolicyName:  "p",
		})
	}

	got, err := s.GetLatestRevision(ctx, "uid-1")
	if err != nil {
		t.Fatalf("GetLatestRevision: %v", err)
	}
	if got.Revision != 3 {
		t.Errorf("Revision = %d, want 3", got.Revision)
	}
}

func TestGetHistory_Pagination(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	for i := int64(1); i <= 10; i++ {
		_ = s.StoreRevision(ctx, &storage.ResourceRevision{
			ResourceUID: "uid-1",
			APIVersion:  "v1",
			Kind:        "Service",
			Namespace:   "ns",
			Name:        "svc",
			Revision:    i,
			EventType:   storage.EventUpdated,
			Snapshot:    []byte(`{}`),
			IsSnapshot:  i == 1,
			Timestamp:   now.Add(time.Duration(i) * time.Minute),
			PolicyName:  "p",
		})
	}

	revs, total, err := s.GetHistory(ctx, storage.ResourceHistoryQuery{
		UID:   "uid-1",
		Limit: 3,
	})
	if err != nil {
		t.Fatalf("GetHistory: %v", err)
	}
	if total != 10 {
		t.Errorf("total = %d, want 10", total)
	}
	if len(revs) != 3 {
		t.Errorf("len(revs) = %d, want 3", len(revs))
	}
	// Should be descending by revision
	if revs[0].Revision != 10 {
		t.Errorf("first revision = %d, want 10", revs[0].Revision)
	}
}

func TestGetHistory_EventTypeFilter(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	_ = s.StoreRevision(ctx, &storage.ResourceRevision{
		ResourceUID: "uid-1", APIVersion: "v1", Kind: "Pod", Namespace: "ns", Name: "p",
		Revision: 1, EventType: storage.EventCreated, Snapshot: []byte(`{}`), IsSnapshot: true,
		Timestamp: now, PolicyName: "p",
	})
	_ = s.StoreRevision(ctx, &storage.ResourceRevision{
		ResourceUID: "uid-1", APIVersion: "v1", Kind: "Pod", Namespace: "ns", Name: "p",
		Revision: 2, EventType: storage.EventUpdated, Patch: []byte(`{}`),
		Timestamp: now.Add(time.Minute), PolicyName: "p",
	})
	_ = s.StoreRevision(ctx, &storage.ResourceRevision{
		ResourceUID: "uid-1", APIVersion: "v1", Kind: "Pod", Namespace: "ns", Name: "p",
		Revision: 3, EventType: storage.EventDeleted, Snapshot: []byte(`{}`), IsSnapshot: true,
		Timestamp: now.Add(2 * time.Minute), PolicyName: "p",
	})

	revs, total, err := s.GetHistory(ctx, storage.ResourceHistoryQuery{
		UID:       "uid-1",
		EventType: "CREATED",
	})
	if err != nil {
		t.Fatalf("GetHistory: %v", err)
	}
	if total != 1 {
		t.Errorf("total = %d, want 1", total)
	}
	if len(revs) != 1 || revs[0].EventType != storage.EventCreated {
		t.Errorf("unexpected revisions: %v", revs)
	}
}

func TestGetNearestSnapshot(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	_ = s.StoreRevision(ctx, &storage.ResourceRevision{
		ResourceUID: "uid-1", APIVersion: "v1", Kind: "Svc", Namespace: "ns", Name: "s",
		Revision: 1, EventType: storage.EventCreated, Snapshot: []byte(`{"v":1}`), IsSnapshot: true,
		Timestamp: now, PolicyName: "p",
	})
	_ = s.StoreRevision(ctx, &storage.ResourceRevision{
		ResourceUID: "uid-1", APIVersion: "v1", Kind: "Svc", Namespace: "ns", Name: "s",
		Revision: 2, EventType: storage.EventUpdated, Patch: []byte(`{"v":2}`),
		Timestamp: now.Add(time.Minute), PolicyName: "p",
	})
	_ = s.StoreRevision(ctx, &storage.ResourceRevision{
		ResourceUID: "uid-1", APIVersion: "v1", Kind: "Svc", Namespace: "ns", Name: "s",
		Revision: 3, EventType: storage.EventUpdated, Patch: []byte(`{"v":3}`),
		Timestamp: now.Add(2 * time.Minute), PolicyName: "p",
	})

	snap, err := s.GetNearestSnapshot(ctx, "uid-1", 3)
	if err != nil {
		t.Fatalf("GetNearestSnapshot: %v", err)
	}
	if snap.Revision != 1 {
		t.Errorf("nearest snapshot revision = %d, want 1", snap.Revision)
	}
}

func TestGetPatchesBetween(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	for i := int64(1); i <= 5; i++ {
		_ = s.StoreRevision(ctx, &storage.ResourceRevision{
			ResourceUID: "uid-1", APIVersion: "v1", Kind: "Svc", Namespace: "ns", Name: "s",
			Revision: i, EventType: storage.EventUpdated,
			Snapshot: []byte(`{}`), IsSnapshot: i == 1,
			Patch:     []byte(`{}`),
			Timestamp: now.Add(time.Duration(i) * time.Minute), PolicyName: "p",
		})
	}

	patches, err := s.GetPatchesBetween(ctx, "uid-1", 1, 4)
	if err != nil {
		t.Fatalf("GetPatchesBetween: %v", err)
	}
	if len(patches) != 3 {
		t.Errorf("len(patches) = %d, want 3 (revisions 2,3,4)", len(patches))
	}
}

func TestUpsertAndGetTrackedResource(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	res := &storage.TrackedResourceRecord{
		UID:             "uid-1",
		APIVersion:      "apps/v1",
		Kind:            "Deployment",
		Namespace:       "default",
		Name:            "nginx",
		CurrentRevision: 1,
		FirstSeen:       now,
		LastSeen:        now,
		IsDeleted:       false,
		PolicyName:      "test",
	}

	if err := s.UpsertTrackedResource(ctx, res); err != nil {
		t.Fatalf("UpsertTrackedResource: %v", err)
	}

	got, err := s.GetTrackedResource(ctx, "uid-1")
	if err != nil {
		t.Fatalf("GetTrackedResource: %v", err)
	}
	if got == nil {
		t.Fatal("GetTrackedResource returned nil")
	}
	if got.Name != "nginx" {
		t.Errorf("Name = %q, want %q", got.Name, "nginx")
	}

	// Upsert should update
	res.CurrentRevision = 5
	res.LastSeen = now.Add(time.Hour)
	if err := s.UpsertTrackedResource(ctx, res); err != nil {
		t.Fatalf("UpsertTrackedResource update: %v", err)
	}
	got, _ = s.GetTrackedResource(ctx, "uid-1")
	if got.CurrentRevision != 5 {
		t.Errorf("CurrentRevision = %d, want 5", got.CurrentRevision)
	}
}

func TestListTrackedResources(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	for _, kind := range []string{"Deployment", "Service", "Deployment"} {
		uid := kind + "-" + now.String()
		_ = s.UpsertTrackedResource(ctx, &storage.TrackedResourceRecord{
			UID: uid, APIVersion: "v1", Kind: kind, Namespace: "ns", Name: uid,
			FirstSeen: now, LastSeen: now, PolicyName: "p",
		})
		now = now.Add(time.Second)
	}

	resources, total, err := s.ListTrackedResources(ctx, storage.ResourceListQuery{Kind: "Deployment"})
	if err != nil {
		t.Fatalf("ListTrackedResources: %v", err)
	}
	if total != 2 {
		t.Errorf("total = %d, want 2", total)
	}
	if len(resources) != 2 {
		t.Errorf("len(resources) = %d, want 2", len(resources))
	}
}

func TestMarkDeleted(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	_ = s.UpsertTrackedResource(ctx, &storage.TrackedResourceRecord{
		UID: "uid-1", APIVersion: "v1", Kind: "Pod", Namespace: "ns", Name: "p",
		FirstSeen: now, LastSeen: now, PolicyName: "p",
	})

	if err := s.MarkDeleted(ctx, "uid-1", now.Add(time.Hour)); err != nil {
		t.Fatalf("MarkDeleted: %v", err)
	}

	got, _ := s.GetTrackedResource(ctx, "uid-1")
	if !got.IsDeleted {
		t.Error("expected resource to be marked deleted")
	}
}

func TestPurgeOldRevisions(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	for i := int64(1); i <= 5; i++ {
		_ = s.StoreRevision(ctx, &storage.ResourceRevision{
			ResourceUID: "uid-1", APIVersion: "v1", Kind: "Pod", Namespace: "ns", Name: "p",
			Revision: i, EventType: storage.EventUpdated, Snapshot: []byte(`{}`), IsSnapshot: i == 1,
			Timestamp: now.Add(time.Duration(i-3) * 24 * time.Hour), PolicyName: "p",
		})
	}

	purged, err := s.PurgeOldRevisions(ctx, now)
	if err != nil {
		t.Fatalf("PurgeOldRevisions: %v", err)
	}
	if purged != 2 {
		t.Errorf("purged = %d, want 2", purged)
	}
}

func TestGetStats(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	_ = s.UpsertTrackedResource(ctx, &storage.TrackedResourceRecord{
		UID: "uid-1", APIVersion: "v1", Kind: "Pod", Namespace: "ns", Name: "p",
		FirstSeen: now, LastSeen: now, PolicyName: "p",
	})
	_ = s.StoreRevision(ctx, &storage.ResourceRevision{
		ResourceUID: "uid-1", APIVersion: "v1", Kind: "Pod", Namespace: "ns", Name: "p",
		Revision: 1, EventType: storage.EventCreated, Snapshot: []byte(`{}`), IsSnapshot: true,
		Timestamp: now, PolicyName: "p",
	})

	stats, err := s.GetStats(ctx)
	if err != nil {
		t.Fatalf("GetStats: %v", err)
	}
	if stats.TotalResources != 1 {
		t.Errorf("TotalResources = %d, want 1", stats.TotalResources)
	}
	if stats.TotalRevisions != 1 {
		t.Errorf("TotalRevisions = %d, want 1", stats.TotalRevisions)
	}
}

func TestGetKindStats(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	_ = s.UpsertTrackedResource(ctx, &storage.TrackedResourceRecord{
		UID: "uid-1", APIVersion: "apps/v1", Kind: "Deployment", Namespace: "ns", Name: "d1",
		FirstSeen: now, LastSeen: now, PolicyName: "p",
	})
	_ = s.StoreRevision(ctx, &storage.ResourceRevision{
		ResourceUID: "uid-1", APIVersion: "apps/v1", Kind: "Deployment", Namespace: "ns", Name: "d1",
		Revision: 1, EventType: storage.EventCreated, Snapshot: []byte(`{}`), IsSnapshot: true,
		Timestamp: now, PolicyName: "p",
	})

	stats, err := s.GetKindStats(ctx)
	if err != nil {
		t.Fatalf("GetKindStats: %v", err)
	}
	if len(stats) != 1 {
		t.Fatalf("len(stats) = %d, want 1", len(stats))
	}
	if stats[0].Kind != "Deployment" {
		t.Errorf("Kind = %q, want %q", stats[0].Kind, "Deployment")
	}
}

func TestGetHistory_AllFilters(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	_ = s.StoreRevision(ctx, &storage.ResourceRevision{
		ResourceUID: "uid-f", APIVersion: "apps/v1", Kind: "Deployment", Namespace: "prod", Name: "web",
		Revision: 1, EventType: storage.EventCreated, Snapshot: []byte(`{}`), IsSnapshot: true,
		ResourceVersion: "100", Timestamp: now, PolicyName: "pol-a",
	})
	_ = s.StoreRevision(ctx, &storage.ResourceRevision{
		ResourceUID: "uid-f", APIVersion: "apps/v1", Kind: "Deployment", Namespace: "prod", Name: "web",
		Revision: 2, EventType: storage.EventUpdated, Patch: []byte(`{}`),
		ResourceVersion: "200", Timestamp: now.Add(time.Minute), PolicyName: "pol-a",
	})

	since := now.Add(-time.Hour)
	until := now.Add(time.Hour)
	revs, total, err := s.GetHistory(ctx, storage.ResourceHistoryQuery{
		UID:        "uid-f",
		APIVersion: "apps/v1",
		Kind:       "Deployment",
		Namespace:  "prod",
		Name:       "web",
		PolicyName: "pol-a",
		EventType:  "UPDATED",
		Since:      &since,
		Until:      &until,
		Limit:      10,
		Offset:     0,
	})
	if err != nil {
		t.Fatalf("GetHistory: %v", err)
	}
	if total != 1 {
		t.Errorf("total = %d, want 1", total)
	}
	if len(revs) != 1 {
		t.Errorf("len = %d, want 1", len(revs))
	}
}

func TestListTrackedResources_AllFilters(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	_ = s.UpsertTrackedResource(ctx, &storage.TrackedResourceRecord{
		UID: "uid-a", APIVersion: "apps/v1", Kind: "Deployment", Namespace: "prod", Name: "web",
		FirstSeen: now, LastSeen: now, PolicyName: "pol-x",
	})
	_ = s.UpsertTrackedResource(ctx, &storage.TrackedResourceRecord{
		UID: "uid-b", APIVersion: "v1", Kind: "Service", Namespace: "staging", Name: "api",
		FirstSeen: now, LastSeen: now, PolicyName: "pol-y", IsDeleted: true,
	})

	// Filter by namespace
	_, total, err := s.ListTrackedResources(ctx, storage.ResourceListQuery{Namespace: "prod"})
	if err != nil {
		t.Fatalf("ListTrackedResources: %v", err)
	}
	if total != 1 {
		t.Errorf("namespace filter: total = %d, want 1", total)
	}

	// Filter by policy
	res, total, _ := s.ListTrackedResources(ctx, storage.ResourceListQuery{PolicyName: "pol-y"})
	if total != 1 || res[0].UID != "uid-b" {
		t.Errorf("policy filter: total=%d, want 1", total)
	}

	// Filter by apiVersion
	res, total, _ = s.ListTrackedResources(ctx, storage.ResourceListQuery{APIVersion: "apps/v1"})
	if total != 1 || res[0].UID != "uid-a" {
		t.Errorf("apiVersion filter: total=%d, want 1", total)
	}

	// Filter by deleted
	deleted := true
	res, total, _ = s.ListTrackedResources(ctx, storage.ResourceListQuery{IsDeleted: &deleted})
	if total != 1 || res[0].UID != "uid-b" {
		t.Errorf("deleted filter: total=%d, want 1", total)
	}

	notDeleted := false
	_, total, _ = s.ListTrackedResources(ctx, storage.ResourceListQuery{IsDeleted: &notDeleted})
	if total != 1 {
		t.Errorf("not-deleted filter: total=%d, want 1", total)
	}
}

func TestPurgeExcessRevisions(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	for i := int64(1); i <= 10; i++ {
		_ = s.StoreRevision(ctx, &storage.ResourceRevision{
			ResourceUID: "uid-1", APIVersion: "v1", Kind: "Pod", Namespace: "ns", Name: "p",
			Revision: i, EventType: storage.EventUpdated, Snapshot: []byte(`{}`), IsSnapshot: i == 1,
			Timestamp: now.Add(time.Duration(i) * time.Minute), PolicyName: "p",
		})
	}

	purged, err := s.PurgeExcessRevisions(ctx, "uid-1", 5)
	if err != nil {
		t.Fatalf("PurgeExcessRevisions: %v", err)
	}
	if purged != 5 {
		t.Errorf("purged = %d, want 5", purged)
	}

	_, total, _ := s.GetHistory(ctx, storage.ResourceHistoryQuery{UID: "uid-1"})
	if total != 5 {
		t.Errorf("remaining = %d, want 5", total)
	}
}

func TestGetTrackedResource_NotFound(t *testing.T) {
	s := newTestStore(t)
	got, err := s.GetTrackedResource(context.Background(), "nonexistent")
	if err != nil {
		t.Fatalf("GetTrackedResource: %v", err)
	}
	if got != nil {
		t.Error("expected nil for nonexistent resource")
	}
}

