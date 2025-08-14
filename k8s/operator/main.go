package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/absmach/propeller/k8s/operator/controller"
	"github.com/absmach/propeller/pkg/mqtt"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	_ = clientgoscheme.AddToScheme(scheme)
}

func main() {
	var metricsAddr string
	var enableLeaderElection bool
	var probeAddr string
	var webhookAddr string
	var domainID string
	var channelID string
	var mqttBroker string

	// Get configuration from environment variables
	metricsAddr = getEnvOrDefault("METRICS_ADDR", ":8080")
	probeAddr = getEnvOrDefault("HEALTH_PROBE_ADDR", ":8081")
	webhookAddr = getEnvOrDefault("WEBHOOK_ADDR", ":9443")
	domainID = getEnvOrDefault("PROPELLER_DOMAIN_ID", "propeller")
	channelID = getEnvOrDefault("PROPELLER_CHANNEL_ID", "control")
	mqttBroker = getEnvOrDefault("MQTT_BROKER", "localhost:1883")
	
	if os.Getenv("ENABLE_LEADER_ELECTION") == "true" {
		enableLeaderElection = true
	}

	opts := zap.Options{
		Development: true,
	}
	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	config, err := ctrl.GetConfig()
	if err != nil {
		setupLog.Error(err, "unable to get kubeconfig")
		os.Exit(1)
	}

	mgr, err := ctrl.NewManager(config, ctrl.Options{
		Scheme:                 scheme,
		Metrics:                server.Options{BindAddress: metricsAddr},
		WebhookServer:          webhook.NewServer(webhook.Options{Host: "0.0.0.0", Port: 9443}),
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "propeller-operator",
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	// Initialize MQTT client for hybrid orchestration
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	pubsub, err := mqtt.NewPubSub(mqttBroker, "", "", logger)
	if err != nil {
		setupLog.Error(err, "unable to create MQTT client")
		os.Exit(1)
	}

	// Setup Proplet controller for managing external proplets
	propletController := controller.NewPropletController(
		mgr.GetClient(),
		logger,
		pubsub,
		domainID,
		channelID,
	)
	
	if err = propletController.SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Proplet")
		os.Exit(1)
	}

	// Setup Task controller for hybrid workload scheduling
	taskController := controller.NewTaskController(
		mgr.GetClient(),
		logger,
		pubsub,
		domainID,
		channelID,
	)
	
	if err = taskController.SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Task")
		os.Exit(1)
	}

	// Setup health check
	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}

func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}