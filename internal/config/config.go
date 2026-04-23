// Copyright 2024 The kflashback Authors
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"context"
	"fmt"
	"os"
	"strconv"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	flashbackv1alpha1 "github.com/kflashback/kflashback/api/v1alpha1"
)

const (
	// EnvStorageDSN is the environment variable that overrides the storage DSN.
	// This is the highest-priority credential source.
	EnvStorageDSN = "KFLASHBACK_STORAGE_DSN"

	// DefaultSecretNamespace is used when credentialsSecret.namespace is empty.
	DefaultSecretNamespace = "kflashback-system"
)

// Resolved holds the final resolved configuration values used by main.
type Resolved struct {
	StorageBackend string
	StorageDSN     string
	APIAddress     string
	MetricsAddress string
	HealthAddress  string
	LeaderElection bool
	UIDir          string

	// AI configuration
	AIEnabled     bool
	AIProvider    string
	AIEndpoint    string
	AIModel       string
	AIAPIKey      string
	AIMaxTokens   int
	AITemperature float64
	AIContextMode string // "compact" or "full"
}

// Defaults returns a Resolved with sensible defaults matching CLI flag defaults.
func Defaults() Resolved {
	return Resolved{
		StorageBackend: "sqlite",
		StorageDSN:     "/data/kflashback.db",
		APIAddress:     ":9090",
		MetricsAddress: ":8080",
		HealthAddress:  ":8081",
		LeaderElection: false,
		UIDir:          "/ui",
	}
}

// LoadFromCR attempts to find and read a KFlashbackConfig CR named configName.
// If found, it merges non-empty fields into the provided Resolved config.
// If not found, it returns the config unchanged with found=false and no error.
//
// Credential resolution priority (highest to lowest):
//  1. KFLASHBACK_STORAGE_DSN environment variable
//  2. Kubernetes Secret referenced by spec.storage.credentialsSecret
//  3. spec.storage.dsn field in the CR
//  4. CLI --storage-dsn flag (the base value)
func LoadFromCR(ctx context.Context, c client.Reader, configName string, base Resolved) (Resolved, bool, error) {
	var cfg flashbackv1alpha1.KFlashbackConfig
	key := client.ObjectKey{Name: configName}
	if err := c.Get(ctx, key, &cfg); err != nil {
		if client.IgnoreNotFound(err) == nil {
			return base, false, nil
		}
		return base, false, fmt.Errorf("reading KFlashbackConfig %q: %w", configName, err)
	}

	// Merge CR values over base (non-empty fields win)
	if cfg.Spec.Storage.Backend != "" {
		base.StorageBackend = cfg.Spec.Storage.Backend
	}
	if cfg.Spec.Storage.DSN != "" {
		base.StorageDSN = cfg.Spec.Storage.DSN
	}

	// Resolve credentials from Secret (overrides spec.storage.dsn)
	if cfg.Spec.Storage.CredentialsSecret != nil {
		secretDSN, err := resolveSecretDSN(ctx, c, cfg.Spec.Storage.CredentialsSecret)
		if err != nil {
			return base, true, fmt.Errorf("resolving credentialsSecret: %w", err)
		}
		if secretDSN != "" {
			base.StorageDSN = secretDSN
		}
	}

	// Environment variable is highest priority (overrides everything)
	if envDSN := os.Getenv(EnvStorageDSN); envDSN != "" {
		base.StorageDSN = envDSN
	}

	if cfg.Spec.Server.APIAddress != "" {
		base.APIAddress = cfg.Spec.Server.APIAddress
	}
	if cfg.Spec.Server.MetricsAddress != "" {
		base.MetricsAddress = cfg.Spec.Server.MetricsAddress
	}
	if cfg.Spec.Server.HealthAddress != "" {
		base.HealthAddress = cfg.Spec.Server.HealthAddress
	}
	if cfg.Spec.Controller.LeaderElection {
		base.LeaderElection = true
	}

	// AI configuration
	if cfg.Spec.AI != nil && cfg.Spec.AI.Enabled {
		base.AIEnabled = true
		if cfg.Spec.AI.Provider != "" {
			base.AIProvider = cfg.Spec.AI.Provider
		}
		if cfg.Spec.AI.Endpoint != "" {
			base.AIEndpoint = cfg.Spec.AI.Endpoint
		}
		if cfg.Spec.AI.Model != "" {
			base.AIModel = cfg.Spec.AI.Model
		}
		if cfg.Spec.AI.MaxTokens > 0 {
			base.AIMaxTokens = cfg.Spec.AI.MaxTokens
		}
		if cfg.Spec.AI.Temperature != "" {
			if t, err := strconv.ParseFloat(cfg.Spec.AI.Temperature, 64); err == nil {
				base.AITemperature = t
			}
		}
		if cfg.Spec.AI.ContextMode != "" {
			base.AIContextMode = cfg.Spec.AI.ContextMode
		}
		// Resolve AI API key from Secret
		if cfg.Spec.AI.CredentialsSecret != nil {
			apiKey, err := resolveSecretDSN(ctx, c, cfg.Spec.AI.CredentialsSecret)
			if err != nil {
				return base, true, fmt.Errorf("resolving AI credentialsSecret: %w", err)
			}
			if apiKey != "" {
				base.AIAPIKey = apiKey
			}
		}
	}

	// Environment variable overrides for AI
	if envKey := os.Getenv("KFLASHBACK_AI_API_KEY"); envKey != "" {
		base.AIAPIKey = envKey
	}

	return base, true, nil
}

// ResolveEnvOverrides applies environment variable overrides regardless of
// whether a CR was found. Call this after LoadFromCR.
func ResolveEnvOverrides(cfg *Resolved) {
	if envDSN := os.Getenv(EnvStorageDSN); envDSN != "" {
		cfg.StorageDSN = envDSN
	}
}

// resolveSecretDSN reads a Kubernetes Secret and returns the value at the specified key.
func resolveSecretDSN(ctx context.Context, c client.Reader, ref *flashbackv1alpha1.SecretKeyReference) (string, error) {
	ns := ref.Namespace
	if ns == "" {
		ns = DefaultSecretNamespace
	}
	key := ref.Key
	if key == "" {
		key = "dsn"
	}

	var secret corev1.Secret
	if err := c.Get(ctx, client.ObjectKey{Name: ref.Name, Namespace: ns}, &secret); err != nil {
		return "", fmt.Errorf("reading Secret %s/%s: %w", ns, ref.Name, err)
	}

	data, ok := secret.Data[key]
	if !ok {
		return "", fmt.Errorf("key %q not found in Secret %s/%s", key, ns, ref.Name)
	}

	return string(data), nil
}
