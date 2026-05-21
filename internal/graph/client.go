// Package graph implements a Microsoft Graph API client that sends e-mail via
// the sendMail endpoint, authenticating with the OAuth2 client-credentials flow.
package graph

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/clientcredentials"
)

const (
	defaultGraphBaseURL = "https://graph.microsoft.com/v1.0"
	graphScope          = "https://graph.microsoft.com/.default"
	defaultTimeout      = 20 * time.Second
	maxRetryDelay       = 30 * time.Second
)

// Sender sends an e-mail message. The HTTP-backed Client implements it; tests
// substitute their own implementations.
type Sender interface {
	SendMail(ctx context.Context, msg Outgoing) error
}

// Outgoing is a transport-neutral e-mail to be sent.
type Outgoing struct {
	From            string
	To              []string
	Cc              []string
	Bcc             []string
	Subject         string
	HTMLBody        string
	SaveToSentItems bool
}

// Options configures a Client.
type Options struct {
	TenantID     string
	ClientID     string
	ClientSecret string
	// TokenURL overrides the OAuth2 token endpoint. Empty uses the standard
	// Azure endpoint derived from TenantID.
	TokenURL string
	// GraphBaseURL overrides the Microsoft Graph base URL. Empty uses the
	// production endpoint.
	GraphBaseURL string
	// HTTPClient is the base HTTP client used for both token and API requests.
	// Empty creates one with the configured Timeout.
	HTTPClient *http.Client
	// From is the default sender mailbox address.
	From string
	// Timeout applies to the default HTTP client when HTTPClient is nil.
	Timeout time.Duration
	// MaxRetries bounds retries on HTTP 429. Zero applies the default of 1.
	MaxRetries int
}

// Client sends e-mail through the Microsoft Graph API.
type Client struct {
	httpClient *http.Client
	graphBase  string
	from       string
	maxRetries int
}

var _ Sender = (*Client)(nil)

// New builds a Client. It constructs an OAuth2 client-credentials token source
// and wraps the base HTTP client so every request carries a cached bearer
// token that is refreshed automatically on expiry.
func New(opts Options) (*Client, error) {
	if opts.TenantID == "" {
		return nil, errors.New("graph: tenantID is required")
	}
	if opts.ClientID == "" || opts.ClientSecret == "" {
		return nil, errors.New("graph: clientID and clientSecret are required")
	}

	tokenURL := opts.TokenURL
	if tokenURL == "" {
		tokenURL = fmt.Sprintf("https://login.microsoftonline.com/%s/oauth2/v2.0/token", opts.TenantID)
	}

	base := opts.HTTPClient
	if base == nil {
		timeout := opts.Timeout
		if timeout <= 0 {
			timeout = defaultTimeout
		}
		base = &http.Client{Timeout: timeout}
	}

	ccfg := clientcredentials.Config{
		ClientID:     opts.ClientID,
		ClientSecret: opts.ClientSecret,
		TokenURL:     tokenURL,
		Scopes:       []string{graphScope},
		AuthStyle:    oauth2.AuthStyleInParams,
	}
	// Routing the base client through the oauth2 context makes both the token
	// fetch and the Graph API call use the same (test-injectable) transport.
	tokenCtx := context.WithValue(context.Background(), oauth2.HTTPClient, base)
	tokenClient := ccfg.Client(tokenCtx)

	graphBase := opts.GraphBaseURL
	if graphBase == "" {
		graphBase = defaultGraphBaseURL
	}

	maxRetries := opts.MaxRetries
	if maxRetries <= 0 {
		maxRetries = 1
	}

	return &Client{
		httpClient: tokenClient,
		graphBase:  strings.TrimRight(graphBase, "/"),
		from:       opts.From,
		maxRetries: maxRetries,
	}, nil
}
