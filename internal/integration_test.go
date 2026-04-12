//go:build integration

// Copyright 2024 The kflashback Authors
// SPDX-License-Identifier: Apache-2.0

package internal

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/kflashback/kflashback/internal/diff"
	"github.com/kflashback/kflashback/internal/storage"
	"github.com/kflashback/kflashback/internal/storage/sqlite"
)

// TestEndToEnd_StoreAndReconstruct tests the full flow:
// store snapshots and patches, then reconstruct a resource at a given revision.
func TestEndToEnd_StoreAndReconstruct(t *testing.T) {
	dir := t.TempDir()
	store, err := sqlite.New(dir + "/integration.db")
	if err != nil {
		t.Fatalf("create store: %v", err)
	}
	defer store.Close()
	if err := store.Initialize(context.Background()); err != nil {
		t.Fatalf("initialize: %v", err)
	}

	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)
	uid := "uid-integration"

	// --- Step 1: Store initial snapshot (revision 1) ---
	v1 := []byte(`{"kind":"Deployment","metadata":{"name":"web"},"spec":{"replicas":1,"image":"nginx:1.0"}}`)
	compressed, err := diff.Compress(v1)
	if err != nil {
		t.Fatalf("compress: %v", err)
	}

	_ = store.UpsertTrackedResource(ctx, &storage.TrackedResourceRecord{
		UID: uid, APIVersion: "apps/v1", Kind: "Deployment", Namespace: "prod", Name: "web",
		CurrentRevision: 1, FirstSeen: now, LastSeen: now, PolicyName: "default",
	})

	_ = store.StoreRevision(ctx, &storage.ResourceRevision{
		ResourceUID: uid, APIVersion: "apps/v1", Kind: "Deployment", Namespace: "prod", Name: "web",
		Revision: 1, EventType: storage.EventCreated, Snapshot: compressed, IsSnapshot: true,
		ResourceVersion: "100", Timestamp: now, PolicyName: "default",
	})

	// --- Step 2: Store patches (revisions 2-4) ---
	v2 := []byte(`{"kind":"Deployment","metadata":{"name":"web"},"spec":{"replicas":3,"image":"nginx:1.0"}}`)
	patch12, _ := diff.ComputeMergePatch(v1, v2)

	v3 := []byte(`{"kind":"Deployment","metadata":{"name":"web"},"spec":{"replicas":3,"image":"nginx:2.0"}}`)
	patch23, _ := diff.ComputeMergePatch(v2, v3)

	v4 := []byte(`{"kind":"Deployment","metadata":{"name":"web"},"spec":{"replicas":5,"image":"nginx:2.0"}}`)
	patch34, _ := diff.ComputeMergePatch(v3, v4)

	for i, patch := range [][]byte{patch12, patch23, patch34} {
		rev := int64(i + 2)
		_ = store.StoreRevision(ctx, &storage.ResourceRevision{
			ResourceUID: uid, APIVersion: "apps/v1", Kind: "Deployment", Namespace: "prod", Name: "web",
			Revision: rev, EventType: storage.EventUpdated, Patch: patch,
			ResourceVersion: "200", Timestamp: now.Add(time.Duration(i+1) * time.Minute), PolicyName: "default",
		})
	}

	// --- Step 3: Reconstruct at each revision ---
	for _, tc := range []struct {
		rev      int64
		replicas float64
		image    string
	}{
		{1, 1, "nginx:1.0"},
		{2, 3, "nginx:1.0"},
		{3, 3, "nginx:2.0"},
		{4, 5, "nginx:2.0"},
	} {
		data, err := reconstruct(ctx, store, uid, tc.rev)
		if err != nil {
			t.Fatalf("reconstruct revision %d: %v", tc.rev, err)
		}

		var obj map[string]interface{}
		if err := json.Unmarshal(data, &obj); err != nil {
			t.Fatalf("unmarshal revision %d: %v", tc.rev, err)
		}

		spec := obj["spec"].(map[string]interface{})
		if spec["replicas"] != tc.replicas {
			t.Errorf("revision %d: replicas = %v, want %v", tc.rev, spec["replicas"], tc.replicas)
		}
		if spec["image"] != tc.image {
			t.Errorf("revision %d: image = %v, want %v", tc.rev, spec["image"], tc.image)
		}
	}

	// --- Step 4: Verify diff between revisions ---
	data1, _ := reconstruct(ctx, store, uid, 1)
	data4, _ := reconstruct(ctx, store, uid, 4)
	patch14, err := diff.ComputeMergePatch(data1, data4)
	if err != nil {
		t.Fatalf("compute diff 1->4: %v", err)
	}
	if diff.IsEmpty(patch14) {
		t.Error("diff between revision 1 and 4 should not be empty")
	}

	changedPaths, _ := diff.GetChangedPaths(data1, data4)
	if len(changedPaths) == 0 {
		t.Error("expected changed paths between revision 1 and 4")
	}

	// --- Step 5: Verify stats ---
	stats, err := store.GetStats(ctx)
	if err != nil {
		t.Fatalf("GetStats: %v", err)
	}
	if stats.TotalResources != 1 {
		t.Errorf("TotalResources = %d, want 1", stats.TotalResources)
	}
	if stats.TotalRevisions != 4 {
		t.Errorf("TotalRevisions = %d, want 4", stats.TotalRevisions)
	}

	// --- Step 6: Purge old revisions ---
	cutoff := now.Add(2 * time.Minute) // keeps revisions 3 and 4
	purged, err := store.PurgeOldRevisions(ctx, cutoff)
	if err != nil {
		t.Fatalf("PurgeOldRevisions: %v", err)
	}
	if purged != 2 {
		t.Errorf("purged = %d, want 2", purged)
	}

	_, total, _ := store.GetHistory(ctx, storage.ResourceHistoryQuery{UID: uid})
	if total != 2 {
		t.Errorf("remaining revisions = %d, want 2", total)
	}
}

// reconstruct rebuilds a resource at the given revision using the store.
func reconstruct(ctx context.Context, store storage.Store, uid string, revision int64) ([]byte, error) {
	snapshot, err := store.GetNearestSnapshot(ctx, uid, revision)
	if err != nil {
		return nil, err
	}
	if snapshot == nil {
		return nil, nil
	}

	base := snapshot.Snapshot
	if decompressed, err := diff.Decompress(base); err == nil {
		base = decompressed
	}

	if snapshot.Revision == revision {
		return base, nil
	}

	patches, err := store.GetPatchesBetween(ctx, uid, snapshot.Revision, revision)
	if err != nil {
		return nil, err
	}

	current := base
	for _, p := range patches {
		if p.IsSnapshot {
			data := p.Snapshot
			if decompressed, err := diff.Decompress(data); err == nil {
				data = decompressed
			}
			current = data
		} else if len(p.Patch) > 0 {
			current, err = diff.ApplyMergePatch(current, p.Patch)
			if err != nil {
				return nil, err
			}
		}
	}
	return current, nil
}
