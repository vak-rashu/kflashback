// Copyright 2024 The kflashback Authors
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"os"
	"testing"
)

func TestDefaults(t *testing.T) {
	d := Defaults()
	if d.StorageBackend != "sqlite" {
		t.Errorf("StorageBackend = %q, want sqlite", d.StorageBackend)
	}
	if d.APIAddress != ":9090" {
		t.Errorf("APIAddress = %q, want :9090", d.APIAddress)
	}
	if d.LeaderElection {
		t.Error("LeaderElection should default to false")
	}
}

func TestResolveEnvOverrides(t *testing.T) {
	cfg := Defaults()

	// No env var set — should not change
	ResolveEnvOverrides(&cfg)
	if cfg.StorageDSN != "/data/kflashback.db" {
		t.Errorf("StorageDSN = %q, want default", cfg.StorageDSN)
	}

	// Set env var — should override
	os.Setenv(EnvStorageDSN, "postgres://localhost/test")
	defer os.Unsetenv(EnvStorageDSN)

	ResolveEnvOverrides(&cfg)
	if cfg.StorageDSN != "postgres://localhost/test" {
		t.Errorf("StorageDSN = %q, want postgres://localhost/test", cfg.StorageDSN)
	}
}

func TestResolveEnvOverrides_EmptyNoChange(t *testing.T) {
	os.Unsetenv(EnvStorageDSN)
	cfg := Resolved{StorageDSN: "original"}
	ResolveEnvOverrides(&cfg)
	if cfg.StorageDSN != "original" {
		t.Errorf("StorageDSN changed to %q, should remain original", cfg.StorageDSN)
	}
}
