package controller

import (
	"context"
	"testing"
	"time"

	flashbackv1alpha1 "github.com/kflashback/kflashback/api/v1alpha1"
	"github.com/kflashback/kflashback/internal/storage"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func boolPtr(v bool) *bool {
	return &v
}

type fakeStore struct {
	tracked       *storage.TrackedResourceRecord
	upserted      *storage.TrackedResourceRecord
	storedRev     *storage.ResourceRevision
	upsertCalls   int
	storeRevCalls int
}

func (f *fakeStore) Initialize(ctx context.Context) error { return nil }
func (f *fakeStore) Close() error                         { return nil }

func (f *fakeStore) StoreRevision(ctx context.Context, rev *storage.ResourceRevision) error {
	f.storeRevCalls++
	cp := *rev
	f.storedRev = &cp
	return nil
}

func (f *fakeStore) GetRevision(ctx context.Context, resourceUID string, revision int64) (*storage.ResourceRevision, error) {
	return nil, nil
}

func (f *fakeStore) GetLatestRevision(ctx context.Context, resourceUID string) (*storage.ResourceRevision, error) {
	return nil, nil
}

func (f *fakeStore) GetHistory(ctx context.Context, query storage.ResourceHistoryQuery) ([]storage.ResourceRevision, int64, error) {
	return nil, 0, nil
}

func (f *fakeStore) GetNearestSnapshot(ctx context.Context, resourceUID string, revision int64) (*storage.ResourceRevision, error) {
	return nil, nil
}

func (f *fakeStore) GetPatchesBetween(ctx context.Context, resourceUID string, fromRevision, toRevision int64) ([]storage.ResourceRevision, error) {
	return nil, nil
}

func (f *fakeStore) UpsertTrackedResource(ctx context.Context, resource *storage.TrackedResourceRecord) error {
	f.upsertCalls++
	cp := *resource
	f.upserted = &cp
	f.tracked = &cp
	return nil
}

func (f *fakeStore) GetTrackedResource(ctx context.Context, uid string) (*storage.TrackedResourceRecord, error) {
	if f.tracked == nil {
		return nil, nil
	}
	cp := *f.tracked
	return &cp, nil
}

func (f *fakeStore) ListTrackedResources(ctx context.Context, query storage.ResourceListQuery) ([]storage.TrackedResourceRecord, int64, error) {
	return nil, 0, nil
}

func (f *fakeStore) MarkDeleted(ctx context.Context, uid string, deletedAt time.Time) error {
	return nil
}

func (f *fakeStore) PurgeOldRevisions(ctx context.Context, olderThan time.Time) (int64, error) {
	return 0, nil
}

func (f *fakeStore) PurgeExcessRevisions(ctx context.Context, resourceUID string, maxRevisions int64) (int64, error) {
	return 0, nil
}

func (f *fakeStore) GetStats(ctx context.Context) (*storage.StorageStats, error) {
	return nil, nil
}

func (f *fakeStore) GetKindStats(ctx context.Context) ([]storage.KindStats, error) {
	return nil, nil
}

func newTestObject(uid, resourceVersion string, replicas int64) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"metadata": map[string]interface{}{
				"namespace":       "default",
				"name":            "demo",
				"uid":             uid,
				"resourceVersion": resourceVersion,
			},
			"spec": map[string]interface{}{
				"replicas": replicas,
			},
		},
	}
}

func newTestHandler(store storage.Store) *resourceEventHandler {
	policy := &flashbackv1alpha1.FlashbackPolicy{}
	policy.Name = "test-policy"
	policy.Spec.Tracking.Updates = boolPtr(true)

	return &resourceEventHandler{
		store:  store,
		policy: policy,
		trackedRes: flashbackv1alpha1.TrackedResource{
			APIVersion: "apps/v1",
			Kind:       "Deployment",
		},
	}
}

func TestOnUpdatePreservesExistingFirstSeen(t *testing.T) {
	oldFirstSeen := time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)

	store := &fakeStore{
		tracked: &storage.TrackedResourceRecord{
			UID:             "uid-1",
			APIVersion:      "apps/v1",
			Kind:            "Deployment",
			Namespace:       "default",
			Name:            "demo",
			CurrentRevision: 3,
			FirstSeen:       oldFirstSeen,
			LastSeen:        oldFirstSeen,
			PolicyName:      "test-policy",
		},
	}

	handler := newTestHandler(store)

	oldObj := newTestObject("uid-1", "1", 1)
	newObj := newTestObject("uid-1", "2", 2)

	handler.OnUpdate(oldObj, newObj)

	if store.storeRevCalls != 1 {
		t.Fatalf("expected 1 revision to be stored, got %d", store.storeRevCalls)
	}
	if store.upsertCalls != 1 {
		t.Fatalf("expected 1 tracked resource upsert, got %d", store.upsertCalls)
	}
	if store.storedRev == nil {
		t.Fatal("expected stored revision, got nil")
	}
	if store.upserted == nil {
		t.Fatal("expected upserted tracked resource, got nil")
	}

	if got := store.upserted.FirstSeen; !got.Equal(oldFirstSeen) {
		t.Fatalf("expected FirstSeen to stay %v, got %v", oldFirstSeen, got)
	}
	if got := store.upserted.LastSeen; !got.After(oldFirstSeen) {
		t.Fatalf("expected LastSeen to be after %v, got %v", oldFirstSeen, got)
	}
	if got := store.upserted.CurrentRevision; got != 4 {
		t.Fatalf("expected CurrentRevision=4, got %d", got)
	}
	if got := store.storedRev.Revision; got != 4 {
		t.Fatalf("expected stored revision=4, got %d", got)
	}
	if got := store.storedRev.EventType; got != storage.EventUpdated {
		t.Fatalf("expected EventType=%q, got %q", storage.EventUpdated, got)
	}
	if store.storedRev.IsSnapshot {
		t.Fatal("expected update revision to be stored as patch, got snapshot")
	}
	if len(store.storedRev.Patch) == 0 {
		t.Fatal("expected non-empty patch for changed update")
	}
}

func TestOnUpdateSetsFirstSeenWhenTrackedResourceDoesNotExist(t *testing.T) {
	store := &fakeStore{}
	handler := newTestHandler(store)

	oldObj := newTestObject("uid-2", "1", 1)
	newObj := newTestObject("uid-2", "2", 2)

	handler.OnUpdate(oldObj, newObj)

	if store.storeRevCalls != 1 {
		t.Fatalf("expected 1 revision to be stored, got %d", store.storeRevCalls)
	}
	if store.upsertCalls != 1 {
		t.Fatalf("expected 1 tracked resource upsert, got %d", store.upsertCalls)
	}
	if store.upserted == nil {
		t.Fatal("expected upserted tracked resource, got nil")
	}

	if store.upserted.FirstSeen.IsZero() {
		t.Fatal("expected FirstSeen to be set, got zero time")
	}
	if store.upserted.LastSeen.IsZero() {
		t.Fatal("expected LastSeen to be set, got zero time")
	}
	if !store.upserted.FirstSeen.Equal(store.upserted.LastSeen) {
		t.Fatalf("expected FirstSeen and LastSeen to match on first update, got %v and %v",
			store.upserted.FirstSeen, store.upserted.LastSeen)
	}
	if got := store.upserted.CurrentRevision; got != 1 {
		t.Fatalf("expected CurrentRevision=1, got %d", got)
	}
	if got := store.storedRev.Revision; got != 1 {
		t.Fatalf("expected stored revision=1, got %d", got)
	}
}
