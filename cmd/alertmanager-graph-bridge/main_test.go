package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/slauger/alertmanager-graph-bridge/internal/config"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestEnvOr(t *testing.T) {
	t.Run("returns the environment value when set", func(t *testing.T) {
		t.Setenv("AGB_TEST_ENVOR", "from-env")
		if got := envOr("AGB_TEST_ENVOR", "fallback"); got != "from-env" {
			t.Errorf("envOr() = %q, want from-env", got)
		}
	})
	t.Run("returns the fallback when unset", func(t *testing.T) {
		if got := envOr("AGB_TEST_ENVOR_UNSET", "fallback"); got != "fallback" {
			t.Errorf("envOr() = %q, want fallback", got)
		}
	})
}

func TestNewLogger(t *testing.T) {
	tests := []struct {
		name string
		cfg  config.LogConfig
	}{
		{name: "json info", cfg: config.LogConfig{Level: "info", Format: "json"}},
		{name: "text debug", cfg: config.LogConfig{Level: "debug", Format: "text"}},
		{name: "json warn", cfg: config.LogConfig{Level: "warn", Format: "json"}},
		{name: "text error", cfg: config.LogConfig{Level: "error", Format: "text"}},
		{name: "unknown level falls back to info", cfg: config.LogConfig{Level: "trace", Format: "json"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if logger := newLogger(tt.cfg); logger == nil {
				t.Fatal("newLogger() returned nil")
			}
		})
	}
}

func TestRegisterBuildInfo(t *testing.T) {
	reg := prometheus.NewRegistry()
	registerBuildInfo(reg)

	expected := fmt.Sprintf(`
# HELP agb_build_info Build information labelled by version and Go version.
# TYPE agb_build_info gauge
agb_build_info{goversion="%s",version="%s"} 1
`, runtime.Version(), version)

	if err := testutil.GatherAndCompare(reg, strings.NewReader(expected), "agb_build_info"); err != nil {
		t.Errorf("agb_build_info metric mismatch: %v", err)
	}
}

func TestRunVersionFlag(t *testing.T) {
	if err := run(context.Background(), []string{"-version"}); err != nil {
		t.Errorf("run(-version) error: %v", err)
	}
}

func TestRunConfigLoadError(t *testing.T) {
	err := run(context.Background(), []string{"-config", filepath.Join(t.TempDir(), "missing.yaml")})
	if err == nil {
		t.Fatal("run() error = nil, want a config load error")
	}
}

func TestRunFlagParseError(t *testing.T) {
	err := run(context.Background(), []string{"-this-flag-does-not-exist"})
	if err == nil {
		t.Fatal("run() error = nil, want a flag parse error")
	}
}

func TestRunStartsAndShutsDown(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "config.yaml")
	content := `
server:
  port: 0
azure:
  tenantId: test-tenant
  clientId: test-client
  clientSecret: test-secret
mail:
  from: monitoring@example.com
  to:
    - ops@example.com
log:
  level: error
  format: text
`
	if err := os.WriteFile(cfgPath, []byte(content), 0o600); err != nil {
		t.Fatalf("writing config: %v", err)
	}

	// A cancelled context makes Run start the listener and shut down at once.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := run(ctx, []string{"-config", cfgPath}); err != nil {
		t.Errorf("run() error: %v", err)
	}
}
