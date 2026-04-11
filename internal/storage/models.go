// Copyright 2024 The kflashback Authors
// SPDX-License-Identifier: Apache-2.0

package storage

import "time"

// EventType represents the type of change event.
type EventType string

const (
	EventCreated EventType = "CREATED"
	EventUpdated EventType = "UPDATED"
	EventDeleted EventType = "DELETED"
)

// TrackedResourceRecord represents a Kubernetes resource being tracked.
type TrackedResourceRecord struct {
	UID             string    `json:"uid"`
	APIVersion      string    `json:"apiVersion"`
	Kind            string    `json:"kind"`
	Namespace       string    `json:"namespace"`
	Name            string    `json:"name"`
	CurrentRevision int64     `json:"currentRevision"`
	FirstSeen       time.Time `json:"firstSeen"`
	LastSeen        time.Time `json:"lastSeen"`
	IsDeleted       bool      `json:"isDeleted"`
	PolicyName      string    `json:"policyName"`
}

// ResourceRevision represents a single point-in-time revision of a resource.
type ResourceRevision struct {
	ID              int64     `json:"id"`
	ResourceUID     string    `json:"resourceUid"`
	APIVersion      string    `json:"apiVersion"`
	Kind            string    `json:"kind"`
	Namespace       string    `json:"namespace"`
	Name            string    `json:"name"`
	Revision        int64     `json:"revision"`
	EventType       EventType `json:"eventType"`
	Snapshot        []byte    `json:"snapshot,omitempty"`
	Patch           []byte    `json:"patch,omitempty"`
	IsSnapshot      bool      `json:"isSnapshot"`
	ResourceVersion string    `json:"resourceVersion"`
	ChangedFields   []string  `json:"changedFields,omitempty"`
	Timestamp       time.Time `json:"timestamp"`
	PolicyName      string    `json:"policyName"`
	SizeBytes       int64     `json:"sizeBytes"`
}

// ResourceHistoryQuery defines parameters for querying resource history.
type ResourceHistoryQuery struct {
	UID        string
	APIVersion string
	Kind       string
	Namespace  string
	Name       string
	PolicyName string
	EventType  string
	Since      *time.Time
	Until      *time.Time
	Limit      int
	Offset     int
}

// ResourceListQuery defines parameters for listing tracked resources.
type ResourceListQuery struct {
	APIVersion string
	Kind       string
	Namespace  string
	PolicyName string
	IsDeleted  *bool
	Limit      int
	Offset     int
}

// StorageStats provides storage statistics.
type StorageStats struct {
	TotalResources int64      `json:"totalResources"`
	TotalRevisions int64      `json:"totalRevisions"`
	StorageBytes   int64      `json:"storageBytes"`
	OldestRevision *time.Time `json:"oldestRevision,omitempty"`
	NewestRevision *time.Time `json:"newestRevision,omitempty"`
}

// KindStats provides per-kind statistics.
type KindStats struct {
	APIVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
	Resources  int64  `json:"resources"`
	Revisions  int64  `json:"revisions"`
}

// DiffResult represents the diff between two revisions.
type DiffResult struct {
	FromRevision int64    `json:"fromRevision"`
	ToRevision   int64    `json:"toRevision"`
	Patch        []byte   `json:"patch"`
	ChangedPaths []string `json:"changedPaths"`
	FromSnapshot []byte   `json:"fromSnapshot"`
	ToSnapshot   []byte   `json:"toSnapshot"`
}
