// Copyright 2024 The kflashback Authors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,shortName=fbp
// +kubebuilder:printcolumn:name="Tracked",type=integer,JSONPath=`.status.trackedResources`
// +kubebuilder:printcolumn:name="Revisions",type=integer,JSONPath=`.status.totalRevisions`
// +kubebuilder:printcolumn:name="Storage",type=string,JSONPath=`.status.storageUsed`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// FlashbackPolicy defines which Kubernetes resources should be tracked
// and how their history should be stored.
type FlashbackPolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   FlashbackPolicySpec   `json:"spec,omitempty"`
	Status FlashbackPolicyStatus `json:"status,omitempty"`
}

// FlashbackPolicySpec defines the desired state of FlashbackPolicy.
type FlashbackPolicySpec struct {
	// Resources is the list of Kubernetes resource types to track.
	// +kubebuilder:validation:MinItems=1
	Resources []TrackedResource `json:"resources"`

	// Retention configures how long history is kept.
	// +optional
	Retention RetentionPolicy `json:"retention,omitempty"`

	// Storage configures storage optimization settings.
	// +optional
	Storage StorageConfig `json:"storage,omitempty"`

	// FieldConfig configures which fields to track or ignore.
	// +optional
	FieldConfig FieldConfig `json:"fieldConfig,omitempty"`

	// Tracking configures which event types to capture.
	// +optional
	Tracking TrackingConfig `json:"tracking,omitempty"`

	// Paused stops all tracking when set to true.
	// +optional
	Paused bool `json:"paused,omitempty"`
}

// TrackedResource defines a Kubernetes resource type to track.
type TrackedResource struct {
	// APIVersion is the API version of the resource (e.g., "apps/v1", "v1").
	// +kubebuilder:validation:Required
	APIVersion string `json:"apiVersion"`

	// Kind is the resource kind (e.g., "Deployment", "Service").
	// +kubebuilder:validation:Required
	Kind string `json:"kind"`

	// Namespaces limits tracking to specific namespaces. Empty means all namespaces.
	// +optional
	Namespaces []string `json:"namespaces,omitempty"`

	// ExcludeNamespaces excludes specific namespaces from tracking.
	// +optional
	ExcludeNamespaces []string `json:"excludeNamespaces,omitempty"`

	// LabelSelector filters resources by labels.
	// +optional
	LabelSelector *metav1.LabelSelector `json:"labelSelector,omitempty"`

	// ExcludeNames excludes specific resource names from tracking.
	// +optional
	ExcludeNames []string `json:"excludeNames,omitempty"`

	// IncludeNames limits tracking to specific resource names.
	// +optional
	IncludeNames []string `json:"includeNames,omitempty"`
}

// RetentionPolicy configures how long resource history is kept.
type RetentionPolicy struct {
	// MaxAge is the maximum duration to retain history (e.g., "720h" for 30 days).
	// +optional
	// +kubebuilder:default="720h"
	MaxAge string `json:"maxAge,omitempty"`

	// MaxRevisions is the maximum number of revisions to keep per resource.
	// +optional
	// +kubebuilder:default=1000
	// +kubebuilder:validation:Minimum=1
	MaxRevisions int32 `json:"maxRevisions,omitempty"`
}

// StorageConfig configures storage optimization.
type StorageConfig struct {
	// SnapshotEvery stores a full snapshot every N revisions to cap reconstruction cost.
	// +optional
	// +kubebuilder:default=20
	// +kubebuilder:validation:Minimum=1
	SnapshotEvery int32 `json:"snapshotEvery,omitempty"`

	// CompressSnapshots enables gzip compression for full snapshots.
	// +optional
	// +kubebuilder:default=true
	CompressSnapshots *bool `json:"compressSnapshots,omitempty"`
}

// FieldConfig configures which fields to track or ignore.
type FieldConfig struct {
	// IgnoreFields lists JSON paths to ignore when detecting changes.
	// Default ignores: .metadata.resourceVersion, .metadata.managedFields,
	// .metadata.generation, .metadata.annotations["kubectl.kubernetes.io/last-applied-configuration"]
	// +optional
	IgnoreFields []string `json:"ignoreFields,omitempty"`

	// IncludeFields limits change tracking to specific fields. Empty means all fields.
	// +optional
	IncludeFields []string `json:"includeFields,omitempty"`

	// TrackStatus controls whether .status changes are tracked.
	// +optional
	// +kubebuilder:default=false
	TrackStatus bool `json:"trackStatus,omitempty"`
}

// TrackingConfig configures which event types to capture.
type TrackingConfig struct {
	// Creations tracks resource creation events.
	// +optional
	// +kubebuilder:default=true
	Creations *bool `json:"creations,omitempty"`

	// Updates tracks resource update events.
	// +optional
	// +kubebuilder:default=true
	Updates *bool `json:"updates,omitempty"`

	// Deletions tracks resource deletion events.
	// +optional
	// +kubebuilder:default=true
	Deletions *bool `json:"deletions,omitempty"`
}

// FlashbackPolicyStatus defines the observed state of FlashbackPolicy.
type FlashbackPolicyStatus struct {
	// TrackedResources is the number of resources currently being tracked.
	TrackedResources int32 `json:"trackedResources,omitempty"`

	// TotalRevisions is the total number of revisions stored.
	TotalRevisions int64 `json:"totalRevisions,omitempty"`

	// StorageUsed is the human-readable storage size used (e.g., "10.5 MiB").
	StorageUsed string `json:"storageUsed,omitempty"`

	// StorageUsedBytes is the storage size in bytes.
	StorageUsedBytes int64 `json:"storageUsedBytes,omitempty"`

	// LastReconcileTime is the last time the policy was reconciled.
	LastReconcileTime *metav1.Time `json:"lastReconcileTime,omitempty"`

	// Conditions represent the latest available observations of the policy's state.
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// ResourceSummaries provides per-resource-type tracking stats.
	// +optional
	ResourceSummaries []ResourceSummary `json:"resourceSummaries,omitempty"`
}

// ResourceSummary provides tracking stats for a specific resource type.
type ResourceSummary struct {
	// APIVersion is the API version being tracked.
	APIVersion string `json:"apiVersion"`

	// Kind is the resource kind being tracked.
	Kind string `json:"kind"`

	// Count is the number of resources of this type being tracked.
	Count int32 `json:"count"`

	// Revisions is the total revisions for this resource type.
	Revisions int64 `json:"revisions"`
}

// +kubebuilder:object:root=true

// FlashbackPolicyList contains a list of FlashbackPolicy.
type FlashbackPolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []FlashbackPolicy `json:"items"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,shortName=fbcfg
// +kubebuilder:printcolumn:name="Backend",type=string,JSONPath=`.spec.storage.backend`
// +kubebuilder:printcolumn:name="API Address",type=string,JSONPath=`.spec.server.apiAddress`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// KFlashbackConfig is the Schema for the kflashbackconfigs API.
// It is a cluster-scoped singleton that configures the kflashback controller.
type KFlashbackConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   KFlashbackConfigSpec   `json:"spec,omitempty"`
	Status KFlashbackConfigStatus `json:"status,omitempty"`
}

// KFlashbackConfigSpec defines the desired configuration for kflashback.
type KFlashbackConfigSpec struct {
	// Storage configures the storage backend.
	// +optional
	Storage KFlashbackStorageSpec `json:"storage,omitempty"`

	// Server configures the API and metrics servers.
	// +optional
	Server KFlashbackServerSpec `json:"server,omitempty"`

	// Controller configures controller behaviour.
	// +optional
	Controller KFlashbackControllerSpec `json:"controller,omitempty"`

	// AI configures AI-powered features (change summaries, anomaly detection, natural language queries).
	// +optional
	AI *KFlashbackAISpec `json:"ai,omitempty"`
}

// KFlashbackAISpec configures the AI/LLM provider.
type KFlashbackAISpec struct {
	// Enabled activates AI features. Requires a valid provider configuration.
	// +optional
	// +kubebuilder:default=false
	Enabled bool `json:"enabled,omitempty"`

	// Provider is the AI provider type (e.g. "openai", "ollama").
	// All providers use the OpenAI-compatible chat completions API.
	// +optional
	// +kubebuilder:default="openai"
	Provider string `json:"provider,omitempty"`

	// Endpoint is the API base URL.
	// OpenAI: https://api.openai.com/v1
	// Ollama: http://localhost:11434/v1
	// +optional
	Endpoint string `json:"endpoint,omitempty"`

	// Model is the model name to use (e.g. "gpt-4o-mini", "llama3", "claude-sonnet-4-20250514").
	// +optional
	// +kubebuilder:default="gpt-4o-mini"
	Model string `json:"model,omitempty"`

	// CredentialsSecret references a Secret containing the API key.
	// The key in the Secret should be "api-key".
	// +optional
	CredentialsSecret *SecretKeyReference `json:"credentialsSecret,omitempty"`

	// MaxTokens is the maximum number of tokens the AI can generate per request.
	// +optional
	// +kubebuilder:default=1024
	MaxTokens int `json:"maxTokens,omitempty"`

	// Temperature controls the randomness of AI responses (0.0 = deterministic, 1.0 = creative).
	// +optional
	// +kubebuilder:default="0.3"
	Temperature string `json:"temperature,omitempty"`

	// ContextMode controls how much cluster data is sent to the AI.
	// "compact" sends a short text summary (fast, works with small local models).
	// "full" sends detailed JSON data (slower, better answers with large cloud models).
	// +optional
	// +kubebuilder:default="compact"
	// +kubebuilder:validation:Enum=compact;full
	ContextMode string `json:"contextMode,omitempty"`
}

// KFlashbackStorageSpec configures the storage backend.
type KFlashbackStorageSpec struct {
	// Backend is the storage backend to use (e.g. "sqlite", "postgres").
	// +optional
	// +kubebuilder:default="sqlite"
	Backend string `json:"backend,omitempty"`

	// DSN is the data source name or file path for the storage backend.
	// For sqlite this is a file path, for postgres a connection string.
	// Avoid putting credentials directly here — use credentialsSecret instead.
	// +optional
	// +kubebuilder:default="/data/kflashback.db"
	DSN string `json:"dsn,omitempty"`

	// CredentialsSecret references a Kubernetes Secret that contains the storage
	// connection string or credentials. This is the recommended way to supply
	// database credentials for production deployments.
	//
	// The controller resolves the DSN in this priority order:
	//   1. KFLASHBACK_STORAGE_DSN environment variable (highest priority)
	//   2. The Secret key referenced here
	//   3. spec.storage.dsn field
	//   4. CLI --storage-dsn flag (lowest priority)
	// +optional
	CredentialsSecret *SecretKeyReference `json:"credentialsSecret,omitempty"`
}

// SecretKeyReference holds a reference to a key within a Kubernetes Secret.
type SecretKeyReference struct {
	// Name is the name of the Secret.
	Name string `json:"name"`

	// Namespace is the namespace of the Secret.
	// Defaults to the kflashback-system namespace if not set.
	// +optional
	Namespace string `json:"namespace,omitempty"`

	// Key is the key within the Secret whose value contains the DSN or credential.
	// +kubebuilder:default="dsn"
	Key string `json:"key"`
}

// KFlashbackServerSpec configures server endpoints.
type KFlashbackServerSpec struct {
	// APIAddress is the bind address for the REST API and UI server.
	// +optional
	// +kubebuilder:default=":9090"
	APIAddress string `json:"apiAddress,omitempty"`

	// MetricsAddress is the bind address for the metrics endpoint.
	// +optional
	// +kubebuilder:default=":8080"
	MetricsAddress string `json:"metricsAddress,omitempty"`

	// HealthAddress is the bind address for the health probe endpoint.
	// +optional
	// +kubebuilder:default=":8081"
	HealthAddress string `json:"healthAddress,omitempty"`
}

// KFlashbackControllerSpec configures controller behaviour.
type KFlashbackControllerSpec struct {
	// LeaderElection enables leader election for HA deployments.
	// +optional
	// +kubebuilder:default=false
	LeaderElection bool `json:"leaderElection,omitempty"`

	// ReconcileInterval is how often policies are re-reconciled for retention cleanup.
	// +optional
	// +kubebuilder:default="5m"
	ReconcileInterval string `json:"reconcileInterval,omitempty"`
}

// KFlashbackConfigStatus defines the observed state of the configuration.
type KFlashbackConfigStatus struct {
	// Active indicates whether this config is being used by the controller.
	Active bool `json:"active,omitempty"`

	// StorageBackend is the currently active storage backend.
	StorageBackend string `json:"storageBackend,omitempty"`

	// Message provides additional information about the config state.
	Message string `json:"message,omitempty"`

	// LastApplied is the last time the config was applied.
	// +optional
	LastApplied *metav1.Time `json:"lastApplied,omitempty"`
}

// +kubebuilder:object:root=true

// KFlashbackConfigList contains a list of KFlashbackConfig.
type KFlashbackConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []KFlashbackConfig `json:"items"`
}

func init() {
	SchemeBuilder.Register(&FlashbackPolicy{}, &FlashbackPolicyList{})
	SchemeBuilder.Register(&KFlashbackConfig{}, &KFlashbackConfigList{})
}
