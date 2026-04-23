// Copyright 2024 The kflashback Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"flag"
	"os"
	"os/signal"
	"syscall"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/dynamic"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	flashbackv1alpha1 "github.com/kflashback/kflashback/api/v1alpha1"
	"github.com/kflashback/kflashback/internal/ai"
	"github.com/kflashback/kflashback/internal/config"
	"github.com/kflashback/kflashback/internal/controller"
	"github.com/kflashback/kflashback/internal/server"
	"github.com/kflashback/kflashback/internal/storage"

	// Import storage backends so they self-register via init().
	_ "github.com/kflashback/kflashback/internal/storage/postgres"
	_ "github.com/kflashback/kflashback/internal/storage/sqlite"
)

var scheme = runtime.NewScheme()

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(flashbackv1alpha1.AddToScheme(scheme))
}

func main() {
	var (
		configName           string
		storageBackend       string
		storageDSN           string
		apiAddr              string
		metricsAddr          string
		probeAddr            string
		uiDir                string
		enableLeaderElection bool
		aiEnabled            bool
		aiEndpoint           string
		aiModel              string
		aiAPIKey             string
		aiContextMode        string
	)

	flag.StringVar(&configName, "config-name", "kflashback", "Name of the KFlashbackConfig CR to read (set to empty to skip).")
	flag.StringVar(&storageBackend, "storage-backend", "sqlite", "Storage backend to use (e.g. sqlite, postgres).")
	flag.StringVar(&storageDSN, "storage-dsn", "/data/kflashback.db", "DSN or path for the storage backend.")
	flag.StringVar(&apiAddr, "api-bind-address", ":9090", "The address the API server binds to.")
	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metrics endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the health probe endpoint binds to.")
	flag.StringVar(&uiDir, "ui-dir", "/ui", "Path to the UI static files directory.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false, "Enable leader election for controller manager.")
	flag.BoolVar(&aiEnabled, "ai-enabled", false, "Enable AI-powered features.")
	flag.StringVar(&aiEndpoint, "ai-endpoint", "", "AI provider API endpoint (e.g. http://localhost:11434/v1 for Ollama).")
	flag.StringVar(&aiModel, "ai-model", "qwen3:8b", "AI model name.")
	flag.StringVar(&aiAPIKey, "ai-api-key", "", "AI provider API key (use env KFLASHBACK_AI_API_KEY for secrets).")
	flag.StringVar(&aiContextMode, "ai-context-mode", "compact", "AI context mode: 'compact' (fast, local models) or 'full' (detailed, cloud models).")

	opts := zap.Options{Development: true}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))
	setupLog := ctrl.Log.WithName("setup")

	// Build base config from CLI flags
	cfg := config.Resolved{
		StorageBackend: storageBackend,
		StorageDSN:     storageDSN,
		APIAddress:     apiAddr,
		MetricsAddress: metricsAddr,
		HealthAddress:  probeAddr,
		LeaderElection: enableLeaderElection,
		UIDir:          uiDir,
		AIEnabled:      aiEnabled,
		AIEndpoint:     aiEndpoint,
		AIModel:        aiModel,
		AIAPIKey:       aiAPIKey,
		AIContextMode:  aiContextMode,
	}

	// Attempt to read KFlashbackConfig CR from the cluster
	if configName != "" {
		restCfg, err := ctrl.GetConfig()
		if err == nil {
			directClient, err := client.New(restCfg, client.Options{Scheme: scheme})
			if err == nil {
				resolved, found, err := config.LoadFromCR(context.Background(), directClient, configName, cfg)
				if err != nil {
					setupLog.Info("could not read KFlashbackConfig CR, using CLI flags", "error", err)
				} else if found {
					cfg = resolved
					setupLog.Info("loaded configuration from KFlashbackConfig CR", "name", configName)
				} else {
					setupLog.Info("KFlashbackConfig CR not found, using CLI flags", "name", configName)
				}
			}
		}
	}

	// Environment variable override (works even without a CR)
	config.ResolveEnvOverrides(&cfg)

	// Initialize storage via pluggable backend
	setupLog.Info("available storage backends", "backends", storage.RegisteredBackends())
	store, err := storage.NewStore(cfg.StorageBackend, cfg.StorageDSN)
	if err != nil {
		setupLog.Error(err, "failed to create storage", "backend", cfg.StorageBackend)
		os.Exit(1)
	}
	defer store.Close()

	if err := store.Initialize(context.Background()); err != nil {
		setupLog.Error(err, "failed to initialize storage")
		os.Exit(1)
	}
	setupLog.Info("storage initialized", "backend", cfg.StorageBackend, "dsn", cfg.StorageDSN)

	// Create controller manager
	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                 scheme,
		HealthProbeBindAddress: cfg.HealthAddress,
		Metrics: metricsserver.Options{
			BindAddress: cfg.MetricsAddress,
		},
		LeaderElection:   cfg.LeaderElection,
		LeaderElectionID: "kflashback.flashback.io",
	})
	if err != nil {
		setupLog.Error(err, "unable to create manager")
		os.Exit(1)
	}

	// Create dynamic client
	dynClient, err := dynamic.NewForConfig(mgr.GetConfig())
	if err != nil {
		setupLog.Error(err, "unable to create dynamic client")
		os.Exit(1)
	}

	// Create resource watcher
	watcher := controller.NewResourceWatcher(dynClient, store)

	// Setup controller
	reconciler := &controller.FlashbackPolicyReconciler{
		Client:    mgr.GetClient(),
		Scheme:    mgr.GetScheme(),
		DynClient: dynClient,
		Store:     store,
		Watcher:   watcher,
	}
	if err := reconciler.SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "FlashbackPolicy")
		os.Exit(1)
	}

	// Add health checks
	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	// Start API server
	apiServer := server.New(store, cfg.UIDir, cfg.APIAddress)

	// Configure AI provider if enabled
	if cfg.AIEnabled && cfg.AIEndpoint != "" {
		aiProvider, err := ai.NewProvider(ai.Config{
			Provider:    cfg.AIProvider,
			Endpoint:    cfg.AIEndpoint,
			APIKey:      cfg.AIAPIKey,
			Model:       cfg.AIModel,
			MaxTokens:   cfg.AIMaxTokens,
			Temperature: cfg.AITemperature,
		})
		if err != nil {
			setupLog.Error(err, "failed to create AI provider, AI features disabled")
		} else {
			// Wrap with guardrails for safety
			guarded := ai.NewGuardedProvider(aiProvider, ai.DefaultGuardrails())
			apiServer.SetAIProvider(guarded, cfg.AIContextMode)
			setupLog.Info("AI features enabled with guardrails", "provider", cfg.AIProvider, "model", cfg.AIModel, "endpoint", cfg.AIEndpoint)
		}
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	go func() {
		setupLog.Info("starting API server", "addr", cfg.APIAddress)
		if err := apiServer.Start(ctx); err != nil {
			setupLog.Error(err, "API server error")
		}
	}()

	// Start controller manager
	setupLog.Info("starting controller manager")
	if err := mgr.Start(ctx); err != nil {
		setupLog.Error(err, "controller manager error")
		os.Exit(1)
	}

	watcher.StopAll()
	setupLog.Info("shutdown complete")
}
