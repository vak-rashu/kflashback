// Copyright 2024 The kflashback Authors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/tools/cache"
	ctrl "sigs.k8s.io/controller-runtime"

	flashbackv1alpha1 "github.com/kflashback/kflashback/api/v1alpha1"
	"github.com/kflashback/kflashback/internal/diff"
	"github.com/kflashback/kflashback/internal/storage"
)

var watcherLog = ctrl.Log.WithName("resource-watcher")

// ResourceWatcher dynamically watches Kubernetes resources and records their history.
type ResourceWatcher struct {
	dynClient dynamic.Interface
	store     storage.Store

	mu       sync.RWMutex
	watchers map[string]context.CancelFunc // key: policyName/gvr
}

// NewResourceWatcher creates a new ResourceWatcher.
func NewResourceWatcher(dynClient dynamic.Interface, store storage.Store) *ResourceWatcher {
	return &ResourceWatcher{
		dynClient: dynClient,
		store:     store,
		watchers:  make(map[string]context.CancelFunc),
	}
}

// Reconcile updates watchers based on the FlashbackPolicy spec.
func (rw *ResourceWatcher) Reconcile(ctx context.Context, policy *flashbackv1alpha1.FlashbackPolicy) error {
	if policy.Spec.Paused {
		rw.StopPolicy(policy.Name)
		return nil
	}

	// Stop existing watchers for this policy to re-sync
	rw.StopPolicy(policy.Name)

	for _, res := range policy.Spec.Resources {
		gvr, err := parseGVR(res.APIVersion, res.Kind)
		if err != nil {
			watcherLog.Error(err, "failed to parse GVR", "apiVersion", res.APIVersion, "kind", res.Kind)
			continue
		}

		key := fmt.Sprintf("%s/%s", policy.Name, gvr.String())
		watchCtx, cancel := context.WithCancel(ctx)

		rw.mu.Lock()
		rw.watchers[key] = cancel
		rw.mu.Unlock()

		go rw.watchResource(watchCtx, policy, res, gvr)
	}

	return nil
}

// StopPolicy stops all watchers for a given policy.
func (rw *ResourceWatcher) StopPolicy(policyName string) {
	rw.mu.Lock()
	defer rw.mu.Unlock()

	prefix := policyName + "/"
	for key, cancel := range rw.watchers {
		if len(key) > len(prefix) && key[:len(prefix)] == prefix {
			cancel()
			delete(rw.watchers, key)
		}
	}
}

// StopAll stops all watchers.
func (rw *ResourceWatcher) StopAll() {
	rw.mu.Lock()
	defer rw.mu.Unlock()

	for key, cancel := range rw.watchers {
		cancel()
		delete(rw.watchers, key)
	}
}

func (rw *ResourceWatcher) watchResource(
	ctx context.Context,
	policy *flashbackv1alpha1.FlashbackPolicy,
	trackedRes flashbackv1alpha1.TrackedResource,
	gvr schema.GroupVersionResource,
) {
	log := watcherLog.WithValues("policy", policy.Name, "gvr", gvr.String())
	log.Info("starting resource watch")

	factory := dynamicinformer.NewFilteredDynamicSharedInformerFactory(
		rw.dynClient, 30*time.Second, "", nil,
	)

	informer := factory.ForResource(gvr).Informer()

	handler := &resourceEventHandler{
		store:      rw.store,
		policy:     policy,
		trackedRes: trackedRes,
		gvr:        gvr,
	}

	_, _ = informer.AddEventHandler(handler)

	log.Info("informer started, watching for changes")
	informer.Run(ctx.Done())
	log.Info("resource watch stopped")
}

// resourceEventHandler handles resource events from the informer.
type resourceEventHandler struct {
	store      storage.Store
	policy     *flashbackv1alpha1.FlashbackPolicy
	trackedRes flashbackv1alpha1.TrackedResource
	gvr        schema.GroupVersionResource
}

func (h *resourceEventHandler) OnAdd(obj interface{}, isInInitialList bool) {
	u, ok := obj.(*unstructured.Unstructured)
	if !ok {
		return
	}

	if !h.shouldTrack(u) {
		return
	}

	ctx := context.Background()
	log := watcherLog.WithValues("event", "add", "resource", formatResource(u))

	jsonData, err := json.Marshal(u.Object)
	if err != nil {
		log.Error(err, "failed to marshal resource")
		return
	}

	// Strip noisy fields
	stripped, err := diff.StripFields(jsonData, h.policy.Spec.FieldConfig.IgnoreFields, h.policy.Spec.FieldConfig.TrackStatus)
	if err != nil {
		log.Error(err, "failed to strip fields")
		stripped = jsonData
	}

	now := time.Now().UTC()
	uid := string(u.GetUID())

	// Check if already tracked
	existing, _ := h.store.GetTrackedResource(ctx, uid)

	if isInInitialList && existing != nil {
		// Resource already tracked — this is an informer re-list, not a real creation.
		// Compare against the last stored state to detect drift while controller was down.
		latestRev, err := h.store.GetLatestRevision(ctx, uid)
		if err != nil || latestRev == nil {
			log.V(1).Info("skipping re-list, no previous revision found")
			return
		}

		// Reconstruct the last known state for comparison
		var lastState []byte
		if latestRev.IsSnapshot && len(latestRev.Snapshot) > 0 {
			lastState = latestRev.Snapshot
			if decompressed, derr := diff.Decompress(lastState); derr == nil {
				lastState = decompressed
			}
		}

		if len(lastState) > 0 {
			patch, perr := diff.ComputeMergePatch(lastState, stripped)
			if perr != nil || diff.IsEmpty(patch) {
				// No changes since last recorded state — skip
				log.V(1).Info("skipping re-list, no changes detected")
				return
			}

			// Resource changed while controller was down — record as update
			if !isTrackingEnabled(h.policy.Spec.Tracking.Updates) {
				return
			}

			changedPaths, _ := diff.GetChangedPaths(lastState, stripped)
			revision := existing.CurrentRevision + 1

			snapshotEvery := int64(20)
			if h.policy.Spec.Storage.SnapshotEvery > 0 {
				snapshotEvery = int64(h.policy.Spec.Storage.SnapshotEvery)
			}
			isSnapshotTime := revision%snapshotEvery == 0

			rev := &storage.ResourceRevision{
				ResourceUID:     uid,
				APIVersion:      u.GetAPIVersion(),
				Kind:            u.GetKind(),
				Namespace:       u.GetNamespace(),
				Name:            u.GetName(),
				Revision:        revision,
				EventType:       storage.EventUpdated,
				ResourceVersion: u.GetResourceVersion(),
				ChangedFields:   changedPaths,
				Timestamp:       now,
				PolicyName:      h.policy.Name,
			}

			if isSnapshotTime {
				snapshot := stripped
				compress := h.policy.Spec.Storage.CompressSnapshots == nil || *h.policy.Spec.Storage.CompressSnapshots
				if compress {
					if compressed, cerr := diff.Compress(stripped); cerr == nil {
						snapshot = compressed
					}
				}
				rev.Snapshot = snapshot
				rev.IsSnapshot = true
			} else {
				rev.Patch = patch
				rev.IsSnapshot = false
			}

			if err := h.store.StoreRevision(ctx, rev); err != nil {
				log.Error(err, "failed to store drift-update revision")
				return
			}

			record := &storage.TrackedResourceRecord{
				UID:             uid,
				APIVersion:      u.GetAPIVersion(),
				Kind:            u.GetKind(),
				Namespace:       u.GetNamespace(),
				Name:            u.GetName(),
				CurrentRevision: revision,
				FirstSeen:       existing.FirstSeen,
				LastSeen:        now,
				PolicyName:      h.policy.Name,
			}
			if err := h.store.UpsertTrackedResource(ctx, record); err != nil {
				log.Error(err, "failed to upsert tracked resource")
			}
			log.V(1).Info("recorded drift update", "revision", revision, "changedPaths", changedPaths)
		}
		return
	}

	// Genuine new resource creation
	if !isTrackingEnabled(h.policy.Spec.Tracking.Creations) {
		return
	}

	snapshot := stripped
	compress := h.policy.Spec.Storage.CompressSnapshots == nil || *h.policy.Spec.Storage.CompressSnapshots
	if compress {
		compressed, cerr := diff.Compress(stripped)
		if cerr == nil {
			snapshot = compressed
		}
	}

	var revision int64 = 1
	if existing != nil {
		revision = existing.CurrentRevision + 1
	}

	rev := &storage.ResourceRevision{
		ResourceUID:     uid,
		APIVersion:      u.GetAPIVersion(),
		Kind:            u.GetKind(),
		Namespace:       u.GetNamespace(),
		Name:            u.GetName(),
		Revision:        revision,
		EventType:       storage.EventCreated,
		Snapshot:        snapshot,
		IsSnapshot:      true,
		ResourceVersion: u.GetResourceVersion(),
		Timestamp:       now,
		PolicyName:      h.policy.Name,
	}

	if err := h.store.StoreRevision(ctx, rev); err != nil {
		log.Error(err, "failed to store creation revision")
		return
	}

	record := &storage.TrackedResourceRecord{
		UID:             uid,
		APIVersion:      u.GetAPIVersion(),
		Kind:            u.GetKind(),
		Namespace:       u.GetNamespace(),
		Name:            u.GetName(),
		CurrentRevision: revision,
		FirstSeen:       now,
		LastSeen:        now,
		PolicyName:      h.policy.Name,
	}
	if err := h.store.UpsertTrackedResource(ctx, record); err != nil {
		log.Error(err, "failed to upsert tracked resource")
	}

	log.V(1).Info("recorded creation", "revision", revision)
}

func (h *resourceEventHandler) OnUpdate(oldObj, newObj interface{}) {
	oldU, ok := oldObj.(*unstructured.Unstructured)
	if !ok {
		return
	}
	newU, ok := newObj.(*unstructured.Unstructured)
	if !ok {
		return
	}

	if !h.shouldTrack(newU) {
		return
	}

	if !isTrackingEnabled(h.policy.Spec.Tracking.Updates) {
		return
	}

	ctx := context.Background()
	log := watcherLog.WithValues("event", "update", "resource", formatResource(newU))

	oldJSON, err := json.Marshal(oldU.Object)
	if err != nil {
		return
	}
	newJSON, err := json.Marshal(newU.Object)
	if err != nil {
		return
	}

	// Strip noisy fields
	oldStripped, _ := diff.StripFields(oldJSON, h.policy.Spec.FieldConfig.IgnoreFields, h.policy.Spec.FieldConfig.TrackStatus)
	newStripped, _ := diff.StripFields(newJSON, h.policy.Spec.FieldConfig.IgnoreFields, h.policy.Spec.FieldConfig.TrackStatus)

	// Compute merge patch
	patch, err := diff.ComputeMergePatch(oldStripped, newStripped)
	if err != nil {
		log.Error(err, "failed to compute merge patch")
		return
	}

	// Skip if no actual changes after field stripping
	if diff.IsEmpty(patch) {
		return
	}

	changedPaths, _ := diff.GetChangedPaths(oldStripped, newStripped)

	uid := string(newU.GetUID())
	now := time.Now().UTC()

	existing, _ := h.store.GetTrackedResource(ctx, uid)
	var revision int64 = 1
	if existing != nil {
		revision = existing.CurrentRevision + 1
	}

	// Determine if we should store a full snapshot
	snapshotEvery := int64(20)
	if h.policy.Spec.Storage.SnapshotEvery > 0 {
		snapshotEvery = int64(h.policy.Spec.Storage.SnapshotEvery)
	}
	isSnapshotTime := revision%snapshotEvery == 0

	rev := &storage.ResourceRevision{
		ResourceUID:     uid,
		APIVersion:      newU.GetAPIVersion(),
		Kind:            newU.GetKind(),
		Namespace:       newU.GetNamespace(),
		Name:            newU.GetName(),
		Revision:        revision,
		EventType:       storage.EventUpdated,
		ResourceVersion: newU.GetResourceVersion(),
		ChangedFields:   changedPaths,
		Timestamp:       now,
		PolicyName:      h.policy.Name,
	}

	if isSnapshotTime {
		// Store full snapshot
		snapshot := newStripped
		compress := h.policy.Spec.Storage.CompressSnapshots == nil || *h.policy.Spec.Storage.CompressSnapshots
		if compress {
			if compressed, err := diff.Compress(newStripped); err == nil {
				snapshot = compressed
			}
		}
		rev.Snapshot = snapshot
		rev.IsSnapshot = true
	} else {
		// Store just the patch
		rev.Patch = patch
		rev.IsSnapshot = false
	}

	if err := h.store.StoreRevision(ctx, rev); err != nil {
		log.Error(err, "failed to store update revision")
		return
	}

	firstSeen := now
	if existing != nil && !existing.FirstSeen.IsZero() {
		firstSeen = existing.FirstSeen
	}

	record := &storage.TrackedResourceRecord{
		UID:             uid,
		APIVersion:      newU.GetAPIVersion(),
		Kind:            newU.GetKind(),
		Namespace:       newU.GetNamespace(),
		Name:            newU.GetName(),
		CurrentRevision: revision,
		FirstSeen:       firstSeen,
		LastSeen:        now,
		PolicyName:      h.policy.Name,
	}
	if err := h.store.UpsertTrackedResource(ctx, record); err != nil {
		log.Error(err, "failed to upsert tracked resource")
	}

	log.V(1).Info("recorded update", "revision", revision, "changedPaths", changedPaths)
}

func (h *resourceEventHandler) OnDelete(obj interface{}) {
	u, ok := obj.(*unstructured.Unstructured)
	if !ok {
		tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
		if !ok {
			return
		}
		u, ok = tombstone.Obj.(*unstructured.Unstructured)
		if !ok {
			return
		}
	}

	if !h.shouldTrack(u) {
		return
	}

	if !isTrackingEnabled(h.policy.Spec.Tracking.Deletions) {
		return
	}

	ctx := context.Background()
	log := watcherLog.WithValues("event", "delete", "resource", formatResource(u))
	uid := string(u.GetUID())
	now := time.Now().UTC()

	existing, _ := h.store.GetTrackedResource(ctx, uid)
	var revision int64 = 1
	if existing != nil {
		revision = existing.CurrentRevision + 1
	}

	// Store the last known state as a snapshot on deletion
	jsonData, _ := json.Marshal(u.Object)
	stripped, _ := diff.StripFields(jsonData, h.policy.Spec.FieldConfig.IgnoreFields, h.policy.Spec.FieldConfig.TrackStatus)

	snapshot := stripped
	compress := h.policy.Spec.Storage.CompressSnapshots == nil || *h.policy.Spec.Storage.CompressSnapshots
	if compress {
		if compressed, err := diff.Compress(stripped); err == nil {
			snapshot = compressed
		}
	}

	rev := &storage.ResourceRevision{
		ResourceUID:     uid,
		APIVersion:      u.GetAPIVersion(),
		Kind:            u.GetKind(),
		Namespace:       u.GetNamespace(),
		Name:            u.GetName(),
		Revision:        revision,
		EventType:       storage.EventDeleted,
		Snapshot:        snapshot,
		IsSnapshot:      true,
		ResourceVersion: u.GetResourceVersion(),
		Timestamp:       now,
		PolicyName:      h.policy.Name,
	}

	if err := h.store.StoreRevision(ctx, rev); err != nil {
		log.Error(err, "failed to store deletion revision")
		return
	}

	if err := h.store.MarkDeleted(ctx, uid, now); err != nil {
		log.Error(err, "failed to mark resource as deleted")
	}

	log.V(1).Info("recorded deletion", "revision", revision)
}

// shouldTrack checks if a resource should be tracked based on the policy filters.
func (h *resourceEventHandler) shouldTrack(u *unstructured.Unstructured) bool {
	ns := u.GetNamespace()
	name := u.GetName()

	// Check namespace inclusion
	if len(h.trackedRes.Namespaces) > 0 {
		found := false
		for _, n := range h.trackedRes.Namespaces {
			if n == ns {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	// Check namespace exclusion
	for _, n := range h.trackedRes.ExcludeNamespaces {
		if n == ns {
			return false
		}
	}

	// Check name inclusion
	if len(h.trackedRes.IncludeNames) > 0 {
		found := false
		for _, n := range h.trackedRes.IncludeNames {
			if n == name {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	// Check name exclusion
	for _, n := range h.trackedRes.ExcludeNames {
		if n == name {
			return false
		}
	}

	// Check label selector
	if h.trackedRes.LabelSelector != nil {
		labels := u.GetLabels()
		if h.trackedRes.LabelSelector.MatchLabels != nil {
			for k, v := range h.trackedRes.LabelSelector.MatchLabels {
				if labels[k] != v {
					return false
				}
			}
		}
	}

	return true
}

func isTrackingEnabled(flag *bool) bool {
	return flag == nil || *flag
}

func formatResource(u *unstructured.Unstructured) types.NamespacedName {
	return types.NamespacedName{Namespace: u.GetNamespace(), Name: u.GetName()}
}

func parseGVR(apiVersion, kind string) (schema.GroupVersionResource, error) {
	gv, err := schema.ParseGroupVersion(apiVersion)
	if err != nil {
		return schema.GroupVersionResource{}, fmt.Errorf("parsing group version %q: %w", apiVersion, err)
	}

	// Convert Kind to plural resource name (simple heuristic)
	resource := pluralize(kind)

	return schema.GroupVersionResource{
		Group:    gv.Group,
		Version:  gv.Version,
		Resource: resource,
	}, nil
}

func pluralize(kind string) string {
	lower := toLower(kind)
	switch {
	case hasSuffix(lower, "s"):
		return lower + "es"
	case hasSuffix(lower, "y"):
		return lower[:len(lower)-1] + "ies"
	default:
		return lower + "s"
	}
}

func toLower(s string) string {
	b := make([]byte, len(s))
	for i := range s {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			b[i] = c + 32
		} else {
			b[i] = c
		}
	}
	return string(b)
}

func hasSuffix(s, suffix string) bool {
	return len(s) >= len(suffix) && s[len(s)-len(suffix):] == suffix
}
