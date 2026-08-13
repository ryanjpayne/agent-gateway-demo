package config

import (
	"log/slog"
	"os"
	"testing"
)

// allConfigEnvVars lists every env var read by Load so tests can unset them all
// before exercising specific combinations.
var allConfigEnvVars = []string{
	"AIDR_CLOUD",
	"AIDR_TOKEN",
	"GRPC_PORT",
	"HEALTH_PORT",
	"LOG_LEVEL",
	"DEBUG_MODE",
	"ECHO_MODE",
	"ALLOW_ECHO_MODE",
	"FAILURE_MODE",
	"COLLECTOR_INSTANCE_ID",
}

// clearConfigEnv unsets all config env vars and registers cleanup to restore them.
func clearConfigEnv(t *testing.T) {
	t.Helper()
	for _, key := range allConfigEnvVars {
		prev, hadPrev := os.LookupEnv(key)
		os.Unsetenv(key)
		if hadPrev {
			t.Cleanup(func() { os.Setenv(key, prev) })
		}
	}
}

func TestLoad_RequiredFields(t *testing.T) {
	clearConfigEnv(t)

	// Missing AIDR_CLOUD should fail
	_, err := Load()
	if err == nil {
		t.Fatal("expected error when AIDR_CLOUD is missing")
	}

	// Set AIDR_CLOUD but missing AIDR_TOKEN
	t.Setenv("AIDR_CLOUD", "us-1")
	_, err = Load()
	if err == nil {
		t.Fatal("expected error when AIDR_TOKEN is missing")
	}

	// Set both required fields
	t.Setenv("AIDR_TOKEN", "test-token")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.AIDRCloud != "https://api.crowdstrike.com/aidr/aiguard" {
		t.Errorf("unexpected base URL: %s", cfg.AIDRCloud)
	}
	if cfg.AIDRToken != "test-token" {
		t.Errorf("unexpected token: %s", cfg.AIDRToken)
	}
}

func TestLoad_InvalidCloud(t *testing.T) {
	clearConfigEnv(t)
	t.Setenv("AIDR_CLOUD", "invalid-region")
	t.Setenv("AIDR_TOKEN", "test-token")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for invalid AIDR_CLOUD")
	}
}

func TestLoad_CloudRegions(t *testing.T) {
	tests := []struct {
		cloud   string
		wantURL string
	}{
		{"us-1", "https://api.crowdstrike.com/aidr/aiguard"},
		{"us-2", "https://api.us-2.crowdstrike.com/aidr/aiguard"},
		{"eu-1", "https://api.eu-1.crowdstrike.com/aidr/aiguard"},
		{"us-gov-1", "https://api.laggar.gcw.crowdstrike.com/aidr/aiguard"},
		{"us-gov-2", "https://api.us-gov-2.crowdstrike.mil/aidr/aiguard"},
	}

	for _, tt := range tests {
		t.Run(tt.cloud, func(t *testing.T) {
			clearConfigEnv(t)
			t.Setenv("AIDR_CLOUD", tt.cloud)
			t.Setenv("AIDR_TOKEN", "test-token")

			cfg, err := Load()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if cfg.AIDRCloud != tt.wantURL {
				t.Errorf("AIDR_CLOUD=%s: got base URL %q, want %q", tt.cloud, cfg.AIDRCloud, tt.wantURL)
			}
		})
	}
}

func TestLoad_Defaults(t *testing.T) {
	clearConfigEnv(t)
	t.Setenv("AIDR_CLOUD", "us-1")
	t.Setenv("AIDR_TOKEN", "test-token")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.GRPCPort != 8080 {
		t.Errorf("expected default GRPC port 8080, got %d", cfg.GRPCPort)
	}
	if cfg.HealthPort != 8081 {
		t.Errorf("expected default health port 8081, got %d", cfg.HealthPort)
	}
	if cfg.LogLevel != slog.LevelInfo {
		t.Errorf("expected default log level info, got %s", cfg.LogLevel)
	}
	if cfg.CollectorInstanceID != "" {
		t.Errorf("expected empty collector instance ID, got %s", cfg.CollectorInstanceID)
	}
}

func TestLoad_CustomPorts(t *testing.T) {
	clearConfigEnv(t)
	t.Setenv("AIDR_CLOUD", "us-1")
	t.Setenv("AIDR_TOKEN", "test-token")
	t.Setenv("GRPC_PORT", "9090")
	t.Setenv("HEALTH_PORT", "9091")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.GRPCPort != 9090 {
		t.Errorf("expected GRPC port 9090, got %d", cfg.GRPCPort)
	}
	if cfg.HealthPort != 9091 {
		t.Errorf("expected health port 9091, got %d", cfg.HealthPort)
	}
}

func TestLoad_InvalidPorts(t *testing.T) {
	clearConfigEnv(t)
	t.Setenv("AIDR_CLOUD", "us-1")
	t.Setenv("AIDR_TOKEN", "test-token")
	t.Setenv("GRPC_PORT", "invalid")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for invalid GRPC_PORT")
	}

	t.Setenv("GRPC_PORT", "8080")
	t.Setenv("HEALTH_PORT", "invalid")

	_, err = Load()
	if err == nil {
		t.Fatal("expected error for invalid HEALTH_PORT")
	}
}

func TestLoad_OptionalFields(t *testing.T) {
	clearConfigEnv(t)
	t.Setenv("AIDR_CLOUD", "us-1")
	t.Setenv("AIDR_TOKEN", "test-token")
	t.Setenv("COLLECTOR_INSTANCE_ID", "my-instance")
	t.Setenv("LOG_LEVEL", "debug")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.CollectorInstanceID != "my-instance" {
		t.Errorf("expected collector instance ID 'my-instance', got %s", cfg.CollectorInstanceID)
	}
	if cfg.LogLevel != slog.LevelDebug {
		t.Errorf("expected log level debug, got %s", cfg.LogLevel)
	}
}

func TestLoad_DebugModes(t *testing.T) {
	clearConfigEnv(t)
	t.Setenv("AIDR_CLOUD", "us-1")
	t.Setenv("AIDR_TOKEN", "test-token")

	// Default: both disabled
	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.DebugMode {
		t.Error("expected DebugMode to be false by default")
	}
	if cfg.EchoMode {
		t.Error("expected EchoMode to be false by default")
	}

	// Enable debug mode
	t.Setenv("DEBUG_MODE", "true")
	cfg, err = Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cfg.DebugMode {
		t.Error("expected DebugMode to be true when DEBUG_MODE=true")
	}

	// Enable echo mode
	t.Setenv("ECHO_MODE", "true")
	t.Setenv("ALLOW_ECHO_MODE", "true")
	cfg, err = Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cfg.EchoMode {
		t.Error("expected EchoMode to be true when ECHO_MODE=true")
	}

	// Non-true values should be false
	t.Setenv("DEBUG_MODE", "false")
	t.Setenv("ECHO_MODE", "1")
	os.Unsetenv("ALLOW_ECHO_MODE")
	cfg, err = Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.DebugMode {
		t.Error("expected DebugMode to be false when DEBUG_MODE=false")
	}
	if cfg.EchoMode {
		t.Error("expected EchoMode to be false when ECHO_MODE=1 (must be 'true')")
	}
}

func TestLoad_FailureMode(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		want    bool
		wantErr bool
	}{
		{"default is allow", "", false, false},
		{"explicit allow", "allow", false, false},
		{"explicit deny", "deny", true, false},
		{"invalid value", "block", false, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearConfigEnv(t)
			t.Setenv("AIDR_CLOUD", "us-1")
			t.Setenv("AIDR_TOKEN", "test-token")
			if tt.value != "" {
				t.Setenv("FAILURE_MODE", tt.value)
			}

			cfg, err := Load()
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error for invalid FAILURE_MODE")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if cfg.FailClosed != tt.want {
				t.Errorf("FailClosed = %v, want %v", cfg.FailClosed, tt.want)
			}
		})
	}
}

func TestLoad_EchoModeGuard(t *testing.T) {
	tests := []struct {
		name          string
		echoMode      string
		allowEchoMode string
		wantErr       bool
	}{
		{"echo off needs no guard", "false", "", false},
		{"echo on without guard fails", "true", "", true},
		{"echo on with guard succeeds", "true", "true", false},
		{"echo on with guard=false fails", "true", "false", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearConfigEnv(t)
			t.Setenv("AIDR_CLOUD", "us-1")
			t.Setenv("AIDR_TOKEN", "test-token")
			if tt.echoMode != "" {
				t.Setenv("ECHO_MODE", tt.echoMode)
			}
			if tt.allowEchoMode != "" {
				t.Setenv("ALLOW_ECHO_MODE", tt.allowEchoMode)
			}

			_, err := Load()
			if tt.wantErr && err == nil {
				t.Fatal("expected error for unguarded ECHO_MODE")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestLoad_LogLevelValidation(t *testing.T) {
	tests := []struct {
		name    string
		level   string
		want    slog.Level
		wantErr bool
	}{
		{"default is info", "", slog.LevelInfo, false},
		{"debug", "debug", slog.LevelDebug, false},
		{"info", "info", slog.LevelInfo, false},
		{"warn", "warn", slog.LevelWarn, false},
		{"error", "error", slog.LevelError, false},
		{"invalid value", "trace", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearConfigEnv(t)
			t.Setenv("AIDR_CLOUD", "us-1")
			t.Setenv("AIDR_TOKEN", "test-token")
			if tt.level != "" {
				t.Setenv("LOG_LEVEL", tt.level)
			}

			cfg, err := Load()
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error for invalid LOG_LEVEL")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if cfg.LogLevel != tt.want {
				t.Errorf("LogLevel = %v, want %v", cfg.LogLevel, tt.want)
			}
		})
	}
}

func TestLoad_PortRangeValidation(t *testing.T) {
	tests := []struct {
		name    string
		envVar  string
		value   string
		wantErr bool
	}{
		{"grpc port 0", "GRPC_PORT", "0", true},
		{"grpc port -1", "GRPC_PORT", "-1", true},
		{"grpc port 65536", "GRPC_PORT", "65536", true},
		{"grpc port 1", "GRPC_PORT", "1", false},
		{"grpc port 65535", "GRPC_PORT", "65535", false},
		{"health port 0", "HEALTH_PORT", "0", true},
		{"health port 99999", "HEALTH_PORT", "99999", true},
		{"health port 443", "HEALTH_PORT", "443", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearConfigEnv(t)
			t.Setenv("AIDR_CLOUD", "us-1")
			t.Setenv("AIDR_TOKEN", "test-token")
			t.Setenv(tt.envVar, tt.value)

			_, err := Load()
			if tt.wantErr && err == nil {
				t.Fatalf("expected error for %s=%s", tt.envVar, tt.value)
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error for %s=%s: %v", tt.envVar, tt.value, err)
			}
		})
	}
}

func TestLoad_WhitespaceToken(t *testing.T) {
	clearConfigEnv(t)
	t.Setenv("AIDR_CLOUD", "us-1")
	t.Setenv("AIDR_TOKEN", "   ")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for whitespace-only AIDR_TOKEN")
	}
}
