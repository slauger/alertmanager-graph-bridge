package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/slauger/alertmanager-graph-bridge/internal/mail"
)

const validYAML = `
server:
  port: 9090
  bearerToken: secret-token
azure:
  tenantId: tenant-1
  clientId: client-1
  clientSecret: client-secret
mail:
  from: alerts@example.com
  to:
    - ops@example.com
    - oncall@example.com
  subjectPrefix: "[Test]"
log:
  level: debug
  format: text
`

func writeConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("writing config: %v", err)
	}
	return path
}

func TestLoadFromYAML(t *testing.T) {
	cfg, err := Load(writeConfig(t, validYAML))
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if cfg.Server.Port != 9090 {
		t.Errorf("Server.Port = %d, want 9090", cfg.Server.Port)
	}
	if cfg.Server.BearerToken != "secret-token" {
		t.Errorf("Server.BearerToken = %q", cfg.Server.BearerToken)
	}
	if cfg.Azure.TenantID != "tenant-1" {
		t.Errorf("Azure.TenantID = %q", cfg.Azure.TenantID)
	}
	if len(cfg.Mail.To) != 2 {
		t.Errorf("len(Mail.To) = %d, want 2", len(cfg.Mail.To))
	}
	if cfg.Mail.SubjectPrefix != "[Test]" {
		t.Errorf("Mail.SubjectPrefix = %q", cfg.Mail.SubjectPrefix)
	}
	if cfg.Log.Level != "debug" || cfg.Log.Format != "text" {
		t.Errorf("Log = %+v", cfg.Log)
	}
	// Defaults must still fill in unset fields.
	if cfg.Server.ReadTimeout.Std() != 10*time.Second {
		t.Errorf("Server.ReadTimeout = %v, want default 10s", cfg.Server.ReadTimeout.Std())
	}
	if cfg.Mail.SendTimeout.Std() != 20*time.Second {
		t.Errorf("Mail.SendTimeout = %v, want default 20s", cfg.Mail.SendTimeout.Std())
	}
}

func TestLoadDurationsFromYAML(t *testing.T) {
	const yamlCfg = `
server:
  port: 8080
  readTimeout: 5s
  writeTimeout: 50s
  shutdownGrace: 1m
azure:
  tenantId: t
  clientId: c
  clientSecret: s
mail:
  from: from@example.com
  to:
    - to@example.com
  sendTimeout: 25s
`
	cfg, err := Load(writeConfig(t, yamlCfg))
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.Server.ReadTimeout.Std() != 5*time.Second {
		t.Errorf("ReadTimeout = %v, want 5s", cfg.Server.ReadTimeout.Std())
	}
	if cfg.Server.WriteTimeout.Std() != 50*time.Second {
		t.Errorf("WriteTimeout = %v, want 50s", cfg.Server.WriteTimeout.Std())
	}
	if cfg.Server.ShutdownGrace.Std() != time.Minute {
		t.Errorf("ShutdownGrace = %v, want 1m", cfg.Server.ShutdownGrace.Std())
	}
	if cfg.Mail.SendTimeout.Std() != 25*time.Second {
		t.Errorf("SendTimeout = %v, want 25s", cfg.Mail.SendTimeout.Std())
	}
}

func TestLoadRejectsInvalidDuration(t *testing.T) {
	const yamlCfg = `
server:
  readTimeout: "not-a-duration"
azure:
  tenantId: t
  clientId: c
  clientSecret: s
mail:
  from: from@example.com
  to:
    - to@example.com
`
	if _, err := Load(writeConfig(t, yamlCfg)); err == nil {
		t.Fatal("Load() error = nil, want an invalid-duration error")
	}
}

func TestDurationEnvOverride(t *testing.T) {
	t.Setenv("AGB_AZURE_TENANTID", "t")
	t.Setenv("AGB_AZURE_CLIENTID", "c")
	t.Setenv("AGB_AZURE_CLIENTSECRET", "s")
	t.Setenv("AGB_MAIL_FROM", "from@example.com")
	t.Setenv("AGB_MAIL_TO", "to@example.com")
	t.Setenv("AGB_SERVER_READTIMEOUT", "7s")
	t.Setenv("AGB_MAIL_SENDTIMEOUT", "15s")

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.Server.ReadTimeout.Std() != 7*time.Second {
		t.Errorf("ReadTimeout = %v, want 7s", cfg.Server.ReadTimeout.Std())
	}
	if cfg.Mail.SendTimeout.Std() != 15*time.Second {
		t.Errorf("SendTimeout = %v, want 15s", cfg.Mail.SendTimeout.Std())
	}
}

func TestLoadDefaultsWithEnv(t *testing.T) {
	t.Setenv("AGB_AZURE_TENANTID", "t")
	t.Setenv("AGB_AZURE_CLIENTID", "c")
	t.Setenv("AGB_AZURE_CLIENTSECRET", "s")
	t.Setenv("AGB_MAIL_FROM", "from@example.com")
	t.Setenv("AGB_MAIL_TO", "a@example.com, b@example.com")

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load(\"\") error: %v", err)
	}
	if cfg.Server.Port != 8080 {
		t.Errorf("Server.Port = %d, want default 8080", cfg.Server.Port)
	}
	if cfg.Log.Format != "json" {
		t.Errorf("Log.Format = %q, want default json", cfg.Log.Format)
	}
	if len(cfg.Mail.To) != 2 {
		t.Errorf("Mail.To = %v, want 2 entries", cfg.Mail.To)
	}
}

func TestLoadTemplate(t *testing.T) {
	t.Setenv("AGB_AZURE_TENANTID", "t")
	t.Setenv("AGB_AZURE_CLIENTID", "c")
	t.Setenv("AGB_AZURE_CLIENTSECRET", "s")
	t.Setenv("AGB_MAIL_FROM", "from@example.com")
	t.Setenv("AGB_MAIL_TO", "to@example.com")

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.Mail.Template != mail.TemplateModern {
		t.Errorf("Mail.Template = %q, want default %q", cfg.Mail.Template, mail.TemplateModern)
	}

	// AGB_MAIL_TEMPLATE overrides the default.
	t.Setenv("AGB_MAIL_TEMPLATE", mail.TemplateClassic)
	cfg, err = Load("")
	if err != nil {
		t.Fatalf("Load() with template override error: %v", err)
	}
	if cfg.Mail.Template != mail.TemplateClassic {
		t.Errorf("Mail.Template = %q, want %q", cfg.Mail.Template, mail.TemplateClassic)
	}
}

func TestLoadRejectsUnknownTemplate(t *testing.T) {
	t.Setenv("AGB_AZURE_TENANTID", "t")
	t.Setenv("AGB_AZURE_CLIENTID", "c")
	t.Setenv("AGB_AZURE_CLIENTSECRET", "s")
	t.Setenv("AGB_MAIL_FROM", "from@example.com")
	t.Setenv("AGB_MAIL_TO", "to@example.com")
	t.Setenv("AGB_MAIL_TEMPLATE", "fancy")

	if _, err := Load(""); err == nil {
		t.Fatal("Load() error = nil, want an unknown-template error")
	}
}

func TestLoadMissingFile(t *testing.T) {
	_, err := Load(filepath.Join(t.TempDir(), "missing.yaml"))
	if err == nil {
		t.Fatal("Load() error = nil, want error for missing file")
	}
}

func TestLoadMalformedYAML(t *testing.T) {
	// A tab used for indentation is invalid YAML.
	_, err := Load(writeConfig(t, "server:\n\tport: 8080\n"))
	if err == nil {
		t.Fatal("Load() error = nil, want error for malformed YAML")
	}
}

func TestEnvPrecedence(t *testing.T) {
	t.Setenv("AGB_SERVER_PORT", "7070")
	t.Setenv("AGB_AZURE_CLIENTSECRET", "env-secret")
	t.Setenv("AGB_MAIL_TO", "env@example.com")

	cfg, err := Load(writeConfig(t, validYAML))
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.Server.Port != 7070 {
		t.Errorf("Server.Port = %d, want 7070 (env overrides YAML)", cfg.Server.Port)
	}
	if cfg.Azure.ClientSecret != "env-secret" {
		t.Errorf("Azure.ClientSecret = %q, want env-secret", cfg.Azure.ClientSecret)
	}
	if len(cfg.Mail.To) != 1 || cfg.Mail.To[0] != "env@example.com" {
		t.Errorf("Mail.To = %v, want [env@example.com]", cfg.Mail.To)
	}
}

func TestApplyEnvParseErrors(t *testing.T) {
	cfg := Default()
	getenv := func(key string) string {
		switch key {
		case "AGB_SERVER_PORT":
			return "not-a-number"
		case "AGB_MAIL_SAVETOSENTITEMS":
			return "not-a-bool"
		case "AGB_SERVER_READTIMEOUT":
			return "not-a-duration"
		default:
			return ""
		}
	}
	if err := cfg.applyEnv(getenv); err == nil {
		t.Fatal("applyEnv() error = nil, want parse errors")
	}
}

func TestApplyDefaults(t *testing.T) {
	c := Config{}
	c.applyDefaults()

	d := Default()
	if c.Server.Port != d.Server.Port {
		t.Errorf("Server.Port = %d, want %d", c.Server.Port, d.Server.Port)
	}
	if c.Server.ReadTimeout != d.Server.ReadTimeout {
		t.Errorf("Server.ReadTimeout = %v, want %v", c.Server.ReadTimeout, d.Server.ReadTimeout)
	}
	if c.Server.WriteTimeout != d.Server.WriteTimeout {
		t.Errorf("Server.WriteTimeout = %v, want %v", c.Server.WriteTimeout, d.Server.WriteTimeout)
	}
	if c.Server.ShutdownGrace != d.Server.ShutdownGrace {
		t.Errorf("Server.ShutdownGrace = %v, want %v", c.Server.ShutdownGrace, d.Server.ShutdownGrace)
	}
	if c.Mail.SubjectPrefix != d.Mail.SubjectPrefix {
		t.Errorf("Mail.SubjectPrefix = %q, want %q", c.Mail.SubjectPrefix, d.Mail.SubjectPrefix)
	}
	if c.Mail.Template != d.Mail.Template {
		t.Errorf("Mail.Template = %q, want %q", c.Mail.Template, d.Mail.Template)
	}
	if c.Mail.SendTimeout != d.Mail.SendTimeout {
		t.Errorf("Mail.SendTimeout = %v, want %v", c.Mail.SendTimeout, d.Mail.SendTimeout)
	}
	if c.Log.Level != d.Log.Level || c.Log.Format != d.Log.Format {
		t.Errorf("Log = %+v, want %+v", c.Log, d.Log)
	}
}

func TestLoadRejectsInvalidEnv(t *testing.T) {
	t.Setenv("AGB_AZURE_TENANTID", "t")
	t.Setenv("AGB_AZURE_CLIENTID", "c")
	t.Setenv("AGB_AZURE_CLIENTSECRET", "s")
	t.Setenv("AGB_MAIL_FROM", "from@example.com")
	t.Setenv("AGB_MAIL_TO", "to@example.com")
	t.Setenv("AGB_SERVER_PORT", "not-a-number")

	if _, err := Load(""); err == nil {
		t.Fatal("Load() error = nil, want an environment parse error")
	}
}

func TestValidate(t *testing.T) {
	base := func() Config {
		c := Default()
		c.Azure = AzureConfig{TenantID: "t", ClientID: "c", ClientSecret: "s"}
		c.Mail.From = "from@example.com"
		c.Mail.To = []string{"to@example.com"}
		return c
	}

	tests := []struct {
		name    string
		mutate  func(*Config)
		wantErr bool
	}{
		{name: "valid", mutate: func(*Config) {}},
		{name: "port too low", mutate: func(c *Config) { c.Server.Port = 0 }, wantErr: true},
		{name: "port too high", mutate: func(c *Config) { c.Server.Port = 70000 }, wantErr: true},
		{name: "missing tenant", mutate: func(c *Config) { c.Azure.TenantID = "" }, wantErr: true},
		{name: "missing clientID", mutate: func(c *Config) { c.Azure.ClientID = "" }, wantErr: true},
		{name: "missing secret", mutate: func(c *Config) { c.Azure.ClientSecret = "" }, wantErr: true},
		{name: "missing from", mutate: func(c *Config) { c.Mail.From = "" }, wantErr: true},
		{name: "invalid from", mutate: func(c *Config) { c.Mail.From = "not-an-email" }, wantErr: true},
		{name: "empty recipients", mutate: func(c *Config) { c.Mail.To = nil }, wantErr: true},
		{name: "invalid recipient", mutate: func(c *Config) { c.Mail.To = []string{"bad"} }, wantErr: true},
		{name: "bad log level", mutate: func(c *Config) { c.Log.Level = "trace" }, wantErr: true},
		{name: "bad log format", mutate: func(c *Config) { c.Log.Format = "xml" }, wantErr: true},
		{name: "bad template", mutate: func(c *Config) { c.Mail.Template = "fancy" }, wantErr: true},
		{name: "classic template", mutate: func(c *Config) { c.Mail.Template = mail.TemplateClassic }},
		{
			name:    "writeTimeout not greater than sendTimeout",
			mutate:  func(c *Config) { c.Server.WriteTimeout = c.Mail.SendTimeout },
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := base()
			tt.mutate(&c)
			err := c.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr = %v", err, tt.wantErr)
			}
		})
	}
}
