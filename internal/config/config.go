// Package config loads the alertmanager-graph-bridge configuration from a YAML
// file and applies environment-variable overrides.
package config

import (
	"errors"
	"fmt"
	netmail "net/mail"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/slauger/alertmanager-graph-bridge/internal/branding"
	"github.com/slauger/alertmanager-graph-bridge/internal/mail"
	"gopkg.in/yaml.v3"
)

// Config is the top-level application configuration.
type Config struct {
	Server ServerConfig `yaml:"server"`
	Azure  AzureConfig  `yaml:"azure"`
	Mail   MailConfig   `yaml:"mail"`
	Log    LogConfig    `yaml:"log"`
}

// Duration is a time.Duration that unmarshals from a human-readable string
// such as "10s" in YAML (gopkg.in/yaml.v3 only decodes time.Duration from a
// raw nanosecond integer).
type Duration time.Duration

// Std returns the value as a standard time.Duration.
func (d Duration) Std() time.Duration { return time.Duration(d) }

// String renders the duration in Go's human-readable form.
func (d Duration) String() string { return time.Duration(d).String() }

// UnmarshalYAML decodes a duration string such as "10s" or "1m30s".
func (d *Duration) UnmarshalYAML(node *yaml.Node) error {
	var s string
	if err := node.Decode(&s); err != nil {
		return fmt.Errorf(`duration must be a string such as "10s": %w`, err)
	}
	parsed, err := time.ParseDuration(s)
	if err != nil {
		return fmt.Errorf("invalid duration %q: %w", s, err)
	}
	*d = Duration(parsed)
	return nil
}

// MarshalYAML renders the duration as a human-readable string.
func (d Duration) MarshalYAML() (any, error) {
	return time.Duration(d).String(), nil
}

// ServerConfig configures the HTTP server.
type ServerConfig struct {
	Port          int      `yaml:"port"`
	BearerToken   string   `yaml:"bearerToken"`
	ReadTimeout   Duration `yaml:"readTimeout"`
	WriteTimeout  Duration `yaml:"writeTimeout"`
	ShutdownGrace Duration `yaml:"shutdownGrace"`
}

// AzureConfig configures access to the Microsoft Graph API.
type AzureConfig struct {
	TenantID     string `yaml:"tenantId"`
	ClientID     string `yaml:"clientId"`
	ClientSecret string `yaml:"clientSecret"`
	// TokenURL and GraphBaseURL override the default Azure endpoints. They are
	// primarily useful for testing; leave empty for production.
	TokenURL     string `yaml:"tokenUrl"`
	GraphBaseURL string `yaml:"graphBaseUrl"`
}

// MailConfig configures e-mail delivery. Template selects the e-mail layout
// and is one of mail.TemplateModern or mail.TemplateClassic.
type MailConfig struct {
	From            string   `yaml:"from"`
	To              []string `yaml:"to"`
	SubjectPrefix   string   `yaml:"subjectPrefix"`
	Template        string   `yaml:"template"`
	SaveToSentItems bool     `yaml:"saveToSentItems"`
	SendTimeout     Duration `yaml:"sendTimeout"`
}

// LogConfig configures structured logging.
type LogConfig struct {
	Level  string `yaml:"level"`
	Format string `yaml:"format"`
}

// Default returns a Config populated with all default values.
func Default() Config {
	return Config{
		Server: ServerConfig{
			Port:          8080,
			ReadTimeout:   Duration(10 * time.Second),
			WriteTimeout:  Duration(30 * time.Second),
			ShutdownGrace: Duration(15 * time.Second),
		},
		Mail: MailConfig{
			SubjectPrefix:   "[" + branding.ProductName + "]",
			Template:        mail.TemplateModern,
			SaveToSentItems: false,
			SendTimeout:     Duration(20 * time.Second),
		},
		Log: LogConfig{
			Level:  "info",
			Format: "json",
		},
	}
}

// Load reads configuration from the YAML file at path, applies environment
// variable overrides, fills in defaults and validates the result.
//
// If path is empty, only defaults and environment overrides are applied.
func Load(path string) (*Config, error) {
	cfg := Default()

	if path != "" {
		// G304: the config path is supplied by the operator (flag / env var),
		// so reading it is the intended behaviour.
		raw, err := os.ReadFile(path) //nolint:gosec
		if err != nil {
			return nil, fmt.Errorf("config: reading %s: %w", path, err)
		}
		if err := yaml.Unmarshal(raw, &cfg); err != nil {
			return nil, fmt.Errorf("config: parsing %s: %w", path, err)
		}
	}

	if err := cfg.applyEnv(os.Getenv); err != nil {
		return nil, err
	}
	cfg.applyDefaults()

	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// applyDefaults fills in any fields left at their zero value after loading.
func (c *Config) applyDefaults() {
	d := Default()
	if c.Server.Port == 0 {
		c.Server.Port = d.Server.Port
	}
	if c.Server.ReadTimeout == 0 {
		c.Server.ReadTimeout = d.Server.ReadTimeout
	}
	if c.Server.WriteTimeout == 0 {
		c.Server.WriteTimeout = d.Server.WriteTimeout
	}
	if c.Server.ShutdownGrace == 0 {
		c.Server.ShutdownGrace = d.Server.ShutdownGrace
	}
	if c.Mail.SubjectPrefix == "" {
		c.Mail.SubjectPrefix = d.Mail.SubjectPrefix
	}
	if c.Mail.Template == "" {
		c.Mail.Template = d.Mail.Template
	}
	if c.Mail.SendTimeout == 0 {
		c.Mail.SendTimeout = d.Mail.SendTimeout
	}
	if c.Log.Level == "" {
		c.Log.Level = d.Log.Level
	}
	if c.Log.Format == "" {
		c.Log.Format = d.Log.Format
	}
}

// applyEnv applies AGB_-prefixed environment variable overrides. The getenv
// function is injectable for testing.
func (c *Config) applyEnv(getenv func(string) string) error {
	var errs []error

	setString := func(key string, dst *string) {
		if v := getenv(key); v != "" {
			*dst = v
		}
	}
	setInt := func(key string, dst *int) {
		v := getenv(key)
		if v == "" {
			return
		}
		n, err := strconv.Atoi(v)
		if err != nil {
			errs = append(errs, fmt.Errorf("config: %s: %w", key, err))
			return
		}
		*dst = n
	}
	setBool := func(key string, dst *bool) {
		v := getenv(key)
		if v == "" {
			return
		}
		b, err := strconv.ParseBool(v)
		if err != nil {
			errs = append(errs, fmt.Errorf("config: %s: %w", key, err))
			return
		}
		*dst = b
	}
	setDuration := func(key string, dst *Duration) {
		v := getenv(key)
		if v == "" {
			return
		}
		parsed, err := time.ParseDuration(v)
		if err != nil {
			errs = append(errs, fmt.Errorf("config: %s: %w", key, err))
			return
		}
		*dst = Duration(parsed)
	}

	setInt("AGB_SERVER_PORT", &c.Server.Port)
	setString("AGB_SERVER_BEARERTOKEN", &c.Server.BearerToken)
	setDuration("AGB_SERVER_READTIMEOUT", &c.Server.ReadTimeout)
	setDuration("AGB_SERVER_WRITETIMEOUT", &c.Server.WriteTimeout)
	setDuration("AGB_SERVER_SHUTDOWNGRACE", &c.Server.ShutdownGrace)
	setString("AGB_AZURE_TENANTID", &c.Azure.TenantID)
	setString("AGB_AZURE_CLIENTID", &c.Azure.ClientID)
	setString("AGB_AZURE_CLIENTSECRET", &c.Azure.ClientSecret)
	setString("AGB_AZURE_TOKENURL", &c.Azure.TokenURL)
	setString("AGB_AZURE_GRAPHBASEURL", &c.Azure.GraphBaseURL)
	setString("AGB_MAIL_FROM", &c.Mail.From)
	setString("AGB_MAIL_SUBJECTPREFIX", &c.Mail.SubjectPrefix)
	setString("AGB_MAIL_TEMPLATE", &c.Mail.Template)
	setBool("AGB_MAIL_SAVETOSENTITEMS", &c.Mail.SaveToSentItems)
	setDuration("AGB_MAIL_SENDTIMEOUT", &c.Mail.SendTimeout)
	setString("AGB_LOG_LEVEL", &c.Log.Level)
	setString("AGB_LOG_FORMAT", &c.Log.Format)

	if v := getenv("AGB_MAIL_TO"); v != "" {
		c.Mail.To = splitList(v)
	}

	return errors.Join(errs...)
}

// Validate checks the configuration for consistency and completeness.
func (c *Config) Validate() error {
	var errs []error

	if c.Server.Port < 1 || c.Server.Port > 65535 {
		errs = append(errs, fmt.Errorf("config: server.port %d out of range 1-65535", c.Server.Port))
	}
	if c.Azure.TenantID == "" {
		errs = append(errs, errors.New("config: azure.tenantId is required"))
	}
	if c.Azure.ClientID == "" {
		errs = append(errs, errors.New("config: azure.clientId is required"))
	}
	if c.Azure.ClientSecret == "" {
		errs = append(errs, errors.New("config: azure.clientSecret is required"))
	}
	if c.Mail.From == "" {
		errs = append(errs, errors.New("config: mail.from is required"))
	} else if _, err := netmail.ParseAddress(c.Mail.From); err != nil {
		errs = append(errs, fmt.Errorf("config: mail.from %q is invalid: %w", c.Mail.From, err))
	}
	if len(c.Mail.To) == 0 {
		errs = append(errs, errors.New("config: mail.to must contain at least one recipient"))
	}
	for _, addr := range c.Mail.To {
		if _, err := netmail.ParseAddress(addr); err != nil {
			errs = append(errs, fmt.Errorf("config: mail.to %q is invalid: %w", addr, err))
		}
	}
	if !mail.IsValidTemplate(c.Mail.Template) {
		errs = append(errs, fmt.Errorf("config: mail.template %q must be one of %s",
			c.Mail.Template, strings.Join(mail.TemplateNames(), ", ")))
	}
	if !validLevel(c.Log.Level) {
		errs = append(errs, fmt.Errorf("config: log.level %q must be one of debug, info, warn, error", c.Log.Level))
	}
	if c.Log.Format != "text" && c.Log.Format != "json" {
		errs = append(errs, fmt.Errorf("config: log.format %q must be text or json", c.Log.Format))
	}
	// The HTTP write deadline must outlast a send so a slow Microsoft Graph
	// call cannot trip it mid-response.
	if c.Server.WriteTimeout <= c.Mail.SendTimeout {
		errs = append(errs, fmt.Errorf(
			"config: server.writeTimeout (%s) must be greater than mail.sendTimeout (%s)",
			c.Server.WriteTimeout.Std(), c.Mail.SendTimeout.Std()))
	}

	return errors.Join(errs...)
}

func validLevel(level string) bool {
	switch level {
	case "debug", "info", "warn", "error":
		return true
	default:
		return false
	}
}

// splitList splits a comma-separated string, trimming whitespace and dropping
// empty entries.
func splitList(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
