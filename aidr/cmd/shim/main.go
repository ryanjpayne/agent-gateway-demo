// Package main provides the entry point for the AIDR GCP ext_proc shim.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	extprocv3 "github.com/envoyproxy/go-control-plane/envoy/service/ext_proc/v3"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"

	"github.com/crowdstrike/aidr-go"
	"github.com/crowdstrike/aidr-go/option"

	"github.com/crowdstrike/aidr-gcp-shim/internal/config"
	"github.com/crowdstrike/aidr-gcp-shim/internal/server"
)

func main() {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		slog.Error("failed to load configuration", "error", err)
		os.Exit(1)
	}

	// Set log level
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: cfg.LogLevel,
	}))
	slog.SetDefault(logger)

	// Log startup mode
	mode := "normal"
	if cfg.EchoMode {
		mode = "echo"
	} else if cfg.DebugMode {
		mode = "debug"
	}
	logger.Info("starting AIDR ext_proc shim",
		"mode", mode,
		"debug_mode", cfg.DebugMode,
		"echo_mode", cfg.EchoMode,
	)

	// Initialize AIDR client
	// The base URL template supports {SERVICE_NAME} placeholder
	aidrClient := aidr.NewClient(
		option.WithBaseURLTemplate(cfg.AIDRCloud),
		option.WithToken(cfg.AIDRToken),
	)

	// Create callout service
	calloutService := server.NewCalloutService(server.CalloutServiceParams{
		AIDRClient:          server.NewAIDRClientWrapper(&aidrClient),
		CollectorInstanceID: cfg.CollectorInstanceID,
		Logger:              logger,
		DebugMode:           cfg.DebugMode,
		EchoMode:            cfg.EchoMode,
		FailClosed:          cfg.FailClosed,
	})

	// Create gRPC server.
	// No TLS configured here — Cloud Run terminates TLS at the edge and
	// forwards plaintext to the container over an internal Unix socket.
	grpcServer := grpc.NewServer()
	extprocv3.RegisterExternalProcessorServer(grpcServer, calloutService)

	// Register gRPC health service
	healthServer := health.NewServer()
	healthpb.RegisterHealthServer(grpcServer, healthServer)
	healthServer.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)

	// Enable reflection for debugging (disabled in production)
	if cfg.DebugMode {
		reflection.Register(grpcServer)
	}

	// Create listeners
	grpcListener, err := net.Listen("tcp", fmt.Sprintf(":%d", cfg.GRPCPort))
	if err != nil {
		logger.Error("failed to create gRPC listener", "error", err, "port", cfg.GRPCPort)
		os.Exit(1)
	}

	// Create HTTP health check server
	healthMux := http.NewServeMux()
	healthMux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write([]byte("OK")); err != nil {
			logger.Debug("failed to write health response", "error", err)
		}
	})

	healthHTTPServer := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.HealthPort),
		Handler:      healthMux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// Start servers
	errCh := make(chan error, 2)

	go func() {
		logger.Info("starting gRPC server", "port", cfg.GRPCPort)
		if err := grpcServer.Serve(grpcListener); err != nil {
			errCh <- fmt.Errorf("gRPC server error: %w", err)
		}
	}()

	go func() {
		logger.Info("starting health HTTP server", "port", cfg.HealthPort)
		if err := healthHTTPServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- fmt.Errorf("health HTTP server error: %w", err)
		}
	}()

	// Wait for shutdown signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-sigCh:
		logger.Info("received shutdown signal", "signal", sig)
	case err := <-errCh:
		logger.Error("server error", "error", err)
	}

	// Graceful shutdown with timeout
	logger.Info("shutting down servers")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	// GracefulStop can block indefinitely if a client holds a stream open,
	// so run it in a goroutine and fall back to Stop() on timeout.
	grpcDone := make(chan struct{})
	go func() {
		grpcServer.GracefulStop()
		close(grpcDone)
	}()

	select {
	case <-grpcDone:
		logger.Info("gRPC server stopped gracefully")
	case <-shutdownCtx.Done():
		logger.Warn("gRPC graceful stop timed out, forcing stop")
		grpcServer.Stop()
	}

	if err := healthHTTPServer.Shutdown(shutdownCtx); err != nil {
		logger.Error("error during health server shutdown", "error", err)
	}
	logger.Info("shutdown complete")
}
